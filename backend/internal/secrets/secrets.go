// Package secrets loads API keys from an optional .env file and the process
// environment. The loader is intentionally tiny and dependency-free: it reads
// KEY=VALUE lines, ignores comments and blanks, and lets process env override
// .env values so a developer can shadow a checked-in default with an export.
package secrets

import (
	"bufio"
	"errors"
	"io/fs"
	"os"
	"strings"
)

// Config is the typed view of the secrets the openzerg control plane and its
// pods need. All fields are optional at load time; callers gate live calls on
// presence per integration.
type Config struct {
	// OpenRouterAPIKey is the bearer token for openrouter.ai. Used by the
	// optional control-plane LLM mutation step and propagated to attacker
	// pods via a k8s Secret.
	OpenRouterAPIKey string

	// NimbleAPIKey is the auth token for nimbleway.com. Used inside
	// attacker pods for JS-rendered page fetches and optionally by the
	// control plane for CVE seeding.
	NimbleAPIKey string

	// EnvFilePath records the .env path that was actually read, or "" if
	// no file was found. doctor reports this for transparency.
	EnvFilePath string
}

// HasOpenRouter reports whether an OpenRouter API key was loaded.
func (c Config) HasOpenRouter() bool { return c.OpenRouterAPIKey != "" }

// HasNimble reports whether a Nimble API key was loaded.
func (c Config) HasNimble() bool { return c.NimbleAPIKey != "" }

// Load reads .env from envPath if it exists, then overlays process env, and
// returns a Config. A missing .env is not an error; any other read or parse
// failure is. Process env always wins over .env so a shell export can shadow
// a checked-in default during development.
func Load(envPath string) (Config, error) {
	cfg := Config{}
	values, err := readDotEnv(envPath)
	if err != nil {
		return cfg, err
	}
	if values != nil {
		cfg.EnvFilePath = envPath
	}
	// Layer 1: .env file values.
	openRouter := values["OPENROUTER_API_KEY"]
	nimble := values["NIMBLE_API_KEY"]
	// Layer 2: process env wins.
	if v := os.Getenv("OPENROUTER_API_KEY"); v != "" {
		openRouter = v
	}
	if v := os.Getenv("NIMBLE_API_KEY"); v != "" {
		nimble = v
	}
	cfg.OpenRouterAPIKey = openRouter
	cfg.NimbleAPIKey = nimble
	return cfg, nil
}

// readDotEnv parses a tiny subset of the dotenv format: KEY=VALUE lines, with
// optional surrounding whitespace, and # for line comments. Quoted values are
// stripped of a single matching pair of single or double quotes. A missing
// file returns (nil, nil); any other I/O error is propagated.
func readDotEnv(path string) (map[string]string, error) {
	if path == "" {
		return nil, nil
	}
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	out := map[string]string{}
	sc := bufio.NewScanner(f)
	// Allow long-ish lines without ballooning memory.
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Strip optional `export ` prefix.
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(line[len("export "):])
		}
		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		// Drop trailing inline comment that begins with " #" (space-hash).
		if i := strings.Index(val, " #"); i >= 0 {
			val = strings.TrimSpace(val[:i])
		}
		val = unquote(val)
		out[key] = val
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// unquote removes a single matching pair of surrounding single or double
// quotes. Unmatched or absent quotes return the input unchanged.
func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
