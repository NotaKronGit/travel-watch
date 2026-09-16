package yandex

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/NotaKronGit/travel-watch/services/collector/internal/rail"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

const station = `{"code":"s-test","title":"Тестовый вокзал","type":"station","transport_type":"train","lat":56.9,"lng":40.9}`

func query() rail.StationQuery {
	return rail.StationQuery{Center: &rail.Coordinates{Latitude: 56.9, Longitude: 40.9}, RadiusKM: 20, Limit: 10}
}
func provider(t *testing.T, code int, body string) *Provider {
	t.Helper()
	p, err := New(Config{APIKey: "test-only-secret", Timeout: time.Second, MaxResponseBytes: 4096})
	if err != nil {
		t.Fatal(err)
	}
	p.client.Transport = transport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Scheme != "https" || r.URL.Host != "api.rasp.yandex-net.ru" || r.URL.Path != "/v3.0/nearest_stations/" || r.URL.Query().Get("apikey") != "" || r.Header.Get("Authorization") != "test-only-secret" {
			t.Error("incorrect endpoint or key handling")
		}
		if r.URL.Query().Get("transport_types") != "train" || r.URL.Query().Get("distance") != "20" || r.URL.Query().Get("lat") != "56.9" || r.URL.Query().Get("lng") != "40.9" {
			t.Error("incorrect station query")
		}
		return &http.Response{StatusCode: code, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})
	return p
}
func TestStations(t *testing.T) {
	p := provider(t, http.StatusOK, `{"pagination":{"total":1,"limit":10,"offset":0},"stations":[`+station+`]}`)
	r, err := p.FindStations(context.Background(), query())
	if err != nil || len(r.Stations) != 1 || !r.Complete || r.LimitReached || r.Synthetic || r.ObservedAt.IsZero() {
		t.Fatalf("result: %+v %v", r, err)
	}
	s := r.Stations[0]
	if s.Ref.Provider != ID || s.Name != "Тестовый вокзал" || s.Country != "" || s.City != "" || s.Timezone != "" || s.Coordinates == nil {
		t.Fatalf("station: %+v", s)
	}
}
func TestEmptyAndPartial(t *testing.T) {
	for _, tc := range []struct {
		body     string
		complete bool
	}{
		{`{"pagination":{"total":0,"limit":10,"offset":0},"stations":[]}`, true},
		{`{"pagination":{"total":2,"limit":10,"offset":0},"stations":[` + station + `]}`, false},
	} {
		p := provider(t, http.StatusOK, tc.body)
		r, err := p.FindStations(context.Background(), query())
		if err != nil || r.Complete != tc.complete || r.LimitReached == tc.complete {
			t.Fatalf("%+v %v", r, err)
		}
	}
}
func TestInvalidResponses(t *testing.T) {
	for _, body := range []string{`{`, `{}`, `{"pagination":{"total":0},"stations":null}`, `{"pagination":{"total":-1},"stations":[]}`, `{"pagination":{"total":2},"stations":[]}`,
		`{"pagination":{"total":2,"limit":10},"stations":[` + station + `,` + station + `]}`,
		`{"pagination":{"total":1,"limit":10},"stations":[` + strings.Replace(station, `"lat":56.9`, `"lat":190`, 1) + `]}`,
		`{"pagination":{"total":1,"limit":10},"stations":[` + strings.Replace(station, `"train"`, `"bus"`, 1) + `]}`,
		strings.Repeat(" ", 4097),
	} {
		p := provider(t, http.StatusOK, body)
		r, err := p.FindStations(context.Background(), query())
		if !errors.Is(err, rail.ErrUnavailable) || r.Provider != "" {
			t.Fatalf("invalid response accepted: %+v %v", r, err)
		}
	}
}
func TestHTTPFailuresRedacted(t *testing.T) {
	for _, code := range []int{http.StatusForbidden, http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusFound} {
		p := provider(t, code, "test-only-secret")
		r, err := p.FindStations(context.Background(), query())
		want := rail.ErrUnavailable
		if code == http.StatusTooManyRequests {
			want = rail.ErrRateLimited
		}
		if !errors.Is(err, want) || strings.Contains(err.Error(), "test-only-secret") || r.Provider != "" {
			t.Fatalf("%+v %v", r, err)
		}
	}
}
func TestContextAndUnsupported(t *testing.T) {
	p := provider(t, http.StatusOK, `{}`)
	p.client.Transport = transport(func(_ *http.Request) (*http.Response, error) {
		t.Error("unexpected HTTP request")
		return nil, errors.New("unexpected")
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := p.FindStations(ctx, query()); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := p.FindStations(context.Background(), rail.StationQuery{Text: "Москва", Country: "RU", Limit: 10}); !errors.Is(err, rail.ErrUnsupported) {
		t.Fatal(err)
	}
	q := query()
	q.RadiusKM = 0
	if _, err := p.FindStations(context.Background(), q); !errors.Is(err, rail.ErrInvalidQuery) {
		t.Fatal(err)
	}
}
func TestTimeoutAndTransportRedaction(t *testing.T) {
	p := provider(t, http.StatusOK, `{}`)
	p.client.Timeout = 5 * time.Millisecond
	p.client.Transport = transport(func(r *http.Request) (*http.Response, error) { <-r.Context().Done(); return nil, r.Context().Err() })
	if _, err := p.FindStations(context.Background(), query()); !errors.Is(err, rail.ErrUnavailable) {
		t.Fatal(err)
	}
	p.client.Transport = transport(func(_ *http.Request) (*http.Response, error) { return nil, errors.New("test-only-secret") })
	if _, err := p.FindStations(context.Background(), query()); !errors.Is(err, rail.ErrUnavailable) || strings.Contains(err.Error(), "test-only-secret") {
		t.Fatal(err)
	}
}

func TestProviderCap(t *testing.T) {
	entries := make([]string, 50)
	for i := range entries {
		entries[i] = strings.Replace(station, "s-test", fmt.Sprintf("s-test-%d", i), 1)
	}
	p := provider(t, http.StatusOK, `{"pagination":{"total":50,"limit":50,"offset":0},"stations":[`+strings.Join(entries, ",")+`]}`)
	p.maxBytes = 16384
	original := p.client.Transport
	p.client.Transport = transport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Query().Get("limit") != "50" {
			t.Error("provider cap not applied")
		}
		return original.RoundTrip(r)
	})
	q := query()
	q.Limit = 100
	r, err := p.FindStations(context.Background(), q)
	if err != nil || len(r.Stations) != 50 || r.Complete || !r.LimitReached {
		t.Fatalf("cap: %+v %v", r, err)
	}
}
