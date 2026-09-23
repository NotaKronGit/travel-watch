package yandex

import (
	"context"
	wire "github.com/NotaKronGit/travel-watch/api/transport"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestDatedSchedules(t *testing.T) {
	body := `{"pagination":{"total":1,"limit":100,"offset":0},"segments":[{"from":{"code":"s1","title":"A"},"to":{"code":"s2","title":"B"},"thread":{"number":"123","transport_type":"train"},"departure":"2027-01-10T23:00:00+03:00","arrival":"2027-01-11T03:00:00+04:00"}]}`
	q := wire.Request{Version: 1, Method: "departures", From: "s1", To: "s2", System: "yandex", Mode: "train", Date: "2027-01-10"}
	p := transportProvider(t, body, 200)
	p.client.Transport = transport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Query().Get("date") != q.Date || r.URL.Query().Get("transfers") != "false" || r.URL.Query().Get("show_systems") != "all" {
			t.Error("dated query not sent")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
	})
	result, err := p.Transport(context.Background(), q)
	if err != nil || len(result.Departures) != 1 || result.ObservedAt.IsZero() {
		t.Fatal(err)
	}
	if result.Departures[0].Arrival.Sub(result.Departures[0].Departure).Hours() != 3 {
		t.Fatal("offset ignored")
	}
	for _, invalid := range []string{strings.Replace(body, "+03:00", "", 1), strings.Replace(body, `"code":"s2"`, `"code":"s3"`, 1), strings.Replace(body, "2027-01-11T03", "2027-01-09T03", 1)} {
		if _, err := transportProvider(t, invalid, 200).Transport(context.Background(), q); err == nil {
			t.Fatal("invalid dated response accepted")
		}
	}
	q.Date = ""
	if _, err := p.Transport(context.Background(), q); err == nil {
		t.Fatal("undated timetable accepted")
	}
}

func TestDatedIATAResolution(t *testing.T) {
	body := `{"search":{"from":{"code":"s1","type":"station"},"to":{"code":"s2","type":"station"}},"pagination":{"total":1,"limit":100,"offset":0},"segments":[{"from":{"code":"s1","title":"A","codes":{"iata":"AAA"}},"to":{"code":"s2","title":"B","codes":{"iata":"BBB"}},"thread":{"number":"F1","transport_type":"plane"},"departure":"2027-01-10T23:00:00+03:00","arrival":"2027-01-11T03:00:00+04:00"}]}`
	q := wire.Request{Version: 1, Method: "departures", From: "AAA", To: "BBB", System: "iata", Mode: "plane", Date: "2027-01-10"}
	r, err := transportProvider(t, body, 200).Transport(context.Background(), q)
	if err != nil || len(r.Departures) != 1 || r.Departures[0].From.IATA != "AAA" || r.Departures[0].To.IATA != "BBB" {
		t.Fatal("unresolved IATA", err)
	}
	body = strings.Replace(body, `"iata":"BBB"`, `"iata":"CCC"`, 1)
	if _, err := transportProvider(t, body, 200).Transport(context.Background(), q); err == nil {
		t.Fatal("misidentified airport accepted")
	}
}
