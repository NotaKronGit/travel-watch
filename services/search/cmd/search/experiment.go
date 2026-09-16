package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"time"

	"github.com/NotaKronGit/travel-watch/services/search/internal/config"
	"github.com/NotaKronGit/travel-watch/services/search/internal/gemini"
	"github.com/NotaKronGit/travel-watch/services/search/internal/routeexperiment"
)

func compareRealRoutes(ctx context.Context, c config.Config, args []string) error {
	if len(args) != 1 {
		return errors.New("usage: search compare-real-routes <reviewed-cases.json>")
	}
	input, err := os.Open(args[0])
	if err != nil {
		return errors.New("cannot open experiment cases")
	}
	defer input.Close()
	cases, err := routeexperiment.Load(input)
	if err != nil {
		return err
	}
	var planner routeexperiment.Finder
	timeout := 30 * time.Second
	if c.Gemini.APIKey != "" {
		p, e := gemini.New(c.Gemini)
		if e != nil {
			return e
		}
		planner = p
		timeout = c.Gemini.Timeout
	}
	report, err := routeexperiment.Run(ctx, cases, planner, c.Gemini.Model, timeout)
	if err != nil {
		return err
	}
	report.GeminiPrompt = gemini.DiscoveryInstructions
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err = enc.Encode(report); err != nil {
		return errors.New("cannot write experiment report")
	}
	return nil
}
