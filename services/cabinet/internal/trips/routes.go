package trips

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	v1 "github.com/NotaKronGit/travel-watch/gen/travelwatch/cabinet/v1"
	searchv1 "github.com/NotaKronGit/travel-watch/gen/travelwatch/search/v1"
	"github.com/NotaKronGit/travel-watch/services/cabinet/internal/storage"
)

func (s *Service) GetTripRoutes(ctx context.Context, req *connect.Request[v1.GetTripRoutesRequest]) (*connect.Response[v1.GetTripRoutesResponse], error) {
	user, err := s.user(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	if !uuid.MatchString(req.Msg.Id) || (req.Msg.PlannerId != "" && req.Msg.PlannerId != "graph" && req.Msg.PlannerId != "gemini") || req.Msg.Offset < 0 || req.Msg.Offset > 10000 || req.Msg.PageSize < 1 || req.Msg.PageSize > 10 || (req.Msg.PlannerId == "" && req.Msg.Offset != 0) {
		return nil, invalid("Некорректные параметры страницы маршрутов")
	}
	// Ownership is checked before ANY call to Search, including for cancelled trips.
	if _, err = s.store.GetTrip(ctx, user, req.Msg.Id); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("Заявка не найдена"))
		}
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("Не удалось проверить заявку"))
	}
	if s.routes == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("Просмотр маршрутов пока не настроен"))
	}
	result, err := s.routes.GetRoutes(ctx, connect.NewRequest(&searchv1.GetRoutesRequest{RequestId: req.Msg.Id, PlannerId: req.Msg.PlannerId, Offset: req.Msg.Offset, PageSize: req.Msg.PageSize}))
	if err != nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("Не удалось загрузить маршруты. Повторите попытку"))
	}
	return connect.NewResponse(&v1.GetTripRoutesResponse{Result: result.Msg}), nil
}
