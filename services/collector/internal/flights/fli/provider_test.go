package fli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/NotaKronGit/travel-watch/services/collector/internal/flights"
)

// The test binary replaces Python; no package installation or network in CI.
func TestBridgeProcess(t *testing.T) {
	if len(os.Args) != 2 || filepath.Ext(os.Args[1]) != ".flightfixture" {
		return
	}
	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		os.Exit(2)
	}
	if string(data) == "wait" {
		time.Sleep(5 * time.Second)
		os.Exit(0)
	}
	fmt.Print(string(data))
	os.Exit(0)
}
func fixtureProvider(t *testing.T, reply string) *Provider {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "reply.flightfixture")
	if err = os.WriteFile(path, []byte(reply), 0600); err != nil {
		t.Fatal(err)
	}
	p, err := New(Config{Python: exe, Bridge: path, Timeout: 3 * time.Second, MaxResponseBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func TestProviderFailuresAndCancellation(t *testing.T) {
	q := flights.Query{From: "SVO", To: "BKK", Date: "2027-01-10", Adults: 2, Limit: 5}
	for _, tc := range []struct {
		reply string
		want  error
	}{
		{`{"version":1,"error":"rate_limited"}`, flights.ErrRateLimited},
		{`{"version":1,"error":"unsupported"}`, flights.ErrUnsupported},
		{`{"version":1,"error":"invalid_response"}`, flights.ErrInvalidResponse},
		{`{"version":1,"error":"unavailable"}`, flights.ErrUnavailable},
		{`{`, flights.ErrInvalidResponse},
		{`{"version":1,"result":{}}`, flights.ErrInvalidResponse},
	} {
		t.Run(tc.reply, func(t *testing.T) {
			_, err := fixtureProvider(t, tc.reply).FindFlights(context.Background(), q)
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v want %v", err, tc.want)
			}
		})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := fixtureProvider(t, "wait").FindFlights(ctx, q); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
}
func TestProviderSuccessfulEmptyIsIncomplete(t *testing.T) {
	q := flights.Query{From: "SVO", To: "BKK", Date: "2027-01-10", Adults: 2, Limit: 5}
	raw, _ := json.Marshal(map[string]any{"version": 1, "result": flights.Result{Provider: "google-flights-fli", Query: q, ObservedAt: time.Now(), Options: []flights.Option{}}})
	r, err := fixtureProvider(t, string(raw)).FindFlights(context.Background(), q)
	if err != nil || r.Complete || len(r.Options) != 0 {
		t.Fatal(r, err)
	}
}
