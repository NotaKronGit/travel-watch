package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"

	"github.com/NotaKronGit/travel-watch/services/search/internal/routes"
)

func TestExplorationKeepsLongKnownPath(t *testing.T) {
	f, err := os.Open("testdata/exploration.json")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var c struct {
		Snapshot routes.Snapshot
		Query    routes.Query
	}
	err = json.NewDecoder(f).Decode(&c)
	if err != nil {
		t.Fatal(err)
	}
	answer := `{"paths":[[0,1,2,3,4,5]]}`
	p := testPlanner(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = fmt.Fprint(w, envelope(answer, "STOP")) })
	if _, err = p.Plan(context.Background(), c.Snapshot, c.Query, testLimits); err == nil {
		t.Fatal("strict planner accepted a long path")
	}
	r, err := (ExplorationPlanner{Planner: p}).Plan(context.Background(), c.Snapshot, c.Query, testLimits)
	if err != nil || len(r.Candidates) != 1 {
		t.Fatalf("lost reviewable path: %v", err)
	}
	if len(routes.RecommendationWarnings(c.Snapshot, c.Query, r.Candidates[0])) == 0 {
		t.Fatal("no complexity warning")
	}
	for _, bad := range []string{`{"paths":[[0,1,2,3,4,99]]}`, `{"paths":[[0,2,1,3,4,5]]}`, `{"paths":[[0,1,2,3,4,3,4,5]]}`} {
		answer = bad
		if _, err = (ExplorationPlanner{Planner: p}).Plan(context.Background(), c.Snapshot, c.Query, testLimits); err == nil {
			t.Fatal("invalid path accepted")
		}
	}
}
