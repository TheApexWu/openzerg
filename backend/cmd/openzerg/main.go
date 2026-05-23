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
	"math/rand"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/TheApexWu/openzerg/backend/internal/attacks"
	"github.com/TheApexWu/openzerg/backend/internal/config"
	"github.com/TheApexWu/openzerg/backend/internal/evolve"
	"github.com/TheApexWu/openzerg/backend/internal/k8s"
	"github.com/TheApexWu/openzerg/backend/internal/openrouter"
	"github.com/TheApexWu/openzerg/backend/internal/secrets"
	"github.com/TheApexWu/openzerg/backend/internal/spawn"
	"github.com/TheApexWu/openzerg/backend/internal/store"
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

// preferredOpenRouterModel is the model id the control plane uses for the
// optional LLM-mutation step. Paid Gemma 4 only — free variants are banned
// by RALPH_README.
const preferredOpenRouterModel = "google/gemma-4-26b-a4b-it"

// llmMutationCallBudget caps total OpenRouter requests across one run.
// Matches integrations.openrouter.budget_guardrail in PRD.json.
const llmMutationCallBudget = 32

// survivorFitnessThreshold and survivorCap mirror evolution_loop in PRD.json.
const (
	survivorFitnessThreshold = 0.1
	survivorCap              = 7
)

// runSwarm is the cluster-touching path of `openzerg run`. It runs the full
// N-generation evolutionary loop: spawn pods, score results, pick survivors,
// mutate, repeat. Stops on first fitness=1.0 OR after --generations,
// whichever comes first. SIGINT writes a partial summary and returns nil.
func runSwarm(stdout, stderr io.Writer, cfg config.RuntimeConfig) error {
	cr, err := k8s.BuildClientset(cfg.KubeconfigPath)
	if err != nil {
		return fmt.Errorf("run: build clientset: %w", err)
	}
	fmt.Fprintln(stdout, "")
	fmt.Fprintf(stdout, "kube client: ready (in-cluster=%t)\n", cr.InCluster)

	runID := fmt.Sprintf("r%d", time.Now().Unix())
	runStore := store.NewRunStore(runID, cfg.TargetURL)

	// Optional OpenRouter client for LLM mutation. Missing key falls back
	// to pure-Go mutation; we never gate the run on it.
	openRouterClient := buildOpenRouterClientIfAvailable(stdout)
	llmBudget := &evolve.LLMMutationBudget{Remaining: llmMutationCallBudget}

	rootContext, cancelRoot := context.WithCancel(context.Background())
	defer cancelRoot()
	installSignalHandler(rootContext, cancelRoot, stdout, runStore)

	currentPopulation := attacks.PickSeedGenomes(cfg.Population)
	randomGenerator := rand.New(rand.NewSource(time.Now().UnixNano()))

	for generationNumber := 1; generationNumber <= cfg.Generations; generationNumber++ {
		if rootContext.Err() != nil {
			runStore.Cancelled = true
			break
		}
		fmt.Fprintf(stdout, "\n=== generation %d/%d: spawning %d pods ===\n",
			generationNumber, cfg.Generations, len(currentPopulation))

		scored, gerr := runOneGeneration(rootContext, cr.Clientset, stdout, cfg, runID, generationNumber, currentPopulation)
		if gerr != nil && rootContext.Err() == nil {
			return fmt.Errorf("run: generation %d: %w", generationNumber, gerr)
		}
		runStore.RecordGeneration(generationNumber, scored)

		if runStore.Breach != nil {
			fmt.Fprintf(stdout, "\nBREACH detected in generation %d. Stopping.\n", generationNumber)
			break
		}
		if generationNumber == cfg.Generations {
			break
		}

		survivors := evolve.PickSurvivors(scored, survivorFitnessThreshold, survivorCap)
		fmt.Fprintf(stdout, "survivors: %d (threshold %.2f, cap %d)\n",
			len(survivors), survivorFitnessThreshold, survivorCap)

		currentPopulation = nextGenerationFromSurvivors(
			rootContext, openRouterClient, llmBudget,
			survivors, cfg.Population, cfg.TargetURL, randomGenerator,
			stdout,
		)
	}

	if rootContext.Err() != nil {
		runStore.Cancelled = true
	}
	runStore.Finalize()
	jsonPath, mdPath, werr := runStore.WriteArtifacts(cfg.OutDir)
	if werr != nil {
		fmt.Fprintf(stderr, "summary write error: %v\n", werr)
	} else {
		fmt.Fprintf(stdout, "\nsummary: %s\n         %s\n", jsonPath, mdPath)
	}
	fmt.Fprintf(stdout, "outcome: %s (best fitness %.2f)\n", runStore.Outcome, runStore.BestFitness)
	fmt.Fprintln(stdout, "run: ok")
	return nil
}

// runOneGeneration builds N pods from currentPopulation, fans them out via
// spawn.RunPods, prints per-pod outcomes, and returns the scored result for
// each. Pod build failures become a fitness-0.0 ScoredGenome so the loop
// stays the same shape.
func runOneGeneration(
	ctx context.Context,
	clientset kubernetes.Interface,
	stdout io.Writer,
	cfg config.RuntimeConfig,
	runID string,
	generationNumber int,
	population []attacks.Genome,
) ([]evolve.ScoredGenome, error) {
	pods := make([]*corev1.Pod, 0, len(population))
	podIDs := make([]string, 0, len(population))
	for podIndex, genome := range population {
		podID := fmt.Sprintf("%s-g%d-p%d", runID, generationNumber, podIndex)
		pod, perr := spawn.BuildAttackerPod(spawn.AttackerPodOptions{
			Name:           fmt.Sprintf("openzerg-attacker-%s-g%d-p%d", runID, generationNumber, podIndex),
			Namespace:      cfg.Namespace,
			Image:          cfg.AttackerImage,
			Genome:         genome,
			RunID:          runID,
			PodID:          podID,
			Generation:     generationNumber,
			TargetURL:      cfg.TargetURL,
			RateLimitRPS:   cfg.RateLimitRPS,
			TimeoutSeconds: 60,
		})
		if perr != nil {
			return nil, fmt.Errorf("build pod %d: %w", podIndex, perr)
		}
		pods = append(pods, pod)
		podIDs = append(podIDs, podID)
	}

	generationContext, cancelGeneration := context.WithTimeout(ctx, 5*time.Minute)
	defer cancelGeneration()
	outcomes, err := spawn.RunPods(generationContext, clientset, pods)
	if err != nil {
		return nil, fmt.Errorf("RunPods: %w", err)
	}

	scored := make([]evolve.ScoredGenome, 0, len(outcomes))
	for _, outcome := range outcomes {
		result := evolve.Result{}
		podID := ""
		if outcome.Index < len(podIDs) {
			podID = podIDs[outcome.Index]
		}
		genome := population[outcome.Index]
		if outcome.Err != nil {
			fmt.Fprintf(stdout, "[gen %d pod %d] ERROR %v\n", generationNumber, outcome.Index, outcome.Err)
			result.Status = "ERROR"
			result.Evidence = outcome.Err.Error()
		} else if outcome.Result == nil || len(outcome.Result.RawLine) == 0 {
			parseErr := error(nil)
			if outcome.Result != nil {
				parseErr = outcome.Result.ParseError
			}
			fmt.Fprintf(stdout, "[gen %d pod %d] no result line (parse=%v)\n", generationNumber, outcome.Index, parseErr)
			result.Status = "ERROR"
			result.Evidence = "no JSON result line"
		} else {
			if err := json.Unmarshal(outcome.Result.RawLine, &result); err != nil {
				fmt.Fprintf(stdout, "[gen %d pod %d] decode error: %v raw=%s\n",
					generationNumber, outcome.Index, err, string(outcome.Result.RawLine))
				result.Status = "ERROR"
				result.Evidence = "result line decode failed"
			}
			fmt.Fprintf(stdout, "[gen %d pod %d] %s\n", generationNumber, outcome.Index, string(outcome.Result.RawLine))
		}
		fitness := evolve.Score(result)
		fmt.Fprintf(stdout, "[gen %d pod %d] fitness=%.2f vector=%s status=%s\n",
			generationNumber, outcome.Index, fitness, genome.Vector, result.Status)
		scored = append(scored, evolve.ScoredGenome{
			Genome:  genome,
			Result:  result,
			Fitness: fitness,
			PodID:   podID,
		})
	}
	return scored, nil
}

// nextGenerationFromSurvivors builds the next generation's genome list. If
// an OpenRouter client is available and the budget allows, it asks Gemma 4
// for half the slots; the rest comes from pure-Go Mutate. Any LLM failure
// falls back silently to all-Mutate.
func nextGenerationFromSurvivors(
	ctx context.Context,
	openRouterClient *openrouter.Client,
	llmBudget *evolve.LLMMutationBudget,
	survivors []attacks.Genome,
	populationSize int,
	targetURL string,
	random *rand.Rand,
	stdout io.Writer,
) []attacks.Genome {
	if openRouterClient != nil && llmBudget.Remaining > 0 && len(survivors) > 0 {
		llmCount := populationSize / 2
		llmContext, llmCancel := context.WithTimeout(ctx, 30*time.Second)
		llmGenomes, llmErr := evolve.MutateLLM(llmContext, openRouterClient,
			preferredOpenRouterModel, survivors, llmCount, targetURL, llmBudget)
		llmCancel()
		if llmErr != nil {
			fmt.Fprintf(stdout, "llm-mutation: skipped (%v); falling back to pure-Go\n", llmErr)
			return evolve.Mutate(evolve.MutationContext{
				Survivors: survivors, PopulationSize: populationSize, Random: random,
			})
		}
		fmt.Fprintf(stdout, "llm-mutation: produced %d genomes (budget remaining: %d)\n",
			len(llmGenomes), llmBudget.Remaining)
		// Top up the remainder with pure-Go mutations so we always hit
		// populationSize exactly.
		remainder := evolve.Mutate(evolve.MutationContext{
			Survivors: survivors, PopulationSize: populationSize - len(llmGenomes), Random: random,
		})
		combined := append(llmGenomes, remainder...)
		if len(combined) > populationSize {
			combined = combined[:populationSize]
		}
		return combined
	}
	return evolve.Mutate(evolve.MutationContext{
		Survivors: survivors, PopulationSize: populationSize, Random: random,
	})
}

// buildOpenRouterClientIfAvailable returns a configured client if the env
// has OPENROUTER_API_KEY, else nil. Missing key is logged but never fatal.
func buildOpenRouterClientIfAvailable(stdout io.Writer) *openrouter.Client {
	envFilePath := defaultEnvPath()
	cfg, err := secrets.Load(envFilePath)
	if err != nil {
		fmt.Fprintf(stdout, "secrets: load error (%v); LLM mutation disabled\n", err)
		return nil
	}
	if !cfg.HasOpenRouter() {
		fmt.Fprintln(stdout, "OPENROUTER_API_KEY missing; LLM mutation disabled (pure-Go only)")
		return nil
	}
	return openrouter.New(cfg.OpenRouterAPIKey)
}

// installSignalHandler wires SIGINT / SIGTERM to cancel the root context so
// the generation loop exits early. We still finalize and write a partial
// summary at the end of runSwarm.
func installSignalHandler(ctx context.Context, cancel context.CancelFunc, stdout io.Writer, runStore *store.RunStore) {
	signalChannel := make(chan os.Signal, 1)
	signal.Notify(signalChannel, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case sig := <-signalChannel:
			fmt.Fprintf(stdout, "\nreceived %s; cancelling run...\n", sig)
			runStore.Cancelled = true
			cancel()
		case <-ctx.Done():
		}
	}()
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
