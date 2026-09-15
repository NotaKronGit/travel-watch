package config

import "time"

type Config struct {
	Database Database `mapstructure:"database"`
	Consumer Consumer `mapstructure:"consumer"`
}
type Database struct {
	Host             string        `mapstructure:"host"`
	Port             int           `mapstructure:"port"`
	Name             string        `mapstructure:"name"`
	SSLMode          string        `mapstructure:"sslmode"`
	AppUser          string        `mapstructure:"app_user"`
	AppPassword      string        `mapstructure:"app_password" json:"-"`
	OwnerUser        string        `mapstructure:"owner_user"`
	OwnerPassword    string        `mapstructure:"owner_password" json:"-"`
	ConnectTimeout   time.Duration `mapstructure:"connect_timeout"`
	PingTimeout      time.Duration `mapstructure:"ping_timeout"`
	MaxOpenConns     int           `mapstructure:"max_open_conns"`
	MaxIdleConns     int           `mapstructure:"max_idle_conns"`
	ConnMaxLifetime  time.Duration `mapstructure:"conn_max_lifetime"`
	MigrationTimeout time.Duration `mapstructure:"migration_timeout"`
}

type Consumer struct {
	Brokers       []string      `mapstructure:"brokers"`
	Topic         string        `mapstructure:"topic"`
	GroupID       string        `mapstructure:"group_id"`
	DBTimeout     time.Duration `mapstructure:"db_timeout"`
	CommitTimeout time.Duration `mapstructure:"commit_timeout"`
	DialTimeout   time.Duration `mapstructure:"dial_timeout"`
}
