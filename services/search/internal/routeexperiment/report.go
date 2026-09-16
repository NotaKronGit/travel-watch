// Package routeexperiment produces a read-only report from reviewed transport data.
// It is separate from synthetic evaluation and does not schedule user trips.
package routeexperiment

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"
)

type Path struct {
	Steps    []string `json:"steps"`
	Warnings []string `json:"warnings,omitempty"`
}
type Case struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Origin      string `json:"origin"`
	Destination string `json:"destination"`
}
type Outcome struct {
	LimitReached bool   `json:"limit_reached,omitempty"`
	Status       string `json:"status"`
	Message      string `json:"message,omitempty"`
	DurationMS   int64  `json:"duration_ms"`
	Paths        []Path `json:"paths"`
}
type Report struct {
	ID     string  `json:"id"`
	Title  string  `json:"title"`
	Graph  Outcome `json:"graph"`
	Gemini Outcome `json:"gemini"`
}
type Bundle struct {
	GeminiPrompt string    `json:"gemini_prompt"`
	Version      int       `json:"version"`
	GeneratedAt  time.Time `json:"generated_at"`
	Model        string    `json:"model"`
	Reports      []Report  `json:"reports"`
}

// Query deliberately contains no graph, source URLs, expected paths or other provider output.
type Query struct {
	Origin        string `json:"origin"`
	Destination   string `json:"destination"`
	MaxCandidates int    `json:"max_candidates"`
}
type Finder interface {
	Search(context.Context, Query) ([]Path, error)
}

func Load(r io.Reader) ([]Case, error) {
	data, err := io.ReadAll(io.LimitReader(r, (1<<20)+1))
	if err != nil || len(data) > 1<<20 {
		return nil, errors.New("invalid experiment input size")
	}
	var cases []Case
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&cases); err != nil || len(cases) == 0 || len(cases) > 10 {
		return nil, errors.New("invalid experiment cases")
	}
	if !errors.Is(decoder.Decode(new(any)), io.EOF) {
		return nil, errors.New("trailing experiment data")
	}
	seen := map[string]bool{}
	for _, c := range cases {
		if seen[c.ID] || c.ID == "" || c.Title == "" || c.Origin == "" || c.Destination == "" || c.Origin == c.Destination {
			return nil, errors.New("experiment requires distinct origin and destination")
		}
		seen[c.ID] = true
	}
	return cases, nil
}

// Run executes independent searches sequentially with a deadline per query.
func Run(ctx context.Context, cases []Case, alternative Finder, model string, timeout time.Duration) (Bundle, error) {
	if timeout <= 0 || timeout > 2*time.Minute {
		return Bundle{}, errors.New("invalid experiment timeout")
	}
	bundle := Bundle{Version: 3, GeneratedAt: time.Now().UTC(), Model: model, Reports: []Report{}}
	for _, c := range cases {
		if err := ctx.Err(); err != nil {
			return bundle, err
		}
		graph := Outcome{Status: "unavailable", Message: "Независимый источник транспортных связей ещё не подключён.", Paths: []Path{}}
		gemini := Outcome{Status: "not_configured", Message: "Gemini не запущен: ключ не настроен.", Paths: []Path{}}
		if alternative != nil {
			gemini = attempt(ctx, c, alternative, timeout)
		}
		bundle.Reports = append(bundle.Reports, Report{ID: c.ID, Title: c.Title, Graph: graph, Gemini: gemini})
	}
	return bundle, ctx.Err()
}
func attempt(ctx context.Context, c Case, p Finder, timeout time.Duration) Outcome {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	start := time.Now()
	paths, err := p.Search(ctx, Query{Origin: c.Origin, Destination: c.Destination, MaxCandidates: 4})
	out := Outcome{Status: "hypothesis", DurationMS: time.Since(start).Milliseconds(), Paths: []Path{}}
	if err != nil {
		out.Status = "error"
		out.Message = "Independent search failed."
		var safe interface{ SafeMessage() string }
		if errors.As(err, &safe) {
			out.Message = safe.SafeMessage()
		}
		return out
	}
	out.Paths = paths
	return out
}
