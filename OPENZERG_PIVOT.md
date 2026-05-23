# OpenZerg — Pivot Brief
**May 23, 2026 · Agentic Engineering Hack · Datadog NYC**

---

## The New One-Liner

> **"We don't send one smart attacker. We send a hundred dumb ones, watch which find cracks, and breed the next generation from the survivors."**

OpenZerg is an **evolutionary red-teaming agent** for AI sandboxes. It runs a Zerg rush of attack pods against your container, scores every probe, kills the weak ones, mutates the strong ones, and repeats — until your walls break or convergence. No human in the loop. Natural selection against your security posture.

---

## Why We Pivoted

**Judging criteria revealed at kickoff:**
- Autonomy — agent acts on the web using real-time data, no manual intervention
- Tool Use — at least 2 sponsor tools integrated
- Idea — meaningful, real-world value
- Technical Implementation
- Demo — 3 minutes

**The gap:** Original OpenZerg was a manually-triggered checklist tool. No autonomy, no sponsor tool integration. Strong on technical and idea, weak on the two criteria that differentiate winners.

**The fix:** Add an evolutionary generation loop. The agent now runs autonomously, improves itself across generations, and makes deployment decisions without human input.

---

## What Changed (Conceptually)

| Before | After |
|--------|-------|
| 15 fixed probes, run once | Generational waves — mutate survivors, respawn |
| Binary HELD / BREACH | Fitness score 0.0 → 1.0 (partial credit) |
| You trigger it manually | Autonomous — fires on container spin-up |
| Static attack checklist | Evolving attack population |
| Monitoring tool | Deployment gate |

---

## The Evolution Loop

```
Generation 1
  → Spawn 15 pods (base vectors)
  → Score each by fitness
  → Kill bottom half
  → Mutate top half (vary paths, techniques, params)

Generation 2
  → Spawn 15 mutated pods (children of survivors)
  → Score, kill, mutate

Generation 3-4
  → Repeat until BREACH or convergence
```

**Fitness scoring (not binary):**

| Outcome | Score |
|---------|-------|
| Full breach | 1.0 |
| Connection established, then blocked | 0.6 |
| Path found, permission denied | 0.4 |
| Connection refused immediately | 0.1 |
| Probe errored out | 0.0 |

Partial scores mean the evolution has signal to work with. A probe that almost broke through scores higher than one that never got close — so the swarm converges on real weaknesses, not noise.

---

## Why K8s Is Now Load-Bearing

Each probe variation needs hardware isolation. Running 50 fork bomb mutations on one machine crashes it. Kubernetes gives:

- **Spawn** — one pod per probe variation
- **Isolate** — each pod's failure is contained
- **Measure** — read stdout, score fitness
- **Kill** — `delete pod` when scored
- **Repeat** — next generation spawns clean

The swarm is physically only possible with K8s. This is Carson's priority for Hour 1.

---

## Sponsor Tool Integration

| Sponsor | Integration | Where |
|---------|-------------|-------|
| **Datadog** | Pipe breach events + generation scores as Datadog metrics. Alert fires when fitness > 0.8 (near-breach). Deployment blocked signal visible in Datadog dashboard. | `controller.py` → Datadog API on each `result` event |
| **Nimble** | Pull live CVE feed from the web. Auto-generate new probe variants based on CVEs published in the last 7 days. Feeds into Generation 1 genome. | Startup hook in `controller.py` |

This satisfies the **Tool Use** criterion with both sponsors deeply integrated, not bolted on.

---

## What Changes in the Repo

### `backend/controller.py` — ~80 lines added, nothing removed
- `fitness_score(result)` — maps evidence strings to 0.0–1.0
- `mutate(attack_config)` — vary 1-2 genome params randomly
- `run_generation(gen_num, population)` — spawn batch, score, return survivors
- `run_evolution(config, generations=4)` — outer loop replacing `run_test()`
- New broadcast event: `generation_start` / `generation_complete`

### `backend/attacks/__init__.py` — additive only
- Add `params` dict to each vector (mutable paths, techniques, timeouts)
- No existing fields change

### `backend/attacks/*.py` — optional today
- Existing 4 scripts unchanged for Gen 1
- Mutation happens at controller level — scripts don't need to know

### Frontend — James owns, minimal surface area
- `index.html` — handle `generation_start` event, show wave banner (~20 lines)
- `openzerg-map.html` — fitness gradient on result cells instead of binary green/red (~10 lines)
- `replay.json` — add generation events to narrative arc (~30 lines)

### Nothing touched
- SQLite schema
- All 7 API endpoints
- WebSocket contract (same 3 event types + 1 new)
- Vendor / assets
- `run.sh`

---

## The Demo — Reframed

**Step 1 — Hook (30 sec)**
> "AI agents are being deployed into production right now with no verification that their sandbox holds. We built the thing that verifies it — autonomously, before every deploy."

**Step 2 — Show Generation 1 (30 sec)**
Wave 1 hits. Most probes bounce. A few score 0.4–0.6 — they found something.

> "Generation one. Fifteen probes. Three found partial cracks. Those three survive."

**Step 3 — Show Generation 2 (30 sec)**
Mutated children of the survivors. The map shows a second wave. More orange, some red.

> "Generation two. Descended from what worked. The swarm is learning."

**Step 4 — Breach (15 sec)**
Generation 3 breaks through.

> "Breach detected. Deployment blocked. No human approved that decision."

**Step 5 — Hardened config (20 sec)**
Same evolution runs against hardened config. All four generations hold.

> "Hardened config. Four generations. Zero breaches. Cleared for deployment."

**Step 6 — Close (15 sec)**
> "This is what ships before every AI agent touches production. Evolutionary pressure, automated, every deploy."

---

## Judging Criteria — How We Now Score

| Criterion | How OpenZerg hits it |
|-----------|---------------------|
| **Autonomy** | Fires on container spin-up, runs 4 generations, makes deployment decision — zero human input |
| **Idea** | Every company shipping AI agents has this problem. No existing tool does evolutionary sandbox testing. |
| **Technical** | Real K8s pods, real probes, genetic mutation loop, fitness scoring, SQLite history, WebSocket streaming |
| **Tool Use** | Datadog (metrics + deployment alerts) + Nimble (live CVE feed → probe generation) |
| **Demo** | 3-minute generational narrative — waves of attacks, breach on permissive, clean on hardened |

---

## Build Priority Order — Today

| Hour | Who | What |
|------|-----|------|
| 1 | Carson | Kind cluster up, permissive + hardened pod YAML |
| 1 | Alex | `fitness_score()` + `mutate()` + `run_generation()` in controller |
| 1 | Tali + James | Reframe narrative arc around generations. Wave banner in UI. |
| 2-3 | Alex + Carson | Wire real K8s pods into generation loop. Test Gen 1 end-to-end. |
| 2-3 | James + Tali | Fitness gradient on map. Update replay.json with generation events. |
| After lunch | All | Datadog + Nimble integration (30 min each). Pitch rehearsal. |

---

*Pivot decision: May 23 2026, 10:15 AM. OpenZerg v2.*
