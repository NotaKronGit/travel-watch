package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/NotaKronGit/travel-watch/services/cabinet/internal/auth"
	"github.com/NotaKronGit/travel-watch/services/cabinet/migrations"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"
	"github.com/pressly/goose/v3"
)

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
func databaseURL(migrate bool) (string, error) {
	key, user := "CABINET_APP_PASSWORD", "cabinet_app"
	if migrate {
		key, user = "CABINET_OWNER_PASSWORD", "cabinet_owner"
	}
	password := os.Getenv(key)
	if password == "" {
		return "", fmt.Errorf("%s is required", key)
	}
	u := url.URL{Scheme: "postgres", Host: net.JoinHostPort(env("POSTGRES_HOST", "127.0.0.1"), env("POSTGRES_PORT", "55432")), Path: "cabinet", User: url.UserPassword(user, password)}
	// TLS завершает reverse proxy; PostgreSQL stage доступен только во внутренней Docker-сети.
	u.RawQuery = "sslmode=disable&connect_timeout=5"
	return u.String(), nil
}
func run() error {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.New("cannot read .env")
	}
	command := "serve"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}
	if command != "serve" && command != "migrate" {
		return errors.New("usage: cabinet [serve|migrate]")
	}
	dsn, err := databaseURL(command == "migrate")
	if err != nil {
		return err
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return errors.New("cannot initialize database connection")
	}
	defer db.Close()
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	err = db.PingContext(pingCtx)
	cancel()
	if err != nil {
		return errors.New("database unavailable; check .env and make db-up")
	}
	if command == "migrate" {
		goose.SetBaseFS(migrations.Files)
		if err = goose.SetDialect("postgres"); err != nil {
			return err
		}
		migrationCtx, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()
		return goose.UpContext(migrationCtx, db, ".")
	}
	origin := env("CABINET_ORIGIN", "http://localhost:5173")
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return errors.New("CABINET_ORIGIN must be an HTTP(S) origin without a path")
	}
	secure, err := strconv.ParseBool(env("CABINET_COOKIE_SECURE", "true"))
	if err != nil {
		return errors.New("invalid CABINET_COOKIE_SECURE")
	}
	if !secure && (u.Scheme != "http" || (u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1")) {
		return errors.New("insecure cookies are allowed only on local HTTP")
	}
	if secure && u.Scheme != "https" {
		return errors.New("secure cookies require an HTTPS origin; use .env.example for local HTTP")
	}
	server := &http.Server{Addr: env("CABINET_ADDR", "127.0.0.1:8080"), Handler: auth.Handler(db, origin, secure), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	errCh := make(chan error, 1)
	go func() { slog.Info("cabinet listening", "address", server.Addr); errCh <- server.ListenAndServe() }()
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case err = <-errCh:
			if errors.Is(err, http.ErrServerClosed) {
				return nil
			}
			return err
		case <-ctx.Done():
			shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			return server.Shutdown(shutdown)
		case <-ticker.C:
			cleanup, cancel := context.WithTimeout(ctx, 5*time.Second)
			_, err = db.ExecContext(cleanup, "DELETE FROM sessions WHERE expires_at<=now()")
			cancel()
			if err != nil {
				slog.Error("expired session cleanup failed")
			}
		}
	}
}
func main() {
	if err := run(); err != nil {
		slog.Error("cabinet stopped", "error", err)
		os.Exit(1)
	}
}
