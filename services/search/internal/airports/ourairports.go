package airports

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const URL = "https://davidmegginson.github.io/ourairports-data/airports.csv"

type OurAirports struct {
	URL      string
	Timeout  time.Duration
	MaxBytes int64
	MaxRows  int
}

func (s OurAirports) Load(ctx context.Context) (Snapshot, error) {
	if s.Timeout <= 0 || s.MaxBytes < 1 || s.MaxRows < 1 {
		return Snapshot{}, errors.New("invalid airport download limits")
	}
	started := time.Now().UTC()
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.URL, nil)
	if err != nil {
		return Snapshot{}, errors.New("invalid airport source URL")
	}
	client := http.Client{Timeout: s.Timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return Snapshot{}, errors.New("airport download failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Snapshot{}, errors.New("airport source returned non-200 status")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, s.MaxBytes+1))
	if err != nil || int64(len(data)) > s.MaxBytes {
		return Snapshot{}, errors.New("airport download incomplete or too large")
	}
	return Parse(ctx, data, started, s.MaxRows)
}
func Parse(ctx context.Context, data []byte, at time.Time, maxRows int) (Snapshot, error) {
	r := csv.NewReader(bytes.NewReader(data))
	header, err := r.Read()
	if err != nil {
		return Snapshot{}, errors.New("airport CSV header missing")
	}
	columns := map[string]int{}
	for j, h := range header {
		h = strings.TrimPrefix(h, "\ufeff")
		if _, ok := columns[h]; ok {
			return Snapshot{}, errors.New("duplicate airport CSV column")
		}
		columns[h] = j
	}
	required := []string{"id", "ident", "name", "type", "latitude_deg", "longitude_deg", "iso_country", "iso_region", "municipality", "iata_code", "icao_code", "scheduled_service"}
	for _, h := range required {
		if _, ok := columns[h]; !ok {
			return Snapshot{}, errors.New("airport CSV required column missing")
		}
	}
	s := Snapshot{FetchedAt: at}
	for {
		if err = ctx.Err(); err != nil {
			return Snapshot{}, err
		}
		row, e := r.Read()
		if errors.Is(e, io.EOF) {
			break
		}
		if e != nil {
			return Snapshot{}, errors.New("malformed airport CSV")
		}
		if len(s.Airports) >= maxRows {
			return Snapshot{}, errors.New("airport row limit exceeded")
		}
		get := func(k string) string { return strings.TrimSpace(row[columns[k]]) }
		id, e1 := strconv.ParseInt(get("id"), 10, 64)
		lat, e2 := strconv.ParseFloat(get("latitude_deg"), 64)
		lon, e3 := strconv.ParseFloat(get("longitude_deg"), 64)
		scheduled := get("scheduled_service")
		if e1 != nil || e2 != nil || e3 != nil || (scheduled != "yes" && scheduled != "no") {
			return Snapshot{}, errors.New("invalid airport CSV value")
		}
		s.Airports = append(s.Airports, Airport{SourceID: id, Ident: get("ident"), Name: get("name"), Type: get("type"), Latitude: lat, Longitude: lon, Country: get("iso_country"), Region: get("iso_region"), Municipality: get("municipality"), IATA: get("iata_code"), ICAO: get("icao_code"), Scheduled: scheduled == "yes"})
	}
	if err = Validate(ctx, s, 1, maxRows); err != nil {
		return Snapshot{}, err
	}
	return s, nil
}
