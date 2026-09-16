package main

import (
	"context"
	"database/sql"
	"errors"
	"github.com/NotaKronGit/travel-watch/api/progress"
	"github.com/NotaKronGit/travel-watch/api/transport"
	eventsv1 "github.com/NotaKronGit/travel-watch/gen/travelwatch/events/v1"
	"github.com/NotaKronGit/travel-watch/services/search/internal/config"
	"github.com/NotaKronGit/travel-watch/services/search/internal/planning"
	"github.com/NotaKronGit/travel-watch/services/search/internal/realroutes"
	"github.com/NotaKronGit/travel-watch/services/search/internal/storage"
	"github.com/segmentio/kafka-go"
	"google.golang.org/protobuf/proto"
	"time"
)

func buildRoutes(ctx context.Context, db *sql.DB, c config.Config) error {
	store := storage.New(db)
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
		q := realroutes.Query{OriginName: e.Origin.Name, DestinationName: e.Destination.Name, Origin: transport.Point{Latitude: e.Origin.Latitude, Longitude: e.Origin.Longitude}, Destination: transport.Point{Latitude: e.Destination.Latitude, Longitude: e.Destination.Longitude}}
		return (realroutes.Planner{Provider: provider, Airports: catalog, Config: c.Planner}).Plan(ctx, q)
	}
	return (planning.Worker{Repository: store, Build: build, Poll: c.Progress.PollInterval, DBTimeout: c.Progress.Timeout, BuildTimeout: c.Planner.Timeout, Lease: c.Progress.Lease, MaxAttempts: c.Progress.MaxAttempts}).Run(ctx)
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
