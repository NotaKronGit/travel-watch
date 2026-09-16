//go:build integration

package storage

import (
	"context"
	"encoding/json"
	v1 "github.com/NotaKronGit/travel-watch/gen/travelwatch/search/v1"
	"github.com/NotaKronGit/travel-watch/services/search/internal/consumer"
	"github.com/NotaKronGit/travel-watch/services/search/internal/planning"
	"github.com/NotaKronGit/travel-watch/services/search/internal/realroutes"
	"github.com/google/uuid"
	"testing"
	"time"
)

func TestSavedResultPages(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, db := testDB(t, ctx)
	s := NewBuilder(db, []string{"graph", "gemini"})
	id := uuid.NewString()
	if err := s.Apply(ctx, consumer.Event{ID: uuid.NewString(), RequestID: id, Type: consumer.Created, OccurredAt: time.Now(), Payload: []byte("snapshot")}); err != nil {
		t.Fatal(err)
	}
	j, ok, err := s.ClaimBuilding(ctx, time.Minute, 3)
	if err != nil || !ok {
		t.Fatal(err)
	}
	candidates := []realroutes.Candidate{}
	for i := 0; i < 7; i++ {
		candidates = append(candidates, realroutes.Candidate{Steps: []realroutes.Step{{From: "A", To: "B", Mode: "transfer", Evidence: "assumed"}}, Warnings: []string{"synthetic warning"}})
	}
	raw, _ := json.Marshal(realroutes.Result{Candidates: candidates})
	if err = s.FinishSource(ctx, j, "graph", planning.SourceResult{Data: raw, Count: 7, Incomplete: true}); err != nil {
		t.Fatal(err)
	}
	first, err := s.ReadRoutes(ctx, &v1.GetRoutesRequest{RequestId: id, PageSize: 5})
	if err != nil || len(first.Sources) != 2 || first.Stage != "building" {
		t.Fatal("partial result", err)
	}
	var graph *v1.SourceRoutes
	for _, source := range first.Sources {
		if source.PlannerId == "graph" {
			graph = source
		}
	}
	if graph == nil || len(graph.Routes) != 5 || !graph.HasMore || graph.Total != 7 || graph.Routes[0].Steps[0].Evidence == "" {
		t.Fatal("first page", graph)
	}
	last, err := s.ReadRoutes(ctx, &v1.GetRoutesRequest{RequestId: id, PlannerId: "graph", Offset: 5, PageSize: 5})
	if err != nil || len(last.Sources[0].Routes) != 2 || last.Sources[0].HasMore {
		t.Fatal("last page", err)
	}
	if err = s.FinishSource(ctx, j, "gemini", planning.SourceResult{Data: json.RawMessage(`{"paths":[{"steps":["independent A → C → B"],"warnings":["unverified"]}]}`), Count: 1, Incomplete: true}); err != nil {
		t.Fatal(err)
	}
	gemini, err := s.ReadRoutes(ctx, &v1.GetRoutesRequest{RequestId: id, PlannerId: "gemini", PageSize: 5})
	if err != nil || len(gemini.Sources) != 1 || gemini.Sources[0].Routes[0].Steps[0].Description != "independent A → C → B" {
		t.Fatal("Gemini independent result", err)
	}
	missing, err := s.ReadRoutes(ctx, &v1.GetRoutesRequest{RequestId: uuid.NewString(), PageSize: 5})
	if err != nil || len(missing.Sources) != 0 {
		t.Fatal("not yet received", err)
	}
}
