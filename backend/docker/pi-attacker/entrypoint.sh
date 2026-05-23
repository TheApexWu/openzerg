#!/bin/bash
# OpenZerg attacker pod entrypoint.
#
# Contract with the control plane (see PRD.json data_contracts.attacker_result_jsonl):
#   - Last non-empty line on stdout MUST be a single-line JSON result object.
#   - Earlier stdout lines are free-form; they are ignored by the parser.
#   - Exit 0 even on benign no-breach outcomes. Exit 2 only on infra errors
#     (missing required envs, Pi crash before producing any result).
#
# The control plane calls evolve.ParseLastJSONLine over the streamed log,
# which scans backward for the last "{...}" line; everything else is logs.

set -u
set -o pipefail

log() {
  printf '%s [attacker] %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*"
}

emit_result() {
  # Args: status fitness evidence_string
  local status="$1"
  local fitness="$2"
  local evidence="$3"
  local now_ms
  now_ms="$(date +%s%3N)"
  # jq builds a compact one-line JSON object. --arg is safe against
  # embedded quotes / control characters in evidence.
  jq -c -n \
    --arg type "result" \
    --arg run_id "${RUN_ID:-unknown}" \
    --arg pod_id "${POD_ID:-unknown}" \
    --argjson generation "${GENERATION:-0}" \
    --arg vector "${VECTOR:-unknown}" \
    --arg category "${CATEGORY:-unknown}" \
    --arg status "$status" \
    --argjson fitness "$fitness" \
    --arg evidence "$evidence" \
    --argjson duration_ms "$(( ${SECONDS} * 1000 ))" \
    --argjson t "$now_ms" \
    '{
       type: $type, run_id: $run_id, pod_id: $pod_id,
       generation: $generation, vector: $vector, category: $category,
       status: $status, fitness: $fitness, evidence: $evidence,
       raw_findings: [], duration_ms: $duration_ms, t: $t
     }'
}

require_env() {
  local name="$1"
  if [ -z "${!name:-}" ]; then
    log "missing required env: $name"
    emit_result "ERROR" 0.0 "missing env $name"
    exit 2
  fi
}

# Required envs. OPENROUTER_API_KEY is required for any real model call;
# without it Pi will fail immediately and we want a clean ERROR result.
require_env TARGET_URL
require_env GENOME
require_env OPENROUTER_API_KEY

# Optional / defaulted envs are read with :- below.
GENERATION="${GENERATION:-0}"
RUN_ID="${RUN_ID:-unknown}"
POD_ID="${POD_ID:-unknown}"
TIMEOUT_SECONDS="${TIMEOUT_SECONDS:-60}"
RATE_LIMIT_RPS="${RATE_LIMIT_RPS:-10}"
PI_MODEL="${PI_MODEL:-google/gemma-4-26b-a4b-it}"
PI_PROVIDER="${PI_PROVIDER:-openrouter}"

# Pull vector/category out of the GENOME JSON so the result line can echo
# them. Failing to parse the genome is an ERROR-class infra failure.
if ! VECTOR="$(printf '%s' "$GENOME" | jq -r '.vector // "unknown"')"; then
  log "GENOME is not valid JSON"
  emit_result "ERROR" 0.0 "invalid GENOME json"
  exit 2
fi
CATEGORY="$(printf '%s' "$GENOME" | jq -r '.category // "unknown"')"
HINT="$(printf '%s' "$GENOME" | jq -r '.hint // ""')"
TARGET_PATH="$(printf '%s' "$GENOME" | jq -r '.target_path // ""')"
export VECTOR CATEGORY

log "starting attacker pod (vector=$VECTOR target=$TARGET_URL model=$PI_MODEL)"

# Render the user prompt from the template by substituting placeholders.
# Pi takes the prompt as a positional argument; the system prompt lives in
# the skill (loaded automatically by Pi's skill discovery).
user_prompt_path="/home/node/prompts/user.tmpl"
if [ ! -f "$user_prompt_path" ]; then
  log "missing prompt template at $user_prompt_path"
  emit_result "ERROR" 0.0 "missing user prompt template"
  exit 2
fi

# Compose the prompt. We deliberately keep this short; the skill's SKILL.md
# carries the bulky white-hat scope + output-format instructions.
user_prompt="$(sed \
  -e "s|{{TARGET_URL}}|${TARGET_URL}|g" \
  -e "s|{{VECTOR}}|${VECTOR}|g" \
  -e "s|{{CATEGORY}}|${CATEGORY}|g" \
  -e "s|{{TARGET_PATH}}|${TARGET_PATH}|g" \
  -e "s|{{HINT}}|${HINT}|g" \
  -e "s|{{RATE_LIMIT_RPS}}|${RATE_LIMIT_RPS}|g" \
  "$user_prompt_path")"

# Hard wall-clock cap so a stuck Pi session cannot outlive the pod's
# activeDeadlineSeconds. `timeout` returns 124 on expiry; we treat that as
# a NOOP outcome (the model didn't reach a verdict, but the infra worked).
log "invoking pi --mode json (timeout=${TIMEOUT_SECONDS}s)"
pi_stdout_log="/tmp/pi-stdout.log"
set +e
timeout "${TIMEOUT_SECONDS}" pi \
  --mode json \
  --provider "$PI_PROVIDER" \
  --model "$PI_MODEL" \
  "$user_prompt" \
  > "$pi_stdout_log" 2>&1
pi_exit=$?
set -e

# Echo Pi's JSONL events so they show up in `kubectl logs`. The control
# plane parser ignores anything that is not the final result line.
if [ -s "$pi_stdout_log" ]; then
  log "--- pi event stream (begin) ---"
  cat "$pi_stdout_log"
  log "--- pi event stream (end) ---"
fi

# Look for a result line the model emitted. The skill instructs the model
# to print a `{"type":"result",...}` line at the end of its final turn. We
# scan from the end for the first JSON object that has "type":"result".
final_line="$(grep -E '"type"[[:space:]]*:[[:space:]]*"result"' "$pi_stdout_log" 2>/dev/null | tail -n 1)"

if [ -n "$final_line" ]; then
  # Trust the model's result line as long as it parses as JSON. Validate
  # with jq -e; on parse failure, downgrade to a NOOP.
  if printf '%s' "$final_line" | jq -e . >/dev/null 2>&1; then
    log "emitting model-authored result line"
    printf '%s\n' "$final_line"
    exit 0
  fi
  log "model result line did not parse as JSON; downgrading to NOOP"
fi

case "$pi_exit" in
  0)
    log "pi exited 0 but no model result line found"
    emit_result "NOOP" 0.0 "pi finished without emitting a result line"
    exit 0
    ;;
  124)
    log "pi timed out after ${TIMEOUT_SECONDS}s"
    emit_result "NOOP" 0.0 "pi timed out after ${TIMEOUT_SECONDS}s"
    exit 0
    ;;
  *)
    log "pi failed with exit=$pi_exit"
    emit_result "ERROR" 0.0 "pi exited $pi_exit"
    exit 2
    ;;
esac
