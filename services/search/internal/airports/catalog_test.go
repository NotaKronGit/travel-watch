package airports

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

type fixtureSource struct {
	snapshot Snapshot
	err      error
}

func (f fixtureSource) Load(context.Context) (Snapshot, error) { return f.snapshot, f.err }

type fakeRepository struct{ calls int }

func (r *fakeRepository) PublishAirports(context.Context, Snapshot, int) error { r.calls++; return nil }
func fixture(t *testing.T) ([]byte, Snapshot) {
	t.Helper()
	data, err := os.ReadFile("testdata/airports.csv")
	if err != nil {
		t.Fatal(err)
	}
	s, err := Parse(context.Background(), data, time.Now(), 10)
	if err != nil {
		t.Fatal(err)
	}
	return data, s
}
func TestImportValidation(t *testing.T) {
	_, s := fixture(t)
	r := &fakeRepository{}
	i := Importer{Source: fixtureSource{snapshot: s}, Repository: r, MinRows: 2, MaxRows: 10, MinRetainedPercent: 90}
	if n, err := i.Run(context.Background()); err != nil || n != 2 || r.calls != 1 {
		t.Fatal(n, err)
	}
	s.Airports[1].SourceID = 1
	i.Source = fixtureSource{snapshot: s}
	if _, err := i.Run(context.Background()); err == nil || r.calls != 1 {
		t.Fatal("duplicate published")
	}
	i.Source = fixtureSource{err: errors.New("offline")}
	if _, err := i.Run(context.Background()); err == nil || r.calls != 1 {
		t.Fatal("failure published")
	}
	_, s = fixture(t)
	i.Source = fixtureSource{snapshot: s}
	i.MinRows = 3
	if _, err := i.Run(context.Background()); err == nil || r.calls != 1 {
		t.Fatal("partial snapshot published")
	}
}
func TestCSVFailures(t *testing.T) {
	data, _ := fixture(t)
	for _, bad := range []string{strings.Replace(string(data), "latitude_deg", "missing", 1), strings.Replace(string(data), ",57,", ",NaN,", 1), strings.Replace(string(data), ",140,", ",181,", 1), strings.Replace(string(data), ",yes", ",maybe", 1), string(data) + "broken", strings.Replace(string(data), "Test airport one", `"unclosed`, 1)} {
		if _, err := Parse(context.Background(), []byte(bad), time.Now(), 10); err == nil {
			t.Fatal("bad CSV accepted")
		}
	}
	if _, err := Parse(context.Background(), data, time.Now(), 1); err == nil {
		t.Fatal("row limit ignored")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Parse(ctx, data, time.Now(), 10); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
func TestDownload(t *testing.T) {
	data, _ := fixture(t)
	for _, tc := range []struct {
		status int
		limit  int64
		ok     bool
	}{{200, 4096, true}, {500, 4096, false}, {200, 100, false}} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(tc.status); _, _ = w.Write(data) }))
		s := OurAirports{URL: server.URL, Timeout: time.Second, MaxBytes: tc.limit, MaxRows: 10}
		_, err := s.Load(context.Background())
		server.Close()
		if (err == nil) != tc.ok {
			t.Fatal(err)
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/other", http.StatusFound) }))
	defer server.Close()
	if _, err := (OurAirports{URL: server.URL, Timeout: time.Second, MaxBytes: 4096, MaxRows: 10}).Load(context.Background()); err == nil {
		t.Fatal("redirect accepted")
	}
}
