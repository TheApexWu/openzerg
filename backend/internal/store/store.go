// Package store keeps in-memory run state (per-pod results and per-generation
// aggregates) and writes the final summary JSON + Markdown artifacts to disk.
//
// The store is single-goroutine. The control plane's generation loop appends
// to it sequentially; concurrency happens inside spawn.RunPods, but the
// generation loop collects all outcomes before calling RecordGeneration.
package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/TheApexWu/openzerg/backend/internal/attacks"
	"github.com/TheApexWu/openzerg/backend/internal/evolve"
)

// GenerationRecord captures the outcome of one generation in the store.
// PerPod stores every pod's scored result; the summary writer derives
// aggregates from it.
type GenerationRecord struct {
	Number     int                    `json:"generation"`
	Population int                    `json:"population"`
	Survivors  int                    `json:"survivors"`
	BestFit    float64                `json:"best_fitness"`
	Breaches   int                    `json:"breaches"`
	PerPod     []evolve.ScoredGenome  `json:"per_pod"`
}

// RunStore is the in-memory ledger for one openzerg run.
type RunStore struct {
	RunID       string             `json:"run_id"`
	TargetURL   string             `json:"target_url"`
	StartedAt   time.Time          `json:"started_at"`
	FinishedAt  time.Time          `json:"finished_at"`
	Generations []GenerationRecord `json:"per_generation"`
	Outcome     string             `json:"outcome"`
	BestFitness float64            `json:"best_fitness"`
	Breach      *BreachRecord      `json:"breach"`
	Cancelled   bool               `json:"cancelled,omitempty"`
}

// BreachRecord summarises the winning pod when a generation hit fitness 1.0.
type BreachRecord struct {
	PodID       string           `json:"pod_id"`
	Generation  int              `json:"generation"`
	Vector      string           `json:"vector"`
	Category    string           `json:"category"`
	Evidence    string           `json:"evidence"`
	RawFindings []map[string]any `json:"raw_findings"`
}

// NewRunStore constructs an empty store and stamps the start time.
func NewRunStore(runID, targetURL string) *RunStore {
	return &RunStore{
		RunID:     runID,
		TargetURL: targetURL,
		StartedAt: time.Now().UTC(),
		Outcome:   "PENDING",
	}
}

// RecordGeneration appends a generation's scored outcomes and updates
// aggregates. It also detects a breach (fitness == 1.0) and stamps Outcome.
func (store *RunStore) RecordGeneration(generationNumber int, scored []evolve.ScoredGenome) {
	record := GenerationRecord{
		Number:     generationNumber,
		Population: len(scored),
		PerPod:     scored,
	}
	for _, sg := range scored {
		if sg.Fitness > record.BestFit {
			record.BestFit = sg.Fitness
		}
		if sg.Fitness > 0.1 {
			record.Survivors++
		}
		if sg.Fitness >= 1.0 {
			record.Breaches++
			if store.Breach == nil {
				store.Breach = &BreachRecord{
					PodID:       sg.PodID,
					Generation:  generationNumber,
					Vector:      sg.Genome.Vector,
					Category:    sg.Genome.Category,
					Evidence:    sg.Result.Evidence,
					RawFindings: sg.Result.RawFindings,
				}
			}
		}
	}
	if record.BestFit > store.BestFitness {
		store.BestFitness = record.BestFit
	}
	store.Generations = append(store.Generations, record)
}

// Finalize sets the outcome (BREACH / EXHAUSTED), finished_at, and returns.
func (store *RunStore) Finalize() {
	store.FinishedAt = time.Now().UTC()
	if store.Breach != nil {
		store.Outcome = "BREACH"
		return
	}
	store.Outcome = "EXHAUSTED"
}

// WriteArtifacts writes summary-<run_id>.json and summary-<run_id>.md into
// outDir, creating the directory if needed. Returns the two paths so the
// caller can print them.
func (store *RunStore) WriteArtifacts(outDir string) (jsonPath, mdPath string, err error) {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", "", fmt.Errorf("store: mkdir %s: %w", outDir, err)
	}
	jsonPath = filepath.Join(outDir, "summary-"+store.RunID+".json")
	mdPath = filepath.Join(outDir, "summary-"+store.RunID+".md")

	jsonPayload := store.toJSONPayload(mdPath)
	jsonBytes, err := json.MarshalIndent(jsonPayload, "", "  ")
	if err != nil {
		return "", "", fmt.Errorf("store: marshal json: %w", err)
	}
	if err := os.WriteFile(jsonPath, jsonBytes, 0o644); err != nil {
		return "", "", fmt.Errorf("store: write json: %w", err)
	}

	mdBytes, err := store.renderMarkdown()
	if err != nil {
		return "", "", fmt.Errorf("store: render md: %w", err)
	}
	if err := os.WriteFile(mdPath, mdBytes, 0o644); err != nil {
		return "", "", fmt.Errorf("store: write md: %w", err)
	}
	return jsonPath, mdPath, nil
}

// jsonPayloadShape mirrors data_contracts.final_summary_json. Kept separate
// from RunStore so we can decorate it with the rendered narrative path.
type jsonPayloadShape struct {
	RunID           string             `json:"run_id"`
	TargetURL       string             `json:"target_url"`
	StartedAt       string             `json:"started_at"`
	FinishedAt      string             `json:"finished_at"`
	DurationS       float64            `json:"duration_s"`
	GenerationsRun  int                `json:"generations_run"`
	TotalPods       int                `json:"total_pods"`
	Outcome         string             `json:"outcome"`
	BestFitness     float64            `json:"best_fitness"`
	Breach          *BreachRecord      `json:"breach"`
	PerGeneration   []GenerationRecord `json:"per_generation"`
	NarrativeMDPath string             `json:"narrative_md_path"`
	Cancelled       bool               `json:"cancelled,omitempty"`
}

func (store *RunStore) toJSONPayload(narrativeMDPath string) jsonPayloadShape {
	totalPods := 0
	for _, g := range store.Generations {
		totalPods += g.Population
	}
	durationSeconds := store.FinishedAt.Sub(store.StartedAt).Seconds()
	return jsonPayloadShape{
		RunID:           store.RunID,
		TargetURL:       store.TargetURL,
		StartedAt:       store.StartedAt.Format(time.RFC3339),
		FinishedAt:      store.FinishedAt.Format(time.RFC3339),
		DurationS:       durationSeconds,
		GenerationsRun:  len(store.Generations),
		TotalPods:       totalPods,
		Outcome:         store.Outcome,
		BestFitness:     store.BestFitness,
		Breach:          store.Breach,
		PerGeneration:   store.Generations,
		NarrativeMDPath: narrativeMDPath,
		Cancelled:       store.Cancelled,
	}
}

const markdownTemplateSource = `# OpenZerg run {{.RunID}}

- Target: {{.TargetURL}}
- Outcome: **{{.Outcome}}**{{if .Cancelled}} (cancelled by user){{end}}
- Best fitness: {{printf "%.2f" .BestFitness}}
- Generations: {{.GenerationsRun}} / pods total: {{.TotalPods}}
- Duration: {{printf "%.1fs" .DurationS}}

{{if .Breach}}
## BREACH path

The swarm found a working exploit in generation {{.Breach.Generation}}, pod {{.Breach.PodID}}.

- Vector: ` + "`{{.Breach.Vector}}`" + ` (category ` + "`{{.Breach.Category}}`" + `)
- Evidence: {{.Breach.Evidence}}

### Raw findings
{{range .Breach.RawFindings}}- ` + "`{{.method}} {{.url}}`" + ` -> {{.status_code}}
  {{.snippet}}
{{end}}
{{else}}
## Outcome: EXHAUSTED

No pod returned fitness 1.0. The swarm explored {{.TotalPods}} probes across
{{.GenerationsRun}} generations. Best fitness reached: {{printf "%.2f" .BestFitness}}.
{{end}}

## Narrative

{{.Narrative}}

## Per-generation summary

| Gen | Pods | Survivors | Best fitness | Breaches |
| --- | ---- | --------- | ------------ | -------- |
{{range .PerGeneration}}| {{.Number}} | {{.Population}} | {{.Survivors}} | {{printf "%.2f" .BestFit}} | {{.Breaches}} |
{{end}}

## Top scorers

{{range .TopScorers}}- gen {{.Generation}} pod {{.PodID}} vector ` + "`{{.Vector}}`" + ` fitness {{printf "%.2f" .Fitness}} — {{.Evidence}}
{{end}}
`

type markdownData struct {
	jsonPayloadShape
	Narrative  string
	TopScorers []topScorerRow
}

type topScorerRow struct {
	Generation int
	PodID      string
	Vector     string
	Fitness    float64
	Evidence   string
}

func (store *RunStore) renderMarkdown() ([]byte, error) {
	tmpl, err := template.New("summary").Parse(markdownTemplateSource)
	if err != nil {
		return nil, err
	}
	payload := store.toJSONPayload("")
	data := markdownData{
		jsonPayloadShape: payload,
		Narrative:        store.buildNarrative(),
		TopScorers:       store.collectTopScorers(5),
	}
	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, err
	}
	return []byte(buf.String()), nil
}

func (store *RunStore) buildNarrative() string {
	if len(store.Generations) == 0 {
		return "The swarm did not complete a single generation. " +
			"This usually means no attacker pods produced parseable result lines."
	}
	var b strings.Builder
	for _, gen := range store.Generations {
		fmt.Fprintf(&b,
			"Generation %d spawned %d probes; %d survived above the 0.1 fitness floor "+
				"(best %.2f, %d breach%s).\n\n",
			gen.Number, gen.Population, gen.Survivors, gen.BestFit, gen.Breaches,
			pluralS(gen.Breaches),
		)
		topThree := topThreeOfGeneration(gen)
		for _, sg := range topThree {
			fmt.Fprintf(&b, "- pod %s tried `%s` (%s) — fitness %.2f. Evidence: %s\n",
				sg.PodID, sg.Genome.Vector, sg.Genome.Technique, sg.Fitness, summarise(sg.Result.Evidence))
		}
		b.WriteString("\n")
	}
	switch store.Outcome {
	case "BREACH":
		fmt.Fprintf(&b,
			"The run converged on a working exploit (vector `%s` in generation %d). "+
				"See the BREACH section above for the exact request that worked.\n",
			store.Breach.Vector, store.Breach.Generation)
	case "EXHAUSTED":
		fmt.Fprintf(&b,
			"After %d generations the swarm did not produce a fitness-1.0 result. "+
				"Best fitness reached was %.2f. Survivors carried interesting partial "+
				"findings forward across generations but none escalated to a full breach.\n",
			len(store.Generations), store.BestFitness)
	}
	return b.String()
}

func (store *RunStore) collectTopScorers(limit int) []topScorerRow {
	all := make([]evolve.ScoredGenome, 0)
	genByPod := map[string]int{}
	for _, gen := range store.Generations {
		for _, sg := range gen.PerPod {
			all = append(all, sg)
			genByPod[sg.PodID] = gen.Number
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Fitness > all[j].Fitness })
	if len(all) > limit {
		all = all[:limit]
	}
	rows := make([]topScorerRow, 0, len(all))
	for _, sg := range all {
		rows = append(rows, topScorerRow{
			Generation: genByPod[sg.PodID],
			PodID:      sg.PodID,
			Vector:     sg.Genome.Vector,
			Fitness:    sg.Fitness,
			Evidence:   summarise(sg.Result.Evidence),
		})
	}
	return rows
}

func topThreeOfGeneration(gen GenerationRecord) []evolve.ScoredGenome {
	copyOfPods := make([]evolve.ScoredGenome, len(gen.PerPod))
	copy(copyOfPods, gen.PerPod)
	sort.Slice(copyOfPods, func(i, j int) bool { return copyOfPods[i].Fitness > copyOfPods[j].Fitness })
	if len(copyOfPods) > 3 {
		copyOfPods = copyOfPods[:3]
	}
	return copyOfPods
}

func summarise(evidence string) string {
	evidence = strings.TrimSpace(evidence)
	if evidence == "" {
		return "(no evidence)"
	}
	if len(evidence) > 160 {
		return evidence[:160] + "..."
	}
	return evidence
}

func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "es"
}

// Genomes is a re-export of the seed genomes from attacks, kept here for
// callers that want a stable home for "initial population" without a second
// import.
var Genomes = attacks.SeedGenomes
