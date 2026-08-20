# Factory Engine Fixes From E26 Factory-Authoring Feedback

---
author: andreas abdi
status: proposed
verified-against: `main` @ `0f3c19544` (2026-08-13)
source: `factories/translation/KNOWN-ISSUES.md` @ `9a8dfc8` (mangaka-cli), E26 rounds 6-9
---

# problem statement

A customer building a real multi-phase Factory hit seven behaviours they read as
engine contract violations; four are genuine engine gaps, and the diagnostic
surfaces were thin enough that each one cost hours of recording-JSON spelunking
rather than minutes.

## customer ask

Fan-in, batch lineage, and re-submission should behave the way the reference
docs describe, and when a Factory stops making progress the runtime should say
why instead of requiring the author to reconstruct enablement by hand.

## solution

Close the four real engine gaps (A4 verification, A6 cross-batch parent
lineage, A7 silent re-upsert, C1-C4 diagnostics), add one authoring-time
validation rule that would have prevented two of the reported stalls outright,
and keep every new mechanism out of the customer-facing vocabulary except the
single field that customers must actually type.

# original document

`C:\Users\andre\work\translate\manga-image-translator\mangaka-cli\factories\translation\KNOWN-ISSUES.md`
(rendered as the "Factory known issues - E26 r6-r9" artifact). Sections A and C
of that document are engine-owned and in scope here. Sections B, D, and E are
customer-factory-owned and are explicitly out of scope.

---

## Baseline verification (do this before writing code)

This plan describes `main` as of `0f3c19544`. Plan documents are snapshots.
Before starting any story, confirm the baseline still holds:

- `git log --oneline -5` and re-read the cited file:line anchors in each story.
- If an anchor has moved or the behaviour already changed, stop and re-scope
  that story rather than implementing against a stale premise.

Two items from the source document are **already resolved on `main`** and must
not be re-implemented:

| Item | Disposition |
|---|---|
| A1 - `ALL_CHILDREN_COMPLETE` vacuous for submitted `PARENT_CHILD` | Fixed 2026-08-08 by `88127e409`, `525a9f854`, `6344b0311`. `Marking.ParentChildRegistrations` (`pkg/services/factory_runtime/internal/orchestrators/petri/marking.go:16-34`) is the reverse index by parent, and `ParentChildRegistrationSet.Complete` makes the guard fail closed on an empty set. |
| A2 - work payload does not survive `AGENT_RUN` | Not a defect. `workPropagation` defaults to `OUTPUT_AS_PAYLOAD`, which replaces the downstream payload with worker output (`token_transformer/transformer.go:213-226`). Documented at `docs/reference/workstations.md:225-248`. Customer-side configuration. |

A3 (`TIMED_OUT` bypassing an explicit `onFailure`) is **deferred, not
scheduled**: a script timeout maps to `WorkOutcome = FAILED`
(`workers/internal/services/workstations/executor/script_test.go:477-491`) and
`onFailure` is documented as covering timeouts
(`docs/reference/workstations.md:219`), so the reported behaviour is not
reproducible from the current code. It re-enters this plan only if S1's
methodology produces a minimal repro.

---

## Scope

**In scope.** Engine behaviour in `infinite-you` only: per-input guard
evaluation on logical workstations, authoring-time route validation,
enablement diagnostics, work-request re-submission reporting, cross-batch
parent lineage, and three read-surface gaps.

**Out of scope.** Everything in the customer's factory (sections B, D, E of the
source document): their closer design, payload cache, work-name conventions,
failure-lane shape, artifact assertions, hot-reload discipline, disk pruning,
and their red `main`. No opportunistic refactors of `scheduler`,
`definitionmapping`, or `requestadmission` beyond what each story names.

---

# changes

## Story sequence

Ordered so each story establishes contract or invariant that later stories
depend on. S1-S2 are independent and can start immediately; S9 must land
before S10.

| # | Behaviour | Depends on |
|---|---|---|
| S1 | Logical workstations honour per-input fan-in guards | - |
| S2 | Authoring rejects states that no workstation can consume | - |
| S3 | The runtime records why a workstation was not enabled | - |
| S4 | `you work explain` reports working / blocked / no-route | S3 |
| S5 | Re-submitting a request id reports that it added nothing | - |
| S6 | `you work list --json` returns real payloads | - |
| S7 | Script stderr appears in the runtime log | - |
| S8 | Visit counts are readable without opening a recording | - |
| S9 | A parent stops accepting children once its fan-in can select it | - |
| S10 | A child can name a parent submitted in an earlier request | S9 |

---

## S1 - Logical workstations honour per-input fan-in guards

**Behaviour.** A `LOGICAL_MOVE` workstation carrying an
`ALL_CHILDREN_COMPLETE` per-input guard stays disabled until every registered
child is terminal, exactly as a worker-backed workstation does.

**Why this is a characterization story first.** The reported constraint is
"guards only evaluate on worker-backed stations," which forces authors into
no-op `/usr/bin/true` workers. That gate is not present in any path I can find:
`mapper.go:97` calls `applyInputGuards` for every workstation unconditionally;
`topology_rules.go:638-766` applies no logical exclusion to per-input guard
validation; and `enablement.go:166-185` builds the full `RuntimeGuardContext`,
including `ParentChildRegistrations`, with no worker-type branch. The reporting
factory hit this before A1's fix landed, when the fan-in guard failed closed for
an unrelated reason - indistinguishable from "guards don't fire here."

Write the test first. If it passes, the story closes as a permanent regression
guard plus a docs line. If it fails, the failure localises the real gate and the
fix follows in the same story.

**Acceptance criteria.**
- A functional test authors a `LOGICAL_MOVE` workstation with a parent input and
  an `ALL_CHILDREN_COMPLETE` guarded child input, registers three children, and
  asserts the transition is not enabled while any child is non-terminal.
- The same test asserts the transition becomes enabled on the tick after the
  third child reaches a terminal place, and consumes all three.
- An `ANY_CHILD_FAILED` variant asserts the parallel behaviour for one failed
  child.
- If the assertions fail against `main`, the story additionally lands the
  narrowest change that makes them pass, and the change does not alter
  worker-backed guard behaviour (existing `scheduler` and `definitionmapping`
  suites stay green without modification).

**Tests.**
- `pkg/services/factory_runtime/internal/services/orchestration/scheduler/enablement_test.go` - unit-level enablement over a logical transition with a guarded arc.
- `tests/functional/factory/` - one end-to-end fan-in through a logical closer, asserting the parent's outbound state only after all children complete.

**Docs.**
- `docs/reference/workstations.md` - state plainly that per-input guards are
  evaluated on `LOGICAL_MOVE` workstations, and that the existing
  `LOGICAL_MOVE` exclusion at line 263 applies to implicit failure-arc
  normalization only. That line was read as covering guards.
- `docs/reference/guards.md` - matching sentence on the per-input guard contract.

---

## S2 - Authoring rejects states that no workstation can consume

**Behaviour.** Loading a Factory whose configuration produces work into a state
that no workstation input consumes, and which is not terminal or failed, emits a
validation finding naming the state and the workstations that produce it.

**Why.** Two of the reported stalls have this single root cause. A3's chapter
hung in `*-dispatched` because nothing consumed `*-set: failed`; A7's deadlock
was a marking with no enabled transition. Both were discovered at 3am from a
quiescent runtime. Both are statically detectable at load. `topology_rules.go`
already owns the `Finding` / `Severity` / `Path` / `Rule` machinery this needs.

**Acceptance criteria.**
- A Factory with an output, `onFailure`, `onRejection`, or `onContinue` route
  into a non-terminal, non-failed state that appears in no workstation
  `inputs[]` produces a finding with a stable rule id (for example
  `state-has-no-consumer`), the state path, and the producing workstation names.
- Terminal and failed states never produce the finding.
- A state consumed only by a `LOGICAL_MOVE` workstation counts as consumed.
- Severity is warning, not error: existing valid Factories that intentionally
  park work must keep loading. The finding must appear in `you` validation
  output where other topology findings appear.
- The packaged factories under `packages/packaged-factories/factories/` load
  without new findings, or the finding is corrected before the story lands.

**Tests.**
- `pkg/services/factory_definitions/internal/services/validation/impl/topology_guards_test.go` - positive case, terminal-state negative case, failed-state negative case, logical-consumer negative case.
- `tests/functional/factory/packaged/` - assert every packaged factory loads with no `state-has-no-consumer` finding.

**Docs.**
- `docs/reference/authoring-factories.md` - document the finding, what triggers
  it, and the two ways to resolve it (route the state, or mark it terminal).

---

## S3 - The runtime records why a workstation was not enabled

**Behaviour.** When the scheduler declines to enable a transition, the reason is
retained as structured data on the tick rather than only formatted into a debug
log line.

**Why.** `enablement.go` already computes five distinct reasons and discards
them into `logger.Debug` with `fmt.Sprintf`:

```
:87   "no input arcs"
:118  "insufficient tokens for unguarded arc %q (place %s, cardinality %d, candidates %d)"
:154  "dependency guard failed for selected binding"
:190  "guard failed for arc %q (place %s, candidates %d)"
:199  "insufficient tokens after guard for arc %q (place %s, cardinality %d, matched %d)"
```

This is an enabling story: it produces no customer-visible behaviour by itself,
and is kept deliberately small because S4 is the behaviour that pays for it.

**Acceptance criteria.**
- A `DisablementReason` value type carries at minimum: a stable reason code, the
  transition id, the arc key, the place id, the candidate count, and, where the
  reason is guard-related, the guard type.
- All five existing reason sites produce that value; no reason site continues to
  exist only as an interpolated string.
- The most recent reason per transition is readable from the tick's engine
  state, with the projection bounded so a long-running session does not
  accumulate unbounded reason history.
- Existing debug log output remains, sourced from the structured value.
- `TestEnablementEvaluator_LogsDisabledGuardFailed` and the disabled-count
  assertions at `enablement_test.go:596` keep passing without modification.

**Tests.**
- `pkg/services/factory_runtime/internal/services/orchestration/scheduler/enablement_test.go` - one case per reason code asserting the emitted value's fields, not its formatted text.
- A replay test near `pkg/services/recordings/internal/projections/` if the projection is persisted, asserting reasons do not alter replay determinism.

**Docs.** None. Internal projection with no customer surface.

---

## S4 - `you work explain` reports working, blocked, or no-route

**Behaviour.** For a given work id, the CLI reports which of three states it is
in, and for the blocked case names the workstation, arc, and reason:

- **working** - an active dispatch holds this work
- **blocked** - candidate workstations exist but each is disabled, with reasons
- **no route** - no workstation input matches this work's type and state

**Why.** This is the question the reporting factory could not answer for hours:
"the chapter sat in `capture-dispatched` with a `joined` set, no command
explained enablement... distinguishing working from stalled from no-route-exists
took log spelunking."

**Acceptance criteria.**
- `you work explain <work-id>` prints the classification and, when blocked, one
  line per candidate workstation with the reason code and the blocking arc.
- The no-route case names the work type and state and states that no workstation
  consumes it, matching S2's rule so the two surfaces agree.
- A work item with an in-flight dispatch reports working and names the
  workstation and dispatch id.
- `--json` emits the same information in a stable machine-readable shape.
- An unknown work id exits non-zero with a specific message.

**Tests.**
- `pkg/transports/cli/work/` - unit coverage per classification against a fixture runtime.
- `tests/functional/transport/cli/` - end-to-end: submit work that cannot route, assert the no-route classification; hold a resource so a workstation is capacity-blocked, assert blocked with the reason.
- `tests/functional/smoke/cli_docs_smoke_test.go` - the new topic resolves if a docs topic is added.

**Docs.**
- `docs/reference/work.md` - the command, its three outcomes, and when to reach
  for it.
- `docs/reference/` topic index and `pkg/transports/cli/baseline/testdata/docs_topic_index.txt` if a topic is added.

---

## S5 - Re-submitting a request id reports that it added nothing

**Behaviour.** Submitting a `FACTORY_REQUEST_BATCH` whose `requestId` and work
names have already been admitted reports how many work items were newly created
and how many already existed, instead of reporting a count indistinguishable
from a successful first submission.

**Why.** `requestadmission/normalize.go:304` mints `batch-<requestId>-<name>`
when `workId` is omitted, so replaying a request id collides on every id. The
submission returns the pre-existing work and prints work ids, so it reads as
success at the call site. In the reported incident this stranded a live chapter
through a designed rework path: the re-dispatch minted nothing and the previous
attempt's tokens were already terminal, leaving a marking with no enabled
transition and no visible failure.

**Acceptance criteria.**
- The submit result distinguishes created work from pre-existing work, and the
  CLI prints both counts.
- Re-submitting an identical request exits non-zero, or exits zero with an
  unmistakable "0 created, N already existed" line - pick one and apply it
  consistently across CLI, HTTP, and MCP submission surfaces.
- A partially overlapping request (some names new, some already admitted) is
  reported accurately rather than rounded to all-or-nothing.
- The behaviour is identical across `you submit batch`, the HTTP upsert
  endpoint, and the MCP tool, per the CLI/API equivalence rule.
- `--dry-run` reports the same counts without creating work.

**Contracts.** The submit response shape gains created/pre-existing counts. This
touches `api/components/schemas/` and regenerates
`pkg/transports/http/generated/server.gen.go`,
`pkg/transports/http/client/client.gen.go`, and `ui/src/api/generated/openapi.ts`.
Run `make generate-api` and `make interfaces-all`; do not hand-edit generated
files.

**Tests.**
- `pkg/services/work/internal/requestadmission/submit_test.go` - created vs pre-existing accounting, including partial overlap.
- `tests/functional/work/transports/cli/submit/batch_contract/` - CLI reporting and exit code.
- `pkg/transports/http/contracttests/` - response shape.
- One functional test asserting CLI and HTTP report identical counts for the same re-submission.

**Docs.**
- `docs/reference/batch-inputs.md` - `requestId` reuse semantics and what the
  counts mean.
- `docs/reference/relationships.md:348-359` - the normalization section states
  that ids are generated as `batch-<requestId>-<work-name>`; add the consequence
  for re-submission.

---

## S6 - `you work list --json` returns real payloads

**Behaviour.** `payload` in work listings carries the work's actual payload, or
the key is absent when there is none. It is never a null that implies an empty
payload on work whose payload is intact.

**Why.** `Work.payload` is declared in `api/components/schemas/data-models/Work.yaml:41`
but the read model never projects it. During the reported A2 investigation the
null read as corroborating evidence for a payload-loss theory that was wrong,
and helped a wrong root cause survive into two releases.

**Acceptance criteria.**
- Listing work with a non-empty payload returns that payload.
- Work with no payload omits the key rather than emitting null.
- If projecting payloads is rejected on response-size grounds, the key is
  removed from the schema instead, and the docs say where to read payloads.
  Silent null is not an acceptable end state either way.

**Contracts.** Either outcome changes `Work.yaml` and regenerates Go and
TypeScript clients. `make generate-api`, `make interfaces-all`.

**Tests.**
- `pkg/services/work/internal/read_test.go` - projection carries payload.
- `pkg/transports/cli/work/list_test.go` - JSON output for both present and absent payload.
- `pkg/transports/http/contracttests/` - schema alignment.

**Docs.**
- `docs/reference/work.md` - what `payload` contains in list responses.

---

## S7 - Script stderr appears in the runtime log

**Behaviour.** When a script worker exits non-zero or times out, its stderr is
visible in the runtime log alongside the exit code.

**Why.** The runtime log records `exit_code` but not stderr. The traceback that
explained the reported failure existed only inside the recording artifact under
`~/.you-agent-factory/recordings/`, reachable by walking JSON. The data is
already carried: `SafeCommandDiagnostic` has a `Stderr` field
(`pkg/services/workers/internal/service/normalize.go:231`).

**Acceptance criteria.**
- A failing script worker logs stderr at the failure site, bounded to a
  documented maximum length with truncation marked.
- Timeout failures log whatever stderr was captured before the kill.
- Successful dispatches do not log stderr at info level.
- The existing redaction posture is preserved: `normalize.go:225-226`
  deliberately omits stdin and env from persisted diagnostics, and this story
  must not widen what is persisted, only what is logged from already-safe
  diagnostics.

**Tests.**
- `pkg/services/workers/internal/services/workstations/executor/script_test.go` - stderr present on the failure path, truncation applied at the boundary.
- One functional test asserting a failing script's stderr reaches runtime log output.

**Docs.**
- `docs/reference/workers.md` - what a failing script worker logs and where the
  full record still lives.

---

## S8 - Visit counts are readable without opening a recording

**Behaviour.** The number of times a work item has visited a workstation is
readable from the CLI.

**Why.** `VISIT_COUNT` guards prove the runtime tracks this, and the customer's
own rework scoping reads `History.TotalVisits`. Nothing in
`pkg/transports/cli` references it, so answering "how many times has this page
looped through review" meant counting events in recording JSON.

**Acceptance criteria.**
- Work detail output includes total visits and per-workstation visit counts.
- `--json` includes the same values under stable keys.
- Work that has never been dispatched reports zero rather than omitting the
  field.

**Contracts.** If visit counts are added to the API work shape, update
`api/components/schemas/data-models/Work.yaml` and regenerate.

**Tests.**
- `pkg/transports/cli/work/` - rendering for zero, single, and repeated visits.
- `pkg/services/recordings/internal/projections/` - projection correctness across a replayed rework loop.

**Docs.**
- `docs/reference/work.md` and `docs/reference/guards.md` - cross-reference the
  visit counts that `VISIT_COUNT` guards act on.

---

## S9 - A parent stops accepting children once its fan-in can select it

**Behaviour.** A parent work item accepts new `PARENT_CHILD` children while it
sits in any state its fan-in workstation does not consume. The first time it
occupies a state that a parent-aware fan-in workstation takes as its parent
input, the child set is sealed; a later request that tries to attach a child to
that parent is rejected with a specific diagnostic.

**Why this story exists, and why it comes before S10.** Today's fan-in
correctness rests on batch atomicity. `marking.go:47-50` says completion is
recorded separately "so callers can add every child from an atomic request
before exposing the set to the scheduler," and the projection deliberately
reopens on a late registration (`marking_test.go:226-228`). That is safe only
because a submitted `PARENT_CHILD` batch must contain both endpoints, so every
child of a parent arrives in one atomic admission. S10 removes that guarantee.
Without a sealing rule, a batch could append a child to a set the fan-in has
already fired on, and `ALL_CHILDREN_COMPLETE` would silently have joined a
partial set - the exact class of bug A1 was just fixed to prevent.

### Design: sealing without a customer-facing field

The constraint is that customers must not learn about child-set completion.
No `childSetComplete`, no `expectedChildCount`, no new authoring vocabulary.

**The customer already declares the set is closed - by where the parent sits.**
`docs/reference/relationships.md:287` instructs authors to "place the parent in
the waiting `state` consumed by the parent-aware fan-in workstation." That
placement is the declaration. A parent parked in a state no fan-in consumes is
still collecting; a parent that has arrived in the fan-in's parent input place
is, by the author's own topology, ready to join. Nothing else needs to be said,
and moving a parent between states is something authors already express with an
ordinary workstation.

So the rule is:

| Phase | Condition | Child admission |
|---|---|---|
| Open | Parent occupies no place bound as a parent input of a parent-aware transition | Accepted; appends to the ordered registration |
| Sealed | Parent has occupied such a place at least once | Rejected with a specific diagnostic |

**Two flags, not one.** `Complete` must keep its current meaning - "the current
admission request finished recording its children" - because it legitimately
oscillates false to true per request and the reopening behaviour is tested.
Sealing is a second, monotone, internal flag on
`ParentChildRegistrationSet`. It only ever transitions once. The fan-in guard
reads sealed; `Complete` remains the intra-request barrier it is today.

**The single-batch case is unchanged.** Customers submitting parent and children
in one batch place the parent directly into the fan-in's waiting state, so the
parent lands in a parent input place on the admission tick and the set seals
immediately. Behaviour is identical to today, which is what backward
compatibility requires.

**Rejected alternatives**, recorded so this is not relitigated:

- `childSetComplete: true` on the final batch - exactly the customer-facing
  surface that was ruled out, and it fails open: forgetting it leaves a set that
  never seals.
- `expectedChildCount: N` on the parent - requires the author to know N before
  submitting any children, which defeats multi-request attachment, and a wrong N
  deadlocks silently. Worst of the options.
- Quiescence or timeout sealing ("seal when no request has touched this parent
  for T") - **disqualified**. This repo's model is event-replay-canonical;
  recordings must replay deterministically, and a wall-clock-dependent seal
  makes replay dependent on scheduling.
- Seal when the first child reaches a terminal state - races with a slow
  submitter and changes the atomic case's behaviour.

**Acceptance criteria.**
- `ParentChildRegistrationSet` carries a monotone sealed flag distinct from
  `Complete`, and sealing is recorded from the marking when a parent token
  enters a place bound as a parent input arc of a transition carrying a
  parent-aware guard.
- Sealing is derived from canonical facts, contains no wall-clock input, and
  survives replay: replaying a recording reconstructs the same sealed state at
  the same tick.
- The fan-in guard requires the set to be sealed; an unsealed set with all
  visible children terminal does not enable the join.
- The existing atomic single-batch fan-in continues to work with no
  configuration change, proven by the existing fan-in functional coverage
  passing unmodified.
- The sealed flag appears in no OpenAPI schema, no CLI output, and no reference
  doc. Its only customer-visible consequence is S10's rejection message.

**Tests.**
- `pkg/services/factory_runtime/internal/orchestrators/petri/marking_test.go` - sealing is monotone; a post-seal registration attempt does not mutate the set.
- `pkg/services/factory_runtime/internal/orchestrators/petri/guard_test.go` - `AllWithParentGuard` fails closed on an unsealed set with all children terminal, and passes once sealed.
- `pkg/services/recordings/internal/replay/` - a recording containing a parent that collects across ticks replays to identical sealed state.
- `tests/functional/factory/` - the existing single-batch fan-in path, unchanged, still joins.

**Docs.** None in `docs/reference/`. This story is deliberately invisible; S10
documents the one observable consequence in behavioural language.

---

## S10 - A child can name a parent submitted in an earlier request

**Behaviour.** A `PARENT_CHILD` relation in a submitted batch may target a work
item that already exists, by work id, instead of requiring the parent to be
declared in the same batch.

**Why.** The reported factory had to carry the set parent inside its children's
batch on every dispatch, which forced attempt-scoped work-name schemes and made
rework dispatches mint a parallel parent each time. The intra-batch rule exists
for a real reason - work names are not unique across requests, which is why
`relationships.md:30-42` scopes relation endpoints to one batch's name
namespace - so the fix is to resolve by id, not to widen name resolution.

`relationships.md:339` currently reserves the field: "Do not use `targetWorkId`
in submitted batch relations." This story turns that reservation on, for
`PARENT_CHILD` only.

**`DEPENDS_ON` is explicitly not widened.** Cross-request prerequisite ordering
would require cycle detection against live Factory state rather than against one
batch's relation set, which is a materially larger change with no reported
customer need.

**Acceptance criteria.**
- A `PARENT_CHILD` relation may carry `targetWorkId` naming an existing work
  item; the child is admitted with that parent's lineage and joins the parent's
  registered child set.
- Supplying both `targetWorkName` and `targetWorkId` on one relation rejects the
  whole batch with a message naming the conflict.
- `targetWorkId` naming a work item that does not exist rejects the whole batch.
- `targetWorkId` naming a parent whose set is already sealed (S9) rejects the
  whole batch with a message that explains the cause in customer terms - the
  parent has already reached the state its fan-in consumes - without naming
  sealing, registrations, or projections.
- `targetWorkId` on a `DEPENDS_ON` relation continues to be rejected, with the
  existing message.
- Whole-batch rejection semantics hold: no partial work and no partial lineage
  is created on any rejection path.
- Behaviour is identical across CLI, HTTP, and MCP submission surfaces.

**Contracts.** `api/components/schemas/` batch relation shape gains
`targetWorkId`. Regenerate with `make generate-api` and `make interfaces-all`;
update `pkg/transports/mapping` mappers and normalizers and the contract tests.
Run `make api-smoke`.

**Tests.**
- `pkg/services/work/internal/requestadmission/normalize_test.go` - accepts id-targeted `PARENT_CHILD`; rejects both-fields, unknown id, sealed parent, and `DEPENDS_ON` with an id.
- `pkg/services/work/internal/lineagegraph/` - lineage derivation for a cross-request parent.
- `tests/functional/factory/batch/batch_characterization_test.go` - a parent submitted in request 1, children attached across requests 2 and 3, then a workstation moves the parent into the fan-in's waiting state and the join fires over all children.
- One functional test asserting a post-seal attach is rejected with the documented message and creates no work.
- `pkg/transports/http/contracttests/` - relation schema.

**Docs.**
- `docs/reference/relationships.md` - replace the blanket "Do not use
  `targetWorkId`" line with the `PARENT_CHILD`-only rule; add `targetWorkId` to
  the source/target semantics table; add the rejection to the whole-batch
  validation table. State the observable rule in customer vocabulary: *a parent
  stops accepting new children once it reaches the state its fan-in workstation
  consumes.* Do not describe sealing, registrations, or the projection.
- `docs/reference/batch-inputs.md` - field table entry for `targetWorkId`.

---

## Quality gates

Per story, narrowest useful tier first, broadening with blast radius:

| Story | Required |
|---|---|
| S1, S3, S9 | `make test`, `make lint` |
| S2 | `make test`, `make lint`, packaged-factory load coverage |
| S4, S7, S8 | `make test`, `make lint`, `make docs-reference-smoke` |
| S5, S6, S10 | `make interfaces-all`, `make test`, `make lint`, `make api-smoke`, `make verify-pr` |

All stories: generated files stay in sync with their source contracts and are
never hand-edited. Stories touching event replay, dispatch lifecycle, or
projections (S3, S5, S9) add or update projection and replay tests near the
affected package.

## Delivery

A story is complete only when required CI is terminal and passing, blocking PR
conversation feedback has been explicitly addressed, merge conflicts are
resolved, and **the PR is merged**. Opening a PR, pushing the latest
implementation, obtaining approval, or reaching green CI without merge does not
count as completion. Shared-file and baseline churn is reconciled through the
same loop.
