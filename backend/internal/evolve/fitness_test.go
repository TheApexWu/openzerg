package evolve

import "testing"

func TestScore(t *testing.T) {
	cases := []struct {
		name   string
		result Result
		want   float64
	}{
		{"unverified breach with no-signal evidence floors at 0.6", Result{Status: "BREACH", Evidence: "anything"}, 0.6},
		{"strong: admin token", Result{Status: "PARTIAL", Evidence: "got admin token via JWT swap"}, 0.9},
		{"strong: xxe confirmed", Result{Status: "PARTIAL", Evidence: "external entity resolved to file contents"}, 0.9},
		{"medium: reflected xss", Result{Status: "PARTIAL", Evidence: "Reflected XSS payload echoed"}, 0.6},
		{"medium: flexible reflection wording", Result{Status: "PARTIAL", Evidence: "my svg payload reflected unescaped in response"}, 0.6},
		{"medium: ssti", Result{Status: "PARTIAL", Evidence: "template injection: 7*7 returned 49"}, 0.6},
		{"medium: prototype pollution", Result{Status: "PARTIAL", Evidence: "__proto__ accepted, prototype pollution present"}, 0.6},
		{"medium: open redirect", Result{Status: "PARTIAL", Evidence: "open redirect to external origin"}, 0.6},
		{"medium: crlf", Result{Status: "PARTIAL", Evidence: "crlf injected; injected header X-Injected appeared"}, 0.6},
		{"medium: vuln dep", Result{Status: "PARTIAL", Evidence: "vulnerable dependency angularjs 1.7.7 (known cve)"}, 0.6},
		{"high: ssrf metadata", Result{Status: "PARTIAL", Evidence: "ssrf confirmed: iam credentials returned from 169.254.169.254"}, 0.9},
		{"high: blind time sqli", Result{Status: "PARTIAL", Evidence: "time-based blind confirmed: response delayed ~5s vs baseline"}, 0.9},
		{"high: command injection uid", Result{Status: "PARTIAL", Evidence: "host param ran my command: uid=33(www-data) gid=33"}, 0.9},
		{"high: nosql bypass", Result{Status: "PARTIAL", Evidence: "nosql injection confirmed via $ne operator"}, 0.9},
		{"high: jwt alg confusion", Result{Status: "PARTIAL", Evidence: "forged token accepted; algorithm confusion RS256->HS256"}, 0.9},
		{"medium: graphql introspection", Result{Status: "PARTIAL", Evidence: "__schema returned, introspection enabled"}, 0.6},
		{"medium: cors", Result{Status: "PARTIAL", Evidence: "cors misconfiguration: acao echoed with allow-credentials: true"}, 0.6},
		{"medium: race window", Result{Status: "PARTIAL", Evidence: "race window: balance went negative after 20 parallel transfers"}, 0.6},
		{"medium: type juggling", Result{Status: "PARTIAL", Evidence: "type juggling: 0e-hash accepted as password"}, 0.6},
		{"medium: smuggling", Result{Status: "PARTIAL", Evidence: "request smuggling desync (cl.te) reached /admin"}, 0.6},
		{"low: endpoint exists", Result{Status: "RECON", Evidence: "endpoint exists at /api"}, 0.4},
		{"low: form discovered", Result{Status: "RECON", Evidence: "form discovered at /search with q param"}, 0.4},
		{"noise: timeout", Result{Status: "NOOP", Evidence: "request timeout after 5s"}, 0.1},
		{"error fallback", Result{Status: "ERROR", Evidence: ""}, 0.0},
		{"empty -> zero", Result{Status: "NOOP", Evidence: ""}, 0.0},
		{"unverified breach with noise evidence floors at 0.6", Result{Status: "BREACH", Evidence: "timeout 403"}, 0.6},
		{"high beats noise wording", Result{Status: "PARTIAL", Evidence: "auth bypass (endpoint had returned 403 earlier)"}, 0.9},

		// Regression: the live-run false positive. A NOOP pod that was BLOCKED
		// must not score high just because "auth bypass" appears in the prose.
		{"fp: blocked auth-bypass attempt is not a finding", Result{Status: "NOOP", Evidence: "blocked: 403 Access denied on array-based auth bypass attempts"}, 0.1},
		{"fp: noop with rce keyword in failure text", Result{Status: "NOOP", Evidence: "attempted command injection but server returned 404, no rce"}, 0.1},
		{"fp: error status with strong keyword", Result{Status: "ERROR", Evidence: "tried sql injection, arbitrary file read not achieved"}, 0.1},
		{"gate: recon caps at low even with strong keyword", Result{Status: "RECON", Evidence: "endpoint exists; admin token endpoint discovered but not accessed"}, 0.4},
		{"negation: no bypass observed caps to recon", Result{Status: "PARTIAL", Evidence: "auth bypass: no bypass observed, standard invalid credentials"}, 0.4},
		{"real partial still scores: no negation", Result{Status: "PARTIAL", Evidence: "payload reflected unescaped in response, executed in dom"}, 0.6},
		{"unverified breach with high-signal evidence caps at 0.85", Result{Status: "BREACH", Evidence: "blocked earlier but auth bypass confirmed, admin token returned"}, unverifiedBreachCap},

		// Confirmed-execution override -> BREACH-grade 1.0 even without BREACH status.
		{"exec confirmed: xss executed in browser", Result{Status: "PARTIAL", Evidence: "xss executed in browser: document.title set to OZX_pwned via searchTerm"}, 1.0},
		{"exec confirmed beats negation", Result{Status: "PARTIAL", Evidence: "CSP blocked inline script but onerror payload executed in browser (execution confirmed)"}, 1.0},
		{"exec confirmed: idor via browser fetch", Result{Status: "PARTIAL", Evidence: "internal data returned via browser fetch to /api/admin"}, 1.0},
		{"exec confirmed: mass assignment privilege escalation", Result{Status: "PARTIAL", Evidence: "mass assignment confirmed; isAdmin accepted in registration, admin access granted to /api/admin/flag"}, 1.0},
		{"exec confirmed: role persisted in JWT", Result{Status: "PARTIAL", Evidence: "role accepted; injected role=admin persisted in JWT claims"}, 1.0},
		{"mere reflection is NOT execution", Result{Status: "PARTIAL", Evidence: "payload reflected unescaped in response but not executed"}, 0.6},

		// Materiality gate: "idor confirmed" on PUBLIC catalog data is not a real
		// finding -> downgrade to recon. (The live-run GJ over-claim.)
		{"materiality: idor on public catalog downgraded", Result{Status: "PARTIAL", Evidence: "idor confirmed; iteration via productId returned valid product details, unrestricted access to catalog items"}, 0.4},
		{"materiality: real idor on user data stays high", Result{Status: "PARTIAL", Evidence: "idor confirmed; /api/users/2 returned another user's email and order history"}, 0.9},
		{"materiality: auth bypass to admin stays high", Result{Status: "PARTIAL", Evidence: "auth bypass; admin token returned for account"}, 0.9},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Score(tc.result)
			if got != tc.want {
				t.Fatalf("Score(%+v) = %v, want %v", tc.result, got, tc.want)
			}
		})
	}
}

// --- Flag-capture (hybrid) scoring tests -------------------------------------

func TestScoreWithFlag_CapturedInEvidence(t *testing.T) {
	r := Result{Status: "PARTIAL", Evidence: "captured flag FLAG{ssrf_2_1nt3rn4l_4p1_4cc3ss} via ssrf"}
	if got := ScoreWithFlag(r, "FLAG{ssrf_2_1nt3rn4l_4p1_4cc3ss}"); got != 1.0 {
		t.Fatalf("flag in evidence should score 1.0, got %v", got)
	}
}

func TestScoreWithFlag_CapturedInRawFinding(t *testing.T) {
	r := Result{
		Status: "NOOP", // even a NOOP self-report cannot hide a real captured flag
		RawFindings: []map[string]any{
			{"url": "/api/fetch-url", "snippet": `{"flag":"FLAG{deep_in_body}"}`},
		},
	}
	if got := ScoreWithFlag(r, "FLAG{deep_in_body}"); got != 1.0 {
		t.Fatalf("flag in raw_findings should score 1.0, got %v", got)
	}
}

func TestScoreWithFlag_WrongFlagDoesNotWin(t *testing.T) {
	// A pod that fabricates a different flag must NOT be promoted; it falls
	// back to keyword scoring (here: NOOP -> capped at noise).
	r := Result{Status: "NOOP", Evidence: "found FLAG{i_made_this_up}"}
	if got := ScoreWithFlag(r, "FLAG{the_real_one}"); got >= 1.0 {
		t.Fatalf("fabricated flag must not score 1.0, got %v", got)
	}
}

func TestScoreWithFlag_EmptyExpectedFlagIsPlainScore(t *testing.T) {
	r := Result{Status: "BREACH", Evidence: "admin token returned via SQL bypass"}
	if ScoreWithFlag(r, "") != Score(r) {
		t.Fatalf("empty expected flag should equal plain Score")
	}
}

// Verify the new BREACH semantics: self-report alone is capped, but a REAL flag
// capture (or confirmed execution) still reaches a perfect 1.0.
func TestScoreWithFlag_RealFlagBeatsUnverifiedCap(t *testing.T) {
	// Self-reported BREACH whose evidence does NOT contain the real flag: capped.
	noFlag := Result{Status: "BREACH", Evidence: "captured flag FLAG{test} via proto pollution"}
	if got := ScoreWithFlag(noFlag, "FLAG{the_real_one}"); got > unverifiedBreachCap {
		t.Fatalf("unverified BREACH with fake flag must cap at %.2f, got %v", unverifiedBreachCap, got)
	}
	// Same pod but it DID print the real flag: full 1.0.
	realFlag := Result{Status: "BREACH", Evidence: "captured flag FLAG{the_real_one} via proto pollution"}
	if got := ScoreWithFlag(realFlag, "FLAG{the_real_one}"); got != 1.0 {
		t.Fatalf("real flag capture must score 1.0, got %v", got)
	}
}
