package sstv

import (
	"time"
)

// CacheClient Interface for cacheclient
type CacheClient interface {
	Get(key string) (string, error)
	Set(key string, value string, expiration time.Duration) error
}

// RuntimeUtils should contain everything external
type RuntimeUtils struct {
	Cache CacheClient
}
