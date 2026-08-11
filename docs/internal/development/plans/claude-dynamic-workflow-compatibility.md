# Claude Dynamic Workflow Compatibility Plan

## Problem statement

You Agent Factory already provides a durable JavaScript Factory runtime, but its
authoring API and several execution semantics differ from the Claude Code
`Workflow` contract closely enough that customers must learn two script APIs
and an unmodified Claude workflow can execute with different concurrency or
resume the wrong child results.

## Customer ask

Customers should be able to understand the Claude dynamic-workflow schema,
compare it with You's shipped behavior, and eventually run Claude-compatible
workflow sources through You without giving up canonical `FactorySession`,
`Dispatch`, `FactoryArtifact`, and `FactoryEvent` behavior.

## Solution

Converge the supported You JavaScript surface on the Claude workflow script
contract. Claude names, signatures, callback behavior, return values, and
failure semantics become the canonical base. You-specific provider selection,
presets, permissions, artifacts, checkpoints, TypeScript loading, structured
log fields, and durable inspection remain additive extensions. Existing
`agent.run` and object-form fan-out receive a bounded migration window rather
than defining a permanent second dialect.

Publish this as two related contract descriptors: `claude-workflow-v1` is the
unchanged Claude base, and `you-workflow-v1` is a strict additive superset.
Every field shared by the two descriptors has exactly the same validation,
execution, return, and failure semantics. A source does not select a runtime
mode: using a You-only field simply requires validation against the superset.

All execution continues to create and return a `FactorySession`; Claude terms
such as run and agent remain authoring aliases rather than new canonical
resources. The top-level Claude Code tool payload is documented and mapped to
the existing Factory Session source selectors, but exact flat-payload parity is
not required to converge the JavaScript language itself.

The work is intentionally ordered around observable behavior:

1. Freeze the canonical JavaScript contract and executable fixtures.
2. Parse Claude metadata and map existing Factory Session source selectors.
3. Make child scheduling genuinely asynchronous before promising Claude
   `parallel` or `pipeline` behavior.
4. Add the Claude `agent()` signature and its structured-output, phase,
   isolation, and agent-type behavior.
5. Add deterministic prefix resume and one-level nested workflows on the shared
   scheduler.
6. Project the resulting progress and compatibility facts consistently through
   REST, CLI, MCP, recordings, and the existing Factory Session dashboard.

## Original document

The supplied compatibility reference is
`C:\Users\andre\claude-dynamic-workflows.md`. A customer-readable snapshot is
packaged at `docs/reference/claude-dynamic-workflows.md`. The shipped You
contract remains `docs/reference/javascript-workflows.md`.

## Current-state comparison

This inventory was taken from the working tree on 2026-08-10. The tree already
contains uncommitted `skipPermissions` propagation and documentation changes;
those changes are treated as observed in-progress behavior, not as merged
baseline. The unrelated unresolved conflict in
`docs/internal/baselines/backend-exemption-budget.json` is outside this plan.

### What is already reusable

- Source selection already supports inline source, explicit workflow files,
  named workflows, `.claude/workflows/`, user workflows, package-relative
  workflows, named JavaScript Factories, and built-ins.
- JavaScript runs already use canonical durable Factory Sessions with sync and
  async starts, session status/results, dispatches, artifacts, events,
  lifecycle controls, recording, replay, and restart recovery.
- The Goja runtime already binds `args`, host-resolved `meta`, `phase`, `log`,
  `workflow.log`, `agent.run`, `parallel`, `pipeline`, `workflow.artifact`,
  `workflow.checkpoint`, `workflow.resumeState`, `workflow.budget`, and
  `workflow.final`.
- Child execution already crosses Factory Runtime, Factory Sessions, Workers,
  Providers, Provider Sessions, and Recordings instead of calling provider
  processes directly from the VM.
- The child request and provider bridge already carry dormant output-schema,
  sandbox, writable-root, and related values internally, although the public
  JavaScript child contract currently rejects most of them.
- Effective policy already provides `maxAgents`, concurrency, duration, output,
  artifact, model, effort, runner, and token-budget fields. The default is
  read-only, 16 agents, and concurrency 4, with an absolute deployment planning
  cap of 1000 agents.
- The dashboard and generated API already understand JavaScript phase,
  checkpoint, dispatch-count, artifact, and provider-session projections.

### Compatibility matrix

| Area | Claude `Workflow` contract | You working-tree behavior | Gap or decision |
| --- | --- | --- | --- |
| Start payload | Flat `script`, `scriptPath`, or `name`; optional JSON `args`, `resumeFromRunId`, ignored `title`/`description` | `FactorySessionExecutionRequest` uses `requestId` plus a nested source selector and object-shaped `args`; sync and async routes are explicit | Keep the canonical Factory Session request and document the direct selector mapping. Widen `args` to any JSON value. Consider an exact flat transport alias only after script convergence is proven. |
| Background lifecycle | Always returns a run id immediately; completion is notified later | Async start returns a `sessionId`; clients poll or consume durable events. Sync start is also available | Async start is already the canonical equivalent. Let host adapters bridge terminal Factory Events into notifications; a Claude-shaped run-id alias is a deferred transport convenience. |
| Source persistence | Every invocation persists source and returns an editable path | File sources remain files; durable sessions retain source identity/hash and artifacts, but inline source is not promised as an editable workflow path | Persist an internal source snapshot for every JavaScript run and optionally materialize a reusable workflow through an explicit save operation; never expose unrestricted internal paths. |
| Metadata | Required pure `export const meta = {name, description, whenToUse?, phases?}` | `meta` is injected from request/factory metadata; `export` is rejected; packaged JavaScript uses a separate `@you-factory-meta` header | Parse and validate the pure Claude literal, remove the export from executable source, and merge it into resolved Factory metadata. Preserve the You header for native factories. |
| Language | Plain JavaScript; TypeScript is rejected | JavaScript plus a bounded TypeScript stripping loader | JavaScript follows the canonical Claude contract. Keep bounded TypeScript as an explicitly documented You extension with the same runtime host API. |
| Determinism | `Date.now`, `Math.random`, and argument-less `new Date()` are unavailable | `Date` is not on the allowed-global list, but `Math` is and `Math.random` is not specifically rejected | Add static and runtime member-level denial for nondeterministic built-ins across the canonical JavaScript surface and test computed-member bypasses. |
| `agent` call | `agent(prompt, opts?)` | `agent.run({prompt, ...})` | Make `agent` a callable function object and deprecate `.run` after checked-in sources migrate. Both paths normalize into one child request during the migration window. |
| Agent options | `label`, `phase`, `schema`, `model`, `effort`, `isolation: "worktree"`, `agentType` | `label`, `preset`, `executorProvider`, `modelProvider`, `model`, `reasoningEffort`, and in-progress `skipPermissions`; output schema and access values are internal only | Give the Claude fields their canonical meanings. Route schema through existing structured-output capability, isolation through Workers, and agent type through the configured Factory-agent/Worker-profile registry. Add the provider/preset/permission fields only in `you-workflow-v1`. |
| Agent result | Text without schema, validated object with schema, `null` for skipped/terminal child failure | Structured envelope containing status, dispatch identity, output, and diagnostics; a direct child failure currently throws | Make text/object/null the canonical `agent()` result. Durable identity stays on Dispatch reads. Keep `agent.run` temporarily as a deprecated envelope-returning extension for migration. |
| `parallel` | Concurrent thunks, hard barrier, per-thunk failure becomes `null`, whole call does not reject | Object child specs run concurrently, but function items are invoked serially; failure shapes differ | Replace blocking host execution with a VM event loop and scheduler. Thunks and Claude failure behavior become canonical; object child specs remain a You convenience extension. |
| `pipeline` | Any number of stages; every item advances independently without cross-item barriers; later callbacks receive `(prev, item, index)` | One required and one optional stage; items run serially; results are per-stage status envelopes | Implement arbitrary stages and concurrent per-item chains with Claude result/failure behavior. Remove the status-envelope return from the canonical path after migrating checked-in workflows and fixtures. |
| Phases and logs | Declared phases, runtime `phase`, per-agent `opts.phase`, labels, and narrator `log` | Runtime phases/logs exist and become ordered records; no parsed declared phases or explicit per-child phase | Persist declared phase summaries before execution and put explicit phase on the child request/dispatch so concurrent calls never race on global phase state. |
| Nested workflows | `workflow(nameOrRef, args?)`, one nesting level, shared cap/budget/abort | `workflow` is a non-callable namespace for final/log/artifact/checkpoint/resume/budget | Make the namespace callable, keep its methods, resolve children through Factory Definitions/Runtime source services, and share the same scheduler, ledger, records, and cancellation context. |
| Budget | Global `budget.total`, `spent()`, `remaining()` with hard stop for new agents | `workflow.budget()` returns a static policy snapshot; `maxTokens` is exposed but not a live global ledger | Defer live token accounting. Keep existing static policy budgets and hard agent/concurrency/duration caps; report the Claude `budget` global as unsupported until usage accounting is truthful. |
| Resume | `resumeFromRunId` reuses the longest successful prefix whose authored `(prompt, opts)` calls match; a mismatch invalidates the suffix | Interrupted sessions replay completed child results by ordinal `dispatch-N`; call arguments are not compared. Checkpoint state is separately explicit | Persist an authored child-call signature and enforce prefix invalidation. Keep interrupted same-session resume and add explicit continuation lineage for completed or edited sessions. |
| Caps | Concurrency `min(16, cores-2)`, 4096 items per `parallel`/`pipeline`, 1000 agents lifetime | Policy defaults to concurrency 4 and max agents 16; deployment cap defaults to 1000; there is no independent 4096-item guard | Add the 4096 item and 1000 lifetime guards. Treat You's effective Factory/session/provider policy as an allowed narrowing extension; decide separately whether the default concurrency should rise from 4. |
| Files and MCP | Script has no host IO; children may use their allowed tools/MCP; worktree isolation is explicit | Script has no host IO; provider-backed children use Worker/Provider capabilities; access remains policy/provider dependent | Existing boundary is correct. Capability mismatch must fail before dispatch and docs must not promise every provider has the same tools. |
| Streaming | No partial result in the caller; progress UI/logs only | Async sessions expose richer durable event and response-event streams plus polling | Treat You's event streams as a compatible superset. Do not inject partial child outputs into the caller result. |
| Multimodal args | No native blob/image/audio parameter; agents may read referenced paths | Start `args` is object-shaped; child prompts can reference files subject to worker capabilities | Preserve indirect file-path behavior. Do not introduce binary VM globals. Widen invocation args to any JSON value without weakening source-owned args-schema validation. |
| Explicit opt-in | Workflow invocation requires explicit user/session opt-in | A customer explicitly selects and starts a Factory/Factory Session | Already satisfied; no automatic workflow selection is added by this plan. |

## Scope estimate

The authored-source migration is modest. The current tree has 30 checked-in
workflow sources across packaged factories, customer examples, and runtime
fixtures: 40 `agent.run` calls, 11 `parallel` calls, and 3 `pipeline` calls.
Only three first-party packaged Factories use JavaScript. Those call sites can
be migrated mechanically once canonical return shapes are decided.

The runtime behavior is the larger part. The JavaScript runtime/validation/source
area contains about 40 Go files, and the focused functional orchestration lane
contains 19 test files. Not every file changes, but concurrency, child results,
resume, recordings, and projections cross several durable owners.

| Lane | Relative size | Rough implementation effort | Why |
| --- | --- | --- | --- |
| Canonical contract, `meta`, JSON `args`, validation, and source migrations | Medium | 3–5 focused engineering days | Parser/descriptor work plus 30 small source migrations and contract fixtures. |
| Non-blocking Goja scheduler, real `parallel`, and streaming per-item `pipeline` | Large | 6–10 days | The VM must remain single-threaded while child work and promise completions are concurrent, canceled, ordered, and race-tested. |
| Canonical `agent()` results, schema support, and You provider extensions | Large | 5–8 days | Crosses Factory Runtime, Factory Sessions, Workers, Providers, capability checks, recordings, and failure translation. |
| Call-signature resume safety | Medium–large | 4–7 days | Requires durable record versioning, prefix invalidation, restart/replay tests, and separation from checkpoint resume. |
| One-level callable nested workflows | Medium | 3–5 days | Reuses source resolution and scheduler state but needs nested scope records, errors, and progress projection. |
| Cross-surface contracts, docs, generated artifacts, and dashboard projection | Medium | 4–6 days | REST/MCP/CLI parity, generated clients, reference docs, and existing Factory Session UI tests. |

The rough total is 25–40 focused engineering days, preferably delivered as
five or six independently reviewable PRs. The first three lanes—enough for most
Claude-authored fan-out, pipelines, phases, and structured output—are roughly
14–23 days. Resume and nesting can follow without changing the canonical
surface. Deferring live token-budget accounting avoids a separate provider-
usage and aggregation program that would likely add at least one or two more
cross-service PRs.

## Compatibility decisions

### One canonical script contract with You extensions

- Publish `claude-workflow-v1` as the frozen base descriptor and
  `you-workflow-v1` as `claude-workflow-v1` plus a closed `x-you` extension
  section. These are documentation, validation, and capability-negotiation
  contracts, not separate interpreters.
- A workflow containing only Claude fields must satisfy
  `claude-workflow-v1` unchanged. A workflow using any You-only field satisfies
  `you-workflow-v1`; the overlapping surface is byte-for-byte the same authored
  shape and has one implementation path.
- Claude's workflow JavaScript surface is the base contract: pure exported
  `meta`, callable `agent`, thunk-based `parallel`, streaming per-item
  `pipeline`, `phase`, `log`, callable `workflow`, JSON `args`, and Claude
  value/failure semantics.
- You extensions add fields rather than fork behavior. The first extension set
  is `preset`, `executorProvider`, `modelProvider`, `reasoningEffort`, and
  `skipPermissions` on agent options, plus `workflow.artifact`,
  `workflow.checkpoint`, `workflow.resumeState`, `workflow.final`, structured
  log fields, and bounded TypeScript loading.
- Prefer Claude option names when both exist. `effort` is canonical;
  `reasoningEffort` is a You extension alias. `model` and `schema` are shared.
- `agent.run(spec)` and object entries in `parallel` remain temporary migration
  aliases for checked-in You workflows. New examples and packaged factories
  use callable `agent` and Claude result shapes.
- Preview reports unsupported fields and deprecated aliases before execution.
  Provider adapters receive only normalized provider-neutral child requests and
  never learn Claude- versus You-authored field names.

The initial canonical option object is therefore:

```javascript
await agent("Review the proposed change", {
  // Claude base contract
  label: "review",
  phase: "verify",
  schema: REVIEW_SCHEMA,
  model: "gpt-5.6",
  effort: "high",
  isolation: "worktree",
  agentType: "code-reviewer",

  // You extensions
  preset: "careful-review",
  executorProvider: "ACP",
  modelProvider: "cursor-acp",
  reasoningEffort: "high",
  skipPermissions: false,
});
```

- Claude fields keep Claude meanings and return behavior regardless of whether
  You extensions are present.
- `reasoningEffort` is a migration alias for existing You sources. New
  sources use `effort`; supplying both with different values is a validation
  error.
- Explicit call fields override the selected `agentType`/`preset`, which override
  operator/session defaults. Effective policy and provider capability checks
  still narrow the result.
- Unknown extension fields fail closed. Future provider controls are added to
  this closed extension set only after their provider-neutral semantics exist.

### Canonical execution and invocation

- REST and MCP keep the existing Factory Session start operations and canonical
  request. `INLINE_WORKFLOW`, `WORKFLOW_FILE`, and `WORKFLOW_NAME` map directly
  to Claude's `script`, `scriptPath`, and `name` concepts.
- Widen invocation `args` from object-only to any JSON value while retaining
  source-owned JSON Schema validation. Existing object-shaped factory inputs
  remain backward compatible.
- Keep `requestId`, explicit sync/async choice, requested policy, and canonical
  `sessionId` as You Factory extensions. They provide stronger idempotency and
  durable lifecycle behavior without changing script semantics.
- Defer an exact flat Claude Code tool-payload alias and `wf_...` response alias
  until the canonical script fixtures run unchanged. Those are transport
  conveniences and should not block the runtime convergence.
- Host-specific fire-and-forget notifications remain transport behavior derived
  from terminal Factory Events, not state owned by Factory Runtime.

### Scheduler and Goja event loop

The current `hostAgentRun` blocks until child execution completes and then
returns an already-resolved promise. That must change before any concurrency
parity claim.

- A session-scoped scheduler owns lexical call indexes, queue state,
  concurrency permits, item/lifetime limits, and abort propagation.
- The Goja VM stays on one goroutine. Host calls enqueue work and return pending
  promises. Worker goroutines execute injected child operations and send
  completions to the VM event loop, which alone resolves promises and resumes
  JavaScript callbacks.
- Calling `parallel` thunks schedules each chain without awaiting the prior
  thunk. Calling `pipeline` creates one independent chain per item. Ordered
  result arrays use lexical item order, never completion order.
- Queued, running, completed, skipped, and failed child states append canonical
  runtime records and Factory Events as they change.
- No package introduces a secondary dependency graph or service locator. The
  scheduler is request-scoped state created by the injected Factory Runtime
  implementation and calls already-injected Factory Sessions/Workers ports.

### Child calls and structured output

- The callable `agent(prompt, opts)` is canonical. Deprecated `agent.run(spec)`
  shares its normalization and policy decision path during migration.
- `opts.schema` is validated as JSON Schema during preview when literal and at
  runtime when computed. It is serialized into the existing Worker/Provider
  output-schema contract and its digest is recorded on the Dispatch.
- Providers declare structured-output capability. Unsupported routes fail
  before provider execution; no adapter silently downgrades to unvalidated
  text.
- The completed output is validated again at the Worker boundary. Canonical
  `agent` receives text, a validated object, or `null`. Deprecated `agent.run`
  may retain its durable envelope until checked-in consumers migrate.
- `opts.phase` is copied onto the child record and wins over the current global
  phase for that child only.
- `opts.isolation: "worktree"` becomes a provider-neutral Worker isolation
  request. Workers owns creation/cleanup and Providers owns truthful capability
  mapping. Unsupported routes fail before execution.
- `opts.agentType` resolves through an explicit Factory-agent or operator Worker
  profile contract. Unknown types fail safely. It is not treated as a raw
  provider prompt injection.
- `opts.model` and `opts.effort` map to the existing model and reasoning-effort
  selectors; omission inherits resolved session/worker settings.

### Resume and replay safety

- Every admitted child call stores a canonical authored-call digest covering
  prompt plus normalized options, including schema and phase.
  Raw prompts remain protected by existing artifact/audit policy.
- Lexical call identity is allocated when work is scheduled, including within
  parallel and pipeline chains.
- Resume compares successful calls in order. Reuse stops permanently at the
  first missing or mismatched call; all later calls execute live even if a
  later digest happens to match.
- Cached `null`, skipped, failed, canceled, or terminal-error calls are not
  treated as successful prefix entries.
- A continuation from a completed or edited run creates a new
  Factory Session with explicit `resumedFromSessionId` lineage. Existing
  interrupted-session resume remains same-session lifecycle recovery.
- Checkpoint state remains an explicit You extension and is not confused with
  Claude prefix caching.

### Budget deferral

- Do not add Claude's live `budget` global in the initial convergence. Current
  provider usage is not yet complete enough to make `spent()` and `remaining()`
  truthful across every route.
- Retain `workflow.budget()` as a You extension that returns static effective
  policy limits. Document that it is not Claude's live token ledger.
- Enforce existing agent count, concurrency, duration, output-size, artifact,
  model, and provider limits independently of live token accounting.
- Add the live global later as its own vertically sliced project after provider
  usage capture and session aggregation are authoritative. No placeholder
  `spent() = 0` implementation is acceptable.

## Work stories

### Story 1 — Publish and lock the canonical JavaScript baseline

As a customer evaluating migration, I can read the Claude schema snapshot next
to the shipped You contract and tell which document describes which product.

Acceptance criteria:

- `you docs claude-dynamic-workflows` renders the supplied compatibility
  snapshot with its snapshot date and an explicit link to the shipped You
  contract.
- `you docs javascript-workflows` remains the authoritative description of
  currently supported You behavior and never implies that planned compatibility
  fields have shipped.
- A machine-readable manifest identifies every Claude-base global, function,
  option, cap, failure rule, and return shape, then lists You extensions
  separately as the versioned `claude-workflow-v1` and `you-workflow-v1`
  descriptors.
- Golden Claude-style workflow fixtures cover fan-out, pipeline, structured
  output, phase metadata, nested workflow, and resume. A negative fixture makes
  the deferred live `budget` global explicit.
- Contract tests fail when the reference matrix, runtime descriptor, fixtures,
  and public docs disagree.

### Story 2 — Preview and run Claude-authored source

As a workflow author, I can validate and start a Claude-style JavaScript file
without rewriting its metadata declaration or source selector.

Acceptance criteria:

- Existing inline, file, and named source selectors accept Claude-authored
  source and apply a 524,288-character inline-source limit.
- A pure `export const meta` literal with required one-line name/description and
  optional `whenToUse`/phases is extracted, validated, and projected before a
  session starts.
- Variables, calls, spreads, computed values, malformed phases, and phase/title
  mismatches produce stable source diagnostics with authored line locations.
- Nondeterministic time/random APIs are rejected before execution, including
  computed member access that can be resolved statically. TypeScript remains an
  explicitly documented You loader extension.
- Start returns canonical Factory Session identity; async progress is readable
  through existing session/event surfaces.
- Inline source is durably snapshotted without exposing unrestricted host paths
  or raw source on ordinary public reads.

### Story 3 — Execute real concurrent barriers and pipelines

As a workflow author, I observe Claude-compatible concurrency rather than
serial execution hidden behind promises.

Acceptance criteria:

- `parallel([thunkA, thunkB])` admits both child chains before either provider
  result is required and waits for all results before continuing.
- One rejected/failed thunk yields `null` at its original index and does not
  reject the canonical `parallel` call.
- `pipeline(items, ...stages)` supports one or more stages, starts independent
  item chains up to the effective concurrency cap, and lets one item enter a
  later stage while another remains in an earlier stage.
- Every stage receives `(previousResult, originalItem, index)`; the first stage
  receives `undefined` as its previous result.
- A failed stage short-circuits only that item to `null`; other items continue.
- The 4096 item cap, 1000 agent lifetime cap, effective concurrency cap,
  cancellation, and result ordering are deterministic under race and repeat-run
  tests.
- Object-form `parallel` remains a documented You extension. Checked-in callers
  of the old pipeline status envelope are migrated to canonical raw/null results
  before that envelope is removed.

### Story 4 — Run canonical child agents with You extensions

As a workflow author, I can use the documented Claude agent options and receive
the documented value shape while operators retain durable child inspection.

Acceptance criteria:

- `agent(prompt, opts)` accepts the Claude base fields plus the closed You
  extension fields and maps them through one shared child request.
- Without schema, a successful canonical child resolves to final text.
  With schema, it resolves to the validated JSON object.
- Skipped or terminally failed children resolve to `null`, while
  the associated Dispatch and safe failure detail remain inspectable.
- Deprecated `agent.run(spec)` remains only for a documented migration window;
  first-party packaged Factories, examples, and fixtures move to `agent()`.
- Literal and computed schemas receive equivalent validation, capability
  negotiation, provider transport, output validation, digest, event, recording,
  and replay behavior.
- Explicit phase, model, effort, worktree isolation, and agent type are applied
  to only that child and remain visible in safe dispatch projections.
- Unsupported provider capability combinations fail before the provider starts;
  no route silently drops schema, isolation, agent type, or access requirements.

### Story 5 — Resume the longest unchanged successful prefix

As a workflow author iterating on a later stage, I can continue from prior
successful child work without replaying stale results after a changed call.

Acceptance criteria:

- A continuation with identical source-relevant args and child call signatures
  reuses every successful prior call without invoking a provider.
- The first changed prompt or option executes live and disables reuse for the
  entire suffix.
- Concurrent calls use stable lexical identities independent of completion
  order.
- Prior failed, skipped, canceled, null, or incomplete calls execute live.
- Resume lineage, cache hit/miss decision, call digest, source hash, args hash,
  and reused Dispatch identity are durable and replayable without exposing raw
  prompts or secrets.
- Existing interrupted checkpoint resume still restores approved checkpoint
  state and cannot accidentally use the new continuation path.

### Story 6 — Compose one nested workflow within shared limits

As a workflow author, I can compose one saved workflow without escaping parent
limits.

Acceptance criteria:

- `workflow(nameOrRef, args)` returns the child workflow's value, uses normal
  source lookup, and renders a nested progress group within the same parent
  Factory Session.
- The child shares concurrency, agent count, duration/output caps, abort signal,
  effective policy, durable records, and artifact controls with its parent.
- A child attempting another `workflow()` call fails with a stable nesting-depth
  diagnostic. Unknown names, unreadable paths, and syntax errors are catchable
  JavaScript errors.
- You extensions `workflow.log`, `workflow.artifact`, `workflow.checkpoint`,
  `workflow.resumeState`, `workflow.budget`, and `workflow.final` remain callable
  properties on the canonical function object.
- The Claude live `budget` global remains rejected with a stable unsupported-
  global diagnostic until the deferred accounting project ships.

### Story 7 — Inspect workflow progress everywhere

As an operator, I can explain a workflow run through the same Factory Session
surfaces used for native workflows.

Acceptance criteria:

- Declared phases exist in the initial read model; observed phase changes,
  explicit child phase, label, queue state, nested group, usage, and terminal
  outcome are derived from canonical Factory Events and durable records.
- REST, CLI, MCP, recordings/replay, and the dashboard agree on session,
  dispatch, phase, usage, resume-lineage, and artifact facts.
- The Factory Session detail feature treats API/session data as canonical and
  builds the progress tree as a projection; component state does not become a
  second execution source of truth.
- Existing shared status, disclosure, table/tree, tooltip, and action primitives
  are reused; new copy lives in the feature message catalog and all controls are
  keyboard accessible.
- Mobile and desktop layouts can inspect phases and nested groups without
  horizontal page scrolling, and high-volume child lists use bounded rendering.
- The workflow view does not expose raw prompts, credentials, unrestricted
  paths, provider payloads, or checkpoint bodies.

## Changes

### Package changes

- Extend source metadata extraction and canonical JavaScript validation under
  `pkg/services/factory_runtime/internal/services/orchestration/javascript/source/`
  and `validation/`.
- Add Claude-base call normalization, You-extension normalization, deprecation
  handling, and manifest generation under the
  JavaScript orchestration owner; keep the authored descriptor as the canonical
  source for generated `contracts/javascript/runtime-api.json` artifacts.
- Refactor `javascript/runtime` around a request-scoped scheduler/event loop.
  Keep pure call normalization, limit decisions, signature hashing, and result
  translation in small focused modules.
- Extend JavaScript runtime child records with authored-call digest, explicit
  phase, nested scope, and resume decision fields. Update strict
  record decoding and migrations; older records must remain readable or fail
  with an explicit incompatible-version diagnostic.
- Extend Factory Sessions execution/resume composition to create continuation
  lineage and persist runtime records before exposing projections.
- Extend Workers with provider-neutral structured-output and worktree-isolation
  requirements only where existing public Worker contracts do not already carry
  them. Providers remains the owner of capability truth and native mapping.
- Use Factory Definitions' JavaScript agent map and operator worker presets as
  explicit injected inputs to agent-type resolution; do not introduce a global
  runtime registry lookup.
- Extend Recordings projections for call signatures, reuse decisions, and nested
  scope without moving canonical ledger ownership into Factory
  Runtime.
- Extend `ui/src/features/factory-session-detail/` projections, hooks,
  components, messages, and tests rather than creating a parallel workflow-run
  feature.

### Contracts

- Publish `claude-workflow-v1` as the base JavaScript runtime descriptor and
  `you-workflow-v1` as its strict additive superset, with You option/host
  extensions in a closed `x-you` section.
- Widen `FactorySessionExecutionRequest.args` to any JSON value while keeping
  the existing source-selector and request-id contract.
- Add canonical callable-agent option/result schemas and mark `agent.run` plus
  legacy pipeline envelopes as deprecated migration aliases.
- Add stable codes for invalid metadata, nondeterministic APIs,
  item/lifetime limits, child capability mismatch, resume mismatch, and
  nested-depth violation.
- Add call-signature, resume-lineage, nested-scope, and explicit-phase fields
  only to public projections that customers need to explain the run. Keep raw
  source/prompt/checkpoint bodies artifact-owned and protected.
- Define Claude limits as ceilings narrowed by effective Factory, session,
  deployment, runner, and provider limits. You extensions never widen policy.
- Version the JavaScript runtime record and descriptor contracts before changing
  strict unions or resume hashing.

### Services

- Factory Definitions owns saved/named Factory and workflow source identity,
  authored agents, metadata, and persistence.
- Factory Runtime owns JavaScript validation, base/extension normalization,
  scheduler decisions, phase/nested scope, child admission, limit checks, and
  call-signature matching.
- Factory Sessions owns durable start/resume/continuation lifecycle,
  idempotency, canonical session lineage, and translation of admitted children
  to Worker invocations.
- Workers owns request-scoped child execution, worktree lifecycle, tool-policy
  shaping, output validation, and retry policy.
- Providers owns model/provider capability truth, structured-output transport,
  provider permission/isolation mapping, usage capture, and one normalized
  execution attempt.
- Recordings owns the canonical Factory Event ledger, artifacts, replay, and
  historical projections.
- Factory Visualization and the dashboard consume canonical projections; they
  do not infer scheduler state from local timers.

### API changes

- Author new request/response/projection components under `api/components/` and
  compose them from `api/openapi-main.yaml`; do not edit bundled or generated
  files directly.
- Keep `POST /factory-sessions/async` and `/sync` as the execution operations.
  Keep their canonical source-selector request shape and widen only `args` to
  any JSON value.
- Extend Factory preview and MCP source validation with canonical JavaScript
  feature, extension, and deprecation diagnostics.
- Extend MCP `you.factory_session.start_async`/`start_sync` schemas from the same
  authored contract. Do not create a transport-only workflow state store.
- Add CLI authoring flags only as adapters to the same input fields; CLI and API
  return the same canonical Factory Session.
- Refresh `api/openapi.yaml`, generated Go clients/servers, generated TypeScript,
  MCP discovery, publishable API schemas, and the JavaScript runtime manifest
  through repository generation targets.

### Tests

- Unit tests for metadata purity, source-size limits, arbitrary JSON args,
  nondeterministic members, child
  option normalization, JSON Schema validation, signature hashing, prefix
  invalidation, item/lifetime caps, deprecation diagnostics, and depth
  enforcement.
- Runtime integration tests with a controllable child executor proving pending
  promises, true thunk concurrency, independent pipeline advancement, ordered
  results, barrier behavior, per-item failure isolation, cancellation, and VM
  single-goroutine safety.
- Race, stress, and repeat-run tests at 4096 items, 1000 admitted agents, mixed
  completion order, nested workflows, and resume after
  process restart. Use deterministic gates/channels, not sleeps.
- Worker/Provider adapter tests that observe actual structured-output,
  worktree/isolation, model/effort, permission, usage, and capability decisions
  at injected command/ACP edges.
- Factory Session/Recordings integration tests for persistence-before-
  visibility, canonical events, source snapshots, continuation lineage,
  call-signature reuse, replay, and older-record compatibility.
- Functional scenarios under `tests/functional/orchestration/javascript/`
  constructed through `root.BuildProcess` and `Process.Execute`, using only
  `edges.Edges` replacements and provider command-runner mocks. Cover one
  unmodified Claude fan-out, pipeline, structured-output, nested, and resumed
  workflow, plus the expected diagnostic for a deferred live-budget workflow.
- REST/MCP/CLI contract parity tests over the same fixtures, including async
  start, notification/event bridging, status, dispatch, artifact, and result
  reads.
- UI projection, hook, component, localization, accessibility, responsive, and
  focused browser tests for declared phases, queue state, nested groups, usage,
  failure, and resume lineage.
- Documentation smoke tests for both reference topics and executable examples.

## Quality gates

Use focused tests during each story, then run the relevant shared gates:

- `go test ./pkg/services/factory_runtime/... ./pkg/services/factory_sessions/... ./pkg/services/workers/... ./pkg/services/providers/... ./pkg/services/recordings/...`
- focused functional orchestration cells through `root.BuildProcess` and
  `Process.Execute`
- `make generate-api` for direct API/Go/dashboard changes, or
  `make interfaces-all` when publishable schemas/clients change
- `make api-smoke`
- `make docs-reference-smoke`
- `make ui-test` and `make ui-lint` when the progress projection changes
- `go test -race` for the scheduler/runtime packages plus the explicit stress
  lane
- `make verify-fast`, followed by `make verify-pr`

## Out of scope

- Replacing canonical Factory/Factory Session vocabulary with Claude run/task
  resources.
- Exposing raw Node, Bun, filesystem, process, module, shell, network, secret,
  or connector globals inside the JavaScript VM.
- Guaranteeing identical tools or MCP authentication across providers that do
  not declare those capabilities.
- Token-level partial result streaming into the calling conversation.
- Claude's live `budget.total`/`spent()`/`remaining()` accounting in the initial
  convergence.
- An exact flat clone of Claude Code's top-level `Workflow` tool payload before
  the canonical script contract and fixtures are proven.
- Native binary multimodal values in `args`; file-path references remain the
  supported indirect mechanism.
- More than one nested workflow level.
- Opportunistic Petri-runtime, provider, CLI, or dashboard redesign unrelated
  to compatibility behavior.

## Delivery boundary

The work is complete only after required CI is terminal and passing, generated
artifacts are current, every blocking review conversation is addressed, merge
conflicts are resolved without discarding concurrent work, and the pull request
is actually merged. Opening a PR, pushing the implementation, obtaining
approval, or reaching green CI without merge is not completion.
