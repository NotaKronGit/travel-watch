package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfig(t *testing.T) {
	t.Setenv("SEARCH_DATABASE_APP_PASSWORD", "test-placeholder")
	t.Setenv("SEARCH_CONSUMER_BROKERS", "localhost:19092")
	c, err := Load("../../config.yaml", "consume")
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Consumer.Brokers) != 1 || c.Consumer.Brokers[0] != "localhost:19092" {
		t.Fatal("env override lost")
	}
	t.Setenv("SEARCH_DATABASE_APP_PASSWORD", "")
	if _, err = Load("../../config.yaml", "consume"); err == nil {
		t.Fatal("empty secret accepted")
	}
	p := filepath.Join(t.TempDir(), "config.yaml")
	if err = os.WriteFile(p, []byte("unknown: true\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = Load(p, "migrate"); err == nil {
		t.Fatal("unknown key accepted")
	}
}
