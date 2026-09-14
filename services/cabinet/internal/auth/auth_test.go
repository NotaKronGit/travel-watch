package auth

import (
	"github.com/NotaKronGit/travel-watch/services/cabinet/internal/config"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testConfig(t *testing.T) config.Config {
	t.Helper()
	t.Setenv("CABINET_DATABASE_APP_PASSWORD", "test-only")
	cfg, err := config.Load("../../config.yaml", "serve")
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestPassword(t *testing.T) {
	password := "correct horse battery staple"
	a, b := hashPassword(password), hashPassword(password)
	if a == b {
		t.Fatal("password hashes must use independent salts")
	}
	for _, tc := range []struct {
		hash, password string
		want           bool
		bad            bool
	}{{a, password, true, false}, {a, "wrong password here", false, false}, {"broken", password, false, true}} {
		got, err := verifyPassword(tc.hash, tc.password)
		if got != tc.want || (err != nil) != tc.bad {
			t.Fatalf("verification got=%v err=%v", got, err)
		}
	}
}
func TestCredentials(t *testing.T) {
	email, err := credentials(" USER@Example.COM ", "long enough password")
	if err != nil || email != "user@example.com" {
		t.Fatalf("normalization: %q %v", email, err)
	}
	for _, tc := range []struct{ email, password string }{{"not-email", "long enough password"}, {"a@b.c", "short"}, {"a@b.c", strings.Repeat("x", 1025)}} {
		if _, err := credentials(tc.email, tc.password); err == nil {
			t.Fatal("invalid credentials accepted")
		}
	}
	if _, err := credentials("a@b.c", strings.Repeat("я", 15)); err != nil {
		t.Fatal(err)
	}
}
func TestLimit(t *testing.T) {
	l := &limiter{entries: map[string]attempt{}, config: testConfig(t).Auth}
	now := time.Now()
	for i := 0; i < 10; i++ {
		if !l.allow("ip", now) {
			t.Fatal("early rejection")
		}
	}
	if l.allow("ip", now) {
		t.Fatal("limit not enforced")
	}
	if !l.allow("ip", now.Add(time.Minute)) {
		t.Fatal("window not reset")
	}
}
func TestOriginAndCSRF(t *testing.T) {
	handler := Handler(nil, testConfig(t))
	for _, tc := range []struct{ origin, csrf string }{{"http://evil.example", "1"}, {"http://localhost:5173", ""}, {"", "1"}} {
		r := httptest.NewRequest(http.MethodPost, "/travelwatch.cabinet.v1.AuthService/Login", strings.NewReader(`{}`))
		r.Header.Set("Origin", tc.origin)
		r.Header.Set("X-Travel-Watch-CSRF", tc.csrf)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatalf("got %d", w.Code)
		}
	}
}
