package config

import (
	"errors"
	"net"
	"strconv"
	"time"
)

func (c Progress) Validate() error {
	if len(c.Brokers) == 0 || c.Topic == "" || c.GroupID == "" || c.PollInterval < time.Second || c.PollInterval > time.Minute || c.Timeout < time.Second || c.Timeout > time.Minute || c.Lease < time.Minute || c.Lease > 30*time.Minute || c.MaxAttempts < 1 || c.MaxAttempts > 10 {
		return errors.New("invalid progress configuration")
	}
	for _, b := range c.Brokers {
		h, p, e := net.SplitHostPort(b)
		n, pe := strconv.Atoi(p)
		if e != nil || pe != nil || h == "" || n < 1 || n > 65535 {
			return errors.New("invalid progress broker")
		}
	}
	return nil
}
