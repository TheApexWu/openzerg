package attacks

import "testing"

// TestSeedGenomesWellFormed guards the hand-written catalog: every seed must
// have the fields the pod prompt and scorer rely on. Catches copy-paste slips
// when new vuln-class genomes are added.
func TestSeedGenomesWellFormed(t *testing.T) {
	seen := map[string]bool{}
	for i, g := range SeedGenomes {
		if g.Vector == "" {
			t.Errorf("genome %d has empty Vector", i)
		}
		if g.Category == "" {
			t.Errorf("genome %q has empty Category", g.Vector)
		}
		if g.Technique == "" {
			t.Errorf("genome %q has empty Technique", g.Vector)
		}
		if g.TargetPath == "" {
			t.Errorf("genome %q has empty TargetPath", g.Vector)
		}
		if seen[g.Vector] {
			t.Errorf("duplicate vector %q", g.Vector)
		}
		seen[g.Vector] = true
	}
}

// TestReconSeedIsFirst documents the ordering contract that PickSeedGenomes
// relies on: recon must lead so small populations always include it.
func TestReconSeedIsFirst(t *testing.T) {
	if len(SeedGenomes) == 0 || SeedGenomes[0].Category != "recon" {
		t.Fatalf("expected first seed genome to be the recon vector, got %+v", SeedGenomes[0])
	}
}

// TestPickSeedGenomesIsCategoryDiverse guards the selection fix: at the default
// population the picked set must span many vulnerability categories (not just
// the first few list entries), recon must lead and appear once, and there must
// be no accidental duplicates while the catalog is larger than n.
func TestPickSeedGenomesIsCategoryDiverse(t *testing.T) {
	const n = 15
	picked := PickSeedGenomes(n)
	if len(picked) != n {
		t.Fatalf("expected %d genomes, got %d", n, len(picked))
	}
	if picked[0].Category != "recon" {
		t.Fatalf("expected recon first, got %q", picked[0].Vector)
	}

	cats := map[string]bool{}
	seen := map[string]int{}
	for _, g := range picked {
		cats[g.Category] = true
		seen[g.Vector]++
	}
	// With a catalog much larger than n, every pick should be unique.
	if len(SeedGenomes) >= n {
		for v, c := range seen {
			if c > 1 {
				t.Errorf("genome %q selected %d times; expected unique picks when catalog >= n", v, c)
			}
		}
	}
	// Diversity: expect coverage well beyond the handful of original classes.
	if len(cats) < 8 {
		t.Errorf("expected >=8 categories at population %d, got %d: %v", n, len(cats), cats)
	}
	// The modern classes must be reachable at the default population.
	for _, want := range []string{"ssrf", "deserialization", "business_logic"} {
		if !cats[want] {
			t.Errorf("category %q not seeded at population %d (modern class unreachable)", want, n)
		}
	}
}

// TestPickSeedGenomesWrapsForLargeN ensures n beyond the catalog still returns
// exactly n genomes (wrap-around), preserving the old contract.
func TestPickSeedGenomesWrapsForLargeN(t *testing.T) {
	n := len(SeedGenomes) + 7
	picked := PickSeedGenomes(n)
	if len(picked) != n {
		t.Fatalf("expected %d genomes for large n, got %d", n, len(picked))
	}
}
