// Package routes builds transport candidates, not bookable itineraries.
package routes

import "context"

type City struct{ ID, Name, Timezone string }
type NodeKind string

const (
	CityPoint NodeKind = "city"
	Airport   NodeKind = "airport"
	Station   NodeKind = "station"
)

type Node struct {
	ID, CityID, Name, Timezone string
	Kind                       NodeKind
}
type Mode string

const (
	Transfer Mode = "transfer"
	Train    Mode = "train"
	Flight   Mode = "flight"
)

// Links are directed. A transfer is an explicit connection, never inferred by distance.
type Link struct {
	From, To string
	Mode     Mode
}
type Snapshot struct {
	Source    string
	Synthetic bool
	// Complete means complete topology coverage for this query, not ticket availability.
	Complete bool
	Cities   []City
	Nodes    []Node
	Links    []Link
}
type Query struct{ OriginCityID, DestinationCityID string }
type Limits struct{ MaxNodes, MaxLinks, MaxSteps, MaxCandidates int }

// Source must bound fetching by limits and honor context cancellation. Truncated
// upstream data must set Complete=false; source failures must return an error.
type Source interface {
	Load(context.Context, Query, Limits) (Snapshot, error)
}
type Candidate struct{ Legs []Link }
type Result struct {
	Source           string
	Synthetic        bool
	Candidates       []Candidate
	SourceIncomplete bool
	MissingMapping   bool
	LimitReached     bool
}

func (r Result) Complete() bool { return !r.SourceIncomplete && !r.MissingMapping && !r.LimitReached }
