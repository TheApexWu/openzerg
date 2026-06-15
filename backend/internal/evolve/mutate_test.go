package evolve

import (
	"math/rand"
	"testing"

	"github.com/TheApexWu/openzerg/backend/internal/attacks"
)

func TestMutate_FallsBackToSeedsWhenNoSurvivors(t *testing.T) {
	got := Mutate(MutationContext{Survivors: nil, PopulationSize: 5})
	if len(got) != 5 {
		t.Fatalf("expected 5 fallback genomes, got %d", len(got))
	}
	if got[0].Vector == "" {
		t.Fatalf("fallback genome has empty vector")
	}
}

func TestMutate_KeepsParentsAndFillsToPopulationSize(t *testing.T) {
	survivors := []attacks.Genome{
		{Vector: "sqli_login", Category: "injection", Technique: "tautology", TargetPath: "/rest/user/login"},
		{Vector: "xss_search_reflected", Category: "xss", Technique: "reflected", TargetPath: "/#/search"},
	}
	got := Mutate(MutationContext{
		Survivors:      survivors,
		PopulationSize: 6,
		Random:         rand.New(rand.NewSource(42)),
	})
	if len(got) != 6 {
		t.Fatalf("expected 6 genomes, got %d", len(got))
	}
	if got[0].Vector != "sqli_login" || got[1].Vector != "xss_search_reflected" {
		t.Fatalf("survivors not preserved at head: %+v", got[:2])
	}
}

func TestMutate_UsesDiscoveredPathsWhenVaryingPath(t *testing.T) {
	parent := attacks.Genome{
		Vector: "xss_reflected_generic", Category: "xss", Technique: "reflected",
		TargetPath: "/", Params: map[string]any{"q": "x"},
	}
	discovered := []string{"/shop", "/news/article", "/shop/item/stock"}
	discoveredSet := map[string]bool{}
	for _, p := range discovered {
		discoveredSet[p] = true
	}
	// With many output slots and a fixed seed, at least one path-varying
	// mutation should land on a discovered path rather than only the static
	// pool. Sweep a few seeds to avoid flakiness from strategy rolls.
	hitDiscovered := false
	for seed := int64(1); seed <= 20 && !hitDiscovered; seed++ {
		got := Mutate(MutationContext{
			Survivors:       []attacks.Genome{parent},
			PopulationSize:  12,
			Random:          rand.New(rand.NewSource(seed)),
			DiscoveredPaths: discovered,
		})
		for _, g := range got {
			if discoveredSet[g.TargetPath] {
				hitDiscovered = true
				break
			}
		}
	}
	if !hitDiscovered {
		t.Fatal("expected at least one mutated genome to target a discovered path")
	}
}

func TestTargetPathPoolCoversAllSeedCategories(t *testing.T) {
	// Every category used by a seed genome should have its own curated path
	// pool (not silently fall through to the generic default), so path-varying
	// mutations stay relevant to the vector's class.
	seen := map[string]bool{}
	for _, g := range attacks.SeedGenomes {
		if seen[g.Category] {
			continue
		}
		seen[g.Category] = true
		if len(targetPathPoolForCategory(g.Category)) == 0 {
			t.Errorf("category %q has empty path pool", g.Category)
		}
	}
}

func TestPickSurvivors_FiltersByThresholdAndCap(t *testing.T) {
	scored := []ScoredGenome{
		{Genome: attacks.Genome{Vector: "a", Category: "injection"}, Fitness: 0.9},
		{Genome: attacks.Genome{Vector: "b", Category: "xss"}, Fitness: 0.05},
		{Genome: attacks.Genome{Vector: "c", Category: "auth"}, Fitness: 0.5},
		{Genome: attacks.Genome{Vector: "d", Category: "ssrf"}, Fitness: 0.4},
	}
	got := PickSurvivors(scored, 0.1, 2)
	if len(got) != 2 {
		t.Fatalf("expected 2 survivors, got %d (%+v)", len(got), got)
	}
	if got[0].Vector != "a" || got[1].Vector != "c" {
		t.Fatalf("expected fitness-desc order a,c; got %s,%s", got[0].Vector, got[1].Vector)
	}
}

func TestPickSurvivors_CapsPerCategory(t *testing.T) {
	// All five clear the threshold and share one category; without the
	// per-category cap the swarm would breed only this family (the live-run
	// /admin convergence). maxPerCategory=2 must keep at most 2.
	scored := []ScoredGenome{
		{Genome: attacks.Genome{Vector: "admin1", Category: "access_control"}, Fitness: 0.4},
		{Genome: attacks.Genome{Vector: "admin2", Category: "access_control"}, Fitness: 0.4},
		{Genome: attacks.Genome{Vector: "admin3", Category: "access_control"}, Fitness: 0.4},
		{Genome: attacks.Genome{Vector: "admin4", Category: "access_control"}, Fitness: 0.4},
		{Genome: attacks.Genome{Vector: "sqli1", Category: "injection"}, Fitness: 0.3},
	}
	got := PickSurvivorsDiverse(scored, 0.1, 7, 2)
	ac := 0
	for _, g := range got {
		if g.Category == "access_control" {
			ac++
		}
	}
	if ac > 2 {
		t.Fatalf("expected <=2 access_control survivors, got %d (%+v)", ac, got)
	}
	// The injection genome should have survived so diversity is preserved.
	found := false
	for _, g := range got {
		if g.Category == "injection" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected the injection-category survivor to be kept for diversity")
	}
}

func TestInjectFreshDiversity_ReplacesTailWithDiverseSeeds(t *testing.T) {
	// A fully-converged population: every genome is the same access_control
	// vector. Diversity injection must introduce other categories.
	pop := make([]attacks.Genome, 10)
	for i := range pop {
		pop[i] = attacks.Genome{Vector: "admin_panel", Category: "access_control", TargetPath: "/admin"}
	}
	out := InjectFreshDiversity(pop, 3, []string{"/shop"}, rand.New(rand.NewSource(7)))
	if len(out) != len(pop) {
		t.Fatalf("population size changed: %d -> %d", len(pop), len(out))
	}
	cats := map[string]bool{}
	for _, g := range out {
		cats[g.Category] = true
	}
	if len(cats) < 2 {
		t.Fatalf("expected diversity injection to add new categories, got only %v", cats)
	}
}
