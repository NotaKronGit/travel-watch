package config

import (
	"errors"
	"strings"

	"github.com/spf13/viper"
)

func Load(path, command string) (Config, error) {
	// Bind fields from mapstructure tags, including fields absent from the YAML.
	v := viper.NewWithOptions(viper.ExperimentalBindStruct())
	v.SetEnvPrefix("SEARCH")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")
	if err := v.ReadInConfig(); err != nil {
		// YAML/parser errors may include secret values; do not propagate them.
		return Config{}, errors.New("cannot read Search YAML configuration")
	}
	v.AllowEmptyEnv(true)
	var cfg Config
	if err := v.UnmarshalExact(&cfg); err != nil {
		return Config{}, errors.New("invalid Search configuration: check keys and value types")
	}
	if err := cfg.Validate(command); err != nil {
		return Config{}, err
	}
	return cfg, nil
}
