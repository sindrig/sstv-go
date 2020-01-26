package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-redis/redis"
	"github.com/gorilla/mux"
	"github.com/sindrig/sstv-go/sstv"
)

func logRequest(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s %s\n", r.RemoteAddr, r.Method, r.URL)
		handler.ServeHTTP(w, r)
	})
}

// Redis Real redis client
type Redis struct {
	c *redis.Client
}

// Get Wrapper for redis.Get
func (r Redis) Get(key string) (string, error) {
	return r.c.Get(key).Result()
}

// Set Wrapper for redis.set
func (r Redis) Set(key string, value string, expr time.Duration) error {
	return r.c.Set(key, value, expr).Err()
}

func k8sProbe(w http.ResponseWriter, r *http.Request) {
	// TODO: Check redis connection? Something.
	w.WriteHeader(200)
	w.Write([]byte("Ready!"))
}

func main() {
	r := mux.NewRouter()

	runtime := sstv.RuntimeUtils{
		Cache: Redis{
			c: redis.NewClient(&redis.Options{
				Addr:     sstv.GetConfig().RedisURL,
				Password: "",
				DB:       0,
			}),
		},
	}

	r.HandleFunc("/c", sstv.ServeChanList(runtime))
	r.HandleFunc("/c/{chan}", sstv.ServeChanRedir(runtime))
	r.HandleFunc("/ruv/{chan}", sstv.ServeRuvRedir(runtime))
	r.HandleFunc("/g", sstv.ServeEPG(runtime))
	r.HandleFunc("/ready", k8sProbe)

	addr := fmt.Sprintf(":%s", sstv.GetConfig().Port)
	srv := &http.Server{
		Handler:      logRequest(r),
		Addr:         addr,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	// Start Server
	go func() {
		log.Printf("Starting Server on %s", addr)
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
