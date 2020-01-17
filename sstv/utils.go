package sstv

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"time"
)

func epochToTime(s string) (time.Time, error) {
	sec, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(sec, 0), nil
}

func cache(client CacheClient, key string, value string, minutes int64) error {
	dur, _ := time.ParseDuration(fmt.Sprintf("%dm", minutes))
	error := client.Set(key, value, dur)
	if error != nil {
		log.Printf("Error setting value in cache: %s", error)
	} else {
		log.Printf("Cached %s", key)
	}
	return error
}

func getFile(c chan string, url string) {
	defer close(c)
	log.Printf("Getting url: '%s'", url)
	client := http.Client{
		Timeout: time.Duration(15 * time.Second),
	}
	resp, err := client.Get(url)
	if err != nil {
		log.Printf("Error in http get: %s", err)
		return
	}
	defer resp.Body.Close()
	log.Printf("Received status %d for %s", resp.StatusCode, url)
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
