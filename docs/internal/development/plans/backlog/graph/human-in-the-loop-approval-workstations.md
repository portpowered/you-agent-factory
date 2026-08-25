# Human-in-the-Loop Approval Workstations Plan

## Status

Proposed.

## Problem statement

The Factory can be made to stop near a human-review state, but it cannot author,
observe, resolve, replay, and functionally prove a real human approval gate as
one first-class Workstation behavior.

## Customer ask

Make human-in-the-loop (HITL) loops a supported graph-Factory behavior by
providing:

- a workerless workstation/node that waits for an explicit human approval or
  rejection and then follows the authored route;
- CLI behavior for discovering and resolving pending approvals, including an
  attached `you run` interaction;
- canonical Factory Events that distinguish a request for human approval from
  the eventual decision; and
- high-value functional tests that prove approval, rejection/rework, event
  ordering, replay, cancellation, and CLI behavior through the customer process
  boundary.

## Current-state findings

The reported gap is confirmed.

1. **There is no human workstation type.** The authored `WorkstationType`
   contract currently contains inference, agent, script, poller, model,
   logical-move, and classifier variants, but no human gate. Existing taxonomy
   and worker-compatibility branches recognize only those current types; no
   runtime binding creates a pending human interaction.
2. **“Needs human” is a naming convention, not a domain fact.** Work states
   have only `INITIAL`, `PROCESSING`, `TERMINAL`, and `FAILED` categories.
   Invocation and Factory Session projections classify Work as needing a human
   by searching for the exact authored state name `needs-human`. A differently
   named review state has no equivalent meaning, and a state with that name has
   no typed approval request, allowed decision, actor, or decision history.
3. **The existing approval lifecycle is unrelated.**
   `POST /factory-sessions/{session_id}/approve`, `AWAITING_APPROVAL`, and
   `approvalPreviewId` approve durable JavaScript orchestrator policy. They do
   not approve Work at a graph workstation and must not be reused or renamed for
   HITL Work.
4. **The CLI has only a generic escape hatch.** `you work move <work-id>
   <state-name>` can relocate non-dispatched Work to any authored state, but it
   does not discover a pending approval, validate that the caller chose one of
   the gate's allowed decisions, require a rejection reason, or correlate the
   action to the gate that requested it. Because the CLI calls the HTTP move
   endpoint, production moves are also recorded as source `api`; the declared
   `cli` source is exercised only by lower-level tests.
5. **The canonical event stream cannot express the interaction.** The Factory
   event vocabulary has `WORK_STATE_CHANGE` for generic operator/cascade moves
   and dispatch events for Worker execution, but it has no “human approval
   requested” or “human approval resolved” event. `WorkStateChangeEventPayload`
   records only from/to state, source, optional trigger Work, and a free-form
   reason. It cannot prove which Workstation asked, which choices were offered,
   who decided, whether the result was approve/reject/cancel, or whether a retry
   was idempotent.
6. **`you work watch` is not HITL observation.** Its v1 reducer emits only
   `WORK_STATE_CHANGE` records. Ordinary workstation routing is represented by
   dispatch facts, and no pending-human record exists for the reducer to show.
7. **One-shot invocation stops instead of looping.** The invocation wait loop
   turns exact-name `needs-human` Work into terminal
   `INVOCATION_NEEDS_HUMAN`; `you run` then presents a failure and releases its
   owned Factory Session. There is no attached prompt or external-wait mode
   that keeps the invocation alive for a decision.
8. **Functional coverage proves adjacent behavior only.** The suite proves
   generic `you work move`, Work watch transitions, model-produced reviewer and
   classifier outcomes, and JavaScript policy lifecycle controls. It contains
   no functional scenario with a human workstation, pending approval read,
   human decision event, reject-then-approve loop, or replayed pending gate.
9. **Generated and frontend consumers will require deliberate compatibility
   work.** The dashboard editor, event projections, publishable client, replay
   package, and emulator enumerate current workstation and event types. Adding
   enum values without updating those consumers will create either compile
   failures or silent unsupported behavior.

## Solution

Add a graph-only, workerless `HUMAN_APPROVAL` Workstation type and a canonical
session-scoped Human Approval resource. Reaching the workstation creates a
durable pending approval instead of invoking Workers or Providers. Approving
produces the ordinary accepted route; rejecting produces the ordinary
`onRejection` route, allowing the same Work to return later and create a new
approval request.

This is not a fifth Work state category. Pending-human status is derived from a
canonical approval request associated with the Work and Workstation, not from
an authored state name. Existing factories that use a state literally named
`needs-human` retain their current diagnostic compatibility, but that convention
is documented as a recovery signal rather than supported HITL execution.

### Authored node contract

Use this public shape:

```json
{
  "id": "release-approval",
  "name": "release-approval",
  "description": {
    "type": "LOCALIZABLE_ASSET",
    "value": "Confirm that the release candidate may be published."
  },
  "behavior": "STANDARD",
  "type": "HUMAN_APPROVAL",
  "inputs": [{ "workType": "release", "state": "awaiting-approval" }],
  "outputs": [{ "workType": "release", "state": "approved" }],
  "onRejection": [{ "workType": "release", "state": "changes-requested" }]
}
```

Contract rules:

- `HUMAN_APPROVAL` is supported only by the graph/Petri orchestrator in this
  plan. JavaScript workflows do not acquire an implicit approval API.
- The node is workerless. `worker`, `runner`, provider/model operation fields,
  prompt files, output schemas/contracts, execution limits, environment,
  worktree, resources, `classificationRoutes`, `onContinue`, and `onFailure`
  are rejected rather than ignored.
- `description` is required and is the concise, localizable instruction shown
  to the operator. Work content and expected artifacts remain available through
  the existing Work read instead of being copied into the event payload.
- At least one `input`, one approval `output`, and one `onRejection` output are
  required. The node uses the existing topology checks for Work type/state
  compatibility and supported input arity.
- Approval preserves the consumed Work payload; rejection preserves it and adds
  no hidden Work tags. Decision reason and actor belong to the approval event,
  not to mutable payload metadata.
- Each firing creates a new approval ID. A rejection loop that returns the same
  Work to the node therefore creates a new request and retains the earlier
  decision in history.

### Runtime behavior

When a `HUMAN_APPROVAL` transition is selected:

1. Factory Runtime claims the input Work through the normal dispatch-admission
   boundary so another transition cannot consume it concurrently.
2. It records the ordinary `DISPATCH_REQUEST`, then records exactly one
   `HUMAN_APPROVAL_REQUESTED` event with a stable approval ID.
3. It leaves the dispatch pending without calling Workers, Worker Sessions,
   Providers, Models, scripts, or runner selection. Pending approval consumes no
   declared runtime resource capacity.
4. An accepted human-decision command records exactly one
   `HUMAN_APPROVAL_RESOLVED` event. The event re-enters Factory Runtime as the
   result of the pending workstation request.
5. `APPROVED` produces an accepted `DISPATCH_RESPONSE` and follows `outputs`;
   `REJECTED` produces a rejected `DISPATCH_RESPONSE` and follows
   `onRejection`. The existing transition/result pipeline remains the only code
   that applies output Work mutations.
6. Session cancellation or termination resolves an outstanding request as
   `CANCELED` and follows the existing dispatch interruption path; it does not
   synthesize approval or rejection.

The pending request projection is keyed by `approvalId` and joins the request
event to the effective Factory topology. It contains
`factorySessionId`, `dispatchId`, `workstationId`, `workstationName`, ordered
`workIds`, description, status, request event identity/sequence/time, and
available decisions. The resolved projection additionally contains outcome,
decision request ID, source, safe actor label when supplied, reason when
supplied, and resolution event identity/sequence/time.

Decision admission is serialized with dispatch cancellation and session
shutdown. Exactly one terminal outcome wins. Repeating the same request ID and
normalized decision returns the original result; reusing it with a different
decision or reason is a conflict. A late decision against an already resolved
approval returns the recorded terminal result for an exact replay and a typed
conflict otherwise.

### Canonical Factory Events

Add two event types rather than overloading `WORK_STATE_CHANGE` or the existing
session policy-approval event:

```text
DISPATCH_REQUEST
  -> HUMAN_APPROVAL_REQUESTED
  -> HUMAN_APPROVAL_RESOLVED
  -> DISPATCH_RESPONSE
```

`HUMAN_APPROVAL_REQUESTED` contains:

- `approvalId`, `dispatchId`, `workstationId`, `workstationName`;
- fixed available decisions `APPROVE` and `REJECT`; and
- request status `PENDING`.

The localized Workstation description is already part of the effective Factory
snapshot. Reads and presentation join it by stable Workstation ID instead of
copying mutable display text into the event.

Authoritative session, request, trace, sequence, time, and ordered Work IDs stay
in `FactoryEvent.context` where applicable.

`HUMAN_APPROVAL_RESOLVED` contains:

- `approvalId`, `dispatchId`, and outcome `APPROVED`, `REJECTED`, or `CANCELED`;
- decision source `cli`, `api`, or `attached-cli`;
- optional safe actor label and reason; and
- the original request event ID/sequence needed to audit the pair.

The events do not contain raw Work payloads, credentials, terminal input, or
provider data. Recordings validates pair ordering, rejects an orphan or second
conflicting resolution, and rebuilds pending/resolved approval projections from
history. Recording export/import, reconnect, historical reads, and dashboard
replay preserve the events unchanged.

### REST and CLI behavior

Add these session-scoped REST operations:

```text
GET  /factory-sessions/{session_id}/human-approvals
GET  /factory-sessions/{session_id}/human-approvals/{approval_id}
POST /factory-sessions/{session_id}/human-approvals/{approval_id}/decision
```

List defaults to pending requests and supports explicit status filtering. The
decision body contains `requestId`, `decision`, optional `actor`, and optional
`reason`. Reject requires a non-empty reason; approve permits one. Responses
return the canonical approval projection and links to the Factory Session,
Work, Workstation/current Factory, events, and the decision operation when
pending.

Add these manifest-authored CLI commands:

```text
you work approval list [--session <session-id>] [--status pending|resolved]
you work approval show <approval-id> [--session <session-id>]
you work approval approve <approval-id> [--reason <text>] [--request-id <id>]
you work approval reject <approval-id> --reason <text> [--request-id <id>]
```

Human output identifies the approval, Workstation, related Work, decision, and
next inspection command. `--json` returns the API-shaped contract. Diagnostics
remain on stderr and sensitive Work content is never echoed implicitly.
`you work move` remains a generic recovery operation; it does not resolve or
close a pending approval and cannot move Work claimed by that pending dispatch.

Add `you run --human-approval auto|prompt|external|fail`:

- `auto` is the default. It selects `prompt` only for human output with an
  interactive input terminal and when invocation input did not consume stdin;
  otherwise it selects `fail`.
- `prompt` renders pending requests in canonical sequence order to the
  diagnostic terminal, reads approve/reject and an optional/required reason,
  and submits the decision through the injected Work service. It rejects JSON,
  NDJSON/response-stream, quiet, non-interactive, and stdin-as-invocation-input
  combinations before runtime startup.
- `external` keeps the invocation waiting for REST/CLI decisions and requires
  `--with-server`; normal timeout and cancellation still apply.
- `fail` preserves non-interactive compatibility by returning
  `INVOCATION_NEEDS_HUMAN`, now with `approvalId`, Workstation, Work, and
  inspection/decision links derived from a real pending request.

The attached prompt uses a bounded queue outside the event-append call stack so
terminal input cannot block Recordings or the runtime tick. Multiple concurrent
requests are prompted one at a time by canonical sequence; external mode may
resolve independent requests concurrently.

The existing `you session approve` gap for JavaScript policy approval is not
part of this plan. HITL commands live under Work and use Human Approval IDs so
the two approval concepts remain unambiguous.

### Dashboard and package compatibility

The canonical editable state remains the generated `Factory`/`Workstation`
document. Existing current-selection workstation operations own type changes,
validation, save preparation, and server mutation. React Flow nodes and handles
remain disposable projections from that canonical document.

- Add `HUMAN_APPROVAL` to the localized workstation-type catalog and editable
  validation rules.
- Project one approval output handle and one rejection handle using the same
  semantic handle IDs consumed by saved edges.
- Render a distinct human-approval label/icon and pending/resolved status using
  existing shared node/status primitives; do not add a dashboard decision
  mutation in this first CLI-focused delivery.
- Project canonical approval events into timeline and current-activity state so
  a pending node remains visibly waiting and a resolved node shows its outcome.
- Teach `@you-agent-factory/factory-replay` the two event kinds.
- Make `@you-agent-factory/factory-emulator` report `HUMAN_APPROVAL` as an
  explicit unsupported external-interaction capability until an injected
  decision source is designed; it must not auto-approve or silently run it as a
  normal agent node.

## Original document

There is no earlier standalone product plan for this request. This plan records
the repository audit and is constrained by:

- `C:\Users\andre\work\portos\infinite-you\AGENTS.md`
- `C:\Users\andre\work\portos\infinite-you\docs\architecture\architecture.md`
- `C:\Users\andre\work\portos\infinite-you\docs\architecture\structures.md`
- `C:\Users\andre\work\portos\infinite-you\docs\architecture\data-model.md`
- `C:\Users\andre\work\portos\infinite-you\docs\architecture\packaged-structure.md`
- `factory/docs/standards/planning-standards.md`
- `C:\Users\andre\work\portos\infinite-you\docs\internal\standards\code\code-review-standards.md`
- `C:\Users\andre\work\portos\infinite-you\docs\internal\standards\code\general-backend-standards.md`
- `C:\Users\andre\work\portos\infinite-you\docs\internal\standards\code\general-website-standards.md`

## Work stories

### HITL-001: Author and validate a human approval node

As a Factory author, I can add a `HUMAN_APPROVAL` Workstation with explicit
approve and reject routes, so the Factory definition states where a human must
decide without binding a fake Worker.

Acceptance criteria:

- JSON and YAML Factory definitions accept `HUMAN_APPROVAL` and preserve it
  through load, validate, save, split/inline layout, export/import, and generated
  public schemas.
- Valid nodes require a stable ID, non-empty localized description, input,
  approval output, and rejection output.
- Worker, runner, provider/model, prompt, environment, worktree, resource,
  classifier, continue, and failure fields are rejected with field-specific
  diagnostics.
- The compiler produces a workerless runtime transition and never creates a
  Workers runtime binding for this node.
- The editor changes only the canonical Factory document through existing
  workstation draft/type/save operations; its React Flow node and approve/reject
  handles are reproducible projections with matching edge handle IDs.
- Type-change, validation, save-preparation, projection, localized-label, and
  focused component tests pass, including one non-default locale.
- `make generate-api` and `make interfaces-all` produce current Go,
  TypeScript, JSON Schema, and publishable package artifacts without hand edits.

### HITL-002: Observe a durable pending approval

As an operator, I can see exactly which Workstation and Work are waiting for my
decision, so I do not infer human work from a state name or a stalled queue.

Acceptance criteria:

- Selecting a human node claims its input once, records `DISPATCH_REQUEST`
  followed by exactly one `HUMAN_APPROVAL_REQUESTED`, and invokes no Worker,
  Worker Session, Provider, Model, script, or runner.
- The event has a stable approval ID, canonical session/dispatch/Work
  correlation, allowed decisions, and no copied Work payload; the read model
  resolves the localized description from the effective Factory snapshot.
- Pending approval keeps the Factory Session live and prevents the same Work
  from being consumed by another transition or moved through generic
  `you work move`.
- REST list/get and `you work approval list/show` return the same pending
  projection in human and JSON modes, including Work and event links.
- Work and Factory Session inspection identify the real pending approval and
  `INVOCATION_NEEDS_HUMAN` context uses it; legacy exact-name fallback remains
  distinguishable when no approval request exists.
- Recordings live and replay projections agree on pending state, and reconnect
  from before or after the request never duplicates it.
- Runtime admission, projection, REST mapping, CLI rendering, replay, and
  no-provider-call tests pass.

### HITL-003: Approve or reject through one idempotent decision contract

As an operator, I can approve or reject a pending request and have the Factory
follow only the corresponding authored route.

Acceptance criteria:

- REST and `you work approval approve/reject` call one Work-owned service-root
  operation with explicit session, approval, request, decision, actor, and
  reason values.
- Approve records `HUMAN_APPROVAL_RESOLVED(APPROVED)` before one accepted
  `DISPATCH_RESPONSE`, preserves input payload, and emits only `outputs`.
- Reject requires a reason, records
  `HUMAN_APPROVAL_RESOLVED(REJECTED)` before one rejected
  `DISPATCH_RESPONSE`, preserves input payload, and emits only `onRejection`.
- A rejection route that loops back eventually creates a new approval ID; a
  later approval completes normally while both decisions remain auditable.
- Same-body request-ID retry returns the original result without new events or
  routing. Changed-body reuse, unknown approval, wrong session, malformed
  decision, missing rejection reason, and competing resolution return typed,
  safe outcomes without mutation.
- Concurrent approve/reject attempts have one winner, one terminal dispatch
  result, and race/repeated-run coverage.
- Service operations log accepted intent and terminal outcome with safe stable
  identifiers and duration, without logging Work content or terminal input.

### HITL-004: Complete attached and externally operated CLI loops

As a CLI user, I can either answer a human gate in the `you run` terminal or
leave it open for a second CLI/API operator without losing stdout contracts.

Acceptance criteria:

- `--human-approval auto` prompts only in compatible interactive human-output
  runs and otherwise preserves the explicit fail behavior.
- Prompt mode renders requests in canonical sequence order, accepts only
  approve/reject, requires a rejection reason, retries invalid terminal input
  without mutating runtime state, and prints the final primary result only after
  the Factory reaches its authored result.
- Prompting never writes UI text into JSON/NDJSON stdout and all new visible
  copy follows the owning CLI message/catalog conventions.
- Unsupported combinations fail before opening a Factory Session or invoking a
  provider.
- External mode requires a bound server, remains cancellable, respects the
  invocation timeout, and completes after a separate approval CLI command.
- Fail mode returns `INVOCATION_NEEDS_HUMAN` with approval context and leaves no
  false success output.
- A bounded asynchronous prompt queue prevents event append/tick deadlock and
  handles cancellation while waiting for terminal input.
- Focused CLI unit/integration tests and root-built functional tests cover
  prompt approve, prompt reject-then-approve, external approval, headless fail,
  invalid input, timeout, and Ctrl-C cancellation.

### HITL-005: Preserve approval truth through cancellation, restart, and replay

As an operator or auditor, I can restart or replay a Factory Session and see the
same pending or resolved human decision without re-prompting completed work.

Acceptance criteria:

- Restart after `HUMAN_APPROVAL_REQUESTED` reconstructs one pending request and
  accepts a later decision without starting another dispatch.
- Restart after `HUMAN_APPROVAL_RESOLVED` but before or after
  `DISPATCH_RESPONSE` reconciles exactly one route/result and never asks the
  human twice.
- Session cancel/terminate wins atomically against a decision, records a
  canceled resolution plus ordinary dispatch interruption, and never emits an
  accepted/rejected route after cancellation wins.
- Recording validation rejects orphaned, mismatched, reordered, and conflicting
  approval event pairs with safe diagnostics.
- Export/import, historical tick inspection, replay packages, dashboard
  timeline/current-activity projections, and live reconnect retain the same
  request/decision correlation and outcome.
- The emulator fails compatibility inspection before event mutation and names
  human approval as an unsupported external interaction.
- Replay, recovery, race, malformed-recording, frontend projection, and package
  compatibility tests pass.

### HITL-006: Publish and functionally prove the supported contract

As a customer, I can follow one documented example and trust that the shipped
CLI, API, events, and runtime perform a real human approval loop.

Acceptance criteria:

- Add a maintained example Factory with a human approval node whose rejection
  route returns Work for changes and whose later approval reaches a terminal
  result.
- `you docs workstations`, `you docs work`, `you docs run`, and `you docs
  sessions` explain authoring, pending inspection, attached/external operation,
  event ordering, idempotency, cancellation, and the distinction from
  JavaScript policy approval and generic `work move`.
- New functional scenarios live under
  `tests/functional/workstations/human_approval/`, construct through
  `root.BuildProcess`, execute ordinary customer paths through
  `Process.Execute` and the CLI, and replace only exact external effects through
  `edges.Edges`.
- Functional proof covers no-worker request creation, list/show, approve to
  terminal, reject-then-approve loop, attached prompt, external decision,
  canonical event order/content, request-ID retry, competing decision,
  cancellation, and restart/replay.
- Tests use deterministic event observation and injected edges, contain no
  sleeps or timeout-padded polling as their synchronization strategy, and do not
  use `--with-mock-workers` outside the workers/mock feature cells.
- API smoke, docs smoke, generated-artifact checks, focused Go/UI/package
  tests, race/repeated-run checks, lint, and PR verification pass.

## Changes

### Package changes

- Extend authored Factory contracts under `api/components/schemas/data-models/`,
  `pkg/services/factory_definitions/internal/contracts/`, validation taxonomy,
  compilation/loading, layout, clone, and persistence paths for
  `HUMAN_APPROVAL`.
- Add a narrow Factory Runtime human-approval admission and resolution contract
  at `pkg/services/factory_runtime`; keep Petri claim, pending dispatch,
  idempotency, cancellation, and result re-entry under its existing private
  orchestration packages.
- Extend the Work service root with approval list/get/decision operations.
  Implement session resolution and Recordings-backed reads under
  `pkg/services/work/internal/`; do not expose engine state or call private
  Factory Runtime implementations.
- Extend Recordings event contracts, validation, append paths, reducers,
  projection clones, export/import, and replay under
  `pkg/services/recordings/`. Recordings remains the canonical Factory Event
  ledger; `pkg/services/events` is not used as approval storage.
- Update Factory Sessions invocation observation/wait policy so real pending
  approvals support fail, attached-prompt, and external-wait modes without
  weakening timeout, cancellation, or session cleanup.
- Add Work-owned HTTP adapters and CLI adapters; keep top-level HTTP/CLI code
  focused on composition and generated command wiring.
- Extend `pkg/services/factory_visualization` and frontend/package replay
  projections from canonical events. Do not create a second mutable approval
  store in UI state.
- Update the dashboard's existing current-selection workstation draft,
  validation, mutation/save, localized enum, and React Flow projection paths.
  Reuse shared node/status/form primitives and keep the graph library state
  disposable.
- Compose new public service dependencies once through owning `wire/` providers
  and `pkg/wire`; do not add a dependency bag, service locator, child injector,
  runtime callback registry, or out-of-`wire` production construction.

### Contracts

- Define `Human Approval` in `docs/architecture/data-model.md` as one
  pending-or-resolved operator decision for a Workstation dispatch within a
  Factory Session, linked to the affected Work. Keep JavaScript policy approval
  terminology explicitly separate.
- Add `HUMAN_APPROVAL` to `WorkstationType` and document graph-only support.
- Add request, status, decision, outcome, list/read, and decision-response
  schemas for the Human Approval resource.
- Add `HUMAN_APPROVAL_REQUESTED` and `HUMAN_APPROVAL_RESOLVED` to
  `FactoryEventType`, their payload union/discriminator mappings, canonical Go
  contracts, portable schemas, and published client types.
- Add optional pending-approval context and links to Work, Factory Session stop
  diagnostics, and `INVOCATION_NEEDS_HUMAN` without changing the meaning of
  JavaScript `AWAITING_APPROVAL`.
- Preserve `agent-factory.event.v1` only if the two additive enum/payload
  variants satisfy the repository's compatibility policy; otherwise introduce
  a deliberate event schema version and migration rather than silently
  changing a closed consumer contract.
- Keep decision source semantic (`cli`, `api`, `attached-cli`) and separate from
  transport-derived generic work-move source.
- Regenerate, never hand-edit, `api/openapi.yaml`, generated Go server/client,
  dashboard OpenAPI types, publishable API/client schemas, CLI manifest output,
  and MCP discovery artifacts that derive from the changed contract.

### Services

- **Factory Definitions** owns what constitutes a valid human workstation and
  how it compiles into a workerless graph transition.
- **Factory Runtime** owns selection, Work claim, pending execution state,
  decision admission against that state, cancellation races, and re-entry into
  ordinary transition routing.
- **Work** owns the customer-facing Human Approval resource and decision
  operation because the decision acts on Work held by a Workstation.
- **Recordings** owns canonical approval events, ordering validation, durable
  replay, and pending/resolved read projections.
- **Factory Sessions** owns invocation wait-mode behavior and session lifecycle
  coordination, not approval truth.
- **Workers/Worker Sessions/Providers/Models** receive no human-workstation
  dispatch and own no approval state.
- **Factory Visualization/UI packages** consume canonical contracts and events
  for presentation only.

### API changes

- Author the three Human Approval routes in `api/openapi-main.yaml` and their
  component parameters, schemas, responses, error outcomes, and examples under
  `api/components/`.
- Map handlers through the Work service contract and keep decision validation
  in the owning service rather than duplicating it in HTTP and CLI.
- Add typed errors for approval/session not found, already resolved,
  idempotency conflict, invalid decision, rejection reason required, session
  terminal, and decision/cancellation conflict.
- Update API contract tests, generated-client fixtures, schema inventories,
  public package manifests, and `make api-smoke` expectations.
- Do not reuse `/factory-sessions/{session_id}/approve` or its
  `FactorySessionApproveRequest`; that route remains JavaScript policy approval.

### Tests

- Factory Definition unit/contract tests for valid graph nodes, every forbidden
  field family, YAML/JSON parity, split/inline persistence, and compiler output.
- Factory Runtime package integration tests for one pending request, no Worker
  call, approve/reject routing, loop request identity, payload preservation,
  idempotency, cancellation race, and result/event order.
- Recordings tests for append validation, live/replay parity, pending/resolved
  projections, reconnect, export/import, malformed pairs, and event-kind parity.
- Work service and HTTP/CLI adapter tests for mapping, safe typed errors, human
  and JSON output, status filters, request IDs, and reason validation.
- Factory Sessions/CLI tests for invocation modes, stdout/stderr separation,
  TTY/input conflicts, prompt queue cancellation, timeout, and cleanup.
- Frontend operation, validation, projection, mutation, localization, component,
  replay-package, and explicit emulator-compatibility tests.
- Root-built functional scenarios under
  `tests/functional/workstations/human_approval/` as specified by HITL-006.
- Race or repeated-run coverage for competing approve/reject/cancel and replay
  reconciliation. Add stress coverage only if the implementation introduces a
  shared pending-approval queue or high-volume projection path.

## Dependency-aware delivery sequence

1. **HITL-001** establishes the authored type, strict validation, generated
   contract, compiler behavior, and editor projection used by every later
   story.
2. **HITL-002** establishes pending runtime truth, request events, projections,
   and read-only CLI/API inspection.
3. **HITL-003** adds the one idempotent decision operation and ordinary
   approve/reject routing.
4. **HITL-004** composes that operation into attached and external `you run`
   behavior after the decision contract is stable.
5. **HITL-005** hardens cancellation, restart, replay, dashboard projections,
   and package consumers against the established event sequence.
6. **HITL-006** publishes the example/docs and completes the cross-boundary
   functional proof before release.

HITL-001 editor projection work can proceed alongside backend compiler work
after the schema is fixed. HITL-005 frontend replay work can proceed alongside
backend recovery work after the event payloads and ordering invariants are
fixed. Decision mutation UI is intentionally deferred; it is independently
valuable and is not required to deliver the requested CLI-operated loop.

## Verification strategy

### Focused backend and contract

- Run focused Go tests for Factory Definitions, Factory Runtime, Work,
  Recordings, Factory Sessions, HTTP mapping, CLI adapters, and visualization.
- Run `make generate-api`, `make interfaces-all`, and `make api-smoke` for the
  public contract and generated consumers.
- Run event-kind inventory/parity, portable recording validation, package
  schema, CLI-manifest generation, and generated-artifact stability checks.
- Run focused `go test -race` or repeated execution for resolution/cancellation
  races and recovery idempotency.

### Functional

Functional tests construct through `root.BuildProcess` and execute through
`Process.Execute` by default. They enter through CLI for ordinary customer
flows, use REST only for API-owned parity assertions, and replace exact external
effects only through `edges.Edges`. They use canonical event subscriptions and
process readiness signals instead of sleeps.

Required functional cells:

- valid human node reaches one pending request with zero provider calls;
- list/show returns the same request observed on the Factory event stream;
- CLI approve reaches the authored terminal output;
- CLI reject returns Work for rework, creates a new approval ID, then approve
  completes;
- attached prompt completes without corrupting stdout;
- external mode completes after a second CLI invocation resolves the request;
- headless fail returns `INVOCATION_NEEDS_HUMAN` with no false success;
- exact request-ID retry produces no duplicate decision or route;
- competing decision/cancel produces one winner and one terminal dispatch fact;
- restart/replay reconstructs pending and resolved cases without a second
  prompt; and
- canonical event order and safe payload content match the public schemas.

### Frontend and packages

- Test canonical Factory draft operations and save preparation for type changes
  to/from `HUMAN_APPROVAL`.
- Test React Flow projection handle IDs and visible node state from canonical
  definition/event inputs.
- Test event reducers with pending, approved, rejected, canceled, replayed, and
  malformed sequences.
- Test localized type/status copy, keyboard-visible node detail behavior, and
  mobile/desktop presentation of pending state.
- Run package client/replay/emulator verification and confirm emulator rejection
  is explicit before mutation.

## Quality gates

Use the narrowest focused checks during each story, then run the shared/public
gates before merge:

```sh
make generate-api
make interfaces-all
make api-smoke
make docs-reference-smoke
make ui-test
make ui-lint
make verify-fast
make lint
make verify-pr
```

Also run focused race/repeated-run commands for decision/cancellation and the
targeted functional package under
`tests/functional/workstations/human_approval/`. Visually inspect the editor and
running pending/resolved node at mobile and desktop sizes when the dashboard
projection changes.

## Project acceptance criteria

- A Factory author can create a workerless `HUMAN_APPROVAL` node with explicit
  approve and reject routes, and invalid execution fields fail validation.
- Reaching the node creates one durable pending Human Approval with no Worker or
  Provider execution.
- CLI/API inspection and decision operations use one canonical approval ID and
  one Work-owned contract.
- Approval and rejection follow only their authored routes; rejection can loop
  back and later approval can complete with distinct auditable request IDs.
- Factory Events explicitly and safely record request and resolution in stable
  order, and Recordings replay reconstructs pending/resolved truth.
- `you run` supports attached prompt, external wait, and compatibility fail
  modes without corrupting stdout, leaking sensitive content, or stranding
  session lifecycle.
- Generic `you work move` and JavaScript policy approval remain distinct and
  cannot silently resolve a Human Approval.
- Dashboard/editor and publishable package consumers handle the new type/events
  deliberately; the emulator fails explicitly rather than auto-approving.
- Required unit, integration, contract, functional, replay, race, frontend,
  package, docs, generation, lint, and CI checks pass.
- Delivery for every implementation PR continues until required CI is terminal
  and passing, all blocking review feedback is explicitly addressed, conflicts
  and shared-file/generated-artifact drift are resolved, and the PR is actually
  merged. An opened, approved, or green-but-unmerged PR is not complete.

## Delivery loop

Each work story should normally be one independently reviewable PR. For every
PR, continue implementation and review until required CI is terminal and green,
blocking review conversations are addressed, conflicts are resolved, generated
artifacts are current, and the PR is merged. Follow-up work may be split out
only when it is independently valuable and does not weaken the current story's
observable acceptance criteria.
