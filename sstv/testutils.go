package sstv

import (
	"fmt"
	"time"
)

// FakeCache to be used for testing
type FakeCache struct {
	GetFunc func(string) (string, error)
	SetFunc func(string, string, time.Duration) error
}

// Get Getter overloaded by GetFunc
func (r *FakeCache) Get(key string) (string, error) {
	if r.GetFunc != nil {
		return r.GetFunc(key)
	}
	return "", fmt.Errorf("Get %s Error", key)
}

// Set Setter overloaded by SetFunc
func (r *FakeCache) Set(key string, value string, exp time.Duration) error {
	if r.SetFunc != nil {
		return r.SetFunc(key, value, exp)
	}
	return fmt.Errorf("Set %s: %s Error", key, value)
}
