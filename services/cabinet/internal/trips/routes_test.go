package trips

import (
	"connectrpc.com/connect"
	"context"
	"encoding/base64"
	"errors"
	v1 "github.com/NotaKronGit/travel-watch/gen/travelwatch/cabinet/v1"
	searchv1 "github.com/NotaKronGit/travel-watch/gen/travelwatch/search/v1"
	"github.com/NotaKronGit/travel-watch/services/cabinet/internal/auth"
	"github.com/NotaKronGit/travel-watch/services/cabinet/internal/config"
	"github.com/NotaKronGit/travel-watch/services/cabinet/internal/storage"
	"testing"
	"time"
)

type routesClient struct {
	calls int
	err   error
}

func (c *routesClient) GetRoutes(_ context.Context, r *connect.Request[searchv1.GetRoutesRequest]) (*connect.Response[searchv1.GetRoutesResponse], error) {
	c.calls++
	return connect.NewResponse(&searchv1.GetRoutesResponse{Revision: 9}), c.err
}
func TestRouteOwnershipBeforeSearch(t *testing.T) {
	for _, tc := range []struct {
		name                string
		noSession           bool
		storeErr, searchErr error
		offset              int32
		want                connect.Code
		calls               int
	}{
		{name: "owner", calls: 1},
		{name: "missing session", noSession: true, want: connect.CodeUnauthenticated},
		{name: "foreign request", storeErr: storage.ErrNotFound, want: connect.CodeNotFound},
		{name: "database unavailable", storeErr: errors.New("private database error"), want: connect.CodeUnavailable},
		{name: "invalid page", offset: -1, want: connect.CodeInvalidArgument},
		{name: "search unavailable", searchErr: errors.New("private transport error"), want: connect.CodeUnavailable, calls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &readTripRepository{err: tc.storeErr}
			client := &routesClient{err: tc.searchErr}
			s := &Service{store: store, auth: auth.NewService(&readAuthRepository{}, config.Auth{SessionTTL: time.Hour}), routes: client}
			req := connect.NewRequest(&v1.GetTripRoutesRequest{Id: "11111111-1111-4111-8111-111111111111", PageSize: 5, Offset: tc.offset})
			if !tc.noSession {
				req.Header().Set("Cookie", "tw_session="+base64.RawURLEncoding.EncodeToString(make([]byte, 32)))
			}
			res, err := s.GetTripRoutes(context.Background(), req)
			if tc.want == 0 {
				if err != nil || res.Msg.Result.Revision != 9 {
					t.Fatal(err)
				}
			} else if connect.CodeOf(err) != tc.want {
				t.Fatal(err)
			}
			if client.calls != tc.calls {
				t.Fatal("Search called without ownership or validation", client.calls)
			}
			if store.calls > 0 && store.owner != "session-owner" {
				t.Fatal("wrong owner")
			}
		})
	}
}
