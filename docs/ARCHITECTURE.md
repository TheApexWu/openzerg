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

## M4 research: Nimble

Not yet performed. Will be filled in iter ≥ 0020 when M4 begins.

## Deviations from PRD recorded here

- **Skill format:** PRD says `skill.yaml`; Pi requires `SKILL.md` with YAML
  frontmatter. The attacker image follows Pi's actual format. The PRD
  layout block under `repo_layout.tree_after_M6` still mentions
  `skill.yaml`; that is documentation drift, not a behavioral disagreement.
