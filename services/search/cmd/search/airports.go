package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/NotaKronGit/travel-watch/services/search/internal/airports"
	"github.com/NotaKronGit/travel-watch/services/search/internal/config"
	"github.com/NotaKronGit/travel-watch/services/search/internal/storage"
	"os"
	"regexp"
	"strings"
)

func runAirports(ctx context.Context, db *sql.DB, c config.Config, command string, args []string) error {
	repo := storage.New(db)
	if command == "airports-find" {
		if len(args) != 1 || !regexp.MustCompile(`^[A-Z]{3}$`).MatchString(strings.ToUpper(args[0])) {
			return errors.New("usage: search airports-find IATA")
		}
		ctx, cancel := context.WithTimeout(ctx, c.Database.PingTimeout)
		defer cancel()
		result, err := repo.FindAirports(ctx, strings.ToUpper(args[0]))
		if err != nil {
			return errors.New("airport lookup failed")
		}
		return json.NewEncoder(os.Stdout).Encode(result)
	}
	if len(args) != 0 {
		return errors.New("usage: search airports-sync")
	}
	ctx, cancel := context.WithTimeout(ctx, c.Airports.ImportTimeout)
	defer cancel()
	source := airports.OurAirports{URL: c.Airports.URL, Timeout: c.Airports.DownloadTimeout, MaxBytes: c.Airports.MaxBytes, MaxRows: c.Airports.MaxRows}
	n, err := (airports.Importer{Source: source, Repository: repo, MinRows: c.Airports.MinRows, MaxRows: c.Airports.MaxRows, MinRetainedPercent: c.Airports.MinRetainedPercent}).Run(ctx)
	if err != nil {
		return errors.New("airport import failed; previous snapshot retained (check source, thresholds, migrations and database)")
	}
	_, err = fmt.Fprintf(os.Stdout, "Imported %d airport records from OurAirports\n", n)
	return err
}
