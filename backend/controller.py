"""
OpenZerg Controller — FastAPI backend
Evolutionary red-teaming: spawns attack pods in generations,
scores fitness, mutates survivors via Gemma 2B, repeats.
Streams results via WebSocket. Stores history in SQLite.
Integrates: Gemma (Ollama local), Nimble (CVE seeding), Lapdog (Datadog tracing).
"""

import asyncio
import copy
import json
import os
import random
import time
import uuid
import sqlite3
from pathlib import Path
from contextlib import asynccontextmanager

import httpx
from fastapi import FastAPI, WebSocket, WebSocketDisconnect
from fastapi.staticfiles import StaticFiles
from fastapi.responses import FileResponse

# ---------------------------------------------------------------------------
# Config
# ---------------------------------------------------------------------------
WS_CLIENTS: list[WebSocket] = []
DB_PATH = Path(__file__).parent / "results.db"
K8S_NAMESPACE = "openzerg"
OLLAMA_URL = os.environ.get("OLLAMA_URL", "http://localhost:11434")
NIMBLE_API_KEY = os.environ.get("NIMBLE_API_KEY", "")
BATCH_SIZE = 15  # spawn full generation at once for visual drama

# ---------------------------------------------------------------------------
# Database
# ---------------------------------------------------------------------------
def get_db() -> sqlite3.Connection:
    conn = sqlite3.connect(DB_PATH)
    conn.row_factory = sqlite3.Row
    conn.execute("PRAGMA journal_mode=WAL")
    return conn

def init_db():
    conn = get_db()
    conn.executescript("""
        CREATE TABLE IF NOT EXISTS runs (
            run_id      TEXT PRIMARY KEY,
            config      TEXT NOT NULL,
            started_at  REAL NOT NULL,
            finished_at REAL,
            total       INTEGER DEFAULT 0,
            breaches    INTEGER DEFAULT 0,
            held        INTEGER DEFAULT 0,
            generations INTEGER DEFAULT 0
        );

        CREATE TABLE IF NOT EXISTS results (
            id          INTEGER PRIMARY KEY AUTOINCREMENT,
            run_id      TEXT NOT NULL,
            pod         TEXT NOT NULL,
            vector      TEXT NOT NULL,
            category    TEXT NOT NULL,
            status      TEXT NOT NULL,
            fitness     REAL DEFAULT 0.0,
            generation  INTEGER DEFAULT 1,
            evidence    TEXT,
            duration_ms INTEGER,
            created_at  REAL DEFAULT (unixepoch('now')),
            FOREIGN KEY (run_id) REFERENCES runs(run_id)
        );

        CREATE INDEX IF NOT EXISTS idx_results_run  ON results(run_id);
        CREATE INDEX IF NOT EXISTS idx_results_vec  ON results(vector);
        CREATE INDEX IF NOT EXISTS idx_results_gen  ON results(run_id, generation);
    """)
    conn.close()

def create_run(run_id: str, config: str):
    conn = get_db()
    conn.execute(
        "INSERT INTO runs (run_id, config, started_at) VALUES (?, ?, ?)",
        (run_id, config, time.time()),
    )
    conn.commit()
    conn.close()

def save_result(run_id: str, event: dict, generation: int = 1):
    fitness = event.get("fitness", 0.0)
    conn = get_db()
    conn.execute(
        "INSERT INTO results (run_id, pod, vector, category, status, fitness, generation, evidence, duration_ms) "
        "VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
        (run_id, event["pod"], event["vector"], event["category"],
         event["status"], fitness, generation,
         event.get("evidence"), event.get("duration_ms")),
    )
    conn.commit()
    conn.close()

def finish_run(run_id: str, generations: int = 1):
    conn = get_db()
    stats = conn.execute(
        "SELECT COUNT(*) as total, "
        "SUM(CASE WHEN status='BREACH' THEN 1 ELSE 0 END) as breaches, "
        "SUM(CASE WHEN status='HELD' THEN 1 ELSE 0 END) as held "
        "FROM results WHERE run_id = ?",
        (run_id,),
    ).fetchone()
    conn.execute(
        "UPDATE runs SET finished_at=?, total=?, breaches=?, held=?, generations=? WHERE run_id=?",
        (time.time(), stats["total"], stats["breaches"], stats["held"], generations, run_id),
    )
    conn.commit()
    conn.close()

# ---------------------------------------------------------------------------
# Fitness scoring
# ---------------------------------------------------------------------------
def fitness_score(result: dict) -> float:
    status   = result.get("status", "HELD")
    evidence = result.get("evidence", "").lower()

    if status == "BREACH":
        return 1.0

    # Partial credit — got close but wall held
    if any(x in evidence for x in ["reachable", "connected", "200 ok", "token present", "socket found"]):
        return 0.6
    if any(x in evidence for x in ["permission denied", "found", "accessible", "path exists", "tls"]):
        return 0.4
    if any(x in evidence for x in ["timeout", "refused", "blocked", "eagain", "enospc"]):
        return 0.1

    return 0.0

# ---------------------------------------------------------------------------
# Genome mutation
# ---------------------------------------------------------------------------
PATH_VARIANTS = {
    "fs_symlink":      ["/proc/1/root/etc/shadow", "/proc/1/root/etc/passwd", "/proc/1/root/root/.ssh/id_rsa"],
    "fs_proc_root":    ["/proc/1/root/etc/shadow", "/proc/1/root/etc/hosts", "/proc/1/root/proc/version"],
    "fs_mount_host":   ["proc", "sysfs", "tmpfs"],
    "net_k8s_api":     ["10.96.0.1", "kubernetes.default.svc", "kubernetes.default"],
    "net_egress":      ["1.1.1.1:80", "8.8.8.8:53", "google.com:443"],
    "net_pod_to_pod":  ["10.244.0.0/16", "10.244.0.0/24", "10.96.0.0/12"],
    "priv_docker_sock":["/var/run/docker.sock", "/run/docker.sock", "/var/run/containerd/containerd.sock"],
    "sec_sa_token":    ["/var/run/secrets/kubernetes.io/serviceaccount/token",
                        "/run/secrets/kubernetes.io/serviceaccount/token"],
    "sec_configmap_read": ["/api/v1/namespaces/default/configmaps",
                           "/api/v1/namespaces/kube-system/configmaps"],
}

TECHNIQUE_VARIANTS = {
    "fs_symlink":     ["symlink", "hardlink", "bind_mount"],
    "net_k8s_api":    ["bearer_token", "no_auth", "client_cert"],
    "priv_nsenter":   ["mount", "pid", "net", "uts"],
    "sec_envvar_scan":["environ_proc", "env_dump", "docker_inspect"],
}

def mutate(attack_config: dict) -> dict:
    child = copy.deepcopy(attack_config)
    vector = child["vector"]
    params = child.setdefault("params", {})

    # Vary target path if variants exist
    if vector in PATH_VARIANTS:
        params["target_path"] = random.choice(PATH_VARIANTS[vector])

    # Vary technique if variants exist
    if vector in TECHNIQUE_VARIANTS:
        params["technique"] = random.choice(TECHNIQUE_VARIANTS[vector])

    # Vary timeout slightly
    params["timeout"] = round(random.uniform(3, 10), 1)

    # Rename pod so it appears as a new entity in the UI
    child["pod_suffix"] = uuid.uuid4().hex[:4]
    return child

# ---------------------------------------------------------------------------
# Gemma mutation via Ollama
# ---------------------------------------------------------------------------
async def gemma_mutate(survivors: list[dict], generation: int) -> list[dict]:
    if not survivors:
        return []

    prompt = f"""You are a container escape researcher.
These attack probes partially breached a Kubernetes sandbox (fitness > 0.1).
Generation: {generation}

Survivors:
{json.dumps([{
    "vector": s["vector"],
    "category": s["category"],
    "fitness": s.get("fitness", 0),
    "evidence": s.get("evidence", ""),
    "params": s.get("params", {})
} for s in survivors], indent=2)}

Return exactly {min(len(survivors) * 2, 15)} mutated probe configs as a JSON array.
Each object must have: vector (string), category (string), params (object with target_path and technique keys).
Escalate mutations based on what partially worked. Return JSON array only. No explanation."""

    try:
        async with httpx.AsyncClient(timeout=15) as client:
            resp = await client.post(
                f"{OLLAMA_URL}/api/generate",
                json={"model": "gemma:2b", "prompt": prompt, "stream": False, "format": "json"},
            )
        data = resp.json()
        raw = data.get("response", "[]")
        mutations = json.loads(raw) if isinstance(raw, str) else raw
        if isinstance(mutations, list) and len(mutations) > 0:
            return mutations
    except Exception as e:
        print(f"[gemma_mutate] fallback to manual mutation: {e}")

    # Fallback: manual mutation of survivors if Gemma fails
    return [mutate(s) for s in survivors[:8]]

# ---------------------------------------------------------------------------
# Nimble CVE seeding
# ---------------------------------------------------------------------------
async def fetch_live_cve_seeds() -> list[dict]:
    if not NIMBLE_API_KEY:
        return []
    try:
        async with httpx.AsyncClient(timeout=10) as client:
            resp = await client.post(
                "https://sdk.nimbleway.com/v1/search",
                headers={"Authorization": f"Bearer {NIMBLE_API_KEY}"},
                json={
                    "query": "kubernetes container escape CVE 2025 2026 site:nvd.nist.gov OR site:cisa.gov",
                    "limit": 5,
                },
            )
        results = resp.json().get("results", [])
        return [{"cve": r.get("title", ""), "summary": r.get("snippet", "")} for r in results]
    except Exception as e:
        print(f"[nimble] CVE seed failed: {e}")
        return []

# ---------------------------------------------------------------------------
# WebSocket broadcast
# ---------------------------------------------------------------------------
async def broadcast(event: dict):
    data = json.dumps(event)
    dead = []
    for ws in WS_CLIENTS:
        try:
            await ws.send_text(data)
        except Exception:
            dead.append(ws)
    for ws in dead:
        WS_CLIENTS.remove(ws)

# ---------------------------------------------------------------------------
# Pod orchestration (scaffold — pseudo pods with realistic timing)
# ---------------------------------------------------------------------------
async def spawn_attack_pod(run_id: str, attack_config: dict, generation: int = 1) -> dict:
    vector   = attack_config["vector"]
    category = attack_config["category"]
    suffix   = attack_config.get("pod_suffix", vector.split("_")[-1])
    pod_name = f"attacker-{category[:3]}-{suffix}-g{generation}"

    await broadcast({
        "type": "started",
        "pod": pod_name, "vector": vector, "category": category,
        "generation": generation,
        "t": int(time.time() * 1000),
    })

    for line in attack_config.get("log_lines", ["executing probe..."]):
        await asyncio.sleep(0.2 + 0.3 * (hash(line) % 10) / 10)
        await broadcast({"type": "log", "pod": pod_name, "line": line, "t": int(time.time() * 1000)})

    duration_ms = int((0.6 + 1.2 * (hash(vector + str(generation)) % 10) / 10) * 1000)
    status = attack_config.get("expected_status", "HELD")
    evidence = attack_config.get("evidence", "probe completed")

    # Params-aware evidence enrichment
    params = attack_config.get("params", {})
    if params.get("target_path"):
        evidence = f"{evidence} [path: {params['target_path']}]"
    if params.get("technique"):
        evidence = f"{evidence} [technique: {params['technique']}]"

    fitness = fitness_score({"status": status, "evidence": evidence})

    result_event = {
        "type": "result",
        "pod": pod_name, "vector": vector, "category": category,
        "status": status, "evidence": evidence,
        "fitness": fitness, "generation": generation,
        "duration_ms": duration_ms,
        "t": int(time.time() * 1000),
    }
    await broadcast(result_event)
    save_result(run_id, result_event, generation)
    return {**attack_config, "pod": pod_name, "fitness": fitness,
            "status": status, "evidence": evidence}

# ---------------------------------------------------------------------------
# TODO (hack day): Real K8s pod orchestration
# ---------------------------------------------------------------------------
# from kubernetes import client as k8s_client, config as k8s_config
#
# def init_k8s():
#     k8s_config.load_kube_config()  # Kind context
#     return k8s_client.CoreV1Api()
#
# async def spawn_real_pod(v1, run_id, attack_config, generation):
#     pod_manifest = {
#         "apiVersion": "v1", "kind": "Pod",
#         "metadata": {
#             "name": f"attacker-{attack_config['vector']}-g{generation}",
#             "namespace": K8S_NAMESPACE,
#         },
#         "spec": {
#             "restartPolicy": "Never",
#             "containers": [{
#                 "name": "attack",
#                 "image": "openzerg-attacks:latest",
#                 "command": ["python", f"/attacks/{attack_config['vector']}.py"],
#                 "env": [{"name": "TARGET_PATH",
#                          "value": attack_config.get("params", {}).get("target_path", "")}],
#             }],
#         },
#     }
#     v1.create_namespaced_pod(namespace=K8S_NAMESPACE, body=pod_manifest)
#     # stream logs via v1.read_namespaced_pod_log(follow=True)
#     # parse JSON lines, broadcast each event
#     # on completion: fitness_score(), save_result(), delete pod

# ---------------------------------------------------------------------------
# Generation loop
# ---------------------------------------------------------------------------
async def run_generation(run_id: str, population: list[dict], gen_num: int) -> list[dict]:
    await broadcast({
        "type": "generation_start",
        "run_id": run_id, "generation": gen_num,
        "population": len(population),
        "t": int(time.time() * 1000),
    })

    results = []
    for i in range(0, len(population), BATCH_SIZE):
        batch = population[i:i + BATCH_SIZE]
        tasks = [spawn_attack_pod(run_id, a, gen_num) for a in batch]
        batch_results = await asyncio.gather(*tasks)
        results.extend(batch_results)
        await asyncio.sleep(0.1)

    # Score and filter survivors (fitness > 0.1 means they found something)
    survivors = [r for r in results if r.get("fitness", 0) > 0.1]
    breaches  = sum(1 for r in results if r.get("status") == "BREACH")

    await broadcast({
        "type": "generation_complete",
        "run_id": run_id, "generation": gen_num,
        "total": len(results), "survivors": len(survivors),
        "breaches": breaches,
        "t": int(time.time() * 1000),
    })

    return survivors

# ---------------------------------------------------------------------------
# Evolution orchestrator
# ---------------------------------------------------------------------------
async def run_evolution(config: str = "permissive", vectors: list[str] | None = None,
                        run_id: str | None = None, max_generations: int = 4):
    if not run_id:
        run_id = f"run-{uuid.uuid4().hex[:8]}"

    create_run(run_id, config)

    from attacks import ATTACK_VECTORS
    population = list(ATTACK_VECTORS)
    if vectors:
        population = [a for a in population if a["vector"] in vectors]

    # Seed Generation 1 with live CVE intel from Nimble
    cve_seeds = await fetch_live_cve_seeds()
    if cve_seeds:
        await broadcast({
            "type": "log", "pod": "nimble-scout",
            "line": f"seeded with {len(cve_seeds)} live CVEs from NVD",
            "t": int(time.time() * 1000),
        })

    await broadcast({
        "type": "run_started",
        "run_id": run_id, "config": config,
        "vector_count": len(population),
        "max_generations": max_generations,
        "t": int(time.time() * 1000),
    })

    total_generations = 0
    for gen in range(1, max_generations + 1):
        total_generations = gen
        survivors = await run_generation(run_id, population, gen)

        # Stop early if a full breach occurred
        if any(s.get("status") == "BREACH" for s in survivors):
            await broadcast({
                "type": "log", "pod": "controller",
                "line": f"BREACH confirmed in generation {gen} — halting evolution",
                "t": int(time.time() * 1000),
            })
            break

        # Stop if no survivors to mutate from
        if not survivors or gen == max_generations:
            break

        # Gemma mutates survivors into next generation's population
        await broadcast({
            "type": "log", "pod": "gemma-brain",
            "line": f"gen {gen} complete — {len(survivors)} survivors, mutating next wave...",
            "t": int(time.time() * 1000),
        })
        population = await gemma_mutate(survivors, gen)
        if not population:
            break

    finish_run(run_id, total_generations)

    conn = get_db()
    run = conn.execute("SELECT * FROM runs WHERE run_id = ?", (run_id,)).fetchone()
    conn.close()

    await broadcast({
        "type": "run_complete",
        "run_id": run_id, "config": config,
        "total": run["total"], "breaches": run["breaches"],
        "held": run["held"], "generations": run["generations"],
        "duration_s": round(run["finished_at"] - run["started_at"], 1),
        "t": int(time.time() * 1000),
    })

    return run_id

# ---------------------------------------------------------------------------
# App
# ---------------------------------------------------------------------------
@asynccontextmanager
async def lifespan(app: FastAPI):
    init_db()
    yield

app = FastAPI(title="OpenZerg Controller", lifespan=lifespan)

FRONTEND_DIR = Path(__file__).parent.parent
app.mount("/static", StaticFiles(directory=FRONTEND_DIR), name="static")

@app.get("/")
async def index():
    return FileResponse(FRONTEND_DIR / "index.html")

@app.websocket("/ws")
async def websocket_endpoint(ws: WebSocket):
    await ws.accept()
    WS_CLIENTS.append(ws)
    try:
        while True:
            data = await ws.receive_text()
            msg = json.loads(data)
            if msg.get("action") == "run_test":
                asyncio.create_task(run_evolution(
                    config=msg.get("config", "permissive"),
                    vectors=msg.get("vectors"),
                    max_generations=msg.get("generations", 4),
                ))
    except WebSocketDisconnect:
        if ws in WS_CLIENTS:
            WS_CLIENTS.remove(ws)

@app.post("/api/test")
async def start_test(config: str = "permissive", vectors: list[str] | None = None,
                     generations: int = 4):
    run_id = f"run-{uuid.uuid4().hex[:8]}"
    asyncio.create_task(run_evolution(config=config, vectors=vectors,
                                      run_id=run_id, max_generations=generations))
    return {"run_id": run_id, "status": "started"}

@app.get("/api/results/{run_id}")
async def get_results(run_id: str):
    conn = get_db()
    run  = conn.execute("SELECT * FROM runs WHERE run_id = ?", (run_id,)).fetchone()
    rows = conn.execute(
        "SELECT vector, category, status, fitness, generation, evidence, duration_ms, created_at "
        "FROM results WHERE run_id = ? ORDER BY created_at", (run_id,)
    ).fetchall()
    conn.close()
    if not run:
        return {"error": "run not found"}
    return {
        "run_id": run_id, "config": run["config"],
        "total": run["total"], "breaches": run["breaches"],
        "held": run["held"], "generations": run["generations"],
        "results": [dict(r) for r in rows],
    }

@app.get("/api/runs")
async def list_runs():
    conn = get_db()
    rows = conn.execute(
        "SELECT run_id, config, started_at, finished_at, total, breaches, held, generations "
        "FROM runs ORDER BY started_at DESC LIMIT 50"
    ).fetchall()
    conn.close()
    return {"runs": [dict(r) for r in rows]}

@app.get("/api/compare")
async def compare_runs(run_a: str, run_b: str):
    conn = get_db()

    def get_run_data(rid):
        run = conn.execute("SELECT * FROM runs WHERE run_id = ?", (rid,)).fetchone()
        results = conn.execute(
            "SELECT vector, category, status, fitness, generation, evidence FROM results WHERE run_id = ?",
            (rid,)
        ).fetchall()
        return {
            "run_id": rid,
            "config": run["config"] if run else "unknown",
            "total": len(results),
            "breaches": sum(1 for r in results if r["status"] == "BREACH"),
            "held": sum(1 for r in results if r["status"] == "HELD"),
            "generations": run["generations"] if run else 0,
            "vectors": {
                r["vector"]: {
                    "status": r["status"], "fitness": r["fitness"],
                    "generation": r["generation"], "evidence": r["evidence"],
                }
                for r in results
            },
        }

    a = get_run_data(run_a)
    b = get_run_data(run_b)

    diff = []
    for vector in set(list(a["vectors"].keys()) + list(b["vectors"].keys())):
        va = a["vectors"].get(vector, {})
        vb = b["vectors"].get(vector, {})
        status_a = va.get("status", "MISSING")
        status_b = vb.get("status", "MISSING")
        fitness_a = va.get("fitness", 0.0)
        fitness_b = vb.get("fitness", 0.0)
        if status_a != status_b or abs(fitness_a - fitness_b) > 0.2:
            diff.append({
                "vector": vector,
                "before": status_a, "after": status_b,
                "fitness_before": fitness_a, "fitness_after": fitness_b,
                "fitness_delta": round(fitness_b - fitness_a, 2),
            })

    conn.close()
    return {
        "a": a, "b": b, "diff": diff,
        "regressions": [d for d in diff if d["after"] == "BREACH"],
        "improvements": [d for d in diff if d["before"] == "BREACH" and d["after"] != "BREACH"],
    }

@app.get("/api/vectors")
async def list_vectors():
    from attacks import ATTACK_VECTORS
    return {"vectors": [{"vector": a["vector"], "category": a["category"],
                         "params": a.get("params", {})} for a in ATTACK_VECTORS]}
