// Package yandex implements online railway station discovery using Yandex Rasp.
package yandex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/NotaKronGit/travel-watch/services/collector/internal/rail"
)

const ID = "yandex-rasp"
const endpoint = "https://api.rasp.yandex-net.ru/v3.0/nearest_stations/"

// Config is supplied by the owning process; never log it because it contains a key.
// The CLI loads these values from the service YAML with environment overrides.
type Config struct {
	APIKey           string
	Timeout          time.Duration
	MaxResponseBytes int64
}

type Provider struct {
	key      string
	client   *http.Client
	maxBytes int64
}

var _ rail.StationProvider = (*Provider)(nil)

func New(c Config) (*Provider, error) {
	if strings.TrimSpace(c.APIKey) == "" || strings.ContainsAny(c.APIKey, "\r\n") || c.Timeout <= 0 || c.Timeout > time.Minute || c.MaxResponseBytes < 1 || c.MaxResponseBytes > 4<<20 {
		return nil, errors.New("invalid Yandex station provider configuration")
	}
	return &Provider{key: c.APIKey, maxBytes: c.MaxResponseBytes, client: &http.Client{
		Timeout:       c.Timeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}}, nil
}

// FindStations performs one bounded request. Text search is unsupported; no full
// station catalog is downloaded. Missing country/city/timezone stay unknown.
func (p *Provider) FindStations(ctx context.Context, q rail.StationQuery) (rail.StationResult, error) {
	if err := ctx.Err(); err != nil {
		return rail.StationResult{}, err
	}
	if err := q.Validate(); err != nil {
		return rail.StationResult{}, err
	}
	if q.Center == nil {
		return rail.StationResult{}, rail.ErrUnsupported
	}
	limit := min(q.Limit, 50) // The API documents a maximum of 50 returned stations.
	params := url.Values{
		"lat":      {strconv.FormatFloat(q.Center.Latitude, 'f', -1, 64)},
		"lng":      {strconv.FormatFloat(q.Center.Longitude, 'f', -1, 64)},
		"distance": {strconv.FormatFloat(q.RadiusKM, 'f', -1, 64)},
		"limit":    {strconv.Itoa(limit)}, "offset": {"0"},
		"transport_types": {"train"}, "lang": {"ru_RU"}, "format": {"json"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+params.Encode(), nil)
	if err != nil {
		return rail.StationResult{}, rail.ErrUnavailable
	}
	req.Header.Set("Authorization", p.key)
	res, err := p.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return rail.StationResult{}, ctx.Err()
		}
		// Transport errors can contain request details; never expose them or the key.
		return rail.StationResult{}, rail.ErrUnavailable
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		kind := rail.ErrUnavailable
		if res.StatusCode == http.StatusTooManyRequests {
			kind = rail.ErrRateLimited
		}
		// Deliberately omit response body: a proxy/provider might echo credentials.
		return rail.StationResult{}, fmt.Errorf("%w: Yandex HTTP %d", kind, res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, p.maxBytes+1))
	if err != nil || int64(len(body)) > p.maxBytes {
		return rail.StationResult{}, rail.ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return rail.StationResult{}, err
	}
	return decode(body, limit)
}

type response struct {
	Pagination *struct{ Total, Limit, Offset int }
	Stations   *[]struct {
		Code, Title   string
		Type          string
		TransportType string `json:"transport_type"`
		Lat, Lng      *float64
	}
}

func decode(body []byte, limit int) (rail.StationResult, error) {
	var raw response
	if err := json.Unmarshal(body, &raw); err != nil || raw.Pagination == nil || raw.Stations == nil {
		return rail.StationResult{}, rail.ErrUnavailable
	}
	pg := raw.Pagination
	stations := *raw.Stations
	if pg.Offset != 0 || pg.Total < len(stations) || pg.Limit < len(stations) || len(stations) > limit || (len(stations) == 0 && pg.Total != 0) {
		return rail.StationResult{}, rail.ErrUnavailable
	}
	result := rail.StationResult{Provider: ID, ObservedAt: time.Now().UTC(), Stations: make([]rail.Station, 0, len(stations))}
	seen := make(map[string]bool, len(stations))
	for _, s := range stations {
		if strings.TrimSpace(s.Code) == "" || len(s.Code) > 200 || strings.TrimSpace(s.Title) == "" || len(s.Title) > 1000 || s.Lat == nil || s.Lng == nil || s.Type != "station" || s.TransportType != "train" || seen[s.Code] {
			return rail.StationResult{}, rail.ErrUnavailable
		}
		coords := rail.Coordinates{Latitude: *s.Lat, Longitude: *s.Lng}
		if !coords.Valid() {
			return rail.StationResult{}, rail.ErrUnavailable
		}
		seen[s.Code] = true
		result.Stations = append(result.Stations, rail.Station{Ref: rail.Ref{Provider: ID, Code: s.Code}, Name: s.Title, Coordinates: &coords})
	}
	// At the documented provider cap we conservatively do not assert completeness.
	result.LimitReached = pg.Total > len(stations) || len(stations) == 50
	result.Complete = !result.LimitReached
	return result, nil
}
