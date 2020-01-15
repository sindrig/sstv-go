package sstv

import (
	// "encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	// "net/url"
	"os"
	"strconv"
	"time"

	"github.com/go-redis/redis/v7"
)

func epochToTime(s string) (time.Time, error) {
	sec, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(sec, 0), nil
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

func cache(client *redis.Client, key string, value string, minutes int64) {
	dur, _ := time.ParseDuration(fmt.Sprintf("%dm", minutes))
	error := client.Set(key, value, dur).Err()
	if error != nil {
		log.Printf("Error setting value in cache: %s", error)
	} else {
		log.Printf("Cached %s", key)
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
