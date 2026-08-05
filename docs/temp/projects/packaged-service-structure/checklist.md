# Packaged Service Structure — checklist

Granular implementation and admission checklist for the packaged-service-structure
program. Cross-links the committed decision artifacts and planner hold text.

## Runtime — Checkpoint/Recovery (IMP-RUN-04)

**Decision:** [`DEC-RUN-REC-DURABILITY`](../../../internal/packaged-service-structure/dec-run-rec-durability.md) (accepted and amended by D1)

**Admission status:** **Factory-terminal** after IMP-RUN-04 PR #1580 merged
(`3bf957012`). DEC-RUN-REC-DURABILITY remains the ownership decision record;
the private process-local adapter is permanent, and D1 cancels any
Recordings-backed durable checkpoint storage follow-on.

- [x] DEC-RUN-REC-DURABILITY decision note checked in at
  `docs/internal/packaged-service-structure/dec-run-rec-durability.md`
- [x] Decision states Runtime owns opaque `checkpoint_recovery` and Recordings
  owns durable history/artifact authority
- [x] IMP-RUN-04 shipped a Runtime opaque CheckpointStore port with a
  process-local/default adapter
- [x] D1 cancels Recordings-backed durable checkpoint storage and the proposed
  Recordings durable log/cursor/retention sequence; neither is deferred
- [x] Plan Runtime sequence step 7 cites DEC-RUN-REC-DURABILITY; ownership
  decision is no longer open ([`plan.md`](plan.md))
- [x] **IMP-RUN-04 implementation** (`factory_runtime/checkpoint_recovery`) —
  Factory-terminal via PR #1580 / `3bf957012` (`pss-imp-run-04-checkpoint-recovery`);
  opaque CheckpointStore + process-local adapter shipped and permanent under
  D1; no durable checkpoint follow-on remains

### Final scope (closed — no future PSS packet)

The settled scope remains:

- Runtime-owned opaque CheckpointStore port inside `checkpoint_recovery`
- Process-local/default adapter sufficient for compatible-restore and
  corrupt-checkpoint proofs
- No Petri/JavaScript internals on the peer surface
- No top-level Checkpoint service
- No Recordings-backed durable CheckpointStore adapter, durable event journal,
  or Recordings durable log/cursor/retention work

## Runtime — Dispatch reconciliation (IMP-RUN-03 superseded by L2 IMP-RUN-DISPATCH)

**Decision:** [`docs/internal/projects/packaged-service-structure/README.md`](../../../internal/projects/packaged-service-structure/README.md)
§ "Runtime dispatch ownership reconciliation" (accepted).

**Admission status:** Reconciled. Metadata/lease decision only; no Runtime
implementation changed.

- [x] Record names L2 `IMP-RUN-DISPATCH` as sole owner of `PlanDispatch`,
  `AcceptDispatchResult`, and the stable Runtime dispatch identity; L4 named
  only as consumer
- [x] Record states `IMP-RUN-03` is superseded, not narrowed, with no coherent
  remainder, and cites why (behavior already live on `main` under other
  packets)
- [x] `IMP-RUN-03` claims no Factory Runtime implementation path; no second
  dispatch contract or implementation packet remains active
- [x] Record cites current-main prerequisite evidence, including that
  `CTR-WRK-EXEC` (the sealed Workers execution contract `IMP-RUN-DISPATCH`
  depends on) has not yet landed
- [x] Record preserves D1/D2/D3 and is identified as metadata/lease
  reconciliation, not dispatch implementation or checkpoint policy
- [x] Committed ledger (`path-lease-packet-manifest.json`) holds no exclusive
  path under `pkg/services/factory_runtime/`, so it already permits L2
  `IMP-RUN-DISPATCH` admission without edits
- [x] `internal/psslease` regression proves the reconciled single-owner ledger
  state passes and an ambiguous/overlapping dispatch-owner ledger is rejected

## DEC-RUN-REC-DURABILITY changed-path lease proof

Lease matrix: [`plan.md`](plan.md) **Changed-Path Lease Matrix
(DEC-RUN-REC-DURABILITY)**.

- [x] Diff does not add or modify `pkg/services/factory_runtime/**` (including
  `checkpoint_recovery`)
- [x] Diff does not add or modify `pkg/services/recordings/**` for durable
  log/cursor/retention or durable checkpoint storage
- [x] Diff does not create a top-level Checkpoint service package; plan language
  keeps Checkpoint/Recovery as a Runtime private subservice
- [x] Changed paths stay within the DEC-RUN-REC-DURABILITY lease:
  `docs/temp/projects/packaged-service-structure/**`, optional IMP-RUN-04 hold
  text in `docs/temp/meta.md`, plus supporting durable-artifact infrastructure
  (`.gitignore` exceptions, `docs/internal/projects/packaged-service-structure/README.md`
  cross-link index)

### Verification commands (merge base `main`)

Run from the repository root after fetching `main`:

```sh
# Forbidden implementation surfaces must be empty
test -z "$(git diff --name-only main...HEAD -- pkg/services/factory_runtime pkg/services/recordings)"

# No checkpoint implementation paths in the packet diff
! git diff --name-only main...HEAD | rg -i 'pkg/services/.*/checkpoint'

# Observed changed paths (2026-07-28 UTC, branch pss-dec-run-rec-durability)
git diff --name-only main...HEAD
```

Expected changed paths for this packet:

- `.gitignore`
- `docs/internal/projects/packaged-service-structure/README.md`
- `docs/temp/meta.md`
- `docs/temp/projects/packaged-service-structure/README.md`
- `docs/temp/projects/packaged-service-structure/checklist.md`
- `docs/internal/packaged-service-structure/dec-run-rec-durability.md`
- `docs/temp/projects/packaged-service-structure/plan.md`

## Planner cross-links

| Surface | IMP-RUN-04 guidance |
| --- | --- |
| [`plan.md`](plan.md) Runtime sequence step 7 | Closed under D1; the process-local implementation is permanent |
| `docs/temp/meta.md` | Historical planner state; it does not admit a checkpoint packet |
| [`DEC-RUN-REC-DURABILITY`](../../../internal/packaged-service-structure/dec-run-rec-durability.md) | Authoritative ownership decision, amended by D1 |
