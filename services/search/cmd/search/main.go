package main

import (
	"context"
	"database/sql"
	"errors"
	"github.com/NotaKronGit/travel-watch/services/search/internal/config"
	"github.com/NotaKronGit/travel-watch/services/search/internal/consumer"
	"github.com/NotaKronGit/travel-watch/services/search/internal/storage"
	"github.com/NotaKronGit/travel-watch/services/search/migrations"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"
	"github.com/pressly/goose/v3"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	_ "time/tzdata"
)

func run() error {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.New("cannot read .env")
	}
	command := "consume"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}
	path := os.Getenv("SEARCH_CONFIG")
	if path == "" {
		path = "config.yaml"
	}
	cfg, err := config.Load(path, command)
	if err != nil {
		return err
	}
	db, err := sql.Open("pgx", cfg.Database.URL(command == "migrate"))
	if err != nil {
		return errors.New("cannot initialize Search database")
	}
	defer db.Close()
	db.SetMaxOpenConns(cfg.Database.MaxOpenConns)
	db.SetMaxIdleConns(cfg.Database.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.Database.ConnMaxLifetime)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ping, cancel := context.WithTimeout(ctx, cfg.Database.PingTimeout)
	err = db.PingContext(ping)
	cancel()
	if err != nil {
		return errors.New("Search database unavailable")
	}
	if command == "migrate" {
		goose.SetBaseFS(migrations.Files)
		if err = goose.SetDialect("postgres"); err != nil {
			return err
		}
		migration, cancel := context.WithTimeout(ctx, cfg.Database.MigrationTimeout)
		defer cancel()
		if err = goose.UpContext(migration, db, "."); err != nil {
			return errors.New("Search migration failed")
		}
		return nil
	}
	reader := consumer.NewReader(cfg.Consumer)
	defer func() {
		if err := reader.Close(); err != nil {
			slog.Warn("Search reader close failed")
		}
	}()
	slog.Info("Search consuming trip events")
	err = (consumer.Consumer{Reader: reader, Repository: storage.New(db), DBTimeout: cfg.Consumer.DBTimeout, CommitTimeout: cfg.Consumer.CommitTimeout}).Run(ctx)
	if ctx.Err() != nil {
		return nil
	}
	return err
}
func main() {
	if err := run(); err != nil {
		slog.Error("Search stopped", "error", err)
		os.Exit(1)
	}
}
