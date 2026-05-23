// Package main is the entrypoint for the openzerg control-plane CLI.
//
// Subcommands:
//   - version: print build version and exit.
//   - doctor:  print env / kubeconfig / secret status; never mutate cluster.
//   - run:     run an evolution. M1 only supports --dry-run (planning print).
//
// Real cluster operations land in M2.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/TheApexWu/openzerg/backend/internal/config"
	"github.com/TheApexWu/openzerg/backend/internal/k8s"
	"github.com/TheApexWu/openzerg/backend/internal/secrets"
	"github.com/TheApexWu/openzerg/backend/internal/spawn"
)

// version is the build-time version string. It is overridden at link time via
// -ldflags "-X main.version=..." in CI; the default below is for local dev.
const version = "0.1.0-dev"

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		fmt.Fprintf(stdout, "openzerg %s\n", version)
		printUsage(stdout)
		return nil
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "version", "-v", "--version":
		fmt.Fprintf(stdout, "openzerg %s\n", version)
		return nil
	case "doctor":
		return cmdDoctor(rest, stdout)
	case "run":
		return cmdRun(rest, stdout, stderr)
	case "-h", "--help", "help":
		fmt.Fprintf(stdout, "openzerg %s\n", version)
		printUsage(stdout)
		return nil
	default:
		printUsage(stderr)
		return fmt.Errorf("unknown subcommand %q", cmd)
	}
}

func printUsage(w io.Writer) {
	fmt.Fprint(w, `usage: openzerg <command> [flags]

commands:
  run       run an evolutionary attack swarm against --target
  doctor    print env / kubeconfig / secret status (no side effects)
  version   print build version

run --help for full flag list.
`)
}

// cmdDoctor prints a multi-line, side-effect-free status report. It must
// exit 0 even when keys are missing; doctor is the diagnostic tool, not a
// gate.
func cmdDoctor(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(out)
	envPath := fs.String("env-file", defaultEnvPath(), "path to .env (optional)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	fmt.Fprintf(out, "openzerg doctor (version %s)\n", version)
	fmt.Fprintln(out, "----------------------------------------")

	// Secrets.
	cfg, err := secrets.Load(*envPath)
	if err != nil {
		fmt.Fprintf(out, "secrets:        ERROR loading %s: %v\n", *envPath, err)
	} else {
		if cfg.EnvFilePath != "" {
			fmt.Fprintf(out, "env file:       %s (loaded)\n", cfg.EnvFilePath)
		} else {
			fmt.Fprintf(out, "env file:       %s (not present; using process env only)\n", *envPath)
		}
		fmt.Fprintf(out, "OPENROUTER_API_KEY: %s\n", presence(cfg.HasOpenRouter()))
		fmt.Fprintf(out, "NIMBLE_API_KEY:     %s\n", presence(cfg.HasNimble()))
	}

	// Kubeconfig.
	kubePath := config.ResolveKubeconfigPath()
	st := k8s.ProbeKubeconfig(kubePath)
	fmt.Fprintf(out, "kubeconfig path:    %s\n", kubePath)
	fmt.Fprintf(out, "kubeconfig exists:  %s\n", yesno(st.Exists))
	if st.ParseError != nil {
		fmt.Fprintf(out, "kubeconfig parse:   ERROR %v\n", st.ParseError)
	}
	if st.Exists {
		ctx := st.CurrentContext
		if ctx == "" {
			ctx = "(none found)"
		}
		fmt.Fprintf(out, "current-context:    %s\n", ctx)
	}

	// Defaults the run subcommand will use.
	fmt.Fprintf(out, "default namespace:  %s\n", config.DefaultNamespace)
	fmt.Fprintf(out, "default image:      %s\n", config.DefaultImage)
	fmt.Fprintln(out, "doctor: ok")
	return nil
}

// cmdRun handles `openzerg run`. M1 only implements --dry-run, which prints
// the plan and exits. Real pod orchestration is M2.
func cmdRun(args []string, stdout, stderr io.Writer) error {
	cfg, err := config.ParseRunFlags(args, stderr)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "openzerg run (version %s)\n", version)
	fmt.Fprintln(stdout, "----------------------------------------")
	fmt.Fprintf(stdout, "target:        %s\n", cfg.TargetURL)
	fmt.Fprintf(stdout, "generations:   %d\n", cfg.Generations)
	fmt.Fprintf(stdout, "population:    %d per generation\n", cfg.Population)
	fmt.Fprintf(stdout, "namespace:     %s\n", cfg.Namespace)
	fmt.Fprintf(stdout, "image:         %s\n", cfg.AttackerImage)
	fmt.Fprintf(stdout, "rate limit:    %d req/s aggregate\n", cfg.RateLimitRPS)
	fmt.Fprintf(stdout, "out dir:       %s\n", cfg.OutDir)
	fmt.Fprintf(stdout, "kubeconfig:    %s\n", cfg.KubeconfigPath)

	if !cfg.DryRun {
		return runSwarm(stdout, stderr, cfg)
	}

	fmt.Fprintln(stdout, "")
	fmt.Fprintln(stdout, "DRY RUN: no pods will be created.")
	fmt.Fprintf(stdout, "Plan: spawn %d pods/generation x %d generations = %d total pod attempts.\n",
		cfg.Population, cfg.Generations, cfg.Population*cfg.Generations)
	fmt.Fprintln(stdout, "Per-pod spec preview:")
	fmt.Fprintf(stdout, "  image:                %s\n", cfg.AttackerImage)
	fmt.Fprintf(stdout, "  restartPolicy:        Never\n")
	fmt.Fprintf(stdout, "  activeDeadlineSeconds: 120\n")
	fmt.Fprintf(stdout, "  env.TARGET_URL:       %s\n", cfg.TargetURL)
	fmt.Fprintf(stdout, "  env.RATE_LIMIT_RPS:   %d\n", cfg.RateLimitRPS)
	fmt.Fprintln(stdout, "  env.GENOME:           <rendered per pod from seed list>")
	fmt.Fprintln(stdout, "  envFrom.secret:       openzerg-keys (OPENROUTER_API_KEY, NIMBLE_API_KEY)")
	fmt.Fprintln(stdout, "dry-run: ok")
	return nil
}

// defaultEnvPath returns the conventional .env location at the repo root,
// derived from the binary's working directory. It is only a default; doctor
// accepts --env-file to override.
func defaultEnvPath() string {
	wd, err := os.Getwd()
	if err != nil {
		return ".env"
	}
	// Try common ancestors so the binary is friendly when run from
	// backend/ during dev or from repo root.
	for _, candidate := range []string{
		filepath.Join(wd, ".env"),
		filepath.Join(wd, "..", ".env"),
	} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return filepath.Join(wd, ".env")
}

// runSwarm is the M2 cluster-touching path of `openzerg run`. It builds a
// clientset from the resolved kubeconfig, renders --population busybox stub
// pods (each emitting a unique synthetic result JSON), fans them out via
// spawn.RunPods, and prints each outcome. Generations are not yet looped:
// this is the smallest end-to-end "the swarm spawns and reports" wiring.
//
// Real PI attacker images and fitness scoring land in M3+. For now this is
// enough to verify the kube path against the live DO cluster.
func runSwarm(stdout, stderr io.Writer, cfg config.RuntimeConfig) error {
	cr, err := k8s.BuildClientset(cfg.KubeconfigPath)
	if err != nil {
		return fmt.Errorf("run: build clientset: %w", err)
	}
	fmt.Fprintln(stdout, "")
	fmt.Fprintf(stdout, "kube client: ready (in-cluster=%t)\n", cr.InCluster)
	fmt.Fprintf(stdout, "spawning %d stub pod(s) in namespace %q...\n", cfg.Population, cfg.Namespace)

	pods := make([]*corev1.Pod, 0, cfg.Population)
	runID := fmt.Sprintf("r%d", time.Now().Unix())
	for i := 0; i < cfg.Population; i++ {
		final := map[string]any{
			"type":        "result",
			"run_id":      runID,
			"pod_id":      fmt.Sprintf("%s-p%d", runID, i),
			"generation":  1,
			"vector":      "stub",
			"category":    "stub",
			"status":      "NOOP",
			"fitness":     0.0,
			"evidence":    "M2 busybox stub pod",
			"raw_findings": []any{},
			"duration_ms": 0,
			"t":           time.Now().UnixMilli(),
		}
		buf, _ := json.Marshal(final)
		p, perr := spawn.BuildBusyboxPod(spawn.PodOptions{
			Name:      fmt.Sprintf("openzerg-stub-%s-p%d", runID, i),
			Namespace: cfg.Namespace,
			FinalJSON: string(buf),
		})
		if perr != nil {
			return fmt.Errorf("run: build pod %d: %w", i, perr)
		}
		pods = append(pods, p)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	outcomes, err := spawn.RunPods(ctx, cr.Clientset, pods)
	if err != nil {
		return fmt.Errorf("run: spawn.RunPods: %w", err)
	}
	for _, o := range outcomes {
		if o.Err != nil {
			fmt.Fprintf(stdout, "[pod %d] ERROR %v\n", o.Index, o.Err)
			continue
		}
		if o.Result == nil || len(o.Result.RawLine) == 0 {
			fmt.Fprintf(stdout, "[pod %d] no result line (parse=%v)\n", o.Index, o.Result.ParseError)
			continue
		}
		fmt.Fprintf(stdout, "[pod %d] %s\n", o.Index, string(o.Result.RawLine))
	}
	fmt.Fprintln(stdout, "run: ok")
	return nil
}

func presence(ok bool) string {
	if ok {
		return "present"
	}
	return "missing"
}

func yesno(ok bool) string {
	if ok {
		return "yes"
	}
	return "no"
}
