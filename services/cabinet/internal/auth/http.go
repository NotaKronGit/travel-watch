package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"
	"github.com/NotaKronGit/travel-watch/gen/travelwatch/cabinet/v1/cabinetv1connect"
)

type attempt struct {
	count int
	until time.Time
}
type limiter struct {
	mu      sync.Mutex
	entries map[string]attempt
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
		if len(l.entries) >= 4096 {
			return false
		}
		a = attempt{until: now.Add(time.Minute)}
	}
	if a.count >= 10 {
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
func Handler(db *sql.DB, origin string, secure bool) http.Handler {
	service := NewService(db, secure)
	path, rpc := cabinetv1connect.NewAuthServiceHandler(service, connect.WithReadMaxBytes(8192), connect.WithSendMaxBytes(8192))
	limits := &limiter{entries: make(map[string]attempt)}
	hashSlots := make(chan struct{}, 4)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		if err := db.PingContext(ctx); err != nil {
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
		if r.Header.Get("Origin") != origin || r.Header.Get("X-Travel-Watch-CSRF") != "1" {
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
				w.Header().Set("Retry-After", "60")
				reject(w, 429, "resource_exhausted", "Слишком много попыток. Попробуйте через минуту")
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
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		r.Body = http.MaxBytesReader(w, r.Body, 8192)
		rpc.ServeHTTP(w, r.WithContext(ctx))
	}))
	return mux
}
