package sstv

import (
	"log"
	"sync"

	"github.com/kelseyhightower/envconfig"
)

// Config All configuration that sstv logic should need
type Config struct {
	RedisURL  string `envconfig:"REDIS_URL" default:"localhost:6379"`
	EpgBase   string `envconfig:"EPG_BASE"`
	JSONTVUrl string `envconfig:"JSONTVURL" default:"https://fast-guide.smoothstreams.tv/"`
	Username  string `envconfig:"USERNAME"`
	Password  string `envconfig:"PASSWORD"`
	BaseURL   string `envconfig:"BASE_URL"`
	RuvAPIURL string `envconfig:"RUV_API_URL" default:"http://ruv.is/sites/all/themes/at_ruv/scripts/ruv-stream.php?format=json"`
}

var cfg Config
var once sync.Once

// GetConfig get global SSTV configuration
func GetConfig() Config {
	once.Do(func() {
		log.Print("Initializing config...")
		err := envconfig.Process("SSTV", &cfg)
		if err != nil {
			log.Fatalf("Error initializing config: %s", err.Error())
		}
	})
	return cfg
}
