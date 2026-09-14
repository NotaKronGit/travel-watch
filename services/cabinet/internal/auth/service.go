package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"connectrpc.com/connect"
	cabinetv1 "github.com/NotaKronGit/travel-watch/gen/travelwatch/cabinet/v1"
	"github.com/jackc/pgx/v5/pgconn"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const sessionTTL = 7 * 24 * time.Hour

var emailPattern = regexp.MustCompile(`^[a-z0-9.!#$%&'*+/=?^_` + "`" + `{|}~-]+@[a-z0-9](?:[a-z0-9-]*[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]*[a-z0-9])?)+$`)

type Service struct {
	db         *sql.DB
	secure     bool
	cookieName string
	dummyHash  string
}

func NewService(db *sql.DB, secure bool) *Service {
	name := "tw_session"
	if secure {
		name = "__Host-tw_session"
	}
	return &Service{db: db, secure: secure, cookieName: name, dummyHash: hashPassword("dummy-password-for-equal-work")}
}
func credentials(email, password string) (string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if len(email) > 254 || !emailPattern.MatchString(email) {
		return "", connect.NewError(connect.CodeInvalidArgument, errors.New("Укажите корректный email"))
	}
	if !utf8.ValidString(password) || utf8.RuneCountInString(password) < 15 || len(password) > 1024 {
		return "", connect.NewError(connect.CodeInvalidArgument, errors.New("Пароль: минимум 15 символов, максимум 1024 байта"))
	}
	return email, nil
}
func internalError(err error) error {
	// Не включаем SQL, email, пароль или токен в логи и ответ клиенту.
	slog.Error("cabinet database operation failed", "error_type", errorKind(err))
	return connect.NewError(connect.CodeInternal, errors.New("Не удалось выполнить запрос. Попробуйте позже"))
}
func errorKind(err error) string {
	if pg, ok := errors.AsType[*pgconn.PgError](err); ok {
		return pg.Code
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	return "internal"
}
func (s *Service) cookie(token string, expires time.Time) *http.Cookie {
	return &http.Cookie{Name: s.cookieName, Value: token, Path: "/", HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteStrictMode, Expires: expires, MaxAge: int(sessionTTL.Seconds())}
}
func (s *Service) sessionHash(header http.Header) []byte {
	req := &http.Request{Header: header}
	c, err := req.Cookie(s.cookieName)
	if err != nil {
		return nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(c.Value)
	if err != nil || len(raw) != 32 {
		return nil
	}
	hash := sha256.Sum256(raw)
	return hash[:]
}
func (s *Service) issueSession(ctx context.Context, tx *sql.Tx, userID string, header http.Header) (*http.Cookie, error) {
	// При повторном входе заменяем только текущую сессию браузера.
	if old := s.sessionHash(header); old != nil {
		if _, err := tx.ExecContext(ctx, "DELETE FROM sessions WHERE token_hash=$1", old); err != nil {
			return nil, err
		}
	}
	raw := make([]byte, 32)
	_, _ = rand.Read(raw)
	hash := sha256.Sum256(raw)
	expires := time.Now().UTC().Add(sessionTTL)
	_, err := tx.ExecContext(ctx, "INSERT INTO sessions(token_hash,user_id,expires_at) VALUES($1,$2,$3)", hash[:], userID, expires)
	if err != nil {
		return nil, err
	}
	return s.cookie(base64.RawURLEncoding.EncodeToString(raw), expires), nil
}
func (s *Service) Register(ctx context.Context, req *connect.Request[cabinetv1.RegisterRequest]) (*connect.Response[cabinetv1.RegisterResponse], error) {
	email, err := credentials(req.Msg.Email, req.Msg.Password)
	if err != nil {
		return nil, err
	}
	hash := hashPassword(req.Msg.Password)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, internalError(err)
	}
	defer tx.Rollback()
	var id string
	var created time.Time
	err = tx.QueryRowContext(ctx, "INSERT INTO users(email,password_hash) VALUES($1,$2) RETURNING id,created_at", email, hash).Scan(&id, &created)
	if err != nil {
		if pg, ok := errors.AsType[*pgconn.PgError](err); ok && pg.Code == "23505" {
			return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("Не удалось зарегистрировать аккаунт с этим email"))
		}
		return nil, internalError(err)
	}
	cookie, err := s.issueSession(ctx, tx, id, req.Header())
	if err != nil {
		return nil, internalError(err)
	}
	if err = tx.Commit(); err != nil {
		return nil, internalError(err)
	}
	res := connect.NewResponse(&cabinetv1.RegisterResponse{User: &cabinetv1.User{Id: id, Email: email, CreatedAt: timestamppb.New(created)}})
	res.Header().Add("Set-Cookie", cookie.String())
	return res, nil
}
func (s *Service) Login(ctx context.Context, req *connect.Request[cabinetv1.LoginRequest]) (*connect.Response[cabinetv1.LoginResponse], error) {
	email, err := credentials(req.Msg.Email, req.Msg.Password)
	if err != nil {
		return nil, err
	}
	var id, hash string
	var created time.Time
	err = s.db.QueryRowContext(ctx, "SELECT id,password_hash,created_at FROM users WHERE email=$1", email).Scan(&id, &hash, &created)
	missing := errors.Is(err, sql.ErrNoRows)
	if err != nil && !missing {
		return nil, internalError(err)
	}
	if missing {
		hash = s.dummyHash
	}
	valid, err := verifyPassword(hash, req.Msg.Password)
	if err != nil {
		return nil, internalError(err)
	}
	if missing || !valid {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("Неверный email или пароль"))
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, internalError(err)
	}
	defer tx.Rollback()
	cookie, err := s.issueSession(ctx, tx, id, req.Header())
	if err != nil {
		return nil, internalError(err)
	}
	if err = tx.Commit(); err != nil {
		return nil, internalError(err)
	}
	res := connect.NewResponse(&cabinetv1.LoginResponse{User: &cabinetv1.User{Id: id, Email: email, CreatedAt: timestamppb.New(created)}})
	res.Header().Add("Set-Cookie", cookie.String())
	return res, nil
}
func (s *Service) Logout(ctx context.Context, req *connect.Request[cabinetv1.LogoutRequest]) (*connect.Response[cabinetv1.LogoutResponse], error) {
	if hash := s.sessionHash(req.Header()); hash != nil {
		if _, err := s.db.ExecContext(ctx, "DELETE FROM sessions WHERE token_hash=$1", hash); err != nil {
			return nil, internalError(err)
		}
	}
	res := connect.NewResponse(&cabinetv1.LogoutResponse{})
	cookie := s.cookie("", time.Unix(1, 0))
	cookie.MaxAge = -1
	res.Header().Add("Set-Cookie", cookie.String())
	return res, nil
}
func (s *Service) GetCurrentUser(ctx context.Context, req *connect.Request[cabinetv1.GetCurrentUserRequest]) (*connect.Response[cabinetv1.GetCurrentUserResponse], error) {
	hash := s.sessionHash(req.Header())
	unauth := connect.NewError(connect.CodeUnauthenticated, errors.New("Войдите в аккаунт"))
	if hash == nil {
		return nil, unauth
	}
	var id, email string
	var created time.Time
	err := s.db.QueryRowContext(ctx, `SELECT u.id,u.email,u.created_at FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=$1 AND s.expires_at>now()`, hash).Scan(&id, &email, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, unauth
	}
	if err != nil {
		return nil, internalError(err)
	}
	return connect.NewResponse(&cabinetv1.GetCurrentUserResponse{User: &cabinetv1.User{Id: id, Email: email, CreatedAt: timestamppb.New(created)}}), nil
}
