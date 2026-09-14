package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
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
	"github.com/NotaKronGit/travel-watch/services/cabinet/internal/config"
	"github.com/NotaKronGit/travel-watch/services/cabinet/internal/storage"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var emailPattern = regexp.MustCompile(`^[a-z0-9.!#$%&'*+/=?^_` + "`" + `{|}~-]+@[a-z0-9](?:[a-z0-9-]*[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]*[a-z0-9])?)+$`)

type Repository interface {
	FindUserByEmail(context.Context, string) (storage.User, error)
	FindUserBySession(context.Context, []byte) (storage.User, error)
	RegisterWithSession(context.Context, string, string, storage.Session, []byte) (storage.User, error)
	ReplaceSession(context.Context, string, storage.Session, []byte) error
	DeleteSession(context.Context, []byte) error
	Ping(context.Context) error
}

type Service struct {
	store      Repository
	secure     bool
	cookieName string
	dummyHash  string
	sessionTTL time.Duration
}

func NewService(store Repository, cfg config.Auth) *Service {
	name := "tw_session"
	if cfg.CookieSecure {
		name = "__Host-tw_session"
	}
	return &Service{store: store, secure: cfg.CookieSecure, cookieName: name, sessionTTL: cfg.SessionTTL, dummyHash: hashPassword("dummy-password-for-equal-work")}
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
	slog.Error("cabinet database operation failed", "error_type", storage.ErrorKind(err))
	return connect.NewError(connect.CodeInternal, errors.New("Не удалось выполнить запрос. Попробуйте позже"))
}
func (s *Service) cookie(token string, expires time.Time) *http.Cookie {
	return &http.Cookie{Name: s.cookieName, Value: token, Path: "/", HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteStrictMode, Expires: expires, MaxAge: int(s.sessionTTL.Seconds())}
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
func (s *Service) newSession() (storage.Session, *http.Cookie) {
	raw := make([]byte, 32)
	_, _ = rand.Read(raw)
	hash := sha256.Sum256(raw)
	expires := time.Now().UTC().Add(s.sessionTTL)
	return storage.Session{TokenHash: hash[:], ExpiresAt: expires}, s.cookie(base64.RawURLEncoding.EncodeToString(raw), expires)
}
func (s *Service) Register(ctx context.Context, req *connect.Request[cabinetv1.RegisterRequest]) (*connect.Response[cabinetv1.RegisterResponse], error) {
	email, err := credentials(req.Msg.Email, req.Msg.Password)
	if err != nil {
		return nil, err
	}
	hash := hashPassword(req.Msg.Password)
	session, cookie := s.newSession()
	user, err := s.store.RegisterWithSession(ctx, email, hash, session, s.sessionHash(req.Header()))
	if errors.Is(err, storage.ErrEmailExists) {
		return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("Не удалось зарегистрировать аккаунт с этим email"))
	}
	if err != nil {
		return nil, internalError(err)
	}
	res := connect.NewResponse(&cabinetv1.RegisterResponse{User: &cabinetv1.User{Id: user.ID, Email: user.Email, CreatedAt: timestamppb.New(user.CreatedAt)}})
	res.Header().Add("Set-Cookie", cookie.String())
	return res, nil
}
func (s *Service) Login(ctx context.Context, req *connect.Request[cabinetv1.LoginRequest]) (*connect.Response[cabinetv1.LoginResponse], error) {
	email, err := credentials(req.Msg.Email, req.Msg.Password)
	if err != nil {
		return nil, err
	}
	user, err := s.store.FindUserByEmail(ctx, email)
	hash := user.PasswordHash
	missing := errors.Is(err, storage.ErrNotFound)
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
	session, cookie := s.newSession()
	if err := s.store.ReplaceSession(ctx, user.ID, session, s.sessionHash(req.Header())); err != nil {
		return nil, internalError(err)
	}
	res := connect.NewResponse(&cabinetv1.LoginResponse{User: &cabinetv1.User{Id: user.ID, Email: user.Email, CreatedAt: timestamppb.New(user.CreatedAt)}})
	res.Header().Add("Set-Cookie", cookie.String())
	return res, nil
}
func (s *Service) Logout(ctx context.Context, req *connect.Request[cabinetv1.LogoutRequest]) (*connect.Response[cabinetv1.LogoutResponse], error) {
	if hash := s.sessionHash(req.Header()); hash != nil {
		if err := s.store.DeleteSession(ctx, hash); err != nil {
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
	user, err := s.store.FindUserBySession(ctx, hash)
	if errors.Is(err, storage.ErrNotFound) {
		return nil, unauth
	}
	if err != nil {
		return nil, internalError(err)
	}
	return connect.NewResponse(&cabinetv1.GetCurrentUserResponse{User: &cabinetv1.User{Id: user.ID, Email: user.Email, CreatedAt: timestamppb.New(user.CreatedAt)}}), nil
}
