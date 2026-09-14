package config

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func configFile(t *testing.T, transform func(string) string) string {
	t.Helper()
	data, err := os.ReadFile("../../config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(transform(string(data))), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestPrecedenceAndTypes(t *testing.T) {
	path := configFile(t, func(s string) string { return strings.ReplaceAll(s, `app_password: ""`, `app_password: yaml-password`) })
	t.Setenv("CABINET_DATABASE_APP_PASSWORD", "env-password")
	t.Setenv("CABINET_AUTH_SESSION_TTL", "24h")
	t.Setenv("CABINET_DATABASE_MAX_OPEN_CONNS", "12")
	t.Setenv("CABINET_SERVER_ORIGIN", "https://example.com")
	t.Setenv("CABINET_AUTH_COOKIE_SECURE", "true")
	c, err := Load(path, "serve")
	if err != nil {
		t.Fatal(err)
	}
	if c.Database.AppPassword != "env-password" || c.Auth.SessionTTL != 24*time.Hour || c.Database.MaxOpenConns != 12 || !c.Auth.CookieSecure {
		t.Fatal("env overrides not decoded")
	}
	t.Setenv("CABINET_DATABASE_APP_PASSWORD", "")
	if _, err := Load(path, "serve"); err == nil {
		t.Fatal("empty explicit secret fell back to YAML")
	}
}

func TestEnvForMissingYAMLKey(t *testing.T) {
	path := configFile(t, func(s string) string { return strings.ReplaceAll(s, "  app_password: \"\"\n", "") })
	t.Setenv("CABINET_DATABASE_APP_PASSWORD", "test-password")
	if _, err := Load(path, "serve"); err != nil {
		t.Fatal(err)
	}
}

func TestComposeEnvironment(t *testing.T) {
	t.Setenv("CABINET_DATABASE_HOST", "postgres")
	t.Setenv("CABINET_DATABASE_PORT", "5432")
	t.Setenv("CABINET_SERVER_ADDRESS", "0.0.0.0:8080")
	t.Setenv("CABINET_SERVER_ORIGIN", "http://localhost:18080")
	t.Setenv("CABINET_AUTH_COOKIE_SECURE", "false")
	t.Setenv("CABINET_DATABASE_APP_PASSWORD", "test-only-app")
	c, err := Load("../../config.yaml", "serve")
	if err != nil {
		t.Fatal(err)
	}
	if c.Database.Host != "postgres" || c.Database.Port != 5432 || c.Database.AppPassword != "test-only-app" || c.Server.Address != "0.0.0.0:8080" || c.Server.Origin != "http://localhost:18080" || c.Auth.CookieSecure {
		t.Fatal("Compose variables ignored")
	}
}

func TestCommandSpecificSecrets(t *testing.T) {
	path := configFile(t, func(s string) string { return s })
	t.Setenv("CABINET_DATABASE_APP_PASSWORD", "")
	t.Setenv("CABINET_DATABASE_OWNER_PASSWORD", "test-owner")
	if _, err := Load(path, "migrate"); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path, "serve"); err == nil {
		t.Fatal("serve accepted empty app password")
	}
	t.Setenv("CABINET_DATABASE_OWNER_PASSWORD", "")
	t.Setenv("CABINET_DATABASE_APP_PASSWORD", "test-app")
	if _, err := Load(path, "serve"); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path, "migrate"); err == nil {
		t.Fatal("migrate accepted empty owner password")
	}
}

func TestInvalidConfiguration(t *testing.T) {
	for _, tc := range []struct{ key, value string }{
		{"CABINET_DATABASE_PORT", "0"}, {"CABINET_DATABASE_MAX_OPEN_CONNS", "0"},
		{"CABINET_DATABASE_MAX_IDLE_CONNS", "11"}, {"CABINET_SERVER_REQUEST_TIMEOUT", "0s"},
		{"CABINET_SERVER_REQUEST_TIMEOUT", "secret-invalid-duration"},
		{"CABINET_AUTH_COOKIE_SECURE", "secret-not-bool"}, {"CABINET_AUTH_HASH_CONCURRENCY", "-1"},
		{"CABINET_SERVER_ORIGIN", "https://example.com"}, {"CABINET_DATABASE_SSLMODE", "invalid"},
	} {
		t.Run(tc.key+tc.value, func(t *testing.T) {
			t.Setenv("CABINET_DATABASE_APP_PASSWORD", "secret-test-password")
			t.Setenv(tc.key, tc.value)
			_, err := Load("../../config.yaml", "serve")
			if err == nil {
				t.Fatal("invalid configuration accepted")
			}
			if strings.Contains(err.Error(), "secret-") {
				t.Fatal("secret leaked in error")
			}
		})
	}
}

func TestInvalidYAML(t *testing.T) {
	for _, suffix := range []string{"\nunknown_key: secret-test-password\n", "\ninvalid: [secret-test-password\n"} {
		path := configFile(t, func(s string) string { return s + suffix })
		_, err := Load(path, "serve")
		if err == nil || strings.Contains(err.Error(), "secret-test-password") {
			t.Fatal("invalid YAML accepted or leaked")
		}
	}
	if _, err := Load(filepath.Join(t.TempDir(), "missing.yaml"), "serve"); err == nil {
		t.Fatal("missing explicit configuration accepted")
	}
}

func TestDatabaseURL(t *testing.T) {
	d := Database{Host: "::1", Port: 5432, Name: "cabinet", AppUser: "cabinet_app", AppPassword: "p@ss:/?#%", OwnerUser: "cabinet_owner", OwnerPassword: "owner", SSLMode: "verify-full", ConnectTimeout: 5 * time.Second}
	u, err := url.Parse(d.URL(false))
	if err != nil {
		t.Fatal(err)
	}
	password, _ := u.User.Password()
	if password != d.AppPassword || u.Host != "[::1]:5432" || u.Query().Get("sslmode") != "verify-full" {
		t.Fatal("incorrect DSN encoding")
	}
	u, err = url.Parse(d.URL(true))
	if err != nil || u.User.Username() != d.OwnerUser {
		t.Fatal("migration uses wrong role")
	}
}

func TestCatalogCommandConfig(t *testing.T) {
	t.Setenv("CABINET_DATABASE_OWNER_PASSWORD", "test-owner")
	t.Setenv("CABINET_DATABASE_APP_PASSWORD", "")
	t.Setenv("CABINET_CATALOG_MIN_CITIES", "2")
	c, err := Load("../../config.yaml", "sync-cities")
	if err != nil {
		t.Fatal(err)
	}
	if c.Catalog.MinCities != 2 {
		t.Fatal("catalog env override ignored")
	}
	t.Setenv("CABINET_CATALOG_CITIES_URL", "http://example.com/cities.zip")
	if _, err := Load("../../config.yaml", "sync-cities"); err == nil {
		t.Fatal("insecure source accepted")
	}
	t.Setenv("CABINET_CATALOG_CITIES_URL", "https://example.com/cities.zip")
	t.Setenv("CABINET_CATALOG_MAX_CITIES", "1")
	if _, err := Load("../../config.yaml", "sync-cities"); err == nil {
		t.Fatal("invalid count limits accepted")
	}
	t.Setenv("CABINET_CATALOG_MAX_CITIES", "10")
	t.Setenv("CABINET_DATABASE_OWNER_PASSWORD", "")
	if _, err := Load("../../config.yaml", "sync-cities"); err == nil {
		t.Fatal("missing owner password accepted")
	}
}
