package main

import (
    "context"
    "encoding/json"
    "fmt"
    "io/ioutil"
    "log"
    "net/http"
    "net/url"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/go-redis/redis/v7"
    "github.com/gorilla/mux"
    "gopkg.in/natefinch/lumberjack.v2"
)

func root(w http.ResponseWriter, r *http.Request) {
    log.Printf("Accept: %s", r.Header.Get("Accept"))
    w.Write([]byte(""))
}

func chanList(w http.ResponseWriter, r *http.Request) {
    log.Printf("Received chanList request from %s\n", r.RemoteAddr)
    BASE_M3U := os.Getenv("M3U_BASE")
    baseChan := make(chan string)
    log.Println("Creating base...")
    if BASE_M3U != "" {
        go getFile(baseChan, BASE_M3U)
        log.Printf("sent request for base at '%s'", BASE_M3U)
    } else {
        go func(c chan string) {
            c <- "#EXTM3U\n"
        }(baseChan)
        log.Println("No base set, sent empty string")
    }

    chanChan := make(chan string)
    go ssChans(chanChan, r.Host)

    base, ok := <-baseChan
    if ok {
        w.Write([]byte(base))
    }

    for {
        val, ok := <-chanChan
        if ok {
            w.Write([]byte(val))
        } else {
            log.Println("Breaking out of chanChan loop")
            break
        }
    }

}

func ssChans(c chan string, host string) {
    defer close(c)
    for i := 1; i <= 150; i++ {
        c <- fmt.Sprintf("#EXTINF:-1 tvg-id=\"SSTV-%02d\" tvg-logo=\"\", SmoothStreams %d\n", i, i)
        c <- fmt.Sprintf("http://%s/channel/%02d\n", host, i)
    }
}

func getFile(c chan string, url string) {
    defer close(c)
    log.Printf("Getting url: '%s'", url)
    resp, err := http.Get(url)
    if err != nil {
        log.Printf("Error in http get: %s", err)
        return
    }
    defer resp.Body.Close()

    if resp.StatusCode == http.StatusOK {
        bodyBytes, err := ioutil.ReadAll(resp.Body)
        if err != nil {
            log.Printf("Error in http get, reading body: %s", err)
            return
        }
        bodyString := string(bodyBytes)
        c <- bodyString
    }
}

type AuthResponse struct {
    Hash  string
    Valid int64
    Code  string
    Error string
}

func getAuth(c chan string) {
    defer close(c)
    cacheKey := "authHash"
    redisUrl := os.Getenv("REDIS_URL")
    if len(redisUrl) == 0 {
        redisUrl = "localhost:6379"
    }
    client := redis.NewClient(&redis.Options{
        Addr:     redisUrl,
        Password: "",
        DB:       0,
    })
    val, err := client.Get(cacheKey).Result()
    if err != nil {
        log.Printf("Error getting auth: %s", err)
    }
    if len(val) > 0 {
        c <- val
        log.Println("Got auth from cache")
        return
    }

    response, err := http.PostForm("https://auth.smoothstreams.tv/hash_api.php", url.Values{
        "username": {os.Getenv("USERNAME")},
        "password": {os.Getenv("PASSWORD")},
        "site":     {"viewss"},
    })

    if err != nil {
        log.Printf("Error in postform: %s", err)
        return
    }

    defer response.Body.Close()
    body, err := ioutil.ReadAll(response.Body)

    if err != nil {
        log.Printf("Error in read response: %s", err)
        return
    }

    var auth AuthResponse
    jsonErr := json.Unmarshal([]byte(string(body)), &auth)

    if jsonErr != nil {
        log.Printf("jsonErr unmarshaling: %s", jsonErr)
        return
    }

    if auth.Code != "1" {
        log.Printf("Auth Code: %s. Error: %s", auth.Code, auth.Error)
        return
    }

    c <- auth.Hash

    dur, _ := time.ParseDuration(fmt.Sprintf("%dm", auth.Valid))
    log.Printf("Expires in %.2f minutes", dur.Minutes())
    error := client.Set(cacheKey, auth.Hash, dur).Err()
    if error != nil {
        log.Printf("Error setting value in cache: %s", error)
        return
    }
}

func chanRedir(w http.ResponseWriter, r *http.Request) {
    c := make(chan string)
    go getAuth(c)
    channel := mux.Vars(r)["chan"]
    log.Printf("Creating url...")
    url := fmt.Sprintf("https://deu.SmoothStreams.tv/viewss/ch%sq1.stream/playlist.m3u8?wmsAuthSign=%s", channel, <-c)
    log.Printf("Url created...")
    http.Redirect(w, r, url, http.StatusFound)
}

func main() {
    // Create Server and Route Handlers
    r := mux.NewRouter()

    r.HandleFunc("/", root)
    r.HandleFunc("/channels.m3u", chanList)
    r.HandleFunc("/channel/{chan}", chanRedir)

    srv := &http.Server{
        Handler:      r,
        Addr:         ":8080",
        ReadTimeout:  10 * time.Second,
        WriteTimeout: 10 * time.Second,
    }

    // Configure Logging
    LOG_FILE_LOCATION := os.Getenv("LOG_FILE_LOCATION")
    if LOG_FILE_LOCATION != "" {
        log.SetOutput(&lumberjack.Logger{
            Filename:   LOG_FILE_LOCATION,
            MaxSize:    500, // megabytes
            MaxBackups: 3,
            MaxAge:     28,   //days
            Compress:   true, // disabled by default
        })
    }

    // Start Server
    go func() {
        log.Println("Starting Server")
        if err := srv.ListenAndServe(); err != nil {
            log.Fatal(err)
        }
    }()

    // Graceful Shutdown
    waitForShutdown(srv)
}

func waitForShutdown(srv *http.Server) {
    interruptChan := make(chan os.Signal, 1)
    signal.Notify(interruptChan, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

    // Block until we receive our signal.
    <-interruptChan

    // Create a deadline to wait for.
    ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
    defer cancel()
    srv.Shutdown(ctx)

    log.Println("Shutting down")
    os.Exit(0)
}
