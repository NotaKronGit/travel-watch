package outbox

import (
	"context"
	"log/slog"
	"time"

	"github.com/NotaKronGit/travel-watch/services/cabinet/internal/config"
	"github.com/NotaKronGit/travel-watch/services/cabinet/internal/storage"
)

type Repository interface {
	ClaimOutbox(context.Context, time.Duration) (storage.OutboxMessage, bool, error)
	AckOutbox(context.Context, storage.OutboxMessage) error
	RetryOutbox(context.Context, storage.OutboxMessage, time.Duration) error
}
type Publisher interface {
	Publish(context.Context, storage.OutboxMessage) error
}
type Relay struct {
	Repository Repository
	Publisher  Publisher
	Config     config.Outbox
}

// Step processes at most one event; errors leave it recoverable by retry or lease expiry.
func (r Relay) Step(ctx context.Context) (bool, error) {
	dbctx, cancel := context.WithTimeout(ctx, r.Config.DBTimeout)
	m, found, err := r.Repository.ClaimOutbox(dbctx, r.Config.LeaseDuration)
	cancel()
	if err != nil || !found {
		return false, err
	}
	pubctx, cancel := context.WithTimeout(ctx, r.Config.PublishTimeout)
	err = r.Publisher.Publish(pubctx, m)
	cancel()
	// On shutdown keep the lease: publication may have succeeded despite cancellation.
	if ctx.Err() != nil {
		return true, ctx.Err()
	}
	dbctx, cancel = context.WithTimeout(ctx, r.Config.DBTimeout)
	defer cancel()
	if err != nil {
		if retryErr := r.Repository.RetryOutbox(dbctx, m, retryDelay(m.Attempts, r.Config.RetryMin, r.Config.RetryMax)); retryErr != nil {
			return true, retryErr
		}
		return true, err
	}
	return true, r.Repository.AckOutbox(dbctx, m)
}
func (r Relay) Run(ctx context.Context) error {
	if err := r.Config.Validate(); err != nil {
		return err
	}
	for ctx.Err() == nil {
		found, err := r.Step(ctx)
		if ctx.Err() != nil {
			break
		}
		if err != nil {
			slog.Warn("outbox attempt failed; event remains pending")
		}
		if err != nil || !found {
			timer := time.NewTimer(r.Config.PollInterval)
			select {
			case <-ctx.Done():
				timer.Stop()
			case <-timer.C:
			}
		}
	}
	return ctx.Err()
}
func retryDelay(attempts int, minDelay, maxDelay time.Duration) time.Duration {
	delay := minDelay
	for i := 1; i < attempts && delay < maxDelay; i++ {
		if delay > maxDelay/2 {
			return maxDelay
		}
		delay *= 2
	}
	return delay
}
