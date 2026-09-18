package config

import (
	"errors"
	"strings"
	"time"

	"github.com/NotaKronGit/travel-watch/services/collector/internal/flights/fli"
	"github.com/NotaKronGit/travel-watch/services/collector/internal/rail/yandex"
	"github.com/spf13/viper"
)

type Config struct {
	Fli    fli.Config `mapstructure:"fli"`
	Yandex struct {
		APIKey           string        `mapstructure:"api_key"`
		Timeout          time.Duration `mapstructure:"timeout"`
		MaxResponseBytes int64         `mapstructure:"max_response_bytes"`
	} `mapstructure:"yandex"`
}

func (c Config) ProviderConfig() yandex.Config {
	return yandex.Config{APIKey: c.Yandex.APIKey, Timeout: c.Yandex.Timeout, MaxResponseBytes: c.Yandex.MaxResponseBytes}
}
func readConfig(path string) (Config, error) {
	v := viper.NewWithOptions(viper.ExperimentalBindStruct())
	v.SetEnvPrefix("COLLECTOR")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	v.AllowEmptyEnv(true)
	v.SetConfigFile(path)
	v.SetConfigType("yaml")
	if err := v.ReadInConfig(); err != nil {
		return Config{}, errors.New("cannot read Collector YAML configuration")
	}
	var c Config
	if err := v.UnmarshalExact(&c); err != nil {
		return Config{}, errors.New("invalid Collector configuration")
	}
	return c, nil
}

func Load(path string) (Config, error) {
	c, err := readConfig(path)
	if err != nil {
		return Config{}, err
	}
	if _, err := yandex.New(c.ProviderConfig()); err != nil {
		return Config{}, err
	}
	return c, nil
}

// LoadFlights validates only the explicitly selected flight provider.
func LoadFlights(path string) (Config, error) {
	c, err := readConfig(path)
	if err != nil {
		return Config{}, err
	}
	if err = c.Fli.Validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}
