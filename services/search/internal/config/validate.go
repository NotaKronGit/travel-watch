package config

import (
	"errors"
	"net"
	"strconv"
	"strings"
	"time"
)

func (c Config) Validate(command string) error {
	d := c.Database
	if command != "consume" && command != "migrate" {
		return errors.New("usage: search [consume|migrate]")
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
