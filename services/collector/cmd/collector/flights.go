package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/NotaKronGit/travel-watch/services/collector/internal/config"
	"github.com/NotaKronGit/travel-watch/services/collector/internal/flights"
	"github.com/NotaKronGit/travel-watch/services/collector/internal/flights/fli"
	"github.com/joho/godotenv"
)

func flightsFind(args []string) error {
	flags := flag.NewFlagSet("flights-find", flag.ContinueOnError)
	var q flights.Query
	flags.StringVar(&q.From, "from", "", "origin airport IATA")
	flags.StringVar(&q.To, "to", "", "destination airport IATA")
	flags.StringVar(&q.Date, "date", "", "local departure date YYYY-MM-DD")
	flags.IntVar(&q.Adults, "adults", 2, "adult passengers")
	flags.IntVar(&q.Limit, "limit", 5, "maximum returned options (1..50)")
	flags.BoolVar(&q.Nonstop, "nonstop", false, "direct flights only")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected flight arguments")
	}
	if err := q.Validate(); err != nil {
		return err
	}
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.New("cannot read .env")
	}
	path := os.Getenv("COLLECTOR_CONFIG")
	if path == "" {
		path = "services/collector/config.yaml"
	}
	c, err := config.LoadFlights(path)
	if err != nil {
		return err
	}
	var provider flights.Provider
	provider, err = fli.New(c.Fli)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	result, err := provider.FindFlights(ctx, q)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}
