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
| 4 | IMP-RUN-03 — Dispatch Planning private subservice | **Superseded** | Superseded by L2 `IMP-RUN-DISPATCH`; see reconciliation below |
| 5 | Orchestration fold / engine-pipeline CLN | **Factory-terminal** | `CLN-RUN-FOLD-SERVICE` + `CLN-RUN-FOLD-ENGINE-PIPELINE` (#1602 / `a9d50a34b`); DEL-RUN-SERVICE (`655e4167e`) + DEL-RUN-ENGINE (#1637 / `6e48c875f`) terminal |
| 6 | CUT-VIS-RUN / CUT-RUN-WRK / consumer-edge CUTs | Mixed | CUT-VIS-RUN + CUT-RUN-WRK terminal; CUT-RUN-REC admitted with FUN-runtime (`planner-wave-fun-run-cut-run-rec-20260728`) |
| 7 | **Checkpoint/Recovery** (`IMP-RUN-04`, `checkpoint_recovery`) | **Closed under D1** | The private process-local adapter shipped in PR #1580 is permanent. The Recordings-backed durable checkpoint follow-on and its durable-log/cursor/retention prerequisite are cancelled, not deferred. |

### Step 7 — Checkpoint/Recovery (IMP-RUN-04) closed under D1

**Decision owner:** [`DEC-RUN-REC-DURABILITY`](../../../internal/packaged-service-structure/dec-run-rec-durability.md) (accepted and amended by D1).

The prior open question — who owns opaque Runtime checkpoints versus Recordings
durable history — is **no longer unowned**. Planners must cite DEC-RUN-REC-DURABILITY
instead of treating durability as an indefinite verbal hold.

**Final state:**

- The private, process-local `CheckpointStore` adapter shipped with
  `IMP-RUN-04` is the permanent implementation.
- A Recordings-backed durable `CheckpointStore` adapter is **cancelled under
  D1**, not deferred. No durable checkpoint-storage packet, durable event
  journal, or Recordings durable log/cursor/retention sequence remains to be
  admitted.
- The public Runtime checkpoint operations that once motivated a durable
  follow-on were deleted by L2. This plan does not reopen them.

**Explicit non-goals for the decision packet:**

- No additional `checkpoint_recovery` implementation or public checkpoint API.
- No top-level Checkpoint service promotion.
- No Runtime second canonical event ledger.

### Step 4 — Dispatch Planning (IMP-RUN-03) superseded by L2 IMP-RUN-DISPATCH

**Decision owner:** [`docs/internal/projects/packaged-service-structure/README.md`](../../../internal/projects/packaged-service-structure/README.md)
§ "Runtime dispatch ownership reconciliation" (accepted).

L2's `IMP-RUN-DISPATCH` (`docs/internal/projects/root-consolidation/proposal.md`)
is the sole owner of `PlanDispatch`, `AcceptDispatchResult`, and the stable
Runtime dispatch identity L4 consumes. `IMP-RUN-03` claims no Factory Runtime
implementation path going forward:

- The dispatch-planning behavior this step anticipated is already implemented
  on current `main` (`pkg/services/factory_runtime/internal/root.go`), shipped
  under prior dispatch-cutover packets, not under an `IMP-RUN-03` implementation
  packet.
- No coherent PSS-owned remainder exists, so the packet is superseded rather
  than retained as a duplicate dispatch contract.
- This is a metadata/lease reconciliation; it changes no Runtime implementation
  and preserves D1/D2/D3.

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
- `PSS-I05` has been re-scoped to
  [`event-boundary-d2-rescope.md`](../../../internal/projects/packaged-service-structure/event-boundary-d2-rescope.md):
  it records residual metadata only and does not converge persistence with
  streaming.
- `factory_sessions/internal/{responseeventstore,responsestream,cursors}` migrate
  to `pkg/services/events` under L1. PSS packets must not touch those paths while
  L1 is active.

## Composition surfaces are not leased (D3)

`pkg/wire/**`, `pkg/root/**`, and `pkg/initializer/**` are shared, append-only
composition surfaces. Registering a service is a few additive lines; two packets
adding two different providers produce a textual conflict, not a semantic one,
and textual conflicts are resolved by rebase rather than by scheduling.

`PSS-I01` (`root-wire-process`) has already been narrowed to its concrete
structural contract files in the committed manifest. Ordinary additive edits
under `pkg/wire/`, `pkg/root/`, and `pkg/initializer/` are shared and resolved
by normal rebase; only genuine structural contract edits remain exclusive. See
the canonical README's [applied PSS-I01 narrowing](../../../internal/projects/packaged-service-structure/README.md#applied-manifest-narrowing--pss-i01).

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
| [`DEC-RUN-REC-DURABILITY`](../../../internal/packaged-service-structure/dec-run-rec-durability.md) | Durability ownership decision, amended by D1 |
| [`checklist.md`](checklist.md) | Granular implementation checklist recording IMP-RUN-04's final status |
| [`README.md`](README.md) | Local planner index |
| `docs/temp/meta.md` | Planner hold/admission text for IMP-RUN-04 |
| `docs/internal/projects/packaged-service-structure/README.md` | Committed program-metadata index |
| `docs/internal/projects/acp-program/README.md` | ACP Program lane map — owns D1/D2/D3 and the L2 extraction |
| `docs/internal/projects/root-consolidation/` | **L2** — root consolidation this program now depends on |
