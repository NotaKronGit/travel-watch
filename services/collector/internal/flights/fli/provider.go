// Package fli adapts an isolated local Fli bridge; it is not an official Google API.
package fli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"time"

	"github.com/NotaKronGit/travel-watch/services/collector/internal/flights"
)

type Config struct {
	Python           string        `mapstructure:"python"`
	Bridge           string        `mapstructure:"bridge"`
	Timeout          time.Duration `mapstructure:"timeout"`
	MaxResponseBytes int           `mapstructure:"max_response_bytes"`
}

func (c Config) Validate() error {
	if c.Python == "" || c.Bridge == "" || c.Timeout < time.Second || c.Timeout > 2*time.Minute || c.MaxResponseBytes < 1024 || c.MaxResponseBytes > 4<<20 {
		return errors.New("invalid Fli configuration")
	}
	return nil
}

type Provider struct{ config Config }

func New(c Config) (*Provider, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &Provider{config: c}, nil
}

var _ flights.Provider = (*Provider)(nil)

type cappedOutput struct {
	bytes.Buffer
	limit int
}

func (b *cappedOutput) Write(p []byte) (int, error) {
	if len(p) > b.limit-b.Len() {
		return 0, flights.ErrInvalidResponse
	}
	return b.Buffer.Write(p)
}

func (p *Provider) FindFlights(ctx context.Context, q flights.Query) (flights.Result, error) {
	if err := q.Validate(); err != nil {
		return flights.Result{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, p.config.Timeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return flights.Result{}, err
	}
	input, _ := json.Marshal(q)
	cmd := exec.CommandContext(ctx, p.config.Python, p.config.Bridge) // #nosec G204 -- executable and bridge are operator configuration, never model or route input.
	cmd.Stdin = bytes.NewReader(input)
	cmd.Stderr = io.Discard
	cmd.WaitDelay = time.Second
	cmd.Env = []string{"FLI_TIMEOUT=15", "PYTHONIOENCODING=utf-8"}
	output := &cappedOutput{limit: p.config.MaxResponseBytes}
	cmd.Stdout = output
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return flights.Result{}, ctx.Err()
		}
		return flights.Result{}, flights.ErrUnavailable
	}
	var reply struct {
		Version int            `json:"version"`
		Error   string         `json:"error"`
		Result  flights.Result `json:"result"`
	}
	dec := json.NewDecoder(bytes.NewReader(output.Bytes()))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&reply); err != nil || reply.Version != 1 {
		return flights.Result{}, flights.ErrInvalidResponse
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		return flights.Result{}, flights.ErrInvalidResponse
	}
	switch reply.Error {
	case "":
	case "rate_limited":
		return flights.Result{}, flights.ErrRateLimited
	case "unsupported":
		return flights.Result{}, flights.ErrUnsupported
	case "invalid_response":
		return flights.Result{}, flights.ErrInvalidResponse
	default:
		return flights.Result{}, flights.ErrUnavailable
	}
	if reply.Result.Provider != "google-flights-fli" {
		return flights.Result{}, flights.ErrInvalidResponse
	}
	if err := reply.Result.Validate(q); err != nil {
		return flights.Result{}, err
	}
	return reply.Result, nil
}
