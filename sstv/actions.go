package sstv

import (
	"encoding/xml"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"

	"github.com/gorilla/mux"
)

// ServeChanList Serve the combined m3u playlist
func ServeChanList(runtime RuntimeUtils) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Received chanList request from %s\n", r.RemoteAddr)

		baseURL := GetConfig().BaseURL

		if len(baseURL) == 0 {
			baseURL = fmt.Sprintf("http://%s", r.Host)
		}
		log.Printf("Using base url: '%s'", baseURL)

		baseChan := make(chan string)
		go getBasem3u(baseChan)

		epgChan := make(chan SSEpg)
		go getSsJSONEpg(runtime, epgChan)

		chanChan := make(chan string)

		go func() {
			defer close(chanChan)
			for _, channel := range (<-epgChan).Channels {
				chanID := fmt.Sprintf("SSTV-%s", channel.Number)
				chanChan <- fmt.Sprintf("#EXTINF:-1 tvg-id=\"%s\" tvg-logo=\"%s\", %s\n", chanID, channel.Img, channel.Name)
				chanChan <- fmt.Sprintf("%s/c/%s\n", baseURL, channel.Number)
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
}

// ServeChanRedir Redirect to authenticated m3u8 stream
func ServeChanRedir(runtime RuntimeUtils) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		c := make(chan string)
		go getAuth(runtime, c)
		chanStr := mux.Vars(r)["chan"]
		channel, err := strconv.Atoi(chanStr)
		if err != nil {
			w.WriteHeader(404)
			w.Write([]byte(fmt.Sprintf("No channel found for %s", chanStr)))
			return
		}
		log.Printf("Creating url for chan %d...", channel)
		url := fmt.Sprintf("https://deu-uk1.SmoothStreams.tv/viewss/ch%02dq1.stream/playlist.m3u8?wmsAuthSign=%s", channel, <-c)
		log.Printf("Url created... %s", url)
		http.Redirect(w, r, url, http.StatusFound)
	}
}

// ServeEPG Serve the combined EPG
func ServeEPG(runtime RuntimeUtils) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		baseChan := make(chan string)
		go getBaseEpg(baseChan)

		epgChan := make(chan SSEpg)
		go getSsJSONEpg(runtime, epgChan)

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
			chanID := fmt.Sprintf("SSTV-%s", channel.Number)
			resultEpg.Channel = append(resultEpg.Channel, Channel{
				ID: chanID,
				DisplayName: TextLang{
					Lang: "en",
					Text: channel.Name,
				},
			})
			for _, event := range channel.Events {
				prog := Programme{
					Title: TextLang{
						Text: event.Name,
						Lang: "en",
					},
					Channel: chanID,
					Start:   event.Start.Format(timeFormat),
					Stop:    event.Stop.Format(timeFormat),
				}
				if len(event.Description) > 0 {
					prog.Desc = &TextLang{
						Text: event.Description,
						Lang: "en",
					}
				}
				resultEpg.Programme = append(resultEpg.Programme, prog)
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
}
