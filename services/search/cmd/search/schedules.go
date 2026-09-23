package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/NotaKronGit/travel-watch/api/transport"
	eventsv1 "github.com/NotaKronGit/travel-watch/gen/travelwatch/events/v1"
	v1 "github.com/NotaKronGit/travel-watch/gen/travelwatch/search/v1"
	"github.com/NotaKronGit/travel-watch/services/search/internal/config"
	"github.com/NotaKronGit/travel-watch/services/search/internal/realroutes"
	"github.com/NotaKronGit/travel-watch/services/search/internal/schedules"
	"github.com/NotaKronGit/travel-watch/services/search/internal/storage"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type guardedProvider struct {
	provider realroutes.Provider
	store    *storage.Store
	job      schedules.Job
	timeout  time.Duration
	stop     context.CancelFunc
}

func (p guardedProvider) Call(ctx context.Context, q transport.Request) (transport.Response, error) {
	op, cancel := context.WithTimeout(ctx, p.timeout)
	active, err := p.store.SchedulesActive(op, p.job)
	cancel()
	if err != nil || !active {
		p.stop()
		return transport.Response{}, errors.New("schedule job no longer active")
	}
	return p.provider.Call(ctx, q)
}
func checkSchedules(ctx context.Context, db *sql.DB, c config.Config) error {
	store := storage.New(db)
	for ctx.Err() == nil {
		op, cancel := context.WithTimeout(ctx, c.Progress.Timeout)
		job, found, err := store.ClaimSchedules(op, c.Schedules.Timeout+3*c.Progress.Timeout, c.Schedules.RecheckInterval)
		cancel()
		if err != nil {
			return err
		}
		if !found {
			timer := time.NewTimer(c.Progress.PollInterval)
			select {
			case <-ctx.Done():
				timer.Stop()
			case <-timer.C:
			}
			continue
		}
		result := runScheduleCheck(ctx, store, job, c)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		op, cancel = context.WithTimeout(ctx, c.Progress.Timeout)
		err = store.FinishSchedules(op, job, result, c.Schedules.RecheckInterval)
		cancel()
		if err != nil {
			return err
		}
	}
	return ctx.Err()
}
func runScheduleCheck(ctx context.Context, store *storage.Store, job schedules.Job, c config.Config) *v1.ScheduleCheck {
	work, stop := context.WithTimeout(ctx, c.Schedules.Timeout)
	defer stop()
	failure := func() *v1.ScheduleCheck {
		return &v1.ScheduleCheck{State: "failed", Incomplete: true, CheckedAt: timestamppb.Now(), Warnings: []string{"Не удалось проверить расписания. Сохранённые схемы не удалены; ошибка не означает отсутствия рейсов."}}
	}
	var graph realroutes.Result
	event := new(eventsv1.TripRequestCreated)
	if json.Unmarshal(job.Graph, &graph) != nil || proto.Unmarshal(job.Payload, event) != nil || event.Origin == nil {
		return failure()
	}
	graph.Query.DepartureFrom = event.DepartureFrom
	graph.Query.DepartureTo = event.DepartureTo
	provider, err := realroutes.Start(work, c.Planner.CollectorBinary)
	if err != nil {
		return failure()
	}
	defer provider.Close()
	guarded := guardedProvider{provider, store, job, c.Progress.Timeout, stop}
	result, err := (schedules.Checker{Provider: guarded, Config: c.Schedules}).Check(work, graph, event.Origin.Timezone)
	if err != nil {
		if result == nil {
			return failure()
		}
		if len(result.Schemes) > 5 {
			result.Schemes = result.Schemes[:5]
		}
		result.State = "failed"
		result.Incomplete = true
		result.CheckedAt = timestamppb.Now()
		result.Warnings = append(result.Warnings, "Проверка прервана или исходные данные неполны. Частичные результаты не подтверждают весь маршрут.")
	}
	return result
}
