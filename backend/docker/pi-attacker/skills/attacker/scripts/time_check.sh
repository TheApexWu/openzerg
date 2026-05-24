#!/bin/bash
# OpenZerg attacker wall-clock check.
#
# The attacker pod runs under two budgets, set by the entrypoint:
#
#   SOFT (target ~60s) - the model is supposed to aim for this. Going
#       past it is fine, nothing kills the model. This helper returns
#       WARN/EXPIRING as the soft deadline approaches so the agent can
#       wrap up gracefully and emit its final JSON line.
#
#   HARD (default 600s / 10min) - the entrypoint runs pi under `timeout
#       $TIMEOUT_SECONDS`, and the k8s pod has activeDeadlineSeconds =
#       TIMEOUT_SECONDS + 30. Past this, the model is killed mid-call and
#       the control plane never sees a result line -- a wasted probe.
#
# This helper makes the soft budget visible to the model so it converges
# voluntarily. The hard budget is reported only so the model knows the
# real wall (HARD_EXPIRING means: stop NOW, you are about to be killed).
#
# Inputs (env, set by the entrypoint before invoking pi):
#   START_EPOCH_MS          - pod start time in unix milliseconds
#   SOFT_DEADLINE_EPOCH_MS  - soft deadline in unix ms, 0 = no soft budget
#   DEADLINE_EPOCH_MS       - hard deadline in unix ms, 0 = no hard budget
#   SOFT_TIMEOUT_SECONDS    - original soft budget in seconds (for logging)
#   TIMEOUT_SECONDS         - original hard budget in seconds (for logging)
#
# Output: one line of `key=value` pairs, e.g.
#   status=OK elapsed_ms=12345 soft_remaining_ms=47655 hard_remaining_ms=587655 soft_budget_ms=60000 hard_budget_ms=600000
#
# status is one of:
#   UNLIMITED      - no soft AND no hard deadline; proceed freely
#   OK             - >30s of soft budget remaining; proceed
#   WARN           - <=30s of soft budget remaining; finish current probe,
#                    do NOT start a new one
#   EXPIRING       - past soft budget OR <=10s of soft remaining; stop NOW
#                    and emit the final JSON result line with what you have
#   HARD_EXPIRING  - <=30s of HARD budget remaining; you are about to be
#                    killed -- emit the final JSON line IMMEDIATELY (one
#                    shell call), do not start anything else
#   HARD_EXPIRED   - past the hard deadline; SIGTERM is imminent
#
# Exit code mirrors status:
#   0 OK / UNLIMITED
#   1 WARN
#   2 EXPIRING
#   3 HARD_EXPIRING
#   4 HARD_EXPIRED

set -u

now_ms="$(date +%s%3N)"
start_ms="${START_EPOCH_MS:-0}"
soft_deadline_ms="${SOFT_DEADLINE_EPOCH_MS:-0}"
hard_deadline_ms="${DEADLINE_EPOCH_MS:-0}"
soft_budget_s="${SOFT_TIMEOUT_SECONDS:-0}"
hard_budget_s="${TIMEOUT_SECONDS:-0}"

if [ "$start_ms" = "0" ]; then
  # Entrypoint did not stamp a start time. Treat as unlimited so the model
  # does not panic-emit on every check.
  printf 'status=UNLIMITED elapsed_ms=0 soft_remaining_ms=-1 hard_remaining_ms=-1 soft_budget_ms=0 hard_budget_ms=0 reason=no_start_stamp\n'
  exit 0
fi

elapsed_ms=$(( now_ms - start_ms ))
soft_budget_ms=$(( soft_budget_s * 1000 ))
hard_budget_ms=$(( hard_budget_s * 1000 ))

# Compute remainings. -1 sentinel means "no deadline of this kind".
if [ "$soft_deadline_ms" = "0" ]; then
  soft_remaining_ms=-1
else
  soft_remaining_ms=$(( soft_deadline_ms - now_ms ))
fi
if [ "$hard_deadline_ms" = "0" ]; then
  hard_remaining_ms=-1
else
  hard_remaining_ms=$(( hard_deadline_ms - now_ms ))
fi

# Both unlimited? Nothing to enforce.
if [ "$soft_remaining_ms" = "-1" ] && [ "$hard_remaining_ms" = "-1" ]; then
  printf 'status=UNLIMITED elapsed_ms=%d soft_remaining_ms=-1 hard_remaining_ms=-1 soft_budget_ms=0 hard_budget_ms=0\n' "$elapsed_ms"
  exit 0
fi

emit() {
  # status exit_code
  printf 'status=%s elapsed_ms=%d soft_remaining_ms=%d hard_remaining_ms=%d soft_budget_ms=%d hard_budget_ms=%d\n' \
    "$1" "$elapsed_ms" "$soft_remaining_ms" "$hard_remaining_ms" "$soft_budget_ms" "$hard_budget_ms"
  exit "$2"
}

# Hard-deadline pressure ALWAYS wins. Going past it = SIGTERM.
if [ "$hard_remaining_ms" != "-1" ]; then
  if [ "$hard_remaining_ms" -le 0 ]; then
    emit HARD_EXPIRED 4
  fi
  if [ "$hard_remaining_ms" -le 30000 ]; then
    emit HARD_EXPIRING 3
  fi
fi

# Soft-deadline pressure: model is supposed to converge here, but not
# killed if it overruns.
if [ "$soft_remaining_ms" != "-1" ]; then
  # Past soft deadline or within 10s -- tell the model to stop now.
  if [ "$soft_remaining_ms" -le 10000 ]; then
    emit EXPIRING 2
  fi
  if [ "$soft_remaining_ms" -le 30000 ]; then
    emit WARN 1
  fi
fi

emit OK 0
