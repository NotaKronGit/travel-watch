// Package results exposes saved route schemes over authenticated internal gRPC.
package results

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"connectrpc.com/connect"
	v1 "github.com/NotaKronGit/travel-watch/gen/travelwatch/search/v1"
	"github.com/NotaKronGit/travel-watch/gen/travelwatch/search/v1/searchv1connect"
	"github.com/google/uuid"
)

type Repository interface {
	ReadRoutes(context.Context, *v1.GetRoutesRequest) (*v1.GetRoutesResponse, error)
}
type Service struct {
	Store   Repository
	Timeout time.Duration
}

func Validate(q *v1.GetRoutesRequest) error {
	id, err := uuid.Parse(q.RequestId)
	if err != nil || id == uuid.Nil || id.String() != q.RequestId || (q.PlannerId != "" && q.PlannerId != "graph" && q.PlannerId != "gemini") || q.Offset < 0 || q.Offset > 10000 || q.PageSize < 1 || q.PageSize > 10 || (q.PlannerId == "" && q.Offset != 0) {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("invalid route page"))
	}
	return nil
}
func (s Service) GetRoutes(ctx context.Context, r *connect.Request[v1.GetRoutesRequest]) (*connect.Response[v1.GetRoutesResponse], error) {
	if err := Validate(r.Msg); err != nil {
		return nil, err
	}
	op, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()
	result, err := s.Store.ReadRoutes(op, r.Msg)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("saved routes unavailable"))
	}
	return connect.NewResponse(result), nil
}
func Handler(store Repository, timeout time.Duration) http.Handler {
	mux := http.NewServeMux()
	path, h := searchv1connect.NewRouteResultsServiceHandler(Service{store, timeout}, connect.WithReadMaxBytes(8192), connect.WithSendMaxBytes(2<<20))
	mux.Handle(path, h)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil || len(r.TLS.VerifiedChains) == 0 {
			http.Error(w, "mTLS required", http.StatusUnauthorized)
			return
		}
		if r.ProtoMajor != 2 || !strings.HasPrefix(r.Header.Get("Content-Type"), "application/grpc") || strings.HasPrefix(r.Header.Get("Content-Type"), "application/grpc-web") {
			http.Error(w, "gRPC required", http.StatusUnsupportedMediaType)
			return
		}
		mux.ServeHTTP(w, r)
	})
}
