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

### IMP-RUN-04 may start now (CheckpointStore port)

**IMP-RUN-04 is authorized to proceed now** against a Runtime-owned opaque
**CheckpointStore** port inside `checkpoint_recovery`. The port is the stable
seam between capture/load/restore logic and byte persistence; peers interact
through Runtime root contracts and do not import CheckpointStore types or
checkpoint bytes.

For IMP-RUN-04 admission and proof obligations:

- A **process-local / default CheckpointStore adapter** (in-process or
  otherwise process-scoped default storage sufficient for the packet) is
  **sufficient** to prove **compatible restore** and **corrupt-checkpoint**
  handling.
- **Recordings durable-log completion is not a gate** for starting IMP-RUN-04.
  Opaque checkpoint capture, codec selection, compatibility policy, and restore
  proofs may land before Recordings durable log, cursor, or retention exist.

### Recordings-backed durable checkpoint storage (follow-on)

A **Recordings-backed durable CheckpointStore adapter** is explicitly
**deferred** until **after** Recordings durable log, cursor, and retention
exist. That adapter persists opaque checkpoint bytes through Recordings durable
artifact authority; codec selection and compatibility remain Runtime-owned.

| Phase | Scope | Recordings durable-log gate? |
| --- | --- | --- |
| **Start now** (IMP-RUN-04) | Runtime opaque CheckpointStore port + process-local/default adapter; compatible-restore and corrupt-checkpoint proofs | **No** — may proceed without Recordings durable log |
| **Follow-on** | Recordings-backed durable CheckpointStore adapter for checkpoint byte persistence | **Yes** — requires Recordings durable log/cursor/retention first |

Maintainers reading only this decision should treat **IMP-RUN-04 may start now**
and **Recordings-backed durable checkpoint bytes are follow-on** as distinct,
non-interchangeable phases.

## Ownership summary

| Concern | Owner | Notes |
| --- | --- | --- |
| Opaque checkpoint capture/load/compatibility/restore | Runtime `checkpoint_recovery` | Versioned opaque blobs; Runtime-owned codec |
| Canonical event history and replay artifacts | Recordings | Durable history authority |
| CheckpointStore port (opaque persistence seam) | Runtime `checkpoint_recovery` | IMP-RUN-04 may start with process-local/default adapter |
| Future durable checkpoint byte persistence | Recordings (storage) + Runtime (codec/compatibility) | Recordings-backed CheckpointStore adapter; follow-on after durable log/cursor/retention |
| Resume identity coordination | Sessions via Recordings + Runtime roots | No checkpoint-internals import on Sessions |
| Top-level Checkpoint service | **Excluded** | Checkpoint/Recovery stays a Runtime nested subservice |

## Explicit exclusions

This decision packet **does not**:

- implement `checkpoint_recovery` code (IMP-RUN-04),
- implement Recordings durable log/cursor/retention,
- create a top-level Checkpoint service package,
- change Runtime or Recordings public package layout beyond docs/checklist/meta,
- make Runtime a second canonical event ledger.

Changed-path lease proof and reviewer verification commands live in
[`checklist.md`](checklist.md) (**DEC-RUN-REC-DURABILITY changed-path lease
proof**) and the lease matrix in [`plan.md`](plan.md).

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
| PSS plan ([`plan.md`](plan.md)) | Runtime sequence step 7 / Checkpoint/Recovery row |
| PSS checklist ([`checklist.md`](checklist.md)) | IMP-RUN-04 admission status |
| Planner meta (`docs/temp/meta.md`) | IMP-RUN-04 hold/admission text |
| Ownership inventory | `docs/internal/baselines/ownership-inventory.json` (`factory_runtime/checkpoint_recovery`) |
| PSS program README | `docs/internal/projects/packaged-service-structure/README.md` |
