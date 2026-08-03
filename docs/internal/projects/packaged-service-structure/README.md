# Packaged Service Structure — Changed-Path Lease and Packet-State Manifest

Program metadata owned by **FND-10**, plus the PSS-F01 initial path-lease freeze
derived from the ownership inventory. Planners update packet leases and states
here (and in co-located validator fixtures) without editing generated OpenAPI,
CLI manifests/generators, or provider registry/conductor surfaces.

## Authority

| Artifact | Role |
| --- | --- |
| [`path-lease-packet-manifest.json`](./path-lease-packet-manifest.json) | Machine-readable lease + packet-state ledger (FND-10) |
| [`ownership-path-lease-freeze.json`](./ownership-path-lease-freeze.json) | PSS-F01 initial exclusive leases for ownership inventory + first PSS-F02 checker slice |
| `internal/psslease` | Decode/validate FND-10 contract; focused fixture tests |
| `internal/ownershipinventory` | Builds/validates the PSS-F01 freeze using FND-10 mechanics |
| `docs/temp/projects/packaged-service-structure/plan.md` | Source plan prose and Changed-Path Lease Matrix |
| [`docs/temp/projects/packaged-service-structure/dec-run-rec-durability.md`](../../../temp/projects/packaged-service-structure/dec-run-rec-durability.md) | DEC-RUN-REC-DURABILITY — Runtime checkpoint vs Recordings history ownership |
| [`docs/internal/projects/acp-program/README.md`](../acp-program/README.md) | ACP Program lane map — owns D1/D2/D3 and the L2 extraction this program now consumes |
| `docs/internal/projects/root-consolidation/` | **L2** — root consolidation extracted from this program; PSS depends on it |

`docs/temp/**` remains local planner working state (gitignored). The committed
ledger under this directory is the reviewable program-metadata source for
scheduling evidence.

## PSS-F01 initial path-lease freeze

`ownership-path-lease-freeze.json` assigns:

- **PSS-F01** (`active`) — exclusive paths for the ownership-inventory artifact,
  freeze artifact, and their validators (`internal/ownershipinventory/`,
  `cmd/ownershipinventoryfreeze/`, `cmd/ownershipinventorycheck/`)
- **PSS-F02** (`ready`) — exclusive path for the first owner-boundary checker
  slice (`owner-boundary-enforcement.md`), unblocked by the freeze

The freeze records portfolio-hold exclusions for CLI-manifest generation and
provider-conductor composition so those live external programs keep their
paths. Focused validation rejects empty exclusive path sets, overlapping
active leases, and claims that collide with those portfolio holds. The combined
verification gate (`make ownership-inventory-check`) proves inventory
completeness, stable sort order, required rationale fields, edge
classifications, named-owner coverage, Process Edges exception presence, and
non-overlapping active leases.

```bash
go test ./internal/ownershipinventory/ ./internal/psslease/ -count=1
make ownership-inventory-check
```

## Packet record shape

Each packet entry requires:

- `packetId` — stable catalog ID (for example `FND-10`)
- `exclusivePaths` — non-empty exclusive changed-path set the packet may edit
- `state` — exactly one of `blocked`, `ready`, `active`, `review`,
  `integration`, and `done`
- `prerequisites` — optional list of packet IDs that must be satisfied first
- `leaseClass` — optional Changed-Path Lease Matrix label for reviewers

Undispatched packets use `blocked` or `ready` according to prerequisite
readiness. Do not place packets into lease-holding states until a planner
intentionally dispatches them.

## Required catalog (Phase 0 + shared lanes)

The committed ledger must include every Phase 0 `FND-01`..`FND-12` foundation
packet plus shared foundation/integration lane IDs `PSS-F01`, `PSS-F02`, and
`PSS-I01`..`PSS-I05`. `internal/psslease.ValidateCatalog` rejects manifests that
omit any of those IDs, duplicate IDs, omit exclusive paths, or omit state.

Exclusive path sets follow the plan’s Changed-Path Lease Matrix wording for each
packet (for example FND-10 remains program-metadata files under this directory
and `internal/psslease/`).

## Lease-holding states

`active`, `review`, and `integration` hold exclusive path claims. Overlapping
exclusive paths among lease holders must fail validation before a packet may be
treated as dispatched/`active`. Non-holding states (`blocked`, `ready`, `done`)
do not block other packets by path overlap alone.

## Path-overlap rule

`ValidateLeaseHolders` and `ValidateDispatchCandidate` treat exclusive paths as
slash-normalized prefixes/files. Two claims overlap when they are equal, or when
one is a path-segment prefix of the other (so `pkg/foo/` covers
`pkg/foo/bar`, and `pkg/foo` covers `pkg/foo/bar`, but `pkg/foo` does not cover
`pkg/foobar`). Before promoting a packet into a lease-holding state, call
`ValidateDispatchCandidate` so overlapping holders are rejected before the
packet is treated as active/dispatched.

## Scope fence

FND-10 owns program metadata only. It does not migrate services, invent a
second architecture tree, or take shared Wire/HTTP/CLI/MCP fan-in cutovers
(`PSS-I01`..`PSS-I05`). Shared Wire/HTTP/CLI/MCP composition remains owned by
those integration packets; do not edit generated OpenAPI bundles, CLI
manifests/generators, or provider registry/conductor surfaces to record lease
or packet-state evidence.

## Program lane position

This program is **L3** in
[`docs/internal/projects/acp-program/README.md`](../acp-program/README.md).

- L3 runs in the **background** and gates nothing. No other lane waits on a PSS
  packet reaching a terminal state.
- The highest-value root cleanup required by ACP Worker Events is extracted into
  **L2** (`docs/internal/projects/root-consolidation/`). PSS **depends on L2**
  rather than owning that work. The dependency is inverted deliberately: L2 is
  small and fast, PSS is large and slow, and coupling ACP delivery to PSS
  scheduling was the constraint being removed.
- Packets whose scope L2 now covers are superseded rather than re-planned here.
  When L2 seals a root, the corresponding PSS packet records the L2 packet as a
  prerequisite and narrows to whatever remains.

PSS retains everything not required by worker events: Petri surface retirement,
architecture-rule enforcement, ownership inventory, behavior baselines, and the
remaining integration lanes.

## Durability and event-stream revisions

Two decisions from the ACP program lane map revise this program's Recordings
sequence. Planners cite them by ID; they are not re-derived here.

### D1 — no storage engine, ever (supersedes a DEC-RUN-REC-DURABILITY follow-on)

[`DEC-RUN-REC-DURABILITY`](../../../temp/projects/packaged-service-structure/dec-run-rec-durability.md)
remains the accepted decision for checkpoint-versus-history ownership. Its
anticipated follow-on — "Recordings-backed durable checkpoint bytes after
Recordings durable log/cursor/retention" — is now **cancelled, not deferred**.

- You introduces no durable event journal, embedded database, or write-ahead log
  for its own state, at any phase.
- The opaque process-local `CheckpointStore` adapter already shipped under
  `factory_runtime/internal/services/checkpoint_recovery` (IMP-RUN-04, PR #1580)
  is the **permanent** answer, not an interim one.
- Historical reconstruction comes from the recorder's recorded JSONL artifacts
  only.
- "Recordings durable log/cursor/retention" is removed from the Recordings
  sequence. It is not a later step; it is not a step.

The read-only SQLite in `provider_sessions` stays read-only and stays scoped to
inspecting third-party agent databases. It is not a precedent.

### D2 — persistence and event stream are separate concerns

This program previously bundled durable log, cursor, and retention into one
Recordings sequence step. That bundling is wrong: the two concerns have
different owners, lifetimes, and failure modes.

| Concern | Owner | Nature |
| --- | --- | --- |
| Canonical Factory history and replay | Recordings | JSONL artifacts on disk |
| Ordering, cursors, subscriptions, retention, gaps, backpressure | Events service (L1) | in-memory, session-scoped |

The Events service is a **stream**, not a store. It carries no persistence
responsibility and has no store package. Recordings remains the canonical ledger
and cedes nothing to it.

Consequences for PSS packets:

- `FND-08` (`event-contract`, `pkg/services/recordings/events/kinds/`) stays
  Recordings-owned and is unaffected — event *kinds* are contract, not stream.
- `PSS-I05` (`event-backbone`) must be re-scoped against D2 before dispatch. An
  "event backbone convergence" that merges persistence with streaming
  contradicts this split.
- `factory_sessions/internal/{responseeventstore,responsestream,cursors}` migrate
  into `pkg/services/events` under L1. **PSS packets must not touch those paths
  while L1 is active.**

## Composition surfaces are not leased

### D3 — `pkg/wire`, `pkg/root`, and `pkg/initializer` carry no exclusive lease

Registering a service in the composition graph is a few additive lines across
`pkg/wire/wire.go`, `pkg/wire/wire_gen.go`, and `pkg/wire/profiles.go`. Holding
an exclusive path lease over those files serialises every lane in the portfolio
behind one packet, for no correctness benefit: two packets adding two different
providers do not produce a semantic conflict, only a textual one.

The lease mechanism exists to prevent **semantically incompatible edits to the
same contract**. It is not a merge-conflict avoidance tool. Textual conflicts in
additive composition are resolved by normal rebase, which is implementation
mechanics rather than a scheduling gate.

Accordingly:

- `pkg/wire/**`, `pkg/root/**`, and `pkg/initializer/**` are **shared,
  append-only composition surfaces**.
- No packet blocks another by claiming them.
- No lane waits for a fan-in integration packet before landing its own
  constructor and lifecycle registration.

Structural changes remain exclusive: altering `root.BuildProcess`'s signature,
restructuring the `profiles.go` provider sets, or changing the `wire_gen`
regeneration contract are contract edits and still warrant a lease.

## Runtime dispatch ownership reconciliation (PSS IMP-RUN-03 vs. L2 IMP-RUN-DISPATCH)

**Status: reconciled.** This is a program-metadata/lease reconciliation record,
not a dispatch implementation change and not a checkpoint-policy decision. It
preserves D1, D2, and D3 above without modification.

### Decision

[`docs/internal/projects/acp-program/README.md`](../acp-program/README.md) §3
already routes "Sealed Runtime dispatch operations" to **L2** for consumption by
**L4**, and
[`docs/internal/projects/root-consolidation/proposal.md`](../root-consolidation/proposal.md)
§5 catalogs `IMP-RUN-DISPATCH` as the packet that implements `PlanDispatch` and
`AcceptDispatchResult` against the sealed Workers execution contract
(`CTR-WRK-EXEC`), returning the dispatch identity L4 associates with a Worker
Session. This record makes that routing binding for PSS:

- **L2 `IMP-RUN-DISPATCH` is the sole owner** of `PlanDispatch`,
  `AcceptDispatchResult`, and the stable Runtime dispatch identity. **L4**
  (`docs/internal/projects/acp-worker-events/proposal.md`) is a **consumer** of
  that identity — it associates a Worker Session with a dispatch, it does not
  implement dispatch planning.
- This program's local plan item `IMP-RUN-03` ("Dispatch Planning private
  subservice", [`plan.md`](../../../temp/projects/packaged-service-structure/plan.md)
  Runtime sequence step 4) is **superseded**, not narrowed. It claims no
  Factory Runtime implementation path. No coherent PSS-owned remainder exists:
  the dispatch-planning behavior that step anticipated is already live on
  current `main` (see Evidence below), delivered under prior dispatch-cutover
  packets rather than under an `IMP-RUN-03` implementation packet, so retaining
  `IMP-RUN-03` as a second, still-open dispatch contract would only duplicate
  L2's sealed ownership.
- No alternate dispatch operation, identity, service, or public transport
  surface is introduced by this reconciliation.

### Evidence (current `main`, verified 2026-08-02)

- `pkg/services/factory_runtime/interfaces.go:85-98` declares `PlanDispatch`
  and `AcceptDispatchResult` on the Runtime root contract.
- `pkg/services/factory_runtime/internal/root.go:106-124` already delegates
  both operations to the active runtime service (`ErrNotRunning` only when no
  session is open) — neither returns `ErrCapabilityUnavailable`. This
  corrects the root-consolidation proposal's E5 evidence, which was captured
  against an earlier snapshot. The three checkpoint methods
  (`CaptureCheckpoint`, `LoadCheckpoint`, `RestoreCheckpoint`) are no longer on
  the root contract at all — see the checkpoint deletion reconciliation below.
- `CTR-WRK-EXEC` (the sealed Workers execution contract `IMP-RUN-DISPATCH`
  depends on per the L2 task catalog) has not landed: there is no sealed
  `WorkstationExecutionService` root and no `CTR-WRK-EXEC` history on `main`.
  L2 `IMP-RUN-DISPATCH` is therefore not yet dispatched; this reconciliation
  assigns ownership prospectively and changes no implementation.
- The committed ledger below (`path-lease-packet-manifest.json`) holds no
  packet with an exclusive path under `pkg/services/factory_runtime/`, so PSS
  holds no lease that could conflict with L2 `IMP-RUN-DISPATCH` admission; no
  manifest edit is required to admit it once `CTR-WRK-EXEC` is satisfied and no
  other lease holder overlaps it. See `internal/psslease` regression coverage
  proving this ledger state passes and that an ambiguous/overlapping
  dispatch-owner ledger is rejected.

## Checkpoint deletion reconciliation (PSS IMP-RUN-04 vs. L2 DEC-L2-CKPT/DEL-RUN-CKPT)

**Status: closed.** This is a program-metadata reconciliation record, not a new
decision and not a reopening of durability scope. It preserves D1 and D2 above
without modification.

### Decision

[`docs/internal/projects/root-consolidation/proposal.md`](../root-consolidation/proposal.md)
§4 `DEC-L2-CKPT` decided that `CaptureCheckpoint`, `LoadCheckpoint`, and
`RestoreCheckpoint` cannot be honestly implemented under D1 (no durable store,
none planned) and are deleted from the Factory Runtime root rather than
implemented. §5 catalogs `DEL-RUN-CKPT` as the packet that performs the
deletion and requires "an amendment note in `packaged-service-structure`
recording that the `IMP-RUN-04` follow-on is closed." `DEL-RUN-CKPT` has now
landed (`ACP-L2-DEL-RUN-CKPT-001`): the three methods, their root-only
vocabulary, and every boundary, caller, and test made dead by the deletion are
removed from the public Factory Runtime root. This record makes that closure
binding for PSS:

- **`IMP-RUN-04` is closed by deletion, not by further implementation.** The
  gate this program's [`dec-run-rec-durability.md`](../../packaged-service-structure/dec-run-rec-durability.md)
  decision recorded for `IMP-RUN-04` (`checkpoint_recovery` implementation) was
  satisfied by PR #1580, as that decision already states; the *public root
  operations* that once motivated a durability follow-on for that
  implementation are now removed under `DEC-L2-CKPT`, so no further PSS
  admission, packet, or scheduling action against `IMP-RUN-04` remains.
- **The private process-local `CheckpointStore` is permanent, not interim.**
  `factory_runtime/internal/services/checkpoint_recovery` (shipped under
  `IMP-RUN-04`, PR #1580) keeps its capture/load/restore behavior unchanged and
  stays a Runtime-private, parent-private subservice. It is not exposed
  through the root, a transport, or a peer dependency.
- **The Recordings-backed durable checkpoint storage follow-on described in
  [`dec-run-rec-durability.md`](../../packaged-service-structure/dec-run-rec-durability.md#recordings-backed-durable-checkpoint-storage-follow-on)
  is cancelled, not deferred**, consistent with D1 above. Recordings JSONL
  artifacts remain the sole canonical history and replay authority; no
  durable checkpoint adapter, journal, or new persistence engine is scheduled.
- This reconciliation creates **no replacement PSS packet, storage-engine
  task, durable journal proposal, L4 requirement, or broader Runtime
  refactoring scope.** It does not claim that the unrelated JavaScript
  workflow checkpoint artifacts or UI replay/timeline checkpoint concepts were
  touched — `DEL-RUN-CKPT` left those unchanged.

### Evidence

- `pkg/services/factory_runtime/interfaces.go` no longer declares
  `CaptureCheckpoint`, `LoadCheckpoint`, or `RestoreCheckpoint` on the Runtime
  root `Service` contract.
- `pkg/services/factory_runtime/checkpoint_deletion_proof_test.go` is the
  external-consumer negative-compilation and positive-behavior proof that the
  three methods are unavailable while supported root operations still compile
  and run.
- `pkg/services/factory_runtime/internal/services/checkpoint_recovery` is
  unchanged in behavior and remains reachable only from its own package's
  `wire` and tests; no root, transport, or peer service imports it (verified by
  ownership-inventory/import audit during `ACP-L2-DEL-RUN-CKPT-002`).
- The committed ledger (`path-lease-packet-manifest.json`) holds no packet with
  an exclusive path under `pkg/services/factory_runtime/`, so this closure
  requires no manifest edit.

## Applied manifest narrowing — PSS-I01

D3 required a `path-lease-packet-manifest.json` change, now **applied**: the
committed ledger's `PSS-I01` packet (`leaseClass: root-wire-process`) claims
exactly the concrete structural contract files it genuinely owns instead of
the blanket `pkg/wire/`, `pkg/root/`, and `pkg/initializer/` directory
prefixes.

| Packet | `leaseClass` | Prior exclusive paths | Applied exclusive paths |
| --- | --- | --- | --- |
| `PSS-I01` | `root-wire-process` | `pkg/wire/` ; `pkg/root/` ; `pkg/initializer/` | `pkg/root/process.go` ; `pkg/wire/profiles.go` ; `pkg/wire/wire.go` |

- `PSS-I01` remains present exactly once, with its lease class, state, and
  prerequisites unchanged. The packet is **narrowed, not deleted**, and
  `exclusivePaths` stays non-empty.
- Because the path-overlap rule (above) is prefix-based, the narrowed set
  lists concrete files rather than directory prefixes, so ordinary additive
  composition elsewhere under `pkg/wire/`, `pkg/root/`, and `pkg/initializer/`
  (for example adding a new provider registration) is no longer serialised
  behind this packet.
- Genuine structural contract edits remain exclusive: a second lease holder
  claiming `pkg/root/process.go`, `pkg/wire/profiles.go`, or `pkg/wire/wire.go`
  while `PSS-I01` holds a lease-holding state is still rejected by
  `ValidateDispatchCandidate`/`ValidateLeaseHolders`, with both packet
  identities and the overlapping path in the diagnostic.
- `internal/psslease` regression coverage proves both halves: an additive
  composition claim outside the retained files (for example
  `pkg/wire/service_registration.go`) is admitted alongside an active
  `PSS-I01`, and a claim on a retained structural file is rejected before
  activation.

If a future planner instead wants "additive edits shared, structural edits
exclusive" expressed directly, that requires a change-kind dimension in
`internal/psslease` which does not exist today. Narrowing the paths is the
change that fits the current mechanism.

`PSS-I05` re-scoping under D2 is a second follow-up and is described above.

## Planner state updates

Update scheduling evidence by editing only this program-metadata ledger (and,
when needed, co-located `internal/psslease` fixtures). Representative lifecycle:

1. `blocked` or `ready` — undispatched; path overlap does not hold a lease
2. `active` — dispatched / lease-holding (run `ValidateDispatchCandidate` first)
3. `review` or `integration` — still lease-holding while exclusive paths are claimed
4. `done` — lease released; packet no longer blocks overlapping dispatch

In code, `psslease.SetPacketState` applies the same gate then writes the new
state. Planners may equivalently edit `state` in
`path-lease-packet-manifest.json` by hand and re-run validation.

Cross-links: plan/checklist lease matrix under
`docs/temp/projects/packaged-service-structure/` (local planner mirror) and the
committed ledger in this directory.

## Validation

```bash
go test ./internal/psslease/ -count=1
```
