package secrets

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingFileIsNotAnError(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "")
	t.Setenv("NIMBLE_API_KEY", "")
	cfg, err := Load(filepath.Join(t.TempDir(), "no-such.env"))
	if err != nil {
		t.Fatalf("expected nil error for missing .env, got %v", err)
	}
	if cfg.HasOpenRouter() || cfg.HasNimble() {
		t.Fatalf("expected empty config, got %+v", cfg)
	}
	if cfg.EnvFilePath != "" {
		t.Fatalf("expected empty EnvFilePath, got %q", cfg.EnvFilePath)
	}
}

func TestLoadParsesAndProcessEnvWins(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	contents := "" +
		"# a comment\n" +
		"OPENROUTER_API_KEY=from_file\n" +
		"NIMBLE_API_KEY=\"quoted_value\"\n" +
		"export NIMBLE_API_URL=http://example.invalid\n"
	if err := os.WriteFile(envPath, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OPENROUTER_API_KEY", "from_env")
	t.Setenv("NIMBLE_API_KEY", "")
	cfg, err := Load(envPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.OpenRouterAPIKey != "from_env" {
		t.Fatalf("process env should win; got %q", cfg.OpenRouterAPIKey)
	}
	if cfg.NimbleAPIKey != "quoted_value" {
		t.Fatalf("nimble should come from .env stripped of quotes; got %q", cfg.NimbleAPIKey)
	}
	if cfg.EnvFilePath != envPath {
		t.Fatalf("expected EnvFilePath=%q, got %q", envPath, cfg.EnvFilePath)
	}
}
