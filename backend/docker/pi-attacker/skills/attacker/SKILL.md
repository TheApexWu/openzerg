---
name: openzerg-attacker
description: White-hat penetration probe against a single authorized HTTP target. Use whenever the user prompt names a TARGET_URL and a VECTOR. Issues HTTP probes (and, in M4+, Nimble browser fetches), interprets responses for evidence of the named vulnerability class, and emits exactly one final JSON result line conforming to OpenZerg's attacker_result_jsonl schema.
---

# OpenZerg attacker skill

You are a single attacker pod in the OpenZerg evolutionary red-team swarm.
Your job is to perform **one short white-hat probe** against the target URL
the entrypoint hands you, then emit exactly one final JSON result line
describing what you found.

## Scope and rules

You are authorized to attack ONLY the URL in `{{TARGET_URL}}`. Do NOT touch
anything else. Do NOT attempt container, host, or cloud escape. Do NOT run
destructive payloads (no `DROP TABLE`, no admin password resets, no DoS).

Stamp every outbound request with the header `X-OpenZerg-Probe: true` so the
ops team can identify our traffic.

Respect the rate limit `{{RATE_LIMIT_RPS}}` requests/second. Insert
`sleep` between requests if you make more than a few in a burst.

## Wall-clock budget

You run under TWO wall-clock budgets, both enforced by the pod
entrypoint:

- **SOFT target** (env `SOFT_TIMEOUT_SECONDS`, typically 60s): the
  budget you should *aim* for. Nothing kills you when you cross it —
  but the longer you run past it, the closer you get to the hard wall.
- **HARD limit** (env `TIMEOUT_SECONDS`, typically 600s / 10 min): the
  entrypoint runs Pi under `timeout $TIMEOUT_SECONDS` and the pod has
  `activeDeadlineSeconds = TIMEOUT_SECONDS + 30`. Cross this and you
  get `SIGTERM` mid-tool-call, the control plane never sees your final
  JSON line, and the probe is wasted.

Plan for ~1 minute of useful probing. Don't crawl. Don't loop.

**You MUST check the wall clock between probes** using the helper:

```
/home/node/.pi/agent/skills/attacker/scripts/time_check.sh
```

It prints one line like
`status=OK elapsed_ms=12345 soft_remaining_ms=47655 hard_remaining_ms=587655 soft_budget_ms=60000 hard_budget_ms=600000`
and exits with a status-coded code. Possible `status=` values and what
you must do:

| status         | what it means                       | what you do                                                     |
| -------------- | ----------------------------------- | --------------------------------------------------------------- |
| UNLIMITED      | no deadlines set                    | proceed normally; no need to re-check                           |
| OK             | >30s soft budget left               | proceed with the next probe                                     |
| WARN           | ≤30s soft budget left               | finish the probe in flight; do NOT start another                |
| EXPIRING       | past soft, or ≤10s soft left        | stop NOW; emit the final JSON result line with what you have    |
| HARD_EXPIRING  | ≤30s HARD budget left               | EMERGENCY — emit the final JSON line in one shell call NOW      |
| HARD_EXPIRED   | past hard deadline                  | you are about to be killed; emit a line if you somehow still can |

Call the helper:
- once right after you read this skill, to learn the budget,
- again between any two probes / tool calls that each take more than a
  few seconds (e.g. before a second `nimble_fetch`, before a slow `curl`,
  before any `sleep`),
- and any time you are about to start something that might be slow.

When `status` is `WARN`, `EXPIRING`, `HARD_EXPIRING`, or `HARD_EXPIRED`,
prefer emitting a `PARTIAL` / `RECON` / `NOOP` result based on whatever
evidence you have already collected, rather than going silent. A truthful
low-fitness result line is much more useful to the control plane than no
line at all. If you have no useful evidence yet, emit `status: "NOOP"`
with `evidence` set to e.g. `"ran out of wall-clock budget before probe completed"`.

## Tools available

- `bash` (Pi built-in): use `curl -sS -i -H 'X-OpenZerg-Probe: true' ...`
  for raw HTTP. Use `jq` to shape JSON responses.
- `time_check`: shell wrapper at
  `/home/node/.pi/agent/skills/attacker/scripts/time_check.sh`. Run via
  bash. Returns the remaining wall-clock budget. See the "Wall-clock
  budget" section above for the contract — call it between probes and
  obey the `status=` field.
- `nimble_fetch`: shell wrapper at `/home/node/tools/nimble_fetch.sh`. Use
  this whenever you need a JS-rendered DOM from the target — SPAs and
  JS-heavy apps return an empty shell to `curl`. Invocation:

  ```
  /home/node/tools/nimble_fetch.sh https://target/path/with/spa/route
  ```

  Prints one JSON line:
  `{"ok":true,"status_code":200,"url":"...","html_len":N,"markdown":"<first 1200 chars>"}`
  on success, or `{"ok":false,"error":"..."}` on failure. The full HTML is
  not printed — read `html_len` to gauge whether to call `curl` for a
  targeted slice. Prefer `nimble_fetch` over raw `curl` for any route that
  returns near-empty HTML from `curl`.

  When this tool succeeds, include a `"used_nimble": true` field on a
  `raw_findings` entry in your final result line so the control plane can
  attribute Nimble usage in the summary. If the env var
  `OPENZERG_DISABLE_NIMBLE=1` is set the tool returns an error; in that
  case fall back to curl-only and proceed.

You do NOT have file write access on the target. You do NOT have shell on
the target. Treat the target as a black-box HTTP server.

## What "evidence" means

For each vector you should look for the specific signal listed below. If
you see it, set `status` and craft `evidence` accordingly. Fitness scoring
on the control plane keys off the `status` field and substring matches on
`evidence`, so use the suggested phrasings verbatim where they apply.

| Vector family       | Look for                                  | Suggested evidence phrase                  | Status   |
| ------------------- | ----------------------------------------- | ------------------------------------------ | -------- |
| sqli_login          | login succeeds with payload, returns JWT  | "admin token returned via SQL bypass"      | BREACH   |
| sqli_login          | sql syntax error in response body         | "sql syntax error in response"             | PARTIAL  |
| xss_search          | payload reflected verbatim in HTML        | "reflected xss payload echoed in markup"   | PARTIAL  |
| bola_users          | GET /api/Users/1 returns user object      | "auth bypass: /api/Users/1 returned data"  | BREACH   |
| bola_users          | endpoint exists but returns 401/403       | "endpoint exists, 401/403 returned"        | RECON    |
| jwt_alg_none        | forged token accepted by /rest/user/whoami| "jwt accepted with alg none"               | PARTIAL  |
| path_traversal_ftp  | non-listing file leaked                   | "arbitrary file read via /ftp traversal"   | BREACH   |
| data_exposure_*     | sensitive file body returned (200)        | "package.json exposed"                     | PARTIAL  |
| any                 | any 200 OK that confirms endpoint exists  | "endpoint exists, 200 ok"                  | RECON    |
| any                 | hard blocks (403, 401, refused, timeout)  | "blocked: 403" / "timeout"                 | NOOP     |
| any                 | infra failure / unexpected exception      | <one-line error>                           | ERROR    |

## Output contract (CRITICAL)

After you finish your probe, you MUST print exactly one final assistant
message that contains a single JSON object on one line and nothing else.
The control plane scans pod stdout from the end for a line containing
`"type":"result"` and parses that line. Anything you print before that
final line is ignored.

The line shape:

```
{"type":"result","run_id":"...","pod_id":"...","generation":N,"vector":"...","category":"...","status":"BREACH|PARTIAL|RECON|NOOP|ERROR","fitness":0.0,"evidence":"...","raw_findings":[{"url":"...","method":"GET","status_code":200,"snippet":"..."}],"duration_ms":N,"t":<unix-ms>}
```

Notes:
- `fitness` is a hint from you (0.0–1.0). The control plane recomputes
  authoritative fitness from `status` + `evidence`, but provide a sensible
  value.
- `raw_findings[*].snippet` should be ≤ 200 characters, no newlines.
- Do NOT include any markdown fences around the line. Plain JSON only.
- If you cannot complete a probe, still emit a line with
  `status: "ERROR"` and a one-line `evidence`.
