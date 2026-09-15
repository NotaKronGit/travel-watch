package config

import (
	"errors"
	"net"
	"strconv"
	"strings"
	"time"
)

func (o Outbox) Validate() error {
	if len(o.Brokers) == 0 {
		return errors.New("outbox.brokers is required")
	}
	for _, broker := range o.Brokers {
		host, port, err := net.SplitHostPort(broker)
		n, parseErr := strconv.Atoi(port)
		if err != nil || host == "" || parseErr != nil || n < 1 || n > 65535 {
			return errors.New("invalid outbox broker address")
		}
	}
	if len(o.Topic) == 0 || len(o.Topic) > 249 || o.Topic == "." || o.Topic == ".." || strings.Trim(o.Topic, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789._-") != "" {
		return errors.New("invalid outbox.topic")
	}
	for _, d := range []time.Duration{o.PollInterval, o.DBTimeout, o.PublishTimeout, o.LeaseDuration, o.RetryMin, o.RetryMax} {
		if d < time.Millisecond || d > 24*time.Hour {
			return errors.New("outbox durations must be between 1ms and 24h")
		}
	}
	if o.RetryMax < o.RetryMin || o.LeaseDuration <= o.PublishTimeout+2*o.DBTimeout {
		return errors.New("outbox retry range or lease duration is invalid")
	}
	return nil
}
