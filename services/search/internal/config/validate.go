package config

import (
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func (c Config) Validate(command string) error {
	if command == "compare-real-routes" {
		if c.Gemini.APIKey == "" {
			return nil
		}
		return c.Gemini.Validate()
	}
	if command == "compare-planners" {
		return c.Gemini.Validate()
	}
	d := c.Database
	if command != "consume" && command != "migrate" && command != "airports-sync" && command != "airports-find" {
		return errors.New("usage: search [consume|migrate|compare-planners|airports-sync|airports-find]")
	}
	if d.Host == "" || d.Name == "" || d.Port < 1 || d.Port > 65535 || d.MaxOpenConns < 1 || d.MaxIdleConns < 0 || d.MaxIdleConns > d.MaxOpenConns {
		return errors.New("invalid Search database settings")
	}
	if !strings.Contains("|disable|require|verify-ca|verify-full|", "|"+d.SSLMode+"|") {
		return errors.New("invalid Search database sslmode")
	}
	if d.ConnectTimeout%time.Second != 0 {
		return errors.New("database connect_timeout must use whole seconds")
	}
	for _, v := range []time.Duration{d.ConnectTimeout, d.PingTimeout, d.ConnMaxLifetime, d.MigrationTimeout, c.Consumer.DBTimeout, c.Consumer.CommitTimeout, c.Consumer.DialTimeout} {
		if v < time.Millisecond || v > 24*time.Hour {
			return errors.New("invalid Search timeout")
		}
	}
	if command == "migrate" {
		if d.OwnerUser == "" || d.OwnerPassword == "" {
			return errors.New("Search database owner credentials required")
		}
		return nil
	}
	if d.AppUser == "" || d.AppPassword == "" {
		return errors.New("Search database app credentials required")
	}
	if command == "airports-find" {
		return nil
	}
	if command == "airports-sync" {
		a := c.Airports
		u, err := url.Parse(a.URL)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return errors.New("invalid airports HTTPS URL")
		}
		if a.DownloadTimeout < time.Second || a.DownloadTimeout > 10*time.Minute || a.ImportTimeout < time.Second || a.ImportTimeout > 30*time.Minute || a.MaxBytes < 1024 || a.MaxBytes > 100*1024*1024 || a.MinRows < 1 || a.MaxRows < a.MinRows || a.MaxRows > 200000 || a.MinRetainedPercent < 1 || a.MinRetainedPercent > 100 {
			return errors.New("invalid airports import limits")
		}
		return nil
	}
	if c.Consumer.Topic == "" || c.Consumer.GroupID == "" || len(c.Consumer.Brokers) == 0 {
		return errors.New("Search Kafka topic, group_id and brokers required")
	}
	for _, broker := range c.Consumer.Brokers {
		host, port, err := net.SplitHostPort(broker)
		n, parseErr := strconv.Atoi(port)
		if err != nil || host == "" || parseErr != nil || n < 1 || n > 65535 {
			return errors.New("invalid Search Kafka broker")
		}
	}
	return nil
}
