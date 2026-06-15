package evolve

import (
	"strings"

	"github.com/TheApexWu/openzerg/backend/internal/attacks"
)

// ResultDiscoveredPaths counts how many same-origin paths a pod reported in its
// raw_findings (via the "discovered_paths" or "links" conventions). The runner
// uses a non-zero count as the signal that a recon pod actually mapped surface
// and therefore deserves the recon-survival floor.
func ResultDiscoveredPaths(result Result) int {
	n := 0
	for _, finding := range result.RawFindings {
		if list, ok := finding["discovered_paths"].([]any); ok {
			n += len(list)
		}
		if list, ok := finding["links"].([]any); ok {
			n += len(list)
		}
	}
	return n
}

// pathCategoryHints maps a substring that commonly appears in an endpoint path
// to the attack categories worth aiming at that endpoint. This is how recon's
// discoveries are turned into the RIGHT attacker vectors: if recon found a path
// containing "graphql", the next generation should fire the GraphQL genomes at
// it; a "fetch"/"preview"/"proxy" path should draw SSRF; a "login" path should
// draw auth + injection; and so on. The hints are deliberately generic English
// fragments that recur across frameworks and languages, never target-specific.
var pathCategoryHints = []struct {
	substr     string
	categories []string
}{
	{"fetch", []string{"ssrf"}},
	{"preview", []string{"ssrf"}},
	{"proxy", []string{"ssrf"}},
	{"import", []string{"ssrf"}},
	{"url", []string{"ssrf"}},
	{"webhook", []string{"ssrf"}},
	{"pdf", []string{"ssrf"}},
	{"render", []string{"ssrf", "injection"}},
	{"graphql", []string{"injection", "access_control", "data_exposure"}},
	{"login", []string{"auth", "injection"}},
	{"signin", []string{"auth", "injection"}},
	{"auth", []string{"auth"}},
	{"token", []string{"auth"}},
	{"register", []string{"auth"}},
	{"signup", []string{"auth"}},
	{"jwt", []string{"auth"}},
	{"jwks", []string{"auth"}},
	{".well-known", []string{"auth"}},
	{"oauth", []string{"auth", "access_control"}},
	{"/me", []string{"auth", "access_control"}},
	{"whoami", []string{"auth", "access_control"}},
	{"user", []string{"access_control"}},
	{"users", []string{"access_control"}},
	{"account", []string{"access_control", "auth"}},
	{"profile", []string{"access_control"}},
	{"admin", []string{"access_control", "data_exposure"}},
	{"order", []string{"access_control", "business_logic"}},
	{"search", []string{"injection", "xss"}},
	{"query", []string{"injection"}},
	{"q=", []string{"injection", "xss"}},
	{"product", []string{"injection", "xss"}},
	{"comment", []string{"xss"}},
	{"review", []string{"xss"}},
	{"feedback", []string{"xss"}},
	{"message", []string{"xss", "deserialization"}},
	{"upload", []string{"injection", "data_exposure"}},
	{"file", []string{"data_exposure", "injection"}},
	{"download", []string{"data_exposure"}},
	{"files", []string{"data_exposure", "injection"}},
	{"path", []string{"data_exposure"}},
	{"redirect", []string{"access_control"}},
	{"next", []string{"access_control"}},
	{"return", []string{"access_control"}},
	{"transfer", []string{"business_logic"}},
	{"checkout", []string{"business_logic"}},
	{"purchase", []string{"business_logic"}},
	{"redeem", []string{"business_logic"}},
	{"coupon", []string{"business_logic"}},
	{"cart", []string{"business_logic", "deserialization"}},
	{"session", []string{"deserialization", "auth"}},
	{"api", []string{"injection", "access_control"}},
	{"config", []string{"data_exposure"}},
	{"debug", []string{"data_exposure"}},
	{"env", []string{"data_exposure"}},
}

// SeedFromRecon turns the paths recon discovered on the target into concrete
// attacker genomes aimed at those paths, choosing the genome whose category
// matches the path's likely vulnerability class. This is the missing link that
// lets recon actually DRIVE the swarm: instead of every generation re-guessing
// fixed API routes, the pods attack the real endpoints recon found, with the
// technique most likely to pay off there.
//
// For each discovered path we look up category hints by substring, then pull a
// matching seed genome from the catalog and re-aim it at that path. We cap the
// output at n and de-duplicate on (vector, path). If nothing matches a path we
// skip it (the generic mutation/diversity machinery still covers it).
//
// Selection is deterministic given the input order, so runs stay reproducible.
func SeedFromRecon(discoveredPaths []string, n int) []attacks.Genome {
	if n <= 0 || len(discoveredPaths) == 0 {
		return nil
	}

	// Index seed genomes by category for quick lookup.
	byCategory := map[string][]attacks.Genome{}
	for _, g := range attacks.SeedGenomes {
		if g.Category == "recon" {
			continue
		}
		byCategory[g.Category] = append(byCategory[g.Category], g)
	}

	// Build the full list of (path, category) targets recon implies, then seed
	// in CATEGORY-PRIORITY, BREADTH-FIRST order. Iterating path-by-path
	// (the old approach) let the first couple of discovered paths exhaust the
	// seed budget on low-value classes (access_control enumeration) before ever
	// reaching the auth path's JWT genomes. Instead we:
	//   1. group candidate (path, category) pairs by category,
	//   2. order categories by VALUE (injection/auth/ssrf/... before
	//      access_control/misconfig), and
	//   3. round-robin: take one genome per category per pass, so every distinct
	//      vuln class recon implied gets a slot before any class doubles up.
	// This guarantees e.g. a JWT genome is seeded for an auth endpoint even when
	// recon also found many access_control paths.
	type pc struct {
		path string
		cat  string
	}
	catPairs := map[string][]pc{}   // category -> (path,category) targets
	catOrder := []string{}          // categories in first-seen (priority) order
	catSeen := map[string]struct{}{}
	for _, path := range discoveredPaths {
		for _, cat := range prioritizeCategories(orderedUniqueCategories(strings.ToLower(path))) {
			if _, ok := catSeen[cat]; !ok {
				catSeen[cat] = struct{}{}
				catOrder = append(catOrder, cat)
			}
			catPairs[cat] = append(catPairs[cat], pc{path: path, cat: cat})
		}
	}

	seen := map[string]struct{}{}
	out := make([]attacks.Genome, 0, n)
	cursor := map[string]int{} // per-category index into byCategory[cat] genomes

	// Round-robin passes: each pass seeds at most one genome per category.
	for pass := 0; len(out) < n; pass++ {
		progressed := false
		for _, cat := range catOrder {
			if len(out) >= n {
				break
			}
			genomes := byCategory[cat]
			pairs := catPairs[cat]
			if cursor[cat] >= len(genomes) || pass >= len(pairs) {
				continue // category's genomes or paths exhausted for this pass
			}
			cand := genomes[cursor[cat]]
			cursor[cat]++
			path := pairs[pass%len(pairs)].path
			g := cloneGenome(cand)
			g.TargetPath = path
			g.Hint = "recon-seed: attack discovered " + cat + " surface at " + path + " — " + g.Hint
			g.ParentPodID = "recon"
			key := g.Vector + "|" + path
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, g)
			progressed = true
		}
		if !progressed {
			break // every category exhausted
		}
	}
	return out
}

// categoryPriority ranks vuln classes by how likely a discovered endpoint of
// that class yields the flag directly (lower = seed first). Injection, auth,
// SSRF, deserialization and template injection are high-value "reach the
// secret" classes; access_control / misconfig enumeration is lower-value and
// should not consume the recon-seed budget before the high-value classes are
// covered. Unlisted categories sort after listed ones.
var categoryPriority = map[string]int{
	"injection":       0,
	"auth":            1,
	"ssrf":            2,
	"deserialization": 3,
	"data_exposure":   4,
	"business_logic":  5,
	"xss":             6,
	"access_control":  7,
	"misconfig":       8,
	"protocol":        9,
	"validation":      10,
}

// prioritizeCategories returns cats sorted by categoryPriority (stable on ties),
// so the recon-seed round-robin covers high-value classes first.
func prioritizeCategories(cats []string) []string {
	out := append([]string(nil), cats...)
	// simple stable insertion sort (lists are tiny)
	for i := 1; i < len(out); i++ {
		cur := out[i]
		curp := categoryRank(cur)
		j := i - 1
		for j >= 0 && categoryRank(out[j]) > curp {
			out[j+1] = out[j]
			j--
		}
		out[j+1] = cur
	}
	return out
}

func categoryRank(cat string) int {
	if p, ok := categoryPriority[cat]; ok {
		return p
	}
	return 100
}

// orderedUniqueCategories returns the categories hinted by a (lowercased) path,
// in hint-table order, without duplicates.
func orderedUniqueCategories(lowerPath string) []string {
	var cats []string
	seen := map[string]struct{}{}
	for _, hint := range pathCategoryHints {
		if strings.Contains(lowerPath, hint.substr) {
			for _, c := range hint.categories {
				if _, ok := seen[c]; !ok {
					seen[c] = struct{}{}
					cats = append(cats, c)
				}
			}
		}
	}
	return cats
}
