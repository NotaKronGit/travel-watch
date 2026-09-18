package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/signal"
	"syscall"

	"github.com/NotaKronGit/travel-watch/api/transport"
	"github.com/NotaKronGit/travel-watch/services/collector/internal/config"
	"github.com/NotaKronGit/travel-watch/services/collector/internal/flights"
	"github.com/NotaKronGit/travel-watch/services/collector/internal/flights/fli"
	"github.com/NotaKronGit/travel-watch/services/collector/internal/rail/yandex"
	"github.com/joho/godotenv"
)

func transportStdio() error {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.New("cannot read .env")
	}
	path := os.Getenv("COLLECTOR_CONFIG")
	if path == "" {
		path = "services/collector/config.yaml"
	}
	c, err := config.Load(path)
	if err != nil {
		return err
	}
	p, err := yandex.New(c.ProviderConfig())
	if err != nil {
		return err
	}
	// The optional flight provider is constructed only when requested.
	var flightProvider flights.Provider
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	input := bufio.NewScanner(os.Stdin)
	input.Buffer(make([]byte, 4096), 16384)
	output := json.NewEncoder(os.Stdout)
	for input.Scan() {
		var q transport.Request
		if json.Unmarshal(input.Bytes(), &q) != nil {
			return errors.New("invalid transport frame")
		}
		var result transport.Response
		var e error
		if q.Method == "flight-paths" {
			if flightProvider == nil {
				flightProvider, e = fli.New(c.Fli)
			}
			if e == nil {
				result, e = flightPaths(ctx, flightProvider, q)
			}
		} else {
			result, e = p.Transport(ctx, q)
		}
		if e != nil {
			result = transport.Response{Version: 1, Error: e.Error()}
		}
		if err = output.Encode(result); err != nil {
			return errors.New("cannot write transport frame")
		}
	}
	return input.Err()
}

// Only topology crosses this protocol; concrete times and prices are not consumed here.
func flightPaths(ctx context.Context, p flights.Provider, q transport.Request) (transport.Response, error) {
	if q.Version != 1 {
		return transport.Response{}, errors.New("invalid flight protocol version")
	}
	r, err := p.FindFlights(ctx, flights.Query{From: q.From, To: q.To, Date: q.Date, Adults: q.Adults, Limit: q.Limit})
	if err != nil {
		return transport.Response{}, err
	}
	out := transport.Response{Version: 1, Incomplete: !r.Complete, Count: len(r.Options), Total: len(r.Options)}
	for _, o := range r.Options {
		path := transport.FlightPath{}
		for _, l := range o.Legs {
			path.Legs = append(path.Legs, transport.Connection{From: transport.Station{Code: "iata:" + l.From, Title: l.From, IATA: l.From}, To: transport.Station{Code: "iata:" + l.To, Title: l.To, IATA: l.To}, Mode: "plane"})
		}
		out.FlightPaths = append(out.FlightPaths, path)
	}
	return out, nil
}
