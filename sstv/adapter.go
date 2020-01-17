package sstv

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/mitchellh/mapstructure"
)

func GetSsJsonEpg(c chan SSEpg) {
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
		go cache(client, cacheKey, jsonFeed, 1)
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

func GetBasem3u(c chan string) {
	BASE_M3U := os.Getenv("M3U_BASE")
	log.Println("Creating base...")
	if BASE_M3U != "" {
		go getFile(c, BASE_M3U)
		log.Printf("sent request for base at '%s'", BASE_M3U)
	} else {
		c <- "#EXTM3U\n"
		log.Println("No base set, sent empty string")
	}
}

func GetBaseEpg(c chan string) {
	BASE_EPG := os.Getenv("EPG_BASE")
	if BASE_EPG != "" {
		go getFile(c, BASE_EPG)
		log.Printf("sent request for base at '%s'", BASE_EPG)
	} else {
		c <- "<tv></tv>"
		log.Println("No base set, sent empty string")
	}
}

func GetAuth(c chan string) {
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
