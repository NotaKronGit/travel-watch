package yandex

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"

	wire "github.com/NotaKronGit/travel-watch/api/transport"
)

type wireStation struct {
	Code, Title string
	Codes       struct {
		IATA string `json:"iata"`
	}
}

func (s wireStation) normalized() wire.Station {
	return wire.Station{Code: s.Code, Title: s.Title, IATA: s.Codes.IATA}
}

// Transport executes one bounded provider request; it never plans a route.
func (p *Provider) Transport(ctx context.Context, q wire.Request) (wire.Response, error) {
	out := wire.Response{Version: 1}
	if q.Version != 1 || q.Offset < 0 || q.Offset > 1000 {
		return out, errors.New("invalid transport request")
	}
	for _, v := range []string{q.Code, q.From, q.To, q.UID} {
		if len(v) > 200 {
			return out, errors.New("transport identifier too long")
		}
	}
	params := url.Values{"format": {"json"}, "lang": {"ru_RU"}}
	method := ""
	switch q.Method {
	case "arrivals":
		if q.Code == "" {
			return out, errors.New("airport required")
		}
		method = "schedule"
		params.Set("station", q.Code)
		params.Set("system", "iata")
		params.Set("event", "arrival")
		params.Set("transport_types", "plane")
		params.Set("offset", strconv.Itoa(q.Offset))
		params.Set("limit", "100")
	case "thread":
		if q.UID == "" {
			return out, errors.New("thread required")
		}
		method = "thread"
		params.Set("uid", q.UID)
		params.Set("show_systems", "all")
	case "search":
		if q.From == "" || q.To == "" || (q.Mode != "train" && q.Mode != "plane") || (q.System != "iata" && q.System != "yandex") {
			return out, errors.New("invalid connection query")
		}
		method = "search"
		params.Set("from", q.From)
		params.Set("to", q.To)
		params.Set("system", q.System)
		params.Set("transport_types", q.Mode)
		params.Set("transfers", "false")
		params.Set("offset", strconv.Itoa(q.Offset))
		params.Set("limit", "100")
	case "stations", "settlement":
		if q.Center == nil || math.IsNaN(q.Center.Latitude) || math.IsNaN(q.Center.Longitude) || math.Abs(q.Center.Latitude) > 90 || math.Abs(q.Center.Longitude) > 180 || q.Radius <= 0 || q.Radius > 50 || math.IsNaN(q.Radius) {
			return out, errors.New("invalid geographic query")
		}
		method = "nearest_stations"
		if q.Method == "settlement" {
			method = "nearest_settlement"
		}
		params.Set("lat", strconv.FormatFloat(q.Center.Latitude, 'f', -1, 64))
		params.Set("lng", strconv.FormatFloat(q.Center.Longitude, 'f', -1, 64))
		params.Set("distance", strconv.FormatFloat(q.Radius, 'f', -1, 64))
		if q.Method == "stations" {
			params.Set("transport_types", "train")
			params.Set("station_types", "train_station")
			params.Set("limit", "50")
		}
	default:
		return out, errors.New("unknown transport method")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.rasp.yandex-net.ru/v3.0/"+method+"/?"+params.Encode(), nil)
	if err != nil {
		return out, errors.New("request creation failed")
	}
	req.Header.Set("Authorization", p.key)
	resp, err := p.client.Do(req)
	if err != nil {
		return out, errors.New("transport provider unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return out, errors.New("Yandex HTTP " + strconv.Itoa(resp.StatusCode))
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, p.maxBytes+1))
	if err != nil || int64(len(data)) > p.maxBytes {
		return out, errors.New("transport response exceeds limit")
	}
	var raw struct {
		wireStation
		Pagination    *struct{ Total, Limit, Offset int }
		Stations      *[]wireStation
		Schedule      *[]struct{ Thread struct{ UID, Number string } }
		Stops         *[]struct{ Station wireStation }
		TransportType string `json:"transport_type"`
		Number        string
		Segments      *[]struct {
			From, To     wireStation
			HasTransfers bool `json:"has_transfers"`
			Thread       struct {
				Number        string
				TransportType string `json:"transport_type"`
			}
		}
		IntervalSchedule []json.RawMessage `json:"interval_schedule"`
		IntervalSegments []json.RawMessage `json:"interval_segments"`
	}
	if json.Unmarshal(data, &raw) != nil {
		return out, errors.New("invalid transport JSON")
	}
	switch q.Method {
	case "settlement":
		if raw.Code == "" {
			return out, errors.New("settlement not found")
		}
		out.Stations = []wire.Station{raw.normalized()}
		out.Total = 1
		out.Count = 1
	case "thread":
		if raw.Stops == nil || raw.TransportType != "plane" {
			return out, errors.New("invalid flight thread")
		}
		stops := *raw.Stops
		// Do not label a flight with intermediate landings as nonstop.
		if len(stops) != 2 {
			out.Incomplete = true
			return out, nil
		}
		out.Connections = []wire.Connection{{From: stops[0].Station.normalized(), To: stops[1].Station.normalized(), Mode: "plane", Number: raw.Number}}
		out.Count = 1
		out.Total = 1
	case "stations":
		if raw.Stations == nil {
			return out, errors.New("missing stations")
		}
		for _, s := range *raw.Stations {
			out.Stations = append(out.Stations, s.normalized())
		}
		out.Count = len(*raw.Stations)
		out.Incomplete = out.Count >= 50
	case "arrivals":
		if raw.Schedule == nil {
			return out, errors.New("missing schedule")
		}
		for _, s := range *raw.Schedule {
			if s.Thread.UID == "" {
				return out, errors.New("missing thread ID")
			}
			out.Threads = append(out.Threads, wire.Thread{UID: s.Thread.UID, Number: s.Thread.Number})
		}
		out.Count = len(*raw.Schedule)
		out.Incomplete = len(raw.IntervalSchedule) > 0
	case "search":
		if raw.Segments == nil {
			return out, errors.New("missing segments")
		}
		out.Count = len(*raw.Segments)
		for _, s := range *raw.Segments {
			if s.HasTransfers || s.Thread.TransportType != q.Mode {
				return out, errors.New("unexpected transport segment")
			}
			out.Connections = append(out.Connections, wire.Connection{From: s.From.normalized(), To: s.To.normalized(), Mode: q.Mode, Number: s.Thread.Number})
		}
		out.Incomplete = len(raw.IntervalSegments) > 0
	}
	if q.Method != "settlement" && q.Method != "thread" {
		if raw.Pagination == nil || raw.Pagination.Offset != q.Offset || raw.Pagination.Total < q.Offset+out.Count || out.Count > raw.Pagination.Limit || out.Count > 100 {
			return out, errors.New("invalid pagination")
		}
		out.Total = raw.Pagination.Total
		if q.Method == "stations" && out.Total > out.Count {
			out.Incomplete = true
		}
	}
	for _, c := range out.Connections {
		if c.From.Code == "" || c.To.Code == "" || c.From.Title == "" || c.To.Title == "" {
			return out, errors.New("invalid connection endpoints")
		}
	}
	for _, s := range out.Stations {
		if s.Code == "" || s.Title == "" {
			return out, errors.New("invalid station")
		}
	}
	return out, ctx.Err()
}
