package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/signal"
	"syscall"

	"github.com/NotaKronGit/travel-watch/api/transport"
	"github.com/NotaKronGit/travel-watch/services/collector/internal/config"
	"github.com/NotaKronGit/travel-watch/services/collector/internal/rail/yandex"
	"github.com/joho/godotenv"
)

func transportStdio() error {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.New("cannot read .env")
	}
	path := os.Getenv("COLLECTOR_CONFIG")
	if path == "" {
		path = "services/collector/config.yaml"
	}
	c, err := config.Load(path)
	if err != nil {
		return err
	}
	p, err := yandex.New(c.ProviderConfig())
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	input := bufio.NewScanner(os.Stdin)
	input.Buffer(make([]byte, 4096), 16384)
	output := json.NewEncoder(os.Stdout)
	for input.Scan() {
		var q transport.Request
		if json.Unmarshal(input.Bytes(), &q) != nil {
			return errors.New("invalid transport frame")
		}
		result, e := p.Transport(ctx, q)
		if e != nil {
			result = transport.Response{Version: 1, Error: e.Error()}
		}
		if err = output.Encode(result); err != nil {
			return errors.New("cannot write transport frame")
		}
	}
	return input.Err()
}
