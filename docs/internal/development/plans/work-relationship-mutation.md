# Operator Mutation Of Work Relationships

---
author: andreas abdi
status: proposed
verified-against: `main` @ `0f3c19544` (2026-08-13)
related: `docs/internal/development/plans/factory-engine-e26-known-issues.md`
---

# problem statement

Work relationships can only be declared at admission inside one batch, so an
operator cannot attach a dependency or a parent to work that already exists, and
cannot remove a relationship that has become wrong - including when a stale
relationship is the reason a live Factory Session is deadlocked.

## customer ask

Manipulate Work relationships against live state from the CLI: add a
relationship between existing Work, remove one, and see what a Work item is
currently related to - without re-submitting a batch and without restarting a
run.

## solution

Promote the existing `RELATIONSHIP_CHANGE_REQUEST` canonical event from an
admission-only side effect into the single mechanism for relationship state,
read through a fold rather than an accumulation. Expose attach, detach, and list
over a **generic relationship type** with per-type parameter validation that
mirrors the batch contract exactly. Apply mutations at tick boundaries so
replay and quiescence stay deterministic, and prove acyclicity against live
state before applying.

# original document

`docs/internal/development/plans/factory-engine-e26-known-issues.md` - this plan
is the follow-on discussed alongside it. The deadlock that motivates the
recovery case is item A7 of
`C:\Users\andre\work\translate\manga-image-translator\mangaka-cli\factories\translation\KNOWN-ISSUES.md`.

---

## Baseline verification (do this before writing code)

Plan documents are snapshots. Confirm before starting:

- `FactoryEventTypeRelationshipChangeRequest` still exists in the event
  vocabulary (`pkg/services/recordings/contracts.go:375`) and is still emitted
  only from the batch path (`internal/events/event_history.go:382-411`).
- `ErrMoveWorkRequestAlreadyApplied` and `WorkStateChangeSource`
  (`pkg/services/work/contracts.go:11-14, 499-506`) still describe the operator
  mutation posture this plan copies.
- The per-type relation semantics table at `docs/reference/relationships.md:46-51`
  still defines the rules R2 must mirror.

## What already exists, and what this plan adds

| Already present | Added here |
|---|---|
| `RELATIONSHIP_CHANGE_REQUEST` canonical event with a full relation payload | An operation discriminator, and emission from a non-admission path |
| Operator mutation posture: request id, idempotency, source attribution (`MoveWorkForSession`) | The same posture applied to relationships |
| Batch-scoped `DEPENDS_ON` cycle rejection (`internal/lineagegraph/dependency_graph.go`) | Cycle detection against live Factory state |
| Relations exposed on tokens and in `.Relations` for templates | Those consumers reading a folded view instead of an accumulation |
| Tick-boundary application of queued token injection (`engine/engine.go:874`) | The same queueing for relationship mutations |

## Dependency on the engine known-issues plan

- **S9 (parent set sealing) must land first.** Attaching a `PARENT_CHILD`
  relation to a parent whose fan-in has already selected it is the same hazard
  S9 exists to prevent. This plan reuses that rule and must not invent a second
  one.
- **R4 depends on S3/S4** (structured disablement reasons and `you work explain`).
- **This plan unblocks a later S10 widening.** S10 deliberately keeps
  `DEPENDS_ON` name-scoped because cross-request cycle detection was out of
  scope there. R2 delivers that machinery. Widening S10 is a follow-on decision,
  not part of either plan as written.

## Scope

**In scope.** Attach, detach, and list of mutable relationship types against
live Work; live-state cycle detection; folded relations projection; tick-boundary
application; CLI, HTTP, and MCP parity.

**Out of scope.** Editing authored Factory topology. Bulk or query-based
mutation (`rm --all-depends-on`). Relationship types that are not
author-declarable. Changing batch admission semantics beyond what the folded
projection requires.

---

# changes

## Story sequence

| # | Behaviour | Depends on |
|---|---|---|
| R1 | `you work relation list` reports a Work item's current relationships | - |
| R2 | An operator attaches a relationship of any mutable type between existing Work | R1, S9 |
| R3 | An operator detaches a relationship with a recorded reason | R1, R2 |
| R4 | `--dry-run` reports the enablement change a mutation would cause | R2, R3, S3, S4 |

---

## R1 - `you work relation list` reports current relationships

**Behaviour.** `you work relation list <work-id>` prints the relationships
currently attached to that Work item: type, target, and for `DEPENDS_ON` the
required state and whether it is currently satisfied. Relationship state is
derived by folding relationship-change events rather than by accumulating
attachments.

**Why the fold ships here and not with detach.** R3 introduces supersession, and
a projection that accumulates attaches cannot represent it. Landing the fold
first means R3 adds one event kind rather than rewriting every consumer, and it
means the read path is correct by construction before anything can produce a
detach. With no detach events in existence the fold is output-identical to
today, so this story is safe to land alone.

**The consumer that matters.** `.Relations` is exposed to prompt templates
(`docs/reference/templates.md`). A relation that survives in a template after
being detached would silently feed an agent stale lineage. Every `.Relations`
read path must consume the folded view, not the raw event stream or a cached
token field.

**Acceptance criteria.**
- `you work relation list <work-id>` prints each relationship with type, target
  work id, and target display identity; `DEPENDS_ON` rows additionally show the
  required state and whether the target currently satisfies it.
- Work with no relationships prints an explicit empty result, not a blank.
- `--json` emits a stable shape.
- An unknown work id exits non-zero with a specific message.
- Relations are produced by a fold over relationship-change events; no consumer
  reads an accumulated relation set. Enumerate the `.Relations` consumers
  (templates, CLI work show, HTTP work read, world-state projection) and point
  each at the folded view.
- Output is byte-identical to the current `you work show` relation summary for
  every existing recording, proving the fold is a faithful refactor.

**Tests.**
- `pkg/services/recordings/internal/projections/` - fold reconstructs identical relation state for existing recordings; ordering is deterministic.
- `pkg/transports/cli/work/` - list rendering for empty, single, multi-relation, and satisfied/unsatisfied `DEPENDS_ON`.
- `pkg/services/recordings/internal/replay/` - replay of an existing recording yields unchanged relation state.
- A template-rendering test asserting `.Relations` is sourced from the folded view.

**Docs.**
- `docs/reference/work.md` - the `relation list` command.
- `docs/reference/relationships.md` - a short "reading current relationships"
  section pointing at the command.

---

## R2 - An operator attaches a relationship between existing Work

**Behaviour.**

```
you work relation add --source <work-id> --type <TYPE> --target <work-id> \
                      [--required-state <state>] [--request-id <id>]
```

The relationship is applied at the next tick boundary, recorded as a canonical
relationship-change event, and reported with a typed result that names the tick
it applied at or the reason it did not.

**Generic type, not per-type flags.** The domain model is already generic -
`FactoryRelation` carries `Type`, `SourceWorkID`, `TargetWorkID`,
`RequiredState`. The CLI takes `--type` and validates parameters per type,
mirroring `docs/reference/relationships.md:46-51` exactly so the mutation
contract and the batch contract cannot drift:

| `--type` | `--target` means | `--required-state` |
|---|---|---|
| `DEPENDS_ON` | The prerequisite; source waits for it | Optional, defaults to `complete`; must name a state on the target's work type |
| `PARENT_CHILD` | The parent; source is the child | **Must be omitted**; supplying it rejects the request |
| `SAME_NAME` | - | Rejected: a workstation input guard, not a relation |
| `SPAWNED_BY` | - | Rejected: runtime-derived, not author-declarable |

A new relation type becomes mutable by adding a row to that table and its
validation, not by adding a CLI flag.

**Cycle detection in the general case.** Acyclicity is proven against live
Factory state before the mutation is queued.

- `DEPENDS_ON` forms a graph where an edge source to target means *source waits
  for target*. Adding edge `S -> T` closes a cycle **iff `T` can already reach
  `S`**. That is a single reachability query bounded by the subgraph reachable
  from `T`, not a topological sort of every Work item - the check is incremental
  because the existing graph is already known acyclic.
- `PARENT_CHILD` is checked separately: a Work item must not become its own
  ancestor. Walk the ancestor chain from the proposed parent and reject if it
  reaches the child.
- **The two are deliberately not unified.** A `DEPENDS_ON` edge and a
  `PARENT_CHILD` edge do not compose into a deadlock; parent lineage does not
  gate dispatch, it feeds fan-in guards. State this in the code comment so a
  later reader does not "fix" it into a single graph.
- Traversal is bounded, and **exceeding the bound rejects rather than allows**.
  Fail closed.
- Detection is deterministic and side-effect-free; it runs during admission and
  must not depend on wall clock, map iteration order, or scheduler timing.

**Typed result payload.** Every outcome, success or failure, returns:

```
{ requestId, applied, tick, relation: {type, sourceWorkId, targetWorkId, requiredState},
  failure: { code, message, details } }
```

Initial failure codes, each with the detail a caller needs to act:

| Code | `details` carries |
|---|---|
| `WORK_NOT_FOUND` | Which of source/target was missing, and the id |
| `RELATION_ALREADY_EXISTS` | The existing relation and the tick it was attached |
| `RELATION_TYPE_NOT_MUTABLE` | The rejected type and the list of mutable types |
| `RELATION_PARAMETER_INVALID` | The offending parameter and the per-type rule it broke |
| `DEPENDENCY_CYCLE` | The **ordered work-id path** that closes the cycle |
| `PARENT_ANCESTRY_CYCLE` | The ordered ancestry path |
| `PARENT_SET_SEALED` | The parent id and the state it reached that sealed it (S9) |
| `TARGET_TERMINAL` | The target id and its terminal state |
| `REQUEST_ALREADY_APPLIED` | The tick the original request applied at |

The cycle path is the point of this shape: a caller told "cycle detected" learns
nothing, and a caller told `page-04 -> page-02 -> page-09 -> page-04` can fix it.

**Idempotency and the anti-A7 posture.** `--request-id` is optional and defaults
to a deterministic hash of the operation. Re-applying a request id returns
`applied: false` with `REQUEST_ALREADY_APPLIED` and the original tick. The CLI
exits zero **only** when `applied` is true. A re-run that changed nothing must
never be indistinguishable from a re-run that did something - that is exactly
the failure mode item A7 records.

**Quiescence and tick-boundary application.** Mutations are queued and applied
between ticks, the same way token injection is (`engine/engine.go:874`). The
mutation is never written directly to the marking and then recorded; the event
is emitted and the projection follows it. A mutation may be issued against a
quiescent session and against work with an in-flight dispatch: it applies from
the next tick and does not retroactively affect a running dispatch. Waking a
quiescent Factory is an expected outcome, not an error.

**Acceptance criteria.**
- Attaching each mutable type between two existing Work items succeeds, reports
  the applied tick, and the relationship is visible in `relation list`.
- Per-type parameter rules are enforced exactly as the batch contract enforces
  them, including `--required-state` rejection on `PARENT_CHILD` and required-state
  validation against the target work type's states.
- `SAME_NAME` and `SPAWNED_BY` are rejected with `RELATION_TYPE_NOT_MUTABLE`.
- A `DEPENDS_ON` that would close a cycle against live state is rejected with the
  ordered path; the graph is unchanged and no event is emitted.
- A `PARENT_CHILD` naming a sealed parent is rejected with `PARENT_SET_SEALED`
  and a message in customer vocabulary, consistent with S10's wording.
- Re-issuing an identical request id reports `REQUEST_ALREADY_APPLIED` with the
  original tick and exits non-zero.
- Behaviour is identical across CLI, HTTP, and MCP.
- Replaying a recording that contains an operator attach reconstructs identical
  relation state at the same tick.
- Attaching against a quiescent session with a satisfiable dependency wakes the
  session on the following tick.

**Contracts.** New HTTP endpoint and MCP tool; OpenAPI schemas for the request,
result, and failure shapes; an operation discriminator on the relationship-change
event payload, where an absent operation reads as attach so existing recordings
keep replaying. `make generate-api`, `make interfaces-all`, `make api-smoke`.
Operator request ids share the request-id namespace used for event ids
(`<prefix>/<requestID>/<index>`) and must be prefixed so they cannot collide with
batch request ids.

**Tests.**
- `pkg/services/work/internal/lineagegraph/` - reachability-based cycle detection: no-cycle accept, direct cycle, transitive cycle with the correct path, ancestry cycle, traversal-bound rejection, determinism across repeated runs.
- `pkg/services/work/` - per-type parameter validation table, one case per rule.
- `pkg/transports/cli/work/` - result rendering per failure code, exit codes.
- `pkg/transports/http/contracttests/` - request/result/failure schema.
- `pkg/services/recordings/internal/replay/` - operator attach replays to identical state.
- `tests/functional/factory/` - attach against a quiescent session enables a transition on the next tick; attach against work with an in-flight dispatch does not disturb it.
- One functional test asserting CLI and HTTP return identical results for the same request.

**Docs.**
- `docs/reference/relationships.md` - a mutation section: the generic type
  surface, the per-type parameter table, tick-boundary application, and the
  failure codes. Keep the existing batch-authoring content authoritative for
  batch relations.
- `docs/reference/work.md` - the command and its exit-code rule.

---

## R3 - An operator detaches a relationship with a recorded reason

**Behaviour.**

```
you work relation rm --source <work-id> --type <TYPE> --target <work-id> \
                     --reason "<text>" [--request-id <id>]
```

`--reason` is **mandatory**. The detach is recorded as a superseding
relationship-change event carrying the reason and operator attribution, and the
relation disappears from the folded view from that tick forward.

**Why detach is the loud one.** Attach can only ever add a constraint. Detach
removes one, and can enable a transition the authored topology intended to gate.
It is also the recovery path - the A7 deadlock is resolved by dropping a stale
blocking relationship on a live run - which means it will be used at the worst
possible moment by someone under pressure. The reason is not bureaucracy; it is
the only record of why the graph no longer matches the topology.

**Events are not deleted.** The detach is a new fact that supersedes an earlier
one. R1's fold is what makes this representable.

**Acceptance criteria.**
- Detaching an existing relationship succeeds, reports the applied tick, and the
  relation is absent from `relation list` and from `.Relations` in templates from
  that tick forward.
- Omitting `--reason` fails before any state is touched, with a message saying
  the reason is required.
- The reason and the operator source (`cli` / `api`) appear on the recorded event
  and in relationship history output.
- Detaching a relationship that does not exist returns `RELATION_NOT_FOUND`.
- Detaching a `PARENT_CHILD` relation whose parent set is sealed is rejected with
  `PARENT_SET_SEALED`. There is no `--force`: the fan-in has already selected the
  set, and permitting the detach would retroactively invalidate a join that
  already fired.
- Re-issuing an identical request id reports `REQUEST_ALREADY_APPLIED`; exit zero
  only when `applied` is true.
- Replaying a recording containing attach then detach then attach on the same
  relation reconstructs the correct final state, and reconstructs the correct
  intermediate state at each intervening tick.
- Recordings produced before this story replay unchanged.

**Contracts.** Detach shares R2's endpoint family, result shape, and failure
codes, adding `RELATION_NOT_FOUND` and a required reason field. Regenerate.

**Tests.**
- `pkg/services/recordings/internal/projections/` - fold over attach/detach/attach; supersession ordering under equal ticks.
- `pkg/services/recordings/internal/replay/` - intermediate-state correctness at each tick, and unchanged replay of pre-existing recordings.
- `pkg/transports/cli/work/` - missing reason, not-found, sealed-parent rejection.
- A template test asserting a detached relation is absent from `.Relations`.
- `tests/functional/factory/` - the recovery case end to end: a session blocked on a dependency, detach with a reason, the blocked workstation dispatches on the next tick.

**Docs.**
- `docs/reference/relationships.md` - the detach contract, the mandatory reason,
  and a plain statement that detaching can enable a workstation the authored
  topology intended to gate.
- `docs/reference/record-replay.md` - relationship state is a fold over
  relationship-change events; detach supersedes rather than deletes.

---

## R4 - `--dry-run` reports the enablement change a mutation would cause

**Behaviour.** `--dry-run` on attach or detach reports what the mutation would
do without applying it, including which workstations would become enabled or
disabled on the next tick.

**Why this is the story that makes the feature safe.** Without it, detaching a
relationship on a live run is a guess. With it the operator loop closes:
`you work explain` says why the work is stuck, `relation rm --dry-run` says
whether removing the relation unsticks it, `relation rm --reason` applies it.
It is also cheap here because it is S3's disablement-reason evaluation run
against a hypothetical relation set - the explain machinery and the mutation
machinery are the same machinery.

**Acceptance criteria.**
- `--dry-run` runs every validation R2 and R3 run, including cycle detection, and
  reports the same failure codes without emitting an event or mutating state.
- On a mutation that would succeed, output names the workstations whose
  enablement would change and the disablement reason that would be cleared or
  introduced, using S3's reason codes.
- Output states plainly when enablement would not change.
- `--dry-run` never advances a tick and never wakes a quiescent session.
- `--json` emits the same information.

**Tests.**
- `pkg/services/factory_runtime/internal/services/orchestration/scheduler/` - hypothetical evaluation does not mutate the evaluator or the marking.
- `pkg/transports/cli/work/` - dry-run rendering for enable, disable, and no-change.
- `tests/functional/factory/` - dry-run on the R3 recovery case predicts the enablement that the real detach then produces.

**Docs.**
- `docs/reference/relationships.md` and `docs/reference/work.md` - the dry-run
  workflow, presented as the recommended path before any detach.

---

## Quality gates

| Story | Required |
|---|---|
| R1 | `make test`, `make lint`, `make docs-reference-smoke` |
| R2, R3 | `make interfaces-all`, `make test`, `make lint`, `make api-smoke`, `make verify-pr`, `make docs-reference-smoke` |
| R4 | `make test`, `make lint`, `make docs-reference-smoke` |

All stories touch event replay and projections, so each adds or updates replay
and projection coverage near the affected package. Generated files stay in sync
with their source contracts and are never hand-edited.

## Delivery

A story is complete only when required CI is terminal and passing, blocking PR
conversation feedback has been explicitly addressed, merge conflicts are
resolved, and **the PR is merged**. Opening a PR, pushing the latest
implementation, obtaining approval, or reaching green CI without merge does not
count as completion. Shared-file and baseline churn is reconciled through the
same delivery loop.
