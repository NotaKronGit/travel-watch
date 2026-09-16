package yandex

import (
	"context"
	wire "github.com/NotaKronGit/travel-watch/api/transport"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func transportProvider(t *testing.T, body string, status int) *Provider {
	t.Helper()
	p, err := New(Config{APIKey: "test-secret", Timeout: time.Second, MaxResponseBytes: 4096})
	if err != nil {
		t.Fatal(err)
	}
	p.client.Transport = transport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Query().Get("apikey") != "" || r.Header.Get("Authorization") != "test-secret" {
			t.Fatal("key exposed or missing")
		}
		if r.URL.Query().Get("transfers") == "true" {
			t.Fatal("provider asked to plan a route")
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
	})
	return p
}
func TestTransportPaginationAndNoConnections(t *testing.T) {
	q := wire.Request{Version: 1, Method: "search", From: "A", To: "B", System: "iata", Mode: "plane", Offset: 100}
	for _, body := range []string{`{}`, `{"pagination":{"total":200,"limit":100,"offset":0},"segments":[]}`, `{"pagination":{"total":1,"limit":100,"offset":100},"segments":[]}`} {
		if _, err := transportProvider(t, body, 200).Transport(context.Background(), q); err == nil {
			t.Fatal("invalid page accepted")
		}
	}
	q.Offset = 0
	r, err := transportProvider(t, `{"pagination":{"total":0,"limit":100,"offset":0},"segments":[]}`, 200).Transport(context.Background(), q)
	if err != nil || r.Count != 0 || r.Total != 0 {
		t.Fatal("empty page failed")
	}
	if _, err = transportProvider(t, "test-secret", 429).Transport(context.Background(), q); err == nil || strings.Contains(err.Error(), "test-secret") {
		t.Fatal("rate error or secret handling failed")
	}
}
func TestFlightThreadRequiresTwoStops(t *testing.T) {
	q := wire.Request{Version: 1, Method: "thread", UID: "flight"}
	body := `{"transport_type":"plane","number":"1","stops":[{"station":{"code":"A","title":"A","codes":{"iata":"AAA"}}},{"station":{"code":"B","title":"B","codes":{"iata":"BBB"}}}]}`
	r, err := transportProvider(t, body, 200).Transport(context.Background(), q)
	if err != nil || len(r.Connections) != 1 || r.Connections[0].To.IATA != "BBB" {
		t.Fatal("missing flight endpoints")
	}
	r, err = transportProvider(t, `{"transport_type":"plane","stops":[]}`, 200).Transport(context.Background(), q)
	if err != nil || !r.Incomplete || len(r.Connections) != 0 {
		t.Fatal("unknown/nonstop topology invented")
	}
}
