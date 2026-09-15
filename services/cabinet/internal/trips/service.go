package trips

import (
	"context"
	"errors"
	"google.golang.org/protobuf/types/known/timestamppb"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"connectrpc.com/connect"
	v1 "github.com/NotaKronGit/travel-watch/gen/travelwatch/cabinet/v1"
	"github.com/NotaKronGit/travel-watch/gen/travelwatch/cabinet/v1/cabinetv1connect"
	"github.com/NotaKronGit/travel-watch/services/cabinet/internal/auth"
	"github.com/NotaKronGit/travel-watch/services/cabinet/internal/storage"
)

type Repository interface {
	GetTrip(context.Context, string, string) (storage.TripDetails, error)
	ListTrips(context.Context, string, uint) ([]storage.TripDetails, bool, error)
	SearchCities(context.Context, string) ([]storage.CityOption, error)
	CreateTrip(context.Context, storage.Trip) (string, error)
}
type Service struct {
	store Repository
	auth  *auth.Service
}

func Handler(store Repository) func(*auth.Service) (string, http.Handler) {
	return func(a *auth.Service) (string, http.Handler) {
		return cabinetv1connect.NewTripServiceHandler(&Service{store: store, auth: a}, connect.WithReadMaxBytes(8192))
	}
}
func (s *Service) user(ctx context.Context, h http.Header) (string, error) {
	req := connect.NewRequest(&v1.GetCurrentUserRequest{})
	req.Header().Set("Cookie", h.Get("Cookie"))
	res, err := s.auth.GetCurrentUser(ctx, req)
	if err != nil {
		return "", err
	}
	return res.Msg.User.Id, nil
}
func invalid(message string) error {
	return connect.NewError(connect.CodeInvalidArgument, errors.New(message))
}
func (s *Service) SearchCities(ctx context.Context, req *connect.Request[v1.SearchCitiesRequest]) (*connect.Response[v1.SearchCitiesResponse], error) {
	if _, err := s.user(ctx, req.Header()); err != nil {
		return nil, err
	}
	query := strings.TrimSpace(req.Msg.Query)
	if utf8.RuneCountInString(query) < 2 || utf8.RuneCountInString(query) > 100 {
		return nil, invalid("Введите от 2 до 100 символов")
	}
	cities, err := s.store.SearchCities(ctx, query)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("Не удалось загрузить города. Повторите попытку"))
	}
	res := &v1.SearchCitiesResponse{}
	for _, c := range cities {
		res.Cities = append(res.Cities, &v1.CityOption{Id: c.ID, Name: c.Name, Country: c.Country, Region: c.Region, Timezone: c.Timezone, IataCode: c.IATACode})
	}
	return connect.NewResponse(res), nil
}

var uuid = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func validate(r *v1.CreateTripRequest) error {
	if !uuid.MatchString(r.RequestId) || !uuid.MatchString(r.OriginId) || !uuid.MatchString(r.DestinationId) {
		return invalid("Выберите города из списка")
	}
	if r.OriginId == r.DestinationId {
		return invalid("Города отправления и назначения должны отличаться")
	}
	if r.Adults < 1 || r.Adults > 9 {
		return invalid("Укажите от 1 до 9 взрослых")
	}
	for _, d := range []string{r.DepartureFrom, r.DepartureTo} {
		if _, err := time.Parse(time.DateOnly, d); err != nil || len(d) != 10 {
			return invalid("Укажите корректные даты")
		}
	}
	if r.DepartureTo < r.DepartureFrom {
		return invalid("Конец диапазона не может быть раньше начала")
	}
	return nil
}
func (s *Service) CreateTrip(ctx context.Context, req *connect.Request[v1.CreateTripRequest]) (*connect.Response[v1.CreateTripResponse], error) {
	user, err := s.user(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	r := req.Msg
	if err := validate(r); err != nil {
		return nil, err
	}
	id, err := s.store.CreateTrip(ctx, storage.Trip{UserID: user, RequestID: r.RequestId, OriginID: r.OriginId, DestinationID: r.DestinationId, DepartureFrom: r.DepartureFrom, DepartureTo: r.DepartureTo, Adults: r.Adults})
	switch {
	case errors.Is(err, storage.ErrNotFound):
		return nil, invalid("Город больше недоступен. Выберите города заново")
	case errors.Is(err, storage.ErrPastDeparture):
		return nil, invalid("Дата отправления уже прошла в городе отправления")
	case errors.Is(err, storage.ErrRequestConflict):
		return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("Эта заявка уже сохранена с другими параметрами"))
	case err != nil:
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("Не удалось подтвердить сохранение. Повторите попытку"))
	}
	return connect.NewResponse(&v1.CreateTripResponse{Id: id}), nil
}

func cityView(c storage.CityOption) *v1.CityOption {
	return &v1.CityOption{Id: c.ID, Name: c.Name, Country: c.Country, Region: c.Region, Timezone: c.Timezone, IataCode: c.IATACode}
}
func tripView(t storage.TripDetails) *v1.TripDetails {
	return &v1.TripDetails{Id: t.ID, Origin: cityView(t.Origin), Destination: cityView(t.Destination), DepartureFrom: t.DepartureFrom, DepartureTo: t.DepartureTo, Adults: t.Adults, CreatedAt: timestamppb.New(t.CreatedAt)}
}
func (s *Service) GetTrip(ctx context.Context, req *connect.Request[v1.GetTripRequest]) (*connect.Response[v1.GetTripResponse], error) {
	user, err := s.user(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	if !uuid.MatchString(req.Msg.Id) {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("Заявка не найдена"))
	}
	t, err := s.store.GetTrip(ctx, user, req.Msg.Id)
	if errors.Is(err, storage.ErrNotFound) {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("Заявка не найдена"))
	}
	if err != nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("Не удалось загрузить заявку"))
	}
	return connect.NewResponse(&v1.GetTripResponse{Trip: tripView(t)}), nil
}
func (s *Service) ListTrips(ctx context.Context, req *connect.Request[v1.ListTripsRequest]) (*connect.Response[v1.ListTripsResponse], error) {
	user, err := s.user(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	if req.Msg.Offset < 0 {
		return nil, invalid("Некорректная страница")
	}
	trips, more, err := s.store.ListTrips(ctx, user, uint(req.Msg.Offset))
	if err != nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("Не удалось загрузить заявки"))
	}
	res := &v1.ListTripsResponse{HasMore: more}
	for _, t := range trips {
		res.Trips = append(res.Trips, tripView(t))
	}
	return connect.NewResponse(res), nil
}
