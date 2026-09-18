package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/NotaKronGit/travel-watch/api/progress"
	"github.com/NotaKronGit/travel-watch/api/transport"
	eventsv1 "github.com/NotaKronGit/travel-watch/gen/travelwatch/events/v1"
	"github.com/NotaKronGit/travel-watch/services/search/internal/config"
	"github.com/NotaKronGit/travel-watch/services/search/internal/gemini"
	"github.com/NotaKronGit/travel-watch/services/search/internal/planning"
	"github.com/NotaKronGit/travel-watch/services/search/internal/realroutes"
	"github.com/NotaKronGit/travel-watch/services/search/internal/routeexperiment"
	"github.com/NotaKronGit/travel-watch/services/search/internal/storage"
	"github.com/segmentio/kafka-go"
	"google.golang.org/protobuf/proto"
	"log/slog"
	"time"
)

func buildRoutes(ctx context.Context, db *sql.DB, c config.Config) error {
	store := storage.NewBuilder(db, c.Progress.Sources)
	build := func(ctx context.Context, payload []byte) (realroutes.Result, error) {
		e := new(eventsv1.TripRequestCreated)
		if proto.Unmarshal(payload, e) != nil || e.Origin == nil || e.Destination == nil {
			return realroutes.Result{}, errors.New("invalid building snapshot")
		}
		read, cancel := context.WithTimeout(ctx, c.Progress.Timeout)
		catalog, err := store.PlannerAirports(read)
		cancel()
		if err != nil || len(catalog) == 0 {
			return realroutes.Result{}, errors.New("airport catalog unavailable")
		}
		provider, err := realroutes.Start(ctx, c.Planner.CollectorBinary)
		if err != nil {
			return realroutes.Result{}, err
		}
		defer provider.Close()
		q := realroutes.Query{DepartureFrom: e.DepartureFrom, DepartureTo: e.DepartureTo, Adults: int(e.Adults), OriginName: e.Origin.Name, DestinationName: e.Destination.Name, Origin: transport.Point{Latitude: e.Origin.Latitude, Longitude: e.Origin.Longitude}, Destination: transport.Point{Latitude: e.Destination.Latitude, Longitude: e.Destination.Longitude}}
		return (realroutes.Planner{Provider: provider, Airports: catalog, Config: c.Planner}).Plan(ctx, q)
	}
	sources := map[string]planning.Source{
		"graph": func(ctx context.Context, payload []byte) (planning.SourceResult, error) {
			r, err := build(ctx, payload)
			data, marshalErr := json.Marshal(r)
			if marshalErr != nil {
				return planning.SourceResult{}, marshalErr
			}
			if err == nil && r.ProviderFailures > 0 && len(r.Candidates) == 0 {
				err = errors.New("transport source failed")
			}
			return planning.SourceResult{Data: data, Count: len(r.Candidates), Incomplete: !r.Complete}, err
		},
		"gemini": func(ctx context.Context, payload []byte) (planning.SourceResult, error) {
			e := new(eventsv1.TripRequestCreated)
			if proto.Unmarshal(payload, e) != nil || e.Origin == nil || e.Destination == nil {
				return planning.SourceResult{}, errors.New("invalid building snapshot")
			}
			p, err := gemini.New(c.Gemini)
			if err != nil {
				return planning.SourceResult{Outcome: "unavailable", Incomplete: true}, err
			}
			paths, err := p.Search(ctx, routeexperiment.Query{Origin: e.Origin.Name, Destination: e.Destination.Name, MaxCandidates: min(c.Planner.MaxCandidates, 10)})
			if err != nil {
				var failure *gemini.Failure
				if errors.As(err, &failure) {
					slog.WarnContext(ctx, "Gemini route building failed", "request_id", e.RequestId, "reason", failure.SafeMessage())
				}
				return planning.SourceResult{Incomplete: true}, err
			}
			data, err := json.Marshal(struct {
				Source   string                 `json:"source"`
				Verified bool                   `json:"verified"`
				Paths    []routeexperiment.Path `json:"paths"`
			}{"gemini", false, paths})
			return planning.SourceResult{Data: data, Count: len(paths), Incomplete: true}, err
		},
	}
	return (planning.MultiWorker{Repository: store, Sources: sources, Poll: c.Progress.PollInterval, DBTimeout: c.Progress.Timeout, BuildTimeout: c.Planner.Timeout, Lease: c.Progress.Lease, MaxAttempts: c.Progress.MaxAttempts}).Run(ctx)
}
func publishProgress(ctx context.Context, db *sql.DB, c config.Config) error {
	tr := &kafka.Transport{DialTimeout: c.Progress.Timeout, MetadataTopics: []string{c.Progress.Topic}}
	defer tr.CloseIdleConnections()
	writer := &kafka.Writer{Addr: kafka.TCP(c.Progress.Brokers...), Topic: c.Progress.Topic, Balancer: &kafka.Hash{}, RequiredAcks: kafka.RequireAll, MaxAttempts: 1, BatchSize: 1, ReadTimeout: c.Progress.Timeout, WriteTimeout: c.Progress.Timeout, Transport: tr}
	defer writer.Close()
	store := storage.New(db)
	for ctx.Err() == nil {
		op, cancel := context.WithTimeout(ctx, c.Progress.Timeout)
		m, found, err := store.ClaimProgress(op, 3*c.Progress.Timeout)
		cancel()
		if err != nil {
			return err
		}
		if found {
			op, cancel = context.WithTimeout(ctx, c.Progress.Timeout)
			err = writer.WriteMessages(op, kafka.Message{Key: []byte(m.RequestID), Value: m.Payload, Headers: []kafka.Header{{Key: "event_type", Value: []byte(progress.EventType)}}})
			cancel()
			if err == nil {
				op, cancel = context.WithTimeout(ctx, c.Progress.Timeout)
				err = store.AckProgress(op, m)
				cancel()
				if err != nil {
					return err
				}
				continue
			}
		}
		timer := time.NewTimer(c.Progress.PollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
		case <-timer.C:
		}
	}
	return ctx.Err()
}
