package consumer

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/NotaKronGit/travel-watch/services/search/internal/config"
	"github.com/segmentio/kafka-go"
)

type Reader interface {
	FetchMessage(context.Context) (kafka.Message, error)
	CommitMessages(context.Context, ...kafka.Message) error
}
type Repository interface {
	Apply(context.Context, Event) error
}
type Consumer struct {
	Reader                   Reader
	Repository               Repository
	DBTimeout, CommitTimeout time.Duration
}

func NewReader(c config.Consumer) *kafka.Reader {
	return kafka.NewReader(kafka.ReaderConfig{
		Brokers: c.Brokers, Topic: c.Topic, GroupID: c.GroupID,
		StartOffset: kafka.FirstOffset, CommitInterval: 0,
		MinBytes: 1, MaxBytes: 1 << 20, QueueCapacity: 1, MaxWait: time.Second,
		Dialer: &kafka.Dialer{Timeout: c.DialTimeout, DualStack: true},
	})
}

// Processing is sequential: committing an offset cannot skip an unfinished message.
// Any failure stops the process; a restart resumes from the last committed offset.
func (c Consumer) Run(ctx context.Context) error {
	for {
		m, err := c.Reader.FetchMessage(ctx)
		if err != nil {
			return errors.New("Search Kafka fetch failed")
		}
		e, err := Decode(m)
		if err != nil {
			return fmt.Errorf("%w at partition %d offset %d", err, m.Partition, m.Offset)
		}
		dbctx, cancel := context.WithTimeout(ctx, c.DBTimeout)
		err = c.Repository.Apply(dbctx, e)
		cancel()
		if err != nil {
			return fmt.Errorf("Search transaction failed at partition %d offset %d", m.Partition, m.Offset)
		}
		commitctx, cancel := context.WithTimeout(ctx, c.CommitTimeout)
		err = c.Reader.CommitMessages(commitctx, m)
		cancel()
		if err != nil {
			return errors.New("Search Kafka offset commit failed; safe to replay")
		}
	}
}
