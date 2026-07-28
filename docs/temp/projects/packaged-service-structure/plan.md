# Packaged Service Structure — plan

Source plan prose and Changed-Path Lease Matrix for the packaged-service-structure
program. Committed program-metadata ledgers live under
`docs/internal/projects/packaged-service-structure/`.

## Runtime implementation sequence

Factory Runtime nested subservices land in dependency order. Checkpoint/Recovery
is a **Runtime private subservice** (`factory_runtime/checkpoint_recovery`), not a
top-level Checkpoint service.

| Step | Packet / scope | Status | Notes |
| --- | --- | --- | --- |
| 1 | CTR-RUN — Runtime root contract invariants | Factory-terminal | Sealed peer surface for IMP-RUN unlock |
| 2 | IMP-RUN-01 — Petri public-surface retirement | In progress / partial | Baseline-driven Petri boundary retirement |
| 3 | IMP-RUN-02 — Instance Host private subservice | Planned | Depends on root contract + orchestration seams |
| 4 | IMP-RUN-03 — Dispatch Planning private subservice | Planned | Workers command/result contract lock |
| 5 | Orchestration fold / engine-pipeline CLN | Active | `CLN-RUN-FOLD-SERVICE`, `CLN-RUN-FOLD-ENGINE-PIPELINE` |
| 6 | CUT-VIS-RUN / CUT-RUN-WRK / consumer-edge CUTs | Mixed | Visualization and cross-owner edges retarget to Runtime root |
| 7 | **Checkpoint/Recovery** (`IMP-RUN-04`, `checkpoint_recovery`) | **Decision closed; implementation dependency-ready** | Durability ownership decided in [`dec-run-rec-durability.md`](dec-run-rec-durability.md) (**DEC-RUN-REC-DURABILITY**). Runtime owns opaque checkpoint capture/load/compatibility/restore; Recordings remains durable history/artifact authority. IMP-RUN-04 may start with a Runtime opaque CheckpointStore port and process-local/default adapter; Recordings-backed durable checkpoint bytes are follow-on after Recordings durable log/cursor/retention. **This packet does not ship IMP-RUN-04 implementation.** |

### Step 7 — Checkpoint/Recovery (IMP-RUN-04)

**Decision owner:** [`dec-run-rec-durability.md`](dec-run-rec-durability.md) (accepted).

The prior open question — who owns opaque Runtime checkpoints versus Recordings
durable history — is **no longer unowned**. Planners must cite DEC-RUN-REC-DURABILITY
instead of treating durability as an indefinite verbal hold.

**Admission after DEC-RUN-REC-DURABILITY Factory-complete:**

- IMP-RUN-04 (`factory_runtime/checkpoint_recovery`) is **dependency-ready** for
  a future implementation packet.
- Recordings durable-log completion is **not** a gate for starting IMP-RUN-04.
- A Recordings-backed durable CheckpointStore adapter remains explicit follow-on
  work after Recordings durable log/cursor/retention exists.

**Explicit non-goals for the decision packet:**

- No `checkpoint_recovery` implementation in DEC-RUN-REC-DURABILITY.
- No top-level Checkpoint service promotion.
- No Runtime second canonical event ledger.

## Recordings implementation sequence (checkpoint bytes context)

Recordings durable log/cursor/retention is a **later Recordings sequence step**.
It is **not** a prerequisite for admitting IMP-RUN-04 with a process-local/default
CheckpointStore adapter. Future Recordings-backed durable checkpoint byte storage
follows DEC-RUN-REC-DURABILITY phase table (see decision note).

## Changed-Path Lease Matrix (DEC-RUN-REC-DURABILITY)

Decision packets publish planner authority without touching Runtime or Recordings
implementation. **DEC-RUN-REC-DURABILITY** (`pss-dec-run-rec-durability`) holds
the following exclusive changed-path lease:

| Packet | `leaseClass` | Exclusive paths | Forbidden paths (must be untouched) |
| --- | --- | --- | --- |
| **DEC-RUN-REC-DURABILITY** | `decision-planner-state` | `docs/temp/projects/packaged-service-structure/**`; optional IMP-RUN-04 hold text in `docs/temp/meta.md`; supporting durable-artifact infrastructure: `.gitignore` gitignore exceptions for tracked `docs/temp/**` planner paths; `docs/internal/projects/packaged-service-structure/README.md` cross-link index row | `pkg/services/factory_runtime/**` (including `checkpoint_recovery`); `pkg/services/recordings/**`; any new top-level Checkpoint service package |

Reviewers verify the lease with the commands recorded in
[`checklist.md`](checklist.md) under **DEC-RUN-REC-DURABILITY changed-path lease
proof**.

## Related documents

| Document | Role |
| --- | --- |
| [`dec-run-rec-durability.md`](dec-run-rec-durability.md) | DEC-RUN-REC-DURABILITY durability ownership decision |
| [`checklist.md`](checklist.md) | Granular implementation checklist including IMP-RUN-04 admission |
| [`README.md`](README.md) | Local planner index |
| `docs/temp/meta.md` | Planner hold/admission text for IMP-RUN-04 |
| `docs/internal/projects/packaged-service-structure/README.md` | Committed program-metadata index |
