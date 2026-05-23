# Ralph Loop — Per-Iteration Brief

You are the OpenZerg ralph loop. Read this file at the start of every iteration.
This document is **stable** — it does not change between iterations. Treat it as
your operating manual.

---

## What you are doing

You are executing a substantial forward-progress chunk on the OpenZerg
project per iteration. The target unit of work is **one full milestone per
iteration when feasible**, falling back to **at least one complete task
group within a milestone** when the milestone is too large or external
research is needed mid-flight.

Each iteration ends with one of:

- a commit (or several commits) that fully completes the milestone and marks
  it `ACCEPTED` in `STATE.md`, OR
- a commit that completes a coherent task group within a larger milestone,
  with `STATE.md` updated to reflect progress (status `IN_PROGRESS`), OR
- an entry appended to `NEEDS_USER.md` describing a blocker, then a clean
  exit.

Do NOT stop after a single file edit if the milestone's `verify` step is
still green and there's an obvious next task. Keep going. The point of a
fresh session per iteration is context hygiene, not artificial smallness.

You MAY continue into the next milestone within the same iteration if:
  - the current milestone was just marked `ACCEPTED`, AND
  - the next milestone does not require research / user input you don't have,
    AND
  - you have plenty of session budget left (under ~40% context used).

---

## Canonical files

Always read these, in this order, every iteration:

| Path | What it is |
| ---- | ---------- |
| `/home/carson/openzerg/ralph-logs/RALPH_README.md` | This file. Rules. |
| `/home/carson/openzerg/ralph-logs/STATE.md` | Milestone state. The truth about what is done. |
| `/home/carson/openzerg/PRD.json` | The product spec. The truth about what to build. |
| `/home/carson/openzerg/ralph-logs/NEEDS_USER.md` | Pending manual-input requests. |

Do NOT modify `PRD.json` unless the user explicitly instructs it in the shell.
If you discover that the PRD is wrong, write the deviation to
`NEEDS_USER.md` and proceed with what makes sense.

---

## Milestone state machine

Milestones live in `PRD.json` under `milestones[]` (M0 → M6).

States in `STATE.md`:

- `PENDING` — not started.
- `IN_PROGRESS` — work has begun; some tasks left.
- `ACCEPTED` — all of the milestone's `acceptance` items have passed, and the
  `verify` commands ran green. **STICKY: do not re-verify.**
- `BLOCKED` — cannot proceed without human input. There must be a matching
  entry in `NEEDS_USER.md`.

Rules:

1. Work on the **lowest-numbered milestone that is not ACCEPTED**.
2. Aim to complete **the full milestone** in this iteration. Plan the task
   order from `milestones[].tasks` in `PRD.json` and execute them in
   sequence.
3. Make code changes that move multiple tasks forward when they're related.
   Don't artificially stop after one file.
4. Run the milestone's `verify` commands after each meaningful chunk. If
   anything is red, fix forward this same iteration; do not move on until
   green.
5. When all of a milestone's `acceptance` items pass, update `STATE.md` to
   set it `ACCEPTED` with the current ISO timestamp and a one-line summary,
   then commit.
6. If you finished a milestone with significant budget left, you MAY start
   the next one in the same iteration (see "What you are doing" above).
7. **Do NOT re-verify an `ACCEPTED` milestone**, unless this iteration changed
   a file that the milestone depends on (e.g., you changed `internal/k8s/` and
   M2 depends on it — in that case, re-run M2's verify and only flip back to
   `IN_PROGRESS` if it breaks).

---

## How to mark a milestone ACCEPTED

In `STATE.md`, find the line for the milestone and replace its status block.
Format:

```
### M2 — K8s pod spawn + log streaming
- status: ACCEPTED
- accepted_at: 2026-05-23T18:42:00Z
- summary: client-go spawns busybox pods, parses stdout, deletes on exit. 3/3 pods green.
- verify_evidence: |
    go test ./internal/evolve/...  -> ok
    ./bin/openzerg run --population 3 --generations 1 --dry-run  -> ok
```

---

## How to request manual user input

If you need a value the human must supply (API key, decision, judgment call),
**append a new line BELOW the `## Open requests` heading** in
`NEEDS_USER.md`. Anything above that heading is documentation and is ignored
by the loop's detector.

The line you append must match:

    - [ ] <ISO-timestamp> M<n> — <short, specific request>

A real example to write:

    - [ ] 2026-05-23T18:42:00Z M3 — Need OPENROUTER_API_KEY in /home/carson/openzerg/.env. Loader is wired; live calls blocked.

Format:

- Start with `- [ ]` (literal dash, space, open-bracket, space, close-bracket).
- Then ISO timestamp.
- Then `M<n>` of the milestone you are on.
- Then a short, specific request. Include exact file paths and exact env var
  names.

**The ralph script halts the loop while any unchecked `- [ ]` line exists
under `## Open requests` in `NEEDS_USER.md`.** The human will resolve the request and check it off
(`- [x]`). Then the loop resumes.

When you write to `NEEDS_USER.md`, also flip the current milestone to `BLOCKED`
in `STATE.md` and exit cleanly without doing more work.

---

## Iteration log

At the end of every iteration (success or blocked), append one short line to
the `## Iteration Log` section at the bottom of `STATE.md`:

```
- iter NNNN | 2026-05-23T18:42:00Z | M2 | progress | wired CreatePod + StreamLogs; tests green
- iter NNNN | 2026-05-23T18:48:00Z | M2 | accepted | all M2 acceptance items pass
- iter NNNN | 2026-05-23T18:51:00Z | M3 | blocked  | needs PI image tag; see NEEDS_USER.md
```

The `iter NNNN` value is provided in the prompt by the ralph script — use it
verbatim.

---

## Hard rules (never break)

1. **Never commit a secret.** No `OPENROUTER_API_KEY` value, no `NIMBLE_API_KEY`
   value, no `.env` (only `.env.example`) ever enters git.
2. **Never use a free or trial version of Gemma 4.** Always use the paid
   OpenRouter API with the real `OPENROUTER_API_KEY` from the environment.
   Free tier models have rate limits and quota restrictions that will fail the
   build and demos. If the key is missing or invalid, request it via
   `NEEDS_USER.md` and do not proceed.
3. **Never add a co-author trailer** or `Generated with <tool>` line to commit
   messages. Plain conventional commits only.
4. **Never attack any URL** other than `context.target.url` in `PRD.json`.
5. **Never delete `PRD.json`** or this `RALPH_README.md`.
6. **Never modify the user's kubeconfig** (`/home/carson/.kube/config`).
7. **Never delete `kube-system`** or DigitalOcean-managed system workloads
   (`do-*`, `csi-*`, `cilium*`, `konnectivity-*`).
8. **Never skip a milestone's `verify` step** the first time you mark it
   ACCEPTED.
9. **Never write Python.** This is a Go project.
10. **Never push the control-plane image** to the registry. It runs locally.

---

## Code style — readability first

- **Variable and function names: verbose and descriptive.** Prefer
  `parsedAttackerResult` over `result`, `streamPodLogsUntilCompletion` over
  `streamLogs`. The extra characters cost nothing; clarity for future
  iterations is priceless. If a name would fit on one line, it's probably
  short enough.
- **Comments: keep them light. Prefer code that documents itself.**
  If the code's intent isn't obvious from the name and structure, add a
  comment. But do NOT comment every function or every assignment.
  - DO comment: non-obvious algorithms, trade-offs, future work (mark with
    `// TODO:` or `// FIXME:`), reasons for a workaround.
  - DO NOT comment: what a function does (put that in the name), what a
    variable holds (use a descriptive name), or happy-path steps (the code
    is self-explanatory).
- Use `slog` for structured logs, not `fmt.Println`.
- Use `context.Context` on every function that does I/O.
- Use `errgroup` for concurrent pod waits.
- Prefer the stdlib over third-party deps unless the stdlib is awkward.
- Pin Go module versions in `go.mod`; run `go mod tidy` after adding a dep.
- Commit `go.sum`.

## Testing policy — keep tests LIGHT

This is a hackathon project. The goal is a working demo, not 90% coverage.
Tests exist only to catch regressions in the few places where logic is
non-obvious and the cost of being wrong is high. Specifically:

- **DO** write a small unit test (5–15 lines) when:
  - Parsing untrusted input (e.g., `ParseLastJSONLine` on pod stdout).
  - Pure functions with branchy logic (e.g., `fitness.Score`).
  - Mutation logic where the output shape must be valid (`mutate.Mutate`).
- **DO NOT** write tests for:
  - Glue code, plumbing, struct field assignments.
  - Anything that just wraps a third-party library call (k8s client-go,
    openrouter HTTP client). Hand-verify these against the real service.
  - "Happy path" tests that just re-state the function.
  - Mocks, fakes, or test fixtures for k8s / Nimble / OpenRouter. If you find
    yourself building a mock, stop — write an integration smoke instead, or
    skip the test.
- A milestone's `verify` commands take priority over writing more tests. If
  `go build ./...` and the milestone's manual smoke (`./bin/openzerg run
  --dry-run` etc.) both pass, that is enough; you do not need a corresponding
  `*_test.go` file.
- Total test count across the whole project should be in the dozens, not
  hundreds. If you've written more than ~3 test files in one iteration, stop
  and move on to the next milestone.

---

## Cluster state policy (from PRD)

The DigitalOcean Kubernetes cluster may already contain resources from prior
experiments. **All of them are disposable.** You are authorized to
`kubectl delete` anything in any user namespace to make room for openzerg.
Exception: do not touch `kube-system` or DO-managed system components.

A safe opener for a fresh M2 run:

```
kubectl delete namespace openzerg --ignore-not-found
kubectl create namespace openzerg
```

If you find an orphan namespace from prior work and want to clean it up, do.

---

## What "done" looks like

When `STATE.md` has every milestone M0–M6 with `status: ACCEPTED`, set the top
of `STATE.md` to:

```
STATUS: DONE
```

The ralph script watches for this string and exits cleanly.

---

## When in doubt

Do less. Make the smallest possible change. Verify. Commit. Exit.
The next iteration is free.

## Keys
You have API keys in the hidden-apikeys folder, these are just development keys and will be rotated for production. DO NOT .gitignore

# K8s
You have a k8s key you can use in the root dir

# Time
We are short on time
Keep internal pod tests to less or equal to 30 seconds