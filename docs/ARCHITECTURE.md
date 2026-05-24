# OpenZerg Architecture

This is a working notes file. It captures (a) the architecture diagram from
`PRD.json` for quick reference, and (b) the research findings the ralph loop
collected before writing M3 (Pi) and M4 (Nimble) code. M5 will fold a polished
1-page version of this into the final demo docs.

## Diagram

```
+--------------------+
|  user shell        |
|  TARGET_URL=...    |
|  ./openzerg        |
+---------+----------+
          |
          v
+--------------------+        +------------------------+
|  control plane     |------->|  DigitalOcean K8s      |
|  (Go binary, local)|        |  namespace: openzerg   |
|  - evolve loop     |        |  pods: attacker-g<N>-* |
|  - k8s client-go   |<-------|  one PI agent per pod  |
|  - openrouter MUT  |  logs  |  (Gemma 4 + Nimble)    |
|  - summary writer  |        +-----------+------------+
+---------+----------+                    |
          |                                v
          |                    +------------------------+
          |                    |  TARGET_URL (Juice     |
          |                    |  Shop on Railway)      |
          |                    +------------------------+
          v
  ./out/summary-<id>.json + .md
```

## M3 research: Pi agent runtime

Source: <https://pi.dev/docs/latest> (fetched iter 0019).

### What Pi actually is

- Package name: `@earendil-works/pi-coding-agent` (npm).
- Author: Earendil Inc. (`https://earendil.com/`).
- Install (Linux): `npm install -g --ignore-scripts @earendil-works/pi-coding-agent`
  or `curl -fsSL https://pi.dev/install.sh | sh`.
- Binary name: `pi`.
- License: MIT.

### Non-interactive invocation (what attacker pods need)

`pi --mode json "<prompt>"` runs one session and streams JSONL events to
stdout. Each line is one event object; the first line is a session header.
The schema is documented at <https://pi.dev/docs/latest/json>. The events we
care about during a one-shot attacker run are:

- `agent_start`, `agent_end` — bookends.
- `turn_start`, `turn_end` — one model turn.
- `message_start`, `message_update`, `message_end` — assistant token stream.
- `tool_execution_start`, `tool_execution_update`, `tool_execution_end` —
  any tool calls (e.g., HTTP, Nimble).

The pod's `entrypoint.sh` does not parse these events; it only needs Pi to
finish and the entrypoint then emits one final `attacker_result_jsonl`-shaped
line (see `PRD.json` `data_contracts.attacker_result_jsonl`). The skill we
ship instructs the model to print that line itself before ending the turn.

There is also `pi --mode rpc` (stdin/stdout JSONL bidirectional) and an
SDK for Node.js, but we do not need them.

### Provider config

Pi supports OpenRouter natively. Two options:

1. Env var: `OPENROUTER_API_KEY=...`. Pi picks it up directly.
2. Auth file: `~/.pi/agent/auth.json` with
   `{ "openrouter": { "type": "api_key", "key": "..." } }`. We use option 1
   inside the pod because the env var is already injected via the k8s
   Secret.

The Gemma 4 model IDs in `PRD.json`
(`google/gemma-4-26b-a4b-it`, `google/gemma-4-31b-it`) are OpenRouter model
slugs and are passed to Pi via `--model` or via a `models.json` entry. We
will pass `--provider openrouter --model google/gemma-4-26b-a4b-it` on the
CLI so model selection is explicit per-pod.

### Skill format (deviation from PRD)

The PRD plans for `skill.yaml`. Pi uses **`SKILL.md`** with YAML frontmatter,
per the Agent Skills standard (<https://agentskills.io/specification>):

```
my-skill/
└── SKILL.md
```

Frontmatter:
- `name` (required, lowercase, `a-z 0-9 -`, max 64 chars)
- `description` (required, max 1024 chars, drives when the model loads the
  skill)
- Optional: `license`, `compatibility`, `metadata`, `allowed-tools`,
  `disable-model-invocation`.

Body is freeform Markdown — usually instructions plus references to helper
scripts in the same directory.

Pi loads skills from (in order):
- `~/.pi/agent/skills/`
- `~/.agents/skills/`
- `./.pi/skills/` (cwd / ancestors)
- `./.agents/skills/`
- Packages with `skills/` dir or `pi.skills` in `package.json`
- `--skill <path>` (CLI, additive even with `--no-skills`)

For our attacker pod we ship the skill at
`/home/agent/.pi/skills/attacker/SKILL.md` and rely on the global
discovery rule.

### No official Docker image

Pi does not publish a Docker image. We base the attacker image on
`node:22-bookworm-slim` (Node 22 is current; Pi's npm package supports
modern Node) and `npm i -g --ignore-scripts @earendil-works/pi-coding-agent`.

### Tool exposure to the model

Pi has built-in tools (read, write, bash, etc.) controlled via
`allowed-tools` in the skill frontmatter and via the `--no-skills` /
`/settings` toggles. For our use case we want the model to:
- Issue HTTP requests against `TARGET_URL` (use `bash` + `curl`, plus the
  rate-limit knob in our entrypoint).
- Call Nimble (M4) via a small shell wrapper script that lives in the skill
  dir.
- Emit the final result JSON line.

We do NOT plan to write a Pi extension (TypeScript module) for now — the
skill + bash approach is sufficient and keeps the image small.

### How tool errors get scored

Pi surfaces tool errors as `tool_execution_end` events with `isError: true`.
We do not parse this from the control plane. Instead, the skill instructs
the model to set `status: "ERROR"` and `evidence: "<reason>"` in the final
result line on any unrecoverable failure. The control plane's
`evolve.Score` (M5) maps `status==ERROR` → `0.0`.

## M6 research: Nimble Integration

Source: <https://docs.nimbleway.com/llms.txt> and the OpenAPI specs at
`/api-reference/extract/extract.md` and `/api-reference/search/search.md`.

### Resolved research questions

- **Endpoint base URL:** `https://sdk.nimbleway.com/v1`.
- **Auth scheme:** `Authorization: Bearer NIMBLE_API_KEY` header. No
  query-string keys; the `NIMBLE_API_URL` hex value found in
  `.env.example` is NOT a URL — it is an alternate API key form. We ignore
  it and use the documented `NIMBLE_API_KEY` bearer flow.
- **Primary endpoint we use:** `POST /v1/extract` for JS-rendered page
  fetches. Body: `{url, render, formats}`. Response:
  `{url, task_id, status, status_code, data:{html, markdown}, metadata}`.
- **Secondary endpoint we use:** `POST /v1/search` for CVE seeding. Body:
  `{query, max_results, search_depth, focus}`. Response:
  `{total_results, results:[{title, description, url, content}], request_id}`.
- **Rate limits:** Documented at `/nimble-sdk/admin/rate-limits.md`. We
  stay well under any tier limit (≤1 call at startup, ≤1 call per pod per
  generation).
- **Tool exposure to Pi:** We do not write a Pi extension. Instead we ship
  a tiny shell wrapper at `/home/node/tools/nimble_fetch.sh` and instruct
  the model in `SKILL.md` to invoke it via the built-in `bash` tool. The
  wrapper hides the API key (env-only, never echoed) and returns one
  summarised JSON line the model can parse.

### Failure modes the Go client handles

- Missing key                -> sentinel `ErrMissingKey`, no HTTP call.
- Empty URL / query          -> validation error, no HTTP call.
- Network failure / timeout  -> wrapped `transport: ...` error.
- 4xx/5xx response           -> `errors.Is(err, ErrUpstream)` true; status
  code embedded in the message; body truncated to 256 chars.
- 200 with malformed JSON    -> `json.SyntaxError` is wrapped.
- API key leak               -> `TestKeyNeverLogged` is the canary; key
  never appears in slog output, error messages, or returned structs.

### Kill switch

`./bin/openzerg run --disable-nimble` drops `NIMBLE_API_KEY` from the pod's
effective context by setting `OPENZERG_DISABLE_NIMBLE=1`. The in-pod
`nimble_fetch.sh` short-circuits when that flag is set and the swarm
proceeds with `curl`-only attacks. Useful as a demo-time safety net if
Nimble itself has a bad day.

### Optional CVE seeding

`./bin/openzerg run --enable-cve-seed` calls
`nimble.SearchWeb(ctx, "OWASP Juice Shop CVE recent vulnerability")` at
startup and folds the top hit's title+snippet into the first Gen-1
genome's `hint`. Off by default so demo runs stay deterministic.

## Old M4 placeholder

(M4 in this repo is the evolution loop, not Nimble. The Nimble integration
landed in M6 per the user's milestone reordering on 2026-05-23.)

## M7 research: Web UI (HTTP + SSE + embedded frontend)

Source: PRD.json milestones[7]. The serve subcommand was added in iter 0032.

### One binary, one port

`openzerg serve --addr :8080` boots an `http.ServeMux` that handles both the
REST/SSE control surface (`/api/*`) and the embedded single-page frontend
(everything else). There is no separate dev server, no nginx, no docker
compose. The frontend tree (`backend/internal/api/frontend_embed/`) is baked
into the Go binary via `//go:embed all:frontend_embed` in
`internal/api/embed.go`. On startup the handler stats `index.html` inside
the embed FS and refuses to start if it is absent — the build fails loud
rather than silently shipping an empty UI.

The `--frontend <dir>` flag overrides the embed with an on-disk directory for
iteration during dev. The same handler interface (`diskFrontendHandler` vs
`embedFrontendHandler`) handles both modes.

### Routes

```
GET    /healthz                              -> 200 ok
GET    /api/events                           -> SSE stream
GET    /api/runs                             -> [{run_id, outcome, ...}]
GET    /api/runs/current                     -> snapshot or 404
GET    /api/runs/{id}                        -> stored summary JSON
POST   /api/runs                             -> start a run (409 if in flight)
POST   /api/runs/current/cancel              -> cancel the in-flight run
GET    /api/integrations/openrouter          -> {ok, model}
GET    /api/integrations/nimble              -> {ok}
GET    /                                     -> embedded index.html (no-store)
GET    /assets/*, /styles/*, /src/*          -> embedded static (max-age=300)
GET    /<anything else>                      -> SPA fallback -> index.html
```

### SSE protocol

`internal/events` is an in-process pub/sub broker with a 2000-event ring
buffer and a monotonic `Seq`. Subscribers get a buffered channel; if they
are slow we drop events on the publish side rather than back-pressuring the
evolution loop. The SSE handler writes one `data:` chunk per event,
includes the `Seq` as the SSE `id`, and respects `Last-Event-ID` on
reconnect to replay anything still in the ring.

Event types emitted by `internal/runner.Runner`: `run_start`,
`generation_start`, `pod_spawn`, `pod_result`, `generation_end`,
`mutation`, `breach`, `run_end`. The `hello` event is emitted by the SSE
handler at connection time so the client knows the stream is open.

### Run controller

`internal/api.RunController` is a tiny state machine: at most one in-flight
run at a time, cancellation is `context.CancelFunc`, completed runs are
kept in memory for the process lifetime (good enough for a hackathon
demo). Concurrent `POST /api/runs` returns 409. `Cancel` triggers the same
SIGINT-style partial-summary path that the headless CLI uses.

### Frontend

Plain ES modules + CSS, no build step. `app.js` opens an `EventSource`,
routes events into a tiny state machine, and updates DOM nodes for the
arena (pods on a golden-angle spiral, colour-coded by fitness), the
generation banner, the integration pills, the log, and the history panel.
The Start Run form `POST`s to `/api/runs` and disables itself while a run
is in flight.

## Deviations from PRD recorded here

- **Skill format:** PRD says `skill.yaml`; Pi requires `SKILL.md` with YAML
  frontmatter. The attacker image follows Pi's actual format. The PRD
  layout block under `repo_layout.tree_after_M6` still mentions
  `skill.yaml`; that is documentation drift, not a behavioral disagreement.
