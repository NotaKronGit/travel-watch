package config

import "time"

type Config struct {
	Progress Progress `mapstructure:"progress"`
	Outbox   Outbox   `mapstructure:"outbox"`
	Server   Server   `mapstructure:"server"`
	Database Database `mapstructure:"database"`
	Catalog  Catalog  `mapstructure:"catalog"`
	Auth     Auth     `mapstructure:"auth"`
}

type Server struct {
	Address           string        `mapstructure:"address"`
	Origin            string        `mapstructure:"origin"`
	ReadHeaderTimeout time.Duration `mapstructure:"read_header_timeout"`
	ReadTimeout       time.Duration `mapstructure:"read_timeout"`
	WriteTimeout      time.Duration `mapstructure:"write_timeout"`
	IdleTimeout       time.Duration `mapstructure:"idle_timeout"`
	ShutdownTimeout   time.Duration `mapstructure:"shutdown_timeout"`
	RequestTimeout    time.Duration `mapstructure:"request_timeout"`
	HealthTimeout     time.Duration `mapstructure:"health_timeout"`
	MaxHeaderBytes    int           `mapstructure:"max_header_bytes"`
	MaxBodyBytes      int           `mapstructure:"max_body_bytes"`
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

type Auth struct {
	CookieSecure        bool          `mapstructure:"cookie_secure"`
	SessionTTL          time.Duration `mapstructure:"session_ttl"`
	CleanupInterval     time.Duration `mapstructure:"cleanup_interval"`
	CleanupTimeout      time.Duration `mapstructure:"cleanup_timeout"`
	LoginAttempts       int           `mapstructure:"login_attempts"`
	LoginWindow         time.Duration `mapstructure:"login_window"`
	MaxTrackedAddresses int           `mapstructure:"max_tracked_addresses"`
	HashConcurrency     int           `mapstructure:"hash_concurrency"`
}

// Catalog settings apply only to the explicit sync-cities command.
type Catalog struct {
	AlternateNamesURL             string        `mapstructure:"alternate_names_url"`
	MaxAlternateDownloadBytes     int64         `mapstructure:"max_alternate_download_bytes"`
	MaxAlternateUncompressedBytes int64         `mapstructure:"max_alternate_uncompressed_bytes"`
	CitiesURL                     string        `mapstructure:"cities_url"`
	RegionsURL                    string        `mapstructure:"regions_url"`
	CountriesURL                  string        `mapstructure:"countries_url"`
	HTTPTimeout                   time.Duration `mapstructure:"http_timeout"`
	SyncTimeout                   time.Duration `mapstructure:"sync_timeout"`
	MaxDownloadBytes              int64         `mapstructure:"max_download_bytes"`
	MaxUncompressedBytes          int64         `mapstructure:"max_uncompressed_bytes"`
	MinCities                     int           `mapstructure:"min_cities"`
	MaxCities                     int           `mapstructure:"max_cities"`
}

// Outbox settings apply only to publish-outbox.
type Outbox struct {
	Brokers        []string      `mapstructure:"brokers"`
	Topic          string        `mapstructure:"topic"`
	PollInterval   time.Duration `mapstructure:"poll_interval"`
	DBTimeout      time.Duration `mapstructure:"db_timeout"`
	PublishTimeout time.Duration `mapstructure:"publish_timeout"`
	LeaseDuration  time.Duration `mapstructure:"lease_duration"`
	RetryMin       time.Duration `mapstructure:"retry_min"`
	RetryMax       time.Duration `mapstructure:"retry_max"`
}

// Progress transport is separate from Cabinet's request topic.
type Progress struct {
	Brokers      []string      `mapstructure:"brokers"`
	Topic        string        `mapstructure:"topic"`
	GroupID      string        `mapstructure:"group_id"`
	PollInterval time.Duration `mapstructure:"poll_interval"`
	Timeout      time.Duration `mapstructure:"timeout"`
	Lease        time.Duration `mapstructure:"lease"`
	MaxAttempts  int           `mapstructure:"max_attempts"`
}
