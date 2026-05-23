// Package config holds the runtime configuration for the openzerg control
// plane. It merges CLI flags with process environment variables (flags win)
// and produces a fully-resolved RuntimeConfig that the rest of the binary
// consumes. Flag names mirror PRD.json components[0].flags.
package config

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
)

// Defaults mirror PRD.json. Keep them in sync.
const (
	DefaultGenerations = 4
	DefaultPopulation  = 15
	DefaultNamespace   = "openzerg"
	DefaultImage       = "registry.digitalocean.com/openzerg/pi-attacker:latest"
	DefaultOutDir      = "./out"
	DefaultRateLimit   = 60
)

// RuntimeConfig is the merged view of flags + env that the run subcommand
// consumes. It is also useful for `doctor`, which prints a subset.
type RuntimeConfig struct {
	TargetURL      string
	Generations    int
	Population     int
	Namespace      string
	AttackerImage  string
	DryRun         bool
	OutDir         string
	RateLimitRPS   int
	KubeconfigPath string
	EnvFilePath    string
}

// ParseRunFlags parses the flags accepted by `openzerg run`. The supplied
// args slice should not include the subcommand name. Output (errors, -h) is
// written to out so tests can capture it.
func ParseRunFlags(args []string, out io.Writer) (RuntimeConfig, error) {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(out)
	cfg := defaultRuntime()

	fs.StringVar(&cfg.TargetURL, "target", cfg.TargetURL, "target URL to attack (env TARGET_URL)")
	fs.IntVar(&cfg.Generations, "generations", cfg.Generations, "max generations (env MAX_GENERATIONS)")
	fs.IntVar(&cfg.Population, "population", cfg.Population, "pods per generation (env POPULATION_SIZE)")
	fs.StringVar(&cfg.Namespace, "namespace", cfg.Namespace, "kubernetes namespace (env K8S_NAMESPACE)")
	fs.StringVar(&cfg.AttackerImage, "image", cfg.AttackerImage, "attacker image ref (env ATTACKER_IMAGE)")
	fs.BoolVar(&cfg.DryRun, "dry-run", cfg.DryRun, "plan but do not create pods (env OPENZERG_DRY_RUN)")
	fs.StringVar(&cfg.OutDir, "out-dir", cfg.OutDir, "output directory (env OPENZERG_OUT_DIR)")
	fs.IntVar(&cfg.RateLimitRPS, "rate-limit", cfg.RateLimitRPS, "aggregate req/s ceiling (env RATE_LIMIT_RPS)")
	fs.StringVar(&cfg.KubeconfigPath, "kubeconfig", cfg.KubeconfigPath, "kubeconfig path (env KUBECONFIG)")

	if err := fs.Parse(args); err != nil {
		return cfg, err
	}
	if cfg.TargetURL == "" {
		return cfg, fmt.Errorf("--target (or env TARGET_URL) is required")
	}
	return cfg, nil
}

// defaultRuntime seeds a RuntimeConfig from env so that explicit flags can
// then override env. This implements the documented precedence: flag > env >
// hard-coded default.
func defaultRuntime() RuntimeConfig {
	cfg := RuntimeConfig{
		TargetURL:      getenv("TARGET_URL", ""),
		Generations:    getenvInt("MAX_GENERATIONS", DefaultGenerations),
		Population:     getenvInt("POPULATION_SIZE", DefaultPopulation),
		Namespace:      getenv("K8S_NAMESPACE", DefaultNamespace),
		AttackerImage:  getenv("ATTACKER_IMAGE", DefaultImage),
		DryRun:         getenvBool("OPENZERG_DRY_RUN", false),
		OutDir:         getenv("OPENZERG_OUT_DIR", DefaultOutDir),
		RateLimitRPS:   getenvInt("RATE_LIMIT_RPS", DefaultRateLimit),
		KubeconfigPath: ResolveKubeconfigPath(),
	}
	return cfg
}

// ResolveKubeconfigPath returns the kubeconfig path as resolved by the same
// rules client-go uses: $KUBECONFIG if set, else $HOME/.kube/config.
func ResolveKubeconfigPath() string {
	if v := os.Getenv("KUBECONFIG"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".kube", "config")
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func getenvBool(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return fallback
}
