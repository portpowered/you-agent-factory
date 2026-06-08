# Infinite You Planner Checklist

Last reviewed: 2026-06-06 22:43:26 +0700

## Planner Control Status

- [ ] `docs/internal/customer-ask.md` exists and names the active customer ask for this repository.
- [ ] The active ask is specific enough to phase into dependency-ordered `idea` work.
- [x] Read live factory submission guidance from `you docs agents`.
- [x] Read live batch schema guidance from `you docs batch-inputs`.
- [x] Read factory-local submission guidance in `factory/docs/overview.md`.
- [x] Read local planning guidance in `docs/internal/standards/code/planning-standards.md`.
- [x] Checked the brief-mandated session with `you work list --session b685c32a-5cfa-42ba-892d-3a8afcbbe427`.
- [x] Checked the repo-local runtime state with `you work list`.
- [x] Checked active and recent sessions with `you session list`.

## Current Pass Result

- [x] Confirmed `docs/internal/customer-ask.md` is still absent.
- [x] Confirmed `factory/internal/asks.md` also records no active checked-in asks.
- [x] Confirmed the hard-coded session ID in the planner brief belongs to another checkout: `/Users/abdifamily/work/awesome-agent-factories`.
- [x] Confirmed the brief-mandated session currently contains only the active planner cron `thoughts:init` item plus previously completed thoughts/ping items.
- [x] Confirmed this repository's `~default` session is the live local runtime and already contains unrelated maintainer lanes in progress.
- [x] Stopped without submitting a new batch because there is no repo-local customer ask to phase and the brief's named session is not this checkout.

## Blocking Notes

- The planner brief says customer intent should be documented in `docs/internal/customer-ask.md`, but that file does not exist in this checkout.
- The checked-in maintainer control surface does not offer a substitute ask; `factory/internal/asks.md` says no active asks are recorded.
- `you session list` shows the brief's target session `b685c32a-5cfa-42ba-892d-3a8afcbbe427` belongs to another repository checkout, so planner submission there would be unsafe from this workspace.
- `you work list` for this checkout shows active unrelated work already in progress, including duplicate or paired lanes for `prd-factory-service-core-coordinator-wire`, `prd-cursor-provider-session-naming-and-loading`, `current-seleciton-standardization`, `runtime-lookup-factory-config-capability-cleanup`, `work-item-current-selection-formatting`, `workstation-current-selection-monaco-formatting`, `trace-drill-down-relationship-cleanup`, and `current-selectoin-work-relations-trace-relations`.
- No `you work move` repair action is warranted in this pass because the observed state is not a broken transition; it is a missing-source-of-truth blocker.

## Next Planner Action

- Wait for a real customer ask to be recorded in `docs/internal/customer-ask.md`, or for the customer to explicitly authorize a different source of truth in conversation.
- If live non-default submission is intended, obtain the correct `infinite-you` session ID; otherwise target the local `~default` session on the next valid planning pass.
- Once an ask exists, rerun queue inspection and submit the next dependency-ordered `idea` batch plus a blocked `thoughts` loopback item.
