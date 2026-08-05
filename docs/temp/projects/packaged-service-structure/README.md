# Packaged Service Structure — planner working state

Local planner mirror for the packaged-service-structure program. Committed
artifacts under this directory are reviewable decision and planner-state
documents; the committed program-metadata ledger lives under
`docs/internal/projects/packaged-service-structure/`.

## Documents

| Document | Role |
| --- | --- |
| [`DEC-RUN-REC-DURABILITY`](../../../internal/packaged-service-structure/dec-run-rec-durability.md) | Runtime opaque checkpoint recovery versus Recordings canonical-history ownership; D1 cancels durable checkpoint storage |
| [`plan.md`](plan.md) | Source plan prose; Runtime step 7 is closed under D1 and step 4 (IMP-RUN-03) is superseded by L2 IMP-RUN-DISPATCH |
| [`checklist.md`](checklist.md) | Granular checklist; IMP-RUN-04 is closed with no durable follow-on, and the Runtime dispatch reconciliation is recorded |

## Cross-links

- Committed program metadata:
  [`docs/internal/projects/packaged-service-structure/README.md`](../../../internal/projects/packaged-service-structure/README.md)
- Ownership inventory baseline:
  [`docs/internal/baselines/ownership-inventory.json`](../../../internal/baselines/ownership-inventory.json)
