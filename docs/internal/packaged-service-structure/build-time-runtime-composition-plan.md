# Build-Time/Runtime Composition Plan

Status: story 002 runtime-ownership and session-lifecycle packets published.
P5A-P7 remain deferred to the later stories in the authoring PRD.

Audit snapshot: 2026-08-14, with origin/main at 1be29c60d
(Define in-flight semantics for throttled isolated lanes) and this worktree
at 9cc6d1977 before the final synchronization pass. The audit uses the current
main tree for implementation evidence. It does not treat a newer main commit
as a reason to rewrite the historical packet sources.

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
| HTTP startup unwind and concurrent request isolation | tests/functional/transport/http/server/startup_shutdown_test.go and tests/functional/transport/http/concurrent_requests_test.go | HTTP lifecycle and request isolation are covered, although later transport packets still need a complete response corpus. |
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
| Runtime construction is inert and one root can serve isolated openings. | `pkg/services/factory_runtime/wire/wire_test.go:TestNewServiceConstructsInertRoot`, `pkg/root/root_test.go:TestBuildProcessReusesCanonicalRootsAcrossTwoIsolatedExecutions`, and `tests/functional/sessions/root_composition/build_process_inert_test.go:TestSessionsEffectsRemainInertThroughRootBuildProcess`. | Local `go test` for the focused packages; PR backend unit and root-process acceptance lanes. |
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
| The reusable process constructs an inert graph and executes isolated sessions through the same root. | `tests/functional/sessions/root_composition/process_reuse_inert_test.go:TestRootBuildProcessIsInertAndReusableAcrossFactorySessions`, `tests/functional/sessions/root_composition/build_process_inert_test.go:TestSessionsEffectsRemainInertThroughRootBuildProcess`, and `pkg/root/root_test.go:TestBuildProcessReusesCanonicalRootsAcrossTwoIsolatedExecutions`. | Local focused Go tests; PR root-process acceptance and backend unit lanes. |
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
  002 now owns the P3 and P4 packet definitions below.

No production source or generated contract is changed by this audit packet.
