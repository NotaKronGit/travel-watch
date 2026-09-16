// Package progress projects Search events into Cabinet without reading Search tables.
package progress

import (
	"context"
	"errors"
	contract "github.com/NotaKronGit/travel-watch/api/progress"
	eventsv1 "github.com/NotaKronGit/travel-watch/gen/travelwatch/events/v1"
	"github.com/NotaKronGit/travel-watch/services/cabinet/internal/config"
	"github.com/segmentio/kafka-go"
	"time"
)

type Reader interface {
	FetchMessage(context.Context) (kafka.Message, error)
	CommitMessages(context.Context, ...kafka.Message) error
}
type Repository interface {
	ApplyProgress(context.Context, *eventsv1.TripRouteBuildingUpdated, []byte) error
}
type Consumer struct {
	Reader     Reader
	Repository Repository
	Timeout    time.Duration
}

func NewReader(c config.Progress) *kafka.Reader {
	return kafka.NewReader(kafka.ReaderConfig{Brokers: c.Brokers, Topic: c.Topic, GroupID: c.GroupID, StartOffset: kafka.FirstOffset, CommitInterval: 0, MinBytes: 1, MaxBytes: 1 << 20, QueueCapacity: 1, MaxWait: time.Second, Dialer: &kafka.Dialer{Timeout: c.Timeout, DualStack: true}})
}
func (c Consumer) Run(ctx context.Context) error {
	for ctx.Err() == nil {
		m, err := c.Reader.FetchMessage(ctx)
		if err != nil {
			return errors.New("progress Kafka fetch failed")
		}
		e, err := contract.Decode(m)
		if err != nil {
			return err
		}
		op, cancel := context.WithTimeout(ctx, c.Timeout)
		err = c.Repository.ApplyProgress(op, e, m.Value)
		cancel()
		if err != nil {
			return errors.New("progress projection failed; message not acknowledged")
		}
		op, cancel = context.WithTimeout(ctx, c.Timeout)
		err = c.Reader.CommitMessages(op, m)
		cancel()
		if err != nil {
			return errors.New("progress commit failed; safe to replay")
		}
	}
	return ctx.Err()
}
