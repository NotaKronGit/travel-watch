package main

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/NotaKronGit/travel-watch/services/cabinet/internal/auth"
	"github.com/NotaKronGit/travel-watch/services/cabinet/internal/catalog"
	"github.com/NotaKronGit/travel-watch/services/cabinet/internal/config"
	"github.com/NotaKronGit/travel-watch/services/cabinet/internal/outbox"
	"github.com/NotaKronGit/travel-watch/services/cabinet/internal/progress"
	"github.com/NotaKronGit/travel-watch/services/cabinet/internal/searchclient"
	"github.com/NotaKronGit/travel-watch/services/cabinet/internal/storage"
	"github.com/NotaKronGit/travel-watch/services/cabinet/internal/trips"
	"github.com/NotaKronGit/travel-watch/services/cabinet/migrations"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"
	"github.com/pressly/goose/v3"
	_ "time/tzdata"
)

func run() error {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.New("cannot read .env")
	}
	command := "serve"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}
	if command != "serve" && command != "migrate" && command != "sync-cities" && command != "publish-outbox" && command != "consume-progress" {
		return errors.New("usage: cabinet [serve|migrate|sync-cities|publish-outbox]")
	}
	if command == "serve" {
		if err := godotenv.Load(".local/tls/cabinet.env"); err != nil && !errors.Is(err, os.ErrNotExist) {
			return errors.New("cannot load Cabinet TLS environment")
		}
	}
	configPath := os.Getenv("CABINET_CONFIG")
	if configPath == "" {
		configPath = "config.yaml"
	}
	cfg, err := config.Load(configPath, command)
	if err != nil {
		return err
	}
	db, err := sql.Open("pgx", cfg.Database.URL(command == "migrate" || command == "sync-cities"))
	if err != nil {
		return errors.New("cannot initialize database connection")
	}
	defer db.Close()
	db.SetMaxOpenConns(cfg.Database.MaxOpenConns)
	db.SetMaxIdleConns(cfg.Database.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.Database.ConnMaxLifetime)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pingCtx, cancel := context.WithTimeout(ctx, cfg.Database.PingTimeout)
	err = db.PingContext(pingCtx)
	cancel()
	if err != nil {
		return errors.New("database unavailable; check configuration and PostgreSQL")
	}
	if command == "migrate" {
		goose.SetBaseFS(migrations.Files)
		if err = goose.SetDialect("postgres"); err != nil {
			return err
		}
		migrationCtx, cancel := context.WithTimeout(ctx, cfg.Database.MigrationTimeout)
		defer cancel()
		return goose.UpContext(migrationCtx, db, ".")
	}
	store := storage.New(db)
	if command == "consume-progress" {
		reader := progress.NewReader(cfg.Progress)
		defer reader.Close()
		err := (progress.Consumer{Reader: reader, Repository: store, Timeout: cfg.Progress.Timeout}).Run(ctx)
		if ctx.Err() != nil {
			return nil
		}
		return err
	}
	if command == "publish-outbox" {
		publisher := outbox.NewKafkaPublisher(cfg.Outbox)
		defer func() {
			if err := publisher.Close(); err != nil {
				slog.Warn("outbox publisher close failed")
			}
		}()
		err := (outbox.Relay{Repository: store, Publisher: publisher, Config: cfg.Outbox}).Run(ctx)
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	}
	if command == "sync-cities" {
		syncCtx, cancel := context.WithTimeout(ctx, cfg.Catalog.SyncTimeout)
		defer cancel()
		g := cfg.Catalog
		source := catalog.GeoNamesSource{Client: &http.Client{Timeout: g.HTTPTimeout}, AlternateNamesURL: g.AlternateNamesURL, MaxAlternateDownloadBytes: g.MaxAlternateDownloadBytes, MaxAlternateUncompressedBytes: g.MaxAlternateUncompressedBytes, CitiesURL: g.CitiesURL, CountriesURL: g.CountriesURL, RegionsURL: g.RegionsURL, MaxDownloadBytes: g.MaxDownloadBytes, MaxUncompressedBytes: g.MaxUncompressedBytes, MaxCities: g.MaxCities}
		count, err := (catalog.Importer{Source: source, Repository: store, MinCities: g.MinCities}).Run(syncCtx)
		if err != nil {
			slog.Error("city catalog import failed", "error_type", storage.ErrorKind(err))
			return errors.New("city catalog import failed; previous catalog preserved; check source, limits and database")
		}
		slog.Info("city catalog imported", "cities", count)
		return nil
	}
	routes, closeRoutes, err := searchclient.New(cfg.Search)
	if err != nil {
		return err
	}
	defer closeRoutes()
	server := &http.Server{Addr: cfg.Server.Address, Handler: auth.Handler(store, cfg, trips.Handler(store, routes)), ReadHeaderTimeout: cfg.Server.ReadHeaderTimeout, ReadTimeout: cfg.Server.ReadTimeout, WriteTimeout: cfg.Server.WriteTimeout, IdleTimeout: cfg.Server.IdleTimeout, MaxHeaderBytes: cfg.Server.MaxHeaderBytes}
	errCh := make(chan error, 1)
	go func() { slog.Info("cabinet listening", "address", server.Addr); errCh <- server.ListenAndServe() }()
	ticker := time.NewTicker(cfg.Auth.CleanupInterval)
	defer ticker.Stop()
	// Trips whose dates are over become expired; the first pass runs at startup.
	expireTrips := func() {
		expiry, cancel := context.WithTimeout(ctx, cfg.Trips.ExpiryTimeout)
		n, err := store.ExpireTrips(expiry, time.Now(), cfg.Trips.ExpiryBatch)
		cancel()
		if err != nil {
			slog.Error("trip expiry failed", "expired", n)
		} else if n > 0 {
			slog.Info("trips expired", "count", n)
		}
	}
	expireTrips()
	expiry := time.NewTicker(cfg.Trips.ExpiryInterval)
	defer expiry.Stop()
	for {
		select {
		case err = <-errCh:
			if errors.Is(err, http.ErrServerClosed) {
				return nil
			}
			return err
		case <-ctx.Done():
			shutdown, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
			defer cancel()
			return server.Shutdown(shutdown)
		case <-ticker.C:
			cleanup, cancel := context.WithTimeout(ctx, cfg.Auth.CleanupTimeout)
			err = store.DeleteExpiredSessions(cleanup)
			cancel()
			if err != nil {
				slog.Error("expired session cleanup failed")
			}
		case <-expiry.C:
			expireTrips()
		}
	}
}
func main() {
	if err := run(); err != nil {
		slog.Error("cabinet stopped", "error", err)
		os.Exit(1)
	}
}
