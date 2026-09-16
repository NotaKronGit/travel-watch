package main

import (
	"context"
	"database/sql"
	"errors"
	"github.com/NotaKronGit/travel-watch/services/search/internal/config"
	"github.com/NotaKronGit/travel-watch/services/search/internal/results"
	"github.com/NotaKronGit/travel-watch/services/search/internal/storage"
	"net/http"
	"time"
)

func serveResults(ctx context.Context, db *sql.DB, c config.Config) error {
	tls, err := c.Results.TLS.Load(true)
	if err != nil {
		return err
	}
	server := &http.Server{Addr: c.Results.Address, TLSConfig: tls, Handler: results.Handler(storage.New(db), c.Results.Timeout), ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: time.Minute, MaxHeaderBytes: 8192}
	done := make(chan error, 1)
	go func() { done <- server.ListenAndServeTLS("", "") }()
	select {
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		stop, cancel := context.WithTimeout(context.WithoutCancel(ctx), c.Results.ShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(stop); err != nil {
			_ = server.Close()
			<-done
			return err
		}
		<-done
		return nil
	}
}
