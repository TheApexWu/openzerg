# OpenZerg Ralph State

STATUS: DONE

Last updated: 2026-05-23T19:46:00Z (all milestones M0-M6 ACCEPTED; STATUS flipped to DONE)

This file is the ledger of milestone state. The ralph loop reads it every
iteration. Milestones marked `ACCEPTED` are sticky — the agent will not
re-verify them unless dependencies change.

See `RALPH_README.md` for state machine rules and update format.

---

## Milestones

### M0 — Repo hygiene + Go module bootstrap
- status: ACCEPTED
- accepted_at: 2026-05-23T17:32:03Z
- summary: Go module scaffolded; all internal packages declared; binary prints version. build + vet green.
- verify_evidence: |
    cd backend && go build ./...  -> ok
    cd backend && go vet ./...    -> ok
    ./bin/openzerg version        -> "openzerg 0.1.0-dev"

### M1 — Config, secrets, doctor command
- status: ACCEPTED
- accepted_at: 2026-05-23T17:35:24Z
- summary: secrets/config/k8s probes implemented; doctor prints multi-line status; run --dry-run prints planned pod spec; tests green.
- verify_evidence: |
    cd backend && go build ./...                                    -> ok
    cd backend && go vet ./...                                      -> ok
    cd backend && go test ./...                                     -> ok (secrets + k8s)
    ./bin/openzerg doctor                                            -> kubeconfig + secret report, exit 0
    ./bin/openzerg run --target https://example.invalid --dry-run    -> planned-pod-spec preview, exit 0

### M2 — K8s pod spawn + log streaming (no PI yet)
- status: ACCEPTED
- accepted_at: 2026-05-23T18:11:45Z
- summary: client-go spawns 3 busybox stub pods in openzerg ns on DO cluster, streams logs, parses final JSON line, and deletes pods. RunAsUser/RunAsGroup pinned to 65532 so busybox passes RunAsNonRoot admission. All 3 results printed; zero leftover pods.
- verify_evidence: |
    cd backend && go build ./...  -> ok
    cd backend && go vet ./...    -> ok
    cd backend && go test ./...   -> ok (evolve, k8s, secrets, spawn)
    ./bin/openzerg run --target https://juice-shop-production-d0c5.up.railway.app --population 3 --generations 1
        -> [pod 0..2] {"type":"result",...} ; run: ok
    kubectl -n openzerg get pods  -> No resources found (clean)

### M3 — Attacker pod image with PI + Gemma 4 (no Nimble yet)
- status: ACCEPTED
- accepted_at: 2026-05-23T18:58:00Z
- summary: pi-attacker:latest pushed to DO registry; pinned to paid google/gemma-4-26b-a4b-it (no :free). Live 3-pod run against juice-shop emits 3 clean attacker_result_jsonl lines: pod 0 BREACHes via SQLi tautology in /rest/user/login (JWT admin token returned); pods 1-2 NOOP via entrypoint timeout fallback. Entrypoint hardened against `set -e` + grep-no-match; uses one streaming jq pass to extract model text (per-event jq loop was 100x too slow on 500m CPU). Control plane re-reads logs after pod terminal phase to defeat kubelet flush race.
- verify_evidence: |
    cd backend && go build ./...   -> ok
    cd backend && go vet ./...     -> ok
    cd backend && go test ./...    -> ok (evolve, k8s, secrets, spawn)
    bash scripts/build-and-push-attacker.sh -> pushed
    ./bin/openzerg run --target https://juice-shop-production-d0c5.up.railway.app --population 3 --generations 1
      -> [pod 0] BREACH (sqli_login, admin token)
         [pod 1] NOOP  (sqli_login_union timeout)
         [pod 2] NOOP  (xss_search_reflected timeout)
         run: ok
    kubectl -n openzerg get pods   -> No resources found (clean)

### M4 — Evolution loop, fitness scoring, mutation, summary
- status: ACCEPTED
- accepted_at: 2026-05-23T19:10:30Z
- summary: full evolution loop wired (seeds -> spawn -> score -> survivors -> mutate -> next gen). PRD-priority fitness scoring; pure-Go mutation with cross-breed/path-vary/technique-swap; optional Gemma 4 LLM mutation gated by 32-call budget; SIGINT writes partial summary; per-run JSON+MD artifacts written to ./out. Live 3-pod 2-gen smoke against juice-shop: BREACH detected gen 1 pod 0 (sqli_login, admin token), short-circuited, summary written, pods cleaned. duration ~103s.
- verify_evidence: |
    cd backend && go build ./...                              -> ok
    cd backend && go vet ./...                                -> ok
    cd backend && go test ./...                               -> ok (evolve fitness+mutate tests added)
    ./bin/openzerg run --target https://juice-shop-production-d0c5.up.railway.app --population 3 --generations 2 --out-dir ./out
        -> BREACH gen 1 pod 0 sqli_login, outcome=BREACH best_fitness=1.00
    jq '.outcome == "BREACH" and .best_fitness == 1' out/summary-r1779563300.json -> true
    kubectl -n openzerg get pods                              -> No resources found

### M5 — Cleanup, docs, demo script
- status: ACCEPTED
- accepted_at: 2026-05-23T19:15:00Z
- summary: legacy Python (backend/controller.py, backend/attacks/*.py, backend/run.sh, backend/requirements.txt, results.db*) and HTML/Phaser/replay (index.html, openzerg-map.html, replay.json, vendor/, prototypes/, scripts/dev_server.py) deleted. README.md, docs/DEMO.md, docs/ARCHITECTURE.md written. Sanitised sample summary committed at docs/sample-summary/summary-demo.{json,md}.
- verify_evidence: |
    find . -name "*.py" -not -path "./.git/*"  -> (none)
    ls index.html openzerg-map.html vendor prototypes 2>&1  -> all missing
    cd backend && go build ./...  -> ok
    cd backend && go vet ./...    -> ok
    cd backend && go test ./...   -> ok

### M6 — Nimble integration (sponsor tool — required)
- status: ACCEPTED
- accepted_at: 2026-05-23T19:44:31Z
- summary: nimble client (Go), 7 client tests (incl. key-leak canary), pod-side nimble_fetch.sh wrapper, SKILL.md tool exposure, --disable-nimble kill switch, --enable-cve-seed startup hook, doctor reachability probe, and summary.md Nimble-attribution section all live. Updated user.tmpl to honour `requires_nimble: true` and PickSeedGenomesEnsuringNimble guarantees at least one nimble-required vector in small populations. Live smokes against juice-shop pass both ways: enabled run (pop=3) BREACHed gen 1 pod 0 (sqli_login admin token) AND pod 2 invoked nimble_fetch on /#/administration with `used_nimble:true` on raw_findings; --disable-nimble run completed cleanly with no nimble usage and summary noting the kill switch.
- verify_evidence: |
    cd backend && go build ./...                            -> ok
    cd backend && go vet ./...                              -> ok
    cd backend && go test ./...                             -> ok (nimble: 7 tests incl. TestKeyNeverLogged)
    bash scripts/build-and-push-attacker.sh                 -> pushed
    ./bin/openzerg doctor                                   -> NIMBLE_API_KEY: present, nimble probe: reachable
    ./bin/openzerg run --target https://juice-shop-production-d0c5.up.railway.app --population 3 --generations 1
        -> BREACH gen 1 pod 0 sqli_login; pod 2 used_nimble:true on /#/administration; summary mentions Nimble usage
    ./bin/openzerg run --target https://juice-shop-production-d0c5.up.railway.app --population 3 --generations 1 --disable-nimble
        -> EXHAUSTED clean; summary notes "Nimble was disabled for this run"
    kubectl -n openzerg get pods                            -> No resources found



---

## Iteration Log

<!-- Append one line per iteration. Format: -->
<!-- - iter NNNN | ISO-timestamp | M<n> | progress|accepted|blocked | one-line note -->
- iter 0002 | 2026-05-23T17:32:03Z | M0 | accepted | go module + package skeleton verified; build/vet green; binary prints version
- iter 0003 | 2026-05-23T17:35:24Z | M1 | accepted | secrets loader + flags + kubeconfig probe; doctor and run --dry-run wired; tests green
- iter 0004 | 2026-05-23T17:37:13Z | M2 | progress | evolve.ParseLastJSONLine + hostile-input tests; client-go spawn next
- iter 0005 | 2026-05-23T17:38:24Z | M2 | progress | added backend/deploy/namespace.yaml manifest for openzerg ns
- iter 0006 | 2026-05-23T17:40:29Z | M2 | progress | spawn.BuildBusyboxPod renders busybox pod manifest with k8s.io/api types; tests pass
- iter 0007 | 2026-05-23T17:42:07Z | M2 | progress | k8s.BuildClientset added (client-go, explicit/$KUBECONFIG/in-cluster); 3 unit tests green
- iter 0008 | 2026-05-23T17:43:26Z | M2 | progress | k8s.CreatePod wrapper + 3 fake-clientset tests; build/vet/test green
- iter 0009 | 2026-05-23T17:45:00Z | M2 | progress | k8s.DeletePod wrapper (idempotent on NotFound) + 3 fake-clientset tests; build/vet/test green
- iter 0010 | 2026-05-23T17:46:30Z | M2 | progress | k8s.WaitForCompletion polls until Succeeded/Failed, ctx-cancellable; 4 fake-clientset tests; build/vet/test green
- iter 0011 | 2026-05-23T17:48:00Z | M2 | progress | k8s.StreamLogs (follow=true) returns io.ReadCloser; 2 fake-clientset tests; build/vet/test green
- iter 0013 | 2026-05-23T17:49:46Z | M2 | progress | spawn.RunPod ties create/stream/wait/parse/delete for one pod; nil-guard tests; build/vet/test green
- iter 0014 | 2026-05-23T17:51:42Z | M2 | progress | spawn.RunPods concurrent fan-out (sync.WaitGroup, ordered outcomes); 2 nil/empty tests; build/vet/test green
- iter 0015 | 2026-05-23T17:53:36Z | M2 | progress | cmd/openzerg run non-dry-run wired: builds clientset, renders N busybox stubs, fans out via spawn.RunPods, prints outcomes; build/vet/test green
- iter 0019 | 2026-05-23T18:11:45Z | M2 | accepted | live DO smoke green: 3/3 busybox stub pods spawn, emit JSON, parse, and delete; pinned RunAsUser=65532 to clear RunAsNonRoot admission
- iter 0019 | 2026-05-23T18:18:00Z | M3 | progress | Pi research + scaffolded Dockerfile, entrypoint, SKILL.md, prompts, build script; deviation recorded (Pi uses SKILL.md, not skill.yaml)
- iter 0022 | 2026-05-23T18:58:00Z | M3 | accepted | pi-attacker live; paid Gemma 4 pinned; 3-pod run BREACH+NOOP+NOOP; entrypoint set -e fixes + single-pass jq extraction; control plane post-completion log re-reads
- iter 0024 | 2026-05-23T19:10:30Z | M4 | accepted | evolution loop wired: fitness.Score (PRD rules), pure-Go Mutate + cross-breed, optional Gemma 4 LLM mutation w/ 32-call budget, RunStore + summary JSON/MD writers, SIGINT cancellation. Live 3-pod 2-gen smoke -> BREACH gen 1 sqli_login (~103s); artifacts at out/summary-r1779563300.{json,md}; pods cleaned.
- iter 0024 | 2026-05-23T19:15:00Z | M5 | accepted | legacy Python + HTML/Phaser artifacts removed; README + DEMO + ARCHITECTURE present; sanitized sample at docs/sample-summary/; build/vet/test green.
- user  ---- | 2026-05-23T19:25:00Z | ALL | reorder  | user moved Nimble to M6; M4=Evolution, M5=Cleanup, M6=Nimble (pending). Existing ACCEPTED milestones preserved; only labels remapped.
- iter 0027 | 2026-05-23T19:44:31Z | M6 | accepted | Nimble live: rebuilt pi-attacker image with nimble_fetch + REQUIRES_NIMBLE prompt; pop-3 smoke BREACHed gen1 sqli_login AND pod 2 invoked nimble on /#/administration (used_nimble:true on raw_findings); summary writer attributes nimble usage; --disable-nimble path produces clean run with no nimble calls; all M0-M6 ACCEPTED -> STATUS: DONE.
- iter 0030 | 2026-05-23T19:46:00Z | ALL | accepted | verified all M0-M6 already ACCEPTED; flipped top-of-file STATUS from PROGRESS to DONE per RALPH_README rule.
