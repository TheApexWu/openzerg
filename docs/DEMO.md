# OpenZerg — 3-minute demo script

Goal: show an evolutionary, agentic red-team swarm finding a real exploit on
a live web target, and explain it in one short summary.

Judging criteria touched: autonomy, tool use (Pi + OpenRouter + Kubernetes +
Nimble), real-world value, technical implementation.

## Pre-roll (do BEFORE you start the clock)

- Two terminal panes:
  - **Pane A**: `cd backend && ./bin/openzerg doctor` ready to run.
  - **Pane B**: `kubectl -n openzerg get pods --watch` running already.
- An editor pane on `out/summary-demo.md` (sanitised sample from a prior
  run) is open but not visible.
- `.env` is present and `openzerg-keys` Secret already applied to the
  cluster.

## 00:00 — The pitch (15 s)

> "We don't send one smart attacker. We send a hundred dumb ones, watch
> which find cracks, and breed the next generation from the survivors.
> OpenZerg is an evolutionary, agentic red-team swarm against a known
> vulnerable web app: OWASP Juice Shop."

## 00:15 — Doctor (15 s)

Run in Pane A:

```
./bin/openzerg doctor
```

Highlights to read aloud as it prints:

- kubeconfig resolved → cluster context is DigitalOcean managed K8s.
- secrets loaded → OPENROUTER_API_KEY present, NIMBLE_API_KEY present.
- doctor exits 0 with no side effects.

## 00:30 — Run the swarm (90 s)

```
./bin/openzerg run \
  --target https://juice-shop-production-d0c5.up.railway.app \
  --population 3 --generations 2
```

While it runs, narrate what is happening on Pane B (`kubectl ... --watch`):

- 3 attacker pods appear with `openzerg-attacker-…-g1-pN` names.
- They Run → Succeed → are deleted by the control plane.
- Stdout in Pane A streams per-pod result JSON lines.
- One pod hits `BREACH` (SQL injection tautology on `/rest/user/login`,
  returns an admin JWT). Control plane short-circuits, prints
  `outcome: BREACH (best fitness 1.00)`, and writes the summary.

The narration callouts:

- "Each pod is an autonomous Pi agent — Gemma 4 model via OpenRouter,
  white-hat scope, rate-limited, bounded to 120 seconds."
- "The control plane fans out pod creation, streams logs, parses each
  pod's last JSON line, scores by the PRD's keyword rubric, and decides
  who survives into the next generation."
- "When any pod reports fitness 1.0 the loop stops immediately. Otherwise
  it runs up to `--generations` and writes EXHAUSTED."

## 02:00 — Show the summary (45 s)

Open `out/summary-<latest>.md` in the editor pane:

- Outcome line at the top is **BREACH**.
- "BREACH path" section: vector, category, the exact `POST /rest/user/login`
  request and the response snippet containing the JWT prefix.
- Narrative paragraph: which pods tried which vectors, top scorers list.
- Per-generation table.

Also briefly show `summary-<latest>.json` — same data, machine-readable, with
a `narrative_md_path` pointer.

## 02:45 — Close (15 s)

> "Two sponsor tools: Pi (the agent framework + Gemma 4 via OpenRouter for
> in-pod reasoning) and Nimble (for JS-rendered page navigation in the
> attacker skill). Everything runs on Kubernetes. No human intervention
> after `openzerg run`. The same loop scales from 3 pods to 100 with one
> flag."

If asked about extending:

- Swap the genome catalog and seed list to point at a different web target.
- Plug in a new mutation strategy (LLM mutation is already wired and budget
  capped at 32 OpenRouter calls per run).

## If the run does NOT breach (fallback narrative)

Sometimes Gemma misses; that is the point of evolution. If the demo run
ends EXHAUSTED:

- Show the `Best fitness reached` line and the survivors carried forward.
- Open the summary's "Top scorers" list — those are the partial findings
  the next generation would have mutated from.
- Re-run with `--generations 4 --population 15` off-camera and screenshare
  the resulting BREACH summary committed at `out/summary-demo.md`.
