# OpenZerg

> We don't send one smart attacker. We send a hundred dumb ones, watch which
> find cracks, and breed the next generation from the survivors.

OpenZerg is an evolutionary, agentic red-team swarm. A Go control plane spawns
a Kubernetes-backed swarm of PI agent pods (Gemma 4 via OpenRouter, web
navigation via Nimble). Each pod is a disposable autonomous pentester aimed at
a fixed HTTP target. The control plane scores every probe by fitness, kills
the weak, mutates the strong, and respawns the next generation — until a full
breach is found or `--generations` is reached. The final summary explains the
path the swarm took to its outcome.

Target for the bundled demo: OWASP Juice Shop on Railway (we are authorised).

## Quick start

```bash
# 1. Put keys in .env at the repo root (.env.example shows the schema).
#    Required: OPENROUTER_API_KEY (paid, no :free).
#    Required: NIMBLE_API_KEY.

# 2. Create the openzerg namespace + the openzerg-keys Secret on the cluster.
kubectl create namespace openzerg
kubectl -n openzerg create secret generic openzerg-keys --from-env-file=.env

# 3. Build and run.
cd backend && go build -o ./bin/openzerg ./cmd/openzerg
./bin/openzerg run \
  --target https://juice-shop-production-d0c5.up.railway.app \
  --population 3 --generations 2
```

A short doctor command reports environment readiness without touching the
cluster:

```bash
./bin/openzerg doctor
```

## Sample output (truncated)

```
=== generation 1/2: spawning 3 pods ===
[gen 1 pod 0] {"type":"result","status":"BREACH","vector":"sqli_login",
  "evidence":"admin token returned via SQL bypass",...}
[gen 1 pod 0] fitness=1.00 vector=sqli_login status=BREACH
[gen 1 pod 1] fitness=0.00 vector=sqli_login_union status=NOOP
[gen 1 pod 2] fitness=0.00 vector=xss_search_reflected status=NOOP

BREACH detected in generation 1. Stopping.

summary: out/summary-r1779563300.json
         out/summary-r1779563300.md
outcome: BREACH (best fitness 1.00)
run: ok
```

The Markdown summary contains a per-generation table, the top-scoring probes,
the exact request that breached the target, and a short narrative.

## Layout

```
backend/                 Go module
  cmd/openzerg/          control-plane CLI
  internal/
    attacks/             genome catalog + seed list
    config/              flag + env merging
    evolve/              fitness scoring + mutation + result parsing
    k8s/                 client-go wrappers (create / stream / wait / delete)
    nimble/              Nimble client (control-plane CVE seeding hook)
    openrouter/          minimal chat-completions client
    pi/                  PI agent helpers
    secrets/             .env + process env loader
    spawn/               pod-spec builders + fan-out orchestrator
    store/               in-memory RunStore + JSON/MD summary writers
  docker/pi-attacker/    attacker pod image (PI + Gemma 4 entrypoint)
  deploy/                namespace + reference pod manifest
docs/                    DEMO.md, ARCHITECTURE.md
scripts/                 helper scripts (build-and-push-attacker.sh, ...)
out/                     run summaries (gitignored except sample)
```

## White-hat scope

OpenZerg only attacks `--target`. The bundled demo targets an OWASP Juice
Shop instance we operate. Do not point it at anything you don't own.

## Status

See `ralph-logs/STATE.md` for milestone state.
