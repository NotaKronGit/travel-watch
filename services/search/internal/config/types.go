package config

import (
	"github.com/NotaKronGit/travel-watch/api/mtls"
	"github.com/NotaKronGit/travel-watch/services/search/internal/gemini"
	"github.com/NotaKronGit/travel-watch/services/search/internal/realroutes"
	"time"
)

type Config struct {
	Results  Results           `mapstructure:"results"`
	Progress Progress          `mapstructure:"progress"`
	Planner  realroutes.Config `mapstructure:"planner"`
	Airports Airports          `mapstructure:"airports"`
	Gemini   gemini.Config     `mapstructure:"gemini"`
	Database Database          `mapstructure:"database"`
	Consumer Consumer          `mapstructure:"consumer"`
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

type Airports struct {
	URL                string        `mapstructure:"url"`
	DownloadTimeout    time.Duration `mapstructure:"download_timeout"`
	ImportTimeout      time.Duration `mapstructure:"import_timeout"`
	MaxBytes           int64         `mapstructure:"max_bytes"`
	MinRows            int           `mapstructure:"min_rows"`
	MaxRows            int           `mapstructure:"max_rows"`
	MinRetainedPercent int           `mapstructure:"min_retained_percent"`
}

// Progress transport is separate from Cabinet's request topic.
type Progress struct {
	Sources      []string      `mapstructure:"sources"`
	Brokers      []string      `mapstructure:"brokers"`
	Topic        string        `mapstructure:"topic"`
	GroupID      string        `mapstructure:"group_id"`
	PollInterval time.Duration `mapstructure:"poll_interval"`
	Timeout      time.Duration `mapstructure:"timeout"`
	Lease        time.Duration `mapstructure:"lease"`
	MaxAttempts  int           `mapstructure:"max_attempts"`
}

type Results struct {
	Address         string        `mapstructure:"address"`
	Timeout         time.Duration `mapstructure:"timeout"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
	TLS             mtls.Config   `mapstructure:"tls"`
}
