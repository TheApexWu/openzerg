package evolve

import (
	"math/rand"

	"github.com/TheApexWu/openzerg/backend/internal/attacks"
)

// MutationContext bundles the inputs Mutate needs from the surrounding
// generation: the surviving genomes (ordered best-first), the desired output
// population size, and a *rand.Rand the caller controls so tests can pin the
// seed.
type MutationContext struct {
	Survivors      []attacks.Genome
	PopulationSize int
	Random         *rand.Rand

	// DiscoveredPaths are real, same-origin paths the recon genome (or any
	// pod) reported finding on THIS target. When non-empty, path-varying
	// mutations prefer these over the static per-category guess pools, which
	// is how the swarm adapts to an arbitrary site instead of guessing API
	// routes. Leave nil/empty to fall back to the static pools.
	DiscoveredPaths []string
}

// Mutate produces the next generation by mixing three sources, in this order:
//  1. The survivors themselves, kept verbatim ("elitism"): the next gen
//     includes the parents so good solutions are not lost.
//  2. Targeted mutations of survivors (swap technique, vary target path,
//     occasional param shuffle).
//  3. Cross-bred children that take params from one survivor and
//     vector+technique from another.
//
// If there are no survivors at all, Mutate falls back to fresh seed genomes
// drawn from attacks.SeedGenomes, so the loop never starves.
//
// The returned slice has exactly PopulationSize entries. Every produced
// genome has its ParentPodID stamped with the source survivor's vector when
// applicable, so the summary can render lineage.
func Mutate(mutationContext MutationContext) []attacks.Genome {
	populationSize := mutationContext.PopulationSize
	if populationSize <= 0 {
		return nil
	}
	random := mutationContext.Random
	if random == nil {
		random = rand.New(rand.NewSource(1))
	}

	if len(mutationContext.Survivors) == 0 {
		return fallbackToSeedGenomes(populationSize)
	}

	output := make([]attacks.Genome, 0, populationSize)

	for _, survivor := range mutationContext.Survivors {
		if len(output) >= populationSize {
			break
		}
		output = append(output, survivor)
	}

	for len(output) < populationSize {
		parent := mutationContext.Survivors[random.Intn(len(mutationContext.Survivors))]
		strategyRoll := random.Intn(10)
		switch {
		case strategyRoll < 4:
			output = append(output, mutateBySwappingTechnique(parent, random))
		case strategyRoll < 7:
			output = append(output, mutateByVaryingTargetPath(parent, random, mutationContext.DiscoveredPaths))
		case strategyRoll < 9 && len(mutationContext.Survivors) >= 2:
			otherParent := mutationContext.Survivors[random.Intn(len(mutationContext.Survivors))]
			output = append(output, crossBreed(parent, otherParent))
		default:
			output = append(output, mutateByShufflingParams(parent, random))
		}
	}

	return output
}

func fallbackToSeedGenomes(populationSize int) []attacks.Genome {
	return attacks.PickSeedGenomes(populationSize)
}

// InjectFreshDiversity guarantees that at least `reserve` slots of a
// population are fresh, diverse seed genomes drawn from categories that are
// currently UNDER-represented in `population`. It mutates a copy and returns
// it. This is the anti-convergence safety valve: even when survivors (and the
// LLM mutating them) pull the whole population toward one endpoint, every
// generation still spends a few pods exploring other vuln classes.
//
// Selection of replacements is deterministic given `random`. Recon is never
// injected here (it is a one-shot mapping vector, handled by seeding), so the
// reserve goes to actual attack classes the swarm may have stopped exploring.
func InjectFreshDiversity(population []attacks.Genome, reserve int, discovered []string, random *rand.Rand) []attacks.Genome {
	if reserve <= 0 || len(population) == 0 {
		return population
	}
	if reserve > len(population) {
		reserve = len(population)
	}
	if random == nil {
		random = rand.New(rand.NewSource(1))
	}

	// Tally current category representation.
	categoryCount := make(map[string]int)
	for _, g := range population {
		categoryCount[g.Category]++
	}

	// Build a candidate pool of fresh seeds, skipping recon, ordered so that
	// under-represented categories come first.
	var freshPool []attacks.Genome
	for _, g := range attacks.SeedGenomes {
		if g.Category == "recon" {
			continue
		}
		freshPool = append(freshPool, g)
	}
	// Sort fresh candidates by how under-represented their category is
	// (ascending current count), so we add variety where it's most missing.
	sortGenomesByCategoryScarcity(freshPool, categoryCount)

	out := append([]attacks.Genome(nil), population...)
	// Replace the LAST `reserve` slots (these are the most-mutated /
	// least-elite entries) with fresh diverse seeds.
	for i := 0; i < reserve && i < len(freshPool); i++ {
		idx := len(out) - 1 - i
		if idx < 0 {
			break
		}
		fresh := cloneGenome(freshPool[i])
		// Aim fresh probes at a discovered path when we have one, so they are
		// relevant to THIS target rather than the static default.
		if len(discovered) > 0 {
			fresh.TargetPath = discovered[random.Intn(len(discovered))]
		}
		fresh.Hint = "diversity-injection: " + fresh.Hint
		out[idx] = fresh
	}
	return out
}

func sortGenomesByCategoryScarcity(genomes []attacks.Genome, categoryCount map[string]int) {
	for i := 1; i < len(genomes); i++ {
		current := genomes[i]
		j := i - 1
		for j >= 0 && categoryCount[genomes[j].Category] > categoryCount[current.Category] {
			genomes[j+1] = genomes[j]
			j--
		}
		genomes[j+1] = current
	}
}

func mutateBySwappingTechnique(parent attacks.Genome, random *rand.Rand) attacks.Genome {
	siblings := genomesInCategory(parent.Category)
	child := cloneGenome(parent)
	if len(siblings) > 0 {
		picked := siblings[random.Intn(len(siblings))]
		child.Technique = picked.Technique
		child.Hint = "swap-technique: " + picked.Hint
	}
	child.ParentPodID = parent.Vector
	return child
}

func mutateByVaryingTargetPath(parent attacks.Genome, random *rand.Rand, discovered []string) attacks.Genome {
	// Prefer real paths discovered on the target; this is what keeps the
	// swarm generic. Blend in the static category pool so we still explore
	// well-known routes the crawl may have missed (e.g. /.env, /admin).
	candidatePaths := append([]string(nil), discovered...)
	candidatePaths = append(candidatePaths, targetPathPoolForCategory(parent.Category)...)
	child := cloneGenome(parent)
	if len(candidatePaths) > 0 {
		child.TargetPath = candidatePaths[random.Intn(len(candidatePaths))]
		child.Hint = "vary-path: " + child.TargetPath
	}
	child.ParentPodID = parent.Vector
	return child
}

func mutateByShufflingParams(parent attacks.Genome, random *rand.Rand) attacks.Genome {
	child := cloneGenome(parent)
	if len(child.Params) >= 2 {
		// Swap two random param values to produce a mildly different probe.
		keys := make([]string, 0, len(child.Params))
		for k := range child.Params {
			keys = append(keys, k)
		}
		i, j := random.Intn(len(keys)), random.Intn(len(keys))
		if i != j {
			child.Params[keys[i]], child.Params[keys[j]] = child.Params[keys[j]], child.Params[keys[i]]
		}
	}
	child.Hint = "shuffle-params"
	child.ParentPodID = parent.Vector
	return child
}

func crossBreed(parentA, parentB attacks.Genome) attacks.Genome {
	child := cloneGenome(parentA)
	child.Vector = parentA.Vector + "_x_" + parentB.Vector
	child.Technique = parentB.Technique
	child.Hint = "cross-breed: " + parentA.Vector + " params with " + parentB.Vector + " technique"
	child.ParentPodID = parentA.Vector + "+" + parentB.Vector
	return child
}

func cloneGenome(g attacks.Genome) attacks.Genome {
	clonedParams := make(map[string]any, len(g.Params))
	for k, v := range g.Params {
		clonedParams[k] = v
	}
	return attacks.Genome{
		Vector:      g.Vector,
		Category:    g.Category,
		Technique:   g.Technique,
		TargetPath:  g.TargetPath,
		Params:      clonedParams,
		Hint:        g.Hint,
		ParentPodID: g.ParentPodID,
	}
}

func genomesInCategory(category string) []attacks.Genome {
	matches := make([]attacks.Genome, 0)
	for _, g := range attacks.SeedGenomes {
		if g.Category == category {
			matches = append(matches, g)
		}
	}
	return matches
}

// targetPathPoolForCategory returns a small curated list of common web
// application paths worth trying for each attack category. These are GENERIC
// guesses used as a fallback/supplement; the primary source of paths at
// runtime is MutationContext.DiscoveredPaths (real routes the recon genome
// found on the actual target). Includes both classic page routes and API
// routes so the swarm works on server-rendered apps and SPAs alike.
func targetPathPoolForCategory(category string) []string {
	switch category {
	case "recon":
		return []string{"/", "/robots.txt", "/sitemap.xml", "/api", "/login", "/about"}
	case "injection":
		return []string{
			"/", "/search", "/products", "/login", "/api/login",
			"/api/search", "/api/products", "/api/comments", "/api/items",
			"/graphql", "/api/graphql",
		}
	case "auth":
		return []string{"/login", "/api/login", "/api/me", "/api/register", "/account", "/api/users", "/api/reset-password", "/oauth/authorize", "/oauth/token"}
	case "access_control":
		return []string{
			"/admin", "/api/admin", "/api/users/1", "/api/users/2", "/api/orders/1",
			"/account", "/profile", "/logout", "/redirect", "/login",
			"/graphql", "/api/v1/admin", "/api/orders/2",
		}
	case "xss":
		return []string{
			"/", "/search", "/products", "/comments", "/login",
			"/contact", "/subscribe", "/api/comments", "/api/reviews", "/api/feedback",
		}
	case "data_exposure":
		return []string{
			"/.env", "/.git/config", "/robots.txt", "/sitemap.xml", "/api/debug",
			"/api/config", "/api/health", "/package.json", "/resources/js", "/static",
			"/graphql", "/download", "/files", "/admin/config", "/debug/env",
		}
	case "ssrf":
		return []string{
			"/", "/api/preview", "/api/fetch", "/api/import", "/api/proxy",
			"/preview", "/fetch-url", "/health-check", "/pdf", "/export", "/api/webhook",
		}
	case "deserialization":
		return []string{"/", "/api/login", "/validate", "/api/session", "/cart", "/api/message"}
	case "business_logic":
		return []string{
			"/api/transfer", "/transfer", "/checkout", "/api/purchase", "/cart",
			"/api/redeem", "/coupon", "/api/orders", "/wallet",
		}
	case "misconfig":
		return []string{
			"/", "/api", "/api/users", "/graphql", "/admin", "/download",
			"/.git/config", "/api/config", "/debug",
		}
	case "protocol":
		return []string{"/", "/admin", "/api/admin", "/login"}
	case "validation":
		return []string{"/contact", "/api/contact", "/register", "/api/register", "/subscribe"}
	}
	return []string{"/", "/search", "/login", "/api/users", "/api/search"}
}

// PickSurvivors selects up to cap genome-result pairs whose fitness exceeds
// threshold, sorted by fitness descending. It mirrors evolution_loop's
// survivor_threshold and survivor_cap settings.
//
// To prevent premature convergence (the whole swarm collapsing onto one
// endpoint — e.g. every pod hammering /admin for 6 generations), it also caps
// how many survivors may share the same category via maxPerCategory. A value
// <= 0 disables the per-category cap. Diversity beats local optima: keeping a
// spread of categories alive lets the swarm keep exploring other vuln classes
// instead of breeding endless variants of the single thing that scored first.
func PickSurvivors(scored []ScoredGenome, threshold float64, cap int) []attacks.Genome {
	return PickSurvivorsDiverse(scored, threshold, cap, 2)
}

// PickSurvivorsDiverse is PickSurvivors with an explicit per-category cap.
func PickSurvivorsDiverse(scored []ScoredGenome, threshold float64, cap int, maxPerCategory int) []attacks.Genome {
	picked := make([]ScoredGenome, 0, len(scored))
	for _, sg := range scored {
		if sg.Fitness > threshold {
			picked = append(picked, sg)
		}
	}
	sortScoredDescending(picked)

	out := make([]attacks.Genome, 0, len(picked))
	perCategory := make(map[string]int)
	for _, sg := range picked {
		if cap > 0 && len(out) >= cap {
			break
		}
		if maxPerCategory > 0 && perCategory[sg.Genome.Category] >= maxPerCategory {
			continue // category already well represented; keep diversity
		}
		perCategory[sg.Genome.Category]++
		out = append(out, sg.Genome)
	}
	return out
}

// ScoredGenome ties one generation's genome to the fitness score the
// control plane computed from its pod's result. It is the input shape for
// PickSurvivors and the unit the store records.
type ScoredGenome struct {
	Genome  attacks.Genome
	Result  Result
	Fitness float64
	PodID   string
}

func sortScoredDescending(scored []ScoredGenome) {
	// tiny insertion sort: populations are <=15 here, so this is fine and
	// avoids pulling in sort.Slice's reflection overhead for a hot path.
	for i := 1; i < len(scored); i++ {
		current := scored[i]
		j := i - 1
		for j >= 0 && scored[j].Fitness < current.Fitness {
			scored[j+1] = scored[j]
			j--
		}
		scored[j+1] = current
	}
}
