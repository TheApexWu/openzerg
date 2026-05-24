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
	// DefaultAttackerTimeoutSeconds is the HARD per-pod wall-clock budget.
	// The entrypoint runs pi under `timeout`; the pod also gets
	// activeDeadlineSeconds = this + 30 as a kubelet-level backstop. The
	// model is killed mid-tool-call if it goes past, so this should be
	// generous. 0 = unlimited.
	DefaultAttackerTimeoutSeconds = 600
	// DefaultAttackerSoftTimeoutSeconds is the SOFT per-pod budget the
	// model is supposed to aim for. The attacker skill's time_check.sh
	// helper returns WARN/EXPIRING as this deadline approaches, so the
	// agent voluntarily wraps up and emits its final JSON result line.
	// Going past it is fine; nothing kills the model. 0 = no soft target.
	DefaultAttackerSoftTimeoutSeconds = 60
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
	// AttackerTimeoutSeconds is the HARD per-pod wall-clock budget set as
	// TIMEOUT_SECONDS on each pi-attacker pod. 0 = unlimited. The model
	// gets SIGTERM if it goes past this; the pod's activeDeadlineSeconds
	// is also set to this + 30.
	AttackerTimeoutSeconds int
	// AttackerSoftTimeoutSeconds is the SOFT per-pod target the agent
	// aims for, set as SOFT_TIMEOUT_SECONDS on the pod. Surfaced to the
	// model by the skill's time_check.sh helper so it wraps up early.
	// Nothing kills the model when it elapses. 0 = no soft target.
	AttackerSoftTimeoutSeconds int
	KubeconfigPath string
	EnvFilePath    string
	// DisableNimble removes NIMBLE_API_KEY from pod env and sets
	// OPENZERG_DISABLE_NIMBLE=1 so the in-pod tool wrapper short-circuits.
	// This is the demo-time kill switch if Nimble has a bad day.
	DisableNimble bool
	// EnableCVESeed turns on the Nimble-driven startup CVE search that
	// rewrites one Gen-1 genome's hint with a fresh snippet. Off by
	// default for demo determinism.
	EnableCVESeed bool
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
	fs.IntVar(&cfg.AttackerTimeoutSeconds, "timeout-seconds", cfg.AttackerTimeoutSeconds, "HARD per-pod wall-clock budget in seconds, 0=unlimited (env TIMEOUT_SECONDS)")
	fs.IntVar(&cfg.AttackerSoftTimeoutSeconds, "soft-timeout-seconds", cfg.AttackerSoftTimeoutSeconds, "SOFT per-pod target the agent aims for, 0=no target (env SOFT_TIMEOUT_SECONDS)")
	fs.StringVar(&cfg.KubeconfigPath, "kubeconfig", cfg.KubeconfigPath, "kubeconfig path (env KUBECONFIG)")
	fs.BoolVar(&cfg.DisableNimble, "disable-nimble", cfg.DisableNimble, "kill-switch: omit NIMBLE_API_KEY from pods and disable the in-pod nimble_fetch wrapper")
	fs.BoolVar(&cfg.EnableCVESeed, "enable-cve-seed", cfg.EnableCVESeed, "use Nimble /v1/search at startup to seed one Gen-1 genome hint with a fresh CVE snippet")

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
		AttackerTimeoutSeconds:     getenvInt("TIMEOUT_SECONDS", DefaultAttackerTimeoutSeconds),
		AttackerSoftTimeoutSeconds: getenvInt("SOFT_TIMEOUT_SECONDS", DefaultAttackerSoftTimeoutSeconds),
		KubeconfigPath: ResolveKubeconfigPath(),
		DisableNimble:  getenvBool("OPENZERG_DISABLE_NIMBLE", false),
		EnableCVESeed:  getenvBool("OPENZERG_ENABLE_CVE_SEED", false),
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

// DefaultRuntimeConfig returns a RuntimeConfig seeded purely from env and
// hard-coded defaults. The HTTP API uses this as the base when assembling a
// config from a POST /api/runs body (which is JSON, not CLI flags).
func DefaultRuntimeConfig() RuntimeConfig { return defaultRuntime() }

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
