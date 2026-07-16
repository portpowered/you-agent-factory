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
| Platform infrastructure | target `pkg/platform` | Put logging, replay/artifact infrastructure, metrics, cursor storage, and non-domain clocks in the collapsed platform owner. Until Batch 006 moves a slice, use its registered narrow migration root; never put domain policy in platform code. |
| Transport boundaries | target `pkg/transports` | Put HTTP, CLI, MCP, generated transport contracts/clients, and boundary mapping at the process edge. Until Batch 006 moves a slice, use its registered migration root; transport adapters must not own domain policy. |
| Process startup and dependency construction | `cmd/factory`, target `pkg/root`, target `pkg/wire`, and `pkg/initializer` | Keep `cmd/factory` thin, normalize process input and select mode in `pkg/root`, expose one concrete graph constructor in `pkg/wire`, and execute startup/shutdown lifecycle for already-built transports and sidecars in `pkg/initializer`. Construction-phase callback bundles are test harnesses, not a public composition API. Compose the runtime core through the generated `pkg/wire` application set, copy caller-owned config before normalization, project only narrow domain contracts into the graph, and retain cleanup ownership for the startup bundle rather than exposing `runtimehost.Host` or the bridge core. Stateful collaborators such as durable Factory Session execution must be constructed once per graph with the graph's normalized roots, clock, and runtime dependencies, then injected into compatibility facades rather than reconstructed there. `pkg/initializer` must not expose config-based session, runtime-host, API, CLI, or MCP constructors; it accepts the completed graph and owns lifecycle only. A fallible graph phase should retain any closeable construction resource so `pkg/wire` can unwind acquired resources once, in reverse order, before returning a later phase failure. Initializer should record only successfully started collaborators, stop them in reverse order on partial failure or shutdown, and make graph close part of the same idempotent shutdown result. |

Production command runners must remain blocking without taking lifecycle ownership
back from `pkg/initializer`. The entrypoint should construct and start the graph
through `pkg/root`, then let the returned application wait for its selected
graph-owned transport and perform the same idempotent reverse-order shutdown.
`initializer.BuildCore` must require the valid worker application composed at
that outer boundary; it must not fill in a missing component with production
defaults before loading configuration or starting lifecycle work.
Production-shaped functional runs replace process side effects through the typed
`pkg/wire.FunctionalEdges` input. Apply those edges to an invocation-local config
copy before calling the shared application builders; do not add CLI flags,
package globals, untyped dependency bags, or test-side service-config mutation.
The zero-value edge input must retain production defaults.
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
though it does not activate run sidecars. Fixture-backed MCP may construct its
narrow fake execution edge, but runtime-backed MCP must construct and retain
the completed `wire.Graph`, including its registry, persistence, runtime-build,
durable-execution, and startup-bundle cleanup ownership. Construct the MCP
stdio lifecycle from the request's explicit reader and writer, and let
`pkg/initializer` start, wait for, stop, and close that graph. Runtime-backed
CLI session execution follows the same rule by retaining its complete graph in
the returned closable execution owner; it must not discard the graph after
extracting durable execution. The CLI command boundary must retain that owner
for the full workflow or durable-list operation and close it exactly once after
both successful and failed command execution. Do not return a separately
composed MCP application from the process graph builder.

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

Construct `pkg/service/runtimebuild.Service` with an explicit clock, logger, and
runtime bundle builder, and propagate its constructor error before initializer
lifecycle begins. Before building any session runtime, application composition
must attach the graph-owned durable execution service's Petri mutation recorder;
that preserves one canonical Factory Session event and snapshot owner across
startup, replacement, and resume builds.

The shared Wire provider set constructs persistence, durable execution, and the
recorder-configured runtime-build service before passing completed collaborators
to `composebridge.ComposeCore`. Root one-shot invocation and the legacy
`InjectFactoryService` facade both adapt that same runtime core; neither path may
re-enter `BuildFactoryService` to create a replacement session foundation.

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
not call canonical event builders or import `pkg/sessionpersistence` directly.
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
