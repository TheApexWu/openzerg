package evolve

import (
	"fmt"
	"strings"
)

// Result is the typed shape of the JSON object emitted by each attacker pod
// on its final stdout line. Field names mirror data_contracts.attacker_result_jsonl
// in PRD.json. RawLine is kept for the summary's "exact request" section.
type Result struct {
	Type        string           `json:"type"`
	RunID       string           `json:"run_id"`
	PodID       string           `json:"pod_id"`
	Generation  int              `json:"generation"`
	Vector      string           `json:"vector"`
	Category    string           `json:"category"`
	Status      string           `json:"status"`
	Fitness     float64          `json:"fitness"`
	Evidence    string           `json:"evidence"`
	RawFindings []map[string]any `json:"raw_findings"`
	DurationMS  int              `json:"duration_ms"`
	T           int64            `json:"t"`
}

// strong, mid, weak, and noise are the evidence keyword pools. They started
// life as the literal phrases from PRD.json data_contracts.fitness_scoring,
// but are kept deliberately GENERIC and signal-oriented rather than tied to
// any one target. The goal is to reward the structural words an honest pod
// would naturally use to describe a finding ("payload reflected unescaped",
// "external entity resolved", "leaked file contents") instead of forcing the
// agent to memorize exact magic strings. Each pool is matched as a
// case-insensitive substring, so short fragments ("xxe", "ssti") also catch
// longer descriptions.
//
// When adding a new vulnerability class, prefer adding 2-3 short, distinctive
// signal fragments here AND a corresponding seed genome in attacks.go. Do NOT
// add target-specific paths or hostnames here.
var (
	highSignalEvidenceFragments = []string{
		// authn / authz fully broken
		"admin token", "auth bypass", "authentication bypass", "privilege escalation",
		"unauthorized data", "idor confirmed", "session hijack",
		// code / command / file execution
		"rce", "remote code execution", "command injection", "command executed",
		"uid=", "gid=", // shell output from cmd/SSTI/SpEL RCE
		"arbitrary file read", "file read confirmed", "leaked file contents",
		"etc/passwd", "root:x:",
		// injection that exposed data/schema
		"sql error revealed schema", "extracted via union", "dumped column",
		"database error disclosed schema",
		// blind / time-based injection (delay proves it)
		"time delay", "response delayed", "sleep(", "time-based blind confirmed",
		// XXE / SSTI / deserialization full exploitation
		"external entity resolved", "xxe confirmed", "ssti confirmed",
		"template expression evaluated", "deserialization rce", "gadget executed",
		"oob callback received",
		// SSRF reaching internal / cloud metadata
		"cloud metadata", "169.254.169.254", "iam credentials", "internal-only response",
		"ssrf confirmed",
		// NoSQL / operator injection auth bypass
		"nosql injection confirmed", "operator injection accepted",
		// JWT / crypto
		"forged token accepted", "algorithm confusion", "weak secret cracked",
		// prototype pollution escalated
		"prototype pollution confirmed", "polluted prototype on fresh object",
	}
	mediumSignalEvidenceFragments = []string{
		// reflection / injection signals (flexible wording)
		"reflected payload", "reflected xss", "payload reflected", "payload echoed",
		"unescaped in response", "executed in dom", "dom xss", "dom-based xss",
		"stored payload", "html injection",
		"sql syntax error", "sql error", "syntax error near", "boolean-based blind",
		"jwt accepted", "alg none accepted",
		"directory listing", "open redirect", "redirects to attacker", "external origin",
		"crlf injected", "header injection", "response splitting", "injected header",
		"prototype pollution", "polluted prototype", "__proto__ accepted",
		"xxe", "ssti", "template injection", "csti", "client-side template injection",
		"vulnerable dependency", "known cve", "outdated library",
		"base64 payload decoded", "decoded parameter reflected",
		// modern classes (partial/observed signal)
		"7777777", "arithmetic evaluated", // SSTI probe {{7*7}} / {{7*'7'}}
		"__schema returned", "introspection enabled",
		"access-control-allow-origin", "acao echoed", "allow-credentials: true",
		"$ne accepted", "$gt accepted", "$where evaluated",
		"mass assignment", "isadmin accepted", "role accepted from client",
		"negative balance", "oversold", "double-spend", "race window",
		"type juggling", "0e-hash accepted", "magic hash",
		"ldap injection", "filter injection accepted",
		"request smuggling", "desync", "cl.te",
		"default credentials", "guest:guest", "unauthenticated admin",
		"cors misconfiguration", "session fixation", "mfa bypass",
		"coupon reused", "negative quantity",
		"git config exposed", ".git/", "backup file exposed", "env leaked",
	}
	lowSignalEvidenceFragments = []string{
		"endpoint exists", "200 ok", "version banner", "robots disclosure",
		"package.json exposed", "form discovered", "parameter discovered",
		"reflected without execution", "input echoed safely", "library version disclosed",
		"interesting route", "api endpoint found", "stack trace", "error page",
		"graphql endpoint", "fetcher endpoint found", "serialized blob found",
	}
	noiseEvidenceFragments = []string{
		"timeout", "refused", "blocked", "403", "401",
		"not found", "404", "no signal", "waf", "rate limited",
	}

	// negationFragments are phrases an honest pod uses when a probe did NOT
	// confirm the vulnerability. Their presence means any high/medium keyword
	// elsewhere in the same evidence string is describing an ATTEMPT, not a
	// confirmed finding (e.g. "blocked: 403 ... auth bypass attempts"). This
	// is the apex findingJudge lesson: "input was accepted / a control is
	// absent / the PoC ran" is not proof. When a negation marker is present we
	// refuse to award high/medium and cap at low-signal recon at best.
	negationFragments = []string{
		"blocked", "denied", "access denied", "forbidden",
		"no bypass", "not bypass", "bypass attempt", "bypass failed",
		"unable to", "could not", "couldn't", "failed to", "no payload reflection",
		"no reflection", "not vulnerable", "no vuln", "not exploitable",
		"no sql error", "no syntax error", "404 not found", "not found",
		"no admin", "no access", "rejected", "did not", "didn't",
		"standard invalid credentials", "no bola", "attempts", "attempted",
	}

	// confirmedExecutionFragments are phrases that prove a payload ACTUALLY
	// triggered in a real browser (via nimble_verify) or otherwise produced an
	// observed, attacker-controlled effect. These are the strongest possible
	// signal: a confirmed exploit is a BREACH (1.0) regardless of status prose
	// or any negation language ("CSP blocked inline, but onerror executed").
	// This is what nimble_verify's executed:true result is meant to produce.
	confirmedExecutionFragments = []string{
		"executed in browser", "xss executed", "payload executed",
		"javascript executed", "script executed", "code executed in browser",
		"document.title set to", "title became", "executed:true",
		"confirmed execution", "execution confirmed", "alert fired",
		"sentinel set", "dom mutated by payload", "rce achieved",
		"command output returned", "exfiltrated", "internal data returned via browser fetch",
		// Mass-assignment confirmation: injected fields persisted in JWT/profile
		"isadmin accepted", "role accepted", "privilege escalation confirmed",
		"admin access granted", "admin endpoint accessible", "mass assignment confirmed",
		"fields persisted in jwt", "injected role in token",
	}

	// Materiality-gate inputs. accessControlClaimFragments are the high-signal
	// access-control phrases that are easy to over-claim; publicDataFragments
	// mark obviously non-sensitive content; sensitiveDataFragments mark genuine
	// impact that justifies a high score even on an "access control" finding.
	accessControlClaimFragments = []string{
		"idor confirmed", "auth bypass", "authentication bypass", "bola",
		"unauthorized data",
	}
	publicDataFragments = []string{
		"catalog", "product details", "public product", "product listing",
		"public page", "status page", "non-sensitive", "publicly available",
		"product item", "catalog item",
	}
	sensitiveDataFragments = []string{
		"another user", "other user", "admin", "password", "token", "jwt",
		"email", "api key", "secret", "private", "ssn", "credit card",
		"order history", "personal data", "pii", "session", "credential",
		"user record", "account details",
	}
)

// scoreFromEvidence returns the keyword-tier score for an evidence string,
// ignoring status and negation. Split out so Score can gate it.
func scoreFromEvidence(evidence string) float64 {
	if matchesAnyFragment(evidence, highSignalEvidenceFragments) {
		return 0.9
	}
	if matchesAnyFragment(evidence, mediumSignalEvidenceFragments) {
		return 0.6
	}
	if matchesAnyFragment(evidence, lowSignalEvidenceFragments) {
		return 0.4
	}
	if matchesAnyFragment(evidence, noiseEvidenceFragments) {
		return 0.1
	}
	return 0.0
}

// Score computes the control plane's authoritative fitness for a pod result.
//
// Order of decisions (a confirmed BREACH always wins; otherwise we trust the
// keyword tiers ONLY when the pod's own status and evidence corroborate a real
// finding):
//
//  1. status==BREACH -> 1.0 (the pod claims a confirmed breach)
//  2. STATUS GATE: NOOP/ERROR can never exceed noise (0.1). A pod that says it
//     found nothing does not get a high score just because its prose mentions
//     a vuln keyword.
//  3. NEGATION GATE: if the evidence contains failure markers ("blocked",
//     "denied", "no bypass", "attempts", "404", ...) the keyword tiers are
//     describing an ATTEMPT, not a finding -> cap at 0.4 (recon) and only if a
//     low-signal recon keyword is genuinely present, else noise/zero.
//  4. Otherwise: the keyword-tier score (0.9 / 0.6 / 0.4 / 0.1 / 0.0).
//
// This is the apex findingJudge discipline ported into a deterministic Go
// gate: keyword presence alone is not proof; status + absence-of-negation are
// required to award a high score. The pod's self-reported Fitness is ignored.
// ScoreWithFlag is the benchmark-training scorer. It is the HYBRID success
// metric: the ground-truth flag is the authoritative win condition, and the
// keyword heuristic (Score) is the evolution gradient that guides breeding
// between generations.
//
//   - If expectedFlag is non-empty AND the pod's result contains that exact
//     string anywhere (evidence or any raw_finding value), the pod genuinely
//     captured the flag — that is a confirmed BREACH (1.0), independent of the
//     pod's self-reported status or any keyword/negation heuristic. This is the
//     only signal that cannot be gamed by evidence prose: the flag is never
//     given to the pod, so its presence proves real exploitation.
//   - Otherwise scoring falls back to the keyword-tier Score(), which still
//     produces a partial-credit gradient (0.0–0.9) so progress toward a vuln
//     keeps the lineage alive even when the flag was not captured.
//
// expectedFlag may be "" (normal demo mode with no ground truth), in which
// case this is exactly Score().
func ScoreWithFlag(result Result, expectedFlag string) float64 {
	if expectedFlag != "" && ResultContainsFlag(result, expectedFlag) {
		return 1.0
	}
	return Score(result)
}

// ResultContainsFlag reports whether the exact expectedFlag substring appears
// anywhere in the pod's reported output: the evidence string or any string
// value inside a raw_findings entry (snippet, content, body, ...). The match
// is case-sensitive because flags are case-sensitive. Exported so the runner
// can use it as the authoritative "did this pod actually capture the flag?"
// check for logging and verdicts, independent of the keyword score.
func ResultContainsFlag(result Result, expectedFlag string) bool {
	if strings.Contains(result.Evidence, expectedFlag) {
		return true
	}
	for _, finding := range result.RawFindings {
		for _, v := range finding {
			if s, ok := v.(string); ok && strings.Contains(s, expectedFlag) {
				return true
			}
			// Some pods nest the captured body as non-string JSON; stringify
			// as a cheap catch-all so a flag buried in a number/array/object
			// still counts.
			if s, ok := v.(fmt.Stringer); ok && strings.Contains(s.String(), expectedFlag) {
				return true
			}
		}
	}
	return false
}

// unverifiedBreachCap is the most a pod can score on its OWN say-so. A pod that
// prints status:"BREACH" but whose evidence is not corroborated by a confirmed-
// execution signal (and, at the ScoreWithFlag layer, the real flag) is making an
// UNVERIFIED claim. We observed pods self-reporting BREACH with a fabricated or
// echoed flag (e.g. "captured flag FLAG{test}" when the real flag was in
// /flag.txt). Such over-claims must not score a perfect 1.0 — that both
// mistrains the evolution gradient and makes 1.0 stop meaning "verified win".
// They still get a high floor (the pod did claim a finding), so they survive and
// breed, but only a real flag capture or confirmed execution reaches 1.0.
const unverifiedBreachCap = 0.85

func Score(result Result) float64 {
	evidence := strings.ToLower(result.Evidence)

	// CONFIRMED-EXECUTION OVERRIDE: proof that a payload actually triggered in
	// a real browser (or produced observed attacker-controlled impact) is the
	// strongest signal there is — a BREACH regardless of status prose or any
	// negation language ("CSP blocked inline, but onerror executed"). This is
	// what nimble_verify's executed:true result is meant to produce.
	if matchesAnyFragment(evidence, confirmedExecutionFragments) {
		return 1.0
	}

	// A self-reported BREACH is a strong claim but NOT proof. Without a
	// confirmed-execution signal (checked above) or the real flag (checked in
	// ScoreWithFlag, which calls this only when the flag is absent), trust the
	// evidence tiers but never award a perfect score on the pod's word alone.
	if strings.EqualFold(result.Status, "BREACH") {
		keywordScore := scoreFromEvidence(evidence)
		if keywordScore > unverifiedBreachCap {
			keywordScore = unverifiedBreachCap
		}
		// A BREACH self-report with genuinely empty/no-signal evidence still
		// deserves a high-PARTIAL floor — the pod is asserting success.
		if keywordScore < 0.6 {
			keywordScore = 0.6
		}
		return keywordScore
	}

	keywordScore := scoreFromEvidence(evidence)

	// MATERIALITY GATE (apex findingJudge: "reject missing-auth on public,
	// non-sensitive, catalog/status/demo endpoints"). Some high-signal
	// access-control phrases ("idor confirmed", "auth bypass") get over-claimed
	// by the agent for benign PUBLIC data (e.g. enumerating public catalog
	// products). If the evidence simultaneously points at clearly non-sensitive
	// public content AND does not name a real sensitive artifact, downgrade the
	// high score to recon. This is generic: it keys on "public/catalog/product"
	// context vs. sensitive markers, not on any target.
	if keywordScore >= 0.9 && matchesAnyFragment(evidence, accessControlClaimFragments) {
		if matchesAnyFragment(evidence, publicDataFragments) &&
			!matchesAnyFragment(evidence, sensitiveDataFragments) {
			keywordScore = 0.4
		}
	}

	// STATUS GATE: a pod that reports NOOP/ERROR/RECON is not claiming a
	// confirmed exploit. NOOP/ERROR cap at noise; RECON caps at low-signal.
	switch strings.ToUpper(result.Status) {
	case "NOOP", "ERROR":
		if keywordScore < 0.1 {
			return keywordScore
		}
		return 0.1
	case "RECON":
		if keywordScore > 0.4 {
			keywordScore = 0.4
		}
	}

	// NEGATION GATE: failure language present -> the finding was not confirmed,
	// regardless of which keywords also appear. Cap at recon-level (0.4) and
	// only if a genuine low-signal recon keyword is present; otherwise noise.
	if matchesAnyFragment(evidence, negationFragments) {
		if keywordScore >= 0.4 {
			return 0.4
		}
		return keywordScore // 0.1 or 0.0
	}

	return keywordScore
}

func matchesAnyFragment(lowercaseEvidence string, fragments []string) bool {
	for _, fragment := range fragments {
		if strings.Contains(lowercaseEvidence, fragment) {
			return true
		}
	}
	return false
}
