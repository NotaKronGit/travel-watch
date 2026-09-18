package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestYAMLEnv(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("yandex:\n  api_key: ''\n  timeout: 15s\n  max_response_bytes: 1024\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COLLECTOR_YANDEX_API_KEY", "test-only")
	t.Setenv("COLLECTOR_YANDEX_TIMEOUT", "2s")
	c, err := Load(path)
	if err != nil || c.Yandex.Timeout != 2*time.Second || c.Yandex.APIKey != "test-only" {
		t.Fatal("env override failed")
	}
	t.Setenv("COLLECTOR_YANDEX_API_KEY", "")
	if _, err := Load(path); err == nil {
		t.Fatal("empty key accepted")
	}
}

func TestFliConfigurationDoesNotRequireYandexKey(t *testing.T) {
	t.Setenv("COLLECTOR_YANDEX_API_KEY", "")
	t.Setenv("COLLECTOR_FLI_TIMEOUT", "5s")
	c, err := LoadFlights("../../config.yaml")
	if err != nil || c.Fli.Timeout != 5*time.Second {
		t.Fatal(c.Fli, err)
	}
	t.Setenv("COLLECTOR_FLI_TIMEOUT", "0s")
	if _, err = LoadFlights("../../config.yaml"); err == nil {
		t.Fatal("invalid timeout accepted")
	}
}
