package catalog

import (
	"context"
	"crypto/sha256"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func alternate(id, geoID, language, name, preferred, short, colloquial, historic, from, to string) string {
	return strings.Join([]string{id, geoID, language, name, preferred, short, colloquial, historic, from, to}, "\t") + "\n"
}
func TestRussianNameSelection(t *testing.T) {
	rows := []string{
		alternate("12", "1", "ru", "Другой краткий", "1", "1", "", "", "", ""),
		alternate("20", "1", "ru", "Вариант", "", "", "", "", "", ""),
		alternate("11", "1", "ru", "Предпочтительный", "1", "", "", "", "", ""),
		alternate("10", "1", "ru", "Краткий", "1", "1", "", "", "", ""),
		alternate("9", "1", "ru", "Старинный", "1", "1", "", "1", "", ""),
		alternate("8", "1", "ru", "Разговорный", "1", "1", "1", "", "", ""),
		alternate("7", "1", "ru", "Прошлый", "1", "1", "", "", "", "1990"),
		alternate("6", "1", "en", "English", "1", "1", "", "", "", ""),
		alternate("5", "2", "ru", "Тестовая страна", "1", "", "", "", "", ""),
		alternate("4", "999", "ru", "Вне набора", "1", "", "", "", "", ""),
	}
	for _, reverse := range []bool{false, true} {
		if reverse {
			for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
				rows[i], rows[j] = rows[j], rows[i]
			}
		}
		s := fixture()
		s.Countries[0].SourceID = 2
		s.Cities = append(s.Cities, City{SourceID: 3, Name: "Untranslated", CountryCode: "RU", Timezone: "Europe/Moscow"})
		if err := applyRussianNames(context.Background(), &s, strings.NewReader(strings.Join(rows, ""))); err != nil {
			t.Fatal(err)
		}
		if s.Cities[0].NameRu != "Краткий" || s.Cities[0].Name != "Test city" || s.Countries[0].NameRu != "Тестовая страна" || s.Cities[1].NameRu != "" {
			t.Fatalf("incorrect names: %+v", s)
		}
	}
}
func TestAlternateNamesFailuresAndCleanup(t *testing.T) {
	valid := alternate("1", "1", "ru", "Город", "1", "", "", "", "", "")
	for _, tc := range []struct {
		name, content string
		status        int
		limit         int64
		rawLimit      int64
		invalidZIP    bool
		wantErr       bool
	}{
		{name: "success", content: valid},
		{name: "empty", wantErr: true},
		{name: "malformed", content: "bad\trow\n", wantErr: true},
		{name: "HTTP", status: 503, wantErr: true},
		{name: "size", content: valid, limit: 10, wantErr: true},
		{name: "uncompressed size", content: valid, rawLimit: 10, wantErr: true},
		{name: "ZIP", invalidZIP: true, wantErr: true},
		{name: "only other languages", content: alternate("1", "1", "en", "City", "1", "", "", "", "", ""), wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("TMPDIR", dir)
			data := namedZip(t, "alternateNamesV2.txt", tc.content)
			if tc.invalidZIP {
				data = []byte("bad zip")
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.status != 0 {
					w.WriteHeader(tc.status)
					return
				}
				_, _ = w.Write(data)
			}))
			defer server.Close()
			src := GeoNamesSource{Client: server.Client(), AlternateNamesURL: server.URL, MaxAlternateDownloadBytes: 100000, MaxAlternateUncompressedBytes: 100000}
			if tc.limit > 0 {
				src.MaxAlternateDownloadBytes = tc.limit
			}
			if tc.rawLimit > 0 {
				src.MaxAlternateUncompressedBytes = tc.rawLimit
			}
			s := fixture()
			err := src.localize(context.Background(), &s, sha256.New())
			if (err != nil) != tc.wantErr {
				t.Fatalf("error=%v", err)
			}
			files, err := os.ReadDir(dir)
			if err != nil || len(files) != 0 {
				t.Fatal("temporary archive not removed", err)
			}
		})
	}
}

func TestCityIATACodes(t *testing.T) {
	for _, tc := range []struct {
		name  string
		codes []string
		want  string
	}{
		{"single", []string{"ABC"}, "ABC"},
		{"duplicate", []string{"ABC", "ABC"}, "ABC"},
		{"ambiguous", []string{"ABC", "DEF"}, ""},
		{"invalid", []string{"AB1"}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := fixture()
			rows := alternate("1", "1", "ru", "Город", "", "", "", "", "", "")
			for _, code := range tc.codes {
				rows += alternate("2", "1", "iata", code, "", "", "", "", "", "")
			}
			rows += alternate("3", "999", "iata", "XYZ", "", "", "", "", "", "")
			rows += alternate("4", "1", "iata", "OLD", "", "", "", "1", "", "")
			if err := applyRussianNames(context.Background(), &s, strings.NewReader(rows)); err != nil {
				t.Fatal(err)
			}
			if s.Cities[0].IATACode != tc.want {
				t.Fatalf("code=%q, want %q", s.Cities[0].IATACode, tc.want)
			}
		})
	}
}
