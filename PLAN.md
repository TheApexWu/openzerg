# OpenZerg — Visualization Layer Prep Plan

**Status:** Revision 2 (May 20 2026) — incorporates AI review feedback
**Demo date:** Saturday May 23 2026
**Repo:** github.com/lifesized/openzerg (private)

---

## Changelog from revision 1 → 2

Adopted from review:
- **Unified event pipeline:** live WebSocket + replay JSON both feed the same `normalizeEvent()` → same downstream subscribers (map, kubectl pane, terminal pane). Was parallel infrastructure, now one pipe.
- **Event schema is `started` / `log` / `result`,** not just final results. Drives all 3 panels naturally.
- **Pacing queue** drains the event stream at 300–700ms between visible impacts. Live pods may burst or stall; queue keeps the map readable.
- **URL-driven modes:** `?mode=live|replay|auto`. `auto` tries live, falls back to replay on socket failure.
- **3-panel host shell: shell owns the event pipeline; map renders in an iframe driven via one-way `postMessage`.** The reviewer's "single-page" intent was about a single source of truth for the pipeline — that property is preserved by having the shell own normalizeEvent / pacingQueue / eventBus and forward already-canonical events to the map. Cheaper than refactoring the Phaser scene into a mount-into-div module. If postMessage causes drift in practice, we collapse to true inline as a Friday-night fallback.
- **Canonical enums pinned** with a frontend normalizer for minor variations.
- **WebSocket contract** sent to Alex this week, not negotiated Saturday morning.

---

## 1. What is this project

**OpenZerg** is a distributed containment stress-testing tool for AI agent sandboxes. Pitch in one sentence: *"AI agents need system access to be useful. Every sandbox claims to be secure. Nobody tests them. We built the test."*

It points at a local Kubernetes cluster (Kind), spawns 20–30 attacker pods, each probing a different escape vector (filesystem, network, privilege escalation, secrets, resource exhaustion). Pods stream results to a browser dashboard with three panels: kubectl-style pod list (left), RTS-style containment map (center), raw exploit stdout (right). The center panel is the wow moment.

## 2. Context that shapes every decision

### 2.1 The hackathon
- **Event:** Agentic Engineering Hack, Saturday May 23 2026
- **Host:** tokens& (community-led)
- **Location:** Datadog NYC office
- **Schedule:** 9:00 doors → 9:30 kickoff → 12:30 lunch → 16:30 due → 19:00 awards
- **Build window: ~6 hours** (7 minus lunch)
- **Team size cap:** 4 (we are 3)
- **Sponsors:** DeepMind, Datadog, Nimble, ClickHouse, Evolution Equity Partners, Senso, Luminai

### 2.2 The team (3 people)
- **Carson Weeks** — Infra (Kind, pod specs, NetworkPolicy, lifecycle)
- **Alex Wu** — Product + attacks (FastAPI controller, attack scripts, WebSocket, SQLite)
- **Plan author (me)** — Viz layer (Phaser 3 map, 3-panel demo shell, pitch). Senior frontend / AI engineer

### 2.3 Judges (~10 total, mixed engineering / VC / sales)
- **Engineering credibility:** Thor Schaeff (DeepMind), Subodh Chaturvedi (Airbyte), Nataly Merezhuk (ClickHouse), Andrii Kovalchuk (WeSoftYou)
- **Commercial framing:** Samantha Feuer (Evolution Equity), Adam Stevens (Nimble), Samuel Eyob (Postral)
- **Natural-buyer bullseye:** Raymond Lin (Crosby — AI law firm), Rushikesh Akhare (Luminai)
- Plus: Linbing Wang (Freeport)

### 2.4 Hard constraints
1. **Offline-capable.** Last hackathon's wifi failed. Everything runs from a single laptop with no network.
2. **Demo polish > research novelty.** tokens& weights storytelling.
3. **Reversibility.** If Kind cluster fails mid-pitch, demo must still complete.
4. **Time.** ~6 hours build day. Saturday is integration + rehearsal, not new code. Feature-freeze Friday night.
5. **Asset legality.** RA2 was never freewared. Ripped assets fine for one-day demo, never to a public repo.

## 3. Architecture

```
            ┌─────────────────────────────────────────┐
            │  index.html (single-page, shared scope) │
            ├─────────────────────────────────────────┤
            │                                         │
            │   ModeSwitch (URL: live|replay|auto)    │
            │       ↓                ↓                │
            │   LiveSource       ReplaySource         │
            │   (WebSocket)      (JSON + timer)       │
            │       └────────┬───────┘                │
            │                ↓                        │
            │       normalizeEvent(raw)               │
            │       (canonical enums + schema check)  │
            │                ↓                        │
            │       PacingQueue (drain 300–700ms)     │
            │                ↓                        │
            │       EventBus (window.dispatchEvent)   │
            │       ┌────────┼────────┐               │
            │       ↓        ↓        ↓               │
            │   kubectlPane mapPane terminalPane      │
            │   (Phase 1.5) (built)  (Phase 1.4)      │
            │                                         │
            └─────────────────────────────────────────┘
```

### Canonical event schema (frontend-side)

```js
// EventBus emits canonical events of these three shapes:

{ type: 'started', pod: 'attacker-fs-01', vector: 'fs_symlink',
  category: 'filesystem', t: 1716220000123 }

{ type: 'log', pod: 'attacker-fs-01',
  line: 'trying symlink /proc/1/root/etc/shadow', t: 1716220000456 }

{ type: 'result', pod: 'attacker-fs-01', vector: 'fs_symlink',
  category: 'filesystem', status: 'BREACH',
  evidence: 'read /etc/shadow via /proc/1/root',
  duration_ms: 1200, t: 1716220001323 }
```

Canonical enums:
- `category`: `filesystem` | `network` | `privilege` | `secrets` | `resource`
- `status`: `HELD` | `BREACH`

`normalizeEvent()` accepts minor variants (uppercase categories, `held`/`hold`, missing `t`) and emits canonical events. If category/status unknown, the event is dropped with a console warning, not crashed.

### Pacing queue
Events from either source push onto a FIFO queue. A drain loop pops one event every 300–700ms (random jitter) and dispatches to the bus. If the queue empties, drain is paused until new events arrive. If the queue grows past ~30 events (live burst), drain accelerates to 200ms to catch up but never drops events. `log` events bypass pacing — they flow straight to the terminal pane because reading scrolling text doesn't need dramatic timing. Only `started` and `result` are paced (those drive the visual map and kubectl pane).

## 4. What is already built (as of May 20)

- Private GitHub repo: `github.com/lifesized/openzerg`
- `openzerg-map.html` — single-file Phaser 3 scene:
  - Per-category attacker FX (fork-bomb, lightning, glitch bars, escalation, secrets siphon)
  - Permanent breach scars pinned to base
  - Asset manifest at top (`./assets/{sprites,audio,music}/`)
  - Graceful procedural fallback if any asset missing
  - Audio system: voices on spawn/impact, music toggle, 6-track Klepacki playlist
  - Console API: `window.OpenZerg.{ping, swarm, send, cycleMode, toggleMusic, ...}`
- `vendor/phaser.min.js` self-hosted (no CDN dependency)
- `.gitignore` blocks `assets/` and scratch files

Note: the current `openzerg-map.html` directly wires its own WebSocket. After Phase 1.1 the map will move to being an event-bus subscriber driven by the host shell. The `window.OpenZerg.send()` API stays as the seam.

## 5. The plan

### Phase 0 — Done (May 20)
See section 4.

### Phase 1 — Prep window (May 20–22, evenings, async)

| # | Task | Owner | Est. | Dep |
|---|---|---|---|---|
| 1.0 | **Send WebSocket contract to Alex** (draft in §9). Lock schema, URL, event types this week | You + me to draft | 15 min | none |
| 1.1 | **Event pipeline core** — `eventBus`, `normalizeEvent()`, `pacingQueue`, `LiveSource`, `ReplaySource`, `ModeSwitch` from URL params | Me | ~1 hr | none |
| 1.2 | **Refactor map** to subscribe to event bus instead of owning its own WebSocket. Keep `OpenZerg.send()` as a seam for tests | Me | ~20 min | 1.1 |
| 1.3 | **`replay.json` curated narrative** — ~90-second story arc: opens with holds → first surprising breach → escalation → swarm finale → final scarred state. Started/log/result events for ~30 pods | Me | ~45 min | 1.1 |
| 1.4 | **Terminal pane (right)** — event-bus subscriber, renders `[fs-01] trying symlink ...` style lines from `log` events, color-coded by category | Me | ~20 min | 1.1 |
| 1.5 | **Kubectl pane (left)** — event-bus subscriber, pod table with Pending → Running → Completed transitions driven by `started` + `result` events | Me | ~25 min | 1.1 |
| 1.6 | **3-panel host shell** — `index.html` single-page flexbox layout, embeds map + terminal + kubectl as inline DOM, owns the bus | Me | ~40 min | 1.1–1.5 |
| 1.7 | **Screenshot button** — captures full 3-panel state as PNG. Money frame for slides | Me | ~20 min | 1.6 |
| 1.8 | **Narration overlay** — optional subtitle band tied to specific event timestamps in `replay.json` | Me | ~30 min | 1.3, 1.6 |
| 1.9 | **Source RA2 assets** locally (sprites + voices + 6 Klepacki tracks) | You | 1–2 hr | none |
| 1.10 | **Senso research** — 30 min | You | 30 min | none |
| 1.11 | **Pitch script v1** — full 90-sec script anchored to specific replay event timestamps | Me | ~30 min | 1.3 |

Total prep coding: **~5 hours** spread over 2 evenings. Slight increase vs rev1 (~4hr) because event pipeline + normalizer + queue are new structural code, but downstream tasks shrink because all 3 panels are just subscribers.

### Phase 2 — Friday May 22 dry run
- Carson runs Kind end-to-end on his actual laptop
- I run the full 3-panel demo from cold boot in all three URL modes (`live`, `replay`, `auto`)
- Replay mode dry run with cluster intentionally OFF — looks identical
- Final commit "freeze for demo" Friday night, no feature changes Saturday

### Phase 3 — Saturday morning pre-9:30
- Laptop fully charged, `caffeinate -dims` running
- Kind pre-pulled, browser bookmarks for both demo URLs
- Slack / OS update / Spotlight disabled
- Backup laptop (mine) with same repo cloned in case primary fails

### Phase 4 — Saturday build day 9:30–16:30
| Block | Time | Focus |
|---|---|---|
| 9:30–10:30 | 60min | Connect to Alex's live WS in `mode=live`. Validate schema matches contract. Fix normalizer for any drift |
| 10:30–12:30 | 2hr | Visual polish + pacing tuning against real burst behavior |
| 12:30–13:30 | 60min | Lunch (don't skip) |
| 13:30–15:30 | 2hr | Pitch script finalization + 3 rehearsals at actual demo station, both live and replay |
| 15:30–16:30 | 1hr | Buffer for fixes |
| 16:30 | — | Projects due |

## 6. Demo strategy (revised per feedback)

**Build live + replay, rehearse both, pick by reliability:**

1. Live integration is built first and tested against Alex's real cluster as soon as it's up
2. Replay is built second using the same canonical event shape — same code path renders both
3. Both are rehearsed at the demo station Saturday afternoon
4. **On stage, use live ONLY if it has passed multiple consecutive clean rehearsals.** Otherwise replay
5. If using replay on stage, frame it honestly: *"This is a deterministic playback of the same event stream emitted by the live Kubernetes run — same code path as live, just paced for a 90-second narrative."*
6. Live can be shown during Q&A if judges want to see it

This gives real-system credibility without gambling the pitch.

## 7. Open questions (revision 2)

| # | Question | Resolution path |
|---|---|---|
| Q1 | Does Alex emit `started`/`log`/`result` separately, or only `result`? | Contract in §9 |
| Q2 | Are pod names included in events? | Contract |
| Q3 | Can we guarantee scripted runs with at least a few breaches? | Contract |
| Q4 | Does live run complete fast enough for 90 sec? | Contract + rehearsal |
| Q5 | Does Kind networking actually enforce NetworkPolicy, or are network probes just reachability tests? | Carson must answer — affects pitch credibility |
| Q6 | Senso — what do they do, is there a pitch angle? | Your 30-min research |

Old Q3 (iframe vs inline) → resolved: **single-page inline** because of shared event bus. Old Q4 (replay source) → resolved: **curated narrative arc using canonical event shape**.

## 8. Risk register

| Risk | Mitigation |
|---|---|
| Kind cluster fails on demo machine | `mode=auto` falls back to replay automatically; same render path |
| Hackathon wifi flaky | Phaser self-hosted, all assets local, replay JSON local |
| Asset files don't land in time | Procedural fallback in map already wired |
| Live event burst overwhelms map | Pacing queue caps display rate at 300–700ms; never drops events |
| Schema drift between Alex's WS and our normalizer | Normalizer is loose on input, strict on output; console warns on unknown enums |
| Pitch goes off-script | Narration overlay (#1.8) anchored to replay event timestamps |
| Judges miss the wow moment | Screenshot button (#1.7), screenshot taken pre-pitch as slide backup |
| Browser blocks autoplay music | Music defaults OFF, click = user gesture |
| Voice clips overlap | Throttled to 350ms cooldown |
| Ripped RA2 assets leak to public repo | `assets/` gitignored, repo private |
| Live run completes too fast (10 sec, no drama) | Pacing queue stretches it; replay is alternative |
| Live run too slow (3 min, judges lose attention) | Replay is alternative |

## 9. WebSocket contract — draft message to Alex

> Hey Alex — front-end is going to support both your live WS and a curated replay file, and they'll feed the same render pipeline. To lock that in cleanly I need to pin a few things on your end. None of this should be much work — mostly confirming the shape you're already emitting.
>
> **URL:** `ws://localhost:8000/ws` — confirm?
>
> **Event types I'm hoping for** (three separate messages per pod, in order):
>
> ```json
> { "type": "started", "pod": "attacker-fs-01", "vector": "fs_symlink", "category": "filesystem" }
> { "type": "log",     "pod": "attacker-fs-01", "line": "trying symlink /proc/1/root/etc/shadow" }
> { "type": "result",  "pod": "attacker-fs-01", "vector": "fs_symlink",
>   "category": "filesystem", "status": "BREACH",
>   "evidence": "read /etc/shadow via /proc/1/root", "duration_ms": 1200 }
> ```
>
> If you can only emit `result` events, that's fine — I'll synthesize `started`/`log` on the frontend. But having all three makes the 3-panel demo *much* stronger (kubectl pane shows Pending→Running, stdout pane scrolls real attempts, map shows impact).
>
> **Canonical enums I'll normalize against:**
> - `category`: `filesystem` | `network` | `privilege` | `secrets` | `resource`
> - `status`: `HELD` | `BREACH`
>
> I have a normalizer so casing variations / aliases won't break me, but pick these as canonical if you have a choice.
>
> **A few yes/no questions:**
> 1. Pod names included in every event? (Will use them to correlate events to rows in the kubectl pane.)
> 2. Multiple events per pod, or one final result? (Either works, just need to know.)
> 3. Can you guarantee at least a few `BREACH` results in any given run? Demo loses drama if every probe holds.
> 4. Roughly how long does a full run take end-to-end? Aiming for ~60–90 sec of visual story.
> 5. Does Kind actually enforce NetworkPolicy in your setup, or are the network probes just reachability tests? Carson would also know this — affects how I describe network impacts in the pitch.
>
> Frontend will do its own pacing (drain queue at 300–700ms per impact) so don't worry if pods finish in bursts. Just emit events whenever they happen.

## 10. Pitch framing (working draft v1, will refine in 1.11)

> "AI agents need system access to be useful. Every sandbox claims to be secure. Nobody tests them — we built the test.
>
> [point left] Real Kubernetes, real pods, scheduling in real time.
>
> [point center] Each unit is a Python script probing a different escape vector — filesystem, network, privilege escalation, secrets, resource exhaustion. Green shield: containment held. Red scar: breach.
>
> [point right] Raw exploit output. The attacks are real.
>
> [final state] This scarred container is the fingerprint of your sandbox's security posture. Crosby is running agents on legal documents right now. Kilo's Gas Town is orchestrating fleets of coding agents in production for 1.4M users. Neither ships with this. We built the verification layer every agent deployment needs before production."

If running on replay, splice in after "real Kubernetes" line: *"What you're watching now is a deterministic playback of the same event stream the live cluster emits — same code path, paced for 90 seconds."*

## 11. Recommended build order if greenlit

`1.0 (contract to Alex)` → `1.1 (event pipeline core)` → `1.2 (refactor map to subscriber)` → `1.3 (replay.json)` → `1.4 (terminal pane)` → `1.5 (kubectl pane)` → `1.6 (host shell)` → `1.7 (screenshot)` → `1.8 (narration)` → `1.11 (pitch script v1)`

You handle `1.9` (assets) and `1.10` (Senso) async on your own time.
