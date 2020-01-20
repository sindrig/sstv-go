package sstv

import (
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gotest.tools/assert"
)

const FormatString = "2006-01-02T15:04:05"

func TestEpochToTime(t *testing.T) {
	got, err := epochToTime("1579267611")
	if err != nil {
		t.Errorf("Got error converting epoch: %s", err)
	}
	cmp, err := time.Parse(FormatString, "2020-01-17T13:26:51")
	if err != nil {
		t.Errorf("Could not create compare time")
	}
	if !got.Equal(cmp) {
		t.Errorf("DT not equals: %s and %s", got.Format(FormatString), cmp.Format(FormatString))
	}
}

func TestEpochToTimeReturnsErrorForNonNumber(t *testing.T) {
	got, err := epochToTime(("notanumber"))
	if err == nil {
		t.Errorf("epochToTime did not return error but %s", got.Format(FormatString))
	}
	expected := "strconv.ParseInt: parsing \"notanumber\": invalid syntax"
	assert.Error(t, err, expected)
}

func TestCache(t *testing.T) {
	type args struct {
		client  CacheClient
		key     string
		value   string
		minutes int64
	}
	tests := []struct {
		name    string
		args    args
		wantErr error
	}{
		{
			"TestRegularSet",
			args{
				client: &FakeCache{
					SetFunc: func(key string, value string, exp time.Duration) error {
						assert.Equal(t, key, "key")
						assert.Equal(t, value, "value")
						assert.Equal(t, exp.Minutes(), 5.0)
						return nil
					},
				},
				key:     "key",
				value:   "value",
				minutes: 5,
			},
			nil,
		},
		{
			"TestReturnsErrorFromSet",
			args{
				client: &FakeCache{
					SetFunc: func(key string, value string, exp time.Duration) error {
						return errors.New("SomeFakeError")
					},
				},
				key:     "key",
				value:   "value",
				minutes: 5,
			},
			errors.New("SomeFakeError"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := cache(tt.args.client, tt.args.key, tt.args.value, tt.args.minutes); (err != nil) && err.Error() != tt.wantErr.Error() {
				t.Errorf("cache() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGetFile(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Request for %s", r.URL.Path)
		if r.URL.Path == "/500" {
			w.WriteHeader(500)
			w.Write([]byte("Server error"))
			return
		}
		w.WriteHeader(200)
		w.Write([]byte("hai"))
		return
	}))
	defer ts.Close()
	log.Printf("We are at %s", ts.URL)
	type args struct {
		c   chan string
		url string
	}
	tests := []struct {
		name     string
		args     args
		result   string
		received bool
	}{
		{
			name: "TestServerError",
			args: args{
				c:   make(chan string),
				url: ts.URL + "/500",
			},
			result:   "",
			received: false,
		},
		{
			name: "TestReceiveData",
			args: args{
				c:   make(chan string),
				url: ts.URL + "/200",
			},
			result:   "hai",
			received: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			go getFile(tt.args.c, tt.args.url)
			result, received := <-tt.args.c
			assert.Equal(t, result, tt.result)
			assert.Equal(t, received, tt.received)
		})
	}
}
