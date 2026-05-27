# OpenZerg attacker pod image.
#
# Runs exactly one Pi (https://pi.dev/) coding-agent session against a fixed
# TARGET_URL using a GENOME-defined attack hint, then prints exactly one
# JSON result line on stdout for the control plane to parse.
#
# Pi has no official Docker image, so we install the npm package directly.
# Node 22 is the current LTS; --ignore-scripts skips post-install hooks Pi
# does not need. Base image is debian-slim for curl/jq availability and a
# non-root user out of the box.

FROM node:22-bookworm-slim

# Tools the attacker skill needs from inside the pod:
#  - curl: raw HTTP probes against the target.
#  - jq:   JSON shaping for the final result line.
#  - ca-certificates: TLS to the target (HTTPS).
RUN apt-get update \
 && apt-get install -y --no-install-recommends curl jq ca-certificates \
 && rm -rf /var/lib/apt/lists/*

# Install Pi (the coding agent). --ignore-scripts is the recommended install
# flag in the Pi docs; Pi does not need lifecycle scripts to run.
RUN npm install -g --ignore-scripts @earendil-works/pi-coding-agent

# Pi looks for skills in ~/.pi/agent/skills. The base node image ships a
# "node" non-root user (uid 1000). We adopt it so the pod's runAsNonRoot
# admission check passes without an extra runAsUser pin in the pod spec.
USER node
WORKDIR /home/node

# Ship the attacker skill into the per-user Pi skill directory.
COPY --chown=node:node skills/attacker /home/node/.pi/agent/skills/attacker
USER root
RUN chmod +x /home/node/.pi/agent/skills/attacker/scripts/*.sh
USER node

# Pi model registry overrides. We need to flip supportsDeveloperRole=false
# for qwen/qwen3.6-plus on the openrouter provider — Pi's built-in compat
# block leaves it unset (defaulting to true), but Alibaba upstream rejects
# the OpenAI-style 'developer' role with HTTP 400, killing every probe
# before the model can produce a result line. See pi-config/models.json
# for the override and full context.
COPY --chown=node:node pi-config/models.json /home/node/.pi/agent/models.json

# Prompt templates (system prompt + user prompt scaffold). The entrypoint
# renders these with the per-pod TARGET_URL / GENOME values.
COPY --chown=node:node prompts /home/node/prompts

# Tool wrappers the attacker skill invokes via bash (e.g. nimble_fetch).
COPY --chown=node:node tools /home/node/tools
USER root
RUN chmod +x /home/node/tools/*.sh
USER node

# Entrypoint owns the I/O contract: read env, render prompts, invoke pi,
# capture model output, emit final attacker_result_jsonl line.
COPY --chown=node:node entrypoint.sh /home/node/entrypoint.sh
USER root
RUN chmod +x /home/node/entrypoint.sh
USER node

# Required envs at runtime (set by the k8s pod spec):
#   TARGET_URL          — full URL to attack
#   GENOME              — JSON string with vector/technique/params/hint
#   GENERATION, RUN_ID, POD_ID — bookkeeping echoed into the result line
#   OPENROUTER_API_KEY  — from k8s Secret openzerg-keys
#   NIMBLE_API_KEY      — from k8s Secret openzerg-keys (M4)
#   RATE_LIMIT_RPS      — cap for in-pod attack request rate
#   TIMEOUT_SECONDS     — HARD wall-clock budget. Entrypoint runs pi under
#                         `timeout` with this value; pod also has
#                         activeDeadlineSeconds = TIMEOUT_SECONDS + 30.
#                         0 = unlimited.
#   SOFT_TIMEOUT_SECONDS — SOFT target the agent aims for. The attacker
#                         skill's time_check.sh helper surfaces remaining
#                         budget so the agent wraps up early. Nothing kills
#                         the model when it elapses. 0 = no soft target.
ENV TIMEOUT_SECONDS=600 \
    SOFT_TIMEOUT_SECONDS=60 \
    RATE_LIMIT_RPS=10

ENTRYPOINT ["/home/node/entrypoint.sh"]
