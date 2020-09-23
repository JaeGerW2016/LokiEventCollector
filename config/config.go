package config

import (
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"time"
)

type Config struct {
	Log       string
	Receivers struct {
		Loki   *Loki
		Stdout bool
	}
}

type Loki struct {
	URL                string
	Labels             string
	BatchWait          time.Duration
	BatchEntriesNumber int
}

var C *Config

func InitConfig(configFile string) {
	if configFile != "" {
		viper.SetConfigFile(configFile)
	} else {
		viper.SetConfigName("config")
		viper.AddConfigPath("/etc/loki-event-collector")
		viper.AddConfigPath("$HOME/.config/loki-event-collector")
		viper.AddConfigPath(".")
	}

	if err := viper.ReadInConfig(); err != nil {
		logrus.Fatalf("read config file： %v", err)
	}

	if err := viper.Unmarshal(&C); err != nil {
		logrus.Fatalf("unmarshal config to strut: %v", err)
	}

}
