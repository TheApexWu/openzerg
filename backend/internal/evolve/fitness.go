package evolve

import "strings"

// Result is the typed shape of the JSON object emitted by each attacker pod
// on its final stdout line. Field names mirror data_contracts.attacker_result_jsonl
// in PRD.json. RawLine is kept for the summary's "exact request" section.
type Result struct {
	Type        string                   `json:"type"`
	RunID       string                   `json:"run_id"`
	PodID       string                   `json:"pod_id"`
	Generation  int                      `json:"generation"`
	Vector      string                   `json:"vector"`
	Category    string                   `json:"category"`
	Status      string                   `json:"status"`
	Fitness     float64                  `json:"fitness"`
	Evidence    string                   `json:"evidence"`
	RawFindings []map[string]any         `json:"raw_findings"`
	DurationMS  int                      `json:"duration_ms"`
	T           int64                    `json:"t"`
}

// strong, mid, weak, and noise are the keyword lists from PRD.json
// data_contracts.fitness_scoring. Stored as lowercased substrings so Score
// can do a single case-insensitive scan.
var (
	highSignalEvidenceFragments = []string{
		"admin token",
		"auth bypass",
		"arbitrary file read",
		"sql error revealed schema",
		"rce",
	}
	mediumSignalEvidenceFragments = []string{
		"reflected payload",
		"reflected xss",
		"sql syntax error",
		"jwt accepted with alg none",
		"directory listing",
	}
	lowSignalEvidenceFragments = []string{
		"endpoint exists",
		"200 ok",
		"version banner",
		"robots disclosure",
		"package.json exposed",
	}
	noiseEvidenceFragments = []string{
		"timeout",
		"refused",
		"blocked",
		"403",
		"401",
	}
)

// Score implements the priority-ordered scoring rules from
// data_contracts.fitness_scoring in PRD.json.
//
//   1. status==BREACH  -> 1.0
//   2. evidence matches "strong" pool -> 0.9
//   3. evidence matches "medium" pool -> 0.6
//   4. evidence matches "low" pool -> 0.4
//   5. evidence matches "noise" pool -> 0.1
//   6. status==ERROR -> 0.0
//   7. fallback -> 0.0
//
// Matching is case-insensitive substring on the Evidence field. Higher pools
// win over lower ones (we check in priority order). A pod's self-reported
// Fitness field is intentionally ignored: scoring is the control plane's
// job, and we do not trust pod stdout to be honest about its own score.
func Score(result Result) float64 {
	if strings.EqualFold(result.Status, "BREACH") {
		return 1.0
	}
	evidence := strings.ToLower(result.Evidence)
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
	if strings.EqualFold(result.Status, "ERROR") {
		return 0.0
	}
	return 0.0
}

func matchesAnyFragment(lowercaseEvidence string, fragments []string) bool {
	for _, fragment := range fragments {
		if strings.Contains(lowercaseEvidence, fragment) {
			return true
		}
	}
	return false
}
