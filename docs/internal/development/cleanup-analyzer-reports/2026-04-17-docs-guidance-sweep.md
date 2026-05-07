# Cleanup Analyzer Report: Documentation Guidance Sweep

Date: 2026-04-17

## Scope

Analyzer dispatch over Agent Factory development docs, process guidance, and API inventory docs that mention removed runtime surfaces.

## Evidence

Commands:

```bash
rg -n "/state|/dashboard|/dashboard/stream|/traces|/work/.*/trace|/workflows" docs/processes/agent-factory-development.md libraries/agent-factory/docs -g "*.md"
rg -n "cleanup analyzer|cleanup sweep|api inventory" docs/processes libraries/agent-factory/docs -g "*.md"
```

Findings:

- `docs/processes/agent-factory-development.md` still described long-running observability checks against removed `/state`, `/dashboard`, and `/dashboard/stream` surfaces.
- `libraries/agent-factory/docs/development/api-inventory.md` still documented removed JSON endpoints as active.
- `libraries/agent-factory/docs/development/live-dashboard.md` still described snapshot and trace routes that the browser no longer calls.

## Recommendation

Keep cleanup-analysis artifacts package-local and update process guidance whenever endpoint cleanup changes future verification paths.

## Outcome

Implemented in this sweep:

- Updated Agent Factory process guidance to require cleanup analyzer reports and supported `/status`, `/events`, and `/dashboard/ui` observability checks.
- Rewrote the API inventory around the current supported API plus explicit removed-route inventory.
- Updated live dashboard guidance to describe event-first `/events` bootstrap and trace projection.
