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

## Required manifest follow-up

D3 requires a `path-lease-packet-manifest.json` change that has **not** been
applied. It is recorded here rather than executed because the manifest is
validated by `internal/psslease` and edits must be made with its validators.

Target packet — identified by its current exclusive paths:

| Packet | `leaseClass` | Current exclusive paths | Required change |
| --- | --- | --- | --- |
| `PSS-I01` | `root-wire-process` | `pkg/wire/` ; `pkg/root/` ; `pkg/initializer/` | Narrow to the structural surfaces it genuinely owns; drop the blanket directory prefixes so additive composition is unleased |

Constraints the follow-up must respect:

- `ValidateCatalog` requires `PSS-I01` to remain present. The packet is
  **narrowed, not deleted**.
- `exclusivePaths` must stay non-empty; an empty set fails validation.
- The path-overlap rule is prefix-based, so `pkg/wire/` must be replaced with
  specific files rather than left as a directory claim.

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
