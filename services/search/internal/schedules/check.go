package schedules

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/NotaKronGit/travel-watch/api/transport"
	v1 "github.com/NotaKronGit/travel-watch/gen/travelwatch/search/v1"
	"github.com/NotaKronGit/travel-watch/services/search/internal/realroutes"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Checker struct {
	Provider realroutes.Provider
	Config   Config
	impl     matcher // overrides Config.Matcher; set only from within this package (tests/benchmarks).
}

func (c Checker) matcherImpl() matcher {
	if c.impl != nil {
		return c.impl
	}
	if c.Config.Matcher == "brute" {
		return bruteMatcher{}
	}
	return windowedMatcher{}
}
type timetableDeparture transport.Departure
type observed struct {
	timetableDeparture
	at time.Time
}
type lookup struct {
	departures []observed
	incomplete bool
}

// Check never invents transfer durations or interprets a provider error as an empty schedule.
// Reuse is scoped to this calculation; it is not a cross-request cache.
func (c Checker) Check(ctx context.Context, input realroutes.Result, timezone string) (*v1.ScheduleCheck, error) {
	out := &v1.ScheduleCheck{State: "done", Incomplete: !input.Complete, Warnings: []string{"Расписания не подтверждают наличие билетов. Билеты отдельных участков не образуют защищённую стыковку.", "Проверяются структурированные схемы нашего алгоритма. Текстовые предложения Gemini пока не проверяются."}}
	if err := c.Config.Validate(); err != nil {
		return out, err
	}
	if c.Provider == nil {
		return out, errors.New("schedule provider required")
	}
	loc, err := time.LoadLocation(timezone)
	if err != nil || timezone == "" {
		return out, errors.New("origin timezone required")
	}
	from, err := time.ParseInLocation(time.DateOnly, input.Query.DepartureFrom, loc)
	if err != nil {
		return out, err
	}
	to, err := time.ParseInLocation(time.DateOnly, input.Query.DepartureTo, loc)
	if err != nil || to.Before(from) {
		return out, errors.New("invalid departure interval")
	}
	if len(input.Candidates) == 0 {
		return out, errors.New("no saved schemes to check")
	}
	if len(input.Candidates) > 50 {
		return out, errors.New("too many saved schemes")
	}
	end := to.AddDate(0, 0, 1)
	sampledEnd := from.AddDate(0, 0, c.Config.MaxDates)
	if end.After(sampledEnd) {
		end = sampledEnd
		out.Incomplete = true
		out.Warnings = append(out.Warnings, "Проверена только начальная часть диапазона дат: достигнут лимит дней.")
	}
	memo := map[[4]string]lookup{}
	fetch := func(step realroutes.Step) lookup {
		mode := step.Mode
		if mode == "flight" {
			mode = "plane"
		}
		key := [4]string{step.FromCode, step.ToCode, mode, ""}
		if value, ok := memo[key]; ok {
			return value
		}
		value := lookup{}
		system, src, dst := "yandex", step.FromCode, step.ToCode
		if strings.HasPrefix(src, "iata:") && strings.HasPrefix(dst, "iata:") {
			system = "iata"
			src = strings.TrimPrefix(src, "iata:")
			dst = strings.TrimPrefix(dst, "iata:")
		}
		if src == "" || dst == "" || (mode != "train" && mode != "plane") || (system == "yandex" && (!strings.HasPrefix(src, "s") || !strings.HasPrefix(dst, "s"))) {
			value.incomplete = true
			memo[key] = value
			return value
		}
		// Calendar-date queries use station timezones. Include a day on either side
		// of the origin interval, plus later departures within the journey horizon.
		last := end.AddDate(0, 0, int(c.Config.MaxJourney/(24*time.Hour))+1)
		for day := from.AddDate(0, 0, -1); day.Before(last); day = day.AddDate(0, 0, 1) {
			if ctx.Err() != nil || int(out.Requests) >= c.Config.MaxRequests {
				value.incomplete = true
				break
			}
			out.Requests++
			response, e := c.Provider.Call(ctx, transport.Request{Version: 1, Method: "departures", From: src, To: dst, System: system, Mode: mode, Date: day.Format(time.DateOnly)})
			if e != nil || response.Error != "" || response.Version != 1 || response.ObservedAt.IsZero() || len(response.Departures) > 100 || response.Count != len(response.Departures) || response.Total < response.Count {
				value.incomplete = true
				continue
			}
			if response.Incomplete || response.Total > response.Count {
				value.incomplete = true
			}
			for _, d := range response.Departures {
				matches := d.From.Code == src && d.To.Code == dst
				if system == "iata" {
					matches = d.From.IATA == src && d.To.IATA == dst
				}
				if !matches || d.Mode != mode || !d.Arrival.After(d.Departure) || d.Number == "" || len(d.Number) > 100 || len(d.From.Title) > 300 || len(d.To.Title) > 300 || d.Departure.IsZero() {
					value.incomplete = true
					continue
				}
				value.departures = append(value.departures, observed{timetableDeparture(d), response.ObservedAt})
			}
		}
		sort.Slice(value.departures, func(i, j int) bool {
			a, b := value.departures[i], value.departures[j]
			if a.Departure.Equal(b.Departure) {
				return a.Number < b.Number
			}
			return a.Departure.Before(b.Departure)
		})
		unique := value.departures[:0]
		seen := map[string]bool{}
		for _, d := range value.departures {
			k := d.Number + "|" + d.Departure.UTC().Format(time.RFC3339) + "|" + d.Arrival.UTC().Format(time.RFC3339)
			if !seen[k] {
				seen[k] = true
				unique = append(unique, d)
			}
		}
		value.departures = unique
		memo[key] = value
		return value
	}
	groupNumbers := map[string]int32{}
	accessCounts := map[int32]int32{}
	var nextNumber int32
	for _, candidate := range input.Candidates {
		key := realroutes.RailAccessKey(candidate)
		schemeNumber := groupNumbers[key]
		if key == "" || schemeNumber == 0 {
			nextNumber++
			schemeNumber = nextNumber
			if key != "" {
				groupNumbers[key] = schemeNumber
			}
		}
		variants := [][]realroutes.Step{candidate.Steps}
		if len(candidate.RailAccessVariants) > 0 && len(candidate.Steps) >= 3 {
			if len(candidate.RailAccessVariants) > 20 {
				return out, errors.New("too many access variants")
			}
			variants = nil
			for _, prefix := range candidate.RailAccessVariants {
				if len(prefix) != 3 {
					return out, errors.New("invalid access prefix")
				}
				steps := append([]realroutes.Step(nil), prefix...)
				variants = append(variants, append(steps, candidate.Steps[3:]...))
			}
		}
		for _, steps := range variants {
			if ctx.Err() != nil {
				return out, ctx.Err()
			}
			if len(out.Schemes) >= 100 {
				out.Incomplete = true
				out.Warnings = append(out.Warnings, "Достигнут лимит проверяемых вариантов подвоза.")
				break
			}
			accessCounts[schemeNumber]++
			row := &v1.ScheduledScheme{SchemeNumber: schemeNumber, AccessVariant: accessCounts[schemeNumber]}
			out.Schemes = append(out.Schemes, row)
			if len(steps) == 0 || len(steps) > 20 {
				row.State = "unverified"
				row.Warnings = append(row.Warnings, "Некорректная сохранённая схема.")
				out.Incomplete = true
				continue
			}
			var main []int
			var available [][]observed
			incomplete := false
			unknown := false
			transfers := make(map[int]time.Duration)
			for i, s := range steps {
				if i > 0 && steps[i-1].To != s.From {
					incomplete = true
					unknown = true
					row.Warnings = append(row.Warnings, "Не удалось сопоставить узлы соседних участков.")
				}
				if s.Mode == "transfer" {
					found := false
					for _, t := range c.Config.Transfers {
						if t.From == s.From && t.To == s.To {
							transfers[i] = t.Duration
							found = true
							row.Warnings = append(row.Warnings, fmt.Sprintf("Переезд %s → %s: %s, источник оценки: %s", s.From, s.To, t.Duration, t.Source))
							break
						}
					}
					if !found {
						unknown = true
						row.Warnings = append(row.Warnings, "Неизвестно время переезда: "+s.From+" → "+s.To)
					}
					continue
				}
				if len(main) > 0 && main[len(main)-1] == i-1 && steps[i-1].ToCode != s.FromCode {
					unknown = true
					row.Warnings = append(row.Warnings, "Не подтверждено совпадение физических узлов пересадки.")
				}
				main = append(main, i)
				data := fetch(s)
				available = append(available, data.departures)
				incomplete = incomplete || data.incomplete
			}
			if len(main) == 0 || len(main) > 8 {
				row.State = "unverified"
				row.Warnings = append(row.Warnings, "Нет поддерживаемой цепочки транспортных участков.")
				out.Incomplete = true
				continue
			}
			journeys, truncated := c.matcherImpl().Match(ctx, matchInput{Steps: steps, Main: main, Transfers: transfers, Available: available, Unknown: unknown, From: from, End: end, Config: c.Config})
			row.Journeys = journeys
			row.State = "no_match"
			if incomplete || unknown || truncated {
				row.State = "unverified"
				out.Incomplete = true
			}
			if len(row.Journeys) > 0 && !unknown {
				row.State = "compatible"
			}
			if incomplete {
				row.Warnings = append(row.Warnings, "Не удалось полностью проверить расписания: ошибка источника, неполная выдача, неподдерживаемые коды или лимит запросов.")
			}
			if truncated {
				row.Warnings = append(row.Warnings, "Перебор сочетаний ограничен; показаны не все варианты.")
			}
			if unknown {
				row.Warnings = append(row.Warnings, "Показанные сочетания предварительные: время неизвестных переездов не учтено, дата выезда из города требует уточнения.")
			}
		}
	}
	if int(out.Requests) >= c.Config.MaxRequests {
		out.Warnings = append(out.Warnings, "Достигнут бюджет запросов к расписаниям.")
		out.Incomplete = true
	}
	out.CheckedAt = timestamppb.Now()
	// Keep the saved report below the storage/RPC limits, including JSON overhead.
	for {
		data, err := protojson.Marshal(out)
		if err != nil {
			return out, err
		}
		if len(data) <= 900000 {
			break
		}
		out.Incomplete = true
		if len(out.Schemes) == 0 {
			return out, errors.New("schedule report too large")
		}
		out.Schemes = out.Schemes[:len(out.Schemes)-1]
		if len(out.Warnings) == 0 || out.Warnings[len(out.Warnings)-1] != "Достигнут лимит размера отчёта; показана часть схем." {
			out.Warnings = append(out.Warnings, "Достигнут лимит размера отчёта; показана часть схем.")
		}
	}
	return out, nil
}
