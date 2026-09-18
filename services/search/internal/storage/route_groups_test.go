package storage

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/NotaKronGit/travel-watch/services/search/internal/realroutes"
)

func railCandidate(station, airport string) realroutes.Candidate {
	return realroutes.Candidate{Steps: []realroutes.Step{
		{From: "Город отправления", To: "Вокзал отправления", Mode: "transfer", Evidence: "assumed"},
		{From: "Вокзал отправления", To: station, FromCode: "origin-rail", ToCode: station, Mode: "train", Number: station, Evidence: "yandex-rasp"},
		{From: station, To: airport, Mode: "transfer", Evidence: "assumed"},
		{From: airport, To: "Аэропорт назначения", FromCode: airport, ToCode: "arrival", Mode: "plane", Number: "TEST123", Evidence: "yandex-rasp"},
		{From: "Аэропорт назначения", To: "Город назначения", Mode: "transfer", Evidence: "assumed"},
	}, Warnings: []string{"common"}}
}

func TestGroupRailAccess(t *testing.T) {
	a, b := railCandidate("station-a", "airport-a"), railCandidate("station-b", "airport-a")
	b.Warnings = append(b.Warnings, "partial")
	original := []realroutes.Candidate{a, b, railCandidate("station-c", "airport-b")}
	before, _ := json.Marshal(original)
	got := groupRailAccess(original)
	if len(got) != 2 || len(got[0].Steps) != 3 || got[0].Steps[1].Number != "" {
		t.Fatalf("unexpected grouped routes: %+v", got)
	}
	if len(got[0].Warnings) != 3 || got[0].Warnings[1] != "partial" {
		t.Fatalf("lost warnings: %v", got[0].Warnings)
	}
	if !reflect.DeepEqual(got[1], original[2]) {
		t.Fatal("distinct airport changed")
	}
	after, _ := json.Marshal(original)
	if string(before) != string(after) {
		t.Fatal("saved candidates mutated")
	}
}

func TestRailAccessKeepsDistinctOrAmbiguousRoutes(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*realroutes.Candidate)
	}{
		{"different origin station", func(c *realroutes.Candidate) { c.Steps[1].FromCode = "other" }},
		{"different arrival airport", func(c *realroutes.Candidate) { c.Steps[3].ToCode = "other" }},
		{"missing airport code", func(c *realroutes.Candidate) { c.Steps[3].FromCode = "" }},
		{"broken continuity", func(c *realroutes.Candidate) { c.Steps[2].To = "other" }},
		{"different evidence", func(c *realroutes.Candidate) { c.Steps[3].Evidence = "assumed" }},
		{"local flight", func(c *realroutes.Candidate) { c.Steps[1].Mode = "plane" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, b := railCandidate("a", "hub"), railCandidate("b", "hub")
			tc.change(&b)
			if got := groupRailAccess([]realroutes.Candidate{a, b}); len(got) != 2 {
				t.Fatal("distinct routes merged")
			}
		})
	}
}
