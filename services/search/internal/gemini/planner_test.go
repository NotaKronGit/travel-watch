package gemini

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/NotaKronGit/travel-watch/services/search/internal/evaluation"
	"github.com/NotaKronGit/travel-watch/services/search/internal/journeys"
	"github.com/NotaKronGit/travel-watch/services/search/internal/routes"
	fixtures "github.com/NotaKronGit/travel-watch/services/search/internal/routes/testsource"
)

var testLimits = routes.Limits{MaxNodes: 100, MaxLinks: 100, MaxSteps: 1000, MaxCandidates: 20}

func testPlanner(t *testing.T, h http.HandlerFunc) *Planner {
	t.Helper()
	server := httptest.NewServer(h)
	t.Cleanup(server.Close)
	p, err := New(Config{Model: "test-model", APIKey: "test-secret", Timeout: time.Second, MaxOutputTokens: 1024, MaxRequestBytes: 32768, MaxResponseBytes: 65536})
	if err != nil {
		t.Fatal(err)
	}
	p.endpoint = server.URL
	return p
}
func envelope(answer, finish string) string {
	raw, _ := json.Marshal(map[string]any{"candidates": []any{map[string]any{"finishReason": finish, "content": map[string]any{"parts": []any{map[string]string{"text": answer}}}}}, "usageMetadata": map[string]int{"promptTokenCount": 100, "candidatesTokenCount": 20, "totalTokenCount": 120}})
	return string(raw)
}
func TestFiveScenariosThroughHTTP(t *testing.T) {
	calls := 0
	p := testPlanner(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "POST" || r.Header.Get("x-goog-api-key") != "test-secret" {
			t.Error("missing authentication")
		}
		var request struct {
			Contents         []struct{ Parts []struct{ Text string } }
			GenerationConfig struct{ MaxOutputTokens int }
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		if request.GenerationConfig.MaxOutputTokens != 1024 {
			t.Error("token limit missing")
		}
		var input struct {
			Snapshot      routes.Snapshot
			Query         routes.Query
			MaxCandidates int
		}
		if err := json.Unmarshal([]byte(request.Contents[0].Parts[0].Text), &input); err != nil {
			t.Error(err)
			return
		}
		result, err := (routes.GraphPlanner{}).Plan(r.Context(), input.Snapshot, input.Query, testLimits)
		if err != nil {
			t.Error(err)
			return
		}
		paths := [][]int{}
		for _, c := range result.Candidates {
			indices := []int{}
			for _, leg := range c.Legs {
				for i, link := range input.Snapshot.Links {
					if leg == link {
						indices = append(indices, i)
						break
					}
				}
			}
			paths = append(paths, indices)
		}
		answer, _ := json.Marshal(map[string]any{"paths": paths})
		_, _ = fmt.Fprint(w, envelope(string(answer), "STOP"))
	})
	tokens := 0
	p.Observe = func(u Usage) { tokens += u.TotalTokens }
	reports, err := evaluation.Run(context.Background(), p, journeys.Limits{Routes: testLimits, MaxOffers: 100, MaxTransfers: 100, MaxCombinations: 1000, MaxJourneys: 20}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 5 || calls != 5 || tokens != 600 {
		t.Fatal("missing calls or usage")
	}
	for _, r := range reports {
		if !r.Compared || r.GraphOnly != 0 || r.AlternativeOnly != 0 || r.Graph.Journeys != r.Alternative.Journeys || r.Alternative.CandidateCoverageComplete {
			t.Fatalf("unexpected comparison %+v", r)
		}
	}
}
func TestInvalidResponses(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"quota", "test-secret", 429},
		{"server", "test-secret", 503},
		{"malformed", "{", 200},
		{"truncated", envelope(`{"paths":[]}`, "MAX_TOKENS"), 200},
		{"missing", envelope(`{}`, "STOP"), 200},
		{"invented", envelope(`{"paths":[[0,1,999]]}`, "STOP"), 200},
		{"disconnected", envelope(`{"paths":[[0,0,0]]}`, "STOP"), 200},
		{"extra", envelope(`{"paths":[],"other":1}`, "STOP"), 200},
		{"trailing", envelope(`{"paths":[]} {}`, "STOP"), 200},
		{"oversize", strings.Repeat("x", 65537), 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := testPlanner(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(tc.status); _, _ = fmt.Fprint(w, tc.body) })
			_, err := p.Plan(context.Background(), fixtures.Ivanovo(), routes.Query{OriginCityID: fixtures.Origin, DestinationCityID: fixtures.Destination}, testLimits)
			var safe *Failure
			if !errors.As(err, &safe) || safe.SafeMessage() == "" {
				t.Fatal("missing safe diagnostic")
			}
			if err == nil || strings.Contains(err.Error(), "test-secret") {
				t.Fatal("invalid response accepted or secret exposed", err)
			}
		})
	}
}
func TestRequestLimitAndTimeout(t *testing.T) {
	calls := 0
	p := testPlanner(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		select {
		case <-r.Context().Done():
		case <-time.After(100 * time.Millisecond):
		}
	})
	p.cfg.MaxRequestBytes = 1024
	q := routes.Query{OriginCityID: fixtures.Origin, DestinationCityID: fixtures.Destination}
	if _, err := p.Plan(context.Background(), fixtures.Ivanovo(), q, testLimits); err == nil || calls != 0 {
		t.Fatal("request limit ignored")
	}
	p.cfg.MaxRequestBytes = 32768
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := p.Plan(ctx, fixtures.Ivanovo(), q, testLimits); err == nil {
		t.Fatal("timeout ignored")
	}
}
func TestNoRedirect(t *testing.T) {
	leaked := false
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { leaked = true }))
	defer target.Close()
	p := testPlanner(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	})
	_, err := p.Plan(context.Background(), fixtures.Ivanovo(), routes.Query{OriginCityID: fixtures.Origin, DestinationCityID: fixtures.Destination}, testLimits)
	if err == nil || leaked {
		t.Fatal("redirect followed")
	}
}
