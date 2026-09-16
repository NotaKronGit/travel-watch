package trips

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	v1 "github.com/NotaKronGit/travel-watch/gen/travelwatch/cabinet/v1"
	"github.com/NotaKronGit/travel-watch/gen/travelwatch/cabinet/v1/cabinetv1connect"
	"github.com/NotaKronGit/travel-watch/services/cabinet/internal/auth"
	"github.com/NotaKronGit/travel-watch/services/cabinet/internal/config"
	"github.com/NotaKronGit/travel-watch/services/cabinet/internal/storage"
)

// Unused embedded methods intentionally fail if a read RPC calls a write operation.
type readAuthRepository struct {
	auth.Repository
	err error
}

func (r *readAuthRepository) FindUserBySession(context.Context, []byte) (storage.User, error) {
	return storage.User{ID: "session-owner", Email: "rpc-test@example.com", CreatedAt: time.Unix(1, 0)}, r.err
}

type readTripRepository struct {
	Repository
	calls     int
	owner, id string
	offset    uint
	err       error
}

func (r *readTripRepository) GetTrip(_ context.Context, owner, id string) (storage.TripDetails, error) {
	r.calls++
	r.owner = owner
	r.id = id
	return storage.TripDetails{ID: id, Adults: 2, CreatedAt: time.Unix(1, 0)}, r.err
}
func (r *readTripRepository) ListTrips(_ context.Context, owner string, offset uint, _ bool) ([]storage.TripDetails, bool, error) {
	r.calls++
	r.owner = owner
	r.offset = offset
	return []storage.TripDetails{{ID: "test-trip", Adults: 2, CreatedAt: time.Unix(1, 0)}}, true, r.err
}

func TestReadRPC(t *testing.T) {
	const id = "11111111-1111-1111-1111-111111111111"
	failure := errors.New("private storage implementation detail")
	for _, tc := range []struct {
		name, method      string
		noCookie          bool
		authErr, storeErr error
		invalidID         bool
		offset            int32
		wantCode          connect.Code
		wantCalls         int
	}{
		{name: "get without session", method: "get", noCookie: true, wantCode: connect.CodeUnauthenticated},
		{name: "list without session", method: "list", noCookie: true, wantCode: connect.CodeUnauthenticated},
		{name: "get expired session", method: "get", authErr: storage.ErrNotFound, wantCode: connect.CodeUnauthenticated},
		{name: "list expired session", method: "list", authErr: storage.ErrNotFound, wantCode: connect.CodeUnauthenticated},
		{name: "get invalid ID", method: "get", invalidID: true, wantCode: connect.CodeNotFound},
		{name: "list negative offset", method: "list", offset: -1, wantCode: connect.CodeInvalidArgument},
		{name: "get missing or foreign", method: "get", storeErr: storage.ErrNotFound, wantCode: connect.CodeNotFound, wantCalls: 1},
		{name: "get storage error", method: "get", storeErr: failure, wantCode: connect.CodeUnavailable, wantCalls: 1},
		{name: "list storage error", method: "list", storeErr: failure, wantCode: connect.CodeUnavailable, wantCalls: 1},
		{name: "get success", method: "get", wantCalls: 1},
		{name: "list success", method: "list", offset: 20, wantCalls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &readTripRepository{err: tc.storeErr}
			a := auth.NewService(&readAuthRepository{err: tc.authErr}, config.Auth{SessionTTL: time.Hour})
			_, handler := Handler(repo)(a)
			server := httptest.NewServer(handler)
			defer server.Close()
			client := cabinetv1connect.NewTripServiceClient(server.Client(), server.URL)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			cookie := "tw_session=" + base64.RawURLEncoding.EncodeToString(make([]byte, 32))
			var err error
			if tc.method == "get" {
				requestedID := id
				if tc.invalidID {
					requestedID = "not-a-uuid"
				}
				req := connect.NewRequest(&v1.GetTripRequest{Id: requestedID})
				if !tc.noCookie {
					req.Header().Set("Cookie", cookie)
				}
				var res *connect.Response[v1.GetTripResponse]
				res, err = client.GetTrip(ctx, req)
				if tc.wantCode == 0 && err == nil && (res.Msg.Trip == nil || res.Msg.Trip.Id != id || res.Msg.Trip.Adults != 2) {
					t.Fatal("incorrect trip response")
				}
			} else {
				req := connect.NewRequest(&v1.ListTripsRequest{Offset: tc.offset})
				if !tc.noCookie {
					req.Header().Set("Cookie", cookie)
				}
				var res *connect.Response[v1.ListTripsResponse]
				res, err = client.ListTrips(ctx, req)
				if tc.wantCode == 0 && err == nil && (len(res.Msg.Trips) != 1 || !res.Msg.HasMore) {
					t.Fatal("incorrect list response")
				}
			}
			if tc.wantCode == 0 {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || connect.CodeOf(err) != tc.wantCode {
				t.Fatalf("error=%v, want code %v", err, tc.wantCode)
			}
			if err != nil && strings.Contains(err.Error(), failure.Error()) {
				t.Fatal("storage error leaked")
			}
			if repo.calls != tc.wantCalls {
				t.Fatalf("storage calls=%d, want %d", repo.calls, tc.wantCalls)
			}
			if repo.calls > 0 {
				if repo.owner != "session-owner" {
					t.Fatal("owner was not taken from session")
				}
				if tc.method == "get" && repo.id != id {
					t.Fatal("incorrect ID passed to storage")
				}
				expectedOffset := tc.offset
				if expectedOffset < 0 {
					t.Fatal("negative offset reached storage")
				}
				if tc.method == "list" && repo.offset != uint(expectedOffset) {
					t.Fatal("incorrect offset passed to storage")
				}
			}
		})
	}
}
