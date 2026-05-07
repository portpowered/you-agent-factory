---
author: Agent Factory Team
last-modified: 2026-04-21
doc-id: agent-factory/guides/workstation-guards-and-exhaustion-limits
---

# Historical Note: Retired Loop-Breaker Guide

This page is retained only so older links to the retired
`exhaustion_rules[]` guidance do not break. New public authoring should use
guarded `LOGICAL_MOVE` loop breakers instead.

Use [Workstation Guards And Guarded Loop Breakers](workstation-guards-and-guarded-loop-breakers.md)
for the current public guidance and the canonical migration example.

## Migration Direction

- Treat `exhaustion_rules[]` as historical compatibility vocabulary only.
- Replace each historical rule with one guarded `LOGICAL_MOVE` workstation that
  keeps the same watched workstation, visit threshold, source, and target.
- If the watched workstation rejects work back to an earlier state, make that
  post-rejection state the guarded loop breaker's source.

## Related

- [Workstation Guards And Guarded Loop Breakers](workstation-guards-and-guarded-loop-breakers.md)
- [Parent-Aware Fan-In](parent-aware-fan-in.md)
- [Work inputs](../reference/work.md)
