package routeexperiment

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func publishedCases(t *testing.T) []Case {
	t.Helper()
	f, err := os.Open("../../../../docs/research/planner-comparison/cases.json")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	c, err := Load(f)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

type recordingFinder struct {
	queries []Query
	err     error
}

func (f *recordingFinder) Search(_ context.Context, q Query) ([]Path, error) {
	f.queries = append(f.queries, q)
	return []Path{{Steps: []string{"independent alternative"}}}, f.err
}
func TestIndependentInputs(t *testing.T) {
	c := publishedCases(t)
	finder := &recordingFinder{}
	first, err := Run(context.Background(), c, finder, "test", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	inputs := append([]Query(nil), finder.queries...)
	finder.queries = nil
	for i := range c {
		c[i].Title = "display-only title"
		c[i].ID = "display-only-id"
	}
	second, err := Run(context.Background(), c, finder, "test", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(inputs, finder.queries) {
		t.Fatal("display metadata contaminated independent query")
	}
	for i, r := range first.Reports {
		if r.Graph.Status != "unavailable" || len(r.Graph.Paths) != 0 {
			t.Fatal("missing source disguised as graph result")
		}
		if !reflect.DeepEqual(r.Gemini, second.Reports[i].Gemini) {
			t.Fatal("display metadata affected result")
		}
	}
}
func TestErrorsAndMissingKey(t *testing.T) {
	c := publishedCases(t)[:1]
	r, err := Run(context.Background(), c, &recordingFinder{err: errors.New("private token")}, "test", time.Second)
	if err != nil || r.Reports[0].Gemini.Status != "error" || strings.Contains(r.Reports[0].Gemini.Message, "private") {
		t.Fatal("error leak")
	}
	r, err = Run(context.Background(), c, nil, "", time.Second)
	if err != nil || r.Reports[0].Gemini.Status != "not_configured" {
		t.Fatal("missing key hidden")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = Run(ctx, c, nil, "", time.Second); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
