#!/usr/bin/env bash
# ralph.sh — OpenZerg ralph loop.
#
# Invokes `opencode run` repeatedly against PRD.json + STATE.md, with each
# iteration making one small forward-progress increment. The agent itself
# decides what to do this iteration based on RALPH_README.md and STATE.md.
#
# Stop conditions (any one):
#   - STATE.md top contains   STATUS: DONE
#   - ./ralph-logs/STOP file exists
#   - --max-iters reached (default 200)
#   - operator Ctrl-C
#
# Pause condition:
#   - NEEDS_USER.md contains any unchecked "- [ ]" line. The loop polls every
#     few seconds and resumes automatically once all checkboxes are checked.
#
# Usage:
#   ./ralph.sh                       # run forever (up to --max-iters)
#   ./ralph.sh --max-iters 5         # quick smoke test
#   ./ralph.sh --sleep 10            # 10s between iters
#   ./ralph.sh --resume              # alias for default behavior
#   touch ralph-logs/STOP            # graceful stop after current iter
#
# Requires: opencode CLI on PATH, authenticated for opencode/claude-opus-4-7.

set -uo pipefail

# ---------------------------------------------------------------------------
# Config
# ---------------------------------------------------------------------------
PROJECT_ROOT="${PROJECT_ROOT:-/home/carson/openzerg}"
LOG_DIR="${PROJECT_ROOT}/ralph-logs"
STATE_FILE="${LOG_DIR}/STATE.md"
README_FILE="${LOG_DIR}/RALPH_README.md"
NEEDS_USER_FILE="${LOG_DIR}/NEEDS_USER.md"
STOP_FILE="${LOG_DIR}/STOP"
PRD_FILE="${PROJECT_ROOT}/PRD.json"

MODEL="${RALPH_MODEL:-opencode/claude-opus-4-7}"
OPENCODE_BIN="${OPENCODE_BIN:-opencode}"
MAX_ITERS="${RALPH_MAX_ITERS:-200}"
SLEEP_BETWEEN="${RALPH_SLEEP:-3}"
PAUSE_POLL_SECONDS="${RALPH_PAUSE_POLL:-10}"

# ---------------------------------------------------------------------------
# Arg parsing (lightweight)
# ---------------------------------------------------------------------------
while [[ $# -gt 0 ]]; do
  case "$1" in
    --max-iters)  MAX_ITERS="$2"; shift 2 ;;
    --sleep)      SLEEP_BETWEEN="$2"; shift 2 ;;
    --model)      MODEL="$2"; shift 2 ;;
    --resume)     shift ;;
    -h|--help)
      sed -n '2,30p' "$0"
      exit 0
      ;;
    *)
      echo "ralph.sh: unknown arg: $1" >&2
      exit 64
      ;;
  esac
done

# ---------------------------------------------------------------------------
# Pre-flight
# ---------------------------------------------------------------------------
mkdir -p "$LOG_DIR"

if ! command -v "$OPENCODE_BIN" >/dev/null 2>&1; then
  echo "ralph.sh: '$OPENCODE_BIN' not found on PATH. Install opencode or set OPENCODE_BIN." >&2
  exit 127
fi

for f in "$PRD_FILE" "$README_FILE" "$STATE_FILE" "$NEEDS_USER_FILE"; do
  if [[ ! -f "$f" ]]; then
    echo "ralph.sh: missing required file: $f" >&2
    exit 66
  fi
done

# Clear any stale stop flag.
rm -f "$STOP_FILE"

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------
iso_now() { date -u +'%Y-%m-%dT%H:%M:%SZ'; }

state_says_done() {
  head -n 5 "$STATE_FILE" | grep -qE '^STATUS:\s*DONE\b'
}

has_unchecked_user_request() {
  # Only consider lines BELOW the "## Open requests" marker. This avoids
  # false positives from documentation/examples earlier in the file.
  awk '/^##[[:space:]]+Open requests/{flag=1; next} flag' "$NEEDS_USER_FILE" 2>/dev/null \
    | grep -qE '^\s*-\s*\[\s\]'
}

list_open_user_requests() {
  awk '/^##[[:space:]]+Open requests/{flag=1; next} flag' "$NEEDS_USER_FILE" 2>/dev/null \
    | grep -E '^\s*-\s*\[\s\]'
}

next_iter_num() {
  # Look at log files, take the highest iter-NNNN.log + 1. Start at 1.
  local max
  max=$(ls "$LOG_DIR" 2>/dev/null \
    | sed -n 's/^iter-\([0-9]\{4,\}\)\.log$/\1/p' \
    | sort -n | tail -n 1)
  if [[ -z "$max" ]]; then
    echo 1
  else
    echo $((10#$max + 1))
  fi
}

print_status_header() {
  local iter="$1"
  echo
  echo "============================================================"
  echo "ralph iter ${iter}  |  $(iso_now)  |  model=${MODEL}"
  echo "------------------------------------------------------------"
  echo "ACCEPTED milestones (will not be re-verified):"
  grep -B1 -E '^\s*-\s*status:\s*ACCEPTED' "$STATE_FILE" \
    | grep -E '^###' | sed 's/^/  /' || echo "  (none yet)"
  echo "------------------------------------------------------------"
  echo "Pending NEEDS_USER items:"
  local _open
  _open=$(list_open_user_requests || true)
  if [[ -n "$_open" ]]; then
    echo "$_open" | sed 's/^/  /'
  else
    echo "  (none)"
  fi
  echo "============================================================"
}

wait_for_user_resolution() {
  echo
  echo "[ralph] Loop paused — NEEDS_USER.md has unresolved items."
  echo "[ralph] Resolve them (set '- [ ]' → '- [x]') or 'touch ${STOP_FILE}' to abort."
  while has_unchecked_user_request; do
    if [[ -f "$STOP_FILE" ]]; then
      echo "[ralph] STOP file found; exiting while paused."
      exit 0
    fi
    sleep "$PAUSE_POLL_SECONDS"
  done
  echo "[ralph] All user requests resolved — resuming."
}

# Cleanly exit on Ctrl-C
trap 'echo; echo "[ralph] interrupted — exiting after current iteration."; exit 130' INT TERM

# ---------------------------------------------------------------------------
# Prompt template
# ---------------------------------------------------------------------------
#
# We keep the prompt SHORT and STABLE. The agent reads the actual content from
# files via its Read tool. This avoids context bloat and keeps every iteration
# starting from the same anchor.
#
build_prompt() {
  local iter="$1"
  cat <<EOF
You are the OpenZerg ralph loop. This is iteration ${iter}.

Make ONE small forward-progress increment on the OpenZerg project. Do not try
to finish multiple milestones in one iteration.

Read these files in order before doing anything:

  1. /home/carson/openzerg/ralph-logs/RALPH_README.md
  2. /home/carson/openzerg/ralph-logs/STATE.md
  3. /home/carson/openzerg/PRD.json
  4. /home/carson/openzerg/ralph-logs/NEEDS_USER.md

Then:

  - Pick the lowest-numbered milestone in STATE.md that is NOT 'ACCEPTED'.
  - Within it, pick the smallest next task from PRD.json milestones[].tasks.
  - Make the smallest possible change that advances that task.
  - Run that milestone's 'verify' commands. Fix forward if red.
  - If ALL of the milestone's 'acceptance' items now pass, update STATE.md to
    mark it 'ACCEPTED' with iso timestamp and a one-line summary.
  - Commit your changes with a conventional-commit message. No co-author
    trailer, no "Generated with" line.
  - If you need human input, append a '- [ ]' line to NEEDS_USER.md and set
    the current milestone to 'BLOCKED' in STATE.md, then exit.

Hard rules:

  - Do NOT re-verify any milestone whose status is already ACCEPTED, unless
    this iteration changed a file that milestone depends on.
  - Do NOT commit any API key value.
  - Do NOT add co-author trailers or "Generated with" lines.
  - Do NOT attack any URL other than context.target.url in PRD.json.
  - Do NOT modify PRD.json or RALPH_README.md unless the user told you to.
  - Do NOT write Python. This is a Go project.

Finish by appending exactly one line to the '## Iteration Log' section at the
bottom of STATE.md, using iter number ${iter}:

  - iter ${iter} | <iso-timestamp> | M<n> | progress|accepted|blocked | <short note>

Now begin. Be brief in chat output; do the work.
EOF
}

# ---------------------------------------------------------------------------
# Main loop
# ---------------------------------------------------------------------------
echo "[ralph] starting. model=${MODEL}  max-iters=${MAX_ITERS}  sleep=${SLEEP_BETWEEN}s"
echo "[ralph] graceful stop: touch ${STOP_FILE}"

iter_count=0
while :; do
  iter_count=$((iter_count + 1))
  if (( iter_count > MAX_ITERS )); then
    echo "[ralph] max-iters (${MAX_ITERS}) reached. exiting."
    exit 0
  fi

  if [[ -f "$STOP_FILE" ]]; then
    echo "[ralph] STOP file present at ${STOP_FILE}. exiting."
    rm -f "$STOP_FILE"
    exit 0
  fi

  if state_says_done; then
    echo "[ralph] STATE.md says STATUS: DONE. all milestones complete. exiting."
    exit 0
  fi

  if has_unchecked_user_request; then
    wait_for_user_resolution
    continue   # re-check stop/done flags after resolution
  fi

  iter_num=$(next_iter_num)
  iter_num_padded=$(printf '%04d' "$iter_num")
  iter_log="${LOG_DIR}/iter-${iter_num_padded}.log"

  print_status_header "$iter_num_padded"

  prompt="$(build_prompt "$iter_num_padded")"

  # Run opencode. Each iteration is a fresh session (no --continue).
  # --dangerously-skip-permissions is REQUIRED for unattended automation.
  # --thinking shows the model's reasoning blocks (so you can read what it's
  # thinking, not just what it ends up doing).
  # We deliberately omit --print-logs / --log-level — those produce opencode's
  # internal service logs (tool registry init, session lifecycle) which are
  # noise, not signal.
  # Output is streamed live to your terminal AND saved to the iter log via tee.
  echo "[ralph] iter ${iter_num_padded} — opencode running (live stream below)"
  echo "------------------------------------------------------------"
  set +e
  # stdbuf -oL to line-buffer so tee flushes promptly.
  # PIPESTATUS[0] gives us opencode's actual exit code through the pipe.
  stdbuf -oL -eL "$OPENCODE_BIN" run \
    --model "$MODEL" \
    --dangerously-skip-permissions \
    --thinking \
    --dir "$PROJECT_ROOT" \
    --title "ralph iter ${iter_num_padded}" \
    "$prompt" 2>&1 \
    | tee "$iter_log"
  rc=${PIPESTATUS[0]}
  set -e
  echo "------------------------------------------------------------"

  if (( rc != 0 )); then
    echo "[ralph] iter ${iter_num_padded} — opencode exited rc=${rc}. log: ${iter_log}"
    echo "[ralph] continuing anyway. inspect log if this repeats."
  else
    echo "[ralph] iter ${iter_num_padded} — done. log: ${iter_log}"
  fi

  # Show what changed in STATE.md this iteration (last 6 log lines).
  if [[ -f "$STATE_FILE" ]]; then
    echo "[ralph] STATE.md tail:"
    tail -n 6 "$STATE_FILE" | sed 's/^/   | /'
  fi

  # Brief breathing room between iterations.
  sleep "$SLEEP_BETWEEN"
done
