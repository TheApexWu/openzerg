# OpenZerg — Team Onboarding

**Hackathon:** Google DeepMind x Datadog Agentic Engineering Hack
**Date:** Saturday May 23, 2026 | 9:00 AM doors, 9:30 kickoff, 4:30 PM due
**Location:** Datadog NYC office
**Build window:** ~6 hours (minus lunch 12:30-1:30)

---

## One Sentence

**AI agents need system access to be useful. Every sandbox claims to be secure. Nobody tests them. We built the test.**

In layman's: we throw 30 real attacks at an AI agent's sandbox and show you exactly where it breaks — in 90 seconds, on one laptop.

---

## What Is OpenZerg

A distributed containment stress-testing tool for AI agent sandboxes. Point it at a containerized agent environment (Kubernetes), spawn 20-30 attacker pods each probing a different escape vector (filesystem, network, privilege escalation, secrets, resource exhaustion), and stream results to a live 3-panel visualization.

The demo has three panels:
- **Left:** kubectl-style live pod view (pods appearing, scheduling, completing — proves the infra is real)
- **Center:** RTS-style containment map (probes animate inward, green flash = held, red scar = breach)
- **Right:** Raw attack logs (real exploit stdout, scrolling in real time)

The center map is the wow moment. The three-panel layout proves it's not a mocked animation.

---

## Team (4 people)

| Person | Role | Owns |
|--------|------|------|
| **Carson Weeks** | Infrastructure | Kind cluster setup, pod spec templates, NetworkPolicy/SecurityContext/PodSecurityPolicy configs, node affinity, pod lifecycle (spawn/logs/cleanup), permissive vs hardened sandbox configs |
| **Alex Wu** | Product + Backend | FastAPI controller, attack script library (15 vectors), WebSocket server (`ws://localhost:8000/ws`), SQLite result storage, historical comparison API |
| **James Burke** | Visualization + Frontend | Phaser 3 containment map, 3-panel demo shell (`index.html`), event pipeline (normalizeEvent → PacingQueue → EventBus), replay system, RA2 aesthetic, pitch script |
| **Sunny** | Attacks + Demo Polish | Attack script expansion (stretch from 15 to 30 vectors), demo rehearsal, pitch deck/slides, sponsor mention integration, screenshot/recording for submission |

### Key interfaces between roles:
- **Carson → Alex:** Pod spec templates + how to spawn/watch pods via `kubernetes` Python client
- **Alex → James:** WebSocket at `ws://localhost:8000/ws` emitting `started`/`log`/`result` events (schema below)
- **James → Everyone:** The frontend consumes events and renders all 3 panels. No changes needed from James unless schema drifts.
- **Sunny:** Can write attack scripts independently (just Python, ~50 lines each, structured JSON stdout). Can also help Carson with hardened config testing.

---

## Event Schema (THE CONTRACT)

Every event flowing from backend to frontend follows one of three shapes:

```json
{ "type": "started", "pod": "attacker-fs-01", "vector": "fs_symlink", "category": "filesystem", "t": 1716220000123 }

{ "type": "log", "pod": "attacker-fs-01", "line": "trying symlink /proc/1/root/etc/shadow", "t": 1716220000456 }

{ "type": "result", "pod": "attacker-fs-01", "vector": "fs_symlink", "category": "filesystem", "status": "BREACH", "evidence": "read /etc/shadow via /proc/1/root", "duration_ms": 1200, "t": 1716220001323 }
```

**Canonical enums:**
- `category`: `filesystem` | `network` | `privilege` | `secrets` | `resource`
- `status`: `HELD` | `BREACH`

The frontend normalizer is lenient (accepts uppercase categories, `held`/`hold`/`pass`, missing `t`), but emit canonical values if possible.

---

## Architecture

```
[Browser: index.html]
    │
    │ WebSocket ws://localhost:8000/ws
    │
[FastAPI Controller — backend/controller.py]
    │
    │ kubernetes Python client
    │
[Kind Cluster — fully local]
    ├── Node 1 (TARGET): agent sandbox pod
    └── Nodes 2-3 (ATTACKERS): 20-30 attack pods
         └── Each pod: ~50-line Python script → JSON stdout
```

Everything runs on ONE laptop. No cloud, no wifi dependency, no sponsor API dependency.

---

## What's Already Built

| Component | Status | File |
|-----------|--------|------|
| Containment map (Phaser 3, RA2 sprites, breach scars, audio) | Done | `openzerg-map.html` |
| 3-panel demo shell (kubectl + map + terminal, event pipeline) | Done | `index.html` |
| Replay system (90-sec curated narrative, 30 pods) | Done | `replay.json` |
| Backend scaffold (FastAPI + WS + 15 attack vectors + SQLite) | Scaffold | `backend/` |
| Real K8s pod orchestration | TODO | Carson's pod specs + Alex wiring |
| Hardened vs permissive sandbox configs | TODO | Carson |
| Attack scripts (real, running in pods) | TODO | `backend/attacks/*.py` (4 real scripts exist as templates) |

---

## What Needs to Happen

### Before Saturday (Tonight/Friday)
1. **Carson:** Kind cluster running end-to-end on his laptop. Permissive + hardened configs ready. Pod spec templates for attacker pods (minimal Python container + mounted script).
2. **Alex:** Wire `backend/controller.py` to real `kubernetes` Python client (replace pseudo spawn with actual `v1.create_namespaced_pod()`). Test against Carson's cluster.
3. **James:** Final replay.json polish. Narration overlay if time permits.
4. **Sunny:** Read this doc. Clone repo. Write 2-3 additional attack scripts following the template in `backend/attacks/`. Each is ~50 lines of Python that outputs JSON to stdout.

### Saturday Morning (9:00-9:30, before kickoff)
- Laptop charged, `caffeinate -dims` running
- Kind cluster pre-pulled on demo machine
- Browser bookmarked for `http://localhost:8000/static/index.html?mode=auto`
- Verify: `cd backend && ./run.sh` starts the controller

### Saturday Build Day
| Time | Focus |
|------|-------|
| 9:30-10:30 | Connect live WS to real cluster. Validate schema. Fix normalizer drift. |
| 10:30-12:30 | Visual polish + pacing tuning against real pod burst behavior |
| 12:30-1:30 | Lunch (don't skip) |
| 1:30-3:30 | Pitch script finalization + 3 rehearsals at actual demo station |
| 3:30-4:30 | Buffer for fixes |
| 4:30 | Projects due |

---

## How to Write an Attack Script

Each attack is a standalone Python script (~50 lines) that runs inside a K8s pod. It:
1. Prints `{"type": "log", "line": "..."}` JSON lines as it probes
2. Prints one final `{"type": "result", "vector": "...", "category": "...", "status": "HELD|BREACH", "evidence": "...", "duration_ms": N}` and exits

See `backend/attacks/fs_symlink.py` for a full template. The pattern:

```python
#!/usr/bin/env python3
import json, sys, time

def run():
    start = time.time()
    print(json.dumps({"type": "log", "line": "what I'm trying..."}))
    sys.stdout.flush()

    try:
        # ... actual probe logic ...
        status, evidence = "HELD", "why it was blocked"
    except Exception as e:
        status, evidence = "HELD", str(e)

    print(json.dumps({
        "type": "result",
        "vector": "my_vector_name",
        "category": "filesystem",  # or network/privilege/secrets/resource
        "status": status,
        "evidence": evidence,
        "duration_ms": int((time.time() - start) * 1000),
    }))
    sys.stdout.flush()

if __name__ == "__main__":
    run()
```

**Vectors still needed** (Sunny — pick from these):
- `sec_configmap_read` — query K8s API for ConfigMaps
- `priv_nsenter` — nsenter into host PID namespace
- `fs_mount_host` — attempt to mount host paths
- `res_memory_bomb` — allocate until OOMKilled
- `res_disk_fill` — write until ENOSPC
- `net_dns_exfil` — DNS tunneling attempt
- `priv_capabilities` — check for dangerous Linux capabilities (CAP_SYS_ADMIN, CAP_NET_RAW)

---

## Demo Strategy

**Build live + replay, rehearse both, pick by reliability:**
1. Live integration tested against real Kind cluster
2. Replay is built using the same event schema — same render path
3. Both rehearsed at demo station Saturday afternoon
4. **On stage: use live ONLY if it passed multiple clean rehearsals.** Otherwise replay.
5. If replay: "This is a deterministic playback of the same event stream the live cluster emits — same code path, paced for 90 seconds."
6. Live shown during Q&A if judges want it

**The money shot:** Run on permissive config (3 breaches). Then run on hardened config (all green). The diff tells the story.

---

## Sponsor Alignment

| Sponsor | What We Say | Alignment |
|---------|-------------|-----------|
| **Datadog** (host) | "In production, probe telemetry pipes into Datadog for real-time alerting." | Strong — we're monitoring security |
| **DeepMind** (co-host) | "Target agent can run any LLM — Gemini, GPT, local. Containment test is model-agnostic." | Medium |
| **Evolution Equity** | Cybersecurity VC. Agent containment IS their thesis. | Bullseye |
| **Luminai** | Builds AI agents for enterprise. They need this. Rushikesh is a judge. | Bullseye |
| **ClickHouse** | "At scale, results go to ClickHouse for sub-second aggregation across thousands of runs." | Medium |
| **Nimble** | "Nimble feeds real-world data to the target agent." | Weak but present |

**Key judge:** Raymond Lin (Crosby — Sequoia-backed AI law firm, $400M val). Agents handling sensitive contract data. Containment is existential for them. Pitch TO him.

---

## Quick Start

```bash
# Clone
git clone https://github.com/lifesized/openzerg
cd openzerg

# Frontend only (replay mode — no backend needed)
open index.html
# or: python3 -m http.server 3000 && open http://localhost:3000

# Backend (scaffold mode — fake pods, real WebSocket)
cd backend
pip install -r requirements.txt
uvicorn controller:app --host 0.0.0.0 --port 8000 --reload

# Frontend connects automatically to ws://localhost:8000/ws
# Open: http://localhost:8000/static/index.html?mode=live
```

---

## Risks

| Risk | Mitigation |
|------|------------|
| No wifi / sponsor APIs down | Entire stack runs offline on one laptop |
| Kind cluster won't start | Pre-build images day before. Replay mode is identical visually. |
| Pods too slow | 10-second timeout per pod, kill stragglers |
| Not enough attack vectors | Ship 15 solid ones, not 30 weak ones |
| Browser blocks autoplay music | Music defaults OFF, click = user gesture |

---

## Repo Structure

```
openzerg/
├── index.html              # 3-panel demo shell (kubectl + map + terminal)
├── openzerg-map.html       # Phaser 3 containment map (center panel)
├── replay.json             # Curated 90-sec replay narrative
├── PLAN.md                 # James's detailed architecture plan
├── ONBOARDING.md           # This file — send to new team members
├── vendor/phaser.min.js    # Self-hosted Phaser (no CDN dependency)
├── assets/                 # RA2 sprites + audio (gitignored, local only)
└── backend/
    ├── controller.py       # FastAPI + WebSocket + K8s orchestration
    ├── run.sh              # Quick start script
    ├── requirements.txt    # Python dependencies
    └── attacks/
        ├── __init__.py     # Attack vector registry (15 vectors defined)
        ├── fs_symlink.py   # Template: filesystem symlink traversal
        ├── net_k8s_api.py  # Template: K8s API server probe
        ├── priv_docker_sock.py  # Template: Docker socket escape
        └── res_fork_bomb.py     # Template: PID cgroup limit test
```
