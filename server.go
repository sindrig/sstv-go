package main

import (
    "context"
    "encoding/json"
    "encoding/xml"
    "fmt"
    "io/ioutil"
    "log"
    "net/http"
    "net/url"
    "os"
    "os/signal"
    "regexp"
    "sort"
    "strconv"
    "syscall"
    "time"

    "github.com/go-redis/redis/v7"
    "github.com/gorilla/mux"
    "github.com/mitchellh/mapstructure"
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

    epgChan := make(chan SSEpg)
    go getSsJsonEpg(epgChan)

    chanChan := make(chan string)

    go ssChans(chanChan, <-epgChan, r.Host)

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

func ssChans(c chan string, epgData SSEpg, host string) {
    defer close(c)
    for _, channel := range epgData.Channels {
        chanId := fmt.Sprintf("SSTV-%s", channel.Number)
        c <- fmt.Sprintf("#EXTINF:-1 tvg-id=\"%s\" tvg-logo=\"%s\", %s\n", chanId, channel.Img, channel.Name)
        c <- fmt.Sprintf("http://%s/channel/%s\n", host, channel.Number)
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

func getRedisClient() *redis.Client {
    redisUrl := os.Getenv("REDIS_URL")
    if len(redisUrl) == 0 {
        redisUrl = "localhost:6379"
    }
    return redis.NewClient(&redis.Options{
        Addr:     redisUrl,
        Password: "",
        DB:       0,
    })
}

func getAuth(c chan string) {
    defer close(c)
    cacheKey := "authHash"
    client := getRedisClient()
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

    go cache(client, cacheKey, auth.Hash, auth.Valid)
}

func chanRedir(w http.ResponseWriter, r *http.Request) {
    c := make(chan string)
    go getAuth(c)
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

type TextLang struct {
    Text string `xml:",chardata"`
    Lang string `xml:"lang,attr"`
}

type Channel struct {
    Text        string   `xml:",chardata"`
    ID          string   `xml:"id,attr"`
    DisplayName TextLang `xml:"display-name"`
    URL         string   `xml:"url,omitempty"`
}

type Programme struct {
    Text     string    `xml:",chardata"`
    Start    string    `xml:"start,attr"`
    Stop     string    `xml:"stop,attr"`
    Channel  string    `xml:"channel,attr"`
    Title    TextLang  `xml:"title"`
    SubTitle *TextLang `xml:"sub-title,omitempty"`
    Desc     TextLang  `xml:"desc"`
}

type EPG struct {
    XMLName           xml.Name    `xml:"tv"`
    Text              string      `xml:",chardata"`
    GeneratorInfoName string      `xml:"generator-info-name,attr"`
    GeneratorInfoURL  string      `xml:"generator-info-url,attr"`
    Channel           []Channel   `xml:"channel"`
    Programme         []Programme `xml:"programme"`
}

type JSONEvent struct {
    Name        string
    Description string
    Time        string
    Runtime     string
    Category    string
}

func epochToTime(s string) (time.Time, error) {
    sec, err := strconv.ParseInt(s, 10, 64)
    if err != nil {
        return time.Time{}, err
    }
    return time.Unix(sec, 0), nil
}

func cache(client *redis.Client, key string, value string, minutes int64) {
    dur, _ := time.ParseDuration(fmt.Sprintf("%dm", minutes))
    error := client.Set(key, value, dur).Err()
    if error != nil {
        log.Printf("Error setting value in cache: %s", error)
    } else {
        log.Printf("Cached %s", key)
    }
}

type SSEpgEvent struct {
    Name        string
    Description string
    Category    string
    Start       time.Time
    Stop        time.Time
}

type SSEpgChannel struct {
    Number string
    Name   string
    Img    string
    Events []SSEpgEvent
}

type SSEpg struct {
    Channels []SSEpgChannel
}

func getSsJsonEpg(c chan SSEpg) {
    defer close(c)
    client := getRedisClient()
    cacheKey := "ssJsonEpgFeed"
    var jsonData map[string]interface{}
    jsonFeed, err := client.Get(cacheKey).Result()
    if err == nil && len(jsonFeed) > 0 {
        log.Println("Got jsonFeed from cache")
    } else {
        u, err := url.Parse(os.Getenv("JSONTVURL"))
        feed, _ := url.Parse("feed-new.json")
        if err != nil {
            log.Fatal("Could not parse json tv url...")
        }
        feedChan := make(chan string)
        go getFile(feedChan, u.ResolveReference(feed).String())

        jsonFeed, _ = <-feedChan
        log.Println(jsonFeed)
        go cache(client, cacheKey, jsonFeed, 1)
    }

    if err := json.Unmarshal([]byte(string(jsonFeed)), &jsonData); err != nil {
        log.Printf("Could not unmarshal: %s\n\n%s", err, jsonFeed)
    }
    var epg SSEpg
    data := jsonData["data"].(map[string]interface{})

    for _, channelI := range data {
        channel := channelI.(map[string]interface{})
        var events []SSEpgEvent
        switch evs := channel["events"].(type) {
        case map[string]interface{}:
            for _, eventI := range evs {
                var event JSONEvent
                mapstructure.Decode(eventI, &event)
                // event := eventI.(map[string]string)
                startTime, _ := epochToTime(event.Time)
                dur, _ := time.ParseDuration(fmt.Sprintf("%sm", event.Runtime))
                events = append(events, SSEpgEvent{
                    Name:        event.Name,
                    Description: event.Description,
                    Start:       startTime,
                    Stop:        startTime.Add(dur),
                })
            }
        }

        epg.Channels = append(epg.Channels, SSEpgChannel{
            Number: channel["number"].(string),
            Name:   channel["name"].(string),
            Img:    channel["img"].(string),
            Events: events,
        })
    }
    sort.Slice(epg.Channels, func(i, j int) bool {
        a, err1 := strconv.Atoi(epg.Channels[i].Number)
        b, err2 := strconv.Atoi(epg.Channels[j].Number)
        return err1 == nil && err2 == nil && a < b
    })

    c <- epg

}

func epg(w http.ResponseWriter, r *http.Request) {
    cacheKey := "epg"
    log.Println(cacheKey)
    baseChan := make(chan string)
    BASE_EPG := os.Getenv("EPG_BASE")
    if BASE_EPG != "" {
        go getFile(baseChan, BASE_EPG)
        log.Printf("sent request for base at '%s'", BASE_EPG)
    } else {
        go func(c chan string) {
            c <- "<tv></tv>"
        }(baseChan)
        log.Println("No base set, sent empty string")
    }

    epgChan := make(chan SSEpg)
    go getSsJsonEpg(epgChan)

    base, _ := <-baseChan
    re := regexp.MustCompile(`\r?\n`)
    base = re.ReplaceAllString(base, "")

    var resultEpg EPG
    if err := xml.Unmarshal([]byte(string(base)), &resultEpg); err != nil {
        log.Printf("Could not unmarshal: %s", base)
        w.Write([]byte(base))
        return
    }

    log.Printf("Got channels: %d", len(resultEpg.Channel))

    epgData := <-epgChan
    timeFormat := "20060102150405 +0000"

    for _, channel := range epgData.Channels {
        break
        chanId := fmt.Sprintf("SSTV-%s", channel.Number)
        resultEpg.Channel = append(resultEpg.Channel, Channel{
            ID: chanId,
            DisplayName: TextLang{
                Lang: "en",
                Text: channel.Name,
            },
        })
        for _, event := range channel.Events {
            resultEpg.Programme = append(resultEpg.Programme, Programme{
                Title: TextLang{
                    Text: event.Name,
                    Lang: "en",
                },
                Channel: chanId,
                Desc: TextLang{
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
        // w.Write([]byte(base))
        w.Write([]byte("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n"))
        w.Write(result)
        // w.Header().Set("real", string(result))
    }

}

func main() {
    // Create Server and Route Handlers
    r := mux.NewRouter()

    r.HandleFunc("/", root)
    r.HandleFunc("/channels.m3u", chanList)
    r.HandleFunc("/channel/{chan}", chanRedir)
    r.HandleFunc("/guide.xml", epg)

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
