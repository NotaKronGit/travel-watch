package catalog

import (
	"archive/zip"
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Synthetic records in the upstream format; no claim of real geography.
const countriesTSV = "# fixture\nRU\tRUS\t643\tRS\tTest country\tCapital\t1\t1\tEU\t.test\tTST\tTest\t0\t\t\tru\t1\t\t\n"
const citiesTSV = "1\tTest city\tTest city\tТестовый город,Test alias\t55\t37\tP\tPPL\tRU\t\t01\t\t\t\t1000\t\t0\tEurope/Moscow\t2026-01-01\n"

func zipData(t *testing.T, text string) []byte { return namedZip(t, "cities500.txt", text) }
func namedZip(t *testing.T, name, text string) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	f, err := z.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.Write([]byte(text)); err != nil {
		t.Fatal(err)
	}
	if err = z.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func TestGeoNamesSource(t *testing.T) {
	for _, tc := range []struct {
		name, cityText      string
		status              int
		badZIP              bool
		maxDownload, maxRaw int64
		maxCities           int
		wantErr             bool
	}{
		{name: "valid", cityText: citiesTSV},
		{name: "malformed", cityText: "bad\trow\n", wantErr: true},
		{name: "numeric", cityText: strings.Replace(citiesTSV, "\t55\t", "\tNaNbad\t", 1), wantErr: true},
		{name: "corrupt ZIP", badZIP: true, wantErr: true},
		{name: "HTTP failure", status: 503, wantErr: true},
		{name: "download limit", maxDownload: 10, wantErr: true},
		{name: "uncompressed limit", cityText: citiesTSV, maxRaw: 10, wantErr: true},
		{name: "city count limit", cityText: citiesTSV + citiesTSV, maxCities: 1, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			archive := zipData(t, tc.cityText)
			translations := namedZip(t, "alternateNamesV2.txt", alternate("10", "1", "ru", "Тестовый город", "1", "", "", "", "", "")+alternate("11", "2", "ru", "Тестовый регион", "1", "", "", "", "", ""))
			if tc.badZIP {
				archive = []byte("not zip")
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.status != 0 {
					w.WriteHeader(tc.status)
					return
				}
				if r.URL.Path == "/names" {
					_, _ = w.Write(translations)
				} else if r.URL.Path == "/regions" {
					_, _ = w.Write([]byte("RU.01\tTest region\tTest region\t2\n"))
				} else if r.URL.Path == "/countries" {
					_, _ = w.Write([]byte(countriesTSV))
				} else {
					_, _ = w.Write(archive)
				}
			}))
			defer server.Close()
			source := GeoNamesSource{AlternateNamesURL: server.URL + "/names", MaxAlternateDownloadBytes: 100000, MaxAlternateUncompressedBytes: 100000, Client: server.Client(), CitiesURL: server.URL + "/cities", CountriesURL: server.URL + "/countries", RegionsURL: server.URL + "/regions", MaxDownloadBytes: 100000, MaxUncompressedBytes: 100000, MaxCities: 10}
			if tc.maxDownload != 0 {
				source.MaxDownloadBytes = tc.maxDownload
			}
			if tc.maxRaw != 0 {
				source.MaxUncompressedBytes = tc.maxRaw
			}
			if tc.maxCities != 0 {
				source.MaxCities = tc.maxCities
			}
			s, err := source.Load(context.Background())
			if (err != nil) != tc.wantErr {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tc.wantErr {
				if err := Validate(s, 1); err != nil {
					t.Fatal(err)
				}
				if s.Cities[0].RegionName != "Тестовый регион" || s.Cities[0].NameRu != "Тестовый город" || s.Cities[0].Aliases[0] != "Тестовый город" || len(s.Version) != 64 {
					t.Fatal("lost aliases or provenance")
				}
			}
		})
	}
}
func TestGeoNamesTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err := (GeoNamesSource{Client: server.Client(), CitiesURL: server.URL, CountriesURL: server.URL, MaxDownloadBytes: 100, MaxUncompressedBytes: 100, MaxCities: 10}).Load(ctx)
	if err == nil {
		t.Fatal("expected timeout")
	}
}
