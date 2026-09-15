// Package testsource supplies synthetic topology only. It is not a production catalog.
package testsource

import (
	"context"
	"github.com/NotaKronGit/travel-watch/services/search/internal/routes"
)

const Origin = "test-city-ivanovo"
const Destination = "test-city-pattaya"

type Source struct{}

func (Source) Load(ctx context.Context, q routes.Query, _ routes.Limits) (routes.Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return routes.Snapshot{}, err
	}
	s := Ivanovo()
	if q.OriginCityID != Origin || q.DestinationCityID != Destination {
		s.Complete = false
	}
	return s, nil
}

// Ivanovo returns a fresh fixture on every call, safe for independent tests.
// Every edge is fictitious, including local direct flights and rail connections.
func Ivanovo() routes.Snapshot {
	const moscow = "test-city-moscow"
	const nizhny = "test-city-nizhny"
	cities := []routes.City{{ID: Origin, Name: "Иваново", Timezone: "Europe/Moscow"}, {ID: moscow, Name: "Москва", Timezone: "Europe/Moscow"}, {ID: nizhny, Name: "Нижний Новгород", Timezone: "Europe/Moscow"}, {ID: Destination, Name: "Паттайя", Timezone: "Asia/Bangkok"}}
	node := func(id, city, name string, kind routes.NodeKind) routes.Node {
		zone := "Europe/Moscow"
		if city == Destination {
			zone = "Asia/Bangkok"
		}
		return routes.Node{ID: id, CityID: city, Name: name, Kind: kind, Timezone: zone}
	}
	return routes.Snapshot{Source: "synthetic-ivanovo-v1", Synthetic: true, Complete: true, Cities: cities,
		Nodes: []routes.Node{
			node("test-iwa", Origin, "Тестовый аэропорт Иваново", routes.Airport),
			node("test-ivanovo-station", Origin, "Тестовый вокзал Иваново", routes.Station),
			node("test-moscow-station", moscow, "Тестовый вокзал Москвы", routes.Station),
			node("test-svo", moscow, "Тестовое Шереметьево", routes.Airport),
			node("test-nizhny-station", nizhny, "Тестовый вокзал Нижнего Новгорода", routes.Station),
			node("test-goj", nizhny, "Тестовое Стригино", routes.Airport),
			node("test-arrival", Destination, "Тестовый аэропорт назначения", routes.Airport),
		}, Links: []routes.Link{
			{From: Origin, To: "test-iwa", Mode: routes.Transfer},
			{From: Origin, To: "test-ivanovo-station", Mode: routes.Transfer},
			{From: "test-iwa", To: "test-arrival", Mode: routes.Flight},
			{From: "test-ivanovo-station", To: "test-moscow-station", Mode: routes.Train},
			{From: "test-moscow-station", To: "test-svo", Mode: routes.Transfer},
			{From: "test-svo", To: "test-arrival", Mode: routes.Flight},
			{From: "test-ivanovo-station", To: "test-nizhny-station", Mode: routes.Train},
			{From: "test-nizhny-station", To: "test-goj", Mode: routes.Transfer},
			{From: "test-goj", To: "test-arrival", Mode: routes.Flight},
			{From: "test-arrival", To: Destination, Mode: routes.Transfer},
			// Synthetic feeder flights exercise connections in the same airport.
			{From: "test-iwa", To: "test-svo", Mode: routes.Flight},
			{From: "test-iwa", To: "test-goj", Mode: routes.Flight},
		}}
}
