# Live Factory Session Resource and Workstation Controls Plan

## Status

Proposed.

## Problem statement

An active Factory Session cannot currently change resource capacity or enable or
disable an authored workstation without replacing or restarting its Factory
Runtime, and the existing replacement and lifecycle-control paths do not provide
one durable, revisioned timeline that both Petri and JavaScript execution can
reconstruct safely.

## Customer ask

Provide explicit, session-scoped CLI and API controls that:

- set a resource's capacity by its stable resource ID, such as `reviewers`;
- enable or disable a workstation by its stable workstation ID, regardless of
  whether the workstation is cron-triggered, ordinarily scheduled, or backed by
  another supported workstation behavior;
- increase throughput without restarting the Factory Session;
- reduce capacity only by removing resource units that are currently available;
- never terminate or interrupt a workstation dispatch to satisfy a resource
  reduction or workstation disable request; and
- record successful changes on the canonical Factory Session event timeline so
  pause, resume, restart, replay, CLI inspection, and dashboard projections agree.

## Current-state findings

- Factory definition save currently requires an idle runtime and replaces the
  hosted runtime rather than updating one active session revision.
- `FACTORY_CHANGE` already carries a complete replacement Factory snapshot and
  the Recordings world-state reducer already treats it as canonical structure.
- The hosted replacement path writes a change boundary into both the old and
  replacement histories, and a publication error is reported without aborting
  the replacement. It is not a sufficiently strong transaction boundary for
  live controls.
- Runtime resource observation already distinguishes `ResourceID`,
  `InUseCount`, and `AvailableCount`; this is sufficient to enforce an
  available-only reduction without exposing internal Petri tokens publicly.
- Resource and workstation `id` fields are optional in the authored Factory
  contract. Live controls must target `id` only and must not silently fall back
  to mutable display names.
- JavaScript `parallel()` currently creates a fixed semaphore from the policy
  concurrency for one call. Increasing a resource cannot release already
  waiting children until JavaScript child dispatch admission uses a shared,
  resizable session resource controller.
- Durable JavaScript resume already persists runtime records, checkpoint
  summaries, and canonical events, but the current pause operation changes
  status and saves a snapshot without actually suspending the running VM.

## Solution

Introduce one Factory Sessions-owned live-change operation that serializes
resource and workstation controls with dispatch admission and records them on a
stable Factory Session timeline. Typed CLI and API operations remain concise,
but map into the same revisioned domain request.

### ID-addressed CLI

Use singular resource and workstation command families, and name positional
arguments as IDs in help, errors, and JSON:

```text
you session resource set <resource-id> <capacity> [session-id]
you session workstation enable <workstation-id> [session-id]
you session workstation disable <workstation-id> [session-id]
```

Examples:

```text
you session resource set reviewers 8 session-alpha
you session workstation disable nightly-review session-alpha
you session workstation enable nightly-review session-alpha
```

In the first example, `reviewers` is the value of `Resource.id`; it is not a
resource display name. Likewise, `nightly-review` is `Workstation.id`. An
authored resource or workstation without an ID is not live-controllable until
the author adds one. Name fallback, case-insensitive matching, and ambiguous
ID-or-name resolution are explicitly unsupported.

The commands inherit global `--json`, `--server`, `--verbose`, and `--debug`
behavior. They also accept `--request-id`, `--if-revision`, and `--reason`.
The CLI generates a request ID and reads/sends the current revision when those
values are omitted.

### Resource capacity rule

For current capacity `C`, in-use count `U`, available count `A = C - U`, and
requested capacity `R`:

- `R > C`: add `R - C` available units and admit the change.
- `R == C`: return `NO_OP`; do not append a Factory event or increment the
  Factory revision.
- `U <= R < C`: remove `C - R` currently available units and admit the change.
- `R < U`: reject with `RESOURCE_CAPACITY_IN_USE`; do not mutate capacity,
  terminate a dispatch, append a Factory change, or increment the revision.
- `R < 0`: reject validation. Runtime capacity zero is allowed only when
  `U == 0`; it blocks new resource acquisition until capacity is raised.

The reduction is therefore immediate or rejected. This plan does not introduce
a `DRAINING` resource status, delayed capacity request, forced lease revocation,
or dispatch cancellation.

Capacity evaluation and dispatch resource acquisition must share one serialized
admission boundary. In a race, either the reduction commits first and the later
dispatch waits, or the dispatch acquires first and the now-invalid reduction is
rejected. The invariant `capacity >= inUse` must always hold.

For the Petri orchestrator, the implementation may add or remove idle internal
resource units, but token vocabulary remains internal. Removal must be
deterministic and replayable. For JavaScript, child dispatches that declare the
resource ID acquire the same session-scoped resource lease. Raising `reviewers`
must be able to admit waiting JavaScript children in an already-running
workflow; a newly allocated fixed semaphore per `parallel()` call is not
sufficient.

### Workstation enable and disable rule

Workstation state is session-scoped desired state addressed by
`Workstation.id`:

- disabling fences new dispatch requests for that workstation after the
  committed Factory-change sequence;
- dispatch requests already present before that sequence continue to their
  ordinary terminal outcome;
- eligible or queued Work remains durable and may dispatch after re-enable;
- disabling never sends terminate, cancel, or interrupt to an active dispatch;
- cron, watcher, and poller sidecars reconcile the workstation's enabled state
  through Automations rather than receiving a cron-specific public command;
- a cron trigger durably admitted before disable remains ordinary Work, while a
  trigger after disable is not admitted; and
- enabling or disabling the already-desired state returns `NO_OP` without a
  Factory event or revision increment.

JavaScript-only child agents are not implicitly treated as workstations. The
workstation command requires an authored workstation ID in the Current Factory.
Sessions without that ID return `WORKSTATION_NOT_FOUND`; the implementation must
not reinterpret an agent ID, worker name, label, or JavaScript task label as a
workstation ID.

### Canonical Factory Session timeline

The authored Factory remains the persisted baseline. Each successful
session-scoped control creates a new immutable Current Factory revision for that
Factory Session. The effective snapshot includes the updated resource capacity
or workstation enabled state and is reconstructible from canonical Factory
events.

Use the following event progression:

```text
FACTORY_CHANGE_REQUEST
    -> FACTORY_CHANGE
    -> or FACTORY_CHANGE_FAILED
```

- `FACTORY_CHANGE_REQUEST` records an admitted, idempotent request with
  `requestId`, `changeId`, `expectedRevision`, target ID, operation, requested
  value, actor/source, and safe reason.
- Existing `FACTORY_CHANGE` remains the sole success boundary. Extend its
  payload additively with `changeId`, `previousRevision`, `revision`,
  `effectiveAtSequence`, and `scope: FACTORY_SESSION`, while retaining the full
  effective Factory snapshot.
- `FACTORY_CHANGE_FAILED` closes a previously admitted request that could not
  be committed. It carries a stable code, stage, retryability, safe message,
  and unchanged revision. It never exposes stack traces, secrets, command
  lines, or provider payloads.

Malformed inputs, missing sessions, missing IDs, stale revisions, negative
capacity, `R < U`, terminal sessions, and exact no-ops are resolved before
request admission and do not enter the canonical Factory timeline. Once a
`FACTORY_CHANGE_REQUEST` is appended, recovery must eventually produce either
`FACTORY_CHANGE` or `FACTORY_CHANGE_FAILED`. A repeated request ID with the same
normalized body returns the original result; reuse with a different body is a
conflict.

Every effect-bearing event, dispatch request, JavaScript checkpoint, and
session snapshot must carry or resolve the effective Factory revision. A
dispatch admitted before `effectiveAtSequence` retains its original revision;
the change is never applied retroactively to an active dispatch.

### Pause, resume, and restart behavior

Live changes are accepted in `RUNNING` or durably `PAUSED` sessions and rejected
while another lifecycle/change transition is committing. Terminal and
historical replay sessions remain immutable.

A real pause boundary must fence new automation and dispatch admission, settle
or durably classify mediated in-flight work, persist a resumable checkpoint and
event cursor, and only then publish accepted `PAUSED` state. It must not claim
the session is paused while JavaScript continues creating effects.

Because arbitrary Goja stack serialization is not a supported primitive,
JavaScript pause/resume uses engine-managed safe points and the existing
completed-dispatch replay strategy. Checkpoints are bound to source hash,
arguments digest, policy hash, Factory revision, and canonical event cursor.
Resume reconstructs the last successful Factory revision before reopening
automation or dispatch admission.

A change made while paused is recorded as the next Factory revision and is in
force before resume. Restart recovery replays any unclosed change request
idempotently, resolves it, reconstructs resource/workstation state, and then
allows the session to resume. Source, argument, orchestrator kind, topology,
guard, worker/provider binding, and semantic JavaScript policy changes remain
outside this live-control contract and require the existing idle replacement
path or a new Factory Session.

## Original document

There is no earlier standalone product plan for this request. This plan records
the clarified design and is constrained by:

- `C:\Users\andre\work\portos\infinite-you\docs\architecture\architecture.md`
- `C:\Users\andre\work\portos\infinite-you\docs\architecture\structures.md`
- `C:\Users\andre\work\portos\infinite-you\docs\architecture\data-model.md`
- `C:\Users\andre\work\portos\infinite-you\docs\architecture\packaged-structure.md`
- `factory/docs/standards/planning-standards.md`
- `C:\Users\andre\work\portos\infinite-you\docs\internal\standards\code\general-backend-standards.md`
- `C:\Users\andre\work\portos\infinite-you\docs\internal\standards\code\code-review-standards.md`

## Work stories

### Story 1 — Revisioned live-change admission

As an operator, I receive one idempotent, ordered outcome when I request a live
Factory Session change, so retries, reconnects, and process restarts cannot
duplicate or lose the change.

Acceptance criteria:

- Factory Sessions exposes one service-root live-change operation and owns
  request normalization, request-ID replay, expected-revision checks, session
  lifecycle checks, and serialization with dispatch admission.
- An admitted request produces exactly one request event and exactly one
  terminal success or failure event in canonical sequence order.
- `FACTORY_CHANGE` advances the revision exactly once and contains the complete
  effective Factory snapshot; rejection and no-op leave the revision unchanged.
- A crash after the request event but before its terminal event is recovered
  idempotently without duplicating resources, sidecars, dispatches, or events.
- Event projections, retained response streams, recording export/import, and
  reconnect cursors preserve the change boundary and request correlation.
- Operation logs record accepted intent and terminal outcome with session ID,
  request ID, change ID, revision, operation kind, and target ID, without
  sensitive payloads.

### Story 2 — Set resource capacity by resource ID

As an operator, I can increase or safely reduce the capacity of resource ID
`reviewers` on an active Factory Session without restarting it or interrupting
work already using that resource.

Acceptance criteria:

- `you session resource set reviewers <capacity> [session-id]` and the matching
  API operation target `Resource.id` only; mutable `name` is presentation data
  and is never a fallback lookup key.
- Increasing capacity makes new capacity available in the same session and can
  increase throughput for waiting Petri and resource-bound JavaScript child
  dispatches.
- Reducing capacity succeeds only when `requestedCapacity >= inUse` and removes
  exactly the required number of currently available units.
- A request below `inUse` returns `RESOURCE_CAPACITY_IN_USE` with
  `resourceId`, `currentCapacity`, `requestedCapacity`, `inUse`, `available`,
  and `minimumAllowedCapacity`; runtime state and Factory revision are
  unchanged.
- No resource change path invokes dispatch cancel, interrupt, or terminate.
- Capacity zero succeeds only when no unit is in use and prevents new
  acquisition until capacity is raised.
- Human and JSON output report resource ID, previous/requested capacity,
  in-use/available counts, outcome, revision, and links without using
  `reviewers` as though it were a display name.

### Story 3 — Enable and disable by workstation ID

As an operator, I can disable or enable a workstation by ID so future work is
fenced without terminating a dispatch that already started.

Acceptance criteria:

- `you session workstation disable <workstation-id> [session-id]` and `enable`
  target `Workstation.id` only and return a typed missing-ID diagnostic when no
  matching authored workstation exists.
- Disabling commits one Factory revision and prevents new dispatch requests for
  the workstation after that event sequence.
- Active dispatches continue unchanged and publish their ordinary result; no
  lifecycle interruption is synthesized.
- Eligible Work remains visible while disabled and becomes schedulable after
  re-enable.
- Automations reconciles cron, watcher, and poller sources from the same
  workstation enabled state; no cron-specific public operation is added.
- Fake-clock cron coverage proves disabled boundaries emit no new scheduled
  Work, re-enable resumes at a later valid boundary, and each nominal boundary
  emits at most once.
- A JavaScript agent/label that happens to equal the requested workstation ID
  is not accepted unless an authored workstation with that ID exists.

### Story 4 — Pause, resume, and restart preserve live controls

As an operator, I can pause or restart a Factory Session after a live change and
resume with exactly the last committed resource and workstation configuration.

Acceptance criteria:

- Accepted pause is published only after new admission is fenced and a durable,
  revision-bound recovery point exists.
- JavaScript cannot create a new child dispatch after accepted `PAUSED` state;
  completed mediated child results are replayed rather than reinvoked on resume.
- A resource or workstation change committed while paused is effective before
  resumed scheduling or automation begins.
- Restart reconstructs the latest Factory revision, resource capacity,
  workstation enabled state, and request-id replay result from durable records.
- Checkpoint source, arguments, policy, Factory revision, and event cursor are
  validated before JavaScript resume contacts a Worker or Provider.
- Corrupt, incomplete, or incompatible recovery facts produce a typed safe
  failure and do not silently revert to the authored baseline or start a second
  dispatch.

### Story 5 — Inspect and document the effective controls

As an operator, I can inspect the effective Factory revision and understand why
a live control was applied, rejected, or treated as a no-op.

Acceptance criteria:

- Session and Factory world reads expose the effective revision, resource ID,
  total/in-use/available capacity, workstation ID, and enabled state.
- The dashboard projects canonical API/event state and does not keep a second
  editable source of truth in graph component state.
- Existing graph nodes use the established visual/status primitives to show a
  disabled workstation and resource capacity; no cron-only control is rendered.
- `you docs` reference material documents ID targeting, the available-only
  reduction rule, capacity zero, no-op behavior, revision conflicts, and the
  guarantee that active dispatches are never terminated by these controls.
- CLI, REST, generated clients, MCP exposure if added, recordings, and UI use
  the same stable outcome and error vocabulary.

# Changes

## Package changes

- Add Factory Sessions root contracts for live Factory changes, typed outcomes,
  stable errors, request-id replay, and effective revision reads under
  `pkg/services/factory_sessions/`; implement private coordination under its
  `internal/` tree.
- Add an orchestration-neutral, session-scoped resource admission capability at
  the Factory Runtime root. Petri and JavaScript implementations consume that
  contract without exposing tokens, markings, Goja records, or local
  semaphores to Factory Sessions.
- Serialize resource acquisition and capacity mutation at the owning runtime
  service boundary; keep the comparison and error construction pure and
  directly unit-testable.
- Extend JavaScript agent/child configuration only as needed to declare resource
  ID requirements, and acquire leases at child-dispatch admission rather than
  once per `parallel()` invocation.
- Add workstation enabled-state reconciliation to Factory Runtime scheduling and
  Automations sidecar ownership. Automations remains responsible for starting
  and stopping cron, watcher, and poller sources.
- Extend Recordings canonical event contracts and reducers for request/failure
  events and revisioned `FACTORY_CHANGE` success while preserving full-snapshot
  replay.
- Extend Factory Visualization projections from canonical Factory world/session
  reads; graph rendering must not mutate runtime state directly.
- Add authored CLI manifest source, handlers, mappings, and HTTP client calls
  under `pkg/transports/cli`; regenerate CLI manifest output rather than editing
  `pkg/transports/cli/generated/` by hand.
- Compose new service dependencies once through owning `wire/` providers and
  `pkg/wire`; do not introduce a service locator, constructor bag, runtime
  lookup callback, or alternate process graph.

## Contracts

- Resource targets are `resourceId`; workstation targets are `workstationId`.
  Both resolve only the public `id` field.
- Add an effective Factory revision to live-change requests, outcomes, relevant
  event context/payloads, Factory Session reads, checkpoints, and dispatch
  admission facts.
- Add optional workstation `enabled` state to the effective Factory snapshot,
  defaulting to true. Preserve compatibility for authored factories that omit
  it.
- Keep authored resource capacity validation separate from the session runtime
  override rule if authored capacity must remain positive; live capacity zero
  has the explicit operational meaning described above.
- Add stable outcomes `APPLIED` and `NO_OP`, and stable errors including
  `RESOURCE_NOT_FOUND`, `RESOURCE_ID_REQUIRED`,
  `RESOURCE_CAPACITY_IN_USE`, `WORKSTATION_NOT_FOUND`,
  `WORKSTATION_ID_REQUIRED`, `REVISION_CONFLICT`,
  `SESSION_TRANSITION_IN_PROGRESS`, `TERMINAL_SESSION`, and
  `CHANGE_APPLICATION_FAILED`.
- A successful resource or workstation operation is represented by
  `FACTORY_CHANGE`; rejected and no-op requests do not masquerade as successful
  Factory changes.

## Services

- Factory Sessions owns command admission, lifecycle eligibility, effective
  revisions, idempotency, durable change coordination, and recovery ordering.
- Factory Runtime owns live resource acquisition/capacity application and
  workstation dispatch eligibility through orchestration-neutral contracts.
- Automations owns reconciliation of enabled workstation sources, including
  cron, watcher, poller, and invocation-schedule behavior.
- Recordings owns canonical request, success, failure, replay, and projection
  behavior.
- Factory Definitions owns authored ID validation and effective Factory snapshot
  shape, but does not mutate a live session.
- Transports and mapping packages own CLI/HTTP/MCP representations and do not
  choose capacity or workstation policy.

## API changes

- Add an idempotent capacity operation under
  `/factory-sessions/{session_id}/resources/{resource_id}/capacity` with
  `capacity`, `requestId`, `expectedRevision`, and optional `reason`.
- Add an idempotent workstation-state operation under
  `/factory-sessions/{session_id}/workstations/{workstation_id}/state` with
  `enabled`, `requestId`, `expectedRevision`, and optional `reason`.
- Return typed applied/no-op results with previous/current values, revision,
  request/change IDs, and canonical inspection links.
- Map malformed values to `400`, missing IDs/sessions to `404`, revision and
  in-use conflicts to `409`, terminal/transition-state rejection to the existing
  lifecycle-compatible conflict shape, and admitted application failure to a
  bounded `5xx` response correlated with `FACTORY_CHANGE_FAILED`.
- Author changes in `api/openapi-main.yaml` and `api/components/`, update HTTP
  handlers and mapping, and regenerate `api/openapi.yaml`, generated Go clients
  and servers, and `ui/src/api/generated/openapi.ts`. Do not hand-edit generated
  files.

## Tests

### Unit and package integration

- Table-test the capacity decision matrix, including increase, no-op, reduction
  to `inUse`, reduction below `inUse`, zero with and without use, negative
  capacity, overflow, stale revision, missing ID, and request-ID replay.
- Test that capacity mutation and dispatch acquisition share one serialization
  boundary and can never produce `inUse > capacity`.
- Test deterministic Petri idle-resource removal and replay without exposing
  token vocabulary in public results.
- Test JavaScript resource acquisition with waiting children and prove a live
  increase releases additional children in the same workflow execution.
- Test workstation eligibility for standard, logical, cron, watcher, and poller
  behaviors, including active-dispatch completion after disable.
- Test event reducers and durable snapshots for request/success/failure order,
  revision advancement, no-op/rejection absence, restart recovery, and
  checkpoint revision validation.
- Test CLI manifest parsing, ID-named help/usage, human output, JSON output,
  exit codes, request correlation, and HTTP error mapping.
- Add OpenAPI discriminator/union, generated-client, event-kind parity, and
  UI projection/operation tests when those surfaces change.

### Functional tests

Place new scenarios under behavior-owned packages rather than the deletion-only
`tests/functional/runtime_api` package:

- `tests/functional/resources/session_controls/` for active capacity increase,
  available-only reduction, in-use conflict, capacity zero, and Petri/JavaScript
  admission behavior;
- `tests/functional/workstations/controls/` for disable/enable, active dispatch
  non-termination, queued Work, and generic workstation ID behavior;
- extend `tests/functional/workstations/cron/` for fake-clock disable/re-enable
  boundaries through the workstation command rather than a cron-specific API;
- extend `tests/functional/events/factory_events/` for change ordering,
  correlation, revision, cursor reconnect, failure closure, and absence on
  rejection/no-op; and
- extend `tests/functional/sessions/restart/` and
  `tests/functional/transport/cli/session_resume/` for post-change restart and
  pause/change/resume reconstruction.

Required high-value functional cells:

1. Start with resource ID `reviewers` at capacity one, hold the first real
   provider-backed dispatch at an injected command-runner barrier, submit more
   Work, set capacity to two through the CLI, and observe a second dispatch
   begin without changing Factory Session identity or restarting the process.
2. Start capacity three with one dispatch in use, set capacity to one, and prove
   the active dispatch completes normally while total capacity becomes one.
3. Start capacity two with two dispatches in use, request capacity one, and
   prove the CLI returns `RESOURCE_CAPACITY_IN_USE`, both dispatches complete
   normally, no interrupt/terminate event exists, and the Factory revision and
   capacity remain unchanged.
4. Race a dispatch acquisition against a capacity reduction repeatedly and
   prove the only outcomes are “reduction commits and dispatch waits” or
   “dispatch acquires and reduction is rejected.”
5. Disable a standard workstation by ID while one dispatch is active, submit
   more eligible Work, prove the active dispatch completes and no new dispatch
   begins, then enable it and observe the queued Work dispatch.
6. Disable a cron workstation by ID, advance the injected clock, prove no new
   scheduled Work is admitted, enable it, advance to a later valid boundary,
   and prove exactly one trigger occurs.
7. Run a JavaScript workflow whose parallel children require resource ID
   `reviewers`, increase capacity while children wait, and prove throughput
   changes within the same durable Factory Session.
8. Commit resource and workstation changes, pause or interrupt at a durable
   JavaScript safe point, reconstruct with a fresh process, resume, and prove
   the latest revision is retained without repeating a completed child.

All functional application tests must follow the repository standards:

- construct through `root.BuildProcess` and execute through `Process.Execute`;
- invoke the public CLI for ordinary customer flows, using HTTP only for
  API-owned contract or explicit parity cells;
- replace external effects only through `edges.Edges`, preferring
  `ProviderCommandRunner` and command/process edge mocks;
- use mocked Codex or another real provider command shape instead of
  `--with-mock-workers` outside the workers/mock-owned test area;
- coordinate blocked and released dispatches with deterministic injected
  barriers, event cursors, fake clocks, or edge observations rather than sleeps
  or timeout-padded polling; and
- assert only public CLI, session, Work, Factory Event, generated API, or
  injected-edge observations, never internal engine snapshots.

### Stress and failure-mode evidence

- Add targeted race/repeat coverage for simultaneous capacity updates,
  resource acquisition/release, duplicate request IDs, workstation toggles,
  cron boundaries, pause, and restart.
- Inject persistence failure before and after request-event append and prove
  every admitted request eventually closes without a partially visible
  revision.
- Inject Automations reconciliation failure and prove the session reports a
  bounded failed/degraded outcome without silently enabling a disabled source.
- Track capacity-change latency, failures, conflicts, current utilization, and
  workstation reconciliation failures through structured metrics/logs suitable
  for saturation diagnosis.

## Quality gates

Use focused package tests during implementation, then run the applicable public
and generated-contract gates:

- `go test ./pkg/services/factory_sessions/... ./pkg/services/factory_runtime/... ./pkg/services/automations/... ./pkg/services/recordings/... ./pkg/transports/cli/...`
- targeted `go test -race` for the resource admission and workstation control
  packages
- focused functional packages through their `root.BuildProcess` +
  `Process.Execute` harnesses
- `make generate-api`
- `make interfaces-all` when publishable/generated contract shapes change
- `make api-smoke`
- `make docs-reference-smoke`
- `make ui-test` and `make ui-lint` when dashboard projections/actions change
- `make test-functional`
- `make test-stress` for the concurrency cells
- `make verify-fast`, followed by `make verify-pr`
- `make build-all` before delivery of the public CLI/API/client surface

## Out of scope

- A cron-specific enable/disable CLI or API.
- Name-based resource or workstation targeting.
- Deferred resource draining, pending lower-capacity requests, forced resource
  revocation, or cancellation/termination of dispatches to satisfy capacity.
- Treating JavaScript agents, worker names, labels, or task labels as authored
  workstation IDs.
- Live topology, Work type/state, relation, guard, worker/provider/model,
  JavaScript source/arguments, or orchestrator-kind replacement.
- Mutating completed sessions or historical replay projections.
- An unrelated graph-editor redesign; UI work is limited to projecting and
  invoking the canonical controls.

## Delivery boundary

The work is complete only after required CI is terminal and passing, all
blocking review feedback is explicitly addressed, merge conflicts are resolved,
generated artifacts are confirmed current, and the pull request is actually
merged. Opening a PR, pushing the implementation, obtaining approval, or
reaching green CI without merge is not completion.
