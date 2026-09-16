package evaluation

import (
	"context"
	"errors"
	"github.com/NotaKronGit/travel-watch/services/search/internal/journeys"
	"github.com/NotaKronGit/travel-watch/services/search/internal/routes"
	"testing"
	"time"
)

var limits = journeys.Limits{Routes: routes.Limits{MaxNodes: 100, MaxLinks: 100, MaxSteps: 1000, MaxCandidates: 20}, MaxOffers: 100, MaxTransfers: 100, MaxCombinations: 1000, MaxJourneys: 20}

type failingPlanner struct{}

func (failingPlanner) Plan(context.Context, routes.Snapshot, routes.Query, routes.Limits) (routes.Result, error) {
	return routes.Result{}, errors.New("test failure")
}
func TestFiveRoutes(t *testing.T) {
	reports, err := Run(context.Background(), routes.GraphPlanner{}, limits, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 5 {
		t.Fatal("missing fixtures")
	}
	for i, r := range reports {
		expected := 1
		if i == 0 {
			expected = 5
		}
		if !r.Compared || r.GraphOnly != 0 || r.AlternativeOnly != 0 || r.Graph.Error != "" || r.Alternative.Error != "" || r.Alternative.Journeys != expected {
			t.Fatalf("unexpected report %+v", r)
		}
	}
	reports, err = Run(context.Background(), failingPlanner{}, limits, time.Second)
	if err != nil || len(reports) != 5 {
		t.Fatal(err)
	}
	for _, r := range reports {
		if r.Compared || r.Alternative.Error == "" || r.Graph.Error != "" {
			t.Fatal("failure lost", r)
		}
	}
}
