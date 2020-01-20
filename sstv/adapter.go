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

// getBasem3u Fetch the base m3u (or only header if not set)
func getBasem3u(c chan string) {
	cfg := GetConfig()

	log.Println("Creating base...")
	if len(cfg.M3uBase) > 0 {
		go getFile(c, cfg.M3uBase)
		log.Printf("sent request for base at '%s'", cfg.M3uBase)
	} else {
		c <- "#EXTM3U\n"
		log.Println("No base set, sent empty string")
	}
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
