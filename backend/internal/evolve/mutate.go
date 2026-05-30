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
}

// Mutate produces the next generation by mixing three sources, in this order:
//   1. The survivors themselves, kept verbatim ("elitism"): the next gen
//      includes the parents so good solutions are not lost.
//   2. Targeted mutations of survivors (swap technique, vary target path,
//      occasional param shuffle).
//   3. Cross-bred children that take params from one survivor and
//      vector+technique from another.
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
			output = append(output, mutateByVaryingTargetPath(parent, random))
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

func mutateByVaryingTargetPath(parent attacks.Genome, random *rand.Rand) attacks.Genome {
	candidatePaths := targetPathPoolForCategory(parent.Category)
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
// application paths worth trying for each attack category.
func targetPathPoolForCategory(category string) []string {
	switch category {
	case "injection":
		return []string{"/api/login", "/api/search", "/api/users", "/api/products", "/api/comments"}
	case "auth":
		return []string{"/api/login", "/api/me", "/api/register", "/api/reset-password", "/api/users"}
	case "access_control":
		return []string{"/api/users/1", "/api/users/2", "/api/orders/1", "/api/admin", "/admin"}
	case "xss":
		return []string{"/search", "/api/comments", "/api/reviews", "/contact", "/api/feedback"}
	case "data_exposure":
		return []string{"/.env", "/robots.txt", "/api/debug", "/api/config", "/api/health"}
	case "validation":
		return []string{"/api/contact", "/api/register", "/api/reset-password"}
	}
	return []string{"/", "/api/users", "/api/search"}
}

// PickSurvivors selects up to cap genome-result pairs whose fitness exceeds
// threshold, sorted by fitness descending. It mirrors evolution_loop's
// survivor_threshold and survivor_cap settings.
func PickSurvivors(scored []ScoredGenome, threshold float64, cap int) []attacks.Genome {
	picked := make([]ScoredGenome, 0, len(scored))
	for _, sg := range scored {
		if sg.Fitness > threshold {
			picked = append(picked, sg)
		}
	}
	sortScoredDescending(picked)
	if len(picked) > cap && cap > 0 {
		picked = picked[:cap]
	}
	out := make([]attacks.Genome, len(picked))
	for i, sg := range picked {
		out[i] = sg.Genome
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
