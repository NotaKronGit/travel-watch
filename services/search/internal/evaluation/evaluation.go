// Package evaluation compares planners on explicitly synthetic routes and schedules.
package evaluation

import (
	"context"
	"errors"
	"time"

	"github.com/NotaKronGit/travel-watch/services/search/internal/journeys"
	fixtures "github.com/NotaKronGit/travel-watch/services/search/internal/journeys/testsource"
	"github.com/NotaKronGit/travel-watch/services/search/internal/routes"
	routefixtures "github.com/NotaKronGit/travel-watch/services/search/internal/routes/testsource"
)

type Report struct {
	Scenario        string  `json:"scenario"`
	Synthetic       bool    `json:"synthetic"`
	Compared        bool    `json:"compared"`
	Common          int     `json:"common"`
	GraphOnly       int     `json:"graph_only"`
	AlternativeOnly int     `json:"alternative_only"`
	Graph           Outcome `json:"graph"`
	Alternative     Outcome `json:"alternative"`
}
type Outcome struct {
	Error                     string `json:"error,omitempty"`
	Candidates                int    `json:"candidates"`
	CandidateCoverageComplete bool   `json:"candidate_coverage_complete"`
	Journeys                  int    `json:"journeys"`
	ScheduleCoverageComplete  bool   `json:"schedule_coverage_complete"`
	DurationMS                int64  `json:"duration_ms"`
}
type snapshotSource struct{ snapshot routes.Snapshot }

func (s snapshotSource) Load(ctx context.Context, _ routes.Query, _ routes.Limits) (routes.Snapshot, error) {
	return s.snapshot, ctx.Err()
}

// Run is deliberately sequential: at most one external planner call is active.
// A failed case is reported and does not masquerade as an empty successful search.
func Run(ctx context.Context, alternative routes.Planner, l journeys.Limits, timeout time.Duration) ([]Report, error) {
	cases := []fixtures.Scenario{{Name: "ivanovo-pattaya", Topology: routefixtures.Ivanovo(), Schedule: fixtures.Snapshot(), Query: fixtures.Query(), Policy: fixtures.Policy()}}
	cases = append(cases, fixtures.AdditionalRoutes()...)
	reports := make([]Report, 0, len(cases))
	for _, c := range cases {
		if err := ctx.Err(); err != nil {
			return reports, err
		}
		comparison, err := routes.Compare(ctx, snapshotSource{c.Topology}, routes.GraphPlanner{}, alternative, c.Query.Route, l.Routes, timeout)
		if err != nil {
			return reports, err
		}
		report := Report{Scenario: c.Name, Synthetic: true, Compared: comparison.Compared, Common: len(comparison.Common), GraphOnly: len(comparison.OnlyLeft), AlternativeOnly: len(comparison.OnlyRight)}
		outcome := func(a routes.Attempt) Outcome {
			o := Outcome{DurationMS: a.Duration.Milliseconds()}
			if a.Err != nil {
				o.Error = "planner_failed"
				var safe interface{ SafeMessage() string }
				if errors.As(a.Err, &safe) {
					o.Error = safe.SafeMessage()
				}
				return o
			}
			o.Candidates = len(a.Result.Candidates)
			o.CandidateCoverageComplete = a.Result.Complete()
			scheduleCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			r, err := journeys.Assemble(scheduleCtx, c.Topology, a.Result, c.Schedule, c.Query, c.Policy, l)
			if err != nil {
				o.Error = "assembly_failed"
				return o
			}
			o.Journeys = len(r.Journeys)
			o.ScheduleCoverageComplete = r.Complete()
			return o
		}
		report.Graph = outcome(comparison.Left)
		report.Alternative = outcome(comparison.Right)
		reports = append(reports, report)
	}
	return reports, ctx.Err()
}
