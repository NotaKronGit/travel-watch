package catalog

import (
	"context"
	"errors"
	"testing"
)

// A deterministic test source; never downloads data or reads production config.
type fakeSource struct {
	snapshot Snapshot
	err      error
}

func (s fakeSource) Load(ctx context.Context) (Snapshot, error) { return s.snapshot, s.err }

type fakeRepository struct {
	saved Snapshot
	calls int
	err   error
}

func (r *fakeRepository) ReplaceCatalog(ctx context.Context, s Snapshot) error {
	r.calls++
	r.saved = s
	return r.err
}
func fixture() Snapshot {
	return Snapshot{Source: "test", Version: "fixture-1", Countries: []Country{{Code: "RU", Name: "Test country"}}, Cities: []City{{SourceID: 1, Name: "Test city", NameRu: "Тестовый город", CountryCode: "RU", Timezone: "Europe/Moscow"}}}
}
func TestImporter(t *testing.T) {
	for _, tc := range []struct {
		name               string
		change             func(*Snapshot)
		sourceErr, repoErr error
		min                int
		calls              int
		wantErr            bool
	}{
		{name: "success", min: 1, calls: 1},
		{name: "source failure", min: 1, sourceErr: errors.New("offline"), wantErr: true},
		{name: "no translations", min: 1, change: func(s *Snapshot) { s.Cities[0].NameRu = "" }, wantErr: true},
		{name: "empty", min: 1, change: func(s *Snapshot) { s.Cities = nil }, wantErr: true},
		{name: "too few", min: 2, wantErr: true},
		{name: "duplicate", min: 1, change: func(s *Snapshot) { s.Cities = append(s.Cities, s.Cities[0]) }, wantErr: true},
		{name: "unknown country", min: 1, change: func(s *Snapshot) { s.Cities[0].CountryCode = "XX" }, wantErr: true},
		{name: "timezone", min: 1, change: func(s *Snapshot) { s.Cities[0].Timezone = "missing/zone" }, wantErr: true},
		{name: "invalid latitude", min: 1, change: func(s *Snapshot) { s.Cities[0].Latitude = 100 }, wantErr: true},
		{name: "storage failure", min: 1, repoErr: errors.New("db unavailable"), calls: 1, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := fixture()
			if tc.change != nil {
				tc.change(&s)
			}
			repo := &fakeRepository{err: tc.repoErr}
			n, err := (Importer{Source: fakeSource{snapshot: s, err: tc.sourceErr}, Repository: repo, MinCities: tc.min}).Run(context.Background())
			if (err != nil) != tc.wantErr || repo.calls != tc.calls {
				t.Fatalf("count=%d err=%v writes=%d", n, err, repo.calls)
			}
			if !tc.wantErr && (n != 1 || repo.saved.Version != "fixture-1") {
				t.Fatal("snapshot not saved")
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	repo := &fakeRepository{}
	_, err := (Importer{Source: fakeSource{snapshot: fixture()}, Repository: repo, MinCities: 1}).Run(ctx)
	if !errors.Is(err, context.Canceled) || repo.calls != 0 {
		t.Fatal("cancelled import wrote data")
	}
}

func TestImporterSkipsUntranslatedCities(t *testing.T) {
	s := fixture()
	s.Cities = append(s.Cities, City{SourceID: 2, Name: "Untranslated", CountryCode: "RU", Timezone: "Europe/Moscow"})
	repo := &fakeRepository{}
	n, err := (Importer{Source: fakeSource{snapshot: s}, Repository: repo, MinCities: 2}).Run(context.Background())
	if err != nil || n != 1 || len(repo.saved.Cities) != 1 || repo.saved.Cities[0].SourceID != 1 {
		t.Fatalf("untranslated city reached storage: count=%d err=%v", n, err)
	}
}
