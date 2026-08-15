# Build-Time/Runtime Composition Plan

Status: story 001 audit packet published. The P3-P7 execution packets are
intentionally deferred to the following stories in the authoring PRD.

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

## Sequencing constraints for the next stories

This audit story does not author the P3-P7 packets. The following constraints
must be carried into those stories:

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

## Story 001 completion criteria

Story 001 is complete when this document is present and records:

- the delivered P0, P1, P2B, and P2C packets and the missing P2A packet;
- one reproducible DELIVERED, PARTIAL, or NOT DELIVERED status per row;
- the stale matrix snapshot, missing P2A anchor, and current-tree
  Recordings disagreement;
- the current root, Wire, initializer, and caller composition baseline;
- current behavioral guards, the current-main throttled-lane guard, and
  caller behavior that still lacks a characterization; and
- the rule that P3-P7 packets are authored only by the subsequent stories.

No production source or generated contract is changed by this audit packet.
