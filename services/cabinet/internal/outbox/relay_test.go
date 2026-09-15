package outbox

import (
	"context"
	"errors"
	"github.com/NotaKronGit/travel-watch/services/cabinet/internal/config"
	"github.com/NotaKronGit/travel-watch/services/cabinet/internal/storage"
	"testing"
	"time"
)

type fakeRepository struct {
	message                    storage.OutboxMessage
	found                      bool
	claimErr, ackErr, retryErr error
	ack, retry                 int
	delay                      time.Duration
}

func (f *fakeRepository) ClaimOutbox(context.Context, time.Duration) (storage.OutboxMessage, bool, error) {
	return f.message, f.found, f.claimErr
}
func (f *fakeRepository) AckOutbox(context.Context, storage.OutboxMessage) error {
	f.ack++
	return f.ackErr
}
func (f *fakeRepository) RetryOutbox(_ context.Context, _ storage.OutboxMessage, d time.Duration) error {
	f.retry++
	f.delay = d
	return f.retryErr
}

type publishFunc func(context.Context, storage.OutboxMessage) error

func (f publishFunc) Publish(c context.Context, m storage.OutboxMessage) error { return f(c, m) }
func TestDeliveryOutcomes(t *testing.T) {
	failure := errors.New("test failure")
	for _, tc := range []struct {
		name                                   string
		found                                  bool
		claimErr, publishErr, ackErr, retryErr error
		ack, retry, calls                      int
		wantErr                                bool
	}{
		{name: "empty"},
		{name: "claim failed", claimErr: failure, wantErr: true},
		{name: "confirmed", found: true, ack: 1, calls: 1},
		{name: "publication failed", found: true, publishErr: failure, retry: 1, calls: 1, wantErr: true},
		{name: "ack lost", found: true, ackErr: failure, ack: 1, calls: 1, wantErr: true},
		{name: "retry update failed", found: true, publishErr: failure, retryErr: failure, retry: 1, calls: 1, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepository{found: tc.found, claimErr: tc.claimErr, ackErr: tc.ackErr, retryErr: tc.retryErr, message: storage.OutboxMessage{ID: "stable-event", RequestID: "trip", Payload: []byte("snapshot"), Attempts: 3}}
			calls := 0
			relay := Relay{Repository: repo, Config: config.Outbox{DBTimeout: time.Second, PublishTimeout: time.Second, LeaseDuration: time.Minute, RetryMin: time.Second, RetryMax: time.Minute}, Publisher: publishFunc(func(ctx context.Context, m storage.OutboxMessage) error {
				calls++
				if _, ok := ctx.Deadline(); !ok {
					t.Fatal("missing publish timeout")
				}
				if m.ID != "stable-event" || string(m.Payload) != "snapshot" {
					t.Fatal("event changed")
				}
				return tc.publishErr
			})}
			_, err := relay.Step(context.Background())
			if (err != nil) != tc.wantErr || repo.ack != tc.ack || repo.retry != tc.retry || calls != tc.calls {
				t.Fatalf("err=%v ack=%d retry=%d calls=%d", err, repo.ack, repo.retry, calls)
			}
			if repo.retry > 0 && repo.delay != 4*time.Second {
				t.Fatal("incorrect backoff", repo.delay)
			}
		})
	}
}
func TestShutdownKeepsLease(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	repo := &fakeRepository{found: true}
	r := Relay{Repository: repo, Config: config.Outbox{DBTimeout: time.Second, PublishTimeout: time.Second}, Publisher: publishFunc(func(context.Context, storage.OutboxMessage) error { cancel(); return nil })}
	_, err := r.Step(ctx)
	if !errors.Is(err, context.Canceled) || repo.ack != 0 || repo.retry != 0 {
		t.Fatal("shutdown modified lease", err)
	}
}
func TestRetryDelay(t *testing.T) {
	for _, n := range []int{1, 2, 7, 2147483647} {
		want := time.Minute
		if n == 1 {
			want = time.Second
		}
		if n == 2 {
			want = 2 * time.Second
		}
		if got := retryDelay(n, time.Second, time.Minute); got != want {
			t.Fatal(n, got)
		}
	}
}

func TestPublishTimeoutSchedulesRetry(t *testing.T) {
	repo := &fakeRepository{found: true, message: storage.OutboxMessage{Attempts: 1}}
	r := Relay{Repository: repo, Config: config.Outbox{DBTimeout: time.Second, PublishTimeout: time.Millisecond, RetryMin: time.Second, RetryMax: time.Minute}, Publisher: publishFunc(func(ctx context.Context, _ storage.OutboxMessage) error { <-ctx.Done(); return ctx.Err() })}
	_, err := r.Step(context.Background())
	if !errors.Is(err, context.DeadlineExceeded) || repo.ack != 0 || repo.retry != 1 {
		t.Fatal("timed out publication not retried", err)
	}
}
