# Build-Time/Runtime Composition Plan

Status: final P3-P7 packet reconciliation complete; ready for review.

Audit snapshot: 2026-08-14, with the historical throttled-lane behavior guard
at 1be29c60d. The authoring branch was synchronized to origin/main at
165bf33a1 before the P6/P7 review pass; current symbols and behavior on that
tree are the implementation evidence. The historical packet sources are
retained for their row semantics and are not silently rewritten.

## Substantially delivered queued packets

The following queued packets are materially delivered and must not be
re-authored as new work in P3-P7:

- P0 characterization is merged as PR #1854.
- P1 root injection and owner-contract work is merged as PR #1857.
- P2B shared Work-root work is merged as PR #1910.
- P2C Definitions, Runtime, and Automations root work is merged as PR #1915.

P2A has no packet, PR, branch, or accepted anchor in the repository or the
GitHub BTRC search results inspected for this audit. It is therefore recorded
as NOT DELIVERED below. Absence of a P2A packet is not permission to infer its
scope from later P2B or P2C changes.

This status is about delivery of the queued packet, not completion of the
broader packaged-service decomposition. The remaining-work audit still
identifies service-root, nested-package, transport, mapping, event, and
functional-test work after P2C.

## Audit method and authority

The audit reconciles these sources before defining a later packet:

1. runtime-opening-ownership-matrix.md, the P0 characterization and
   late-opening ownership matrix;
2. btrc-p1-owner-contracts.md, including its deletion register and owner-port
   rules;
3. packaged-service-restructure-remaining.md, the current remaining-violations
   and decomposition audit;
4. workers-stateless-execution-composition-plan.md, the sibling execution
   plan used as the shape and verification reference for later packets;
5. package-target-manifest.json and
   docs/internal/projects/packaged-service-structure/path-lease-packet-manifest.json,
   which identify package ownership and shared-path conflicts; and
6. docs/architecture/packaged-structure.md,
   docs/architecture/architecture.md, docs/architecture/structures.md, and
   docs/architecture/data-model.md, which define the public composition
   vocabulary and boundaries.

The matrix says it was reconciled against main at 558e9ac47 on 2026-08-10.
That snapshot is stale relative to the audit snapshot above. Its row
classification and ownership assignments remain the source for interpreting
the P0/P1 opening inventory, while current origin/main symbols and tests are
the source for present status. The 1be29c60d movement is behaviorally relevant:
it reconciles throttled in-flight projections with active dispatch IDs and
observed terminal results. A future packet must preserve that guard.

The remaining-work audit also has a documented tree disagreement: its older
summary table describes Recordings as a multi-root inventory, while its newer
notes and the current tree describe one public Recordings Service with private
internals. The current tree wins for present evidence; the older count is kept
only as historical audit context.

## Current public composition baseline

The current production construction path is:

    cmd/factory -> pkg/root.BuildProcess -> pkg/wire -> constructed roles
    -> pkg/initializer

The boundary is evidenced by:

- pkg/root/process.go: BuildProcess validates the context and delegates once
  to wire.InjectBundle; BuildStatelessWorkers is the separate detached-worker
  composition boundary.
- pkg/wire/wire.go: BundleSet composes canonical service roots, the
  Factory Sessions runtime-opening owner, the application service, the
  initializer, and the reusable process. InjectBundle is the application
  injector.
- pkg/initializer/application/process.go: Process owns the reusable command
  factory and lifecycle handle; Execute creates an invocation context and Close
  delegates lifecycle shutdown.
- pkg/initializer/application/entrypoints.go: Initializer owns application
  opening and system initialization entrypoints.
- pkg/initializer/lifecycle/manager.go: Manager owns activation ordering,
  cancellation handling, reverse cleanup, and cleanup-error joining.

The canonical dependency graph is inert until the initializer opens the
selected application. Runtime/session state is not constructed by a transport
or by a second application builder.

The caller inventory relevant to future seam moves is:

| Caller | Current path | Contract that must remain observable |
| --- | --- | --- |
| cmd/factory/main.go:runProcess | root.BuildProcess, Process.Execute, Process.Close, processExitCode | terminal output, cancellation classification, close-error joining, and exit code |
| cmd/clicontractsmoke/main.go:run | root.BuildProcess and Process.Execute | public command invocation through the production root |
| cmd/retiredsurfacecheck/main.go | root.BuildProcess and Process.Execute | compatibility surface remains inert and callable |
| tests/functional/internal/support/process.go:BuildProcess | root.BuildProcess used by functional fixtures | functional tests exercise the same process boundary |
| tests/functional/internal/support/root_run_host.go | root.BuildProcess and Execute | host-backed functional lifecycle and response behavior |
| pkg/wire tests | wire.InjectBundle and canonical composition helpers | construction remains lazy, shared where intended, and isolated where required |

The path-lease manifest identifies PSS-I01 root/Wire/process, PSS-I02 HTTP,
PSS-I03 CLI, PSS-I04 MCP, and PSS-I05 event-backbone work as shared or blocked
integration surfaces. Later packets must name those owners instead of silently
moving a shared caller from one package family to another.

## P0-P2C delivery status

Each row has exactly one delivery status. File-and-symbol evidence is included
so the classification can be reproduced from the tree and the merged history.

| Packet | Status | Evidence |
| --- | --- | --- |
| P0 characterization | DELIVERED | PR #1854 is merged. The source matrix is runtime-opening-ownership-matrix.md. Characterization evidence includes tests/functional/factory/batch/batch_characterization_test.go:TestBTRCP0BatchSuccessCharacterization, tests/functional/workstations/poller/hosted_characterization_test.go:TestBTRCP0HostedServiceSuccessCharacterization, pkg/services/recordings/internal/replay/runtime_decoder_test.go:TestBTRCP0ReplayCharacterization_ReconstructsCheckedInSuccess, tests/functional/factory/invocation/characterization_test.go:TestBTRCP0OneShotSuccessCharacterization, tests/functional/providers/acp/btrc_p0_characterization_test.go:TestBTRCP0ACPTargetSuccessCharacterization, and pkg/services/factory_runtime/internal/services/orchestration/javascript/runtime/host_children_test.go:TestBTRCP0DirectJavaScriptSuccessCharacterization. Cancellation, isolation, and typed-divergence cases are also covered by the companion P0 tests in those packages. |
| P1 root injection and owner contracts | DELIVERED | PR #1857 is merged. btrc-p1-owner-contracts.md records the deletion register. Current composition evidence is pkg/root/process.go:BuildProcess, pkg/wire/wire.go:BundleSet and InjectBundle, pkg/initializer/application/process.go:Process, and pkg/initializer/lifecycle/manager.go:Manager. Boundary and lifecycle behavior is exercised by pkg/wire/runtime_inputs_test.go, pkg/wire/session_runtime_providers_test.go:TestProcessCloseContinuesThroughEveryLifecycleOwnerAfterFailure, pkg/initializer/application/builders_test.go:TestRuntimeRunnerBuilderConsumesNeutralLifecycleOpening, and tests/functional/sessions/root_composition/process_reuse_inert_test.go:TestRootBuildProcessIsInertAndReusableAcrossFactorySessions. |
| P2A | NOT DELIVERED | No P2A packet, PR, branch, issue, or accepted anchor was found in the checked-in BTRC history or GitHub searches. No file-and-symbol implementation claim is made for this row. P2A remains a missing historical packet, not an inferred implementation milestone. |
| P2B shared Work root | DELIVERED | PR #1910 is merged. Current Work ownership is pkg/services/work/service_contract.go:Service. Recording scope and lifecycle behavior is evidenced by pkg/services/recordings/lifecycle_capability.go:RecordingLifecycle and pkg/services/recordings/internal/canonical_recording_lifecycle_test.go:TestRecordingScopesBeginAppendFlushFinalizeAndClose. Boundary use is covered by pkg/services/work/recordings_request_boundary_test.go:TestWorkConstructsRecordingsRequestsThroughRoot and pkg/services/factory_sessions/work_invocation_boundary_test.go. The root composition path is covered by pkg/root/root_test.go:TestBuildProcessReusesCanonicalRootsAcrossTwoIsolatedExecutions. |
| P2C Definitions, Runtime, and Automations roots | DELIVERED | PR #1915 is merged. Runtime activation is exposed by pkg/services/factory_runtime/runtime_activation_contract.go:RuntimeActivationRequest and RuntimeActivationOperation. Automations exposes one root Service in pkg/services/automations/contracts.go and runtime lifecycle values in pkg/services/automations/runtime_lifecycle_contract.go. Behavioral evidence includes pkg/services/factory_runtime/wire/runtime_activation_test.go:TestRuntimeRootActivationPublishesOnlyDetachedSuccessfulState, pkg/services/factory_runtime/wire/runtime_activation_test.go:TestRuntimeRootDeactivationRetainsStateUntilCleanupSucceeds, pkg/services/automations/internal/runtime_lifecycle_test.go, and tests/functional/factory_runtime/root_composition/workflow_orchestration_activation_test.go. The merged packet also removed the obsolete replay lifecycle binding path and routes active runtime behavior through the root boundary. |

## Current behavioral coverage

The existing guards cover the major observable seams that the audit can
attribute before P3-P7 authoring:

| Behavior | Current guard | Coverage interpretation |
| --- | --- | --- |
| Inert reusable process and shared canonical roots | pkg/root/root_test.go:TestBuildProcessReusesCanonicalRootsAcrossTwoIsolatedExecutions and tests/functional/sessions/root_composition/process_reuse_inert_test.go:TestRootBuildProcessIsInertAndReusableAcrossFactorySessions | Root construction is reusable without leaking invocation or session state. |
| Lifecycle ordering, cancellation, and cleanup | pkg/initializer/lifecycle/manager_test.go:TestManagerRunsAndClosesInReverseOrder, TestManagerUnwindsStartFailureAndJoinsCleanupErrors, and TestManagerUsesSameShutdownPathForCancellationAndRunnerFailure | Activation and reverse unwind are directly observable at the lifecycle owner. |
| Runtime activation and deactivation | pkg/services/factory_runtime/wire/runtime_activation_test.go and tests/functional/factory_runtime/root_composition/workflow_orchestration_activation_test.go | Detached successful state, duplicate identity, failed-start cleanup, and retry behavior are guarded. |
| Recording order, scope isolation, stale/foreign references, and replay | pkg/services/recordings/internal/canonical_recording_lifecycle_test.go and pkg/services/recordings/internal/projection_query_contract_test.go | Canonical ledger and scope behavior are protected at the service boundary. |
| ACP delegation, duplicate delivery, busy control, and terminal failure | tests/functional/chat_sessions/root_composition/acp_prompt_delegation_test.go and tests/functional/transport/acp/stdio/cli_serve_acp_controls_test.go | The ACP caller has observable root/session and control-flow coverage. |
| HTTP startup unwind and concurrent request isolation | tests/functional/transport/http/server/startup_shutdown_test.go and tests/functional/transport/http/server/concurrent_requests_test.go | HTTP lifecycle and request isolation are covered, although later transport packets still need a complete response corpus. |
| Throttled in-flight projection and termination race | pkg/services/factory_runtime/internal/rootobservation/project_test.go, pkg/services/factory_sessions/internal/sessionprojection/projection_test.go, pkg/services/factory_runtime/internal/services/orchestration/subsystems/terminationtests/termination_test.go:TestTerminationCheck_DoesNotTerminateWhileObservedResponseAwaitsRetirement, and tests/functional/runtime_api/api_provider_throttle_pause_observability_test.go:TestProviderErrorSmoke_ThrottleFailureIsolatesOtherLaneThroughPublicSession | This is current-main behavior from 1be29c60d and must remain part of any later runtime/session seam move. |

The P0 characterization suite remains the baseline for success, provider
failure, protocol failure, child failure, source failure, cancellation, and
concurrent isolation. It is a behavior baseline, not an inventory test.

## Caller behavior that still needs a guard

These are gaps or partial seams identified for the later packets. They are
listed as observable behaviors to characterize, not as permission to change
production code in this audit story.

| Caller behavior | Evidence found | Guard still needed |
| --- | --- | --- |
| Full factory CLI lifecycle from root construction through Execute, Close, and exit classification | cmd/factory/main.go:runProcess implements the path; cmd/factory/main_test.go covers processExitCode and delegation but does not execute the complete runProcess lifecycle with close-error joining | Add a caller-level characterization before CLI ownership or runtime-opening seams move. Preserve cancellation versus invocation failure and cleanup-error semantics. |
| Detached stateless-worker behavior through its public construction boundary | pkg/services/workers/wire/stateless_execute_test.go:TestNewServiceExecuteDetachedAgentRunPreservesGoalDecisionEnvelope and pkg/wire/session_runtime_providers_test.go:TestCanonicalStatelessWorkersExecuteBeforeRuntimeOpening cover service/wire portions | Add an end-to-end caller guard for root.BuildStatelessWorkers before WSE or transport cutover changes the detached path. |
| Multi-part Work content through a public terminal response | pkg/services/recordings/internal/projections/workstation/workstation_requests_test.go covers nil and multi-part projection fixtures; pkg/services/work/transports/mcp/schemas.go defines multi-part content | Add cross-boundary characterization from Work content to the relevant HTTP/MCP/CLI response before moving mapping or transport ownership. |
| One response/terminal contract across every public caller | CLI, HTTP, and ACP each have focused tests, but no single pre-move corpus ties all caller projections to the same event and terminal facts | The P5 transport packets must add caller-specific guards before deleting or relocating adapters; the guard must assert observable payloads and terminal events rather than source inventories. |

The gaps do not invalidate the delivered P0-P2C statuses. They identify the
behavioral guard work required before a later structural move can claim
independent safety.

## Sequencing constraints for P3-P7

The audit establishes the constraints for the remaining packets. Story 002
adds the P3 and P4 packets below; it does not claim that either packet has been
implemented in the current tree.

- Preserve the customer-facing vocabulary in
  docs/architecture/data-model.md. Internal Petri-net terms may remain private
  implementation vocabulary.
- Keep pkg/root.BuildProcess as the caller-facing construction boundary,
  pkg/wire as the canonical inert graph, and pkg/initializer as the lifecycle
  owner.
- Treat P2A as an explicit missing packet. Do not manufacture a retroactive
  anchor or claim that P2B/P2C delivered P2A.
- Use the runtime-opening matrix for historical row ownership but revalidate
  every affected symbol against current main, including the throttled
  in-flight projection behavior introduced by 1be29c60d.
- Reconcile the P1 deletion register with the current tree before deleting
  compatibility paths. A compatibility method retained for unmigrated callers
  is not a completed deletion.
- Respect the path-lease ownership of PSS-I01 through PSS-I05 when a packet
  crosses root, HTTP, CLI, MCP, or event surfaces.
- Require behavior-focused tests at the caller or service boundary for every
  seam move. Do not add tests that merely scan inventories, source text, route
  lists, or asset internals.
- Each later packet must state its build changes, caller migration, deletion
  register, behavioral guard, CI tier, and sizing. The final reconciliation
  story must compare those claims with the actual tree and merged history.

## P3 — Make Factory Runtime the runtime owner

### Outcome and preserved behavior

P3 moves the runtime authority, not the customer-facing Factory Session model.
After P3, one process-scoped `factoryruntime.Service` owns every live Runtime
identified by an opaque `RuntimeID` (or an opaque
`{FactorySessionID, Generation}` binding). Factory Runtime owns activation of a
resolved definition, Work submission after Work admission, scheduling,
dispatch correlation, pause/resume/termination, context-aware waiting, and
orchestration-neutral observation. Factory Sessions keeps the customer-facing
session identity and lifecycle decisions and calls Runtime with values and
opaque IDs.

The preserved customer behavior is: admitted Work reaches the same terminal
success, failure, cancellation, or timeout outcome; pause and resume do not
lose buffered submissions or worker results; a Runtime that fails during
activation unwinds acquired resources before reporting failure; and the
throttled in-flight lane from current main remains observable until its
dispatch and terminal result retire. Runtime does not become the canonical
Factory Event ledger: Recordings remains the event owner, and Runtime emits or
consumes event facts through the existing owner port.

This is a seam move because the tree already publishes a partial
`factoryruntime.Service` in `pkg/services/factory_runtime/interfaces.go`, but
Factory Sessions still depends on `APIFactory`, `Factory`, `HostedInstance`,
`HostedHandle`, `Lifecycle`, `Sidecars`, `ReplacementBuilder`, legacy snapshot
access, and runtime-opening callback injection. The target is the Runtime
boundary described by `package-service-factory-runtime.md`, reconciled with the
P1 deletion register and the current `pkg/services/factory_runtime/wire` tests.

### Build first

Build the replacement in this order while the old path remains available only
as a delegating compatibility edge:

1. Publish detached Runtime request/result values for `Activate`, `SubmitWork`,
   `Pause`, `Resume`, `Terminate`, `Wait`, `Observe`, and
   `AcceptDispatchResult`. Every request carries an opaque Runtime/session
   identity and correlation values; it carries no Petri net, token, hosted
   instance, filesystem, logger, callback, or transport stream.
2. Make the `factoryruntime.Service` implementation process-scoped and inert
   in `pkg/services/factory_runtime/wire`. Private Runtime state is keyed by
   the opaque identity and owns the runtime instance, dispatch outbox,
   cancellation, and terminal-retirement state. `pkg/wire` constructs this
   root once; opening a session invokes it with explicit values.
3. Add one narrow compatibility adapter over the existing hosted engine. The
   adapter may translate old `Factory`/hosting results while callers migrate,
   but new code must not be added to that adapter or to `APIFactory`.
4. Keep event reads and replay on `recordings.Service`; if a Runtime caller
   needs history, migrate it to the Recordings root rather than adding history
   methods to the new Runtime contract.

The target is not a second Wire graph. `pkg/root.BuildProcess` remains the
caller-facing construction boundary, `pkg/wire.InjectBundle` remains the one
canonical production graph, and `pkg/initializer` still starts and unwinds
already-constructed roles.

### Caller-by-caller migration

The following are the known production caller families in the audit snapshot.
Tests beside each caller move with it; a caller not listed here must be added
to the packet before the seam is changed.

| Caller | Current seam | Migration order and successor |
| --- | --- | --- |
| `pkg/wire/wire.go`, `pkg/wire/session_runtime_providers.go`, and `pkg/services/factory_runtime/wire/wire.go` | Runtime assembly/factory providers construct the hosted graph and expose it to opening code. | Build the one inert Runtime root first; make Wire pass the root and its narrow owner ports. Remove any second Runtime constructor after the root provider is live. |
| `pkg/services/factory_sessions/internal/runtimeopening/open.go`, `runtime_activation.go`, `types.go`, and `runtime_assembler.go` | Opening accepts `HostedInstance`, `ReplacementBuilder`, `Lifecycle`, `Sidecars`, and casts the Runtime root to `APIFactory`. | Resolve Definitions values, call `Runtime.Activate`, retain only the returned opaque binding, and route cleanup through the Runtime operation. The private `instance_host` implementation may remain, but it is no longer a Sessions contract. |
| `pkg/services/factory_sessions/internal/sessionservice/runtime_sessions.go`, `runtime_gateway.go`, `runtime_projection.go`, and `runtime_control_observation_boundary_test.go` | Sessions proxies `ObserveForSession` and controls through a session-bound Runtime object. | Convert session controls and observations to Runtime root requests with the stored opaque binding. Keep session projection and response selection in Sessions; delete the proxy once all transport callers use the owner root. |
| `pkg/services/factory_sessions/internal/services/live_runtime` and `internal/services/durable_execution` | `SessionFactory`, `BindWorkerInvoker`, and callback-based progress wiring recover or inject runtime execution state. | Runtime accepts worker results and owns dispatch correlation; Sessions passes complete request values. Remove callback injection after the caller characterization and retain only a private observation/result handoff. |
| `pkg/transports/mapping/composition/services.go`, `http_binding.go`, and `factory_status.go` | Mapping accepts `factoryruntime.APIFactory` and falls back from `Service` to legacy observation. | Map status/control/observation directly from the Runtime root. Canonical event reads move to Recordings in P5B; no new legacy fallback is allowed. |
| `pkg/services/factory_runtime/transports/cli`, `transports/http`, and `transports/mcp` | These adapters already consume `factoryruntime.Service` for the published control/observation slices, while some observation handlers still accept a Sessions proxy. | Keep them on the Runtime root, add the opaque identity to requests, and preserve typed error and response mapping. Move the remaining Sessions observation adapter to `Service.Observe`. |
| `pkg/services/factory_visualization/internal/service/runtime_source.go` | Visualization type-asserts `APIFactory` to subscribe to runtime events. | Consume the Runtime observation/value path and the Factory Visualization-owned presentation sink; canonical event subscription is a Recordings caller and is retired in P5B. |
| `pkg/services/work/internal/service*` and Work root boundary tests | Work uses a Runtime root-shaped resolver for runtime-scoped admission. | Keep Work policy in Work and replace only any concrete hosted/runtime dependency with the opaque Runtime binding. The Work root remains the only admission authority. |

The CLI command, HTTP application, MCP server, and functional support do not
construct Runtime directly; they remain on `root.BuildProcess` and are P4/P5
callers. Their pre-move behavior is still part of the characterization corpus.

### Strangler sequence and deletion register

P3 is split into independently mergeable slices so no slice depends on a later
slice to restore behavior:

| Slice | Focus and bound | Deletion endpoint |
| --- | --- | --- |
| P3-A | Characterization plus detached Runtime contracts and the inert root; target 8–14 changed files. | No old path is deleted; the compatibility adapter is explicitly temporary and receives no new callers. |
| P3-B | Wire and Factory Sessions opening migration to opaque Runtime bindings; target 12–20 changed files. | Delete the Sessions-facing use of `HostedInstance`, `HostedHandle`, `Lifecycle`, `Sidecars`, `ReplacementBuilder`, and runtime-opening callback injection in this slice. The private `instance_host` records are the named successor. |
| P3-C | Runtime transport/mapping/visualization migration and old root-host surface retirement; target 12–20 changed files. | Delete public `Factory`, `LegacySnapshotProvider`, and the interfaces in `hosting.go` after the listed callers have moved. Delete `APIFactory` as a whole only after its event-stream callers also move; if `SubscribeFactoryEvents` remains, P5B owns the explicitly named whole-interface deletion. The Runtime root plus Recordings is the named successor. |

No P3 slice may delete a compatibility method while a listed caller still
uses it. A retained method is reported as retained compatibility, not as
completed cleanup. P3 does not delete the Sessions `ExecutionService` or
`ForRuntime` surface; P4 owns that retirement.

Before P3-B, the implementer must record one caller-level characterization
test for each row in the migration table: Wire inertness for the Wire row;
activation failure/retry for runtime opening; pause/resume/observation and
terminal cleanup for Sessions control; worker-result correlation for the
live/durable execution row; status/event payloads for mapping; typed response
and cancellation envelopes for Runtime transports; live-view update and
drain behavior for Visualization; and Work admission plus runtime-binding
identity for Work. Existing tests may serve as the characterization only when
they assert that observable behavior directly; otherwise the packet adds a
small named test before the production move.

### Characterization and behavioral guards

Characterization is a precondition of P3-B and P3-C, not a test added after
the move. The following guards are named by observable behavior and tier:

| Behavior to preserve | Guard before the move | Execution tier |
| --- | --- | --- |
| Runtime construction is inert and one root can serve isolated openings. | `pkg/services/factory_runtime/wire/wire_test.go:TestNewServiceConstructsInertRoot`, `pkg/root/root_test.go:TestBuildProcessReusesCanonicalRootsAcrossTwoIsolatedExecutions`, and `tests/functional/sessions/root_composition/build_process_inert_test.go:TestSessionsEffectsRemainInertThroughRootBuildProcessConstruction`. | Local `go test` for the focused packages; PR backend unit and root-process acceptance lanes. |
| Activation publishes only detached successful state, rejects duplicate identity, and retries after failed cleanup. | `pkg/services/factory_runtime/wire/runtime_activation_test.go:TestRuntimeRootActivationPublishesOnlyDetachedSuccessfulState`, `TestRuntimeRootActivationRejectsDuplicateAndConflictingIdentity`, `TestRuntimeRootActivationUnwindsFailedStartAndCanRetry`, and `TestRuntimeRootDeactivationRetainsStateUntilCleanupSucceeds`. | Local Runtime unit tests; PR backend unit coverage and required integration lane. |
| Pause/resume/terminate preserve buffered results and terminal classification. | Existing `pkg/services/factory_runtime/wire/fold_behavior_preservation_test.go:TestWireFoldPreservesControlPauseResumeTerminateThroughPublishedRoot`, plus a planned root-level test that asserts one terminal result after cancellation races with a worker result. | Local focused Runtime tests; PR backend unit and functional short tier. |
| A throttled in-flight dispatch is not treated as terminated before its observed result retires. | `pkg/services/factory_runtime/internal/services/orchestration/subsystems/terminationtests/termination_test.go:TestTerminationCheck_DoesNotTerminateWhileObservedResponseAwaitsRetirement` and `tests/functional/runtime_api/api_provider_throttle_pause_observability_test.go:TestProviderErrorSmoke_ThrottleFailureIsolatesOtherLaneThroughPublicSession`. | Local focused/runtime race test where practical; PR backend functional short tier. |
| Runtime opening failure unwinds resources once and preserves the primary failure. | `pkg/services/factory_runtime/wire/runtime_activation_test.go:TestRuntimeRootActivationUnwindsFailedStartAndCanRetry`, `pkg/services/factory_sessions/internal/runtimeopening/models_bind_test.go:TestRuntimeOpeningCleanupPreservesPrimaryErrorAndAggregatesCleanupFailures`, and `pkg/initializer/lifecycle/manager_test.go:TestManagerUnwindsStartFailureAndJoinsCleanupErrors`. | Local unit tests; PR backend integration and root-process acceptance lanes. |
| Terminal Worker output remains correlated with the Runtime dispatch. | `pkg/services/factory_runtime/internal/services/orchestration/runtime/dispatch_worker_sessions_terminal_semantics_test.go:TestFactoryImpl_DirectAndChildDispatchPreserveIdenticalTerminalOutcomeMapping` and `pkg/services/factory_runtime/internal/services/orchestration/runtime/dispatch_worker_sessions_idempotency_test.go`. Add a caller-level assertion before deleting `BindWorkerInvoker`. | Local Runtime/Worker Sessions tests; PR backend unit and functional coverage lanes. |

Compilation, typecheck, and green CI are required quality gates, but they are
not substitutes for the behavioral rows above. The P3 implementation owner
must run focused `go test` and `go test -race` for changed Runtime/session
packages, then `make verify-fast` while iterating and `make verify-pr` before
merge. `make lint` must keep `backend-size`, `pkg-maint`, `pkg-file-count`,
`pkg-structure`, and `vet` green; no static gate is a behavioral acceptance
criterion.

### P3 sizing and ownership

P3 is one runtime-ownership concern, but it is not one unbounded PR. P3-A,
P3-B, and P3-C are each independently mergeable, each leaves the process
releasable, and each is expected to stay at or below 20 changed files. The
PSS-I01 root/Wire/process lease owns shared `pkg/wire` and initializer changes;
PSS-I05 owns event-backbone changes; Factory Sessions owns its opening caller;
and Factory Runtime owns the root contract and private host cutover. A slice
that exceeds the bound is split by caller family before implementation, not
by adding a temporary second composition path.

## P4 — Converge Factory Sessions onto lifecycle ownership

### Outcome and preserved behavior

P4 leaves one process-scoped `factorysessions.Service` as the customer-facing
Factory Session authority. It owns Factory Session identity, open/start,
activation selection, invocation coordination, lifecycle controls, durable and
live projections, result selection, and ephemeral `FactoryResponseEvent`
delivery. Live and durable sessions are values of one resource, not separate
execution authorities. Runtime mechanics go to `factoryruntime.Service`,
canonical history goes to `recordings.Service`, Work admission stays on
`work.Service`, and application/process activation stays in `pkg/initializer`.

The preserved customer behavior is: CLI, HTTP, MCP, and ACP can open or
resume a Factory Session; list/get/control/result operations identify the same
session; synchronous and asynchronous invocation retain their terminal output
and failure classification; response-event streams retain their documented
ephemeral semantics; cancellation and close still unwind every acquired
resource; and a process can execute isolated invocations repeatedly without
constructing a second graph.

The current tree contradicts this target in known ways. `pkg/services/factory_sessions/service_contracts.go:Service`
publishes the live/durable method families, `ForRuntime`, runtime observation,
and compatibility methods while `pkg/services/factory_sessions/wire` and
`pkg/wire/session_runtime_providers.go` still construct durable/standalone
execution separately. `contracts.go` publishes `ApplicationOpeningRequest`
and `RuntimeHTTPServices`, and the P1 register records `RuntimeHTTPServicesBound`
and `gateCompletionOnRuntimeHost` as deletion seams. P4 resolves that conflict
in favor of the architecture boundary; it does not rewrite the authoritative
WSE or P1 documents.

### Build first

1. Add Sessions-owned detached `Start`, `Invoke`, `Activate`, `Get`, `List`,
   `Control`, `ReadResult`, `PrepareSync`, and response-subscription values in
   a compatibility façade over the existing live/durable implementations.
   Requests carry Factory Session IDs, immutable definition/version selections,
   normalized Work input, correlation, and bounded wait values. They do not
   carry Runtime objects, service bundles, protocol streams, loggers, or
   filesystem handles.
2. Construct one inert Sessions root in `pkg/services/factory_sessions/wire`
   from the canonical `pkg/wire` graph. `Start` creates private session state;
   it must not call another injector or return a service. `pkg/initializer`
   receives only already-constructed application roles and owns start, stop,
   cancellation, join, and reverse cleanup.
3. Migrate live and durable implementations behind the root façade. Sessions
   calls Runtime, Work, Recordings, and Provider Sessions through their public
   roots and assembles only Sessions-owned customer projections.
4. Move application binding, Runtime host readiness, and process binding to
   `pkg/wire`/`pkg/initializer`. Transport adapters retain decode, mapping,
   status/header selection, SSE framing, and typed-error mapping only.

### Caller-by-caller migration

| Caller | Current seam | Migration order and successor |
| --- | --- | --- |
| `pkg/wire/session_runtime_providers.go`, `pkg/wire/profiles.go` | `NewDurableExecution`, `NewStandaloneExecution`, and `NewDurableExecutionRuntime` create separate execution authorities. | Construct one Sessions root and inject it into command, HTTP, MCP, and initializer roles. Delete the constructors after all callers use the root; the root `Start`/`Invoke` methods are the successor. |
| `pkg/services/factory_sessions/wire/wire.go` and `application_graph.go` | Factory Sessions Wire composes runtime-opening bundles and presentation collaborators. | Keep construction providers in `wire`, but return one inert Sessions root with private owner dependencies. Application service-table binding moves to `pkg/wire`; process start/stop moves to `pkg/initializer`. |
| `pkg/services/factory_sessions/internal/sessionservice`, `internal/services/live_runtime`, and `internal/sessionregistry` | Live callers use `OpenFactorySession*`, `ListFactorySessions`, `GetFactorySession`, and mode-specific control methods. | Route each caller through `Start`, `Get`, `List`, and `Control` values while retaining the same IDs, status transitions, and response projection. Registry and lifecycle state remain private Sessions state. |
| `pkg/services/factory_sessions/internal/execution`, `internal/services/durable_execution`, and `internal/executionopening` | Durable callers use `ExecutionService`, `StartAsync`, `StartSync`, resume, result, and separate control families. | Adapt one-shot and durable operations to `Start`/`Invoke`/`ReadResult`/`Control`; preserve wait boundaries and terminal result values. Delete the public execution subroot once parity is proven. |
| `pkg/services/factory_sessions/transports/cli/session`, `transports/cli/sessionexecution`, and `pkg/transports/cli` | CLI command composition receives durable/live facets and performs terminal presentation from split paths. | Bind CLI commands to the Sessions root and keep output/exit translation at the transport boundary. The shared root/process owner is PSS-I03. |
| `pkg/services/factory_sessions/transports/http` and `pkg/transports/http/application` | HTTP receives `RuntimeHTTPServices` and Sessions mapping facets; Session HTTP also contains adjacent Factory/Work policy. | Bind the Session adapter to `factorysessions.Service`, Runtime, Work, Definitions, and Recordings owner roots as needed. Keep domain policy in those owners; `pkg/transports/http` remains application/server composition owned by PSS-I02. |
| `pkg/services/factory_sessions/transports/mcp` and `pkg/transports/mcp` | MCP durable execution and inspection use a separate client/facet vocabulary. | Bind MCP tools to the Sessions root and owner-local Runtime/Recordings/Work contracts. Preserve JSON envelopes and asynchronous terminal polling; PSS-I04 owns shared MCP composition. |
| `pkg/transports/mapping/composition` | Mapping publishes `LiveSessionAPI`, durable facets, and `DurableSessionAPI` as a second service graph. | Replace the facets with owner-local mappers over one Sessions root. Canonical event, dispatch, and artifact reads use Recordings and are not reimplemented in Sessions. |
| `pkg/initializer/application`, `pkg/initializer/lifecycle`, and `pkg/services/factory_sessions/internal/processlifecycle` | Session/application roles publish opening/binding callbacks and perform lifecycle selection around runtime state. | Initializer runs the selected plan over constructed roles; Sessions receives session values only. `Process.Close` continues to join all cleanup errors. |
| `cmd/factory/main.go:runProcess`, `cmd/clicontractsmoke/main.go:run`, `cmd/retiredsurfacecheck/main.go`, and functional support | These callers already use `root.BuildProcess`/`Process.Execute` and must not learn a child injector. | Retain the root boundary unchanged, add the missing full lifecycle characterization, and verify that the new Sessions root is reached through the same reusable process. |

### Strangler sequence and deletion register

P4 is split into focused, independently mergeable slices:

| Slice | Focus and bound | Deletion endpoint or named successor |
| --- | --- | --- |
| P4-A | Pre-move caller characterization and Sessions-owned detached vocabulary; target 10–16 changed files. | The old live/durable implementations remain behind one compatibility façade; no new caller may depend on a legacy facet. |
| P4-B | Wire one Sessions root and migrate internal live/durable callers; target 14–20 changed files. | Delete `ExecutionService`, `NewDurableExecution`, `NewStandaloneExecution`, and `ForRuntime` after their listed callers move. Successor: `factorysessions.Service` plus private per-session state. |
| P4-C | Move application/lifecycle binding and transport composition; target 14–20 changed files. | Delete `ApplicationOpeningRequest`, `RuntimeHTTPServices`, `RuntimeHTTPServicesBound`, `gateCompletionOnRuntimeHost`, `DefinitionActivationGatewayProvider`, and Models presentation collaborators from the Sessions public surface. Successors: `pkg/wire` application roles, `pkg/initializer` lifecycle, `factorydefinitions.Service` values, and `models.Service` scopes. |
| P4-D | Fold the remaining root-owned projection/compatibility methods after P4-B/P4-C parity; target 12–20 changed files. | Delete mode-specific duplicate method families and the Sessions Runtime observation proxy. Runtime observation is the P3 Runtime root; canonical event/dispatch/artifact reads are the P5B Recordings path. Any remaining internal forwarding packages are retired by P6 only if P4 callers have no dependency. |

The exact old path is never described as “cleaned up” while a caller remains.
The P4 deletion register distinguishes compatibility retained for P5 from
deletion completed in P4; P5B owns only the event/transport paths explicitly
named above, and P6 owns only secondary graph residue that P4 cannot safely
remove before transport migration.

Before P4-B, the implementer must record one caller-level characterization
for every migration row: Wire must prove one inert Sessions root; live and
durable callers must prove identity, control, wait, and result behavior;
CLI/HTTP/MCP callers must prove their success, relevant failure, and terminal
response envelopes; mapping must prove owner-root delegation; initializer
must prove reverse unwind and joined cleanup errors; and the root-process
callers must prove repeated isolated execution. The tests named below are the
starting corpus, not permission to infer a guarantee from compilation or from
a source/route inventory.

### Characterization and behavioral guards

P4-A must land caller-level characterization before P4-B changes root
construction or method ownership. The existing coverage and required additions
are:

| Behavior to preserve | Guard before or during the move | Execution tier |
| --- | --- | --- |
| The reusable process constructs an inert graph and executes isolated sessions through the same root. | `tests/functional/sessions/root_composition/process_reuse_inert_test.go:TestRootBuildProcessIsInertAndReusableAcrossFactorySessions`, `tests/functional/sessions/root_composition/build_process_inert_test.go:TestSessionsEffectsRemainInertThroughRootBuildProcessConstruction`, and `pkg/root/root_test.go:TestBuildProcessReusesCanonicalRootsAcrossTwoIsolatedExecutions`. | Local focused Go tests; PR root-process acceptance and backend unit lanes. |
| Factory Session open/list/get/close preserves one stable identity and terminal state. | Existing `tests/functional/sessions/lifecycle/crud_test.go:TestAPIOpenListGetAndCloseFactorySession` plus a planned root-level characterization that exercises the same sequence through `Service` without HTTP-specific policy. | Local Sessions tests; PR backend functional short tier. |
| CLI and API success, domain failure, and cancellation remain equivalent. | `tests/functional/sessions/lifecycle/remote_lifecycle_test.go:TestCLILocalAndRemoteRunSuccessParityThroughRootProcess`, `TestCLILocalAndRemoteRunDomainFailureParityThroughRootProcess`, and `TestCLILocalAndRemoteRunCancellationParityThroughRootProcess`; add the missing `cmd/factory/main.go:runProcess` close-error/cancellation characterization. | Local focused functional cells; PR backend functional short tier and root-process acceptance. |
| Durable asynchronous polling and synchronous invocation retain terminal result shape and not-ready behavior. | `pkg/services/factory_sessions/transports/mcp/execution_test.go:TestMockClient_RuntimeService_StartAsyncRunningObservesStatusAndNotReadyResult` and `TestMockClient_RuntimeService_AsyncPollingObservesTerminalResult`, plus the planned root `Invoke` contract test. | Local MCP/Sessions tests; PR backend contract and functional short lanes. |
| Response events remain ephemeral, ordered, detachable, and distinct from canonical Recordings history. | `pkg/services/factory_sessions/transports/http/handlers_response_events_test.go`, `pkg/services/factory_sessions/internal/responseeventstore/lifecycle_test.go:TestSessionResponseEventStore_CompleteSubscriberDrainsRetainedThenObservesCompletion`, and the existing Recordings lifecycle/projection tests. | Local Sessions/Recordings unit tests; PR backend unit, integration, and API contract lanes. |
| Activation/start failure closes acquired Runtime and visualization resources exactly once while retaining the primary error. | `tests/functional/sessions/root_composition/application_opening_failure_test.go:TestApplicationOpeningClosesRuntimeWhenVisualizationSinkIsUnavailable`, `pkg/wire/session_runtime_providers_test.go:TestProcessCloseContinuesThroughEveryLifecycleOwnerAfterFailure`, and `pkg/initializer/lifecycle/manager_test.go:TestManagerUsesSameShutdownPathForCancellationAndRunnerFailure`. | Local unit/integration tests; PR root-process acceptance and backend integration lanes. |
| Pause, resume, cancel, terminate, and close preserve lifecycle classification and do not affect another session. | `tests/functional/sessions/live_runtime_build_process_test.go:TestBuildProcessRoutesLiveOpenListControlAndCloseThroughFactorySessionsRoot`, `pkg/services/factory_sessions/internal/services/live_runtime/internal/service/concurrency_test.go:TestServiceConcurrentOpenForTargetAllocatesDistinctActivations`, and the current-main throttle/termination guards from the audit. | Local focused tests and `go test -race` for live-runtime changes; PR backend functional short tier. |
| ACP delegation and close/cancel terminal behavior still reach the Sessions authority. | `tests/functional/transport/acp/stdio/cli_serve_acp_controls_test.go:TestServeACP_RootBuildProcessCancelTerminalizesOnlyCapturedPrompt`, `TestServeACP_RootBuildProcessCloseStopsCapturedFactorySession`, and `tests/functional/chat_sessions/root_composition/acp_prompt_delegation_test.go`. | Local ACP-focused tests where available; PR backend functional short tier and pinned ACP evidence. |

The P4 owner must run focused package tests, `go test -race` for changed
session-registry/lifecycle code, `make verify-fast`, and the applicable
contract/functional cells before pushing. Before merge, `make verify-pr`,
`make lint`, `make typecheck`, and `make docs-reference-smoke` where the
packaged-doc embedding path is exercised are required. `backend-size`,
`pkg-maint`, `pkg-file-count`, `pkg-structure`, and `vet` remain named gates;
none is accepted as proof of terminal output, activation, or unwind behavior.

### P4 sizing, ownership, and dependency

P4-A through P4-D each target one concern, stay at or below approximately 20
changed files, and leave `main` releasable. PSS-I01 owns shared root/Wire/
initializer changes, PSS-I02 owns HTTP application binding, PSS-I03 owns CLI
composition, PSS-I04 owns MCP composition, and PSS-I05 owns event-backbone
handoff. Factory Sessions owns its root and private lifecycle state; Factory
Runtime owns the P3 binding it consumes; Recordings owns canonical history.

P4 depends only on the P3 root contract/opaque binding being available. P5A,
P5B, and P5C depend on P4's stable root and must not be used to repair a P4
break. P6 may delete only residue explicitly named by P3/P4 or its own
secondary-graph audit; P7 proves the resulting behavior and race closure.

## P5 — Converge transport and direct-execution callers

P5 is a parallel packet family. P5A owns the CLI composition seam, P5B owns
HTTP/MCP/ACP transport composition, and P5C owns detached Workers execution.
They may proceed concurrently after P3/P4 publish the owner contracts, but
shared paths remain with the named path-lease owner and no packet may repair a
different packet's broken contract.

Every P5 family follows the same migrate-then-delete order: build the owner
adapter or request contract, characterize every caller, make the new path
canonical while the old path only forwards, and delete the old path in a
later slice. P5A-2, P5B-2/P5B-4, and P5C-2/P5C-3 are deletion slices; no
family removes a behavior-preserving path in the slice that first introduces
its successor.

### P5A — Cut the CLI over to owner operations

#### Outcome and preserved behavior

P5A makes the CLI a protocol boundary. The top-level `pkg/transports/cli`
package retains command-tree construction, generated-manifest projection,
global protocol flags, and raw Cobra forwarding. It no longer selects a
Factory, constructs a runtime/session scope, infers completion from Petri
state, joins service roots, or owns domain error classification. The owning
CLI adapters are Factory Definitions, Factory Sessions, Work, Factory Runtime,
Factory Visualization, Workers, Recordings, and Initializer as appropriate.

The preserved customer behavior is: named, path-selected, Current Factory,
local, and remote `you run` invocations accept the same input sources; clean
and server-attached runs retain their startup and shutdown behavior; text,
JSON, and NDJSON output preserve their documented envelopes and ordering; a
successful invocation emits exactly one terminal success result; a failed or
canceled invocation never emits a false success payload; and CLI invocation
requests and API invocation requests remain equivalent where they share the
same public contract. Work submit/list/show/move, Factory CRUD/validation,
session controls, and worker-session commands retain their current exit and
diagnostic behavior.

This is a seam move because `pkg/transports/cli/root.go` still publishes a
large `CommandOperations`/`CommandFactory` operation bag, while
`pkg/transports/cli/run` performs Factory selection, application opening,
runtime/result inspection, and terminal projection. In particular,
`run/run_clean_invocation.go` reads internal marking and dispatch history to
decide success. The successor is the P4 Sessions `Invoke`/`ReadResult` root
contract plus Runtime and Recordings owner projections, not a renamed CLI
facade. The transport-convergence audit's T4, T8, T15, and TC-05/TC-06
dispositions are the authoritative direction for this packet.

#### Prerequisite contract and build first

P5A may start only after P4 has published a stable Sessions root and P3 has
published the Runtime identity, observation, and terminal-result values. The
following contracts must exist before the first caller moves:

- `factorysessions.Service` accepts detached session start/invoke/control and
  result-read values, including sync wait, async polling, partial-result, and
  terminal-result behavior.
- `factorydefinitions.Service` resolves Factory selection, Current Factory,
  invocation inputs, and authored validation without the CLI loading or
  interpreting Factory internals.
- `work.Service` owns submit, staging, materialization, list, show, and move
  policy; `recordings.Service` owns canonical history and replay reads;
  `factoryruntime.Service` and `factoryvisualization.Service` return detached
  status/dashboard values.
- Initializer exposes detached application intents for `run` and server
  attachment. `root.BuildProcess` remains the caller-facing boundary and
  `pkg/wire` remains the sole production graph.

Build the replacement while the current command path remains a delegating
compatibility edge:

1. Add owner-local CLI request/result adapters that receive the original
   `*cobra.Command` and `[]string`, resolve protocol input, call one owner
   root, and render only that owner's result or typed error.
2. Move Factory selection and invocation-input precedence to Definitions
   values, then move invocation and terminal-result selection to Sessions.
   The CLI receives a detached terminal result; it never receives a Runtime
   snapshot, Petri token, marking, or dispatch-history selector.
3. Register the owner adapters in the generated command tree and keep the old
   handlers as thin forwards until the relevant caller row has parity.
4. Move dashboard/workflow projection to Runtime/Visualization and Work
   admission/output to their owner adapters. Initializer owns `run` lifecycle
   and close/error joining; it is not constructed by a command handler.

While both paths coexist, the owner adapter is canonical: every new caller is
registered against it, and the legacy handler can only forward the untouched
command values. A compatibility path that still performs input selection or
terminal inference is not considered migrated.

#### Caller-by-caller migration

The following caller set is exhaustive for the audited CLI seam. A newly
found caller must be added here and characterized before it moves.

| Caller | Current seam and observable behavior | Migration order and successor |
| --- | --- | --- |
| `cmd/factory/main.go:runProcess`, `cmd/clicontractsmoke/main.go:run`, `cmd/retiredsurfacecheck/main.go`, and functional process support | Build/execute/close the reusable process and translate terminal errors to exit status. | Characterize root construction, Execute, Close, cancellation, and joined cleanup errors first. Keep `root.BuildProcess` and `Process.Execute` unchanged; Initializer owns application intent and the process remains the successor. |
| `pkg/transports/cli/root.go`, `root_work.go`, `climanifestcobra`, and `commandregistry` | Aggregate domain operation bags and perform command-family dispatch/input mapping. | Retain only generated node construction and raw forwarding. Wire owner-local adapters once; the owner adapter is the successor for each command family. |
| `pkg/transports/cli/run/run.go`, `selection.go`, `factory_invocation_input.go`, and `runconfig/config.go` | Select Factory/current directory, merge input sources, open runtime/session paths, and carry service/effect dependencies. | Definitions returns a detached selection/input value; Sessions/Initializer consumes it. Delete service/effect fields from `runconfig` and the aggregate `CommandOperations` path; pure parser helpers may remain under the top-level protocol package. |
| `pkg/transports/cli/run/run_clean_invocation.go` and `invocation_observability.go` | Infer success, output, and counts from Petri snapshots, tokens, and dispatch history. | Add caller characterization, then render `factorysessions.Service` terminal results and Runtime/Visualization observations. Delete the Petri/result-selection implementation in this packet; P5B owns only canonical event-stream transport retirement. |
| `pkg/services/factory_sessions/transports/cli/session`, `sessionexecution`, and `pkg/transports/cli/run` invocation callers | Present session create/list/get/control/invoke/result output and terminal errors. | Move each operation to the converged Sessions root while preserving human/JSON/exit projections. The Sessions CLI adapter is the successor; no split live/durable facet may gain a new caller. |
| `pkg/transports/cli/submit`, `batchload`, `work`, and `pkg/services/work/transports/cli` | Decode Work and batch input, apply admission/output policy, and render Work results. | Characterize success, validation, transport failure, and empty output; call the Work root and retain only CLI rendering. Work's service-local CLI adapter is the successor. |
| `pkg/transports/cli/factory`, `workflow`, `dashboard`, and worker-session command families | Perform Factory policy, workflow validation, cross-domain dashboard projection, or worker-session presentation. | Definitions, Runtime, Visualization, and Worker Sessions adapters each receive their own raw command. Delete the cross-domain dashboard fallback and workflow policy from the top-level command path; the named owner adapters are the successors. |

#### Strangler sequence and deletion register

P5A is split into independently mergeable slices. Each slice leaves the
process usable and targets approximately 20 or fewer changed files.

| Slice | Focus and bound | Exact deletion endpoint or named successor |
| --- | --- | --- |
| P5A-1 | Caller characterization and owner-adapter registration; 8–14 changed files. | No compatibility deletion. The generated command node and the legacy forwarder coexist, with owner adapters canonical. |
| P5A-2 | `you run` selection, invocation, terminal output, and exit mapping; 14–20 changed files. | Delete `run_clean_invocation.go`'s Petri/dispatch result inference, the service/effect dependency bag in `runconfig/config.go`, and the corresponding `CommandOperations` fields. Successor: Definitions selection + Sessions result + Initializer intent. |
| P5A-3 | Work, Factory, session, worker-session, and dashboard/workflow command families; 14–20 changed files. | Delete duplicate top-level Work/Factory/session operation wrappers after each owner adapter is registered. Retain only protocol node/forwarding helpers; P5B owns HTTP/MCP/ACP adapters, not these CLI command families. |

Before P5A-2, every row in the caller table must have a characterization test
for successful invocation, relevant domain/provider/transport failure, and
terminal response shape. `run` specifically requires named/path/current
Factory, stdin/positional conflict, clean/server-attached, cancellation, and
text/JSON/NDJSON cases. A passing compile or CLI command inventory is not
characterization.

#### Characterization and behavioral guards

| Behavior to preserve | Direct guard before or during the move | Execution tier |
| --- | --- | --- |
| CLI success/failure/cancellation maps to the correct process status and never claims success after cancellation. | `tests/functional/transport/cli/process/exit_codes_test.go:TestCLIValidationFailureExitCode`, `TestCLIWorkerFailureExitCode`, `TestCLIInterruptedExitCode`, and `tests/functional/transport/cli/process/context_cancellation_test.go:TestCLIContextCancellationEmitsNoSuccessResult`. | Local root-process package tests; PR `test-root-process-acceptance` and Linux functional coverage lane. |
| JSON and NDJSON terminal envelopes remain valid, ordered, and singular. | `tests/functional/transport/cli/output/json_result_test.go:TestCLIJSONSuccessDecodesToPublicInvocationResult`, `TestCLIJSONFailureRemainsValidJSON`, `tests/functional/transport/cli/output/ndjson_stream_test.go:TestCLINDJSONEmitsDecodableResponseEventsThenInvocationResult`, and `TestCLINDJSONFailureEndsWithOneTerminalResult`. | Local focused functional package; PR backend functional coverage lane and root-process acceptance. |
| Named/path Factory invocation, input-source rejection, and stdout failure behavior remain stable. | `tests/functional/transport/cli/commands/run_wiring_test.go:TestCLIRunNamedFactory`, `TestCLIRunFactoryByPath`, `TestCLIRunRejectsConflictingPositionalAndStdinInput`, and `TestCLIRunFailureWritesNoSuccessPayloadToStdout`. | Local CLI functional tests; PR Linux functional coverage lane. |
| CLI/API invocation requests and terminal outcomes stay equivalent. | `pkg/transports/cli/run/run_invocation_parity_test.go:TestFactoryInvocationCLIAndAPIEquivalenceMatrix`, `TestRun_NamedGoalInvocationSuccessParityAcrossCLIAndAPIEnvelope`, and `TestRun_NamedGoalInvocationBlockedFailureParityAcrossCLIAndAPIEnvelope`. | Local CLI package tests; PR backend functional coverage lane and `make api-smoke` where API paths are exercised. |
| Multi-part Work content survives owner mapping into the terminal response. | `pkg/services/work/transports/http/admission_mapping_test.go:TestSubmitWorkResponseToAPI_EncodesDetachedResult`, `pkg/services/work/transports/http/read_mapping_test.go:TestWorkReadModelToAPI_EncodesDetachedReadModel`, `pkg/services/recordings/internal/projections/workstation/workstation_requests_test.go:TestBuildFactoryWorldWorkstationRequestProjectionSlice_ProjectsNilAndDetachedWorkContent`, plus a caller-level CLI response fixture for more than one content part before the move. | Local Work/Recordings mapping tests; PR API contract and Linux functional coverage lanes. |
| Dashboard and workflow output uses owner projections rather than engine internals. | Existing `pkg/transports/cli/run/run_response_stream_renderer_test.go:TestHumanFactoryEventRenderer_WritesTerminalSuccessAndFailureLast` and Runtime/Visualization projection tests; add a root-level assertion that a terminal result remains present when the dashboard is unavailable. | Local Runtime/CLI tests; PR backend functional coverage lane. |

The multi-part row requires a new observable response assertion because
existing mapping fixtures alone do not prove the public CLI terminal
representation. No source, route, command, or asset inventory test may be
used as a behavioral guard.

P5A owners must run focused CLI/owner tests, `make test-root-process-acceptance`,
`make verify-fast`, and `make typecheck` while iterating. Before merge, run
`make verify-pr`, `make test-functional` (or the CI-equivalent Linux
functional coverage lane), `make api-smoke` for any API parity cell, and
`make lint`; `backend-size`, `pkg-maint`, `pkg-file-count`, `pkg-structure`,
and `vet` remain static gates, not behavioral acceptance.

#### P5A sizing, ownership, and dependency

P5A-1 through P5A-3 each target one concern, stay at or below approximately
20 changed files, and leave `main` releasable. PSS-I03 owns shared
`cmd/factory`/CLI composition and generated command registration; PSS-I01 owns
root/Wire/process changes; Factory Definitions, Sessions, Work, Runtime,
Visualization, and Worker Sessions own their adapters and results. P5A does
not edit generated CLI artifacts unless an intentional contract change is
authored under the canonical source and regenerated in the same packet.

P5A depends on P3/P4 and may proceed independently of P5B/P5C after their
owner contracts are stable. It must not use P5B's HTTP/MCP path or P5C's
detached Workers path to repair a broken CLI Sessions contract.

### P5B — Converge HTTP, MCP, and ACP transport cutovers

#### Outcome and preserved behavior

P5B gives each protocol path one owner adapter and one canonical composition
route. `pkg/transports/http` retains generated route registration, aggregate
raw forwarding, UI/static shell behavior, and protocol-only failures.
Service-local HTTP adapters decode and map their own requests to one root.
`pkg/transports/mcp/server` retains raw JSON/tool dispatch and generated
catalog registration; service-local MCP adapters call one owner root.
MCP stdio and ACP application lifecycle are Initializer roles composed by
Wire, not transport-time service construction. ACP delegation retains
Chat Sessions conversation/control semantics and reaches Factory Sessions
through its public contract.

The preserved customer behavior is: HTTP status, headers, content type, body,
SSE framing, reconnect/gap outcomes, and session isolation remain stable;
canonical events, dispatches, artifacts, and historical workstation views are
read through Recordings; Work admission and multi-part content retain their
validation and response shape; MCP discovery, JSON argument errors, sync and
async polling, not-ready results, controls, and terminal failures remain
stable; and ACP delegation preserves session reuse, duplicate delivery
idempotency, busy rejection, cancellation/close terminalization, and typed
failure reporting.

For operations exposed through more than one protocol, the owner result and
typed failure facts are the parity source: CLI, HTTP, and MCP may format them
differently, but they must not select different terminal outcomes or lose
multi-part content. P5A's CLI/API equivalence corpus and P5B's generated HTTP
and MCP contract cells are one acceptance set.

This is a seam move because `pkg/transports/http/application.Handler.Bind`,
`RuntimeHTTPServices`, `pkg/transports/mapping/composition`, the Sessions HTTP
multi-service adapter, and `pkg/transports/mcp/stdio` still act as secondary
graphs or cross-owner façades. The transport-convergence audit's T1-T3,
T5-T7, T10-T14, T16-T18, HTTP/MCP audit, and TC-02/TC-04/TC-05/TC-08/TC-10/
TC-12 are the authoritative scope. P4's deletion of Sessions lifecycle
facets happens first; P5B removes the remaining protocol-owned forwarding
and mapping duplicates rather than reopening that contract.

#### Prerequisite contract and build first

P5B requires the P3 Runtime and P4 Sessions roots, Work admission/read
operations, Definitions authored/packaged values, Recordings history/replay
operations, Provider Sessions inspection, and Initializer application intents.
The generated OpenAPI and MCP contracts are inputs, not hand-edited outputs;
an intentional schema change must be authored under `api/components/`,
regenerated, and covered by contract tests. The ACP envelope/transport
contract remains protocol-owned; its product conversation/control facts remain
Chat Sessions-owned.

Build the replacement in this order:

1. Wire constructs owner HTTP/MCP/ACP adapters once and passes one generated
   HTTP aggregate/raw MCP registry to the protocol shells. No `Bind` call may
   create a service or adapter after application opening.
2. Move Work, Sessions, Definitions, Runtime/Visualization, Models, Provider
   Sessions, and Recordings routes/tools to their local adapters. HTTP maps
   generated values only; MCP decodes raw arguments and maps one root result.
3. Move canonical event, dispatch, artifact, and workstation projection
   reads to Recordings. Sessions retains only ephemeral response-event
   streaming and session links. Delete duplicate Sessions Work handlers after
   Work parity is demonstrated.
4. Move `server`, `mcp serve`, and ACP opening/close/cancel wiring to
   Initializer-owned intents. ACP protocol negotiation and response bridging
   remain in `pkg/transports/acp`; it must not own Factory state or a second
   service graph.

While both paths coexist, Wire's owner-adapter registry is canonical. Legacy
application binding and central mapping may forward to that registry only for
unmigrated callers; no new route/tool or response policy may be added there.
The original HTTP writer/request/path values and raw MCP argument bytes must
reach the owner adapter unchanged.

#### Caller-by-caller migration

| Caller | Current seam and observable behavior | Migration order and successor |
| --- | --- | --- |
| `pkg/transports/http/server.go` and `pkg/transports/http/generated` forwarding | Server/aggregate owns route binding and some duplicate Models/Provider Sessions/catalog behavior. | Characterize raw forwarding, status/content negotiation, then attach prebuilt owner handlers. Delete duplicate global handlers and direct packaged-catalog reads; Factory Definitions, Models, Provider Sessions, and Visualization HTTP adapters are successors. |
| `pkg/transports/http/application/handler.go` and `pkg/transports/mapping/composition/{services,http_binding}.go` | `Bind`/`BindDurableExecution` builds a runtime service table and maps peer roots at application opening. | Consume P4's constructed roles and the Wire aggregate. Delete `Handler.Bind`, `BindDurableExecution`, `RuntimeHTTPServices`, `HTTPBinding`, and operational mapping constructors once all generated methods forward. Successor: PSS-I02 Wire composition plus owner handlers. |
| `pkg/services/factory_sessions/transports/http` | Sessions HTTP contains Factory, Work, Runtime, Recordings, Workers, and durable-facet policy, including duplicate Work and canonical-history routes. | Move Work routes to Work, history/dispatch/artifacts to Recordings, status to Runtime/Visualization, and session lifecycle/invoke/result/ephemeral response events to Sessions. Delete `handlers_work_*`, central session mapping façades, and peer-root fields after parity. |
| `pkg/services/work/transports/http` and `pkg/services/work/transports/mcp` | Work adapters already own parts of admission/read/move but still coexist with Sessions copies and central mapping. | Characterize structured and multi-part content, request-ID/state compatibility, typed errors, and empty results. Make these adapters the only Work protocol implementations. |
| `pkg/transports/http/servertests`, `contracttests`, and `tests/functional/transport/http` | Public server tests cover response events, replay, result polling, startup, content negotiation, and concurrency. | Move tests with the owner adapter; keep them at the generated/public boundary. Do not replace them with source-topology checks. |
| `pkg/transports/mcp/server`, `mcp/generated`, and `mcp/stdio` | Generic MCP dispatch and stdio currently compose runtime/session roles and bind tools late. | Wire prebuilds raw tool handlers; Initializer owns stdio lifecycle. Delete transport-time service construction and keep generated discovery/catalog tests. PSS-I04 owns shared `pkg/transports/mcp`. |
| `pkg/services/factory_sessions/transports/mcp`, Work/Definitions/Runtime/Recordings MCP adapters | MCP tools expose sync/async start, result, controls, and inspection through mixed facets. | Route each tool to one owner root, preserve raw argument/schema/error envelopes, and delete cross-owner tool registries. Sessions MCP is the successor only for Session operations. |
| `pkg/transports/acp`, `pkg/transports/cli/acp`, and `pkg/services/chat_sessions` callers | ACP command/stdio paths combine settings/provider configuration, target delegation, session reuse, controls, and response bridging. | Operator Settings and Providers own configuration; Chat Sessions owns conversation/target/control sequencing; Sessions owns Factory invocation. Delete `pkg/transports/cli/acp.Service` cross-root joins after the owner operations are live. |

#### Strangler sequence and deletion register

P5B is divided by protocol/ownership seam so each slice is independently
reviewable and mergeable after P4. Each slice targets approximately 20 or
fewer changed files; a route family that exceeds that bound is split by
owner, not by introducing another aggregate graph.

P5B migrates and characterizes each route/tool family before P5B-2 through
P5B-4 delete central forwarding, mapping, or transport-time construction.
The P5B-1 owner adapters are canonical while the compatibility routes remain.

| Slice | Focus and bound | Exact deletion endpoint or named successor |
| --- | --- | --- |
| P5B-1 | Wire-composed HTTP owner adapters and Sessions/Work response parity; 14–20 changed files. | Delete duplicate Sessions Work handlers and their central mapped operation path after Work adapter parity. Successor: Work/Sessions owner HTTP adapters and PSS-I02 raw aggregate. |
| P5B-2 | Recordings canonical history, replay, dispatch, artifact, workstation views, and response-event boundary; 12–20 changed files. | Delete `pkg/transports/http/workstationprojection`, `pkg/transports/mapping/factoryeventprojection`, and canonical-history handlers from Sessions. Successor: Recordings HTTP/MCP adapters; ephemeral response events remain Sessions-owned. |
| P5B-3 | MCP registry/stdio lifecycle and ACP delegation/control bridge; 14–20 changed files. | Delete `pkg/transports/mcp/stdio` service construction, `pkg/transports/cli/acp.Service` cross-root join, and transport-time Session/Runtime binding. Successor: PSS-I04 raw MCP registry, Initializer stdio intent, and Chat Sessions/Providers/Settings owner operations. |
| P5B-4 | Residual HTTP application/mapping forwarding after all generated callers move; 12–20 changed files. | Delete `pkg/transports/http/application`, `RuntimeHTTPServices`/`HTTPBinding` compatibility, and operational `pkg/transports/mapping/composition`. Any pure owner mapper that remains is moved beside its owner; P6 owns only separately audited non-transport secondary graph residue. |

Before P5B-1, every HTTP route family and every service-local adapter caller
must have success, relevant failure, and terminal/stream response
characterization. Before P5B-3, enumerate each MCP tool and ACP delegation
caller by behavior, then add guards for successful result, malformed input,
typed failure, cancellation, and terminal response/event shape. The detached
agent-run regression is included when an ACP/Worker route reaches Workers;
otherwise it is owned by P5C. Multi-part Work content is mandatory for any
route that accepts or returns `WorkContentPart` values.

#### Characterization and behavioral guards

| Behavior to preserve | Direct guard before or during the move | Execution tier |
| --- | --- | --- |
| HTTP content type, malformed-body status, and response isolation remain stable. | `tests/functional/transport/http/server/content_negotiation_test.go:TestAPIJSONRequestsAndResponsesUseDocumentedContentType`, `TestAPIUnsupportedContentTypeReturns415`, `TestAPIMalformedJSONReturnsStructured400`, and `tests/functional/transport/http/server/concurrent_requests_test.go:TestAPIConcurrentSessionRequestsRemainIsolated`. | Local HTTP contract/functional tests; PR `make api-smoke` and Linux functional coverage lane. |
| HTTP startup and failure unwind do not leak a listener or active stream. | `tests/functional/transport/http/server/startup_shutdown_test.go:TestAPIServerStartsOnConfiguredListenerAndServesStatus`, `TestAPIServerShutdownClosesListenerAndActiveStreams`, and `TestAPIServerBindFailureUnwindsStartedLifecycleRoles`. | Local server integration tests; PR backend integration and root-process acceptance lanes. |
| Session result polling retains terminal, running/not-ready, timeout, and missing-session envelopes. | `pkg/transports/http/servertests/server_durable_session_result_test.go:TestGetFactorySessionResults_RuntimeBackedCompletedReturnsFinalResult`, `TestGetFactorySessionResults_RuntimeBackedRunningReturnsNotReady`, and `TestGetFactorySessionResults_RuntimeBackedSyncTimeoutReturnsAvailability`; `pkg/services/factory_sessions/transports/mcp/execution_test.go:TestMockClient_AsyncPolling_ObservesCompletedFixtureThroughStatusAndResult`. | Local HTTP/MCP contract tests; PR API contract and Linux functional coverage lanes. |
| Canonical response events and reconnect/replay remain ordered and typed. | `pkg/transports/http/servertests/server_factory_session_orchestrator_test.go:TestFactorySessionsAPI_ResponseEventsRouteStreamsGeneratedContractEnvelope`, `TestFactorySessionsAPI_ResponseEventsRouteSignalsStaleReconnectFirst`, `TestFactorySessionsAPI_GetFactorySessionResult_OmitsRawCheckpointBody`, and `pkg/transports/http/contracttests/openapi_contract_response_events_test.go:TestOpenAPIContract_FactoryResponseEventPayloadUnionCoversAllVariants`. | Local HTTP contract/SSE tests; PR api-smoke, backend integration, and functional coverage lanes. |
| Multi-part Work content maps without losing part order/type or inventing a success result. | `pkg/services/work/transports/http/admission_mapping_test.go:TestStageContentRequestFromAPI_DecodesBase64Payload`, `TestSubmitWorkResponseToAPI_EncodesDetachedResult`, `pkg/services/work/transports/http/read_mapping_test.go:TestWorkReadModelToAPI_EncodesDetachedReadModel`, and `tests/functional/sessions/root_composition/work_admission_response_stream_test.go:TestSessionsWorkAdmissionAndResponseStreamActivateThroughRootBuildProcessAfterLifecycle`; add a two-part public response fixture if the moved route does not already cover it. | Local Work/Session integration; PR API contract and Linux functional coverage lanes. |
| MCP schemas, malformed-input errors, controls, and terminal result envelopes remain stable. | `tests/functional/transport/mcp/stdio/discovery_test.go:TestMCPStdioInitializeAndToolDiscovery`, `TestMCPUnknownToolReturnsProtocolError`, `pkg/services/factory_sessions/transports/mcp/failure_paths_test.go:TestMockClient_GetResult_FailedFixtureReturnsPartialResultWithFailureDetails`, and `tests/functional/sessions/mcp/controls_test.go:TestMCPPauseResumeAndCancelTargetCanonicalFactorySession`. | Local MCP contract/functional tests; PR MCP contract boundary in `make verify-fast` plus Linux functional coverage lane. |
| ACP delegation reuses one session, is idempotent under redelivery, rejects busy work, and terminalizes cancel/close/failure safely. | `tests/functional/chat_sessions/root_composition/acp_prompt_delegation_test.go:TestACPPromptDelegationStartsOneFactorySessionAndReusesItForLaterTurns`, `TestACPPromptDelegationFailedFactoryInvocationReportsAnACPError`, `TestACPPromptDelegationRedeliveredRequestMakesNoSecondFactoryDispatch`, `TestACPPromptDelegationConcurrentPromptRejectsAsBusyWithNoFactoryDispatch`, and `tests/functional/transport/acp/stdio/cli_serve_acp_controls_test.go:TestServeACP_RootBuildProcessCancelTerminalizesOnlyCapturedPrompt`. | Local Chat Sessions/ACP tests; PR Linux functional coverage lane plus the pinned ACP real-client test `TestPinnedAcpxCompletesDefaultFactoryBuilderPrompt`. |

Compilation, typecheck, and green CI remain required quality gates, but none
substitutes for the behavioral rows. P5B owners run focused HTTP/MCP/ACP
tests, `go test -race` for stream/cancellation changes, `make verify-fast`,
and `make typecheck` while iterating. Before merge run `make api-smoke`,
`make verify-pr`, the Linux functional coverage lane, and `make lint`.
OpenAPI/MCP generated artifacts must be regenerated only from their canonical
sources; CI evidence belongs in PR comments, never in this plan or a commit.

#### P5B sizing, ownership, and dependency

PSS-I02 owns `api/` and `pkg/transports/http`; PSS-I03 owns shared CLI ACP
registration; PSS-I04 owns `pkg/transports/mcp`; PSS-I05 owns event-boundary
metadata. Wire owns construction, Initializer owns application opening and
unwind, and the product services own their adapters. Recordings owns canonical
history; Chat Sessions owns ACP conversation/control context; Providers and
Operator Settings own ACP configuration; Factory Sessions owns Factory
Session operations.

P5B depends on P3/P4 and is independently mergeable from P5A/P5C once the
owner contracts are available. It must not move a shared path leased to
another integration packet without that packet's owner, and it must not leave
a temporary adapter without a deletion row above or a named P6 successor.

### P5C — Converge direct Workers execution and detached agent-run

#### Outcome and preserved behavior

P5C makes one detached Workers execution contract the canonical direct path.
`root.BuildStatelessWorkers` remains a public composition boundary for a
single detached attempt; `pkg/wire` constructs the inert Workers root once;
`workers.Service.Execute` receives a complete detached request and returns a
normalized detached result. The path does not construct or open a Factory
Runtime, Factory Session, or second application graph. Runtime-owned dispatches
from P3 use the same Workers root but retain Runtime ownership of capacity,
correlation, Work materialization, and terminal application.

The preserved customer behavior is: script, inference, and agent-shaped
requests return correlated accepted/continued/rejected/failed/canceled
outcomes; detached `agent.run` preserves the last provider turn, tool policy,
goal/decision-envelope output, safe diagnostics, provider continuation, and
terminal response events; provider/model/command failures retain their typed
classification; cancellation and timeout stop request-scoped effects and
release worktrees/temporary resources; repeated calls do not share attempt
state; and a detached run never causes Runtime or Session opening. This
packet covers the detached Workers/agent-run seam, not ACP conversation
sequencing (P5B) or Runtime scheduling policy (P3).

The detached root has no independent CLI or HTTP representation, so it does
not invent a second CLI/API parity contract. When its `ExecuteResult` is
projected through Runtime, Sessions, CLI, HTTP, or MCP, P5C must preserve the
same correlation, outcome, failure facts, and content parts; P5A/P5B own the
caller-specific presentation parity guards.

The current tree already has the intended public boundary in
`pkg/root/process.go:BuildStatelessWorkers`, `pkg/services/workers/wire`, and
`workers.Service.Execute`, but private `WorkerExecutor`,
`WorkstationRequestExecutor`, `AssembledRuntimeBinding`, runtime-assembly,
workstation-pool, and direct-child compatibility paths remain reachable from
Runtime/Sessions composition. The WSE C1-C9 contracts and WSE-02/WSE-03/WSE-05/
WSE-06/WSE-09 stories are the authoritative execution vocabulary; P5C
characterizes and cuts the direct path without reintroducing a Workers-to-
Sessions or Workers-to-Runtime construction dependency.

#### Prerequisite contract and build first

P5C requires the P3 Runtime-to-Workers root call, P4 removal of per-session
Workers construction/back-query, and detached values from Factory Definitions,
Work, Providers, and Models. It also requires exact `edges.Edges` ports for
command execution, filesystem/worktree, temporary files, provider/model
execution, and observation publication. Raw credentials, Runtime tokens,
Petri markings, service interfaces, and process handles are prohibited from
`ExecuteRequest`.

Build the replacement in this order:

1. Freeze `workers.ExecuteRequest`, `ExecuteResult`, outcome/error semantics,
   correlation, continuation, safe diagnostics, and observation values. Add
   validation/cloning at ingress so every call is detached and immutable.
2. Make `workers/wire.NewService` and the canonical `wire.BuildStatelessWorkers`
   compose private runner registries once, with no lifecycle start or
   Runtime/Session lookup. Keep provider/model/worktree effects at injected
   edges.
3. Route script, inference, and agent/`AGENT_RUN` execution through
   `workers.Service.Execute`, preserving decision-envelope, tool, provider
   continuation, output, and failure normalization. Runtime maps its selected
   catalog into this request; it does not pass executor objects.
4. Characterize the same direct agent-run behavior from the public root and
   from Runtime child dispatch where applicable. Recordings receives detached
   safe observations; terminal completion is the result, not a progress-only
   callback.

While both paths coexist, `workers.Service.Execute` is canonical. The old
executor/pool/runner implementations may sit behind the Workers root adapter,
but no caller may construct them or use their service-shaped interfaces. The
stateless root and Runtime path share the implementation; they differ only in
the caller-provided request identity and Runtime-owned lifecycle policy.

#### Caller-by-caller migration

| Caller | Current seam and observable behavior | Migration order and successor |
| --- | --- | --- |
| `pkg/root/process.go:BuildStatelessWorkers`, `pkg/wire/wire.go`, and generated `wire_gen.go` | Public detached construction and canonical provider composition. | Characterize context validation, inert construction, provider validation, and detached execution. Keep these boundaries; `workers.Service.Execute` is the successor for direct calls. |
| `pkg/services/workers/wire/wire.go` and `internal/service` | Builds private runner registries and normalizes script/inference/agent attempts. | Freeze detached request/result semantics, route every strategy through `Service.Execute`, and keep constructors private to Workers `wire`. Delete peer-visible construction callbacks; Workers root is the successor. |
| `pkg/services/workers/internal/services/workstations/executor/agentrun/{detached,executor}.go` and agent runner registries | Detached `agent.run` loop, tool policy, last-turn selection, diagnostics, and final-message publication. | Characterize success, provider/model/harness failure, timeout/cancel, tool policy, and goal envelope. Move behavior behind `workers.Service.Execute`; the private runner implementation may remain, but `ExecuteDetached` is deleted or made private to the adapter. |
| `pkg/services/factory_runtime/internal/services/orchestration/runtime` and Runtime worker-session bridge | Runtime schedules WorkerExecutor objects and consumes legacy result/observation shapes. | Use P3 Runtime requests to call `workers.Service.Execute`, correlate one result, and materialize proposed output through Work. Delete Runtime imports of `WorkerExecutor`, `WorkstationRequestExecutor`, and `AssembledRuntimeBinding`; Runtime root values are the successor. |
| `pkg/services/factory_sessions/internal/execution` direct/child executor callers | JavaScript child and direct execution paths retain per-session worker callbacks and child-specific result wiring. | Move the child attempt through the Runtime/Workers request boundary, preserving child identity, provider continuation, response event, and resource lease behavior. The P3 Runtime + Workers root is the successor; Sessions keeps only customer projection. |
| `tests/functional/workers/agent`, `tests/functional/workers/inference`, and `pkg/wire/session_runtime_providers_test.go` | Public detached and root-composition behavior, including cleanup and “before runtime opening” behavior. | Keep public root tests and add failure/cancellation/terminal response cases before deleting old paths. Functional scenarios use `root.BuildStatelessWorkers` and `edges.Edges`, not a private Workers wire graph. |

#### Strangler sequence and deletion register

P5C is split into independently mergeable slices, each approximately 20 or
fewer changed files and each leaving direct execution usable.

P5C-1 builds and characterizes the detached Workers contract first. P5C-2
and P5C-3 delete direct executor and callback callers only after every listed
caller is on workers.Service.Execute and the Runtime result boundary. The
P5C-4 cleanup is intentionally handed to P6-C, so P5C never introduces and
removes the persistent graph in one slice.

| Slice | Focus and bound | Exact deletion endpoint or named successor |
| --- | --- | --- |
| P5C-1 | Detached request/result contract and public root characterization; 8–14 changed files. | No old implementation deletion. The Workers root adapter is canonical and the old runner path receives no new callers. |
| P5C-2 | Script/inference/agent runner cutover and detached `agent.run`; 14–20 changed files. | Delete direct callers of `ExecuteDetached` and peer-visible runner/executor construction. Successor: `workers.Service.Execute` plus private Workers runner registry. |
| P5C-3 | Runtime/child dispatch and observation/result handoff; 14–20 changed files. | Delete Runtime/Sessions use of `WorkerExecutor`, `WorkstationRequestExecutor`, and `AssembledRuntimeBinding` for the migrated callers. Successor: P3 Runtime active-attempt contract and Workers `ExecuteResult`. |
| P5C-4 | Legacy pool/runtime-assembly retirement after all callers move; 12–20 changed files. | Named successor P6-C: delete `pkg/services/workers/internal/services/runtime_assembly`, the public `workstation_pool_boundary`/`workstation_pool` compatibility contracts, and remaining `AssembledRuntimeBinding` construction after P5C conformance is green. P5C must record any retained compatibility explicitly; it may not silently leave a second production graph. |

Before P5C-2, enumerate every direct `Service.Execute`, detached agent-run,
Runtime child, and provider/model/worktree effect caller. Each caller needs a
pre-move guard for success, relevant failure, cancellation/timeout, and
terminal result shape. The detached agent-run guard must include the goal
decision envelope and last-provider-turn behavior; the Runtime child guard
must include response-event publication and exactly one terminal outcome.
These are behavior tests, not checks that scan runner names or package paths.

#### Characterization and behavioral guards

| Behavior to preserve | Direct guard before or during the move | Execution tier |
| --- | --- | --- |
| The public detached root executes without opening Runtime/Session and returns a correlated terminal result. | `tests/functional/workers/agent/stateless_root_test.go:TestBuildStatelessWorkersExecutesDetachedAttemptThroughRoot`, `pkg/wire/session_runtime_providers_test.go:TestBuildStatelessWorkersExecutesBeforeRuntimeOpening`, and `pkg/services/workers/wire/stateless_execute_test.go:TestNewServiceExecuteRunsScriptInferenceAndAgentAttempts`. | Local Workers/root tests; PR root-process acceptance and Linux functional coverage lane. |
| Detached agent-run preserves goal decision-envelope output and provider identity. | `pkg/services/workers/wire/stateless_execute_test.go:TestNewServiceExecuteDetachedAgentRunPreservesGoalDecisionEnvelope`, `pkg/services/workers/internal/services/workstations/executor/agentrun/executor_test.go:TestAgentRunExecutor_MapsSuccessfulCompletionToWorkResult`, and `pkg/services/workers/internal/services/runners/agents/decision_envelope_test.go:TestAgentExecutor_ReviewWorkstation_ParsesDecisionEnvelopeAccepted`. | Local Workers package tests; PR backend unit and Linux functional coverage lanes. |
| Agent-run failure, timeout, cancellation, and safe diagnostics retain typed observable outcomes. | `pkg/services/workers/internal/services/workstations/executor/agentrun/executor_test.go:TestAgentRunExecutor_HarnessFailureSurfacesAgentRunFailureClass`, `TestAgentRunExecutor_TimeoutSurfacesAgentRunTimeoutClass`, `TestLibraryHarnessAdapter_CancellationStopsLoop`, and `pkg/services/workers/internal/services/workstations/executor/agentrun/failure_test.go:TestAgentRunFailureDiagnostics_ProviderFailurePreservesSafeType`. | Local focused tests and `go test -race` for cancellation; PR backend functional coverage lane. |
| Worktree, temporary-file, and pre-start failure cleanup occurs on every direct attempt exit. | `pkg/wire/session_runtime_providers_test.go:TestCanonicalStatelessWorkersReleasesProductionWorktreeAfterSuccess`, `TestCanonicalStatelessWorkersReleasesProductionWorktreeAfterCancellation`, `TestCanonicalStatelessWorkersReleasesProductionWorktreeAfterPreStartFailure`, and `tests/functional/workers/inference/codex/worktree_workstation_test.go:TestCodexWorktreeReleaseRemovesCreatedCheckout`. | Local Workers/Wire tests; PR backend integration and Linux functional coverage lanes. |
| Runtime dispatch reaches Workers root, accepts one result, and maps direct/child terminal outcomes identically. | `pkg/services/factory_runtime/internal/services/orchestration/runtime/dispatch_workers_root_boundary_test.go:TestFactoryImpl_PlanDispatchExecutesThroughWorkersRootBoundary`, `TestFactoryImpl_PlannedDispatchAcceptsWorkersResultThroughRuntimeRoot`, and `pkg/services/factory_runtime/internal/services/orchestration/runtime/dispatch_worker_sessions_terminal_semantics_test.go:TestFactoryImpl_DirectAndChildDispatchPreserveIdenticalTerminalOutcomeMapping`. | Local Runtime/Workers integration and race tests; PR backend integration and Linux functional coverage lanes. |
| Runtime/ACP child agent-run response events retain identity and terminal publication. | `tests/functional/chat_sessions/root_composition/acp_worker_child_events_test.go` child-event assertions, `pkg/services/factory_sessions/internal/execution/javascript_runtime_result_test.go:TestChildWorkerExecutor_CompletedChildRecordsItsWorkerAndOutput`, and `TestChildWorkerExecutor_InvocationErrorStillRecordsAFailedChild`; add a public root assertion if the moved path changes the observation boundary. | Local child/Chat Sessions tests; PR Linux functional coverage lane and pinned ACP evidence when ACP reaches this path. |

P5C owners run focused Workers/Runtime tests, `go test -race` for attempt and
cleanup changes, `make test-root-process-acceptance`, `make verify-fast`, and
`make typecheck` while iterating. Before merge run `make verify-pr`, the Linux
functional coverage lane, relevant provider/worktree specialty tests, and
`make lint`; no CI/audit evidence is committed.

#### P5C sizing, ownership, and dependency

Workers owns `workers.Service.Execute` and its private runners; Factory Runtime
owns active-attempt scheduling and result application; Work owns proposed
output materialization; Providers and Models own their execution contracts;
Recordings owns safe observations; PSS-I01 owns root/Wire/process composition.
The WSE path owns any package-tree changes under Workers, while PSS-I05 owns
event-boundary metadata.

P5C depends on P3's Runtime call boundary and P4's removal of per-session
Workers construction, but is independently mergeable from P5A and P5B. P6-C
is the named deletion successor for the final legacy pool/runtime-assembly
retirement; until then every compatibility contract must list its remaining
caller and retirement slice.

## P6 — Retire the secondary composition graph

### Outcome and preserved behavior

P6 is the retirement packet after P3, P4, and all three P5 lanes have
migrated their callers. Its outcome is one production composition path:
pkg/root.BuildProcess is the caller-facing boundary, pkg/wire.InjectBundle is
the only production construction graph, and pkg/initializer opens and unwinds
already-constructed roles. No transport, service operation, or worker attempt
may construct a second service graph, bind a service table at operation time,
or recover a legacy owner through a type assertion.

The P6 audit begins from the merged P5 heads and records each row as deleted,
retained compatibility, or still blocking. The known secondary graph and
compatibility candidates are:

| Candidate residue | Current evidence and safe successor |
| --- | --- |
| HTTP application binding and central mapping | pkg/transports/http/application/handler.go:Handler.Bind and BindDurableExecution, plus pkg/transports/mapping/composition/http_binding.go:HTTPBinder.Bind. Successor: Wire-composed owner handlers with generated forwarding. |
| Opened HTTP service bags and Sessions application roles | pkg/services/factory_sessions/contracts.go:RuntimeHTTPServices, pkg/services/factory_sessions/internal/roles/contracts.go, and pkg/services/factory_sessions/wire/application_graph.go. Successor: inert owner adapters built by Wire and activated by Initializer. |
| Sessions runtime compatibility | pkg/services/factory_sessions/internal/service/root.go:ForRuntime, session-service runtime gateways, and any remaining ExecutionService or BindWorkerInvoker caller. Successor: the converged Factory Sessions root plus the P3 Runtime and P5C Workers contracts. |
| Runtime legacy owner surface | pkg/services/factory_runtime/interfaces.go:APIFactory and any remaining hosting, legacy-snapshot, callback, or concrete-runtime caller. Successor: factoryruntime.Service detached values and Recordings for canonical history. |
| Cross-owner projection fallbacks | pkg/services/factory_visualization/internal/service/runtime_source.go and pkg/services/work/internal/live_session_runtime.go type-assert or infer legacy runtime state. Successor: Runtime observation and Visualization/Work owner projections. |
| Workers persistent execution graph | pkg/services/workers/internal/services/runtime_assembly, runtime construction helpers, AssembledRuntimeBinding, and workstation-pool boundary contracts. Successor: workers.Service.Execute with Runtime-owned active-attempt state. |

P6 explicitly retains pkg/root.BuildStatelessWorkers, the Workers root
contract, Runtime-owned opaque checkpoint recovery, Recordings canonical
history/artifacts, and public protocol contracts. The DEC-RUN-REC-DURABILITY
decision prohibits replacing Runtime-private checkpoint recovery with a second
Recordings graph. P6 is not permission to delete a compatibility method whose
caller has not moved or to repair a product behavior defect found during
retirement.

Compatibility retention is explicit while the final callers are still being
migrated: the Workers workstation admission/dispatch capability may remain
behind the Workers root for authored workstation routes until P5C/P6-C proves
that request-scoped `Execute` preserves capacity, cancellation, and terminal
publication; detached `root.BuildStatelessWorkers` remains a public boundary;
and Runtime checkpoint recovery remains Runtime-private. Those retained
surfaces receive no new callers, are not a second composition graph, and have
the P6-C or named durability successor above rather than an unnamed cleanup
obligation.

The preserved customer behavior is: process construction remains inert and
reusable; activation failure closes every acquired role without masking the
primary error; Factory Session control and result reads retain their identity
and terminal shape; CLI, HTTP, MCP, and ACP retain their envelopes and
terminal outcomes; detached agent-run remains outside Runtime/Session opening;
canonical replay and live response streams remain distinct; throttled
in-flight dispatches are not terminated before observed results retire; and
two concurrent sessions cannot share attempt, response, or recording state.

### Build first

P6 must build the proof boundary before deleting any old graph:

1. Reconcile the P3/P4/P5 deletion rows with current main and the merged
   caller set. For each candidate above, record the exact remaining caller,
   the owner contract already serving it, and the slice that will delete it.
   A missing path is recorded as already deleted; it is not reintroduced.
2. Make the canonical Wire registry and Initializer role set the only path
   used by newly migrated callers. Construction tests must prove that
   building a process performs no lifecycle, runtime-opening, worker-pool, or
   protocol-binding effect.
3. Migrate remaining callers to detached owner requests and projections.
   Characterization must pass before the old method, adapter, constructor, or
   type assertion is removed.
4. Delete the compatibility path in a later slice, run the focused behavioral
   guards, and only then remove unused tests, aliases, and imports. Generated
   OpenAPI or Wire output is regenerated from canonical sources only when a
   real contract change requires it.

While a candidate and its successor coexist, the successor is canonical:
new callers must use it, the old path may only forward, and the old path may
not select policy, construct services, infer terminal state, or retain a
second graph. P6 follows migrate, characterize, delete; a slice that
introduces a replacement and removes the only behavior-preserving path in
one step is not independently mergeable.

### Caller-by-caller migration

The post-P5 caller inventory is exhaustive for the retirement seam. A newly
found caller stops deletion of its row until it has a characterization test
and an owner-contract migration entry.

| Caller | Behavior to characterize before moving | Migration order and successor |
| --- | --- | --- |
| cmd/factory/main.go:runProcess, cmd/clicontractsmoke/main.go:run, and functional process support | Root build, Execute, cancellation, Close, exit classification, and joined cleanup errors. | Keep root.BuildProcess and Process.Execute as the public path; move application intent and lifecycle ownership to Initializer, then delete any command-owned graph construction. |
| pkg/transports/http/application and pkg/transports/mapping/composition | Generated route forwarding, status/content negotiation, stream/reconnect outcomes, and malformed-input errors. | Move each route to its owner adapter and precompose it in Wire; delete Handler.Bind, BindDurableExecution, RuntimeHTTPServices binding, HTTPBinding, and operational mapping constructors only after parity. |
| pkg/services/factory_sessions/internal/service, internal/sessionservice, and factory_sessions/wire | Open/list/control/invoke/result identity, live/durable parity, response-event ownership, and runtime cleanup. | Use the converged Sessions root for customer session operations, Runtime for observation/control, and Recordings for canonical reads; delete ForRuntime, ExecutionService, or callback bridges only when their final caller is gone. |
| pkg/services/factory_runtime/internal and factory_visualization/internal/service | Runtime activation, detached observations, status projection, dashboard availability, and legacy APIFactory fallbacks. | Route every caller through factoryruntime.Service and Visualization owner projections; delete legacy interfaces and type assertions after the P3/P5 characterization rows pass. |
| pkg/services/work/internal and Recordings transports | Work admission/materialization, multi-part content, canonical event/replay reads, and historical workstation views. | Work owns Work policy and Recordings owns canonical history/artifacts; delete cross-owner projection and central mapping fallbacks after their transport parity cells pass. |
| pkg/services/workers/internal, workers/wire, and Runtime child dispatch | Direct and child attempt identity, result correlation, cleanup, cancellation, and capacity. | Call workers.Service.Execute with detached values and keep active-attempt policy in Runtime; delete runtime_assembly, AssembledRuntimeBinding, and workstation-pool callers after P5C/P6-C conformance. |
| CLI, HTTP, MCP, ACP, and generated-contract test callers | Public success, failure, cancellation, terminal response, replay, and stream-gap shapes. | Keep tests at the public or owner boundary, update only intentional contract changes, and remove fixtures that exercise a deleted compatibility path rather than preserving that path for tests. |

### Strangler sequence and deletion register

Each slice leaves main releasable and targets approximately 20 or fewer
changed files. The deletion endpoint is deliberately later than caller
migration and characterization.

| Slice | Focus and bound | Exact deletion endpoint or named successor |
| --- | --- | --- |
| P6-A | Retire HTTP application binding and operational central mapping; 14–20 changed files. | Delete pkg/transports/http/application and pkg/transports/mapping/composition operational constructors after all generated routes forward to Wire-built owner handlers. Pure owner mappers are the successor and move beside their owner. |
| P6-B | Retire Sessions runtime/application compatibility; 12–20 changed files. | Delete RuntimeHTTPServices application binding, ForRuntime/ExecutionService compatibility methods, and callback bridges with zero callers. Successor: the converged Sessions, Runtime, Recordings, and Initializer contracts. |
| P6-C | Retire the Workers secondary graph named by P5C-4; 12–20 changed files. | Delete internal/services/runtime_assembly, BuildRuntime/BuildRuntimeExecutors callers, AssembledRuntimeBinding, and workstation-pool lifecycle contracts after workers.Service.Execute and Runtime active-attempt guards are green. |
| P6-D | Retire residual Runtime/Visualization/Work legacy projections and close the deletion register; 8–16 changed files. | Delete APIFactory, legacy snapshot/callback adapters, and cross-owner projection fallbacks only where the P3-P5 owner rows show zero callers. Named successor: Runtime detached observation, Visualization projection, Work root, or Recordings root by concern. |

P6-A through P6-D explicitly use migrate, characterize, then delete. A
temporary adapter introduced by P3-P5 has exactly one row above; if the row is
not safe to remove, the packet records retained compatibility and its
remaining caller rather than claiming completion. No source-topology scan is
a behavioral acceptance test.

### Characterization and behavioral guards

| Behavior to preserve | Direct guard before or during deletion | Execution tier |
| --- | --- | --- |
| One inert, reusable process graph serves isolated executions. | pkg/root/root_test.go:TestBuildProcessReusesCanonicalRootsAcrossTwoIsolatedExecutions, tests/functional/sessions/root_composition/process_reuse_inert_test.go:TestRootBuildProcessIsInertAndReusableAcrossFactorySessions, and tests/functional/sessions/root_composition/build_process_inert_test.go:TestSessionsEffectsRemainInertThroughRootBuildProcessConstruction. | Local root and session tests; PR root-process acceptance and backend unit lanes. |
| Activation failure unwinds acquired roles once and preserves the primary failure while close continues through later owners. | tests/functional/sessions/root_composition/application_opening_failure_test.go:TestApplicationOpeningClosesRuntimeWhenVisualizationSinkIsUnavailable, pkg/initializer/lifecycle/manager_test.go:TestManagerUnwindsStartFailureAndJoinsCleanupErrors, and pkg/wire/session_runtime_providers_test.go:TestProcessCloseContinuesThroughEveryLifecycleOwnerAfterFailure. | Local lifecycle/root-composition tests; PR root-process acceptance and backend integration lanes. |
| CLI and HTTP terminal output never reports success after cancellation or failure. | tests/functional/transport/cli/process/context_cancellation_test.go:TestCLIContextCancellationEmitsNoSuccessResult, tests/functional/transport/cli/output/ndjson_stream_test.go:TestCLINDJSONFailureEndsWithOneTerminalResult, and pkg/transports/http/contracttests/openapi_contract_response_events_test.go:TestOpenAPIContract_FactoryResponseEventPayloadUnionCoversAllVariants. | Local CLI/HTTP contract tests; PR API contract and Linux functional coverage lanes. |
| Concurrent sessions remain isolated while canonical recording order and replay equivalence survive graph retirement. | tests/functional/transport/http/server/concurrent_requests_test.go:TestAPIConcurrentSessionRequestsRemainIsolated, pkg/services/recordings/internal/canonical_recording_lifecycle_test.go:TestRecordingScopesKeepConcurrentSessionsIsolated, and pkg/services/recordings/internal/projection_query_contract_test.go:TestProjectionQueries_AreEquivalentForRetainedAndReplayedCanonicalFacts. | Local integration/recording tests and go test -race; PR backend integration and functional coverage lanes. |
| A throttled in-flight response remains observable until retirement and does not terminate another lane. | pkg/services/factory_runtime/internal/rootobservation/project_test.go:TestProject_ExcludesObservedDispatchResponseFromInFlightProjection, pkg/services/factory_runtime/internal/services/orchestration/subsystems/terminationtests/termination_test.go:TestTerminationCheck_DoesNotTerminateWhileObservedResponseAwaitsRetirement, and tests/functional/runtime_api/api_provider_throttle_pause_observability_test.go:TestProviderErrorSmoke_ThrottleFailureIsolatesOtherLaneThroughPublicSession. | Local Runtime tests with race coverage; PR backend functional coverage lane. |
| Detached Workers execution remains outside Runtime/Session opening and releases effects on every exit. | tests/functional/workers/agent/stateless_root_test.go:TestBuildStatelessWorkersExecutesDetachedAttemptThroughRoot, pkg/wire/session_runtime_providers_test.go:TestBuildStatelessWorkersExecutesBeforeRuntimeOpening, and pkg/wire/session_runtime_providers_test.go:TestCanonicalStatelessWorkersReleasesProductionWorktreeAfterCancellation. | Local Workers/Wire tests; PR root-process acceptance and Linux functional coverage lanes. |

P6 owners run focused package tests, go test -race for stream/attempt
retirement, make verify-fast, make typecheck, make pkg-boundary,
make pkg-structure, and make pkg-file-count while iterating. Before merge,
run make verify-pr, make lint, vet, and the affected API/CLI/MCP/Linux
functional lanes. Static gates support deletion review but do not replace
the behavioral rows.

### P6 sizing, ownership, and dependency

PSS-I01 owns root/Wire/process composition, PSS-I02 owns HTTP, PSS-I03 owns
shared CLI composition, PSS-I04 owns MCP composition, and PSS-I05 owns event
boundary metadata. Runtime, Sessions, Work, Recordings, Visualization, and
Workers own their root contracts and private implementation retirement.
P6-C is the named successor for the final Workers deletion left by P5C.
P6 depends on P3, P4, and all applicable P5 slices; each P6 slice is
independently mergeable and must leave the public process releasable.

## P7 — Close functional and concurrency races

### Outcome and preserved behavior

P7 is the final behavior-closure packet. It proves the P3-P6 composition
against public process, CLI, HTTP, MCP, ACP, replay, and detached Workers
callers after the secondary graph is retired. P7 does not introduce a new
service, adapter, compatibility path, or alternate execution graph. It
converts the audit's remaining behavior gaps into observable functional and
race evidence.

The preserved behavior is: failed activation closes each acquired role
exactly once and retains the primary error; cancellation, timeout, provider
failure, and worker completion produce one stable terminal outcome; terminal
response events preserve identity, ordering, content parts, and reconnect
classification; CLI/API parity holds for shared invocation contracts; ACP
delegation reuses sessions, rejects duplicate/busy work, and terminalizes
close/cancel/failure; detached agent-run preserves its last provider turn,
goal decision envelope, safe diagnostics, and cleanup; replay is equivalent
to retained canonical facts; and concurrent sessions, throttled lanes, and
child dispatches remain isolated.

P7 explicitly covers the audit gaps: full runProcess lifecycle and close-error
joining, a public detached-worker caller guard, a public multi-part Work
terminal response, and one cross-caller terminal-response corpus. A failing
scenario is a product or owning-lane defect, not permission to weaken the
assertion or add a source-inventory test.

The final cross-packet behavioral criterion has one named direct guard:
planned `tests/functional/sessions/root_composition/p3_p7_behavior_matrix_test.go:TestP3P7CanonicalPathPreservesTerminalCleanupAndReplayIsolation`
runs real canonical operations through `root.BuildProcess` and
`Process.Execute`, then asserts one terminal outcome, cleanup after activation,
cancellation, and failure, and equivalent replay facts in isolated sessions.
It is a behavior corpus, not a source or registration inventory; the guard must
run in focused local and race execution and in the PR functional tier before
Story 005 can be considered complete.

### Build first

1. Freeze a behavior matrix keyed by customer operation and observable
   outcome: activation, Work admission, direct/child dispatch, terminal
   result, response stream, replay, cancellation, and cleanup. Record the
   existing assertion and the owning package for every cell.
2. Add or complete characterization at the public root or protocol boundary
   before changing a behavior assertion. Use deterministic edges.Edges,
   command-runner effects, retained event fixtures, and controlled clocks;
   do not use sleeps or timeout padding as synchronization.
3. Run the matrix through root.BuildProcess and Process.Execute for ordinary
   application behavior, root.BuildStatelessWorkers for detached behavior,
   and the protocol contract harnesses only for protocol-owned contracts.
4. Run focused normal and race tests, investigate failures by owner, and
   remove any temporary shadow/comparison fixture after the canonical path
   proves parity. CI evidence is reported in PR comments, never committed.

The canonical path is already the P6-retired graph: one Wire construction,
Initializer lifecycle, owner roots, Runtime active attempts, Recordings
history, and Workers request-scoped Execute. P7 cannot add a fallback to make
a scenario pass. If a residual old path is found, P6 is the named deleting
successor and P7 remains open until that deletion and its characterization
are complete.

### Caller-by-caller migration

P7 does not move production callers, but it enumerates the caller corpus whose
behavior must be observed after P6:

| Caller | Observable scenario | Canonical path and closure evidence |
| --- | --- | --- |
| cmd/factory/main.go:runProcess and root.BuildProcess | Success, domain/provider failure, cancellation, Close, exit code, and cleanup-error joining. | Process.Execute plus Initializer lifecycle; root-process functional cells and lifecycle manager guards. |
| HTTP generated server and SSE response-event routes | Content negotiation, malformed input, isolated concurrent requests, terminal envelopes, reconnect gaps, and active-stream close. | Wire-built owner handlers, Sessions ephemeral response events, and Recordings canonical reads; HTTP contract and functional cells. |
| MCP stdio and service tools | Discovery, malformed arguments, sync/async/not-ready polling, controls, typed failure, and terminal result. | Wire-built raw tool registry and Initializer stdio intent; MCP contract and functional cells. |
| ACP stdio, Chat Sessions, and CLI ACP delegation | Session reuse, redelivery idempotency, busy rejection, cancellation, close, provider failure, and child event identity. | Chat Sessions conversation/control context, Sessions invocation, Providers/Settings configuration, and ACP response bridge. |
| root.BuildStatelessWorkers and detached agent-run | Script/inference/agent success, provider/model/harness failure, timeout/cancel, goal envelope, last-turn output, and resource release. | Workers Service.Execute with injected edges; detached root and Workers functional cells. |
| Runtime direct/child dispatch and Worker Sessions | Exactly one result acceptance, duplicate callback idempotency, terminal mapping, response publication, replay, and unknown callback rejection. | Runtime active-attempt contract, Workers ExecuteResult, Worker Sessions identity, and Runtime integration/race cells. |
| Work and Recordings public reads | Multi-part content order/type, materialization, canonical event order, replay equivalence, cursor/gap classification, and artifact references. | Work owns content policy; Recordings owns history/projection/replay; Work/Recordings contract and functional cells. |

### Strangler sequence and deletion register

P7 is a proof sequence rather than a seam replacement, so it has no
production old path to retain. It still follows the strangler rule:

| Slice | Focus and bound | Exact deletion endpoint or named successor |
| --- | --- | --- |
| P7-A | Public behavior matrix and pre-move gap characterization; 8–14 changed test files. | No production deletion. Any missing migration is returned to its named P6 deletion row; the canonical public test is the successor. |
| P7-B | Terminal response, CLI/API parity, Work content, and protocol contract closure; 12–20 changed files. | Delete temporary shadow/comparison fixtures once the canonical owner assertions pass. No compatibility path may be added to satisfy a fixture. |
| P7-C | Activation failure, cancellation, exactly-once unwind, Runtime/Workers acceptance, and replay races; 12–20 changed files. | Delete temporary race harnesses and one-shot dual-path probes after deterministic race evidence is green. Any old production path found here is deleted by P6, the named successor. |
| P7-D | Final functional/race reruns and owner handoff; 8–16 changed files. | Remove stale test-only adapters and mark the P6 deletion register closed. P7 has no production cleanup endpoint beyond the P6 successor and cannot claim completion with an unnamed old path. |

Every P7 slice leaves main releasable and is independently mergeable. A
behavior assertion may be strengthened, but it may not be replaced by a
source, route, command, registration, inventory, link, or asset scan.

### Characterization and behavioral guards

| Behavior to preserve | Direct guard | Execution tier |
| --- | --- | --- |
| Activation failure closes acquired Runtime/Visualization resources once and preserves the primary error; process close continues through every owner. | tests/functional/sessions/root_composition/application_opening_failure_test.go:TestApplicationOpeningClosesRuntimeWhenVisualizationSinkIsUnavailable, pkg/initializer/lifecycle/manager_test.go:TestManagerUnwindsStartFailureAndJoinsCleanupErrors, and pkg/wire/session_runtime_providers_test.go:TestProcessCloseContinuesThroughEveryLifecycleOwnerAfterFailure. | Local root/lifecycle tests and go test -race where lifecycle state changes; PR root-process acceptance and backend integration lanes. |
| CLI, HTTP, MCP, and ACP retain one terminal response with stable failure/cancellation shape. | tests/functional/transport/cli/process/context_cancellation_test.go:TestCLIContextCancellationEmitsNoSuccessResult, tests/functional/transport/cli/output/ndjson_stream_test.go:TestCLINDJSONFailureEndsWithOneTerminalResult, pkg/services/factory_sessions/transports/mcp/execution_test.go:TestMockClient_RuntimeService_AsyncPollingObservesTerminalResult, and tests/functional/transport/acp/stdio/cli_serve_acp_controls_test.go:TestServeACP_RootBuildProcessCancelTerminalizesOnlyCapturedPrompt. | Local protocol contract tests; PR API/MCP contract and Linux functional coverage lanes. |
| Shared CLI/API invocation success, domain failure, and cancellation remain equivalent. | tests/functional/sessions/lifecycle/remote_lifecycle_test.go:TestCLILocalAndRemoteRunSuccessParityThroughRootProcess, tests/functional/sessions/lifecycle/remote_lifecycle_test.go:TestCLILocalAndRemoteRunDomainFailureParityThroughRootProcess, and tests/functional/sessions/lifecycle/remote_lifecycle_test.go:TestCLILocalAndRemoteRunCancellationParityThroughRootProcess. | Local focused functional cells; PR backend functional short tier and root-process acceptance. |
| HTTP response events and reconnect/replay retain typed ordered payloads and concurrent-session isolation. | pkg/transports/http/contracttests/openapi_contract_response_events_test.go:TestOpenAPIContract_FactoryResponseEventPayloadUnionCoversAllVariants, tests/functional/transport/http/server/concurrent_requests_test.go:TestAPIConcurrentSessionRequestsRemainIsolated, and tests/functional/events/response_events/terminal_outcomes_test.go:TestReadResponseEventStreamUntilTerminalRunOutcomes. | Local HTTP contract/SSE tests and go test -race; PR api-smoke and Linux functional coverage lanes. |
| ACP redelivery, busy control, child attribution, and retained replay do not duplicate or cross streams. | tests/functional/chat_sessions/root_composition/acp_prompt_delegation_test.go:TestACPPromptDelegationRedeliveredRequestMakesNoSecondFactoryDispatch, tests/functional/chat_sessions/root_composition/acp_prompt_delegation_test.go:TestACPPromptDelegationConcurrentPromptRejectsAsBusyWithNoFactoryDispatch, tests/functional/chat_sessions/root_composition/acp_worker_child_events_test.go:TestACPWorkerChildStreamSurvivesRetainedReplay, and tests/functional/transport/acp/stdio/cli_serve_acp_controls_test.go:TestServeACP_RootBuildProcessCloseStopsCapturedFactorySession. | Local Chat Sessions/ACP tests; PR Linux functional coverage and pinned ACP real-client lane. |
| Detached agent-run preserves goal envelope, typed failure, cancellation, terminal output, and resource release without opening Runtime/Session. | tests/functional/workers/agent/stateless_root_test.go:TestBuildStatelessWorkersExecutesDetachedAttemptThroughRoot, pkg/services/workers/wire/stateless_execute_test.go:TestNewServiceExecuteDetachedAgentRunPreservesGoalDecisionEnvelope, pkg/services/workers/internal/services/workstations/executor/agentrun/executor_test.go:TestAgentRunExecutor_TimeoutSurfacesAgentRunTimeoutClass, and pkg/wire/session_runtime_providers_test.go:TestCanonicalStatelessWorkersReleasesProductionWorktreeAfterCancellation. | Local Workers/root tests and race tests for cancellation; PR root-process acceptance and Linux functional coverage lanes. |
| Runtime accepts concurrent/duplicate completion exactly once, preserves terminal mapping, and does not terminate observed in-flight work early. | pkg/services/factory_runtime/internal/services/orchestration/runtime/dispatch_worker_sessions_idempotency_test.go:TestFactoryImpl_ConcurrentAcceptDispatchResultResolvesExactlyOnce, TestFactoryImpl_WorkerSessionCompletionRacesExplicitAcceptanceAndCanonicalReplay, pkg/services/factory_runtime/internal/services/orchestration/runtime/dispatch_worker_sessions_terminal_semantics_test.go:TestFactoryImpl_DirectAndChildDispatchPreserveIdenticalTerminalOutcomeMapping, and pkg/services/factory_runtime/internal/services/orchestration/subsystems/terminationtests/termination_test.go:TestTerminationCheck_DoesNotTerminateWhileObservedResponseAwaitsRetirement. | Local Runtime integration and go test -race; PR backend integration, functional coverage, and race lane. |
| Canonical replay and retained projections are equivalent and isolated across concurrent recording scopes. | pkg/services/recordings/internal/projection_query_contract_test.go:TestProjectionQueries_AreEquivalentForRetainedAndReplayedCanonicalFacts and pkg/services/recordings/internal/projection_query_contract_test.go:TestRecordingScopeQueriesRemainIsolatedAcrossConcurrentScopes, plus pkg/services/recordings/internal/canonical_recording_lifecycle_test.go:TestRecordingScopesKeepConcurrentSessionsIsolated. | Local Recordings tests and race tests; PR backend integration and replay/functional lane. |
| Multi-part Work content reaches a public terminal response without reordering, losing type, or inventing success. | **Planned direct guard:** `tests/functional/transport/http/server/work_terminal_response_test.go:TestWorkTerminalResponsePreservesOrderedTypedContentThroughPublicBoundary` submits a Work whose terminal result contains at least two differently typed and ordered parts (text followed by JSON), asserts both discriminated types and payloads remain in order, and asserts a failure terminal outcome is not reported as success. The existing `pkg/services/work/transports/http/admission_mapping_test.go:TestSubmitWorkResponseToAPI_EncodesDetachedResult`, `pkg/services/work/transports/http/read_mapping_test.go:TestWorkReadModelToAPI_EncodesDetachedReadModel`, and `tests/functional/sessions/root_composition/work_admission_response_stream_test.go:TestSessionsWorkAdmissionAndResponseStreamActivateThroughRootBuildProcessAfterLifecycle` are prerequisites for mapping, read-model, and root activation only; they are not sufficient direct proof. | Local public HTTP/Work functional test; PR API contract and Linux functional coverage lanes. |

P7 owners run focused tests, go test -race for all attempt, stream, replay,
and cleanup changes, make test-root-process-acceptance, make verify-fast,
make typecheck, and the relevant API/MCP smoke targets. Before merge run
make verify-pr, make test-functional, make lint, package structure/boundary
gates, and the affected pinned ACP/provider/worktree lanes. Required CI
evidence belongs in PR comments only.

### P7 sizing, ownership, and dependency

P7 owns cross-boundary functional and race evidence, not product ownership.
PSS-I01 owns root-process fixtures, PSS-I02 HTTP/API contract cells, PSS-I04
MCP cells, PSS-I05 response-event metadata, Chat Sessions owns ACP behavior,
Runtime owns dispatch/termination, Recordings owns replay, Work owns content,
and Workers owns detached execution. P7 depends on P6 and the completed P5
lanes. Each slice is focused, approximately 20 or fewer changed files, and
mergeable on its own; a discovered product defect is routed to its owning
lane instead of being hidden in the closure packet.

## Story 004 secondary-graph and race-closure packet closure

Story 004 is complete when P6 and P7 each contain an outcome, build-first
sequence, exhaustive caller migration, explicit deletion endpoint or named
successor, observable behavioral acceptance, named guards with local and PR
tiers, and bounded ownership/sizing. P6 must retire every P3-P5 temporary
adapter exactly once, and P7 must prove activation failure, exactly-once
unwind, terminal response preservation, replay, detached agent-run, ACP
delegation, and affected concurrent behavior without a meta-test.

## Final P3-P7 reconciliation

The packet dependency order is P3 Runtime ownership, then P4 Factory Sessions
lifecycle, then the independent P5A CLI, P5B HTTP/MCP/ACP, and P5C detached
Workers slices, then P6 secondary-graph deletion, and finally P7 functional
and race closure. No earlier packet depends on a later packet to restore
behavior. P5A, P5B, and P5C name their shared path-lease owners; P6 is the
single deleting successor for residual composition paths, and P7 introduces
no production compatibility path.

The packet definitions cite the authoritative runtime-opening matrix, P1
deletion register, remaining-violations audit, transport-convergence audit,
WSE plan, architecture vocabulary, and planning standard rather than
rewriting them. The current-main audit disagreement and historical
throttled-lane guard remain explicit. Every behavioral row names an observable
guard and execution tier; no packet relies on a source, route, command,
registration, inventory, link, or asset scan as acceptance.

The final reconciliation also requires the named
`TestP3P7CanonicalPathPreservesTerminalCleanupAndReplayIsolation` behavioral
corpus to observe terminal-output preservation, lifecycle cleanup, and
replay/session isolation on the canonical path. This is an observable
outcome assertion with local/race and PR functional execution tiers, not a
structural inventory check.

The authoring diff is documentation-only and contains only this owned plan.
No Go, UI, generated contract, standard, workflow, quarantine, script, or
other live-lane surface is changed. Final delivery requires the synchronized
branch head to be pushed, an open PR to be created from this plan description,
required CI to start on that head, and any blocking conversation to be
addressed; terminal CI and merge remain review-stage responsibilities.

## Story 003 transport and direct-execution packet closure

Story 003 is complete when this document contains P5A, P5B, and P5C packets
that each provide:

- an owned caller set and observable invocation/terminal-response contract;
- prerequisite owner contracts and a build-first strangler sequence;
- a canonical path while compatibility exists and an exact deletion endpoint
  or named successor;
- pre-move characterization requirements for success, relevant failure, and
  terminal response shape, including detached agent-run, ACP delegation, and
  multi-part Work content where the seam reaches them;
- direct behavioral guards with local and PR execution tiers;
- independent mergeability, path-lease ownership, CLI/API parity guidance,
  and approximately 20-file split boundaries; and
- no meta-test requirement based on source, command, route, registration,
  inventory, link, or asset scans.

The packet definitions preserve the dependency order P3 → P4 → P5A/P5B/P5C
and explicitly hand final secondary-graph deletion to P6-C where P5C cannot
safely remove it yet. No production source or generated contract is changed
by this documentation story.

## Story 001 audit closure

Story 001 is complete when this document is present and records:

- the delivered P0, P1, P2B, and P2C packets and the missing P2A packet;
- one reproducible DELIVERED, PARTIAL, or NOT DELIVERED status per row;
- the stale matrix snapshot, missing P2A anchor, and current-tree
  Recordings disagreement;
- the current root, Wire, initializer, and caller composition baseline;
- current behavioral guards, the current-main throttled-lane guard, and
  caller behavior that still lacks a characterization; and
- the handoff rule that later packet authoring must preserve the audit evidence
  and not silently reinterpret P2A or the current ownership boundaries. Story
  002, 003, and 004 now own the P3-P7 packet definitions below.

No production source or generated contract is changed by this audit packet.
