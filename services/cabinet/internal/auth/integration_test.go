//go:build integration

package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	cabinetv1 "github.com/NotaKronGit/travel-watch/gen/travelwatch/cabinet/v1"
	"github.com/NotaKronGit/travel-watch/gen/travelwatch/cabinet/v1/cabinetv1connect"
	"github.com/NotaKronGit/travel-watch/services/cabinet/internal/storage"
	"github.com/NotaKronGit/travel-watch/services/cabinet/migrations"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"
	"github.com/pressly/goose/v3"
)

func TestPostgresAuth(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	migrator, db := integrationDB(t, ctx)
	if _, err := db.ExecContext(ctx, "CREATE TABLE forbidden(id int)"); err == nil {
		t.Fatal("app role can perform DDL")
	}
	t.Run("city catalog", func(t *testing.T) { testCityCatalog(t, ctx, migrator, db) })
	t.Run("trip storage", func(t *testing.T) { testTripStorage(t, ctx, migrator, db) })
	server := httptest.NewServer(Handler(storage.New(db), testConfig(t)))
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := cabinetv1connect.NewAuthServiceClient(&http.Client{Jar: jar, Timeout: 10 * time.Second}, server.URL)
	requestHeaders := func(h http.Header) { h.Set("Origin", "http://localhost:5173"); h.Set("X-Travel-Watch-CSRF", "1") }
	register := func(email string) (*connect.Response[cabinetv1.RegisterResponse], error) {
		r := connect.NewRequest(&cabinetv1.RegisterRequest{Email: email, Password: "correct horse battery staple"})
		requestHeaders(r.Header())
		return client.Register(ctx, r)
	}
	current := func() (*connect.Response[cabinetv1.GetCurrentUserResponse], error) {
		r := connect.NewRequest(&cabinetv1.GetCurrentUserRequest{})
		requestHeaders(r.Header())
		return client.GetCurrentUser(ctx, r)
	}
	logout := func() error {
		r := connect.NewRequest(&cabinetv1.LogoutRequest{})
		requestHeaders(r.Header())
		_, err := client.Logout(ctx, r)
		return err
	}
	login := func(email, password string) error {
		r := connect.NewRequest(&cabinetv1.LoginRequest{Email: email, Password: password})
		requestHeaders(r.Header())
		_, err := client.Login(ctx, r)
		return err
	}
	if _, err := current(); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatal("anonymous access accepted", err)
	}
	registered, err := register(" USER@EXAMPLE.COM ")
	if err != nil {
		t.Fatal(err)
	}
	if registered.Msg.User.Email != "user@example.com" {
		t.Fatal("email not normalized")
	}
	cookies := (&http.Response{Header: registered.Header()}).Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatal("cookie protections missing")
	}
	u, _ := url.Parse(server.URL)
	oldCookie := jar.Cookies(u)[0]
	profile, err := current()
	if err != nil || profile.Msg.User.Id != registered.Msg.User.Id {
		t.Fatal("own profile unavailable", err)
	}
	if _, err := register("user@example.com"); connect.CodeOf(err) != connect.CodeAlreadyExists {
		t.Fatal("duplicate accepted", err)
	}
	if err := login("user@example.com", "wrong password that is long"); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatal("wrong password accepted", err)
	}
	if err := login("unknown@example.com", "wrong password that is long"); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatal("unknown user accepted", err)
	}
	if err := login("user@example.com", "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	// Старый токен перестал работать после повторного входа.
	oldReq := connect.NewRequest(&cabinetv1.GetCurrentUserRequest{})
	requestHeaders(oldReq.Header())
	oldReq.Header().Set("Cookie", oldCookie.String())
	plain := cabinetv1connect.NewAuthServiceClient(server.Client(), server.URL)
	if _, err := plain.GetCurrentUser(ctx, oldReq); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatal("old session survived rotation", err)
	}
	if err := logout(); err != nil {
		t.Fatal(err)
	}
	if err := logout(); err != nil {
		t.Fatal("logout not idempotent", err)
	}
	if _, err := current(); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatal("logout failed", err)
	}
	if err := login("user@example.com", "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	if _, err := migrator.ExecContext(ctx, "UPDATE sessions SET created_at=now()-interval '2 days',expires_at=now()-interval '1 day'"); err != nil {
		t.Fatal(err)
	}
	if _, err := current(); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatal("expired session accepted", err)
	}
	t.Run("storage rolls back partial registration and session replacement", func(t *testing.T) {
		store := storage.New(db)
		first := storage.Session{TokenHash: randBytes(32), ExpiresAt: time.Now().Add(time.Hour)}
		collision := storage.Session{TokenHash: randBytes(32), ExpiresAt: time.Now().Add(time.Hour)}
		for _, session := range []storage.Session{first, collision} {
			if err := store.ReplaceSession(ctx, registered.Msg.User.Id, session, nil); err != nil {
				t.Fatal(err)
			}
		}
		// The second insert violates token uniqueness after deletion of the old token.
		if err := store.ReplaceSession(ctx, registered.Msg.User.Id, collision, first.TokenHash); err == nil {
			t.Fatal("duplicate token accepted")
		}
		if _, err := store.FindUserBySession(ctx, first.TokenHash); err != nil {
			t.Fatal("old session lost on failed replacement", err)
		}
		if _, err := store.RegisterWithSession(ctx, "rollback@example.com", "test-only-hash", collision, first.TokenHash); err == nil {
			t.Fatal("registration with duplicate token accepted")
		}
		if _, err := store.FindUserByEmail(ctx, "rollback@example.com"); !errors.Is(err, storage.ErrNotFound) {
			t.Fatal("partial user survived rollback", err)
		}
		if _, err := store.FindUserBySession(ctx, first.TokenHash); err != nil {
			t.Fatal("old session lost on failed registration", err)
		}
		if _, err := store.FindUserByEmail(ctx, "' OR 1=1 --"); !errors.Is(err, storage.ErrNotFound) {
			t.Fatal("email was not parameterized", err)
		}
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		if err := store.DeleteSession(cancelled, first.TokenHash); err == nil {
			t.Fatal("cancelled operation succeeded")
		}
		if _, err := store.FindUserBySession(ctx, first.TokenHash); err != nil {
			t.Fatal("cancelled deletion removed session", err)
		}
		if err := store.DeleteExpiredSessions(ctx); err != nil {
			t.Fatal(err)
		}
		var expired int
		if err := db.QueryRowContext(ctx, "SELECT count(*) FROM sessions WHERE expires_at<=now()").Scan(&expired); err != nil || expired != 0 {
			t.Fatal("expired sessions not cleaned", err)
		}
		if _, err := store.FindUserBySession(ctx, first.TokenHash); err != nil {
			t.Fatal("cleanup removed active session", err)
		}
	})
	// Новый обработчик даёт отдельное окно лимита для конкурентной регистрации.
	concurrent := httptest.NewServer(Handler(storage.New(db), testConfig(t)))
	defer concurrent.Close()
	cc := cabinetv1connect.NewAuthServiceClient(concurrent.Client(), concurrent.URL)
	var wg sync.WaitGroup
	codes := make(chan connect.Code, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := connect.NewRequest(&cabinetv1.RegisterRequest{Email: "race@example.com", Password: "correct horse battery staple"})
			requestHeaders(r.Header())
			_, err := cc.Register(ctx, r)
			if err == nil {
				codes <- 0
			} else {
				codes <- connect.CodeOf(err)
			}
		}()
	}
	wg.Wait()
	close(codes)
	success, duplicate := 0, 0
	for code := range codes {
		if code == 0 {
			success++
		} else if code == connect.CodeAlreadyExists {
			duplicate++
		} else {
			t.Fatal("unexpected concurrent result", code)
		}
	}
	if success != 1 || duplicate != 1 {
		t.Fatal("registration not atomic")
	}
	var count int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM users WHERE email='race@example.com'").Scan(&count); err != nil || count != 1 {
		t.Fatal("duplicate user", count, err)
	}
	for i := 0; i < 11; i++ {
		r := connect.NewRequest(&cabinetv1.LoginRequest{Email: fmt.Sprintf("n%d@example.com", i), Password: "correct horse battery staple"})
		requestHeaders(r.Header())
		_, err = cc.Login(ctx, r)
	}
	if connect.CodeOf(err) != connect.CodeResourceExhausted {
		t.Fatal("HTTP rate limit not applied", err)
	}
}
func randBytes(n int) []byte { b := make([]byte, n); _, _ = rand.Read(b); return b }

func integrationDB(t *testing.T, ctx context.Context) (*sql.DB, *sql.DB) {
	t.Helper()
	env, err := godotenv.Read(filepath.Join("..", "..", "..", "..", ".env"))
	if err != nil {
		t.Fatal("integration test requires root .env and make db-up:", err)
	}
	port := env["CABINET_DATABASE_PORT"]
	if port == "" {
		port = "55432"
	}
	dsn := func(user, password, schema string) string {
		u := url.URL{Scheme: "postgres", Host: "127.0.0.1:" + port, Path: "cabinet", User: url.UserPassword(user, password)}
		q := url.Values{"sslmode": {"disable"}, "connect_timeout": {"5"}}
		if schema != "" {
			q.Set("search_path", schema)
		}
		u.RawQuery = q.Encode()
		return u.String()
	}
	open := func(user, password, schema string) *sql.DB {
		t.Helper()
		db, err := sql.Open("pgx", dsn(user, password, schema))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = db.Close() })
		return db
	}
	owner := open("cabinet_owner", env["CABINET_DATABASE_OWNER_PASSWORD"], "")
	if err := owner.PingContext(ctx); err != nil {
		t.Fatal("PostgreSQL unavailable; run make db-up")
	}
	schema := "test_auth_" + hex.EncodeToString(randBytes(8))
	if _, err := owner.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		if _, err := owner.ExecContext(cleanup, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error(err)
		}
	})
	if _, err := owner.ExecContext(ctx, "GRANT USAGE ON SCHEMA "+schema+" TO cabinet_app"); err != nil {
		t.Fatal(err)
	}
	migrator := open("cabinet_owner", env["CABINET_DATABASE_OWNER_PASSWORD"], schema)
	goose.SetBaseFS(migrations.Files)
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := goose.UpContext(ctx, migrator, "."); err != nil {
			t.Fatal(err)
		}
	}
	db := open("cabinet_app", env["CABINET_DATABASE_APP_PASSWORD"], schema)

	return migrator, db
}
