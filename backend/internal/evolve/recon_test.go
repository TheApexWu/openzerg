package evolve

import "testing"

func TestSeedFromRecon_SSRFPath(t *testing.T) {
	seeds := SeedFromRecon([]string{"/api/fetch-url", "/preview"}, 4)
	if len(seeds) == 0 {
		t.Fatal("expected recon seeds for SSRF-looking paths, got none")
	}
	foundSSRF := false
	for _, s := range seeds {
		if s.Category == "ssrf" && (s.TargetPath == "/api/fetch-url" || s.TargetPath == "/preview") {
			foundSSRF = true
		}
	}
	if !foundSSRF {
		t.Fatalf("expected an ssrf genome aimed at the discovered path, got %+v", seeds)
	}
}

func TestSeedFromRecon_GraphQLPath(t *testing.T) {
	seeds := SeedFromRecon([]string{"/graphql"}, 4)
	if len(seeds) == 0 {
		t.Fatal("expected seeds for /graphql")
	}
	for _, s := range seeds {
		if s.TargetPath != "/graphql" {
			t.Fatalf("graphql seed should target /graphql, got %s", s.TargetPath)
		}
	}
}

func TestSeedFromRecon_EmptyAndCap(t *testing.T) {
	if got := SeedFromRecon(nil, 4); got != nil {
		t.Fatal("nil paths should yield nil")
	}
	seeds := SeedFromRecon([]string{"/login", "/search", "/api/users", "/admin", "/cart"}, 2)
	if len(seeds) > 2 {
		t.Fatalf("cap not respected: %d", len(seeds))
	}
}

func TestResultDiscoveredPaths(t *testing.T) {
	r := Result{RawFindings: []map[string]any{
		{"discovered_paths": []any{"/a", "/b"}},
		{"links": []any{"/c"}},
	}}
	if got := ResultDiscoveredPaths(r); got != 3 {
		t.Fatalf("want 3, got %d", got)
	}
}

func TestSeedFromRecon_AuthPathSeedsJWT(t *testing.T) {
	// The bug: a discovered auth endpoint only seeded the FIRST auth genome
	// (mass_assign_role), never the jwt_alg_* vectors, so JWT benchmarks were
	// unsolvable. Now an auth path should seed JWT among the auth genomes.
	seeds := SeedFromRecon([]string{"/api/auth/login"}, 8)
	hasJWT := false
	for _, s := range seeds {
		if s.Vector == "jwt_alg_none" || s.Vector == "jwt_alg_confusion" {
			hasJWT = true
		}
	}
	if !hasJWT {
		vecs := ""
		for _, s := range seeds {
			vecs += s.Vector + " "
		}
		t.Fatalf("auth path should seed a JWT vector; got: %s", vecs)
	}
}

func TestSeedFromRecon_AuthSeededDespiteManyAccessControlPaths(t *testing.T) {
	// The realistic APEX-003 case: recon found many access_control-ish paths
	// PLUS one auth path. The old path-by-path seeding exhausted the budget on
	// access_control before reaching auth, so JWT never seeded. Breadth-first
	// priority must now seed a JWT vector even with limited budget.
	paths := []string{"/api/users", "/api/users/me", "/api/admin/flag", "/api/docs", "/api/auth/login"}
	seeds := SeedFromRecon(paths, 5) // small budget on purpose
	hasJWT := false
	for _, s := range seeds {
		if s.Vector == "jwt_alg_none" || s.Vector == "jwt_alg_confusion" {
			hasJWT = true
		}
	}
	if !hasJWT {
		vecs := ""
		for _, s := range seeds {
			vecs += s.Vector + " "
		}
		t.Fatalf("with mixed paths + small budget, JWT must still be seeded; got: %s", vecs)
	}
}
