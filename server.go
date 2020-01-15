package main

import (
    "context"
    "encoding/xml"
    "fmt"
    "log"
    "net/http"
    "os"
    "os/signal"
    "regexp"
    "strconv"
    "syscall"
    "time"

    "github.com/gorilla/mux"
    "github.com/sindrig/sstv-go/sstv"
    "gopkg.in/natefinch/lumberjack.v2"
)

func chanList(w http.ResponseWriter, r *http.Request) {
    log.Printf("Received chanList request from %s\n", r.RemoteAddr)

    baseChan := make(chan string)
    sstv.GetBasem3u(baseChan)

    epgChan := make(chan sstv.SSEpg)
    go sstv.GetSsJsonEpg(epgChan)

    chanChan := make(chan string)

    go func() {
        defer close(chanChan)
        for _, channel := range (<-epgChan).Channels {
            chanId := fmt.Sprintf("SSTV-%s", channel.Number)
            chanChan <- fmt.Sprintf("#EXTINF:-1 tvg-id=\"%s\" tvg-logo=\"%s\", %s\n", chanId, channel.Img, channel.Name)
            chanChan <- fmt.Sprintf("http://%s/c/%s\n", r.Host, channel.Number)
        }
    }()

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

func chanRedir(w http.ResponseWriter, r *http.Request) {
    c := make(chan string)
    go sstv.GetAuth(c)
    chanStr := mux.Vars(r)["chan"]
    channel, err := strconv.Atoi(chanStr)
    if err != nil {
        w.WriteHeader(404)
        w.Write([]byte(fmt.Sprintf("No channel found for %s", chanStr)))
        return
    }
    log.Printf("Creating url for chan %s...", channel)
    url := fmt.Sprintf("https://deu-uk1.SmoothStreams.tv/viewss/ch%02dq1.stream/playlist.m3u8?wmsAuthSign=%s", channel, <-c)
    log.Printf("Url created... %s", url)
    http.Redirect(w, r, url, http.StatusFound)
}

func epg(w http.ResponseWriter, r *http.Request) {
    baseChan := make(chan string)
    go sstv.GetBaseEpg(baseChan)

    epgChan := make(chan sstv.SSEpg)
    go sstv.GetSsJsonEpg(epgChan)

    base, _ := <-baseChan
    re := regexp.MustCompile(`\r?\n`)
    base = re.ReplaceAllString(base, "")

    var resultEpg sstv.EPG
    if err := xml.Unmarshal([]byte(string(base)), &resultEpg); err != nil {
        log.Printf("Could not unmarshal: %s", base)
        w.Write([]byte(base))
        return
    }

    log.Printf("Got channels: %d", len(resultEpg.Channel))

    epgData := <-epgChan
    timeFormat := "20060102150405 +0000"

    for _, channel := range epgData.Channels {
        chanId := fmt.Sprintf("SSTV-%s", channel.Number)
        resultEpg.Channel = append(resultEpg.Channel, sstv.Channel{
            ID: chanId,
            DisplayName: sstv.TextLang{
                Lang: "en",
                Text: channel.Name,
            },
        })
        for _, event := range channel.Events {
            resultEpg.Programme = append(resultEpg.Programme, sstv.Programme{
                Title: sstv.TextLang{
                    Text: event.Name,
                    Lang: "en",
                },
                Channel: chanId,
                Desc: sstv.TextLang{
                    Text: event.Description,
                    Lang: "en",
                },
                Start: event.Start.Format(timeFormat),
                Stop:  event.Stop.Format(timeFormat),
            })
        }
    }
    log.Printf("Got channels: %d", len(resultEpg.Channel))
    result, err := xml.MarshalIndent(resultEpg, "", "    ")
    if err != nil {
        log.Printf("Could not marshal result: %s", err)
        w.WriteHeader(500)
        w.Write([]byte(err.Error()))
    } else {
        w.Header().Set("Content-Type", "text/xml")
        w.Write([]byte("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n"))
        w.Write(result)
    }

}

func logRequest(handler http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        log.Printf("%s %s %s\n", r.RemoteAddr, r.Method, r.URL)
        handler.ServeHTTP(w, r)
    })
}

func main() {
    r := mux.NewRouter()

    r.HandleFunc("/c", chanList)
    r.HandleFunc("/c/{chan}", chanRedir)
    r.HandleFunc("/g", epg)

    srv := &http.Server{
        Handler:      logRequest(r),
        Addr:         ":8080",
        ReadTimeout:  10 * time.Second,
        WriteTimeout: 10 * time.Second,
    }

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
