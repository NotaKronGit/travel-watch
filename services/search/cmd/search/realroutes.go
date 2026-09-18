package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"math"
	"os"

	"github.com/NotaKronGit/travel-watch/services/search/internal/config"
	"github.com/NotaKronGit/travel-watch/services/search/internal/realroutes"
	"github.com/NotaKronGit/travel-watch/services/search/internal/storage"
)

func planRealRoute(ctx context.Context, db *sql.DB, c config.Config, args []string) error {
	flags := flag.NewFlagSet("plan-route", flag.ContinueOnError)
	q := realroutes.Query{}
	flags.StringVar(&q.DepartureFrom, "departure-from", "", "first date for flight discovery")
	flags.StringVar(&q.DepartureTo, "departure-to", "", "last date for flight discovery")
	flags.IntVar(&q.Adults, "adults", 2, "adult passengers")
	flags.StringVar(&q.OriginName, "from", "", "origin display name")
	flags.StringVar(&q.DestinationName, "to", "", "destination display name")
	flags.Float64Var(&q.Origin.Latitude, "from-lat", math.NaN(), "origin latitude")
	flags.Float64Var(&q.Origin.Longitude, "from-lon", math.NaN(), "origin longitude")
	flags.Float64Var(&q.Destination.Latitude, "to-lat", math.NaN(), "destination latitude")
	flags.Float64Var(&q.Destination.Longitude, "to-lon", math.NaN(), "destination longitude")
	if flags.Parse(args) != nil || flags.NArg() != 0 {
		return errors.New("invalid plan-route arguments")
	}
	ctx, cancel := context.WithTimeout(ctx, c.Planner.Timeout)
	defer cancel()
	read, done := context.WithTimeout(ctx, c.Database.PingTimeout)
	catalog, err := storage.New(db).PlannerAirports(read)
	done()
	if err != nil || len(catalog) == 0 {
		return errors.New("Search airport catalog unavailable; run airports-sync first")
	}
	provider, err := realroutes.Start(ctx, c.Planner.CollectorBinary)
	if err != nil {
		return err
	}
	defer provider.Close()
	result, err := (realroutes.Planner{Provider: provider, Airports: catalog, Config: c.Planner}).Plan(ctx, q)
	if err != nil {
		return errors.New("route planning interrupted or invalid input")
	}
	out := json.NewEncoder(os.Stdout)
	out.SetIndent("", "  ")
	return out.Encode(result)
}
