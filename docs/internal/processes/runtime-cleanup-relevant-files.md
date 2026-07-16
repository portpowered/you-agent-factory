# Runtime Cleanup Relevant Files

Use this map when changing runtime cleanup ownership, package placement, startup
wiring, or documentation that tells maintainers where new runtime behavior
belongs. Start with `docs/architecture/architecture.md`, especially the runtime
startup path, Factory Session state ownership, target package-family ownership
map, and migration-era surfaces sections.

## Placement Decision Guide

Classify the change by the owner of durable state or policy first, then keep
API, CLI, MCP, UI, and compatibility code as adapters around that owner. Broad
service facades, runtime-host wrappers, compose bridges, root workflow shims,
and compatibility aliases are placement evidence only when the change is a thin
delegation to the target owner or part of an active removal lane.

| Change area | Preferred owner for new behavior | Review guidance |
| --- | --- | --- |
| Dynamic workflow runtime behavior | `pkg/orchestrators/javascript/*`, with durable lifecycle and resume behavior in `pkg/factory/sessions/execution` | Put JavaScript source resolution, validation, policy, runtime execution, result shaping, checkpoints, and resume state in the JavaScript orchestrator packages. Route durable start, resume, lifecycle/control, artifact, result, and persisted execution through Factory Session execution owners. |
| Factory Session state | `pkg/factory/sessions` for live session state and read models; `pkg/factory/sessions/execution` for durable execution state | A Factory Session owns runtime identity, event history, lifecycle/control state, current work, Current Factory, runtime instances, stream identity, and session projections. `FactoryService` may locate or route to sessions, but must not become the state owner. |
| Model operations | `pkg/models/host` for process-wide runtime lifecycle and leases; `pkg/models/service` for model API service behavior; `pkg/models/local` for catalog compatibility projection | Keep readiness, supervised process lifecycle, capacity, leases, diagnostics, invocation gating, and host-owned execution in the model owner. Construct the model API service once from explicit model-scoped dependencies at application composition, then inject that same instance into transports and compatibility hosts. Transport handlers should call model service or API-surface adapters instead of embedding model runtime policy. |
| Worker and provider execution | `pkg/workers` and `pkg/workers/hosted` | Put provider adapters, script/agent/inference executors, mock workers, process runners, sidecars, hosted-worker integration, and worker execution diagnostics in the worker owner. Shared single-attempt provider invocation and canonical result mapping belong in `pkg/workers/providerexecution`; callers retain retry and durable lifecycle policy. Production creates the default provider per worker with worker-specific permissions, logging, progress publication, and command-runner options, so application graphs should own the runtime builder/factory that creates those providers rather than publishing a fabricated process-wide provider executor. Session or service code should inject callbacks and observers rather than owning provider behavior. |
| Work domain | target `pkg/work` | Put Work content, query/selection, graph/lineage, pure invocation input and return policy, materialization, and cron/time-work concepts in the collapsed Work owner. Until Batch 006 moves a slice, use its registered migration root and do not create a parallel implementation. |
| Platform infrastructure | `pkg/platform` | Put logging in `pkg/platform/logging`, policy-free replay artifact filesystem mechanics in `pkg/platform/replay`, file-backed metric recording in `pkg/platform/metrics`, cursor mechanics and safe persistence diagnostics in `pkg/platform/cursors`, and real or deterministic clock implementations in `pkg/platform/clock`. Keep replay event construction, reduction, recording lifecycle, and delivery policy in `pkg/factory/replay`; keep canonical event meaning, Factory Session state, metric and clock meaning, and cursor identity or recovery decisions at their domain boundaries. Shared log/metric path reservation is internal to `pkg/platform`. Platform implementations receive explicit inputs and never choose domain policy. |
| Transport boundaries | target `pkg/transports` | Put HTTP, CLI, MCP, generated transport contracts/clients, and boundary mapping at the process edge. Canonical Factory snapshots carried by event-derived domain projections or hydrated replay artifacts use `pkg/factory/contracts.FactorySnapshot` so unknown JSON fields survive projection cloning and replay reconstruction; decode them into generated API values only in an explicit boundary adapter such as `factorysnapshot.ToAPI`. Factory replay captures runtime definitions and metadata directly into that snapshot, and reconstructs its embedded runtime lookup from the snapshot without exposing a generated Factory in the replay API. Replay artifact construction accepts that snapshot directly; composition maps it to a generated value only when a public transport response requires one. Replay metadata comparison likewise consumes snapshots on both sides. The Factory owner defines `FactoryEvent`, `FactoryEventContext`, `FactoryEventType`, and the canonical event schema version; in-memory history, reconnect selection, live and durable event streams, runtime recorders, and `ReplayArtifact.Events` carry that detached domain envelope. Clone the envelope before exposing it to recorders or stream consumers so payload and context slices cannot mutate canonical history. Factory Session completion bindings consume the canonical event type recorder, including history replay for late binding, rather than accepting generated HTTP events. Replay parsing, sequencing, validation, and reduction operate on the domain envelope. Typed reducers should decode their owner-defined payload directly with `FactoryEvent.DecodePayload`; Factory run request and response, dispatch request, work-state change, Work-owned work request, and worker-execution-owned inference and dispatch responses demonstrate this while preserving historical compatibility fields and context fallbacks. Run-request artifact encoding likewise starts from the Factory-owned payload and uses the worker diagnostics owner's public-wire encoder; generated union decoding belongs in compatibility tests, not the production artifact builder. Run-request reduction rebuilds the domain `FactoryConfig` and embedded runtime definitions from the Factory-owned snapshot; dispatch reconstruction and submission defaults consume that domain configuration rather than retaining a generated public `Factory` in reduced state. Replay canonicalization, legacy cron repair, and adjacent-config hydration rewrite detached domain snapshots and payloads directly, preserving unknown fields and public JSON compatibility without rebuilding a generated Factory-event union. OpenAPI parity code compares the authored transport contract against the domain vocabulary, and generated discriminator decoding remains transport/test-boundary support rather than a production Factory dependency. Until Batch 006 moves a slice, use its registered migration root; transport adapters must not own domain policy. |
| Process startup and dependency construction | `cmd/factory`, target `pkg/root`, target `pkg/wire`, and `pkg/initializer` | Keep `cmd/factory` thin, normalize process input and select mode in `pkg/root`, expose one concrete graph constructor in `pkg/wire`, and execute startup/shutdown lifecycle for already-built transports and sidecars in `pkg/initializer`. Construction-phase callback bundles are test harnesses, not a public composition API. During migration, concrete graph assembly may reuse `composebridge.BuildCore`, but it must do so once inside `pkg/wire`, copy caller-owned config before normalization, project only narrow domain contracts into the graph, and retain cleanup ownership for the startup bundle rather than exposing `runtimehost.Host` or the bridge core. Stateful collaborators such as durable Factory Session execution must be constructed once per graph with the graph's normalized roots, clock, and runtime dependencies, then injected into compatibility facades rather than reconstructed there. A fallible graph phase should retain any closeable construction resource so `pkg/wire` can unwind acquired resources once, in reverse order, before returning a later phase failure. Initializer should record only successfully started collaborators, stop them in reverse order on partial failure or shutdown, and make graph close part of the same idempotent shutdown result. |

Orchestrator phase and checkpoint producers build canonical envelopes from
Factory-owned payload, status, artifact-reference, warning, and context
contracts. Generated OpenAPI decoding is a transport or compatibility-test
concern, not part of Factory event construction.

Factory Session lifecycle producers use Factory-owned canonical event payloads
with domain status values, then append the Factory-owned envelope directly.
Keep generated session-event unions at public mapping and compatibility-test
boundaries; runtime lifecycle recording should pass domain orchestrator strings,
Work content, and worker-owned failure detail.

Factory world-state reducers for Factory Session lifecycle and orchestrator
progress consume the same owner-defined envelope and payload contracts directly.
Convert a generated event only at the temporary outer projection compatibility
boundary, then use `FactoryEvent.DecodePayload`; do not reintroduce generated
status, content, checkpoint, artifact, or warning values inside the reducer.

Dispatch queue, interruption, reconciliation, synthetic reconnect, and artifact
creation producers use Factory-owned status, usage, artifact, provider-session,
and payload contracts and append the canonical domain envelope directly.
Generated dispatch lifecycle payloads are decoded only by public compatibility
tests and transport-facing projection adapters.

Work-state changes, terminal run responses, and Factory lifecycle changes use
Factory-owned payloads and state values when they enter canonical history.
Preserve the public camel-case JSON shape on those owner-defined payloads so
generated OpenAPI union decoding remains a boundary compatibility check rather
than a production event-construction dependency.

Work request and relationship-change producers use Work-owned payload, Work,
relation, content, and lineage contracts while the Factory owner supplies the
canonical event envelope and correlation context. Preserve accepted public
content filtering and relationship name/ID fallbacks before marshaling the
owner payload; generated union decoding belongs in boundary compatibility
tests, not canonical history construction.

Factory world-state reducers for work requests and relationship changes decode
those same Work-owned payloads from the canonical Factory event. Preserve
request/trace context fallbacks, detached Work content and tags, and name-based
relationship resolution in the reducer; a generated event may be converted
only at the temporary outer projection compatibility entrypoint.

Dispatch-request world-state reduction follows the same boundary: decode the
Factory-owned event and dispatch payload before consuming Work or resource
tokens. Preserve context-first Work and chaining-trace correlation, dispatch
input snapshots, and topology-derived workstation, worker, provider, and model
metadata; generated event conversion remains at the temporary outer projection
compatibility entrypoint.

Dispatch-response world-state reduction decodes the canonical Factory envelope
into the worker-execution-owned completion payload. Route Work output with the
worker outcome, rebuild detached Work content and lineage, release named
resources, and retain failure metadata without converting through generated
HTTP types; generated conversion remains only at the temporary outer projection
compatibility entrypoint.

Inference, script, and agent-run world-state reduction decodes the canonical
Factory envelope into `pkg/workers/execution` payloads. Keep provider-session
metadata detached, decode safe diagnostic JSON through the worker diagnostics
owner, and reject malformed diagnostics before mutating the projection;
generated event conversion remains only at the temporary outer compatibility
entrypoint.

Initial-structure, run-request, Factory-change, Factory-state, and Work-state
world-state reduction follows the same rule: decode the Factory-owned event and
owner-defined payload first, retain a detached `FactorySnapshot`, and apply
state or topology changes from those domain values. Generated event unions may
enter only through the temporary outer projection compatibility path. Decode
the detached snapshot's topology subset beside the Factory reducer into
Factory-owned projection contracts; do not decode it into a generated HTTP
Factory first. Preserve the full detached snapshot separately so unknown
forward-compatible fields survive reconstruction.

JavaScript checkpoint/phase and artifact world-state reduction also consumes
Factory-owned event payloads. Keep checkpoint timestamps UTC-normalized, detach
phase history slices, and translate artifact redaction/capture metadata from
Factory contracts; generated unions belong only at the outer compatibility
entrypoint while that adapter remains.

Initial-structure and run-request producers carry the Factory-owned detached
`FactorySnapshot` inside Factory-owned payloads. Runtime composition should
hand the editable snapshot to event history without decoding it through a
generated transport model; generated Factory event unions remain an outward
compatibility boundary only.

Factory-change producers likewise carry the replacement `FactorySnapshot` in
the Factory-owned payload and append the canonical envelope directly. Runtime
coordinators should read detached canonical history when deriving that payload,
not round-trip the initial structure through the generated event union.

Agent-run executors publish worker-execution-owned completion facts through
`pkg/workers/execution`; Factory event history owns the `AGENT_RUN_RESPONSE`
envelope, ordering, and canonical time normalization. Keep safe diagnostics in
their camel-case event encoding and prove generated OpenAPI union compatibility
at the test boundary instead of constructing generated events in the executor.

Script executors likewise publish worker-execution-owned request and response
facts through `pkg/workers/execution`; Factory event history owns the canonical
`SCRIPT_REQUEST` and `SCRIPT_RESPONSE` envelopes, event vocabulary, ordering,
and time normalization. Keep resolved command arguments and bounded process
results in the worker payload while excluding environment values and stdin;
prove the generated OpenAPI union only at the compatibility boundary.

Model-backed worker composition publishes worker-execution-owned request and
response facts through `pkg/workers/execution`; Factory event history owns the
canonical `MODEL_REQUEST` and `MODEL_RESPONSE` envelopes, vocabulary, ordering,
correlation context, and UTC normalization. Keep resolved bindings, resource
summaries, safe diagnostic JSON, output content, and load timing in the worker
payload, and prove generated OpenAPI union compatibility after history appends
the canonical event rather than constructing generated events in composition.

Production command runners must remain blocking without taking lifecycle ownership
back from `pkg/initializer`. The entrypoint should construct and start the graph
through `pkg/root`, then let the returned application wait for its selected
graph-owned transport and perform the same idempotent reverse-order shutdown.
Map default/service run policies to initializer's API lifecycle plan and explicit
local batch policies to its CLI lifecycle plan before constructing the run
application; runtime mode alone must not silently select the foreground edge.
Transport startup must receive the graph's already-composed API surface, and
startup diagnostics should be copied into immutable graph metadata rather than
recovered later through `runtimehost.Host` or `FactoryService`.

A graph-owned transport is only the foreground API, CLI, or MCP edge. It must
not start the runtime loop, worker scheduler, filesystem watcher, metrics
observer, dashboard renderer, or another sidecar behind its `Run` callback.
`pkg/wire` should construct those inert lifecycle handles explicitly, and
`pkg/initializer` should start runtime, worker/watcher, and dashboard handles
before the selected transport, then stop them in the exact reverse order. A
production-graph test should delegate through the real handles and observe this
sequence so fake-only initializer coverage cannot hide an empty sidecar graph.
Mode-specific lifecycle planning must validate every required handle, including
typed-nil interface values, before activation. Optional handles should be
omitted from the plan, and graph handles that are not selected for the mode must
receive no start, wait, or stop calls. The selected foreground transport must be
joinable so initializer can validate complete lifecycle ownership before any
component starts.

One initializer run derives a single child context for every selected lifecycle.
It observes every started lifecycle that supports `Wait`, not only the foreground
transport; any terminal exit cancels that shared context before reverse-order
stops begin. `Stop` remains responsible for joining component-owned work, and
initializer also drains every lifecycle wait before returning. Cancellation and
deadline exits are normal shutdown outcomes. When multiple components return
non-cancellation failures, report them in declared lifecycle-plan order rather
than arrival order so goroutine scheduling cannot change terminal precedence.

The production `you mcp serve` branch follows the same ownership path even
though it does not activate run sidecars. Resolve the selected fixture-backed
or runtime-backed durable execution service before startup, retain that exact
instance in `wire.Graph`, construct the MCP stdio lifecycle from the request's
explicit reader and writer, and let `pkg/initializer` start, wait for, stop, and
close that graph. Do not return a separately composed MCP application from the
process graph builder.

Factory Session selectors at that graph-owned transport boundary must round-trip
the canonical ID returned by list responses. Registry aliases such as
`~default` remain valid compatibility selectors, but production startup tests
should pass a listed canonical ID back through a session-scoped API operation so
the public identifier cannot drift from runtime-host routing.

When a change spans rows, place the durable state or policy in its owner and
adapt outward. For example, a new session read that exposes JavaScript
orchestrator progress should keep progress derivation in Factory Session or
orchestrator owners, then map it through API, CLI, MCP, or UI boundaries.

## Root Package Guardrails

New root `pkg/` package families need an explicit target-owner rationale in the
PRD, implementation notes, or PR conversation before production behavior lands
there. The rationale should state:

- which durable state or policy the package owns
- why an existing target owner cannot own the behavior
- which adapters are allowed to depend on it
- whether the package is permanent or part of an active removal lane

If that rationale is missing, the change should land in an existing target owner
or be limited to a migration/removal lane that deletes, aliases, or delegates
old behavior toward the documented owner.

For an existing migration-only root, also verify its target owner, Batch 006-008
work item, and deletion gate against the register in
`docs/architecture/architecture.md`. The exception is temporary permission to
finish the named move, not ownership rationale for new product behavior.

`make pkg-boundary` also enforces the composition-root import direction:
`pkg/wire` may import the domain owners it composes, while only `pkg/root`,
`pkg/initializer`, and `pkg/wire` itself may import `pkg/wire`. Domain and
transport packages should own narrow contracts that startup injects instead of
depending outward on the application graph.

The same check rejects recreation or import of converged roots and reports the
canonical replacement: `pkg/packagedfactories` to `pkg/factory/packages`,
`pkg/factorydefinition` to `pkg/factory/definition`,
`pkg/factorysessionexecution` to `pkg/factory/sessions/execution`,
`pkg/factorysessions` to `pkg/factory/sessions`, and `pkg/petri` to
`pkg/orchestrators/petri`. Add a retired-root mapping when completing a package
convergence so an old import cannot hide inside an otherwise approved family.

## Vocabulary Guardrails

Changed customer-facing docs, API descriptions, CLI help, dashboard copy, and
process guidance should use the public resource vocabulary from
`docs/architecture/data-model.md`: Factory, Factory Session, Current Factory,
Work, Work Request, and Provider Session.

Do not describe JavaScript orchestration as a separate customer-facing
`DynamicWorkflowRun` resource. Describe it as Factory Session execution with a
JavaScript orchestrator kind, with workflow-named commands, tools, or routes
treated as compatibility aliases when they still exist.

Petri-net vocabulary such as tokens, places, transitions, markings, and guards
is allowed only when the text explicitly describes internal implementation
details or `pkg/orchestrators/petri` ownership. It should not be the primary wording for
customer-facing Factory Session, Work, or event-stream behavior.

## Focused Verification

For runtime-cleanup documentation changes, run a changed-docs vocabulary check
after review against `docs/architecture/data-model.md`:

```sh
changed_docs="$(git diff --name-only --diff-filter=ACMRT origin/main...HEAD -- docs prd.md)"
test -z "$changed_docs" || rg -n "DynamicWorkflowRun|\\b(Petri|petri|tokens?|places|transitions|markings?)\\b" $changed_docs
```

Inspect each hit. Accept hits only when they are guardrail text or explicitly
internal implementation notes; otherwise revise the changed docs to use Factory
Session, Work, Work Request, Provider Session, event, or target-owner wording.

Documentation-only runtime-cleanup changes should also run `git diff --check`
and `make typecheck`. Add `make docs-reference-smoke` only when packaged
`docs/reference/` content or `you docs` routing changes.

New production composition packages such as `pkg/wire`, `pkg/root`, and
`pkg/initializer` are subject to the non-baselined per-package Go coverage
minimum. Exercise their observable construction, lifecycle, failure, and
shutdown behavior and run `make test-backend-verification`; do not add a new
package to the temporary coverage baseline merely because production adoption
made it visible to the merged coverage profile.

When changing durable Factory Session execution construction, run
`make durable-runtime-construction-check`. The guard permits direct
`NewJavaScriptRuntimeService` calls only in the package-local execution-provider
factory and approved deterministic test harness. Project-local persistence path
resolution and directory-store construction belong at the fallible application
composition boundary in `pkg/factory/sessions/execution/service.go`; production
runtime code must receive either that injected store or an explicit disabled
policy and must not use a persistence boolean.

The guard also rejects `BuildInvocationBootstrap`, `NewExecutionService`,
`NewFakeServiceFromContractFixtures`, and `ProjectPersistence` calls outside
approved application-composition owners. Transport packages consume injected
invocation and durable-execution collaborators; narrow migration exceptions
must name the unfinished story and be removed with that story.
Model invocation composition is owned by `pkg/wire/model_invocation.go`; its
typed builder flows through `pkg/root` into the CLI models adapter, and the
construction guard intentionally grants no model-transport exception.
MCP durable execution composition is owned by `pkg/wire/process.go`; the
typed builder flows through the root-owned production graph builder into
`pkg/wire/process.go`, and `pkg/transports/cli/mcp` accepts only the resulting
service. Production-composition tests should execute the real root command and
also assert that the wire graph retains the exact injected service instance.

Package tests, `testdata`
fixtures, generated code, dependencies, coverage, and build artifacts are not
production construction sites. Production JavaScript live-child packages must
also invoke providers through `pkg/workers/providerexecution`; the guard rejects
direct provider-package imports and `Infer` calls there while permitting tests,
deterministic fakes, and shared contract types.

Canonical Factory Session event recording follows the same ownership check.
`pkg/factory/sessions/execution` owns canonical event validation, ordered append,
persistence, live publication, and replay projection. Production orchestrator
packages return facts and typed orchestration records to that owner; they must
not call canonical event builders or import `pkg/platform/cursors/session` directly.
Petri output mutations become final in the post-transition response boundary;
wire them with `factory.WithPetriMutationRecorder` to
`JavaScriptRuntimeService.RecordPetriTokenMutations`, and propagate persistence
errors so an unrecorded completion cannot finish its tick.
`make durable-runtime-construction-check` reports those bypasses with remediation
to use the Factory Session recorder, while allowing package tests and explicitly
typed JavaScript checkpoint or Petri internal records.

Worker execution boundaries use the same fact-to-envelope placement rule.
Provider, script, and agent-run executors emit worker-owned facts from
`pkg/workers/execution`; `pkg/factory/events` assigns the canonical Factory
event vocabulary, schema version, ordering, correlation context, and UTC time.
Generated OpenAPI event unions are decoded only at transport-facing or explicit
public-compatibility test boundaries, not passed back into worker execution.
Factory world-state reducers likewise decode `pkg/factory/contracts.FactoryEvent`
payloads from their semantic owners. Canonical runtime and history callers use
`ReconstructCanonicalFactoryWorldState`; generated-event callers cross
`pkg/transports/mapping/factoryeventprojection`, which converts the full envelope
before reduction. Keep generated union decoding and generated Work conversion
helpers out of the Factory projection owner.

JavaScript result shaping follows the same boundary. The orchestrator result
package returns owner-defined runtime status, checkpoint, artifact, primary
result, and result-update projections. `pkg/transports/mapping` converts those
values to generated live-session, durable-result, and Factory-event response
models. Factory Session callers may assemble the domain input, but generated
status, checkpoint, artifact, and Work content values must not flow back into
the orchestrator result package.
