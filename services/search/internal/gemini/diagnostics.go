package gemini

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"regexp"
	"strings"
	"unicode"
)

// Only selected error fields are exposed, never the raw body or metadata.
func providerError(status int, raw []byte, key string) error {
	var envelope struct {
		Error struct {
			Code            int
			Status, Message string
			Details         []struct {
				Type   string `json:"@type"`
				Reason string
			}
		}
	}
	prefix := fmt.Sprintf("Gemini HTTP %d", status)
	if json.Unmarshal(raw, &envelope) != nil {
		return errors.New(prefix + ": error body is not valid JSON")
	}
	e := envelope.Error
	if e.Code != 0 {
		prefix += fmt.Sprintf("; code=%d", e.Code)
	}
	if e.Status != "" {
		prefix += "; status=" + sanitize(e.Status, key, 100)
	}
	for _, d := range e.Details {
		if d.Type == "type.googleapis.com/google.rpc.ErrorInfo" && d.Reason != "" {
			prefix += "; reason=" + sanitize(d.Reason, key, 100)
			break
		}
	}
	if e.Message != "" {
		prefix += "; message=" + sanitize(e.Message, key, 1000)
	}
	return errors.New(prefix)
}

var credentialsPattern = regexp.MustCompile(`(?i)(?:bearer\s+|(?:x-goog-api-key|api[_-]?key|access_token|key)\s*[=:]\s*)[^\s&"'<>]+`)

func sanitize(value, key string, limit int) string {
	if key != "" {
		for _, secret := range []string{key, url.QueryEscape(key), url.PathEscape(key)} {
			value = strings.ReplaceAll(value, secret, "[REDACTED]")
		}
	}
	value = credentialsPattern.ReplaceAllString(value, "[REDACTED]")
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return ' '
		}
		return r
	}, value)
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit]) + "…"
	}
	return value
}
func operationError(ctx context.Context, err error, phase string) error {
	kind := "transport_error"
	var dns *net.DNSError
	var op *net.OpError
	var timeout net.Error
	switch {
	case errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled):
		kind = "cancelled"
	case errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded):
		kind = "timeout"
	case errors.As(err, &dns):
		kind = "dns_error"
	case errors.As(err, &timeout) && timeout.Timeout():
		kind = "timeout"
	case errors.As(err, &op) && op.Op == "dial":
		kind = "connect_error"
	case errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF):
		kind = "connection_closed"
	}
	if kind == "timeout" {
		return fmt.Errorf("Gemini %s; phase=%s: %w", kind, phase, context.DeadlineExceeded)
	}
	if kind == "cancelled" {
		return fmt.Errorf("Gemini %s; phase=%s: %w", kind, phase, context.Canceled)
	}
	return fmt.Errorf("Gemini %s; phase=%s", kind, phase)
}
