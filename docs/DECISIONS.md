# OpenZerg Decisions

## 2026-05-23 — Pivot frontend is additive, not a refactor

We will leave the original Carson-plan frontend (`index.html`, `openzerg-map.html`, `replay.json`) intact and create a new standalone pivot page for the evolutionary red-team brief.

Why: the original page is still a useful fallback and proves the three-panel K8s story. The pivot needs different product language: generations, fitness, mutation, sponsor integrations, and deployment gating. A new page avoids destabilizing the working demo.

## 2026-05-23 — No secrets in browser for ClickHouse or Nimble

The frontend may display ClickHouse and Nimble integration status, but authenticated calls must go through a local/backend proxy.

Why: ClickHouse Cloud credentials and Nimble API keys cannot safely live in static browser JavaScript. A browser-only implementation can call optional local endpoints when available and otherwise use a labeled demo stream.

Initial proxy contract:

- `GET /api/integrations/clickhouse` returns ClickHouse metric events or status.
- `GET /api/integrations/nimble` returns recent CVE/probe-seed events or status.

## 2026-05-23 — Integration choice

Use ClickHouse plus Nimble as the two competition-visible data/tool integrations.

Why: ClickHouse is credible for high-volume probe/result telemetry and historical aggregation. Nimble maps naturally to the pivot brief by pulling web/CVE data to seed Generation 1 genomes. Together they support both technical implementation and tool-use judging criteria.

### Outcome: ✅ Completed
**Decision:** Created new standalone pivot frontend while preserving the original Carson-plan UI. Real authenticated integrations are intentionally represented via local proxy contracts rather than browser-side secrets.
**Stats:** 0/1 iterations kept
**Finished:** 2026-05-23T14:49:09.032Z
## Experiment: openzerg-style-pass

**Goal:** Remove rounded-corner/glassy slop from new prototype pages and shift visual language toward restrained high-aesthetic SaaS/AI tooling.
**Started:** 2026-05-23T14:56:11.594Z

### Iterations

### Outcome: ✅ Completed
**Decision:** New prototype pages now use a restrained high-aesthetic tooling style: square geometry, flat panels, subtle borders, calmer typography, minimal motion, and reduced color.
**Stats:** 0/1 iterations kept
**Finished:** 2026-05-23T15:04:40.126Z

## 2026-05-23 — OpenAge remix becomes a preview mode, not the literal data UI

The OpenAge/siege metaphor is strong for explaining the product, especially the center animation. We will keep it as a preview/story mode while allowing the left and right panes to switch to literal run data: configuration, integrations, telemetry, and probe results.

Why: the center animation gives the pitch a memorable mental model, but the product still needs a credible operator interface for real data. Separating preview mode from run-data mode lets us keep the aesthetic without obscuring operational details.
