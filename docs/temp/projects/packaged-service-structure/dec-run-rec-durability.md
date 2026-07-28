# DEC-RUN-REC-DURABILITY: Runtime checkpoint vs Recordings history ownership

| Field | Value |
| --- | --- |
| Status | Accepted |
| Packet | `DEC-RUN-REC-DURABILITY` / `pss-dec-run-rec-durability` |
| Date | 2026-07-28 (UTC) |
| Gates | IMP-RUN-04 (`checkpoint_recovery` implementation) |

This decision record fixes durable ownership between Factory Runtime opaque
checkpoint recovery and Recordings durable history/artifact authority. It is the
authoritative scope contract for admitting IMP-RUN-04 without waiting for
Recordings durable-log deployment. Customer-facing vocabulary remains `Factory
Runtime`, `Recordings`, `Factory Session`, and `Work`; see
`docs/architecture/data-model.md`.

## Context

The packaged-service-structure plan requires an explicit Runtime-checkpoint versus
Recordings-history ownership decision before Checkpoint/Recovery
implementation. IMP-RUN-04 (`factory_runtime/checkpoint_recovery`) has been
held with planner text treating durability as open, but no owning decision
artifact existed.

Runtime must capture, load, and restore orchestration execution state without
exposing Petri-net or JavaScript orchestration internals to peer services.
Recordings must remain the canonical durable history and artifact authority.
Sessions `durable_execution` must continue coordinating resume identity through
public Recordings and Runtime root contracts rather than importing checkpoint
internals.

## Decision

### Runtime owns opaque checkpoint recovery

**Runtime** owns opaque strategy checkpoint capture, load, compatibility, and
restore inside `pkg/services/factory_runtime/internal/services/checkpoint_recovery`.

Checkpoints are **versioned opaque blobs** with **Runtime-owned codec
selection**. Petri-net and JavaScript orchestration internals must **not**
appear on the peer surface. Peers interact with Runtime through root contracts
and commands; they do not read or interpret checkpoint bytes directly.

Checkpoint/Recovery is a **Runtime private subservice**, not a top-level
Checkpoint service. Do not promote Checkpoint Service to a peer owner or
independent deployable in this program.

### Recordings owns durable history and artifact authority

**Recordings** remains the durable history/artifact authority for:

- canonical Factory events,
- replay artifacts, and
- any future durable checkpoint byte storage.

Runtime must **not** become a second canonical event ledger. Checkpoint bytes
may be stored through Recordings in a follow-on adapter, but canonical event
history and replay authority stay with Recordings.

### Sessions durable_execution stays on root contracts

**Sessions** `durable_execution` continues to coordinate resume identity through
**Recordings + Runtime root contracts** rather than importing checkpoint
internals. Session lifecycle and resume admission do not take a direct
dependency on `checkpoint_recovery` package paths.

## Ownership summary

| Concern | Owner | Notes |
| --- | --- | --- |
| Opaque checkpoint capture/load/compatibility/restore | Runtime `checkpoint_recovery` | Versioned opaque blobs; Runtime-owned codec |
| Canonical event history and replay artifacts | Recordings | Durable history authority |
| Future durable checkpoint byte persistence | Recordings (storage) + Runtime (codec/compatibility) | Follow-on adapter; not a gate for starting opaque Runtime checkpoint work |
| Resume identity coordination | Sessions via Recordings + Runtime roots | No checkpoint-internals import on Sessions |
| Top-level Checkpoint service | **Excluded** | Checkpoint/Recovery stays a Runtime nested subservice |

## Explicit exclusions

This decision packet **does not**:

- implement `checkpoint_recovery` code (IMP-RUN-04),
- implement Recordings durable log/cursor/retention,
- create a top-level Checkpoint service package,
- change Runtime or Recordings public package layout beyond docs/checklist/meta,
- make Runtime a second canonical event ledger.

## Consequences

### Positive

- Maintainers can cite a single checked-in decision for Runtime vs Recordings
  durability ownership.
- IMP-RUN-04 admission no longer depends on an unowned verbal hold.
- Peer surfaces stay opaque: Petri/JS internals remain inside Runtime.

### Negative / costs

- Runtime must own codec evolution and compatibility policy for checkpoint blobs.
- A Recordings-backed durable store adapter remains follow-on work after
  Recordings durable log/cursor/retention exists.

## Related documents

| Document | Role |
| --- | --- |
| PSS plan (`plan.md`) | Runtime sequence step 7 / Checkpoint/Recovery row |
| PSS checklist (`checklist.md`) | IMP-RUN-04 admission status |
| Planner meta (`docs/temp/meta.md`) | IMP-RUN-04 hold/admission text |
| Ownership inventory | `docs/internal/baselines/ownership-inventory.json` (`factory_runtime/checkpoint_recovery`) |
| PSS program README | `docs/internal/projects/packaged-service-structure/README.md` |
