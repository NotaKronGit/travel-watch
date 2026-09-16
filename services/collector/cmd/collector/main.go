package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"os/signal"
	"syscall"

	"github.com/NotaKronGit/travel-watch/services/collector/internal/config"
	"github.com/NotaKronGit/travel-watch/services/collector/internal/rail"
	"github.com/NotaKronGit/travel-watch/services/collector/internal/rail/yandex"
	"github.com/joho/godotenv"
)

func run() error {
	if len(os.Args) < 2 || os.Args[1] != "stations-find" {
		return errors.New("usage: collector stations-find -lat N -lon N [-radius 20] [-limit 20]")
	}
	flags := flag.NewFlagSet("stations-find", flag.ContinueOnError)
	lat := flags.Float64("lat", math.NaN(), "WGS84 latitude")
	lon := flags.Float64("lon", math.NaN(), "WGS84 longitude")
	radius := flags.Float64("radius", 20, "radius in km (0,50]")
	limit := flags.Int("limit", 20, "maximum station count")
	if err := flags.Parse(os.Args[2:]); err != nil {
		return errors.New("invalid station command arguments")
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected station command arguments")
	}
	q := rail.StationQuery{Center: &rail.Coordinates{Latitude: *lat, Longitude: *lon}, RadiusKM: *radius, Limit: *limit}
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
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}
	p, err := yandex.New(cfg.ProviderConfig())
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	result, err := p.FindStations(ctx, q)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
