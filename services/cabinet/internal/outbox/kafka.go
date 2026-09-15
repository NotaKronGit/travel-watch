package outbox

import (
	"context"

	"github.com/NotaKronGit/travel-watch/services/cabinet/internal/config"
	"github.com/NotaKronGit/travel-watch/services/cabinet/internal/storage"
	"github.com/segmentio/kafka-go"
)

type KafkaPublisher struct {
	writer    *kafka.Writer
	transport *kafka.Transport
}

func NewKafkaPublisher(c config.Outbox) *KafkaPublisher {
	transport := &kafka.Transport{DialTimeout: c.PublishTimeout, MetadataTopics: []string{c.Topic}}
	return &KafkaPublisher{transport: transport, writer: &kafka.Writer{
		Addr: kafka.TCP(c.Brokers...), Topic: c.Topic, Balancer: &kafka.Hash{},
		RequiredAcks: kafka.RequireAll, MaxAttempts: 1, BatchSize: 1,
		ReadTimeout: c.PublishTimeout, WriteTimeout: c.PublishTimeout, Transport: transport,
	}}
}
func (p *KafkaPublisher) Publish(ctx context.Context, m storage.OutboxMessage) error {
	return p.writer.WriteMessages(ctx, kafka.Message{Key: []byte(m.RequestID), Value: m.Payload, Headers: []kafka.Header{{Key: "event_type", Value: []byte(m.EventType)}}})
}
func (p *KafkaPublisher) Close() error {
	err := p.writer.Close()
	p.transport.CloseIdleConnections()
	return err
}
