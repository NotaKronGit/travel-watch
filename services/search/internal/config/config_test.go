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

func TestComparisonConfig(t *testing.T) {
	t.Setenv("SEARCH_DATABASE_APP_PASSWORD", "")
	t.Setenv("SEARCH_GEMINI_API_KEY", "test-placeholder")
	t.Setenv("SEARCH_GEMINI_MAX_OUTPUT_TOKENS", "512")
	c, err := Load("../../config.yaml", "compare-planners")
	if err != nil {
		t.Fatal(err)
	}
	if c.Gemini.MaxOutputTokens != 512 {
		t.Fatal("Gemini env override lost")
	}
	t.Setenv("SEARCH_GEMINI_MAX_OUTPUT_TOKENS", "99999")
	if _, err = Load("../../config.yaml", "compare-planners"); err == nil {
		t.Fatal("unbounded tokens accepted")
	}
	t.Setenv("SEARCH_GEMINI_MAX_OUTPUT_TOKENS", "512")
	t.Setenv("SEARCH_GEMINI_API_KEY", "")
	if _, err = Load("../../config.yaml", "compare-planners"); err == nil {
		t.Fatal("missing key accepted")
	}
}
