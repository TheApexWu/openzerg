// Package runner contains the evolutionary swarm orchestration that both
// the `openzerg run` CLI subcommand and the `openzerg serve` HTTP API call
// into. It was extracted out of cmd/openzerg/main.go in M7 so the API
// surface (internal/api) could trigger a run without duplicating the logic.
//
// A Runner is single-use: construct it for one run, call Run, then discard.
// Progress is reported through an optional events.Broker so the API can
// fan that out via SSE. The CLI also wires a broker (so the JSON event tail
// is identical in both modes), but typically no subscribers are attached.
package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	neturl "net/url"
	"os/signal"
	"strings"
	"syscall"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/TheApexWu/openzerg/backend/internal/attacks"
	"github.com/TheApexWu/openzerg/backend/internal/config"
	"github.com/TheApexWu/openzerg/backend/internal/events"
	"github.com/TheApexWu/openzerg/backend/internal/evolve"
	"github.com/TheApexWu/openzerg/backend/internal/k8s"
	"github.com/TheApexWu/openzerg/backend/internal/nimble"
	"github.com/TheApexWu/openzerg/backend/internal/openrouter"
	"github.com/TheApexWu/openzerg/backend/internal/secrets"
	"github.com/TheApexWu/openzerg/backend/internal/spawn"
	"github.com/TheApexWu/openzerg/backend/internal/store"
)

// PreferredOpenRouterModel mirrors the constant the CLI used before the
// extract. Paid Gemma 4 only.
const PreferredOpenRouterModel = "google/gemma-4-26b-a4b-it"

// LLMMutationCallBudget caps total OpenRouter requests across one run.
const LLMMutationCallBudget = 32

const (
	survivorFitnessThreshold = 0.1
	survivorCap              = 7
)

// Runner orchestrates one swarm run.
type Runner struct {
	Cfg           config.RuntimeConfig
	EnvFilePath   string
	Stdout        io.Writer
	Stderr        io.Writer
	Broker        *events.Broker
	InstallSignal bool // true for CLI; false for API (cancel comes via ctx)
}

// Run executes the full evolutionary loop and returns the populated store
// plus the on-disk artifact paths. ctx cancellation produces a partial
// summary; the function still returns nil error in that case.
func (r *Runner) Run(ctx context.Context) (*store.RunStore, string, string, error) {
	cr, err := k8s.BuildClientset(r.Cfg.KubeconfigPath)
	if err != nil {
		return nil, "", "", fmt.Errorf("runner: build clientset: %w", err)
	}
	fmt.Fprintf(r.Stdout, "kube client: ready (in-cluster=%t)\n", cr.InCluster)

	ns := r.Cfg.Namespace
	if ns == "" {
		ns = "openzerg"
	}
	_, err = k8s.EnsureNamespace(ctx, cr.Clientset, ns)
	if err != nil {
		fmt.Fprintf(r.Stderr, "warning: ensure namespace %q: %v\n", ns, err)
	}

	secCfg, err := secrets.Load(r.EnvFilePath)
	if err == nil {
		secData := make(map[string]string)
		if secCfg.OpenRouterAPIKey != "" {
			secData["OPENROUTER_API_KEY"] = secCfg.OpenRouterAPIKey
		}
		if secCfg.NimbleAPIKey != "" {
			secData["NIMBLE_API_KEY"] = secCfg.NimbleAPIKey
		}
		if len(secData) > 0 {
			err = k8s.EnsureSecret(ctx, cr.Clientset, ns, "openzerg-keys", secData)
			if err != nil {
				fmt.Fprintf(r.Stderr, "warning: ensure openzerg-keys secret: %v\n", err)
			} else {
				fmt.Fprintf(r.Stdout, "secret: openzerg-keys is synchronized in namespace %q\n", ns)
			}
		}
	} else {
		fmt.Fprintf(r.Stderr, "warning: load secrets for k8s sync: %v\n", err)
	}

	runID := fmt.Sprintf("r%d", time.Now().Unix())
	runStore := store.NewRunStore(runID, r.Cfg.TargetURL)
	runStore.NimbleEnabled = !r.Cfg.DisableNimble

	openRouterClient := r.buildOpenRouterClientIfAvailable()
	llmBudget := &evolve.LLMMutationBudget{Remaining: LLMMutationCallBudget}

	rootContext, cancelRoot := context.WithCancel(ctx)
	defer cancelRoot()
	if r.InstallSignal {
		stopSignal := r.installSignalHandler(rootContext, cancelRoot, runStore)
		defer stopSignal()
	}

	var currentPopulation []attacks.Genome
	if r.Cfg.DisableNimble {
		currentPopulation = attacks.PickSeedGenomes(r.Cfg.Population)
	} else {
		currentPopulation = attacks.PickSeedGenomesEnsuringNimble(r.Cfg.Population)
	}
	if r.Cfg.EnableCVESeed && !r.Cfg.DisableNimble {
		currentPopulation = r.applyCVESeedHint(currentPopulation)
	}
	randomGenerator := rand.New(rand.NewSource(time.Now().UnixNano()))

	// discoveredPaths accumulates real, same-origin paths that recon (and
	// other) pods report finding on the target. It is threaded into mutation
	// so later generations aim at routes that actually exist on THIS site,
	// keeping the swarm generic instead of guessing fixed API paths.
	discoveredPaths := newPathSet()

	r.publish("run_start", runID, map[string]any{
		"target_url":     r.Cfg.TargetURL,
		"population":     r.Cfg.Population,
		"generations":    r.Cfg.Generations,
		"disable_nimble": r.Cfg.DisableNimble,
		"enable_cve":     r.Cfg.EnableCVESeed,
	})

	for generationNumber := 1; generationNumber <= r.Cfg.Generations; generationNumber++ {
		if rootContext.Err() != nil {
			runStore.Cancelled = true
			break
		}
		fmt.Fprintf(r.Stdout, "\n=== generation %d/%d: spawning %d pods ===\n",
			generationNumber, r.Cfg.Generations, len(currentPopulation))
		r.publish("generation_start", runID, map[string]any{
			"generation": generationNumber,
			"population": len(currentPopulation),
		})

		scored, gerr := r.runOneGeneration(rootContext, cr.Clientset, runID, generationNumber, currentPopulation)
		if gerr != nil && rootContext.Err() == nil {
			return runStore, "", "", fmt.Errorf("runner: generation %d: %w", generationNumber, gerr)
		}
		runStore.RecordGeneration(generationNumber, scored)
		discoveredPaths.addFromScored(scored)
		r.publishGenerationEnd(runID, generationNumber, scored)

		if runStore.Breach != nil {
			fmt.Fprintf(r.Stdout, "\nBREACH detected in generation %d. Stopping.\n", generationNumber)
			r.publish("breach", runID, map[string]any{
				"generation": runStore.Breach.Generation,
				"pod_id":     runStore.Breach.PodID,
				"vector":     runStore.Breach.Vector,
				"evidence":   runStore.Breach.Evidence,
			})
			break
		}
		if generationNumber == r.Cfg.Generations {
			break
		}

		survivors := evolve.PickSurvivors(scored, survivorFitnessThreshold, survivorCap)
		fmt.Fprintf(r.Stdout, "survivors: %d (threshold %.2f, cap %d)\n",
			len(survivors), survivorFitnessThreshold, survivorCap)

		currentPopulation = r.nextGenerationFromSurvivors(
			rootContext, openRouterClient, llmBudget,
			survivors, r.Cfg.Population, r.Cfg.TargetURL, randomGenerator, runID, generationNumber,
			discoveredPaths.slice(),
		)

		// Anti-convergence: always reserve ~30% of the next generation for
		// fresh, diverse seed genomes aimed at under-explored vuln classes (and
		// discovered paths). Without this the swarm collapses onto whatever
		// single endpoint scored first (observed: 6 generations all probing
		// /admin). Diversity keeps other vuln classes in play.
		diversityReserve := r.Cfg.Population * 3 / 10
		if diversityReserve < 1 && r.Cfg.Population >= 3 {
			diversityReserve = 1
		}
		currentPopulation = evolve.InjectFreshDiversity(
			currentPopulation, diversityReserve, discoveredPaths.slice(), randomGenerator,
		)

		// RECON-DRIVEN SEEDING: replace a slice of the next generation with
		// attacker genomes aimed at the real endpoints recon discovered, using
		// the technique most likely to pay off at each path (e.g. an SSRF genome
		// at a /fetch-url path, GraphQL genomes at /graphql, JWT genomes at an
		// auth endpoint). This is what makes the swarm act on recon instead of
		// re-guessing fixed routes — the key generalization fix.
		//
		// This MUST be applied LAST and with a GUARANTEED reserve. We previously
		// saw the LLM mutator over-converge (it bred 7 near-identical CORS
		// variants from one gen-1 CORS hit) and crowd the recon-seeded JWT
		// genomes out of the population entirely, so a JWT benchmark whose
		// /api/auth/login recon found never spawned a JWT vector. Reserving ~40%
		// of the population for recon seeds — overwriting the tail, which holds
		// the most-mutated / least-elite entries — guarantees the right vectors
		// reach the discovered endpoints regardless of LLM behaviour.
		reconReserve := r.Cfg.Population * 2 / 5 // ~40%
		reconSeeds := evolve.SeedFromRecon(discoveredPaths.slice(), reconReserve)
		for i, seed := range reconSeeds {
			idx := len(currentPopulation) - 1 - i
			if idx < 0 || idx >= len(currentPopulation) {
				break
			}
			currentPopulation[idx] = seed
		}
		if len(reconSeeds) > 0 {
			// Log the distinct vectors seeded so training triage can confirm the
			// right classes (e.g. JWT for an auth endpoint) actually made the cut.
			seededVectors := map[string]struct{}{}
			for _, s := range reconSeeds {
				seededVectors[s.Vector] = struct{}{}
			}
			vecList := make([]string, 0, len(seededVectors))
			for v := range seededVectors {
				vecList = append(vecList, v)
			}
			fmt.Fprintf(r.Stdout, "recon-seeding: aimed %d genome(s) at discovered endpoints [vectors: %s]\n",
				len(reconSeeds), strings.Join(vecList, ","))
		}
	}

	if rootContext.Err() != nil {
		runStore.Cancelled = true
	}
	runStore.Finalize()
	jsonPath, mdPath, werr := runStore.WriteArtifacts(r.Cfg.OutDir)
	if werr != nil {
		fmt.Fprintf(r.Stderr, "summary write error: %v\n", werr)
	} else {
		fmt.Fprintf(r.Stdout, "\nsummary: %s\n         %s\n", jsonPath, mdPath)
	}
	fmt.Fprintf(r.Stdout, "outcome: %s (best fitness %.2f)\n", runStore.Outcome, runStore.BestFitness)
	fmt.Fprintln(r.Stdout, "run: ok")
	r.publish("run_end", runID, map[string]any{
		"outcome":      runStore.Outcome,
		"best_fitness": runStore.BestFitness,
		"cancelled":    runStore.Cancelled,
		"json_path":    jsonPath,
		"md_path":      mdPath,
	})
	return runStore, jsonPath, mdPath, nil
}

// runEmitter bridges spawn.RunPodsWithEmitter to the events broker.
type runEmitter struct {
	runner      *Runner
	runID       string
	generation  int
	podIDByIdx  []string
	vectorByIdx []string
}

func (e *runEmitter) OnPodSpawn(index int, pod *corev1.Pod) {
	podID := ""
	vector := ""
	if index < len(e.podIDByIdx) {
		podID = e.podIDByIdx[index]
	}
	if index < len(e.vectorByIdx) {
		vector = e.vectorByIdx[index]
	}
	e.runner.publish("pod_spawn", e.runID, map[string]any{
		"generation": e.generation,
		"index":      index,
		"pod_id":     podID,
		"pod_name":   pod.Name,
		"vector":     vector,
	})
}

func (e *runEmitter) OnPodResult(index int, result *spawn.PodResult, err error) {
	payload := map[string]any{
		"generation": e.generation,
		"index":      index,
	}
	if index < len(e.podIDByIdx) {
		payload["pod_id"] = e.podIDByIdx[index]
	}
	if err != nil {
		payload["error"] = err.Error()
	} else if result != nil && len(result.RawLine) > 0 {
		var rawResult evolve.Result
		if jerr := json.Unmarshal(result.RawLine, &rawResult); jerr == nil {
			payload["status"] = rawResult.Status
			payload["evidence"] = rawResult.Evidence
			payload["vector"] = rawResult.Vector
		}
		payload["raw"] = string(result.RawLine)
	}
	e.runner.publish("pod_result", e.runID, payload)
}

func (r *Runner) runOneGeneration(
	ctx context.Context,
	clientset kubernetes.Interface,
	runID string,
	generationNumber int,
	population []attacks.Genome,
) ([]evolve.ScoredGenome, error) {
	pods := make([]*corev1.Pod, 0, len(population))
	podIDs := make([]string, 0, len(population))
	vectors := make([]string, 0, len(population))
	for podIndex, genome := range population {
		podID := fmt.Sprintf("%s-g%d-p%d", runID, generationNumber, podIndex)
		pod, perr := spawn.BuildAttackerPod(spawn.AttackerPodOptions{
			Name:               fmt.Sprintf("openzerg-attacker-%s-g%d-p%d", runID, generationNumber, podIndex),
			Namespace:          r.Cfg.Namespace,
			Image:              r.Cfg.AttackerImage,
			Genome:             genome,
			RunID:              runID,
			PodID:              podID,
			Generation:         generationNumber,
			TargetURL:          r.Cfg.TargetURL,
			RateLimitRPS:       r.Cfg.RateLimitRPS,
			TimeoutSeconds:     r.Cfg.AttackerTimeoutSeconds,     // HARD wrapper budget
			SoftTimeoutSeconds: r.Cfg.AttackerSoftTimeoutSeconds, // SOFT target the agent aims for
			DisableNimble:      r.Cfg.DisableNimble,
		})
		if perr != nil {
			return nil, fmt.Errorf("build pod %d: %w", podIndex, perr)
		}
		pods = append(pods, pod)
		podIDs = append(podIDs, podID)
		vectors = append(vectors, genome.Vector)
	}

	emitter := &runEmitter{
		runner:      r,
		runID:       runID,
		generation:  generationNumber,
		podIDByIdx:  podIDs,
		vectorByIdx: vectors,
	}

	// No per-generation timeout: each pi-attacker pod runs under its own
	// soft TIMEOUT_SECONDS (set above; the entrypoint enforces it with
	// `timeout`, and the pod gets ActiveDeadlineSeconds = TIMEOUT_SECONDS+30
	// as a kubelet-level backstop -- see spawn.BuildAttackerPod).
	outcomes, err := spawn.RunPodsWithEmitter(ctx, clientset, pods, emitter)
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
			fmt.Fprintf(r.Stdout, "[gen %d pod %d] ERROR %v\n", generationNumber, outcome.Index, outcome.Err)
			result.Status = "ERROR"
			result.Evidence = outcome.Err.Error()
		} else if outcome.Result == nil || len(outcome.Result.RawLine) == 0 {
			parseErr := error(nil)
			if outcome.Result != nil {
				parseErr = outcome.Result.ParseError
			}
			fmt.Fprintf(r.Stdout, "[gen %d pod %d] no result line (parse=%v)\n", generationNumber, outcome.Index, parseErr)
			result.Status = "ERROR"
			result.Evidence = "no JSON result line"
		} else {
			if err := json.Unmarshal(outcome.Result.RawLine, &result); err != nil {
				fmt.Fprintf(r.Stdout, "[gen %d pod %d] decode error: %v raw=%s\n",
					generationNumber, outcome.Index, err, string(outcome.Result.RawLine))
				result.Status = "ERROR"
				result.Evidence = "result line decode failed"
			}
			fmt.Fprintf(r.Stdout, "[gen %d pod %d] %s\n", generationNumber, outcome.Index, string(outcome.Result.RawLine))
		}
		fitness := evolve.ScoreWithFlag(result, r.Cfg.ExpectedFlag)
		flagNote := ""
		if r.Cfg.ExpectedFlag != "" && evolve.ResultContainsFlag(result, r.Cfg.ExpectedFlag) {
			// The pod's result actually contains the ground-truth flag — a real
			// capture. Log it regardless of whether keyword scoring ALSO hit 1.0
			// (it often does, since the pod self-reports BREACH). The previous
			// `evolve.Score(result) < 1.0` guard hid genuine captures whenever
			// the keyword path independently scored 1.0, making the scoreboard
			// under-report. This is the authoritative success marker.
			flagNote = " (FLAG CAPTURED)"
		}
		// RECON-SURVIVAL FLOOR: a recon pod that actually mapped attack surface
		// (reported real discovered_paths) is the foundation the rest of the
		// swarm builds on — it tells later generations WHERE the real endpoints
		// are. The keyword scorer caps RECON status low and often at 0.0 when the
		// pod describes the surface in its own words, so such a pod would be
		// culled and its map lost. Give it a floor just above the survival
		// threshold so its lineage (and its discovered paths) carries forward.
		// This is generic: it rewards surface-mapping on ANY target.
		if fitness < 0.3 && genome.Category == "recon" && evolve.ResultDiscoveredPaths(result) > 0 {
			fitness = 0.3
			flagNote += " (recon-floor)"
		}
		fmt.Fprintf(r.Stdout, "[gen %d pod %d] fitness=%.2f vector=%s status=%s%s\n",
			generationNumber, outcome.Index, fitness, genome.Vector, result.Status, flagNote)
		scored = append(scored, evolve.ScoredGenome{
			Genome:  genome,
			Result:  result,
			Fitness: fitness,
			PodID:   podID,
		})
	}
	return scored, nil
}

func (r *Runner) nextGenerationFromSurvivors(
	ctx context.Context,
	openRouterClient *openrouter.Client,
	llmBudget *evolve.LLMMutationBudget,
	survivors []attacks.Genome,
	populationSize int,
	targetURL string,
	random *rand.Rand,
	runID string,
	currentGen int,
	discoveredPaths []string,
) []attacks.Genome {
	if openRouterClient != nil && llmBudget.Remaining > 0 && len(survivors) > 0 {
		llmCount := populationSize / 2
		// No explicit timeout: let the LLM run until the agent finishes or
		// the parent run context is cancelled.
		llmGenomes, llmErr := evolve.MutateLLM(ctx, openRouterClient,
			PreferredOpenRouterModel, survivors, llmCount, targetURL, llmBudget)
		if llmErr != nil {
			fmt.Fprintf(r.Stdout, "llm-mutation: skipped (%v); falling back to pure-Go\n", llmErr)
			r.publish("mutation", runID, map[string]any{
				"generation": currentGen,
				"source":     "go",
				"reason":     fmt.Sprintf("llm fallback: %v", llmErr),
			})
			return evolve.Mutate(evolve.MutationContext{
				Survivors: survivors, PopulationSize: populationSize, Random: random,
				DiscoveredPaths: discoveredPaths,
			})
		}
		fmt.Fprintf(r.Stdout, "llm-mutation: produced %d genomes (budget remaining: %d)\n",
			len(llmGenomes), llmBudget.Remaining)
		r.publish("mutation", runID, map[string]any{
			"generation":       currentGen,
			"source":           "llm",
			"llm_genomes":      len(llmGenomes),
			"budget_remaining": llmBudget.Remaining,
		})
		remainder := evolve.Mutate(evolve.MutationContext{
			Survivors: survivors, PopulationSize: populationSize - len(llmGenomes), Random: random,
			DiscoveredPaths: discoveredPaths,
		})
		combined := append(llmGenomes, remainder...)
		if len(combined) > populationSize {
			combined = combined[:populationSize]
		}
		return combined
	}
	r.publish("mutation", runID, map[string]any{
		"generation": currentGen,
		"source":     "go",
	})
	return evolve.Mutate(evolve.MutationContext{
		Survivors: survivors, PopulationSize: populationSize, Random: random,
		DiscoveredPaths: discoveredPaths,
	})
}

func (r *Runner) buildOpenRouterClientIfAvailable() *openrouter.Client {
	cfg, err := secrets.Load(r.EnvFilePath)
	if err != nil {
		fmt.Fprintf(r.Stdout, "secrets: load error (%v); LLM mutation disabled\n", err)
		return nil
	}
	if !cfg.HasOpenRouter() {
		fmt.Fprintln(r.Stdout, "OPENROUTER_API_KEY missing; LLM mutation disabled (pure-Go only)")
		return nil
	}
	return openrouter.New(cfg.OpenRouterAPIKey)
}

func (r *Runner) installSignalHandler(ctx context.Context, cancel context.CancelFunc, runStore *store.RunStore) func() {
	signalContext, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-signalContext.Done()
		if ctx.Err() == nil {
			fmt.Fprintln(r.Stdout, "\nreceived signal; cancelling run...")
			runStore.Cancelled = true
			cancel()
		}
	}()
	return stop
}

func (r *Runner) applyCVESeedHint(population []attacks.Genome) []attacks.Genome {
	if len(population) == 0 {
		return population
	}
	cfg, err := secrets.Load(r.EnvFilePath)
	if err != nil || !cfg.HasNimble() {
		fmt.Fprintln(r.Stdout, "cve-seed: NIMBLE_API_KEY missing; skipping")
		return population
	}
	nimbleClient := nimble.New(cfg.NimbleAPIKey)
	searchContext, cancelSearch := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelSearch()
	results, err := nimbleClient.SearchWeb(searchContext,
		fmt.Sprintf("%s CVE recent web application vulnerability", r.Cfg.TargetURL),
		nimble.SearchOptions{MaxResults: 3, SearchDepth: "lite"})
	if err != nil {
		fmt.Fprintf(r.Stdout, "cve-seed: search failed: %v\n", err)
		return population
	}
	if len(results) == 0 {
		fmt.Fprintln(r.Stdout, "cve-seed: no results")
		return population
	}
	top := results[0]
	fmt.Fprintf(r.Stdout, "cve-seed: %q -> %s\n", top.Title, top.URL)
	population[0].Hint = population[0].Hint + " [cve-seed: " + top.Title + " — " + top.Snippet + "]"
	return population
}

func (r *Runner) publish(eventType, runID string, payload any) {
	if r.Broker == nil {
		return
	}
	r.Broker.Publish(eventType, runID, payload)
}

func (r *Runner) publishGenerationEnd(runID string, generationNumber int, scored []evolve.ScoredGenome) {
	if r.Broker == nil {
		return
	}
	best := 0.0
	survivors := 0
	breaches := 0
	for _, sg := range scored {
		if sg.Fitness > best {
			best = sg.Fitness
		}
		if sg.Fitness > survivorFitnessThreshold {
			survivors++
		}
		if sg.Fitness >= 1.0 {
			breaches++
		}
	}
	r.Broker.Publish("generation_end", runID, map[string]any{
		"generation":   generationNumber,
		"population":   len(scored),
		"survivors":    survivors,
		"best_fitness": best,
		"breaches":     breaches,
	})
}

// pathSet accumulates unique same-origin paths discovered on the target
// across generations. It is deliberately generic: it harvests any URL/path
// strings a pod reports in its result (recon "discovered_paths"/"links"
// findings, or any raw_finding "url"), normalizes them to a path, and dedupes.
// These feed evolve.MutationContext.DiscoveredPaths so the swarm explores the
// target's real routes instead of a fixed guess list.
type pathSet struct {
	seen  map[string]struct{}
	order []string
}

func newPathSet() *pathSet { return &pathSet{seen: map[string]struct{}{}} }

func (p *pathSet) add(raw string) {
	path := normalizeToPath(raw)
	if path == "" {
		return
	}
	if _, ok := p.seen[path]; ok {
		return
	}
	p.seen[path] = struct{}{}
	p.order = append(p.order, path)
}

func (p *pathSet) slice() []string {
	out := make([]string, len(p.order))
	copy(out, p.order)
	return out
}

// addFromScored harvests candidate paths from every result in a generation.
// It looks at two places: a top-level recon convention where raw_findings
// entries carry a "discovered_paths" list, and the generic "url" field that
// every raw_finding may carry. Both are optional and best-effort.
func (p *pathSet) addFromScored(scored []evolve.ScoredGenome) {
	for _, sg := range scored {
		for _, finding := range sg.Result.RawFindings {
			if u, ok := finding["url"].(string); ok {
				p.add(u)
			}
			if list, ok := finding["discovered_paths"].([]any); ok {
				for _, item := range list {
					if s, ok := item.(string); ok {
						p.add(s)
					}
				}
			}
			if list, ok := finding["links"].([]any); ok {
				for _, item := range list {
					if s, ok := item.(string); ok {
						p.add(s)
					}
				}
			}
		}
	}
}

// normalizeToPath reduces a raw URL or path string to a clean same-origin
// path ("/foo/bar"), dropping scheme/host/query/fragment. It returns "" for
// anything it cannot interpret as a usable path so callers can skip it.
func normalizeToPath(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if u, err := neturl.Parse(raw); err == nil && u.Path != "" {
		return u.Path
	}
	if strings.HasPrefix(raw, "/") {
		if i := strings.IndexAny(raw, "?#"); i >= 0 {
			raw = raw[:i]
		}
		return raw
	}
	return ""
}
