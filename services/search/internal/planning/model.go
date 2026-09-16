// Package planning runs durable, bounded route-building jobs outside the Kafka consumer.
package planning

import (
	"context"
	"github.com/NotaKronGit/travel-watch/services/search/internal/realroutes"
	"time"
)

type Job struct {
	RequestID, Token string
	Payload          []byte
	Attempt          int
}
type Message struct {
	ID, RequestID, Token string
	Payload              []byte
}
type Repository interface {
	ClaimBuilding(context.Context, time.Duration, int) (Job, bool, error)
	BuildingActive(context.Context, Job) (bool, error)
	FinishBuilding(context.Context, Job, string, realroutes.Result) error
}
