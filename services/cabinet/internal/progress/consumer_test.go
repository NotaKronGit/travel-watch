package progress

import (
	"context"
	"errors"
	contract "github.com/NotaKronGit/travel-watch/api/progress"
	eventsv1 "github.com/NotaKronGit/travel-watch/gen/travelwatch/events/v1"
	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"testing"
	"time"
)

type fakeReader struct {
	m          kafka.Message
	commits    int
	read       bool
	failCommit bool
}

func (f *fakeReader) FetchMessage(context.Context) (kafka.Message, error) {
	if f.read {
		return kafka.Message{}, errors.New("end")
	}
	f.read = true
	return f.m, nil
}
func (f *fakeReader) CommitMessages(context.Context, ...kafka.Message) error {
	f.commits++
	if f.failCommit {
		return errors.New("lost commit")
	}
	return nil
}

type fakeStore struct {
	calls int
	fail  bool
}

func (f *fakeStore) ApplyProgress(context.Context, *eventsv1.TripRouteBuildingUpdated, []byte) error {
	f.calls++
	if f.fail {
		return errors.New("database unavailable")
	}
	return nil
}
func TestCommitOnlyAfterProjection(t *testing.T) {
	e := &eventsv1.TripRouteBuildingUpdated{EventId: uuid.NewString(), RequestId: uuid.NewString(), SchemaVersion: 1, PlannerId: "graph", Revision: 1, Stage: 1, OccurredAt: timestamppb.Now()}
	b, _ := proto.Marshal(e)
	for _, tc := range []struct {
		name                        string
		invalid, dbFail, commitFail bool
		wantApply, wantCommit       int
	}{
		{name: "success", wantApply: 1, wantCommit: 1}, {name: "bad event", invalid: true}, {name: "database failure", dbFail: true, wantApply: 1}, {name: "commit failure", commitFail: true, wantApply: 1, wantCommit: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &fakeReader{m: kafka.Message{Key: []byte(e.RequestId), Value: b, Headers: []kafka.Header{{Key: "event_type", Value: []byte(contract.EventType)}}}, failCommit: tc.commitFail}
			if tc.invalid {
				r.m.Key = []byte("wrong")
			}
			s := &fakeStore{fail: tc.dbFail}
			_ = (Consumer{Reader: r, Repository: s, Timeout: time.Second}).Run(context.Background())
			if s.calls != tc.wantApply || r.commits != tc.wantCommit {
				t.Fatal("unsafe acknowledgement", s.calls, r.commits)
			}
		})
	}
}
