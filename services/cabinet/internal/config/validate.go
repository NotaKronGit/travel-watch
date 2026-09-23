package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"time"
)

func (c Config) Validate(command string) error {
	if command != "serve" && command != "migrate" && command != "sync-cities" && command != "publish-outbox" && command != "consume-progress" {
		return errors.New("unknown Cabinet command")
	}
	if command == "serve" {
		if err := c.Search.Validate(); err != nil {
			return err
		}
	}
	d := c.Database
	if d.Host == "" || d.Name == "" || d.Port < 1 || d.Port > 65535 {
		return errors.New("invalid database host, name or port")
	}
	if d.AppUser == "" || d.OwnerUser == "" || d.AppUser == d.OwnerUser {
		return errors.New("database app and owner users must be distinct and nonempty")
	}
	if (command == "serve" || command == "publish-outbox" || command == "consume-progress") && d.AppPassword == "" {
		return errors.New("database.app_password is required")
	}
	if (command == "migrate" || command == "sync-cities") && d.OwnerPassword == "" {
		return errors.New("database.owner_password is required")
	}
	switch d.SSLMode {
	case "disable", "require", "verify-ca", "verify-full":
	default:
		return errors.New("invalid database.sslmode")
	}
	if d.MaxOpenConns <= 0 || d.MaxIdleConns < 0 || d.MaxIdleConns > d.MaxOpenConns {
		return errors.New("invalid database pool limits")
	}
	if d.ConnectTimeout < time.Second || d.ConnectTimeout%time.Second != 0 {
		return errors.New("database.connect_timeout must be a positive whole number of seconds")
	}
	for key, value := range map[string]time.Duration{"database.ping_timeout": d.PingTimeout, "database.conn_max_lifetime": d.ConnMaxLifetime, "database.migration_timeout": d.MigrationTimeout} {
		if value <= 0 {
			return fmt.Errorf("%s must be positive", key)
		}
	}
	if command == "consume-progress" {
		return c.Progress.Validate()
	}
	if command == "publish-outbox" {
		return c.Outbox.Validate()
	}
	if command == "migrate" {
		return nil
	}
	if command == "sync-cities" {
		for _, raw := range []string{c.Catalog.CitiesURL, c.Catalog.CountriesURL, c.Catalog.RegionsURL, c.Catalog.AlternateNamesURL} {
			u, err := url.Parse(raw)
			if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
				return errors.New("catalog URLs must use HTTPS without credentials or fragments")
			}
		}
		g := c.Catalog
		if g.MaxAlternateDownloadBytes <= 0 || g.MaxAlternateUncompressedBytes <= 0 || g.HTTPTimeout <= 0 || g.SyncTimeout <= 0 || g.MaxDownloadBytes <= 0 || g.MaxUncompressedBytes <= 0 || g.MinCities < 1 || g.MaxCities < g.MinCities {
			return errors.New("invalid catalog timeouts or size limits")
		}
		return nil
	}
	s, a := c.Server, c.Auth
	if _, port, err := net.SplitHostPort(s.Address); err != nil {
		return errors.New("invalid server.address")
	} else if p, err := strconv.Atoi(port); err != nil || p < 1 || p > 65535 {
		return errors.New("invalid server.address port")
	}
	u, err := url.Parse(s.Origin)
	if err != nil || u.Host == "" || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return errors.New("server.origin must be an HTTP(S) origin without a path")
	}
	if !a.CookieSecure && (u.Scheme != "http" || (u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1")) {
		return errors.New("insecure cookies are allowed only on local HTTP")
	}
	if a.CookieSecure && u.Scheme != "https" {
		return errors.New("secure cookies require an HTTPS origin")
	}
	for key, value := range map[string]time.Duration{
		"server.read_header_timeout": s.ReadHeaderTimeout, "server.read_timeout": s.ReadTimeout,
		"server.write_timeout": s.WriteTimeout, "server.idle_timeout": s.IdleTimeout,
		"server.shutdown_timeout": s.ShutdownTimeout, "server.request_timeout": s.RequestTimeout,
		"server.health_timeout": s.HealthTimeout, "auth.session_ttl": a.SessionTTL,
		"auth.cleanup_interval": a.CleanupInterval, "auth.cleanup_timeout": a.CleanupTimeout,
		"auth.login_window":     a.LoginWindow,
		"trips.expiry_interval": c.Trips.ExpiryInterval, "trips.expiry_timeout": c.Trips.ExpiryTimeout,
	} {
		if value <= 0 {
			return fmt.Errorf("%s must be positive", key)
		}
	}
	if a.SessionTTL < time.Second {
		return errors.New("auth.session_ttl must be at least one second")
	}
	if s.MaxHeaderBytes <= 0 || s.MaxBodyBytes <= 0 || a.LoginAttempts <= 0 || a.MaxTrackedAddresses <= 0 || a.HashConcurrency <= 0 {
		return errors.New("server size and auth concurrency limits must be positive")
	}
	if c.Trips.ExpiryBatch < 1 || c.Trips.ExpiryBatch > 1000 {
		return errors.New("trips.expiry_batch must be between 1 and 1000")
	}
	return nil
}
