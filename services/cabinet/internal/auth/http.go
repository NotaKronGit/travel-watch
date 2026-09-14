package auth

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"
	"github.com/NotaKronGit/travel-watch/gen/travelwatch/cabinet/v1/cabinetv1connect"
	"github.com/NotaKronGit/travel-watch/services/cabinet/internal/config"
)

type attempt struct {
	count int
	until time.Time
}
type limiter struct {
	mu      sync.Mutex
	entries map[string]attempt
	config  config.Auth
}

func (l *limiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for key, a := range l.entries {
		if !now.Before(a.until) {
			delete(l.entries, key)
		}
	}
	a, exists := l.entries[key]
	if !exists {
		if len(l.entries) >= l.config.MaxTrackedAddresses {
			return false
		}
		a = attempt{until: now.Add(l.config.LoginWindow)}
	}
	if a.count >= l.config.LoginAttempts {
		return false
	}
	a.count++
	l.entries[key] = a
	return true
}
func reject(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"code": code, "message": message})
}
func Handler(store Repository, cfg config.Config) http.Handler {
	service := NewService(store, cfg.Auth)
	path, rpc := cabinetv1connect.NewAuthServiceHandler(service, connect.WithReadMaxBytes(cfg.Server.MaxBodyBytes), connect.WithSendMaxBytes(cfg.Server.MaxBodyBytes))
	limits := &limiter{entries: make(map[string]attempt), config: cfg.Auth}
	hashSlots := make(chan struct{}, cfg.Auth.HashConcurrency)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), cfg.Server.HealthTimeout)
		defer cancel()
		if err := store.Ping(ctx); err != nil {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.Handle(path, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.Method != http.MethodPost {
			reject(w, 405, "unimplemented", "Метод не поддерживается")
			return
		}
		// Cookie-аутентификация требует защиты и входа, и остальных RPC от CSRF.
		// Frontend обращается через same-origin proxy; CORS здесь не включается.
		if r.Header.Get("Origin") != cfg.Server.Origin || r.Header.Get("X-Travel-Watch-CSRF") != "1" {
			reject(w, 403, "permission_denied", "Недопустимый источник запроса")
			return
		}
		if strings.HasSuffix(r.URL.Path, "/Login") || strings.HasSuffix(r.URL.Path, "/Register") {
			host, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				host = r.RemoteAddr
			}
			// X-Forwarded-For не доверяем: этот этап рассчитан на один локальный экземпляр.
			if !limits.allow(host, time.Now()) {
				w.Header().Set("Retry-After", strconv.FormatInt(int64((cfg.Auth.LoginWindow+time.Second-1)/time.Second), 10))
				reject(w, 429, "resource_exhausted", "Слишком много попыток. Попробуйте позже")
				return
			}
			select {
			case hashSlots <- struct{}{}:
				defer func() { <-hashSlots }()
			default:
				reject(w, 429, "resource_exhausted", "Сервер занят. Попробуйте позже")
				return
			}
		}
		ctx, cancel := context.WithTimeout(r.Context(), cfg.Server.RequestTimeout)
		defer cancel()
		r.Body = http.MaxBytesReader(w, r.Body, int64(cfg.Server.MaxBodyBytes))
		rpc.ServeHTTP(w, r.WithContext(ctx))
	}))
	return mux
}
