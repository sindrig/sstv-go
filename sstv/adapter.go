package sstv

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"time"

	"github.com/mitchellh/mapstructure"
)

// getSsJSONEpg Get the EPG from SS
func getSsJSONEpg(runtime RuntimeUtils, c chan SSEpg) {
	defer close(c)
	cacheKey := "ssJsonEpgFeed"
	var jsonData map[string]interface{}
	jsonFeed, err := runtime.Cache.Get(cacheKey)
	if err == nil && len(jsonFeed) > 0 {
		log.Println("Got jsonFeed from cache")
	} else {
		u, err := url.Parse(GetConfig().JSONTVUrl)
		if err != nil {
			log.Fatal("Could not parse json tv url...")
		}
		feed, _ := url.Parse("feed-new.json")
		feedChan := make(chan string)
		go getFile(feedChan, u.ResolveReference(feed).String())

		jsonFeed, _ = <-feedChan
		go cache(runtime.Cache, cacheKey, jsonFeed, 1)
	}

	var epg SSEpg
	if err := json.Unmarshal([]byte(string(jsonFeed)), &jsonData); err != nil {
		log.Printf("Could not unmarshal: %s\n\n%s", err, jsonFeed)
	} else {
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
	}

	c <- epg

}

// getBasem3u Base m3u with header and static channels
func getBasem3u(c chan string, baseURL string) {
	defer close(c)
	c <- fmt.Sprintf("#EXTM3U x-tvg-url=\"%s/g\"\n", baseURL)
	c <- "#EXTINF:-1 tvg-id=\"RÚV\" tvg-logo=\"http://iptv.irdn.is/images/ruv.png\", RÚV\n"
	c <- fmt.Sprintf("%s/ruv/%s\n", baseURL, "ruv")
	c <- "#EXTINF:-1 tvg-id=\"RÚV Íþróttir\" tvg-logo=\"http://iptv.irdn.is/images/ruv2.png\", RÚV Íþróttir\n"
	c <- fmt.Sprintf("%s/ruv/%s\n", baseURL, "ruv2")
	c <- "#EXTINF:-1 tvg-id=\"N4\" tvg-logo=\"http://iptv.irdn.is/images/n4.png\", N4\n"
	c <- "http://tv.vodafoneplay.is/n4/index.m3u8\n"
	c <- "#EXTINF:-1 tvg-id=\"Stöð 2\" tvg-logo=\"http://iptv.irdn.is/images/stod2.png\", Stöð 2\n"
	c <- "http://visirlive.365cdn.is/hls-live/stod2.smil/playlist.m3u8\n"
	c <- "#EXTINF:-1 tvg-id=\"Stöð 2 Sport\" tvg-logo=\"http://iptv.irdn.is/images/stod2sport.png\", Stöð 2 Sport\n"
	c <- "https://visirlive.365cdn.is/hls-live/straumur05.smil/playlist.m3u8\n"
	c <- "#EXTINF:-1 tvg-id=\"Alþingi\" tvg-logo=\"http://iptv.irdn.is/images/althingi.png\", Alþingi\n"
	c <- "http://5-226-137-173.netvarp.is/althingi_600/index.m3u8\n"
}

// getBaseEpg Fetch the base EPG (or only scaffold if empty)
func getBaseEpg(c chan string) {
	cfg := GetConfig()
	if len(cfg.EpgBase) > 0 {
		go getFile(c, cfg.EpgBase)
		log.Printf("sent request for base at '%s'", cfg.EpgBase)
	} else {
		c <- "<tv></tv>"
		log.Println("No base set, sent empty string")
	}
}

// getAuth Get authentication hash for ss
func getAuth(runtime RuntimeUtils, c chan string) {
	defer close(c)
	cfg := GetConfig()
	cacheKey := "authHash"
	val, err := runtime.Cache.Get(cacheKey)
	if err != nil {
		log.Printf("Error getting auth: %s", err)
	}
	if len(val) > 0 {
		c <- val
		log.Println("Got auth from cache")
		return
	}

	response, err := http.PostForm("https://auth.smoothstreams.tv/hash_api.php", url.Values{
		"username": {cfg.Username},
		"password": {cfg.Password},
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

	go cache(runtime.Cache, cacheKey, auth.Hash, auth.Valid)
}

func getRuvStream(c chan string, channel string) {
	defer close(c)
	if GetConfig().RuvUseGeoblocked {
		u, err := url.Parse(GetConfig().RuvAPIURL)
		if err != nil {
			log.Fatal("Could not parse ruv api url...")
		}
		query := u.Query()
		query.Add("channel", channel)
		u.RawQuery = query.Encode()

		fileChan := make(chan string)
		go getFile(fileChan, u.String())

		resultBody, ok := <-fileChan
		log.Printf("result: %s", resultBody)
		if !ok {
			log.Printf("Did not receive result from ruv stream")
			return
		}

		var result RuvChannelResponse
		jsonErr := json.Unmarshal([]byte(string(resultBody)), &result)

		if jsonErr != nil {
			log.Printf("RUV: jsonErr unmarshaling: %s", jsonErr)
			return
		}

		c <- result.Result[0]
	} else {
		c <- fmt.Sprintf("http://ruvruv-live.hls.adaptive.level3.net/ruv/%s/index.m3u8", channel)
	}
}
