package trips

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	v1 "github.com/NotaKronGit/travel-watch/gen/travelwatch/cabinet/v1"
	"github.com/NotaKronGit/travel-watch/services/cabinet/internal/auth"
	"github.com/NotaKronGit/travel-watch/services/cabinet/internal/config"
	"github.com/NotaKronGit/travel-watch/services/cabinet/internal/storage"
)

type lifecycleRepository struct {
	Repository
	calls          int
	owner, comment string
	err            error
}

func (r *lifecycleRepository) CancelTrip(_ context.Context, owner, _ string) error {
	r.calls++
	r.owner = owner
	return r.err
}
func (r *lifecycleRepository) UpdateTripComment(_ context.Context, owner, _, comment string) error {
	r.calls++
	r.owner = owner
	r.comment = comment
	return r.err
}
func TestLifecycleRPC(t *testing.T) {
	for _, method := range []string{"cancel", "comment"} {
		for _, tc := range []struct {
			name        string
			noSession   bool
			id, comment string
			storeErr    error
			code        connect.Code
		}{
			{name: "success", comment: "Личная заметка"},
			{name: "no session", noSession: true, code: connect.CodeUnauthenticated},
			{name: "bad id", id: "invalid", code: connect.CodeNotFound},
			{name: "foreign or absent", storeErr: storage.ErrNotFound, code: connect.CodeNotFound},
			{name: "storage error", storeErr: errors.New("secret-db-error"), code: connect.CodeUnavailable},
		} {
			t.Run(method+tc.name, func(t *testing.T) {
				repo := &lifecycleRepository{err: tc.storeErr}
				s := &Service{store: repo, auth: auth.NewService(&readAuthRepository{}, config.Auth{SessionTTL: time.Hour})}
				id := tc.id
				if id == "" {
					id = "11111111-1111-1111-1111-111111111111"
				}
				cookie := ""
				if !tc.noSession {
					cookie = "tw_session=" + base64.RawURLEncoding.EncodeToString(make([]byte, 32))
				}
				var err error
				if method == "cancel" {
					req := connect.NewRequest(&v1.CancelTripRequest{Id: id})
					req.Header().Set("Cookie", cookie)
					_, err = s.CancelTrip(context.Background(), req)
				} else {
					req := connect.NewRequest(&v1.UpdateTripCommentRequest{Id: id, Comment: tc.comment})
					req.Header().Set("Cookie", cookie)
					_, err = s.UpdateTripComment(context.Background(), req)
				}
				if tc.code == 0 {
					if err != nil {
						t.Fatal(err)
					}
				} else if connect.CodeOf(err) != tc.code {
					t.Fatal(err)
				}
				if err != nil && strings.Contains(err.Error(), "secret-db-error") {
					t.Fatal("private error leaked")
				}
				if tc.noSession || tc.id != "" {
					if repo.calls != 0 {
						t.Fatal("invalid request reached storage")
					}
				} else if repo.calls != 1 || repo.owner != "session-owner" {
					t.Fatal("owner not from session")
				}
			})
		}
	}
	for _, text := range []string{strings.Repeat("я", 2000), strings.Repeat("я", 2001), "bad\x00text"} {
		repo := &lifecycleRepository{}
		s := &Service{store: repo, auth: auth.NewService(&readAuthRepository{}, config.Auth{SessionTTL: time.Hour})}
		req := connect.NewRequest(&v1.UpdateTripCommentRequest{Id: "11111111-1111-1111-1111-111111111111", Comment: text})
		req.Header().Set("Cookie", "tw_session="+base64.RawURLEncoding.EncodeToString(make([]byte, 32)))
		_, err := s.UpdateTripComment(context.Background(), req)
		valid := text == strings.Repeat("я", 2000)
		if valid && (err != nil || repo.comment != text) {
			t.Fatal("unicode limit", err)
		}
		if !valid && (connect.CodeOf(err) != connect.CodeInvalidArgument || repo.calls != 0) {
			t.Fatal("invalid comment accepted", err)
		}
	}
}
