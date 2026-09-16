package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/NotaKronGit/travel-watch/services/search/internal/config"
	"github.com/NotaKronGit/travel-watch/services/search/internal/evaluation"
	"github.com/NotaKronGit/travel-watch/services/search/internal/gemini"
	"github.com/NotaKronGit/travel-watch/services/search/internal/journeys"
	"github.com/NotaKronGit/travel-watch/services/search/internal/routes"
	"os"
)

func comparePlanners(ctx context.Context, c config.Config) error {
	p, err := gemini.New(c.Gemini)
	if err != nil {
		return err
	}
	usage := gemini.Usage{}
	p.Observe = func(u gemini.Usage) {
		usage.PromptTokens += u.PromptTokens
		usage.OutputTokens += u.OutputTokens
		usage.TotalTokens += u.TotalTokens
	}
	limits := journeys.Limits{Routes: routes.Limits{MaxNodes: 100, MaxLinks: 100, MaxSteps: 1000, MaxCandidates: 20}, MaxOffers: 100, MaxTransfers: 100, MaxCombinations: 1000, MaxJourneys: 20}
	reports, runErr := evaluation.Run(ctx, p, limits, c.Gemini.Timeout)
	output := struct {
		Model   string              `json:"model"`
		Usage   gemini.Usage        `json:"reported_usage"`
		Reports []evaluation.Report `json:"reports"`
	}{c.Gemini.Model, usage, reports}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err = encoder.Encode(output); err != nil {
		return errors.New("cannot write comparison report")
	}
	if runErr != nil {
		return runErr
	}
	for _, r := range reports {
		if r.Graph.Error != "" || r.Alternative.Error != "" {
			return errors.New("planner comparison contains failed cases")
		}
	}
	return nil
}
