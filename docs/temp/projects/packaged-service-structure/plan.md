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
| 5 | Orchestration fold / engine-pipeline CLN | **Factory-terminal** | `CLN-RUN-FOLD-SERVICE` + `CLN-RUN-FOLD-ENGINE-PIPELINE` (#1602 / `a9d50a34b`); DEL-RUN-SERVICE (`655e4167e`) + DEL-RUN-ENGINE (#1637 / `6e48c875f`) terminal |
| 6 | CUT-VIS-RUN / CUT-RUN-WRK / consumer-edge CUTs | Mixed | CUT-VIS-RUN + CUT-RUN-WRK terminal; CUT-RUN-REC admitted with FUN-runtime (`planner-wave-fun-run-cut-run-rec-20260728`) |
| 7 | **Checkpoint/Recovery** (`IMP-RUN-04`, `checkpoint_recovery`) | **Factory-terminal** (PR #1580) | Durability ownership decided in [`dec-run-rec-durability.md`](dec-run-rec-durability.md). Opaque CheckpointStore + process-local adapter shipped under `factory_runtime/internal/services/checkpoint_recovery`. **Permanent** under D1 — the Recordings-backed durable checkpoint follow-on is cancelled, not deferred. |

### Step 7 — Checkpoint/Recovery (IMP-RUN-04)

**Decision owner:** [`dec-run-rec-durability.md`](dec-run-rec-durability.md) (accepted).

The prior open question — who owns opaque Runtime checkpoints versus Recordings
durable history — is **no longer unowned**. Planners must cite DEC-RUN-REC-DURABILITY
instead of treating durability as an indefinite verbal hold.

**Admission after DEC-RUN-REC-DURABILITY Factory-complete:**

- IMP-RUN-04 (`factory_runtime/checkpoint_recovery`) is **dependency-ready** for
  a future implementation packet.
- Recordings durable-log completion is **not** a gate for starting IMP-RUN-04.
- ~~A Recordings-backed durable CheckpointStore adapter remains explicit
  follow-on work after Recordings durable log/cursor/retention exists.~~
  **Cancelled under D1.** There is no durable log to wait for. The shipped
  process-local adapter is the permanent implementation.

**Explicit non-goals for the decision packet:**

- No `checkpoint_recovery` implementation in DEC-RUN-REC-DURABILITY.
- No top-level Checkpoint service promotion.
- No Runtime second canonical event ledger.

## Recordings implementation sequence (checkpoint bytes context)

**Revised under D1 and D2** — see
[`docs/internal/projects/acp-program/README.md`](../../../internal/projects/acp-program/README.md).

Recordings durable log/cursor/retention was previously planned as a later
Recordings sequence step. It is now **removed from the sequence entirely**. It is
not a later step; it is not a step.

### D1 — no storage engine, ever

You introduces no durable event journal, embedded database, or write-ahead log
for its own state, at any phase. This supersedes the DEC-RUN-REC-DURABILITY
follow-on that anticipated durable checkpoint bytes.

- Historical reconstruction comes from the recorder's recorded JSONL artifacts
  only.
- Session state is session-scoped: it lives for the process lifetime and is
  discarded on exit. Terminating a session is a normal outcome.
- The process-local `CheckpointStore` adapter is permanent.

### D2 — persistence and event stream are separate concerns

Bundling durable log, cursor, and retention into one Recordings step conflated
two concerns with different owners and lifetimes. They split as follows:

| Concern | Owner | Nature |
| --- | --- | --- |
| Canonical Factory history and replay | Recordings | JSONL artifacts on disk |
| Ordering, cursors, subscriptions, retention, gaps, backpressure | Events service (L1) | in-memory, session-scoped |

The Events service is a **stream**, not a store. Recordings remains the canonical
ledger and cedes nothing.

Packet consequences:

- `FND-08` (`pkg/services/recordings/events/kinds/`) is unaffected — event
  *kinds* are contract, not stream.
- `PSS-I05` (`event-backbone`) must be re-scoped before dispatch; a convergence
  that merges persistence with streaming contradicts D2.
- `factory_sessions/internal/{responseeventstore,responsestream,cursors}` migrate
  to `pkg/services/events` under L1. PSS packets must not touch those paths while
  L1 is active.

## Composition surfaces are not leased (D3)

`pkg/wire/**`, `pkg/root/**`, and `pkg/initializer/**` are shared, append-only
composition surfaces. Registering a service is a few additive lines; two packets
adding two different providers produce a textual conflict, not a semantic one,
and textual conflicts are resolved by rebase rather than by scheduling.

`PSS-I01` (`root-wire-process`) must be narrowed accordingly — it keeps
structural surfaces such as the `root.BuildProcess` signature and the
`profiles.go` provider-set shape, and drops the blanket directory claims. The
required manifest change is recorded under **Required manifest follow-up** in
`docs/internal/projects/packaged-service-structure/README.md`; it is not applied
here because the manifest is validated by `internal/psslease`.

## Program lane position

This program is **L3** in the ACP program lane map. It runs in the background and
gates nothing.

The highest-value root cleanup required by ACP Worker Events is extracted into
**L2** (`docs/internal/projects/root-consolidation/`). PSS **depends on L2**
rather than owning that work — the inversion is deliberate, because L2 is small
and fast while PSS is large and slow, and coupling ACP delivery to PSS
scheduling was the constraint being removed.

Packets whose scope L2 covers are superseded rather than re-planned here. When L2
seals a root, the corresponding PSS packet records the L2 packet as a
prerequisite and narrows to whatever remains.

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
| `docs/internal/projects/acp-program/README.md` | ACP Program lane map — owns D1/D2/D3 and the L2 extraction |
| `docs/internal/projects/root-consolidation/` | **L2** — root consolidation this program now depends on |
