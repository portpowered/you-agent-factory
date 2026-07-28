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
| Model operations | `pkg/models/host` for process-wide runtime lifecycle and leases; `pkg/models/service` for model API service behavior; `pkg/models/local` for catalog compatibility projection | Keep readiness, supervised process lifecycle, capacity, leases, diagnostics, invocation gating, and host-owned execution in the model owner. Construct the model host, managed local runtime, and model API service once through their fallible package constructors from the shared `pkg/wire` model provider set; Wire may adapt process, cache, pull, invocation, clock, logger, and metrics edges but must not recreate model defaults or lifecycle policy. Inject the same model service instance into transports and compatibility hosts. Direct service compatibility builders may consume an explicitly supplied `ModelAPI` or expose an unavailable boundary, but must not assemble a fallback model service; CLI compatibility entrypoints must likewise require the root/Wire-injected builder instead of selecting a service-package constructor. Transport handlers should call model service or API-surface adapters instead of embedding model runtime policy. |
| Worker and provider execution | `pkg/workers` and `pkg/workers/hosted` | Put provider adapters, script/agent/inference executors, mock workers, process runners, sidecars, hosted-worker integration, and worker execution diagnostics in the worker owner. Shared single-attempt provider invocation and canonical result mapping belong in `pkg/workers/providerexecution`; callers retain retry and durable lifecycle policy. Production creates the default provider per worker with worker-specific permissions, logging, progress publication, and command-runner options, so application graphs should own the runtime builder/factory that creates those providers rather than publishing a fabricated process-wide provider executor. `pkg/workers/application` groups the validated provider factory, script factory, selected command edges, and hosted configuration selected by `pkg/wire`; runtime/session construction consumes that component and may add per-session mock or replay wrappers without reselecting its PTY or hosted edges. Downstream service, runtime-build, model-catalog, and compose paths must reject a missing component instead of fabricating production defaults or discarding constructor errors. Package-owned constructors should validate explicit process edges before startup; provider commands, script commands, and Agy PTY allocation are distinct typed edges and must not be collapsed into one runner. Hosted pollers follow the same rule: package construction validates runtime and submission dependencies, normalizes production HTTP, endpoint, secret, clock, and logger defaults, and returns errors before scheduler goroutines start. Pass the composed hosted-worker configuration intact into the scheduler; do not flatten its edges into coordinator fields and reconstruct it during lifecycle startup. Session or service code should inject callbacks and observers rather than owning provider behavior. |
| Work domain | target `pkg/work` | Put Work content, query/selection, graph/lineage, pure invocation input and return policy, materialization, and cron/time-work concepts in the collapsed Work owner. Until Batch 006 moves a slice, use its registered migration root and do not create a parallel implementation. |
| Platform infrastructure | `pkg/platform` | Put logging in `pkg/platform/logging`, policy-free replay artifact filesystem mechanics in `pkg/platform/replay`, file-backed metric recording in `pkg/platform/metrics`, cursor mechanics and safe persistence diagnostics in `pkg/platform/cursors`, and real or deterministic clock implementations in `pkg/platform/clock`. Keep replay event construction, reduction, recording lifecycle, and delivery policy in `pkg/factory/replay`; keep canonical event meaning, Factory Session state, metric and clock meaning, and cursor identity or recovery decisions at their domain boundaries. Shared log/metric path reservation is internal to `pkg/platform`. Platform implementations receive explicit inputs and never choose domain policy. |
| Transport boundaries | target `pkg/transports` | Put HTTP, CLI, MCP, generated transport contracts/clients, and boundary mapping at the process edge. Canonical Factory snapshots carried by event-derived domain projections or hydrated replay artifacts use `pkg/factory/contracts.FactorySnapshot` so unknown JSON fields survive projection cloning and replay reconstruction; decode them into generated API values only in an explicit boundary adapter such as `factorysnapshot.ToAPI`. Factory replay captures runtime definitions and metadata directly into that snapshot, and reconstructs its embedded runtime lookup from the snapshot without exposing a generated Factory in the replay API. Replay artifact construction accepts that snapshot directly; composition maps it to a generated value only when a public transport response requires one. Replay metadata comparison likewise consumes snapshots on both sides. The Factory owner defines `FactoryEvent`, `FactoryEventContext`, `FactoryEventType`, and the canonical event schema version; in-memory history, reconnect selection, live and durable event streams, runtime recorders, and `ReplayArtifact.Events` carry that detached domain envelope. Event-history reads expose only detached canonical values through `CanonicalEvents`; generated union conversion belongs in transport adapters or compatibility-test helpers, not an `Events` method on the Factory owner. Clone the envelope before exposing it to recorders or stream consumers so payload and context slices cannot mutate canonical history. Factory Session completion bindings consume the canonical event type recorder, including history replay for late binding, rather than accepting generated HTTP events. Replay parsing, sequencing, validation, and reduction operate on the domain envelope. Reconnect selection consumes canonical history directly; generated event conversion occurs only after selection when a public response requires it. Typed reducers should decode their owner-defined payload directly with `FactoryEvent.DecodePayload`; Factory run request and response, dispatch request, work-state change, Work-owned work request, and worker-execution-owned inference and dispatch responses demonstrate this while preserving historical compatibility fields and context fallbacks. Run-request artifact encoding likewise starts from the Factory-owned payload and uses the worker diagnostics owner's public-wire encoder; generated union decoding belongs in compatibility tests, not the production artifact builder. Run-request reduction rebuilds the domain `FactoryConfig` and embedded runtime definitions from the Factory-owned snapshot; dispatch reconstruction and submission defaults consume that domain configuration rather than retaining a generated public `Factory` in reduced state. Replay canonicalization, legacy cron repair, and adjacent-config hydration rewrite detached domain snapshots and payloads directly, preserving unknown fields and public JSON compatibility without rebuilding a generated Factory-event union. OpenAPI parity code compares the authored transport contract against the domain vocabulary, and generated discriminator decoding remains transport/test-boundary support rather than a production Factory dependency. Until Batch 006 moves a slice, use its registered migration root; transport adapters must not own domain policy. |
| Process startup and dependency construction | `cmd/factory`, target `pkg/root`, target `pkg/wire`, and `pkg/initializer` | Keep `cmd/factory` thin. `pkg/root` injects the lazy bundle and lets the CLI select an explicit service entrypoint after parsing. Both public injectors use one canonical Wire set. The selected run/API or stdio initializer constructs only its service subtree and flattens it into `bundle.Bundle`; `pkg/initializer` executes startup and shutdown for the attached handles. Copy caller-owned config before normalization and retain cleanup ownership in the bundle. Fallible construction retains each closeable resource so Wire can unwind once in reverse order; initializer records only successfully started collaborators and closes the bundle through the same idempotent shutdown path. |

For services whose behavior depends on one Factory Session runtime, construct an
inert process-scoped root through the service-local `wire` package and pass
runtime-specific values through an owner-defined typed binding. Models uses
`OpenRuntimeScope` on its canonical process-scoped `Service` and returns only an
opaque `RuntimeScopeRef`; Factory Session selection must keep using the same
Models root rather than receiving or constructing a second `Service`. Register
each successfully opened scope with the opening transaction immediately,
release later-opened resources before earlier scopes on failure, and transfer
the same exactly-once reverse-order closer to the successful runtime product.
Thread the opaque scope beside that root through downstream Worker and transport
bindings, and attach it to typed Models requests at the final consumer. When
live session state is not installed yet, snapshot Models scope configuration
from the already-loaded Factory definition so startup-time catalog and
invocation consumers observe the selected Factory without a lazy service view.
When a scope lazily materializes owner-internal runtime capability, revalidate
the scope while serializing cache insertion so a concurrent close cannot let a
stale resolution reinsert capability after cleanup reports success. Prove that
boundary with a deterministic concurrent operation-versus-close test.
Use a cleanup context detached from request cancellation when closing an
already-acquired owner scope. Do not introduce a Wire-owned binder, return a
private peer runtime-assembly contract, or inject a callback that lets the
runtime-opening consumer select or construct the concrete service.

Keep the product-facing service root to its canonical `Service` interface plus
detached value contracts. Construction-only roles belong in the service-local
`wire` package, implementation collaborators belong under `internal`, and
cross-service consumers should own the narrow interface they actually call.
List/read projections must carry detached summaries rather than mutable
session registries, response stores, or live-session implementation pointers.

Parent-private nested services follow the same recursive shape under
`pkg/services/<owner>/internal/services/<capability>`: keep exactly one
capability `Service` interface at that root, construct it through the nested
`wire` package, and place concrete behavior under `internal/service`. Runtime
assembly must clone mutable opening options before passing them to per-role
collaborators and return an empty root-owned result on any validation,
resolution, rejection, or completeness failure so partial bindings never
escape.

Models asset source selection, cache inspection, verified preparation, and
private local-runtime cache layout all belong to the single parent-private
`pkg/services/models/internal/services/assets` service. Compatibility adapters
receive that already-constructed service plus an opaque Models runtime scope;
they must not reconstruct an asset puller or its filesystem/network effects.
When a Factory Session runtime configuration is intentionally unavailable until
assembly completes, resolve and memoize the immutable Models scope on first
asset use, retrying a pre-assembly unavailable result instead of snapshotting a
nil configuration permanently.

When this recursive shape adds measured Go packages, register every package in
both `docs/internal/baselines/go-unit-coverage-package-minimums.json` and
`docs/internal/baselines/go-functional-coverage-package-minimums.json` in
import-path order. Record each lane's observed numeric floor for executable
packages and the standard measurement exception for interface-only packages
with no measurable statements, then verify both coverage lanes.

Editable Factory persistence consumes a detached `FactorySnapshot` and the
Factory-owned `FactoryVersion`; it removes version metadata before split-layout
normalization and stamps the next durable version afterward. Generated Factory
and hybrid timestamp values must be detached at an outer compatibility edge,
and transport error symbols should alias the Factory-owned stale-version
sentinel rather than owning optimistic-concurrency policy.

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
back from `pkg/initializer`. `pkg/root` injects the inert bundle before parsing;
the selected CLI service entrypoint constructs its bundle resources and lets the
returned application wait for its selected transport and perform the same
idempotent reverse-order shutdown.
`initializer.BuildCore` must require the valid worker application composed at
that outer boundary; it must not fill in a missing component with production
defaults before loading configuration or starting lifecycle work.
Production-shaped functional runs replace process side effects through the typed
`pkg/wire.FunctionalEdges` input. Apply those edges to an invocation-local config
copy before calling the shared application builders; do not add CLI flags,
package globals, untyped dependency bags, or test-side service-config mutation.
The zero-value edge input must retain production defaults.
Functional process hosts should cancel and join `root.Run` within a caller-owned
bound, then report an immutable process outcome plus the last customer-boundary
readiness result. They must not close graph resources directly: listener and
sidecar cleanup remains owned by the initializer, and tests should prove release
through the public endpoint and process-completion surface.
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

The production `you mcp serve` branch follows the same ownership path without
constructing HTTP, dashboard, worker, watcher, or runtime sidecars.
Fixture-backed MCP may construct its narrow fake execution edge; runtime-backed
MCP retains the completed `bundle.Bundle`, including registry, persistence,
runtime-build, durable-execution, and startup-bundle cleanup ownership, then
attaches only its MCP host and stdio lifecycle. `pkg/initializer` starts, waits
for, stops, and closes that bundle. Runtime-backed CLI session execution retains
the same flat bundle in its closable execution owner and closes it exactly once
after successful or failed command execution.

The repository-only MCP client harness under `internal/testutil/mcpclient`
records real stdio traffic at the newline-delimited frame boundary. Its reader
must buffer fragmented pipe reads until a complete frame is available before
passing bytes to the SDK; raw `Read` chunk boundaries are not JSON-RPC message
boundaries and cannot safely drive correlation or shutdown assertions.

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

`make ownership-boundary-check` (`cmd/ownershipboundarycheck`) classifies
`pkg/services/<owner>` surfaces from the committed service tree: direct children
of `pkg/services` form the owner inventory, `pkg/services/edges` is the Process
Edges owner, each inventoried `pkg/services/<owner>` path is that owner's root
contract surface, and any deeper path under that owner is non-root for
cross-owner import decisions. Do not grow a hand-maintained private-subpackage
catalog for the generic owner-derived rule. The same inventory drives the
cross-owner peer rule: importing an inventoried peer root
(`pkg/services/<peer>`) is allowed, and same-owner non-root imports do not fire
the peer rule. Peer implementation and nested-subservice imports fail with a
diagnostic that names the importer owner and peer owner and directs remediation
to the peer root contract. Introducing a previously unlisted deeper path under
an inventoried owner is rejected automatically; do not edit a private-root
catalog to make that fail. Exact documented leaf-effect ports remain pairwise
importer→import exceptions, not a private-root allowlist. Peer-import debt in
`ownership-boundary-baseline.json` stays deletion-only/ratchet: clear the
import (or delete the exact baseline entry), and do not relocate production
service packages solely to satisfy the checker.

The guard rejects new production imports from `pkg/factory/**`, `pkg/work/**`,
`pkg/workers/**`, and `pkg/models/**` into `pkg/transports/**`. Generated OpenAPI
values must be converted under `pkg/transports/mapping`, while domain packages
accept and return owner-defined contracts. The exact worker/model migration-file
inventory in `cmd/pkgboundarycheck/main.go` is deletion-only: remove entries as
the remaining compatibility adapters move outward, and never add a new entry to
make a reverse dependency pass.
Factory definition serialization returns a detached
`pkg/factory/contracts.FactorySnapshot` and domain `FactoryVersion`. Read-model
identity changes use snapshot operations that preserve unknown JSON fields;
generated Factory decoding and hybrid-timestamp formatting happen only in the
outer definition service or transport mapping boundary.
Factory definition hosts exchange detached `FactorySnapshot` values and plain
domain names with the definition owner. Composition adapters capture generated
Factory values before invoking that host seam and cast names back to generated
types only when calling transport-compatible runtime activation code.
Replace-current and named-upsert persistence consume the Factory-owned
`EditableFactory` request and return detached snapshots plus domain versions;
the remaining outer definition compatibility adapter captures generated
requests and formats generated responses.
Repository-scanning Factory Session removal gates read canonical sources such as
`docs/reference` directly from their explicit repository root; they must not
import CLI or HTTP packages merely to inspect those source artifacts.
Packaged Factory observability classifies stable domain failure evidence and
must not import transport-mapping error sentinels merely to recognize their
customer-safe text.
Managed-runtime readiness states, invocation-blocking sentinels, and the narrow
readiness-error seam belong in `pkg/models/managedruntime`. Worker policy should
classify that model-owned seam; `pkg/transports/mapping` may preserve historical
error identity by aliasing its public compatibility sentinels to the model owner.
The model host likewise carries model-owned runtime identity, locality,
operation, readiness, lifecycle, and pull-outcome values. Only the outward
transport mapping layer projects that host state into generated `ManagedRuntime`
contracts; local asset integration exchanges `pkg/models/assets.PullResult`
without routing through transport aliases.
Model discovery summaries, details, capabilities, resources, compatibility
status, and load-state vocabulary belong in `pkg/models/catalog`. The local
catalog builder returns those contracts, and
`pkg/services/models/transports/http` alone converts them to generated model
list and detail values.

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

The architecture checker encodes a Petri-public-surface prohibition in
`cmd/pkgboundarycheck/petri_public_surface.go`: raw nets, markings, tokens,
transitions/enabled-transition engine shapes, and engine snapshots are rejected
outside `pkg/services/factory_runtime/internal/`. Authored Factory Definition
`orchestrator.kind = PETRI` remains allowed as configuration. Focused fixtures
in `petri_public_surface_test.go` cover vocabulary shapes and the required
public-surface categories (public API, transport, integration contract, and
functional test). The prohibition runs on the `make lint` package-boundary path;
pre-existing live-tree debt is inventory-only in
`petri-public-surface-baseline.json` with an exact deletion gate pointing at
Runtime Petri-boundary retirement / IMP-RUN-01 (no new baseline growth).
Each entry's count must match every live occurrence of that exact
file/import/symbol edge; after creating or editing the inventory, run
`make pkg-boundary` so an omitted edge or undercount cannot leave the base
branch lint-broken. Correct a count only when history proves all occurrences
predate the deletion gate; new references must be retired instead of baselined.

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
resolution and durable-engine selection belong at the fallible owner-private
composition boundary in
`pkg/services/factory_sessions/internal/services/durable_execution/internal/service/construction.go`;
production runtime code must receive either that injected store or an explicit
disabled policy and must not use a persistence boolean.

Construct `pkg/service/runtimebuild.Service` with an explicit clock, logger, and
runtime bundle builder, and propagate its constructor error before initializer
lifecycle begins. Before building any session runtime, application composition
must attach the graph-owned durable execution service's Petri mutation recorder;
that preserves one canonical Factory Session event and snapshot owner across
startup, replacement, and resume builds.

The shared Wire provider set constructs persistence, durable execution, and the
recorder-configured runtime-build service before `pkg/wire/runtime_core.go`
assembles the completed runtime core. Root one-shot invocation and retained
compatibility facades adapt that same runtime core; neither path may re-enter a
service builder to create a replacement session foundation.

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
Runtime-backed MCP serve and CLI session-execution adapters share
`buildRuntimeBackedSessionExecutionService` in `pkg/wire/session_execution.go`:
they must construct one completed `InjectRuntimeCore` graph and return its exact
durable execution collaborator, never project persistence or construct a second
execution service. Keep fixture-backed requests as explicit edge substitutions,
and prove each runtime-backed adapter's one-core identity plus persistence-store
identity in `pkg/wire/process_test.go`.

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
After session invocation selects its terminal result, route that result through
the same execution owner before returning it so the durable lifecycle, result,
canonical completion event, and primary result become visible atomically to a
subsequent API, CLI, or MCP process.
`make durable-runtime-construction-check` reports those bypasses with remediation
to use the Factory Session recorder, while allowing package tests and explicitly
typed JavaScript checkpoint or Petri internal records.

Worker execution boundaries use the same fact-to-envelope placement rule.
Provider, script, and agent-run executors emit worker-owned facts from
`pkg/workers/execution`; `pkg/factory/events` assigns the canonical Factory
event vocabulary, schema version, ordering, correlation context, and UTC time.
Generated OpenAPI event unions are decoded only at transport-facing or explicit
public-compatibility test boundaries, not passed back into worker execution.
Worker-safe diagnostic redaction, cloning, event-payload normalization, and
rehydration belong in `pkg/workers/diagnostics`; conversion to generated safe
diagnostic, provider-session, failure-metadata, and world-inspection contracts
belongs in `pkg/transports/mapping/workerdiagnostics`. Keep HTTP projections on
that outward mapper rather than adding generated types to the worker owner.
Initial topology snapshots follow the same rule: `pkg/factory/events/snapshot`
constructs the detached canonical Factory document from owner-defined topology,
and transport adapters decode that snapshot only when a generated Factory is
required. Do not construct canonical event snapshots through generated HTTP
models.
Factory world-state reducers likewise decode `pkg/factory/contracts.FactoryEvent`
payloads from their semantic owners. Canonical runtime and history callers use
`ReconstructCanonicalFactoryWorldState`; generated-event callers cross
`pkg/transports/mapping/factoryeventprojection`, which converts the full envelope
before reduction. Keep generated union decoding and generated Work conversion
helpers out of the Factory projection owner.

The top-level `factory.Factory` read contract also returns detached canonical
`FactoryEvent` values. Runtime and session policy should project those values
directly; service or runtime-host compatibility APIs that still expose the
generated OpenAPI union convert only through
`pkg/transports/mapping.FactoryEventsToAPI`.
Reconnect selection returns the Factory event owner's cursor sentinel so the
HTTP boundary can classify it without a transport-owned error flowing back into
the runtime. Runtime lifecycle recording likewise uses Factory-owned operation
values rather than generated enum constants.

JavaScript result shaping follows the same boundary. The orchestrator result
package returns owner-defined runtime status, checkpoint, artifact, primary
result, and result-update projections. `pkg/transports/mapping` converts those
values to generated live-session, durable-result, and Factory-event response
models. Factory Session callers may assemble the domain input, but generated
status, checkpoint, artifact, and Work content values must not flow back into
the orchestrator result package.

Checkpoint-backed live and partial result reads follow that boundary too.
`pkg/factory/sessions.ProjectSessionResult` returns the JavaScript result
owner's detached live projection, while `ProjectSessionPartialResult` returns
its detached checkpoint-backed partial projection. Convert those values through
`pkg/transports/mapping` only at the session-service compatibility edge that
assembles generated public responses. Control-plane result reads return the
detached result-owner values and must not import transport contracts.

Normalized Factory Session logical-target identity, target discovery, selection,
and restart remapping are owned by the injected subservice rooted at
`pkg/services/factory_sessions/internal/services/identity`; its implementation receives
home, directory-inspection, and symlink-resolution effects through service-local
Wire. The outer Factory Sessions service delegates open, list/read projection,
and reconnect behavior to that subservice without exposing it cross-service.
Canonical reference and runtime projection value contracts remain at the
Factory Sessions root. Convert those values to generated
logical-target kinds and provider-boundary fields only in
`pkg/transports/mapping/factorysession`; domain target normalization must not
return generated HTTP values.

Factory Session folder/discovery validation reasons remain plain value
constants at the service root, while concrete error state, target construction,
config-load aggregation, and reason inspection live under
`pkg/services/factory_sessions/internal/sessionvalidation`. Owner-private open,
logical-target, and session-service code may consume that implementation;
cross-owner transport tests should exercise the public error-code/target
protocol with boundary fakes instead of constructing a Factory Sessions
implementation error.

Factory Session response-event validation, retained-then-live subscription,
cursor filtering and gap signaling, event-store lifecycle, and internal stream
registry allocation are owned by the injected subservice rooted at
`pkg/services/factory_sessions/internal/services/response_stream`. Construct the inert
capability in Factory Sessions-local Wire, bind its stores and registries only
after an explicit runtime clock is available, and delegate outer-service
subscriptions through it. Keep transport consumers on the Factory Sessions root
cursor contract; they must not construct stores or import this private
capability.

Live Factory Session opening, ordered registry reads, runtime snapshots,
pause/resume decisions, lifecycle diagnostics, and stop coordination are owned
by the runtime-bound subservice rooted at
`pkg/services/factory_sessions/internal/services/live_runtime`. Construct it through
Factory Sessions-local Wire from explicit runtime host callbacks; construction
must not open, start, stop, or inspect a runtime. The outer Factory Sessions
service delegates live operations through this capability, while discovery and
target selection remain with the identity owner and durable lifecycle remains
with the durable-execution owner. Prove customer-boundary live open/list/get/control/close
through `support.StartFunctionalAPIServer` / `root.BuildProcess` in
`tests/functional/sessions/live_runtime_build_process_test.go`, and prove
live_runtime ownership through Sessions-root composition tests in
`pkg/services/factory_sessions/internal/sessionservice/live_runtime_composition_test.go`.
Reverse-order partial-failure cleanup for scope/instance binding remains owned by
`pkg/services/factory_sessions/internal/runtimeopening` and is evidenced in
`models_bind_test.go` plus the Sessions packaging failure guard in
`live_runtime_reverse_order_evidence_test.go`.

Live Factory Session artifact projection follows that placement rule as well.
`pkg/factory/sessions` normalizes checkpoint-derived and runtime artifacts into
Factory-owned `FactoryArtifact` metadata, including capture and redaction
details. `pkg/transports/mapping.WorkflowArtifactsToAPI` performs the generated
OpenAPI conversion only when assembling the public runtime response.

Live Factory Session runtime derivation starts with
`factorysessions.ProjectRuntimeContract`, which returns the session-owned
status, lifecycle, stream identity, progress, usage, Petri, JavaScript,
checkpoint, and artifact projection. Domain callers such as cursor identity and
result shaping consume that value directly. The legacy `ProjectRuntime`
compatibility entrypoint maps it to the generated response and adds the public
stop summary; do not add new runtime derivation to that compatibility adapter.

Factory configuration enum policy remains string vocabulary in
`pkg/factory/contracts`, including runtime-alias normalization and preservation
of unknown values. Generated OpenAPI enum construction and pointer omission
belong in config serialization, `pkg/transports/mapping`, or the HTTP projection
that emits the public value; those boundaries must not expose generated-enum
helpers from the Factory contract owner.

Factory validation results and operational validation failures carry
`pkg/factory/validation.Target` values through domain, service, and control-plane
boundaries. `pkg/transports/mapping` maps those owner-defined targets and result
collections to generated `FactoryValidationTarget` values only while assembling
CLI or HTTP output. Do not expose generated result helpers from the validation
owner or store generated transport targets in Factory Session errors.

Editable Factory-definition validation crosses that boundary with a detached
`FactorySnapshot`. The definition owner validates canonical names and invokes
the validation seam; service and runtime-host composition decode the snapshot
through `pkg/transports/mapping/validationentry`, preserving generated taxonomy
aliases and public topology-error targets without importing transport packages
back into `pkg/factory/definition`.

Editable Factory-definition reads and writes use `FactorySnapshot`,
`FactoryVersion`, `EditableFactory`, and `SaveMode` at the definition owner.
`pkg/transports/mapping/factorydefinition` captures generated submissions and
assembles generated read/save responses; composition injects that adapter
instead of making `pkg/factory/definition` import transport contracts.

Live Factory Session summaries and discovered targets retain
`ScopedLiveSessionSummary`, `Target`, and `TargetRef` as their owner-defined
detached values. Convert those values, including canonical runtime session
identity and optional target names, through
`pkg/transports/mapping/factorysession` when assembling open or list responses;
do not export generated summary or target constructors from the session owner.
Injected home resolution, absolute-path cleanup, folder inspection, and path
equivalence policy belong to the owner-private
`pkg/services/factory_sessions/internal/logicaltarget` capability. Keep only
the plain filesystem effect contracts at the service root so runtime opening,
identity, and discovery cannot reintroduce root behavioral helpers.

Live Factory Session runtime and detail reads follow the same boundary. Derive
`RuntimeProjection` in `pkg/factory/sessions`, then map status, lifecycle,
progress, usage, Petri and JavaScript projections, checkpoints, artifacts,
stream identity, logical target, and stop-summary compatibility in
`pkg/transports/mapping/factorysession`. The Factory Session owner should not
import generated HTTP contracts to expose runtime, summary, or detail helpers.
Control-plane list reads return ordered `ReadProjection` values, preserving an
identity-only fallback when runtime projection fails, and detail reads return
`ProjectionContext` directly. Generated list and detail assembly belongs at the
session-service compatibility edge through `pkg/transports/mapping/factorysession`.

Reconnect sync preflight follows the canonical event boundary as well. The
control plane validates cursors against detached `FactoryEvent` values and
returns the Factory Session-owned `SyncPreflightResult`; only
`pkg/transports/mapping/factorysession` converts that decision and cursor
identity to the generated HTTP response. Runtime hosts should expose
`FactoryEventHistory.CanonicalEvents` to this path rather than round-tripping
history through a generated event union.

Factory Session invocation input follows the same direction. The invocation
owner accepts its detached `InvocationRequest`, including canonical Work
content, structured arguments, request identity, source kind, and timeout.
Service and runtime-host compatibility adapters convert generated
`InvocationRequest` values through `pkg/transports/mapping/factorysession`
before invoking domain normalization, validation, submission, waiting, or
telemetry policy; generated request and Work-content models must not enter the
Factory Session invocation owner.

Construct the runtime-bound invocation owner through
`factory_sessions/services/invocation/wire` from exact Factory-config,
Work-submission, observation, interpolation, Work-Type, input-file, and
telemetry ports. Runtime assembly and root Wire consume the capability contract
and its service-local constructors; they must not select the concrete session
owner or one-shot invocation implementation directly. Construction remains
inert, while timeout and cancellation ownership stays inside the invocation
capability.

Durable Factory Session start, idempotency, lifecycle control, persistence,
restart recovery, result/dispatch/artifact inspection, and canonical event
reads are owned by `factory_sessions/services/durable_execution`. Construct the
capability through its service-local Wire package, retain the concrete engine
behind that private contract, and expose execution to HTTP, invocation, and MCP
runtime views through the outer Factory Sessions `Service`. Runtime composition
may retain an exact mutation-recording callback while assembling the Factory
Runtime, but consumers must not receive the raw durable engine as a parallel
service boundary. The nested durable_execution public surface (`durable_execution`
package root + `durable_execution/wire`) exposes only the named `Service`,
`wire.NewService`, and approved `NewDurable`/`NewStandalone` composition owners;
it must not declare identity, live-runtime, invocation, response-stream, or
runtime-opening constructors. Focused Sessions durable start/resume/control/
inspect call sites route through the bound owner capability (`sessionservice`
`s.durable` and `durableLifecycleHost`) rather than re-reading the host
`DurableExecution()` accessor after construction.

Canonical root Wire imports only the Factory Sessions root, its service-local
`wire` package, and service-owned transport adapters. When Wire must compose a
legacy implementation during structural migration, expose a real service-local
wrapper function and an owner-defined dependency struct; function variables or
type aliases alone are insufficient because generated Wire resolves them back
to the underlying implementation package and recreates the forbidden import.

Factory Sessions supporting implementations belong below
`pkg/services/factory_sessions/internal`, including target discovery, cursor
storage, response-stream storage, runtime binding/hosting, invocation adapters,
session registries, and process-lifecycle policy. When moving one of these
packages, relocate path-keyed quality entries and fixture-relative paths with
the implementation, but delete its `service-root-unexpected-directory`
baseline finding instead of replacing that finding with an internal path.
Owner-local collaborators shared by multiple private capabilities, such as the
live-session registry, should publish their narrow contract from the owning
`internal` package. Migrate private consumers to that contract before the final
atomic removal of the root's consolidated interface-count baseline; removing
root interfaces one at a time replaces the exact deletion-only finding instead
of reducing it.

When promoting a Factory Definitions `Service` alias into a root-declared
interface, move `SessionHost` with it when `AttachFactoryDefinitions` closes
over `Service`, point the local `service.New` constructor at the root types
(not `contracts.*`), and replace the exact `service-root-interface-count`
baseline target for that package. Characterization proof for the seam belongs
in a root-package external test that implements `Service` using only
`pkg/services/factory_definitions` imports.

CUT-DEF-SES publishes `factorydefinitions.DefinitionActivationGateway` at
`pkg/services/factory_definitions/activation_contract.go` as the narrow
Definitions-owned activation requester contract; Sessions aliases it from
`pkg/services/factory_sessions/activation_contract.go` and implements it via
`pkg/services/factory_sessions/internal/sessionservice/definition_activation_gateway.go`,
exposed from `SessionRuntime.DefinitionActivationGateway()` and
`definitionHost.DefinitionActivationGateway()`. Definitions save/activate/swap
paths consume the gateway directly; `SessionHost` no longer carries activation
ports or `AttachFactoryDefinitions`. Runtime opening binds constructed Definitions
into Sessions through `sessionservice.InstallFactoryDefinitions` after
`FactoryDefinitionsFactory` returns, preserving a one-way Sessions-owned edge.
Activation-adjacent tests should pass a dedicated `DefinitionActivationGateway`
fake or the real Sessions gateway instead of embedding activation ports on
`SessionHost`/`Host` test doubles; see `definition/activation_gateway_test.go`,
`definition/activation_sealed_test.go`, and
`sessionservice/definition_activation_peer_integration_test.go`.

When publishing additive CTR-DEF catalog (or later) slices on that root
`Service`, declare plain request/result value types beside the interface,
keep catalog methods on the singular `Service` rather than elevating
`NamedFactoryCatalog` as a peer-facing authority, and extend the same
external fake-peer characterization test with representative success and
distinct typed invalid-name vs missing outcomes (`ErrInvalidNamedFactoryName`
vs `ErrNamedFactoryNotFound`). Validate slices mirror the same pattern in
`definition/validate_equivalence_test.go`: owner-local `newRootValidateServiceForPeer`
construction, `peerExerciseRootValidateSuccess` / `peerExerciseRootValidateTypedFailures`
helpers that accept only `factoryroot.Service`, shared cross-path fixtures
(`CrossPathValidAlphaFactoryJSON` / `CrossPathInvalidFactoryJSON`), distinct
`ErrInvalidFactoryDefinitionPayload` vs `FactoryDefinitionValidationFailure`
with CTR-DEF characterization codes, and effective success via alpha fixture
plus required DEFAULT handling work type. `definition/compile_equivalence_test.go`:
owner-local `newRootCompileServiceForPeer` construction,
`peerExerciseRootCompileSuccess` / `peerExerciseRootCompileTypedFailures`
helpers that accept only `factoryroot.Service`, authored-directory vs canonical
equivalence with merged worker/workstation facts, and distinct
`ErrInvalidAuthoredFactorySource` vs `ErrUnresolvedDefinitionReference`.
Authoring slices similarly stay on the
singular `Service` with prepare/flatten/expand/create/replace request
shapes that omit filesystem effects and mapping codecs; publish
`ErrMalformedFactoryLayoutPayload` and `AtomicFactoryWriteFailure`
(`ErrAtomicFactoryWriteFailed`, `PreviousPreserved`) instead of peer-facing
`FactorySplitLayoutReplaceResult` restore callbacks. Compile slices stay on
the singular `Service` via `CompileEffectiveFactorySource` returning a
Definitions-owned `EffectiveFactorySource` value (not a separately published
peer-facing loader); publish distinct `ErrInvalidAuthoredFactorySource` vs
`ErrUnresolvedDefinitionReference`. Validate slices stay on the singular
`Service` via `ValidateStructuralFactoryDefinition` and
`ValidateEffectiveFactoryDefinition` returning Definitions-owned
`ValidationResult` success shapes (not a peer-facing nested `Validator`
interface); publish distinct `ErrInvalidFactoryDefinitionPayload` vs
`FactoryDefinitionValidationFailure` (`ErrFactoryDefinitionValidationFailed`
with blocking `ValidationTarget` findings and no Petri vocabulary). CLN-DEF-CONTRACTS
story 001 seals owned catalog and validate peer exercise in
`service_root_contract_invariants_test.go` (`peerOwnedDefinitionsConsumer` uses
only `pkg/services/factory_definitions` imports) and documents the owned vs
foreign split in `owned_contract.go` plus the foreign-vocabulary marker in
`contracts_root.go`. Story 002 retargets owner-local Definition consumers
(`definition`, `validation`, `editable`, `service`, owner transports) off
`factory_definitions/contracts` onto the service root; `workers` keeps
`contracts/namevalue` only because the root already imports `workers` and a
service-root import would cycle. `owned_consumer_import_retarget_test.go` and
`definition/cross_path_equivalence_test.go` prove the retarget with import
boundary checks plus catalog/validate behavioral equivalence. CLN-DEF-CONTRACTS
story 003 publishes Factory Event envelope and type vocabulary at
`pkg/services/recordings/event_contract.go`; `events/kinds` and
`event_vocabulary_boundary_test.go` prove event inventory consumers import
Recordings root instead of `factory_definitions/contracts`. Temporary
deletion-only event aliases remain in
`factory_definitions/event_recording_deletion_aliases.go` until downstream
peers finish cutover. CLN-DEF-CONTRACTS story 004 publishes world-state and
replay vocabulary at `pkg/services/recordings/world_state_contract.go` and
`replay_contract.go`, dispatch vocabulary at
`pkg/services/factory_runtime/dispatch_contract.go`, and
`recordings/internal/services/projection_query/internal/service` plus
`recordings/internal` import Recordings root contracts instead of
`factory_definitions/contracts` for those surfaces. Temporary deletion-only
aliases remain in `world_state_recording_deletion_aliases.go`,
`dispatch_runtime_deletion_aliases.go`, and `replay_recording_deletion_aliases.go`.
CLN-DEF-CONTRACTS story 005 publishes worker execution vocabulary at
`pkg/services/workers` (`worker_vocabulary_contract.go`, `WorkstationResult` in
`execution_contracts.go`); provider-session identity remains on
`pkg/services/providers` (`SessionRef` in `identity_contract.go`) and
`pkg/services/workers` (`ProviderSessionMetadata`). `worker_vocabulary_boundary_test.go`
and `recordings/workers_root_boundary_test.go` prove replay and diagnostics
consumers import Workers root ports instead of `factory_definitions/contracts`.
Temporary worker execution deletion-only aliases remain in
`worker_provider_deletion_aliases.go`. CLN-DEF-CONTRACTS story 006 demotes
`NamedFactoryCatalog` and `CurrentFactoryDirectoryResolver` to deletion-only
aliases in `parallel_operation_deletion_aliases.go`; peers use root
`Service.ListNamedFactories`, `ResolveNamedFactory`, `DeleteNamedFactory`,
`GetCurrentFactoryPointer`, and `ResolveCurrentFactoryDirectory` instead.
`parallel_operation_equivalence_test.go` and
`definition/cross_path_equivalence_test.go` prove list/delete/resolve/current
outcomes through root Service match the legacy catalog operations for unchanged
fixtures. Owner HTTP/MCP transports and `definition` remain on the singular
`Service` binding; `pkg/wire` and process-edge CLI/Factory Sessions construction
retain temporary deletion-only aliases until a later cutover packet. CLN-DEF-CONTRACTS
story 007 deletes the public `factory_definitions/contracts` mega-barrel: implementation
types move to `internal/contracts`, shared `namevalue` moves to
`pkg/services/factory_definitions/internal/services/validation/authoredmodel/namevalue`, Recordings and Factory Runtime alias
from the service root, `cmd/pkgboundarycheck` treats the retired public path as
prohibited, and `contracts_mega_barrel_boundary_test.go` seals peer cutover.
parent-private nested validation subservice locks its public surface in
`internal/services/validation/boundary_test.go`: `service.go` exports only
`Service` and `Dependencies` with factory_definitions root request/result
vocabulary and root-injected ports, direct imports avoid Wire/Runtime/
Petri/peer/sibling-lease paths, and `wire/wire.go` constructs from injected
ports without selecting Runtime/Petri implementations or sibling catalog/
authoring_layout/compilation/snapshots_portability/distribution leases.
The parent-private nested compilation subservice locks its public surface in
`internal/services/compilation/boundary_test.go`: `service.go` exports only
`Service` and `Dependencies` with factory_definitions root compile
request/result vocabulary and contracts injected load/encode ports, direct
imports avoid Wire/Runtime/Petri/peer/sibling-lease paths and public
loading/loadedsource/runtimeconfig packages, and `wire/wire.go` constructs from
injected ports without selecting Runtime/Petri implementations or sibling
catalog/authoring_layout/validation/snapshots_portability/distribution leases.
Compilation-owned loading/loadedsource/runtimeconfig implementation lives under
`internal/services/compilation/{loading,loadedsource,runtimeconfig}`; runtime
load/merge characterization coverage lives under
`internal/services/compilation/runtimetests` (not public `loading/runtimetests`);
public
`loading/`, `loadedsource/`, and `runtimeconfig/` remain transitional re-exports
for `pkg/wire` and in-owner callers until DEL-DEF deletes the emptied public
packages; `definition/` stays on the CLN-DEF-FOLD-COMPOSITION lease and must not
change in CLN-DEF-FOLD-COMPILATION. `compile_load_lease_hold_test.go` locks those
residual directories in place, proves `definition/` has no branch diff, and seals
compilation subpackages from importing the lifecycle host.
Factory Definitions `wire/wire.go` composes the lifecycle host through
`factory_definitions/internal` (`NewWithAuthoringLayout` → `internal/lifecycle`)
and must not import public `definition/` or transitional `service/` shims;
`wire/boundary_test.go`, `wire/wire_lifecycle_compose_test.go`, and
`wire/wire_lifecycle_behavior_test.go` lock that construction path and prove
Activate/Save/GetCurrent*/version behavior on the published `Service` root.
New measurable owner packages such as `internal/lifecycle` must also be registered
in both `go-unit-coverage-package-minimums.json` and
`go-functional-coverage-package-minimums.json` when they first appear in CI profiles.
`non_owner_definition_import_boundary_test.go` seals zero
production peer imports of public `definition/` (including peer integration
tests); peers use `factory_definitions` root or `factory_definitions/wire`.
`wire/wire.go` also composes the compilation subservice from the
nested loader ports and delegates `CompileEffectiveFactorySource` on the returned
root `Service`. Bind compilation canonical encode through
`internal/services/compilation/canonical` so owner wire does not import
transport-mapping `factoryconfig` for content identity; authored/canonical decode
and normalize remain on injected loader ports composed from process wire.
Register new compilation production packages with
`go run ./cmd/packagetargetmanifestcheck -write-inventory -write-owner-packages`,
matching retain rows in `docs/internal/baselines/ownership-inventory.json`, and
unit/functional coverage minimums for each nested package path.
Snapshot
slices stay on the singular `Service` via `CaptureFactorySnapshot`,
`PrepareFactorySnapshotImport`, and `MaterializeFactorySnapshot` returning
detached `FactorySnapshot` / `PortableFactorySnapshotFacts` (not
snapshotcapture or bundled-asset implementation types); publish distinct
`ErrInvalidFactorySnapshotPayload` vs `ErrUnsafeFactorySnapshotMaterialize`.
Distribute slices stay on the singular `Service` via
`ListBuiltInPackagedFactories`, `InstallPackagedFactory`, and
`CreateFactoryScaffold` returning shared
`DistributedFactoryDefinitionFacts` for install and scaffold (not
packagedinstallation/scaffold implementation types or peer-facing
`PackagedFactoryInstaller` / `ScaffoldInitializer` authorities); publish
distinct `ErrUnknownPackagedFactoryIdentity` vs
`ErrFactoryDistributeFailed`. Request shapes omit filesystem effects,
output streams, and `PackagedDefinition` payload bytes.
Prefer growing `service_contract.go` / `service_contract_test.go` while
under the 1000-line maint limit. When characterization coverage forces a
split (as with distribute + six-slice seal proof), add a focused sibling
such as `service_contract_distribute_test.go` and ratchet
`backend-package-file-count.json` for that unavoidable growth.
Keep assignability stubs for not-yet-wired CTR-DEF slices on root
`UnimplementedService` (with focused root tests covering typed outcomes
and `AtomicFactoryWriteFailure` / `FactoryDefinitionValidationFailure`
`Error`/`Unwrap`/`Is` paths) and embed that type from
`definition.Service` rather than duplicating stub bodies in the
definition package—otherwise package-coverage minima regress on both
unit and functional lanes.

Retire leaf compatibility packages that only re-export Factory Sessions root
value or function contracts. Same-owner implementations should consume the root
contract when that does not create a cycle; implementation capabilities needed
on both sides of an existing root-to-implementation dependency belong under
`factory_sessions/internal/contracts`, with only the canonical customer-facing
name published at the root.

When retiring root policy helpers or aliases, keep boundary-check inventories
and synthetic guard coverage that prohibit transport use of those names. The
guard remains useful as resurrection protection even after production no
longer declares the symbol; remove only allowlist entries that permitted the
retired root construction shape.

When a runtime-created gateway needs an owner-private capability, retain that
capability on the runtime state and pass the same injected instance into the
gateway. Do not preserve a duplicate root policy function as a fallback for a
missing private service; an incomplete internal construction path should fail
with the existing typed unavailable outcome instead of silently selecting a
second implementation.

Scoped Factory Session inventory source selection, merging, filtering, and
ordering are owner-private policy. Keep detached list request/result values at
the Factory Sessions root, place the policy under `internal`, and declare the
live-reader role at each service-owned transport adapter that consumes it. The
HTTP projection adapter belongs with the HTTP transport rather than the root;
top-level HTTP composition supplies that adapter without publishing transport
collaborator interfaces from the contract root.

Factory Session checkpoint, runtime, result, artifact, and stop-summary
projection policy is owner-private under
`pkg/services/factory_sessions/internal/sessionprojection`. Keep only detached
projection inputs and result values at the service root. When a peer owner
needs one exact projection operation, publish a plain function contract at the
root and construct its private implementation through
`pkg/services/factory_sessions/wire`; root Wire must not import the private
projection package or reimplement its policy.

Live Factory Session record construction is owner-private under
`pkg/services/factory_sessions/internal/livesession`. Runtime and session
assembly code may use that constructor and mutable `LiveSession`/`SessionState`
records after dependencies have been injected. Keep runtime handles,
response-event stores, response streams, checkpoints, and registry mutation
below the owner-private boundary; the service root exposes detached summaries,
projection values, cursors, and narrow operations only. Keep the
package-boundary synthetic denial for the retired root constructor so external
consumers cannot reintroduce a parallel construction path.

Canonical live Factory Session ID selection, default-alias UUID allocation,
and UUID validation are owned by the same private `internal/livesession`
capability. Owner-projected reads carry `ProjectionContext.FactorySessionID`,
and open results carry a detached `ScopedLiveSessionSummary`, so transport
mappers serialize the resolved identity without importing private policy or
re-deriving it from mutable session records.

Live lifecycle results, including their post-control inspection links, are
Factory Session execution contracts. Build those links in
`pkg/factory/sessions/execution` before the dataplane returns its result; the
transport mapper may serialize the result but must not supply domain result
fields back to the dataplane.

Factory Session lifecycle service entrypoints accept the execution owner's
`ControlRequest`, `ApproveRequest`, `RetryDispatchRequest`, and
`InterruptDispatchRequest` values and return `LifecycleControlResult`. Public
request normalization and generated response assembly belong in
`pkg/transports/mapping/factorysession` or the service/runtime compatibility
adapter that invokes it; `pkg/factory/sessions/service` must not import either
generated HTTP contracts or transport mapping for lifecycle control.

Factory Session open and live-read service entrypoints follow the same rule.
`pkg/factory/sessions/service` accepts `OpenRequest` and returns `OpenResult`,
`ReadProjection`, `ProjectionContext`, `SyncPreflightResult`, or JavaScript
result-owner values. The outer service/runtime-host compatibility adapter maps
generated open input before calling the gateway and assembles generated open,
list, detail, reconnect, terminal-result, and partial-result responses only
after the domain call returns.

## Factory Runtime root contract slices

Cross-service Factory Runtime consumers depend on the singular root `Service`
in `pkg/services/factory_runtime` (`interfaces.go`) plus root typed errors in
`composition_contracts.go`. Factory Sessions production packages must import only
`pkg/services/factory_runtime` (not `factory_runtime/javascript`, deleted
`factory_runtime/service`, `engine`, or other nested Runtime paths); lock this with
`pkg/services/factory_sessions/runtime_consumer_import_boundary_test.go` and
`cmd/pkgboundarycheck` peer-subpackage rules. CUT-RUN-REC seals Factory Runtime
production Recordings imports to the service root only via
`pkg/services/factory_runtime/recordings_import_boundary_test.go`
(`TestProductionPackagesImportRecordingsRootOnly`) and behavioral construction
proof in `pkg/services/factory_runtime/recordings_request_boundary_test.go`
(`TestRuntimeConstructsRecordingsCapabilitiesThroughRoot`) plus
`pkg/services/factory_runtime/internal/build_test.go`
(`TestBuild_ConstructsRecordingsRootLedgerAndHostingCapabilities`); behavior
preservation after the cut is locked in
`pkg/services/factory_runtime/recordings_consumer_behavior_preservation_test.go`
(`TestRuntimeRecordingsConsumerBehaviorPreserved`) alongside existing host,
assembly, replay, and execution suites under `internal/host`,
`internal/build_test.go`, `internal/services/instance_host/internal/service`,
and `internal/services/orchestration/runtime`; mirror
`pkg/services/work/recordings_import_boundary_test.go` and
`pkg/services/recordings/runtime_import_boundary_test.go` for the reverse edge.
After DEL-RUN-SERVICE deletes the
transitional `factory_runtime/service` tree, lock the deletion with
`pkg/services/factory_runtime/wire/service_deletion_proof_test.go` (filesystem,
module import scan, ownership-inventory absence, wire construction, and pipeline
package retention proofs). Lock live control and observation
through `pkg/services/factory_sessions/runtime_control_observation_boundary_test.go`
(import scan plus root peer characterization),
`pkg/services/factory_sessions/internal/services/live_runtime/control_observation_boundary_test.go`,
and consolidated gateway proof in
`pkg/services/factory_sessions/internal/sessionservice/runtime_control_observation_boundary_test.go`
(with additional forwarding checks in `lifecycle_test.go`).
Durable and JavaScript execution handoff must construct through Runtime root
workflow, orchestration, and checkpoint contracts
(`JavaScriptWorkflows`, `OrchestrationJavaScriptExecution`,
`JavaScriptCheckpointSummaries`); prefer `internal/testutil/factoryruntimefixtures`
or root-only fakes in Sessions tests instead of importing
`factory_runtime/javascript`. Lock the execution lease with
`pkg/services/factory_sessions/internal/execution/durable_execution_boundary_test.go`
and production-plus-test import scanning in that package's
`TestExecutionLeaseImportsFactoryRuntimeOnlyThroughRoot`. Session opening and
projection must consume Runtime root observation and snapshot shapes
(`Observation`, `StateSnapshot`, `ObserveRequest`/`ObserveResult`) through
`pkg/services/factory_runtime` only; lock the opening/projection lease with
`pkg/services/factory_sessions/internal/sessionprojection/opening_projection_boundary_test.go`
(import scan plus `TestProjectionContextConstructsFromRootObservationAndSnapshot`).
Do not publish a
second peer-facing Runtime authority
(hosting `Lifecycle`/`HostedInstance`, `Factory` run-loop, or
`JavaScriptWorkflows`) for control, observation, dispatch-plan, or checkpoint
slices. Prove each published slice with a colocated `factory_test`
characterization that implements a fake `Service` using only the root package
and approved peer contracts, without importing `factory_runtime/internal`.

Plain control request/result vocabulary is consolidated in `work_move_errors.go`
(`PauseRequest`/`PauseResult`, `ResumeRequest`/`ResumeResult`,
`TerminateRequest`/`TerminateResult`, `WaitToCompleteRequest`/`WaitToCompleteResult`,
`MoveWorkRequest`/`MoveWorkResult`). Peers call `Service` methods
(`ControlPause`, `ControlResume`, `ControlTerminate`,
`ControlWaitToComplete`, `ControlMoveWork`) and branch on root typed errors (`ErrNotRunning`, `ErrNotFound`,
`ErrAlreadyStopped`, `ErrInvalidLifecycleTransition`) and root work-move
errors (`ErrMoveWorkNotFound`, `ErrMoveWorkInFlightDispatch`,
`ErrMoveWorkRequestConflict`). Concrete root methods must classify lifecycle
state at the `Service` boundary so repeated pause/resume returns `NO_OP` and
failures remain matchable with root sentinels. When a consumer-owned adapter
retains a domain-specific error contract, map the root sentinel back at that
adapter rather than leaking the consumer package's sentinel through `Service`.
Do not route control through hosting `Lifecycle.Stop` as the peer authority for
this slice.

Plain observation request/result/value vocabulary lives in
`projection_contracts.go` (`ObserveRequest`/`ObserveResult`,
`Observation`, `ObservationProgress`, `ObservationDispatchSummary`,
`ObservationResultView`, `ObservationResourceView`, `ObservationHealth`).
Peers call `Service.Observe` and branch on
`ErrNotRunning`, `ErrNotFound`, and `ErrInvalidObservationScope`. Do not treat
legacy `GetEngineStateSnapshot` / `StateSnapshot` Petri-shaped aliases or
JavaScript runtime-record types as the peer source of truth for this slice.
`GetEngineStateSnapshot` remains only on the migration-era `APIFactory`
interface; never embed `APIFactory` into the singular root `Service`. Adapters
that still need legacy snapshots must request or explicitly assert
`APIFactory`, while root-slice peer fakes implement `Service` without a legacy
snapshot method.
Concrete sanitized projection from legacy engine snapshots lives under
`factory_runtime/internal/rootobservation` so raw `EngineStateSnapshot` types
stay off the public Runtime package surface enforced by `make pkg-boundary`.
Adding that new production package (any non-test `.go` under a new
`pkg/services/...` directory) also requires regenerating
`docs/internal/packaged-service-structure/package-target-manifest.json` with
`go run ./cmd/packagetargetmanifestcheck -write-inventory -write-owner-packages`
then adding matching retain rows to
`docs/internal/baselines/ownership-inventory.json` (sorted by `packagePath`)
and registering measured packages in both
`docs/internal/baselines/go-unit-coverage-package-minimums.json` and
`docs/internal/baselines/go-functional-coverage-package-minimums.json` so
`ownershipinventorycheck` / Dev Package Prerequisites / `make lint` stay
green. When rebasing orchestration ownership onto main that already landed a
sibling Runtime owner such as `instance_host`, keep both destination package
rows in the shared manifest, ownership inventory, and coverage minimum files
instead of choosing one side of the conflict.
Migration adapter fakes that explicitly implement `APIFactory` should return
`LegacyEngineObservation` (alias of `StateSnapshot`) rather than naming
prohibited Petri public-surface symbols in non-internal packages.

Plain dispatch-plan request/result vocabulary lives in
`execution_contracts.go` (`PlanDispatchRequest`/`PlanDispatchResult`,
`AcceptDispatchResultRequest`/`AcceptDispatchResultResult`,
`DispatchPlanOutcome` including `DUPLICATE_IDEMPOTENT`, and
`DispatchResultOutcome`). Peers call `Service` methods
(`PlanDispatch`, `AcceptDispatchResult`) and branch on root typed errors
(`ErrDuplicateDispatchIntent`, `ErrUnknownDispatchCorrelation`,
`ErrInvalidDispatchResultBoundary`, plus `ErrNotRunning`/`ErrNotFound`). Do not
expose Petri transition objects or Workers construction/implementation types,
and do not require a separate public Dispatch Service for this slice.

Plain checkpoint request/result/value vocabulary lives in
`javascript_checkpoint_contract.go` (`CaptureCheckpointRequest`/`CaptureCheckpointResult`,
`LoadCheckpointRequest`/`LoadCheckpointResult`,
`RestoreCheckpointRequest`/`RestoreCheckpointResult`, `Checkpoint` with opaque
`Payload` bytes, and `CheckpointOutcome`). Peers call `Service` methods
(`CaptureCheckpoint`, `LoadCheckpoint`, `RestoreCheckpoint`)
and branch on root typed errors (`ErrCheckpointNotFound`, `ErrCorruptCheckpoint`,
`ErrIncompatibleCheckpoint`, plus `ErrNotRunning`/`ErrNotFound`). Do not expose
Petri marking snapshots or JavaScript checkpoint strategy types as peer-facing
vocabulary, and do not claim Recordings immutable history ownership from this
slice.

Sealed CTR-RUN root invariants for IMP-RUN unlock live in the root-only peer
characterization in `javascript_child_contract_test.go`: one peer-shaped `Service`
consumer reaches control, observation, dispatch-plan, and checkpoint slices
through the singular root and asserts representative success plus typed
failures using only the published root package (no `factory_runtime/internal`,
Petri, or JavaScript strategy imports). Concrete `factoryImpl` entrypoints for
those slices are consolidated in `runtime/worker_pool.go` (kept out of
`runtime/factory.go` to preserve the backend-size file limit). Nested IMP-RUN
moves, Wire/root, CLI-manifest, provider-conductor, Workers construction, and
OpenAPI package-motion edits remain outside this seal.

## DEL-RUN-ENGINE-PIPELINE pre-start gates

`DEL-RUN-ENGINE-PIPELINE` must not begin leased pipeline deletion or baseline
burn-down until both prerequisite packets are Factory-complete:

| Gate | Branch | Status artifact |
| --- | --- | --- |
| `DEL-RUN-SERVICE` | `pss-del-run-service` | merged PR #1596; `factory_runtime/service` absent; `wire/service_deletion_proof_test.go` |
| `CLN-RUN-FOLD-ENGINE-PIPELINE` | `pss-cln-run-fold-engine-pipeline` | merged PR #1602; fold moved implementation under `internal/services/orchestration` and `instance_host`; story 002 proves public pipeline dirs are gone via `wire/engine_pipeline_deletion_proof_test.go` |

Record the operational gate snapshot in
`docs/internal/processes/del-run-engine-pipeline-prestart-gates.json` and lock
the confirmation with
`pkg/services/factory_runtime/wire/engine_pipeline_prestart_gate_test.go`.
After both gates are Factory-complete, `deletion_hold_active` is `false` and
story 002+ deletion or baseline burn-down may proceed.

## DEL-RUN-ENGINE-PIPELINE test-support internalization

Story 003 moves unexpected public Runtime root test-support into `internal/`:

| Former public path | Internalized path | Proof |
| --- | --- | --- |
| `pkg/services/factory_runtime/testkit` | `pkg/services/factory_runtime/internal/testkit` | `wire/test_support_internalization_proof_test.go` |
| `pkg/services/factory_runtime/exhaustiontests` | `pkg/services/factory_runtime/internal/exhaustiontests` | same proof test; Makefile unit lane uses internal path |

After internalization, update `owner_top_level` unexpected lists (only `testdata` remains at the Runtime root), extend `factoryRuntimeCanonicalRetainRest` / `isFactoryRuntimeCanonicalRetain` for `internal/testkit` and `internal/exhaustiontests`, and refresh ownership inventory + package-target manifest via `go run ./cmd/ownershipinventoryfreeze` and `go run ./cmd/packagetargetmanifestcheck -write-inventory -write-owner-packages`. Consumer tests import `pkg/services/factory_runtime/internal/testkit`.

## DEL-RUN-ENGINE-PIPELINE structure / ownership / package-target baseline burn-down

Story 004 removes deleted public engine/pipeline package paths from the three
structure ledgers and locks the burn-down with
`pkg/services/factory_runtime/wire/engine_pipeline_baseline_gate_test.go`:

| Ledger | Deleted public paths (examples) | Proof |
| --- | --- | --- |
| `docs/internal/baselines/ownership-inventory.json` | `pkg/services/factory_runtime/{build,engine,javascript,runtime,...}` | baseline gate + `engine_pipeline_deletion_proof_test.go` |
| `docs/internal/packaged-service-structure/package-target-manifest.json` | same top-level fold set | baseline gate |
| `docs/internal/baselines/package-structure-baseline.json` | `service-root-unexpected-directory` rows for the same public dirs | baseline gate |

Regenerate ownership inventory and package-target manifest with
`go run ./cmd/ownershipinventoryfreeze` and
`go run ./cmd/packagetargetmanifestcheck -write-inventory -write-owner-packages`
when reconciling shared dual-ledger rows after fold merge. Internalized pipeline
implementation under `internal/services/orchestration` and
`internal/services/instance_host` remains canonical retain debt and is not
removed by this packet. Coverage minimum burn-down for deleted import paths is
owned by story 005.

## DEL-RUN-ENGINE-PIPELINE coverage baseline burn-down

Story 005 removes unit and functional coverage minimum rows for deleted public
engine/pipeline import paths and locks the burn-down with coverage subtests in
`pkg/services/factory_runtime/wire/engine_pipeline_baseline_gate_test.go`:

| Ledger | Deleted public import paths (examples) | Proof |
| --- | --- | --- |
| `docs/internal/baselines/go-unit-coverage-package-minimums.json` | `github.com/portpowered/infinite-you/pkg/services/factory_runtime/{build,engine,javascript,runtime,...}` | baseline gate coverage subtests |
| `docs/internal/baselines/go-functional-coverage-package-minimums.json` | same deleted public import paths, including measurement exceptions | baseline gate coverage subtests |

The fold merge retargeted surviving pipeline implementation to
`internal/services/orchestration` and `internal/services/instance_host`;
story 003 retargeted `testkit` to `internal/testkit`. Those internal import
paths remain canonical coverage debt and are not removed by this packet.

## DEL-RUN-ENGINE-PIPELINE thin root and end-to-end proof

Story 006 locks the thin Runtime root, wire construction, and reduced structure
debt with
`pkg/services/factory_runtime/engine_pipeline_thin_root_proof_gate_test.go`:

| Invariant | Proof |
| --- | --- |
| Root children are only `wire/`, `internal/`, `transports/` plus thin contract files; `testdata` remains the only recorded unexpected public move-debt directory | `canonical_root_directories` + `unexpected_root_children_recorded_as_move_debt_only` subtests |
| Deleted public engine/pipeline directories are absent | `deleted_public_pipeline_directories_absent` subtest + `wire/engine_pipeline_deletion_proof_test.go` |
| Public `testkit` / `exhaustiontests` internalized | `canonical_root_directories` + `wire/test_support_internalization_proof_test.go` |
| `factory_runtime/wire` constructs published `Service` and exercises Observe / PlanDispatch / ControlPause | `wire_constructs_published_control_observation_dispatch` subtest |
| `service/` deletion remains owned by DEL-RUN-SERVICE; `checkpoint_recovery` undisturbed | `service_directory_absent` + `checkpoint_recovery_undisturbed` subtests |
| Deleted pipeline paths are absent from structure/ownership ledgers for this owner | `package_structure_baseline_omits_deleted_public_pipeline_directories`, `package_target_manifest_omits_deleted_public_pipeline_packages`, and `ownership_inventory_omits_deleted_public_pipeline_packages` subtests |

Sibling proof tests in `pkg/services/factory_runtime/wire/` and
`packaged_root_shape_test.go` remain the focused owners for pre-start gates,
baseline burn-down, and packaged-root shape drift.

## DEL-DEF-RESIDUAL pre-start gates

`DEL-DEF-RESIDUAL` must not begin leased residual transitional package deletion
or baseline burn-down until residual fold packets and
`INV-DEF-INVOCATION-POLICY` are Factory-complete and live `DEL-DEF` is terminal
or no longer owns overlapping deletion leases:

| Gate | Branch | Status artifact |
| --- | --- | --- |
| `INV-DEF-INVOCATION-POLICY` | `pss-inv-def-invocation-policy` | merged PR #1605; `internal/services/invocation_policy`; `wire/invocation_policy_test.go` |
| `CLN-DEF-FOLD-CATALOG` | `pss-cln-def-fold-catalog` | merged PR #1608; `internal/services/catalog` |
| `CLN-DEF-FOLD-COMPILATION` | `pss-cln-def-fold-compilation` | merged PR #1607; `internal/services/compilation` |
| `CLN-DEF-FOLD-COMPOSITION` | `pss-cln-def-fold-composition` | merged PR #1606; `internal/lifecycle` |
| `CLN-DEF-FOLD-VALIDATION` | `pss-cln-def-fold-validation` | merged PR #1610; `internal/services/validation` |
| `CLN-DEF-FOLD-SNAPSHOTS` | `pss-cln-def-fold-snapshots` | merged PR #1613; `internal/services/snapshots_portability` |
| `CLN-DEF-FOLD-DISTRIBUTION` | `pss-cln-def-fold-distribution` | merged PR #1611; `internal/services/distribution` |
| `CLN-DEF-FOLD-INVOCATION-POLICY` | `pss-cln-def-fold-invocation-policy` | merged PR #1615; `internal/services/invocation_policy` |
| `DEL-DEF` (serialization) | `pss-del-def` | merged PR #1603; overlapping TOPLEVEL paths remain DEL-DEF-owned until that packet deletes them |

Record the operational gate snapshot in
`docs/internal/processes/del-def-residual-prestart-gates.json` and lock the
confirmation with
`pkg/services/factory_definitions/del_def_residual_prerequisite_gate_test.go`.
While `DEL-DEF` remains in-flight, `deletion_hold_active` is `true` and story
002+ residual deletion or baseline burn-down must wait.

## DEL-DEF-RESIDUAL baseline burn-down gates

Story 003 lowers structure, ownership, and package-target baselines for the
deleted residual transitional public packages. Lock the ledger absence proof in
`pkg/services/factory_definitions/del_def_residual_baseline_gate_test.go` after
`ownership-inventory.json`, `package-target-manifest.json`, and
`package-structure-baseline.json` no longer list those deleted paths.

## DEL-DEF-RESIDUAL coverage baseline burn-down

Story 004 removes unit and functional coverage minimum rows for deleted residual
transitional public import paths and dedupes story-002 retarget duplicates in
`docs/internal/baselines/go-unit-coverage-package-minimums.json` and
`docs/internal/baselines/go-functional-coverage-package-minimums.json`. Lock the
burn-down with coverage subtests in
`pkg/services/factory_definitions/del_def_residual_baseline_gate_test.go`:

| Ledger | Deleted public import paths (examples) | Proof |
| --- | --- | --- |
| `docs/internal/baselines/go-unit-coverage-package-minimums.json` | `github.com/portpowered/infinite-you/pkg/services/factory_definitions/{namedpaths,loading,decisionenvelope,packages,...}` | baseline gate coverage subtests |
| `docs/internal/baselines/go-functional-coverage-package-minimums.json` | same deleted public import paths, including measurement exceptions | baseline gate coverage subtests |

Internal `internal/services/*` import paths remain canonical coverage debt and
are not removed by this packet.

## DEL-DEF-RESIDUAL root shape and wire proofs

Story 005 locks the post-deletion packaged-service root shape, wire import
hygiene, published Service construction, and invocation_policy contract reachability
after residual transitional public packages are removed:

| Proof | Artifact |
| --- | --- |
| Deleted residual and DEL-DEF transitional top-level packages absent | `deleted_residual_transitional_top_level_packages_absent` and `deleted_del_def_transitional_top_level_packages_absent` subtests |
| Canonical retain children (`internal`, `transports`, `wire`) plus committed move debt only | `canonical_root_directories_present`, `unexpected_children_remain_move_debt_only` |
| Parent-private `internal/services/*` subservices remain | `internal_services_subservices_remain` |
| `factory_definitions/wire` avoids deleted transitional imports | `TestDelDefResidualRootShape_WireDoesNotImportDeletedTransitionalPackages` |
| Published catalog/compile/validate/snapshot surfaces construct through wire | `TestDelDefResidualRootShape_WireConstructsPublishedServiceSurfaces` |
| Invocation policy contracts reachable through Definitions wire | `TestDelDefResidualRootShape_InvocationPolicyContractThroughDefinitionsWire` |

Deeper fold-behavior preservation across catalog, authoring, validate,
snapshot/portability, and distribute slices remains in
`pkg/services/factory_definitions/wire/fold_behavior_preservation_test.go`.

## DEL-WRK baseline burn-down gates

Story 003 lowers structure, ownership, package-target, and coverage baselines for
deleted Workers transitional packages. Lock the ledger absence proof in
`pkg/services/workers/del_wrk_baseline_gate_test.go` after
`ownership-inventory.json`, `package-target-manifest.json`,
`package-structure-baseline.json`, and both `go-*-coverage-package-minimums.json`
no longer list deleted paths (except the held-back `executor/agentrun` shim).

Regenerate ledgers with `go run ./cmd/ownershipinventoryfreeze`,
`go run ./cmd/packagetargetmanifestcheck -write-inventory -write-owner-packages`,
and sync `package-structure-baseline.json` from current `pkgstructurecheck` drift
when CLN contract-root renames change exported-function baseline rows.

## DEL-WRK delete-ready inventory

Story 001 records the confirmed delete-ready set for emptied Workers transitional
packages after CLN-WRK-* consumption:

| Artifact | Purpose |
| --- | --- |
| `docs/internal/processes/del-wrk-delete-ready-inventory.json` | Canonical delete-ready, held-back, and excluded paths for DEL-WRK |
| `pkg/services/workers/del_wrk_delete_ready_inventory_gate_test.go` | Locks CLN-WRK prerequisites, shim-only delete-ready paths, zero-import proof, held-back callers, Providers extraction exclusions, and `internal/services/*` retention |
| `pkg/services/workers/legacy_fold_boundary_test.go` | Existing folded-legacy import boundary and shim-only characterization used as CLN-WRK-LEGACY-PACKAGES observable proof |

Delete-ready paths must contain only `shim.go` and have zero module importers
before story 002 deletes them. Held-back paths still have production or test
imports, or (for `service/`) remain multi-file owner-local compile shims.
Providers extraction sources (`provider/`, `provider_test/`, `agypty/`,
`cliprovider/`) and all `internal/services/*` subservices stay excluded from
deletion until later IMP-PROV absorption or separate packets.

## DEL-WRK root shape gates

Story 004 proves the Workers root matches canonical shape plus thin contracts
after transitional shim deletion. Lock the proof in
`pkg/services/workers/del_wrk_root_shape_test.go`:

| Proof | Test |
| --- | --- |
| Deleted transitional packages absent | `TestDelWrkRootShape_CompletionInvariants/deleted_transitional_packages_absent` |
| Canonical `internal/` + `wire/` present | `TestDelWrkRootShape_CompletionInvariants/canonical_root_directories_present` |
| Providers extraction sources retained | `TestDelWrkRootShape_CompletionInvariants/providers_extraction_sources_remain` |
| `internal/services/*` subservices retained | `TestDelWrkRootShape_CompletionInvariants/internal_services_subservices_remain` |
| Thin root contract inventory sealed | `TestDelWrkRootShape_CompletionInvariants/thin_root_contract_inventory_sealed` |
| Top-level children are canonical or committed move debt only | `TestDelWrkRootShape_UnexpectedChildrenRemainMoveDebtOnly` |
| `workers/wire` avoids deleted transitional imports | `TestDelWrkRootShape_WireDoesNotImportDeletedTransitionalPackages` |
| Wire constructs published Workers root | `TestDelWrkRootShape_WireConstructsPublishedRoot` |
| Runtime-assembly, workstation, and runner paths reachable | `TestDelWrkRootShape_RuntimeAssemblyWorkstationAndRunnerPathsRemainReachable` |

Deeper wire construction characterization remains in `wire/wire_test.go` and
`wire/construction_boundary_test.go`.
