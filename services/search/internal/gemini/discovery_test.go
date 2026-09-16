package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/NotaKronGit/travel-watch/services/search/internal/routeexperiment"
	"net/http"
	"reflect"
	"testing"
)

func TestDiscoveryHasNoGraphOrCompetitorInput(t *testing.T) {
	p := testPlanner(t, func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			SystemInstruction struct{ Parts []struct{ Text string } }
			Contents          []struct{ Parts []struct{ Text string } }
		}
		if json.NewDecoder(r.Body).Decode(&req) != nil {
			t.Fatal("bad request")
		}
		if req.SystemInstruction.Parts[0].Text != DiscoveryInstructions {
			t.Error("wrong prompt")
		}
		var input map[string]any
		if json.Unmarshal([]byte(req.Contents[0].Parts[0].Text), &input) != nil {
			t.Fatal("bad input")
		}
		if !reflect.DeepEqual(input, map[string]any{"origin": "A", "destination": "B", "max_candidates": float64(4)}) {
			t.Errorf("unexpected input: %v", input)
		}
		_, _ = fmt.Fprint(w, envelope(`{"paths":[{"steps":["A -> NEW_AIRPORT -> B"],"warnings":["unverified"]}]}`, "STOP"))
	})
	paths, err := p.Search(context.Background(), routeexperiment.Query{Origin: "A", Destination: "B", MaxCandidates: 4})
	if err != nil || len(paths) != 1 {
		t.Fatalf("new proposal incorrectly rejected: %v", err)
	}
}
func TestDiscoveryRejectsMalformedAndIncompleteResponses(t *testing.T) {
	for _, answer := range []string{`{"paths":null}`, `{"paths":[{"steps":[]}]}`, `{"paths":[{"steps":[""]}]}`, `{"paths":[]} {}`} {
		p := testPlanner(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = fmt.Fprint(w, envelope(answer, "STOP")) })
		if _, err := p.Search(context.Background(), routeexperiment.Query{Origin: "A", Destination: "B", MaxCandidates: 4}); err == nil {
			t.Fatal("invalid response accepted")
		}
	}
}
