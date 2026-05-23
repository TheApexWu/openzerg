# OpenZerg — Build Instructions
**Version 2 — Evolutionary Red-Teaming · May 23, 2026**

---

## Table of Contents
1. [Quick Start](#1-quick-start)
2. [Environment Setup](#2-environment-setup)
3. [Running the Backend](#3-running-the-backend)
4. [Running the Frontend](#4-running-the-frontend)
5. [Sponsor Integrations](#5-sponsor-integrations)
6. [Writing Attack Scripts](#6-writing-attack-scripts)
7. [The Evolution Loop](#7-the-evolution-loop)
8. [API Reference](#8-api-reference)
9. [Repo Structure](#9-repo-structure)
10. [Demo Checklist](#10-demo-checklist)

---

## 1. Quick Start

```bash
# Clone
git clone https://github.com/lifesized/openzerg
cd openzerg

# Frontend only — no backend needed (replay mode)
open index.html

# Full stack
cd backend
source .venv/bin/activate
uvicorn controller:app --port 8000

# Open browser
open http://localhost:8000/static/index.html?mode=auto

# Trigger evolution run
curl -X POST "http://localhost:8000/api/test?config=permissive&generations=4"
```

---

## 2. Environment Setup

### Python venv (already done — just activate)
```bash
cd backend
source .venv/bin/activate
```

If rebuilding from scratch:
```bash
python3 -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt
```

**requirements.txt:**
```
fastapi>=0.115
uvicorn[standard]>=0.30
websockets>=12.0
kubernetes>=30.0
httpx>=0.27
```

### Ollama + Gemma (Carson's machine)
```bash
# Install Ollama
brew install ollama

# Pull Gemma 2B
ollama pull gemma:2b

# Start Ollama server (runs on localhost:11434)
ollama serve
```

Confirm working:
```bash
curl http://localhost:11434/api/tags
# Should show gemma:2b in the list
```

### Lapdog (Datadog tracing)
```bash
brew install datadog/lapdog/lapdog && lapdog claude
```
Open `http://localhost:9000` to see live traces of Gemma mutation calls.

### Environment Variables
```bash
# Optional — Nimble CVE seeding
export NIMBLE_API_KEY=your_key_here

# Optional — custom Ollama URL (default: localhost:11434)
export OLLAMA_URL=http://localhost:11434
```

---

## 3. Running the Backend

```bash
cd backend
source .venv/bin/activate
uvicorn controller:app --host 0.0.0.0 --port 8000
```

Kill existing server if port 8000 in use:
```bash
lsof -ti:8000 | xargs kill -9
```

With auto-reload (dev mode):
```bash
uvicorn controller:app --port 8000 --reload
```

---

## 4. Running the Frontend

The frontend is served by the backend at `http://localhost:8000`.

URL modes:
| URL | Behavior |
|-----|----------|
| `?mode=auto` | Tries live WebSocket, falls back to replay if down |
| `?mode=live` | Live WebSocket only |
| `?mode=replay` | Curated 90-sec replay, no backend needed |

**Demo URL:** `http://localhost:8000/static/index.html?mode=auto`

Frontend-only (no backend):
```bash
python3 -m http.server 3000
open http://localhost:3000?mode=replay
```

---

## 5. Sponsor Integrations

### Gemma 2B (Google/DeepMind) — Mutation Brain
Runs locally via Ollama. Called **once per generation** between waves — not per probe. Takes survivor evidence, returns mutated probe configs for the next generation.

Ollama endpoint used: `POST http://localhost:11434/api/generate`

If Gemma fails or times out, controller falls back to manual mutation automatically. Never blocks.

### Lapdog (Datadog) — LLM Observability
Traces every Gemma mutation call as a span. Shows input (survivors + evidence), output (mutations), latency, and cost per generation.

```bash
brew install datadog/lapdog/lapdog && lapdog claude
```

Open Lapdog at `http://localhost:9000` — keep this in a second browser tab during demo. Shows live traces updating as generations run.

### Nimble — Live CVE Seeding
Seeds Generation 1 with CVEs published this week from NVD/CISA. Adds real threat intel context to the first wave of probes.

```bash
export NIMBLE_API_KEY=your_key_here
```

Falls back gracefully if key is missing or API is slow — never blocks the evolution loop.

---

## 6. Writing Attack Scripts

Each attack script is a standalone Python file in `backend/attacks/`. It runs inside a K8s pod and outputs structured JSON to stdout.

### Template
```python
#!/usr/bin/env python3
"""Attack: my_vector — What it does"""
import json, sys, time

def run():
    result = {
        "vector": "my_vector",
        "category": "filesystem",  # filesystem|network|privilege|secrets|resource
        "status": "HELD",
        "evidence": "",
        "duration_ms": 0,
    }
    start = time.time()

    print(json.dumps({"type": "log", "line": "what I'm attempting..."}))
    sys.stdout.flush()

    try:
        # --- probe logic ---
        result["status"] = "BREACH"
        result["evidence"] = "what was exposed"
    except PermissionError:
        result["evidence"] = "permission denied"
    except FileNotFoundError:
        result["evidence"] = "path not accessible"
    except OSError as e:
        result["evidence"] = f"OS error: {e}"

    result["duration_ms"] = int((time.time() - start) * 1000)
    print(json.dumps({"type": "result", **result}))
    sys.stdout.flush()

if __name__ == "__main__":
    run()
```

### After writing
1. Drop file in `backend/attacks/your_vector.py`
2. Add entry to `ATTACK_VECTORS` list in `backend/attacks/__init__.py`
3. Include `params` dict with `target_path`, `technique`, `timeout` — these are the mutable genome fields

### Fitness scoring (automatic)
The controller scores every result automatically. No changes needed in the script:
| Evidence contains | Fitness |
|---|---|
| BREACH status | 1.0 |
| "reachable", "connected", "200 ok" | 0.6 |
| "permission denied", "found", "accessible" | 0.4 |
| "timeout", "refused", "blocked" | 0.1 |
| anything else | 0.0 |

---

## 7. The Evolution Loop

```
Generation 1
  Spawn all 18 probes in parallel
  Score each → fitness 0.0–1.0
  Filter survivors (fitness > 0.1)
  Broadcast generation_complete

  ↓ ONE Gemma call (2-3 sec)
  Gemma reads survivors + evidence
  Returns 15 mutated probe configs

Generation 2
  Spawn mutated probes
  ...repeat up to 4 generations

Stop when: BREACH found OR no survivors OR max generations reached
```

**Trigger via API:**
```bash
# 4 generations, permissive config
curl -X POST "http://localhost:8000/api/test?config=permissive&generations=4"

# 2 generations, hardened config
curl -X POST "http://localhost:8000/api/test?config=hardened&generations=2"
```

**Trigger via WebSocket:**
```json
{ "action": "run_test", "config": "permissive", "generations": 4 }
```

**New event types (v2):**
```json
{ "type": "generation_start", "generation": 2, "population": 15, "run_id": "...", "t": ... }
{ "type": "generation_complete", "generation": 2, "survivors": 6, "breaches": 1, "t": ... }
{ "type": "run_complete", "generations": 3, "total": 45, "breaches": 3, "held": 42, "t": ... }
```

---

## 8. API Reference

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/` | Frontend |
| `WS` | `/ws` | WebSocket — trigger runs, receive events |
| `POST` | `/api/test?config=permissive&generations=4` | Start evolution run |
| `GET` | `/api/results/{run_id}` | Full results including fitness + generation per probe |
| `GET` | `/api/runs` | Last 50 runs |
| `GET` | `/api/compare?run_a=X&run_b=Y` | Diff two runs — shows fitness delta + regressions |
| `GET` | `/api/vectors` | All 18 registered vectors with genome params |

---

## 9. Repo Structure

```
openzerg/
├── index.html                  3-panel demo shell
├── openzerg-map.html           Phaser 3 containment map
├── replay.json                 90-sec curated replay
├── INSTRUCTIONS.md             This file
├── OPENZERG_DOCS.md            Full project documentation
├── OPENZERG_PIVOT.md           Pivot brief (evolution reframe)
├── PLAN.md                     James's architecture plan
├── vendor/phaser.min.js        Self-hosted Phaser
├── assets/                     RA2 sprites + audio (gitignored)
└── backend/
    ├── controller.py           FastAPI + evolution loop + SQLite
    ├── run.sh                  Quick start
    ├── requirements.txt        Python deps
    ├── results.db              SQLite database
    └── attacks/
        ├── __init__.py         18 attack vectors with genome params
        ├── fs_symlink.py       Symlink traversal
        ├── fs_proc_root.py     /proc/1/root direct access
        ├── fs_mount_host.py    mount() syscall
        ├── net_k8s_api.py      K8s API server probe
        ├── net_egress.py       External internet egress
        ├── net_pod_to_pod.py   Kubelet port scan
        ├── priv_root_check.py  UID check
        ├── priv_docker_sock.py Docker socket escape
        ├── priv_nsenter.py     nsenter host namespace
        ├── priv_capabilities.py CapEff bitmask check
        ├── priv_cgroup_escape.py CVE-2022-0492 class test
        ├── sec_sa_token.py     SA token presence
        ├── sec_envvar_scan.py  Env var + /proc/environ scan
        ├── sec_configmap_read.py K8s ConfigMap read
        ├── res_fork_bomb.py    PID cgroup limit
        ├── res_memory_bomb.py  Memory cgroup limit
        ├── res_disk_fill.py    Ephemeral storage limit
        └── meta_kernel_cve.py  Kernel CVE version check
```

---

## 10. Demo Checklist

**Before presenting:**
- [ ] `caffeinate -dims` running (stops sleep)
- [ ] Kind cluster up: `kubectl get nodes`
- [ ] Ollama serving: `curl http://localhost:11434/api/tags`
- [ ] Backend running: `curl http://localhost:8000/api/vectors`
- [ ] Lapdog open at `http://localhost:9000` (second tab)
- [ ] Browser bookmarked: `http://localhost:8000/static/index.html?mode=auto`
- [ ] Test run passed: `curl -X POST "http://localhost:8000/api/test?config=permissive"`
- [ ] Replay fallback working: `?mode=replay` plays cleanly

**During demo:**
1. Permissive run → watch 3 breaches accumulate across generations
2. Point at Lapdog tab → "Gemma deciding which attacks to evolve"
3. Hardened run → all green across all generations
4. `/api/compare` → show fitness delta between runs

**If Kind dies:** Switch to `?mode=replay`. Identical visual. Say: *"deterministic playback of the same event stream — same code path."*
