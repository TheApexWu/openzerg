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

func TestPickSurvivors_FiltersByThresholdAndCap(t *testing.T) {
	scored := []ScoredGenome{
		{Genome: attacks.Genome{Vector: "a"}, Fitness: 0.9},
		{Genome: attacks.Genome{Vector: "b"}, Fitness: 0.05},
		{Genome: attacks.Genome{Vector: "c"}, Fitness: 0.5},
		{Genome: attacks.Genome{Vector: "d"}, Fitness: 0.4},
	}
	got := PickSurvivors(scored, 0.1, 2)
	if len(got) != 2 {
		t.Fatalf("expected 2 survivors, got %d (%+v)", len(got), got)
	}
	if got[0].Vector != "a" || got[1].Vector != "c" {
		t.Fatalf("expected fitness-desc order a,c; got %s,%s", got[0].Vector, got[1].Vector)
	}
}
