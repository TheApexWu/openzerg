# OpenZerg — Visualization Layer Prep Plan

**Status:** Draft for AI review, May 20 2026
**Demo date:** Saturday May 23 2026
**Audience for this doc:** Another AI being asked to review the plan cold

---

## 1. What is this project

**OpenZerg** is a distributed containment stress-testing tool for AI agent sandboxes. The pitch in one sentence: *"AI agents need system access to be useful. Every sandbox claims to be secure. Nobody tests them. We built the test."*

It points at a local Kubernetes cluster (Kind), spawns 20–30 attacker pods across the cluster, each running a small Python script that probes a different escape vector (filesystem, network, privilege escalation, secrets, resource exhaustion). Pods return `{vector, category, status: HELD|BREACH, evidence, duration_ms}` over WebSocket to a browser dashboard.

The dashboard is three panels side-by-side:
- **Left:** live `kubectl get pods` output — proves the K8s orchestration is real
- **Center:** RTS-style containment map (this document is about this panel)
- **Right:** raw exploit stdout terminal logs

The center panel is described in the project's own pitch doc as "the wow moment." It uses a Real-Time Strategy game aesthetic (Red Alert 2 / Command & Conquer mental model). A static sandbox base sits in the middle. Attacker units spawn at the screen edges and march inward. On impact: green shield pulse for `HELD`, red fracture + screen shake + permanent scar for `BREACH`.

## 2. Context that shapes every decision

### 2.1 The hackathon
- **Event:** Agentic Engineering Hack, Saturday May 23 2026
- **Host:** tokens& (community-led, not the sponsors)
- **Location:** Datadog NYC office, 620 8th Ave
- **Schedule:** 9:00 doors → 9:30 kickoff → 12:30 lunch → 16:30 projects due → 19:00 awards
- **Build window: 7 hours minus lunch = ~6 hours**
- **Team size cap:** 4 (we are 3)
- **Prize pool:** $45k+
- **Sponsors:** DeepMind, Datadog, Nimble, ClickHouse, Evolution Equity Partners, Senso, Luminai

### 2.2 The team (3 people)
- **Carson Weeks** — Infrastructure (Kind cluster, pod specs, NetworkPolicy, lifecycle)
- **Alex Wu** — Product + attacks (FastAPI controller, attack script library, WebSocket server, SQLite results)
- **Me (the plan author)** — Visualization layer (Phaser 3 containment map, 3-panel demo shell, pitch narrative). Senior frontend / AI engineer. This is my lane and I should not get pulled into Kubernetes YAML debugging.

### 2.3 Judges (~10 total, mixed engineering/VC/sales)
The pitch must land for multiple audiences:
- **Engineering credibility:** Thor Schaeff (DeepMind), Subodh Chaturvedi (Airbyte), Nataly Merezhuk (ClickHouse), Andrii Kovalchuk (WeSoftYou)
- **Commercial framing:** Samantha Feuer (Evolution Equity VC), Adam Stevens (Nimble Sales), Samuel Eyob (Postral CEO)
- **Natural-buyer bullseye:** Raymond Lin (Crosby — AI law firm where agent containment is existential), Rushikesh Akhare (Luminai)
- Plus: Linbing Wang (Freeport)

### 2.4 Hard constraints
1. **Offline-capable.** Last hackathon's wifi failed. Everything must run from a single laptop with no network.
2. **Demo polish > research novelty.** tokens& is community-led, weights storytelling.
3. **Reversibility.** If Kind cluster fails mid-pitch on stage, demo must still complete.
4. **Time.** 6 hours of build day. Most of Saturday is integration + rehearsal, not new code. New features after Friday night = banned.
5. **Asset legality.** RA2 was never freewared by EA. Ripped sprites/voices/soundtrack are fine for one-day demo, must not be committed to a public repo or used in post-hack social.

## 3. What is already built (as of May 20)

- Private GitHub repo: `github.com/lifesized/openzerg`
- `openzerg-map.html` — single-file Phaser 3 scene with:
  - Per-category attacker FX (fork-bomb swarm, lightning, glitch bars, escalation beam, secrets siphon)
  - Permanent breach scars pinned to base sprite, repaint on resize
  - Asset manifest at top — sprites in `./assets/sprites/`, voices in `./assets/audio/`, music in `./assets/music/`
  - **Graceful procedural fallback:** if any asset file is missing, procedural Phaser-graphics sprite renders instead. Demo runs with zero assets.
  - Audio system: voice cues on spawn + impact, music toggle (defaults OFF), 6-track Klepacki playlist support
  - Console API: `window.OpenZerg.{ping(cat), swarm(n), send(payload), cycleMode(), toggleMusic(), nextTrack(), assetReport(), reset()}`
  - HUD: title, cluster status, current music track, probe/held/breach counters
  - Bottom control bar: 5 category buttons + SWARM x20 + MODE toggle + MUSIC + NEXT TRACK
- `vendor/phaser.min.js` — Phaser 3.80.1 self-hosted locally (no CDN dependency)
- `.gitignore` — blocks `assets/` (ripped RA2 IP), `Untitled` scratch, OS files

### Engine choice history
- Considered: **chrono-divide** (browser RA2 clone in TS). **Ruled out** after investigation:
  - Engine is closed-source. Only public repos are `chronodivide/mod-sdk` and `chronodivide/game-api-playground` (headless bot wrapper, last commit Oct 2024).
  - Requires legal RA2 mix files on every demo machine via `MIX_DIR="C:\path_to_ra2_install"`.
  - No browser embed API, no WebSocket bridge to rendered client.
  - Estimated effort: structurally impossible in 5 days of prep.
- **Decision:** Phaser 3 single-file scene + RA2 asset loading. Get the RA2 aesthetic at 5% of the integration risk.

## 4. The plan

### Phase 0 — Done (May 20)
See section 3.

### Phase 1 — Prep window (May 20–22, evenings, async)

| # | Task | Owner | Est. | Dep |
|---|---|---|---|---|
| 1 | Source RA2 assets (sprites + voices + 6 Klepacki tracks) into `./assets/{sprites,audio,music}/` per manifest in HTML | Plan author | 1–2 hr | none |
| 2 | Demo replay system: `replay.json` with 60–90 pre-recorded payloads + `OpenZerg.replay()` API. Insurance if Kind dies on stage | Plan author | ~45 min | none |
| 3 | 3-panel host shell: `index.html` flexbox layout (left kubectl pane, center map, right stdout terminal) | Plan author | ~1 hr | none |
| 4 | Terminal widget for right pane — subscribes to `CustomEvent('openzerg:event')`, renders `[fs-01] trying symlink /proc/1/root` lines | Plan author | ~30 min | #3 |
| 5 | Fake kubectl left-pane — pod list with Pending → Running → Completed transitions synced to payload stream. Offline fallback for Carson's real version | Plan author | ~30 min | #3 |
| 6 | "Final state" screenshot button — captures canvas with all scars + counters as PNG. Money frame for slides / social | Plan author | ~20 min | done |
| 7 | Demo narration overlay — optional subtitle band synced to replay timeline | Plan author | ~30 min | #2 |
| 8 | Senso research — 30 min to understand the sponsor not in original Hack Plan | Plan author | 30 min | none |
| 9 | Phaser CDN → local copy | Plan author | done | done |

Total prep coding: ~4 hours, spread over 2 evenings.

### Phase 2 — Friday May 22 dry run
- Carson runs Kind cluster end-to-end on his actual laptop
- I run full 3-panel demo from cold boot — 90-sec pitch lands without keyboard mid-demo
- Replay mode dry run with cluster intentionally OFF — demo looks identical
- Asset bundle frozen Friday night, git commit "freeze for demo"

### Phase 3 — Saturday morning pre-9:30
- Arrive with laptop charged, Kind pre-pulled
- Browser bookmarks: file:// URLs for live and replay modes
- OS auto-update / Slack / Spotlight disabled to prevent demo interruption

### Phase 4 — Saturday build day 9:30–16:30 (6hr work)
| Block | Time | Focus |
|---|---|---|
| 9:30–10:30 | 60min | Wire to Alex's real WebSocket, confirm payload shape matches mock |
| 10:30–12:30 | 2hr | Visual polish + timing tuning against real payloads |
| 12:30–13:30 | 60min | Lunch (don't skip, pitch energy matters) |
| 13:30–15:30 | 2hr | Pitch script + 3 full rehearsals at actual demo station |
| 15:30–16:30 | 1hr | Buffer for fixes |
| 16:30 | — | Projects due |

## 5. Open questions

1. **Payload schema sync with Alex's controller** — does his API return exactly `{vector, category, status, evidence, duration_ms}`? Need to confirm before Friday.
2. **WebSocket URL** — is `ws://localhost:8000/ws` actually what Alex exposes? URL param override?
3. **3-panel host architecture** — iframe-per-panel (clean isolation, postMessage event bus) vs single-page inline (easier event wiring, coupled scope) vs hybrid (center map as iframe, sidebars as inline divs)?
4. **Replay data source** — hand-curated narrative arc (predictable pitch beats: opens with holds, first surprising breach, escalation, swarm, final scarring) vs real recorded run (authentic, less narratively shaped) vs both?
5. **Senso sponsor** — what do they do? Is there an angle for the pitch?

## 6. Risk register

| Risk | Mitigation |
|---|---|
| Kind cluster fails on demo machine | Replay mode (#2) renders identical demo offline |
| Hackathon wifi flaky | Phaser self-hosted (#9 done), assets local, no CDN |
| Asset files don't land in time | Procedural fallback already wired |
| 90-sec pitch goes off-script | Narration overlay (#7) carries the words |
| Judges miss the wow moment | Screenshot (#6) captured pre-pitch, shown on slide |
| Browser blocks autoplay music | Music defaults OFF, user click = gesture |
| Voice clips overlap into noise | Throttled to 350ms cooldown |
| Ripped RA2 assets in public repo | Repo is private, `assets/` gitignored |

## 7. Pitch framing (working draft)

> "AI agents need system access to be useful. Every sandbox claims to be secure. Nobody tests them — we built the test.
>
> [point to left panel] Real Kubernetes, real pods, scheduling in real time.
>
> [point to center] Each unit is a Python script probing a different escape vector — filesystem, network, privilege escalation, secrets, resource exhaustion. Green shield = containment held. Red scar = breach.
>
> [point to right] Raw exploit output. The attacks are real.
>
> [final state] This scarred container is the fingerprint of your sandbox's security posture. Crosby is running agents on legal documents right now. Kilo's Gas Town is orchestrating fleets of coding agents in production for 1.4M users. Neither ships with this. We built the verification layer every agent deployment needs before production."

## 8. What I want reviewed

If you (the reviewing AI) are reading this, the most useful feedback would be:

1. **Is the time allocation realistic?** 4hr prep + 6hr build day. Anything underbudgeted?
2. **What am I missing in the risk register?** Especially failure modes I haven't seen.
3. **Pitch framing** — is the closing line too narrow / too broad? Does the "Crosby + Gas Town" name-drop sequence land or feel forced?
4. **Open questions priority** — which of the 5 must be resolved before any more code, vs deferable?
5. **Replay vs live demo balance** — should the Saturday pitch lead with live cluster (real but risky) or curated replay (polished but less authentic)? Or both — live for technical judges, replay as backup?
6. **Architecture: iframe vs inline shell** — which would you pick and why?
7. **Anything missing entirely** — features, prep tasks, or judging-audience considerations not on this list.

Please be direct about cuts. The build window is too tight to carry passenger features.
