# DEC-RUN-REC-DURABILITY: Runtime checkpoint vs Recordings history ownership

| Field | Value |
| --- | --- |
| Status | Accepted |
| Packet | `DEC-RUN-REC-DURABILITY` / `pss-dec-run-rec-durability` |
| Date | 2026-07-28 (UTC) |
| Gates | IMP-RUN-04 (`checkpoint_recovery` implementation) |

This decision record preserves the implementation-time ownership boundary
between Factory Runtime opaque checkpoint recovery and Recordings canonical
history/artifact authority. `IMP-RUN-04` is complete; D1 supersedes its former
durable-checkpoint follow-on. The private process-local adapter is permanent,
and no Recordings-backed checkpoint store or durable log/cursor/retention work
is scheduled. Customer-facing vocabulary remains `Factory Runtime`,
`Recordings`, `Factory Session`, and `Work`; see
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
- JSONL history used for replay and historical inspection.

Runtime must **not** become a second canonical event ledger. D1 cancels durable
checkpoint-byte storage rather than assigning it to Recordings; canonical event
history and replay authority stay with Recordings.

### Sessions durable_execution stays on root contracts

**Sessions** `durable_execution` continues to coordinate resume identity through
**Recordings + Runtime root contracts** rather than importing checkpoint
internals. Session lifecycle and resume admission do not take a direct
dependency on `checkpoint_recovery` package paths.

### IMP-RUN-04 is complete (CheckpointStore port)

**IMP-RUN-04 shipped** a Runtime-owned opaque **CheckpointStore** port inside
`checkpoint_recovery`. The port remains a Runtime-private seam for
process-local checkpoint recovery; peers do not import CheckpointStore types or
checkpoint bytes.

For IMP-RUN-04 admission and proof obligations:

- A **process-local / default CheckpointStore adapter** (in-process or
  otherwise process-scoped default storage sufficient for the packet) is
  **sufficient** to prove **compatible restore** and **corrupt-checkpoint**
  handling.
- The implementation is permanent under D1. It does not create a durable
  checkpoint API, Recordings storage adapter, or storage-engine follow-on.

### Recordings-backed durable checkpoint storage (cancelled under D1)

A **Recordings-backed durable CheckpointStore adapter** is explicitly
**cancelled, not deferred**. D1 prohibits the durable event journal, embedded
database, and write-ahead-log work that such an adapter would require.

| State | Scope | Recordings durable-log gate? |
| --- | --- | --- |
| **Complete** (IMP-RUN-04) | Runtime-private, process-local CheckpointStore adapter; compatible-restore and corrupt-checkpoint proofs | Not applicable — no durable storage is introduced |
| **Cancelled** | Recordings-backed durable CheckpointStore adapter for checkpoint byte persistence | No gate — this work must not be scheduled |

Maintainers should treat the process-local implementation as permanent and the
durable-checkpoint proposal as cancelled under D1.

## Ownership summary

| Concern | Owner | Notes |
| --- | --- | --- |
| Opaque checkpoint capture/load/compatibility/restore | Runtime `checkpoint_recovery` | Versioned opaque blobs; Runtime-owned codec |
| Canonical event history and replay artifacts | Recordings | Durable history authority |
| CheckpointStore port (opaque recovery seam) | Runtime `checkpoint_recovery` | Private process-local implementation shipped in IMP-RUN-04 |
| Durable checkpoint byte persistence | **Cancelled under D1** | No Recordings-backed adapter or durable-log/cursor/retention work is scheduled |
| Resume identity coordination | Sessions via Recordings + Runtime roots | No checkpoint-internals import on Sessions |
| Top-level Checkpoint service | **Excluded** | Checkpoint/Recovery stays a Runtime nested subservice |

## Explicit exclusions

This decision packet **does not**:

- implement `checkpoint_recovery` code (IMP-RUN-04),
- schedule Recordings durable log/cursor/retention or durable checkpoint storage,
- create a top-level Checkpoint service package,
- change Runtime or Recordings public package layout beyond docs/checklist/meta,
- make Runtime a second canonical event ledger.

Changed-path lease proof and reviewer verification commands live in the PSS
[`checklist`](../../temp/projects/packaged-service-structure/checklist.md)
(**DEC-RUN-REC-DURABILITY changed-path lease proof**) and the lease matrix in
the [`plan`](../../temp/projects/packaged-service-structure/plan.md).

## Consequences

### Positive

- Maintainers can cite a single checked-in decision for Runtime vs Recordings
  durability ownership.
- IMP-RUN-04 admission no longer depends on an unowned verbal hold.
- Peer surfaces stay opaque: Petri/JS internals remain inside Runtime.

### Negative / costs

- Runtime must own codec evolution and compatibility policy for checkpoint blobs.
- Checkpoints do not persist across a process lifetime through a new storage
  engine or Recordings-backed adapter.

## Amendment — IMP-RUN-04 closed (ACP-L2-DEL-RUN-CKPT)

`DEC-L2-CKPT` (`docs/internal/projects/root-consolidation/proposal.md` §4)
decided the public Factory Runtime root's `CaptureCheckpoint`,
`LoadCheckpoint`, and `RestoreCheckpoint` cannot be honestly implemented under
`D1` and are deleted rather than implemented. That deletion has landed
(`ACP-L2-DEL-RUN-CKPT-001`). `IMP-RUN-04` (this decision's gate) remains
Factory-terminal via PR #1580 as recorded above; the **Recordings-backed
durable checkpoint storage follow-on** described above is now **cancelled, not
deferred**, consistent with `D1` in the PSS program README. The private
process-local `CheckpointStore` and `checkpoint_recovery` implementation this
decision authorized remain unchanged and permanent. See the full reconciliation
record in
[`docs/internal/projects/packaged-service-structure/README.md`](../projects/packaged-service-structure/README.md#checkpoint-deletion-reconciliation-pss-imp-run-04-vs-l2-dec-l2-ckptdel-run-ckpt).

## Related documents

| Document | Role |
| --- | --- |
| PSS plan ([`plan.md`](../../temp/projects/packaged-service-structure/plan.md)) | Runtime sequence step 7 / Checkpoint/Recovery row |
| PSS checklist ([`checklist.md`](../../temp/projects/packaged-service-structure/checklist.md)) | IMP-RUN-04 final status |
| Planner meta (`docs/temp/meta.md`) | IMP-RUN-04 hold/admission text |
| Ownership inventory | `docs/internal/baselines/ownership-inventory.json` (`factory_runtime/checkpoint_recovery`) |
| PSS program README | `docs/internal/projects/packaged-service-structure/README.md` |
