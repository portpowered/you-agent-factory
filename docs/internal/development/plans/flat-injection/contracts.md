# Concrete construction and operation contracts

Companion to [plan.md](plan.md). These documentation proposals specify the target; they do not change application code. Current blocks quote source signatures or records from the audited checkout. Task packets contain the additional smaller-owner contract pairs. Two pairs are authored by their lanes as each lane's first required step, before any structural change (operator decision, 2026-10-02): T16's keyed attempt request and T31's `BundleBuilder`/`WorkstationRequestExecutorConfig`. If either cannot be written, that lane stops and returns a delta plan. A deletion does not authorize replacing the container with another aggregate.

Every peer construction alias below belongs in the owning service's outer `wire` package. For a nested `internal` implementation, expose its type first from the nearest legally enclosing `wire` package, then re-export that alias through outer owner Wire; outer Wire cannot directly import a grandchild internal package outside its permitted ancestor. Public private-contract packages can be imported directly where Go permits it. Canonical `pkg/wire` uses those aliases without importing forbidden `internal` packages. Peers continue to consume public service-root contracts; these aliases are construction vocabulary only. Focused providers construct one implementation each, with already selected collaborators, and start no lifecycle activity.

Internal constructor changes are breaking within the repository; focused providers are additive during migration. Migrate all callers and owner fixtures in a releasable increment, register providers in canonical Wire, and run `make generate-wire`. Do not edit `wire_gen.go` manually. Public OpenAPI, CLI grammar, events, configuration, persisted records, and their generated clients remain unchanged. Revert each lane's provider/caller changes together. Temporary compatibility adapters cannot reconstruct graphs or choose fallback effects.

## T01 — Process scheduler seam

Authored source: `pkg/platform/clock/clock.go`, `pkg/services/edges/definition.go`.

Current:

```go
type TimerSource interface {
	Source
	NewTimer(time.Duration) Timer
}
// Edges: Clock platformclock.Source
```

Proposed:

```go
type TimerSource interface {
	Source
	NewTimer(time.Duration) Timer
	After(time.Duration) <-chan time.Time // Real already implements; Deterministic fires on tick advance
}
// Edges: Clock platformclock.Source; ProcessScheduler platformclock.TimerSource
```

Operator decision (2026-10-02): `After` joins the one shared `TimerSource` seam; no per-owner `After` adapters over `NewTimer`. Consumers that need `AfterFunc` or a full clockwork clock (Automations, the watcher `debounceClock`) declare the smallest owner-local interface over `TimerSource` in their owning lane; they do not fake missing methods.


## T05 — Completed Work preparation roles

Authored source: `pkg/services/work/internal/service.go:33-50`.

Current:

```go
func NewService(
 runtimes work.RuntimeResolver,
 readSubmittedFile work.SubmittedFileReader,
 inspectSubmittedFile work.SubmittedFilePathInspector,
 contentStaging work.ContentStagingService,
 contentMaterializer work.ContentMaterializer,
 durability ...work.CompletedFlushSequenceReader,
) work.FileSubmissionService
```

Proposed:

```go
func NewService(
 runtimes work.RuntimeResolver,
 readSubmittedFile work.SubmittedFileReader,
 inspectSubmittedFile work.SubmittedFilePathInspector,
 contentStaging work.ContentStagingService,
 contentMaterializer work.ContentMaterializer,
 stateAccess stateaccess.Service,
 preparation work.RequestPreparationService,
 invocationPreparation work.InvocationInputPreparation,
) work.FileSubmissionService
```

Existing public methods/results remain unchanged. CompletedFlushSequenceReader is a required direct dependency of stateaccess owner, not discarded. Explicit admission-only content implementations preserve current configuration errors. T05 generates Wire only in its later implementation PR.

Authored source: `pkg/services/work/invocation_return_policy_contract.go:337-353`.

Current:

```go
type invocationInputPreparationAdapter struct {
	readFile    SubmittedFileReader
	inspectPath SubmittedFilePathInspector
}

func (adapter invocationInputPreparationAdapter) PrepareInvocationInput(
	ctx context.Context,
	request InvocationInputPreparationRequest,
) (PreparedInvocationInput, error) {
	prepared, err := invocationreturnpolicy.NewInvocationInputPreparation(
		invocationreturnpolicy.InvocationInputFileReader(adapter.readFile),
		invocationreturnpolicy.InvocationInputPathInspector(adapter.inspectPath),
	).PrepareInvocationInput(
		ctx,
		invocationInputPreparationRequestToInternal(request),
	)
	if err != nil {
		return PreparedInvocationInput{}, mapInvocationReturnPolicyError(err)
	}
	return preparedInvocationInputFromInternal(prepared), nil
}
```

Proposed:

```go
type invocationInputPreparationAdapter struct {
 inner invocationreturnpolicy.InvocationInputPreparation
}
func (adapter invocationInputPreparationAdapter) PrepareInvocationInput(
 ctx context.Context,
 request work.InvocationInputPreparationRequest,
) (work.PreparedInvocationInput, error) {
 prepared, err := adapter.inner.PrepareInvocationInput(ctx, invocationInputPreparationRequestToInternal(request))
 if err != nil { return work.PreparedInvocationInput{}, mapInvocationReturnPolicyError(err) }
 return preparedInvocationInputFromInternal(prepared), nil
}
// Focused provider destinations: work/internal/invocationreturnpolicy/wire and
// work/internal/services/invocation_preparation/wire; public mapping and
// its conversion helpers move inside this owner. work/wire composes them.
func NewInvocationInputPolicy(
 readFile invocationreturnpolicy.InvocationInputFileReader,
 inspectPath invocationreturnpolicy.InvocationInputPathInspector,
) invocationreturnpolicy.InvocationInputPreparation
func NewInvocationInputAdapter(
 inner invocationreturnpolicy.InvocationInputPreparation,
) work.InvocationInputPreparation
```

Private policy and public mapping adapter are constructed once. RequestPreparationService separately receives completed ContentPreparation; no operation-time constructor. Preserve original conversion and errors.Is cancellation/deadline semantics at applicationService. No new public protocol. Mapping implementation/converters move into the parent-private invocation_preparation owner; only its public Work interface is returned to peers. No exported Work-root constructor accepts an owner-private policy type.

RequestPreparationService receives completed ContentPreparation separately. Generate Wire only in the later T05 implementation PR; no public generated clients change in this docs amendment.

## T10 — Models fixed collaborators

Authored source: `pkg/services/models/wire/wire.go`, component container.

Current:

```go
type modelsServiceComponents struct {
	runtimeScopes runtimescopes.Service
	assets        scopedassets.Service
	catalog       catalog.Service
	runtimeHost   runtimehost.Service
	inference     inference.Service
}
```

Proposed:

```go
// Removed: modelsServiceComponents and buildModelsServiceComponents.
```

Authored destination: `pkg/services/models/wire`, new focused construction aliases.

Current:

```go
// Not present
```

Proposed:

```go
type RuntimeScopes = runtimescopes.Service
type Assets = scopedassets.Service
type Catalog = catalog.Service
type RuntimeHost = runtimehost.Service
type Inference = inference.Service
```

Expose focused providers corresponding to the existing owner-private `runtime_scopes/wire.NewService`, `assets/wire.NewService`, `catalog/wire.NewService`, `runtime_host/wire.NewService`, and `inference/wire.NewService`. Forward each constructor's complete existing effects individually; do not return a components bag. T10 retains ProcessDependencies declaration/forwarding/consumption as temporary compatibility, removal owner T11; this is not final S01 proof. Keep the old Models Root construction path only until T11's scoped execution operations are available after T10/T30 merge. T10 establishes the independent leaf graph; T11 owns final root cutover.

Authored source: `pkg/services/models/internal/service/runtime_factory.go`, root constructor.

Current:

```go
func NewRoot(
	processLauncher modelhost.ProcessLauncher,
	hostHTTP modelhost.HTTPDoer,
	hostClock modelhost.Clock,
	runtimeRunner platformprocess.CommandRunner,
	runtimeHTTP localmodels.HTTPDoer,
	runtimeInspect localmodels.InspectFile,
	runtimeTempDir localmodels.TempDirectory,
	runtimeTempFile localmodels.CreateTempFile,
	runtimeScopes runtimescopes.Service,
	catalogService modelcatalog.Service,
	assetService scopedassets.Service,
	runtimeHostService runtimehost.Service,
	inferenceService modelinference.Service,
	processDependencies ...modelseffects.ProcessDependencies,
) (*Root, error)
```

Proposed, final T11 cutover:

```go
func NewRoot(
	runtimeScopes runtimescopes.Service,
	catalogService modelcatalog.Service,
	assetService scopedassets.Service,
	runtimeHostService runtimehost.Service,
	inferenceService modelinference.Service,
	pullModel func(context.Context, models.PullModelRequest) (models.PullResult, error),
	invokeLocal func(context.Context, models.LocalInvocationRequest) (models.LocalInvocationResult, error),
) (*Root, error)
```

These injected operations are backed by preconstructed behavior owners, never closures that construct Models services. Remaining Root methods delegate to the existing leaf contracts. Dependencies removed from Root go directly to the consuming leaf; they are not lost. Runtime resource processes, cache state, capacity and cleanup remain under their owners.

## T11 — Models scoped execution and shared capacity state

Authored source: `pkg/services/models/internal/effects/effects.go`.

Current:

```go
type ProcessDependencies struct {
	Logger                     *zap.Logger
	Clock                      func() time.Time
	PullMetrics                PullMetricsRecorder
	RuntimeEvidence            RuntimeEvidenceRecorder
	HostLogger                 HostDiagnosticLogger
	HostMetrics                HostMetricsRecorder
	LocalHooks                 LocalRuntimeHooks
	ResolveHuggingFaceRevision func(context.Context, string) (string, error)
	ResolveBackendArtifact     BackendArtifactResolver
	BackendArtifactPlatform    models.AssetHostPlatform
}
```

Proposed:

```go
// Removed: ProcessDependencies.
```

Authored destination: new private scoped execution operation contract under `models/internal/service`.

Current:

```go
// Not present
```

Proposed:

```go
type ScopedExecution interface {
	PullModelForScope(context.Context, models.PullModelRequest) (models.PullResult, error)
	InvokeLocal(context.Context, models.LocalInvocationRequest) (models.LocalInvocationResult, error)
}
```

This owner receives the existing asset/host/runtime execution effects directly. Its operations resolve binding facts using `request.Scope` and initialize only owned state/resources. Delete `Root.runtimeByScope map[models.RuntimeScopeRef]models.Service`, `scopedRuntime`, and `scopedRuntimeWithBuilder` after migrating both callers (`PullModelForScope` and `InvokeLocal`). Do not implement ScopedExecution by invoking those legacy constructors. Existing public request/result types and scope validation remain unchanged.

T30 exclusively owns the slot-facts/leases cycle in `runtime_host/internal/service/slot_facts.go:NewWired` must become two consumers of one keyed state owner. Remove `adapter.host = host`; leases cannot be constructed against a partially bound host. Runtime Host retains supervision; leases retain admission. The state owner stores supervised readiness and capacity facts, not other services. T11 consumes the independently merged T30 host/leases/coordinator outputs and must render any further changed scoped-execution leaf constructor as a current/proposed pair before implementation; aliases cannot conceal unplanned construction changes.

### T10 slot state and lease coordination cycle cut

> Validation review: this pair is owned by **T30** (split from T10). It sits under the T11 heading only for historical reasons. T11 consumes it.

The same runtime-host constructor also invokes `leaseswire.BindCoordinator(leases, s)` after host creation. Removing only `slotFactsAdapter.host` leaves a second hidden cycle. Move readiness/holder/timer state into one state owner and move its timer/holder behavior into a directly injected coordinator. Host supervision and lease admission consume that state; the coordinator never captures the final host.

Authored destination: `runtime_host/internal/service`; these native declarations are new.

Current:

```go
// Not present
```

Proposed:

```go
// State only; owned process/timer handles retain their per-slot lifetime.
type SlotState struct {
 mu sync.Mutex
 runtimeSlots map[string]*supervisedRuntime
 capacityHolders map[string]int
 idleUnloadTimers map[string]*idleUnload
}
func NewSlotState() *SlotState
func NewSlotFacts(
 scopes runtimescopes.Service,
 assets scopedassets.Service,
 state *SlotState,
) modelseffects.SlotFactsProvider
func NewSlotCoordinator(
 state *SlotState,
 scopes runtimescopes.Service,
 clock modelseffects.HostClock,
 logger modelseffects.HostDiagnosticLogger,
 metrics modelseffects.HostMetricsRecorder,
 idleUnloadAfter time.Duration,
) modelseffects.SlotCapacityCoordinator
```

The coordinator owns holder-change/idle scheduling against the shared keyed handles. Preserve current overlay identity checks, loading contention, timer cancellation, stop/join and eviction policy. It calls handle resource operations, not a callback into an incompletely constructed host. Expose SlotState via runtime_host/wire before re-exporting through models/wire.

Authored source: `pkg/services/models/internal/services/runtime_host/internal/service/service.go`.

Current:

```go
func New(
	scopes runtimescopes.Service,
	assets scopedassets.Service,
	leases hostleases.Service,
	processLauncher modelseffects.HostProcessLauncher,
	hostHTTP modelseffects.HostHTTPDoer,
	hostClock modelseffects.HostClock,
	hostLogger modelseffects.HostDiagnosticLogger,
	hostMetrics modelseffects.HostMetricsRecorder,
	options ...runtimehost.Options,
) runtimehost.Service
```

Proposed:

```go
func New(
 scopes runtimescopes.Service,
 assets scopedassets.Service,
 leases hostleases.Service,
 state *SlotState,
 processLauncher modelseffects.HostProcessLauncher,
 hostHTTP modelseffects.HostHTTPDoer,
 hostClock modelseffects.HostClock,
 hostLogger modelseffects.HostDiagnosticLogger,
 hostMetrics modelseffects.HostMetricsRecorder,
 platform models.AssetHostPlatform,
 protocol modelseffects.HostProtocolNegotiator,
 compatibility modelseffects.HostCompatibilityChecker,
 resolveSymlinks modelseffects.HostResolveSymlinks,
 evidence modelseffects.RuntimeEvidenceRecorder,
 idleUnloadAfter time.Duration,
 maxLoadedRuntimes int,
) runtimehost.Service
```

Delete runtimehost.Options as a mixed effect/config container; idle duration/capacity are still validated configuration values. The host stores injected state rather than creating duplicate maps and does not BindCoordinator. Delete NewWired after its callers receive these independent outputs.

Authored source: `pkg/services/models/internal/services/runtime_host/internal/services/leases/wire/wire.go`.

Current:

```go
func NewService(
	hostClock modelseffects.HostClock,
	slotFacts modelseffects.SlotFactsProvider,
) (hostleases.Service, error)
```

Proposed:

```go
func NewService(
 hostClock modelseffects.HostClock,
 slotFacts modelseffects.SlotFactsProvider,
 coordinator modelseffects.SlotCapacityCoordinator,
) (hostleases.Service, error)
```

Remove BindCoordinator, CoordinatorBindable and the mutable coordinator setter; private leases constructor takes coordinator directly. Preserve required boundary validation and valid internal assignment. Characterize lease expiry, repeated release, idle unloading and overlapping scope capacity before extraction. No mutex or timer is shared across unrelated slot identities accidentally.

## T12 — Automations runtime/source isolation

**AM16 / T12 — approved private behavior-owner decomposition (binding 2026-10-03T10:20Z).** Authority: `C:/Users/andre/work/portos/infinite-you/docs/temp/operator-mailbox/responses/flat-injection.md`, Decision 2026-10-03T10:20Z; this supersedes the 09:05Z deferral only. Retain draft [#2683](https://github.com/portpowered/you-agent-factory/pull/2683) at `62cff1c7cfb4d734d462aa1ef08d2d5e0e6c4ba7`; its stories 002/003 and full F10/S01 remain incomplete. The separately authorized `fi-plan-amendment-am16-t12-20261003` docs lane depends on merged v1.1 #2685 and owns only this six-companion packet. AM16 is now merged historical authority; the retained T12 successor consumes merged AM17 before planning, with no historical AM16 Work dependency or T20/WSV wait. T12 owns implementation, same-PR stale script_pollers baseline deletion and obsolete-helper removal. T20 owns unchanged one-interface enforcement and measurement; deadcode allowance remains 0. No public API, configuration, event, persistence-policy or acceptance change is authorized. Native capability/provider/caller/removal pairs C01–C16 are canonical in [contracts.md](contracts.md#t12--automations-runtimesource-isolation). The docs author handoff requires final head pushed, open PR, CI started and blocking feedback addressed; independent AM16-DOC-VAL/REVIEW owns clean-room loopback, terminal CI and current-main merge. The successor still requires exact-head T12-F10 (F10a–m), T12-S01 and T12-G01/G02, independent review and merge. Historical partial evidence never substitutes for those gates.

### AM17 — T12 evidence boundaries

Binding authority: operator mailbox `C:/Users/andre/work/portos/infinite-you/docs/temp/operator-mailbox/responses/flat-injection.md`, Decisions 2026-10-03T12:01Z A and 12:57Z. AM17 corrects evidence classification under flat-injection-v1.1; it changes no endpoint, service signature, configuration, event, persistence, policy or acceptance criterion. AM16 is merged historical authority, not a live Work dependency. The retained T12 successor consumes merged AM17 before planning and continues from [#2683](https://github.com/portpowered/you-agent-factory/pull/2683) / `62cff1c7cfb4d734d462aa1ef08d2d5e0e6c4ba7`; stories 002/003 and full F10/S01 remain unproven. No T20/WSV wait or queue mutation is required.

Follow [the AM17 resolution](plan.md#authorized-amendment-flat-injection-v11) to [the complete F10 requirements ledger](tasks.md#am17--t12-evidence-boundaries). T12 owns both proofs below; independent VAL01 confirms composed behavior. The existing `automations.Root.GetCursor` delegates the service operation (`pkg/services/automations/contracts.go:215–222,457–474`). Durable reads validate `ExpectedCursor` and return detached opaque facts (`pkg/services/automations/internal/services/script_pollers/internal/service/durable_cursor.go:74–108`). The cursor HTTP adapter (`pkg/services/automations/transports/http/convergence_operations.go:52–97`) is unbound in canonical Wire/HTTP; adapter tests are component evidence. Neither that adapter nor `AutomationsRootFromEdges` is a composed functional entry point. Canonical construction/execution remains `pkg/root/process.go:17` / `pkg/initializer/application/process.go:185`.

| Obligation | Component/service-contract evidence — T12-F10-COMPONENT | Composed evidence — T12-F10 / VAL01 | Owner |
| --- | --- | --- | --- |
| F10b | Existing GetCursor returns exact committed opaque cursor/checkpoint facts | Existing customer activation admits public Work; resumed command receives the same exact committed environment facts | T12; both proofs required |
| F10g | After failed replacement, GetCursor returns prior exact committed opaque cursor/checkpoint facts | Admitted public Work remains; existing error/diagnostic is visible; next command resumes prior-commit environment facts; peer progresses | T12; both proofs required |
| F10j | GetCursor with stale ExpectedCursor returns the existing typed conflict; exact committed cursor/checkpoint facts remain unchanged; no mutation, extra command or Work | No composed public cursor endpoint is required or invented; admission/restart/diagnostic/isolation obligations remain in F10b/g and the other F10 rows | T12 component/service-contract proof |

All F10a–m success, failure and recovery obligations remain required on the successor head. C01–C16 native shapes, exactly one named interface per private service root, same-PR obsolete-helper/stale-baseline deletion, deletion-only baselines, deadcode allowance 0, T12-S01/G01/G02 and FI-A1–A8 remain unchanged. This amendment yields documentation/source/static evidence only, never runtime PASS.

AM17-DOC-TRACE/INVARIANTS/LINT belong to the docs author. Author delivery stops at final head pushed, PR open, required CI started and blocking feedback addressed. Independent ordinary review owns AM17-DOC-VAL: a fresh isolated candidate checkout, the [validation loopback report](../../../../../factory/docs/standards/validation-loopback-template.md) with AM17-1..6, AM17-DELIVERY and FI-A1–A8 individually PASS/FAIL/BLOCKED, and a smallest delta request for findings without silent repair. AM17-DOC-REVIEW owns terminal passing own-head CI, conflicts, current-main rebase, immediate premerge `make lint pkg-file-count`, and merge. `make generate-wire` plus clean diff applies only to a separately authorized graph change. Merge releases corrected successor planning; runtime gates and aggregate Project acceptance remain later work.

### AM16 construction and lifetime

Current shapes below are focused native declaration excerpts from the explicitly named source basis; imports and bodies are omitted. Retained #2683 is the successor starting point, not current-main runtime evidence. This docs lane changes no Go or generated files.

Construct recovery C04/C07 → script C05/C06/C12; independently construct cron/watch/hosted C16 → lifecycle C01/C08 → reconciliation C02/C03 → owner C09 → public Root C14. Canonical `pkg/wire` supplies each completed behavior once through owner Wire aliases. Neither lifecycle nor reconciliation depends on the final owner. No recursive provider, parent callback, SourceDriver concrete alias, RuntimeSourceRegistration interface, dependency bag or service getter is introduced.

Root activation allocates runtime identity/config/context/watcher resources C15, calls lifecycle ConfigureRuntimeSource with those domain facts, then reconciliation runtime-keyed Start/Wait. Deactivation calls Stop/Wait, joins all source users, then ReleaseRuntimeSource and recovery ReleaseScope. Preserve old-instance guards so an old runtime cannot release replacement state. Blank-base memory behavior, durable destinations/bytes, cursor conflicts, checkpoint ordering, hosted retry/redaction and Now-only public caller acceptance remain unchanged. C06 consumes the existing selected clockwork view; retain T01 TimerSource/ClockView and override precedence without real-time fallback.


### C01 — Lifecycle and registration have one private behavior owner

Authored source: `pkg/services/automations/internal/services/sourcelifecycle/service.go (new root)`.

Source basis: new package absent on main and retained head.

Source locators: New root absent from retained tree; proposed `sourcelifecycle/service.go` above.

Owner: AM16 docs: fi-plan-amendment-am16-t12-20261003; implementation/removal: T12; checker unchanged: T20.

Current:

```go
# Not present
```

Proposed:

```go
package sourcelifecycle

type RuntimeSourceConfiguration struct {
	RuntimeID string
	FactorySessionID string
	Snapshot factorydefinitions.RuntimeSnapshot
	Inputs automations.RuntimeActivationInputs
}

type StartEffect struct {
	RuntimeID   string
	Kind        string
	Observation automations.SourceObservation
}

type StopEffect struct {
	RuntimeID   string
	Observation automations.SourceObservation
}

type WaitEffect struct {
	RuntimeID   string
	Desired     automations.DesiredLifecycleState
	Observation automations.SourceObservation
}

type SourceLifecycle interface {
	ConfigureRuntimeSource(context.Context, RuntimeSourceConfiguration) error
	ReleaseRuntimeSource(context.Context, string) error
	Start(context.Context, StartEffect) error
	Stop(context.Context, StopEffect) error
	Wait(context.Context, WaitEffect) (automations.SourceObservation, error)
}
```

Compatibility and rollout: Docs specification only in this lane; T12 applies the private delta in its retained successor. Public Automations API, CLI, configuration, event/status fields, cursor schema/destination, Now-only caller acceptance and persistence fallback policy remain unchanged. No migration or feature flag.

Callers/consumers: `pkg/services/automations/internal/services/reconciliation/internal/service`; `pkg/services/automations/internal/services/reconciliation/wire`; `pkg/services/automations/wire; root activation through focused wire alias`.

Generated outputs: None in docs lane; T12 regenerates pkg/wire/wire_gen.go with make generate-wire.

Removal and migration owner: T12 moves schedulerSources/start/stop/wait and registration into sourcelifecycle/internal/service. Remove root callbacks and schedulerLifecycle bridge; no parent-Service dependency.

### C02 — Reconciliation owns runtime controls on its existing single Service

Authored source: `pkg/services/automations/internal/services/reconciliation/service.go`.

Source basis: retained #2683/62cff1c7cfb4d734d462aa1ef08d2d5e0e6c4ba7.

Source locators: `pkg/services/automations/internal/services/reconciliation/service.go:15` (Service), `:28` (RuntimeSourceControl), `:37` (SourceLifecycle).

Owner: AM16 docs: fi-plan-amendment-am16-t12-20261003; implementation/removal: T12; checker unchanged: T20.

Current:

```go
type Service interface {
	RuntimeSourceControl
	Reconcile(context.Context, automations.ReconcileRequest) (automations.ReconcileResult, error)
	StartSource(context.Context, automations.StartSourceRequest) (automations.StartSourceResult, error)
	StopSource(context.Context, automations.StopSourceRequest) (automations.StopSourceResult, error)
	WaitSource(context.Context, automations.WaitSourceRequest) (automations.WaitSourceResult, error)
	SourceStatus(context.Context, automations.SourceStatusRequest) (automations.SourceStatusResult, error)
	GetStatus(context.Context, automations.GetStatusRequest) (automations.GetStatusResult, error)
	GetCursor(context.Context, automations.GetCursorRequest) (automations.GetCursorResult, error)
}

type RuntimeSourceControl interface {
	StartSourceForRuntime(context.Context, string, automations.StartSourceRequest) (automations.StartSourceResult, error)
	StopSourceForRuntime(context.Context, string, automations.StopSourceRequest) (automations.StopSourceResult, error)
	WaitSourceForRuntime(context.Context, string, automations.WaitSourceRequest) (automations.WaitSourceResult, error)
}

type SourceLifecycle interface {
	Start(context.Context, StartEffect) error
	Stop(context.Context, StopEffect) error
	Wait(context.Context, WaitEffect) (automations.SourceObservation, error)
}

type StartEffect struct {
	RuntimeID   string
	Kind        string
	Observation automations.SourceObservation
}

type StopEffect struct {
	RuntimeID   string
	Observation automations.SourceObservation
}

type WaitEffect struct {
	RuntimeID   string
	Desired     automations.DesiredLifecycleState
	Observation automations.SourceObservation
}
```

Proposed:

```go
package reconciliation

type Service interface {
	StartSourceForRuntime(context.Context, string, automations.StartSourceRequest) (automations.StartSourceResult, error)
	StopSourceForRuntime(context.Context, string, automations.StopSourceRequest) (automations.StopSourceResult, error)
	WaitSourceForRuntime(context.Context, string, automations.WaitSourceRequest) (automations.WaitSourceResult, error)
	Reconcile(context.Context, automations.ReconcileRequest) (automations.ReconcileResult, error)
	StartSource(context.Context, automations.StartSourceRequest) (automations.StartSourceResult, error)
	StopSource(context.Context, automations.StopSourceRequest) (automations.StopSourceResult, error)
	WaitSource(context.Context, automations.WaitSourceRequest) (automations.WaitSourceResult, error)
	SourceStatus(context.Context, automations.SourceStatusRequest) (automations.SourceStatusResult, error)
	GetStatus(context.Context, automations.GetStatusRequest) (automations.GetStatusResult, error)
	GetCursor(context.Context, automations.GetCursorRequest) (automations.GetCursorResult, error)
}

// Value aliases preserve identity without a second representation.
type StartEffect = sourcelifecycle.StartEffect
type StopEffect = sourcelifecycle.StopEffect
type WaitEffect = sourcelifecycle.WaitEffect
```

Compatibility and rollout: Docs specification only in this lane; T12 applies the private delta in its retained successor. Public Automations API, CLI, configuration, event/status fields, cursor schema/destination, Now-only caller acceptance and persistence fallback policy remain unchanged. No migration or feature flag.

Callers/consumers: `pkg/services/automations/internal/runtime_sidecars.go`; `pkg/services/automations/internal/runtime_lifecycle.go`; `pkg/services/automations/internal/services/reconciliation/internal/service`; `pkg/services/automations/internal/services/reconciliation/wire`.

Generated outputs: None in docs lane; T12 regenerates pkg/wire/wire_gen.go with make generate-wire.

Removal and migration owner: T12 deletes named RuntimeSourceControl/SourceLifecycle from reconciliation root. Do not add RuntimeSourceRegistration there; configure/release belongs to the lifecycle owner called directly by root activation. Reconciliation owns the three existing runtime-keyed control methods, inlined on Service.

### C03 — Reconciliation consumes the completed lifecycle owner directly

Authored source: `pkg/services/automations/internal/services/reconciliation/wire/wire.go; internal/service/service.go`.

Source basis: retained #2683/62cff1c7cfb4d734d462aa1ef08d2d5e0e6c4ba7.

Source locators: `pkg/services/automations/internal/services/reconciliation/wire/wire.go:11`; `pkg/services/automations/internal/services/reconciliation/internal/service/service.go:26`.

Owner: AM16 docs: fi-plan-amendment-am16-t12-20261003; implementation/removal: T12; checker unchanged: T20.

Current:

```go
func NewService(lifecycle reconciliation.SourceLifecycle) reconciliation.Service

func New(lifecycle reconciliation.SourceLifecycle) reconciliation.Service
```

Proposed:

```go
func NewService(lifecycle sourcelifecycle.SourceLifecycle) reconciliation.Service

func New(lifecycle sourcelifecycle.SourceLifecycle) reconciliation.Service
```

Compatibility and rollout: Docs specification only in this lane; T12 applies the private delta in its retained successor. Public Automations API, CLI, configuration, event/status fields, cursor schema/destination, Now-only caller acceptance and persistence fallback policy remain unchanged. No migration or feature flag.

Callers/consumers: `pkg/services/automations/wire.NewReconciliation`; `pkg/services/automations/internal/services/reconciliation/owner fixtures`.

Generated outputs: None in docs lane; T12 regenerates pkg/wire/wire_gen.go with make generate-wire.

Removal and migration owner: T12 deletes Effects variadic/default path and intermediate callback adapter; implementation stores the lifecycle owner directly.

### C04 — Cursor recovery has one private owner

Authored source: `pkg/services/automations/internal/services/cursorscopes/service.go (new root)`.

Source basis: new package absent on main and retained head.

Source locators: New root absent from retained tree; recovery values originate at `pkg/services/automations/internal/services/script_pollers/cursor.go:43` (scope), `:49` (interface), `:60` (commit), `:69` (resume).

Owner: AM16 docs: fi-plan-amendment-am16-t12-20261003; implementation/removal: T12; checker unchanged: T20.

Current:

```go
# Not present
```

Proposed:

```go
package cursorscopes

type CursorScope struct {
	RuntimeID string
	BaseDir   string
}

type CommitCursorRequest struct {
	AutomationID   string
	InstanceID     string
	ExpectedCursor automations.Cursor
	Cursor         automations.Cursor
	Checkpoint     string
}

type ResumeCursor struct {
	Cursor     automations.Cursor
	Checkpoint string
}

type CursorScopes interface {
	GetCursor(context.Context, CursorScope, automations.GetCursorRequest) (automations.GetCursorResult, error)
	CommitCursor(context.Context, CursorScope, CommitCursorRequest) error
	// ReleaseScope discards runtime resources after the caller has stopped and
	// joined all scope users. Durable recovery files remain available on restart.
	ReleaseScope(CursorScope)
}
```

Compatibility and rollout: Docs specification only in this lane; T12 applies the private delta in its retained successor. Public Automations API, CLI, configuration, event/status fields, cursor schema/destination, Now-only caller acceptance and persistence fallback policy remain unchanged. No migration or feature flag.

Callers/consumers: `pkg/services/automations/internal/services/script_pollers/internal/service`; `pkg/services/automations/internal/services/script_pollers/wire`; `pkg/services/automations/internal/services/sourcelifecycle/internal/service; release after stop/join`.

Generated outputs: None in docs lane; T12 regenerates pkg/wire/wire_gen.go with make generate-wire.

Removal and migration owner: T12 moves cursor_scopes.go and durable serializer/resource operations under cursorscopes/internal/service; preserve bytes/path and retained race/cursor tests. One named CursorScopes interface in this root.

### C05 — Scoped cursor reads stay on script supervision Service

Authored source: `pkg/services/automations/internal/services/script_pollers/service.go; cursor.go`.

Source basis: retained #2683/62cff1c7cfb4d734d462aa1ef08d2d5e0e6c4ba7.

Source locators: `pkg/services/automations/internal/services/script_pollers/service.go:25` (Service), `:49` (scoped reader), `:54` (Scheduler); `pkg/services/automations/internal/services/script_pollers/cursor.go:36` (recorder), `:49` (scopes).

Owner: AM16 docs: fi-plan-amendment-am16-t12-20261003; implementation/removal: T12; checker unchanged: T20.

Current:

```go
type Service interface {
	ScopedCursorReader
	GetCursor(context.Context, automations.GetCursorRequest) (automations.GetCursorResult, error)
	StartScriptPoller(
		context.Context,
		*sync.WaitGroup,
		factorydefinitions.RuntimeConfigLookup,
		factorydefinitions.FactoryWorkstationConfig,
		*factorydefinitions.FactoryWorkerConfig,
		ScriptPollerSupervision,
		automations.WorkRequestSubmitter,
	)
	RunScriptPoller(
		context.Context,
		platformprocess.CommandRunner,
		factorydefinitions.RuntimeConfigLookup,
		factorydefinitions.FactoryWorkstationConfig,
		*factorydefinitions.FactoryWorkerConfig,
		ScriptPollerSupervision,
		automations.WorkRequestSubmitter,
	) error
}

type ScopedCursorReader interface {
	GetCursorForScope(context.Context, CursorScope, automations.GetCursorRequest) (automations.GetCursorResult, error)
}

type Scheduler interface {
	Now() time.Time
	After(time.Duration) <-chan time.Time
}

type CursorScopes interface {
	GetCursor(context.Context, CursorScope, automations.GetCursorRequest) (automations.GetCursorResult, error)
	CommitCursor(context.Context, CursorScope, CommitCursorRequest) error
	// ReleaseScope discards runtime resources after the caller has stopped and
	// joined all scope users. Durable recovery files remain available on restart.
	ReleaseScope(CursorScope)
}

type CursorRecorder interface {
	GetCursor(context.Context, automations.GetCursorRequest) (automations.GetCursorResult, error)
	CommitCursor(context.Context, CommitCursorRequest) error
}
```

Proposed:

```go
package script_pollers

type Service interface {
	GetCursorForScope(context.Context, CursorScope, automations.GetCursorRequest) (automations.GetCursorResult, error)
	GetCursor(context.Context, automations.GetCursorRequest) (automations.GetCursorResult, error)
	StartScriptPoller(
		context.Context,
		*sync.WaitGroup,
		factorydefinitions.RuntimeConfigLookup,
		factorydefinitions.FactoryWorkstationConfig,
		*factorydefinitions.FactoryWorkerConfig,
		ScriptPollerSupervision,
		automations.WorkRequestSubmitter,
	)
	RunScriptPoller(
		context.Context,
		platformprocess.CommandRunner,
		factorydefinitions.RuntimeConfigLookup,
		factorydefinitions.FactoryWorkstationConfig,
		*factorydefinitions.FactoryWorkerConfig,
		ScriptPollerSupervision,
		automations.WorkRequestSubmitter,
	) error
}

// Domain value aliases; no new named interface in this root.
type CursorScope = cursorscopes.CursorScope
type CommitCursorRequest = cursorscopes.CommitCursorRequest
type ResumeCursor = cursorscopes.ResumeCursor

// Removed: ScopedCursorReader, Scheduler, CursorScopes, CursorRecorder.
```

Compatibility and rollout: Docs specification only in this lane; T12 applies the private delta in its retained successor. Public Automations API, CLI, configuration, event/status fields, cursor schema/destination, Now-only caller acceptance and persistence fallback policy remain unchanged. No migration or feature flag.

Callers/consumers: `pkg/services/automations/internal cursor/supervision routing`; `pkg/services/automations/internal/services/script_pollers/internal/service`; `pkg/services/automations/internal/services/script_pollers/wire`; `pkg/services/automations/wire`.

Generated outputs: None in docs lane; T12 regenerates pkg/wire/wire_gen.go with make generate-wire.

Removal and migration owner: T12 deletes the four non-Service interface declarations. RuntimeSourceControl and ScopedCursorReader are existing-owner method sets; they gain no redundant forwarding services. Scheduler uses existing library clockwork.Clock, already required by the retained internal Automations Clock; no new clock interface root or real-time selection is introduced.

### C06 — Script provider consumes recovery owner and existing selected clock view

Authored source: `pkg/services/automations/internal/services/script_pollers/wire/wire.go; internal/service/service.go`.

Source basis: retained #2683/62cff1c7cfb4d734d462aa1ef08d2d5e0e6c4ba7.

Source locators: `pkg/services/automations/internal/services/script_pollers/wire/wire.go:23`; `pkg/services/automations/internal/services/script_pollers/internal/service/service.go:34`.

Owner: AM16 docs: fi-plan-amendment-am16-t12-20261003; implementation/removal: T12; checker unchanged: T20.

Current:

```go
func NewService(
	logger *zap.Logger,
	scheduler scriptpollers.Scheduler,
	commandRunner platformprocess.CommandRunner,
	resolveTemplates workers.TemplateFieldResolver,
	executionPolicy factorydefinitions.WorkstationExecutionPolicyService,
	cursors scriptpollers.CursorScopes,
) scriptpollers.Service

func New(
	logger *zap.Logger,
	scheduler scriptpollers.Scheduler,
	commandRunner platformprocess.CommandRunner,
	resolveTemplates workers.TemplateFieldResolver,
	executionPolicy factorydefinitions.WorkstationExecutionPolicyService,
	cursors scriptpollers.CursorScopes,
) scriptpollers.Service
```

Proposed:

```go
func NewService(
	logger *zap.Logger,
	scheduler clockwork.Clock,
	commandRunner platformprocess.CommandRunner,
	resolveTemplates workers.TemplateFieldResolver,
	executionPolicy factorydefinitions.WorkstationExecutionPolicyService,
	cursors cursorscopes.CursorScopes,
) scriptpollers.Service

func New(
	logger *zap.Logger,
	scheduler clockwork.Clock,
	commandRunner platformprocess.CommandRunner,
	resolveTemplates workers.TemplateFieldResolver,
	executionPolicy factorydefinitions.WorkstationExecutionPolicyService,
	cursors cursorscopes.CursorScopes,
) scriptpollers.Service
```

Compatibility and rollout: Docs specification only in this lane; T12 applies the private delta in its retained successor. Public Automations API, CLI, configuration, event/status fields, cursor schema/destination, Now-only caller acceptance and persistence fallback policy remain unchanged. No migration or feature flag.

Callers/consumers: `pkg/services/automations/wire.NewScriptPollers`; `pkg/services/automations/internal/services/script_pollers/unit fixtures`.

Generated outputs: None in docs lane; T12 regenerates pkg/wire/wire_gen.go with make generate-wire.

Removal and migration owner: T12 removes scriptpollers.Scheduler and scriptpollers.CursorScopes parameter references. clockwork.Clock is the existing external gocron/time boundary view, not a new service capability. Preserve T01 selected TimerSource/ClockView and override precedence obligations; do not copy retained selectLegacyScheduler fallback into the final graph.

### C07 — Focused recovery provider exposes the private owner through Wire

Authored source: `pkg/services/automations/internal/services/cursorscopes/wire/wire.go (new); pkg/services/automations/internal/services/script_pollers/wire/wire.go`.

Source basis: retained #2683/62cff1c7cfb4d734d462aa1ef08d2d5e0e6c4ba7.

Source locators: `pkg/services/automations/internal/services/script_pollers/wire/wire.go:18`; proposed recovery provider absent.

Owner: AM16 docs: fi-plan-amendment-am16-t12-20261003; implementation/removal: T12; checker unchanged: T20.

Current:

```go
func NewCursorScopes(files CursorPersistenceFileSystem) scriptpollers.CursorScopes
```

Proposed:

```go
// cursorscopes/wire/wire.go
type CursorPersistenceFileSystem = cursorscopesservice.CursorPersistenceFileSystem
func NewService(files CursorPersistenceFileSystem) cursorscopes.CursorScopes

// Removed: script_pollers/wire.NewCursorScopes; callers use the recovery owner.
```

Compatibility and rollout: Docs specification only in this lane; T12 applies the private delta in its retained successor. Public Automations API, CLI, configuration, event/status fields, cursor schema/destination, Now-only caller acceptance and persistence fallback policy remain unchanged. No migration or feature flag.

Callers/consumers: `pkg/services/automations/wire.NewCursorScopes`; `pkg/services/automations/internal/services/script_pollers/internal/service`; `pkg/services/automations/internal/services/sourcelifecycle/internal/service`.

Generated outputs: None in docs lane; T12 regenerates pkg/wire/wire_gen.go with make generate-wire.

Removal and migration owner: T12 moves the unchanged filesystem effect definition to cursorscopes/internal/service and exposes it through recovery Wire; no public FS contract change. Remove the old script_pollers/wire.NewCursorScopes wrapper and migrate recovery fixtures to their owner. Each provider constructs exactly one completed implementation.

### C08 — Focused lifecycle provider exposes its single interface

Authored source: `pkg/services/automations/internal/services/sourcelifecycle/wire/wire.go (new); pkg/services/automations/wire/wire.go`.

Source basis: providers absent on retained head; supersedes retained PRD planned SourceDriver/RuntimeSourceRegistration.

Source locators: Both focused lifecycle/reconciliation exports absent from retained tree; sources are the new Wire paths above.

Owner: AM16 docs: fi-plan-amendment-am16-t12-20261003; implementation/removal: T12; checker unchanged: T20.

Current:

```go
# Not present
```

Proposed:

```go
// sourcelifecycle/wire/wire.go; private implementation has the same direct inputs.
func NewService(logger *zap.Logger, clock clockwork.Clock,
	scriptPollers scriptpollers.Service, cronService cron.Service,
	filesystemWatchers filesystemwatchers.Service,
	hostedPollers automations.HostedPollers,
	cursors cursorscopes.CursorScopes) sourcelifecycle.SourceLifecycle

// automations/wire/wire.go; one provider per behavior, no SourceDriver concrete alias.
type SourceLifecycle = sourcelifecycle.SourceLifecycle
type Reconciliation = reconciliation.Service
type CursorScopes = cursorscopes.CursorScopes
func NewSourceLifecycle(logger *zap.Logger, clock Clock,
	scriptPollers ScriptPollers, cronService Cron,
	filesystemWatchers FilesystemWatchers,
	hostedPollers automations.HostedPollers,
	cursors CursorScopes) SourceLifecycle
func NewReconciliation(lifecycle SourceLifecycle) Reconciliation
```

Compatibility and rollout: Docs specification only in this lane; T12 applies the private delta in its retained successor. Public Automations API, CLI, configuration, event/status fields, cursor schema/destination, Now-only caller acceptance and persistence fallback policy remain unchanged. No migration or feature flag.

Callers/consumers: `pkg/wire named Automations providers`; `pkg/services/automations/wire focused set; owner fixtures`.

Generated outputs: None in docs lane; T12 regenerates pkg/wire/wire_gen.go with make generate-wire.

Removal and migration owner: T12 replaces retained PRD SourceDriver/RuntimeSourceRegistration interface outputs with the completed SourceLifecycle interface. ConfigureRuntimeSource/ReleaseRuntimeSource are its cohesive registration/lifetime methods; no second named interface or root callback.

### C09 — Automations root receives completed lifecycle and reconciliation owners

Authored source: `pkg/services/automations/internal/service.go; pkg/services/automations/wire/wire.go`.

Source basis: retained #2683/62cff1c7cfb4d734d462aa1ef08d2d5e0e6c4ba7.

Source locators: `pkg/services/automations/internal/service.go:125`; `pkg/services/automations/wire/wire.go:57` (Root), `:91` (Service).

Owner: AM16 docs: fi-plan-amendment-am16-t12-20261003; implementation/removal: T12; checker unchanged: T20.

Current:

```go
func newService(
	logger *zap.Logger,
	clock Clock,
	commandRunner platformprocess.CommandRunner,
	workflowID string,
	defaultFactoryDir string,
	hostedPollers automations.HostedPollers,
	resolveTemplates workers.TemplateFieldResolver,
	executionPolicy factorydefinitions.WorkstationExecutionPolicyService,
	cursorFileSystem scriptpollerswire.CursorPersistenceFileSystem,
	cronService cron.Service,
	filesystemWatchers filesystemwatchers.Service,
	pollers scriptpollers.Service,
	cursors scriptpollers.CursorScopes,
) *Service

func NewRoot(
	logger *zap.Logger,
	clock automations.Clock,
	commandRunner platformprocess.CommandRunner,
	workflowID string,
	defaultFactoryDir string,
	hosted HostedSourceInputs,
	resolveTemplates workers.TemplateFieldResolver,
	executionPolicy factorydefinitions.WorkstationExecutionPolicyService,
) (automations.Root, error)

func NewService(
	logger *zap.Logger,
	clock automations.Clock,
	commandRunner platformprocess.CommandRunner,
	workflowID string,
	defaultFactoryDir string,
	hostedPollers automations.HostedPollers,
	resolveTemplates workers.TemplateFieldResolver,
	executionPolicy factorydefinitions.WorkstationExecutionPolicyService,
) (automations.Service, error)
```

Proposed:

```go
// automations/internal/service.go
func newService(logger *zap.Logger, clock Clock,
	lifecycle sourcelifecycle.SourceLifecycle,
	reconciler reconciliation.Service, scriptPollers scriptpollers.Service,
	cronService cron.Service, filesystemWatchers filesystemwatchers.Service,
	hostedPollers automations.HostedPollers) *Service
func New(logger *zap.Logger, clock Clock,
	lifecycle sourcelifecycle.SourceLifecycle,
	reconciler reconciliation.Service, scriptPollers scriptpollers.Service,
	cronService cron.Service, filesystemWatchers filesystemwatchers.Service,
	hostedPollers automations.HostedPollers) *Service

// automations/wire/wire.go
type Owner = automationinternal.Service
type Reconciliation = reconciliation.Service
type ScriptPollers = scriptpollers.Service
type Cron = cron.Service
type FilesystemWatchers = filesystemwatchers.Service
type Clock = automationinternal.Clock
func NewService(logger *zap.Logger, clock Clock, lifecycle SourceLifecycle,
	reconciler Reconciliation, scriptPollers ScriptPollers, cronService Cron,
	filesystemWatchers FilesystemWatchers,
	hostedPollers automations.HostedPollers) *Owner
func NewRoot(service *Owner) automations.Root
```

Compatibility and rollout: Docs specification only in this lane; T12 applies the private delta in its retained successor. Public Automations API, CLI, configuration, event/status fields, cursor schema/destination, Now-only caller acceptance and persistence fallback policy remain unchanged. No migration or feature flag.

Callers/consumers: `pkg/wire canonical Automations provider registrations`; `pkg/services/automations/internal/runtime_lifecycle.go; runtime_sidecars.go`; `pkg/services/automations/wire fixtures`.

Generated outputs: None in docs lane; T12 regenerates pkg/wire/wire_gen.go with make generate-wire.

Removal and migration owner: T12 removes per-runtime newService/NewWithCursorFileSystem/runtimeInstance.owner, parent callback constructor and duplicate runtime reconciler. Activation configures lifecycle domain facts before runtime-keyed reconciliation Start/Wait; deactivation Stop/Wait joins before ReleaseRuntimeSource. An old instance cannot release replacement state. Root projects unchanged Operations/Lifecycle/Runtime; no public Root fields change.

The retained internal `New` at `service.go:63`, `NewWithCursorFileSystem` at `:93` and `NewService` at `:168` route through the nested construction path. T12 migrates their fixtures to completed owner providers and removes the obsolete alternate wrappers at cutover; the proposed internal `New` above takes only completed collaborators. Registration/release are operations on the injected lifecycle owner, never a constructor callback.

### C10 — Retire unreachable cursor constructors at cutover

Authored source: `pkg/services/automations/internal/services/script_pollers/cursor.go; pkg/services/automations/internal/services/script_pollers/wire/wire.go; pkg/services/automations/internal/services/script_pollers/internal/service/durable_cursor.go`.

Source basis: retained #2683/62cff1c7cfb4d734d462aa1ef08d2d5e0e6c4ba7.

Source locators: `pkg/services/automations/internal/services/script_pollers/cursor.go:36` and `:106`; `pkg/services/automations/internal/services/script_pollers/wire/wire.go:36`; `pkg/services/automations/internal/services/script_pollers/internal/service/durable_cursor.go:53`.

Owner: AM16 docs: fi-plan-amendment-am16-t12-20261003; implementation/removal: T12; checker unchanged: T20.

Current:

```go
type CursorRecorder interface {
	GetCursor(context.Context, automations.GetCursorRequest) (automations.GetCursorResult, error)
	CommitCursor(context.Context, CommitCursorRequest) error
}

func NewMemoryCursorRecorder() CursorRecorder

func NewDurableCursorRecorder(
	baseDir string,
	files CursorPersistenceFileSystem,
) (scriptpollers.CursorRecorder, error)

func NewDurableCursorRecorder(
	baseDir string,
	files CursorPersistenceFileSystem,
) (scriptpollers.CursorRecorder, error)
```

Proposed:

```go
# Removed
```

Compatibility and rollout: Docs specification only in this lane; T12 applies the private delta in its retained successor. Public Automations API, CLI, configuration, event/status fields, cursor schema/destination, Now-only caller acceptance and persistence fallback policy remain unchanged. No migration or feature flag.

Callers/consumers: `T12 owner fixtures migrate to CursorScopes; cursor serializer behavior retained in its owner unit suite`.

Generated outputs: None in docs lane; T12 regenerates pkg/wire/wire_gen.go with make generate-wire.

Removal and migration owner: T12 deletes obsolete unscoped recorder/constructors and only newly unreachable legacy helpers in this same PR. Do not delete live durable serialization/load/persist operations. Seven local findings are diagnostic, not a fixed inventory or waiver of 3154; measure newly unreachable symbols after cutover. Tests must not keep dead production constructors alive.

### C11 — Delete only the stale script_pollers interface-count baseline entry

Authored source: `docs/internal/baselines/package-structure-baseline.json / entries selected by rule and filePath`.

Source basis: amended main b5e4523d3d04649cfee4e05e203115504edcfbcb; exact selected entries object.

Source locators: `docs/internal/baselines/package-structure-baseline.json:2090` on amended main (selected stale interface-count object).

Owner: AM16 docs: fi-plan-amendment-am16-t12-20261003; implementation/removal: T12; checker unchanged: T20.

Current:

```json
{
  "rule": "service-root-interface-count",
  "filePath": "pkg/services/automations/internal/services/script_pollers",
  "target": "pkg/services/automations/internal/services/script_pollers/cursor.go:CursorRecorder,pkg/services/automations/internal/services/script_pollers/service.go:Service"
}
```

Proposed:

```json
# Removed
```

Compatibility and rollout: Docs specification only in this lane; T12 applies the private delta in its retained successor. Public Automations API, CLI, configuration, event/status fields, cursor schema/destination, Now-only caller acceptance and persistence fallback policy remain unchanged. No migration or feature flag.

Callers/consumers: `cmd/pkgstructurecheck; canonical Backend Lint`.

Generated outputs: None in docs lane; T12 regenerates pkg/wire/wire_gen.go with make generate-wire.

Removal and migration owner: T12 deletes this exact entry in its code PR when the root has only Service. T20 owns unchanged checker semantics/measurement, not this deletion prerequisite. Other baseline rows and deadcode allowance 0 are preserved.

### C12 — Outer owner exports direct recovery and script providers

Authored source: `pkg/services/automations/wire/wire.go`.

Source basis: retained #2683/62cff1c7cfb4d734d462aa1ef08d2d5e0e6c4ba7.

Source locators: `pkg/services/automations/wire/wire.go:31` (alias), `:34` (filesystem alias), `:37` (provider); NewScriptPollers absent.

Owner: AM16 docs: fi-plan-amendment-am16-t12-20261003; implementation/removal: T12; checker unchanged: T20.

Current:

```go
type CursorScopes = scriptpollers.CursorScopes

type CursorPersistenceFileSystem = scriptpollerswire.CursorPersistenceFileSystem

func NewCursorScopes(files CursorPersistenceFileSystem) CursorScopes

// NewScriptPollers is not present.
```

Proposed:

```go
type CursorScopes = cursorscopes.CursorScopes
type CursorPersistenceFileSystem = cursorscopeswire.CursorPersistenceFileSystem
func NewCursorScopes(files CursorPersistenceFileSystem) CursorScopes
func NewScriptPollers(logger *zap.Logger, clock Clock,
	commandRunner platformprocess.CommandRunner,
	resolveTemplates workers.TemplateFieldResolver,
	executionPolicy factorydefinitions.WorkstationExecutionPolicyService,
	cursors CursorScopes) ScriptPollers
```

Compatibility and rollout: Docs specification only in this lane; T12 applies the private delta in its retained successor. Public Automations API, CLI, configuration, event/status fields, cursor schema/destination, Now-only caller acceptance and persistence fallback policy remain unchanged. No migration or feature flag.

Callers/consumers: `pkg/wire canonical provider set`; `pkg/services/automations/wire.NewSourceLifecycle/NewService`; `pkg/services/automations/internal/services/script_pollers/component fixtures`.

Generated outputs: None in docs lane; T12 regenerates pkg/wire/wire_gen.go with make generate-wire.

Removal and migration owner: T12 updates NewCursorScopes to call cursorscopeswire.NewService and NewScriptPollers to call scriptpollerswire.NewService; each constructs one implementation. Old CursorScopes alias points to the new owner; no private implementation escapes.

### C13 — Remove the root callback-construction caller

Authored source: `pkg/services/automations/internal/runtime_sidecars.go / newSchedulerReconciler`.

Source basis: retained #2683/62cff1c7cfb4d734d462aa1ef08d2d5e0e6c4ba7.

Source locators: `pkg/services/automations/internal/runtime_sidecars.go:83`.

Owner: AM16 docs: fi-plan-amendment-am16-t12-20261003; implementation/removal: T12; checker unchanged: T20.

Current:

```go
func (s *Service) newSchedulerReconciler() reconciliation.Service {
	return reconciliationwire.NewService(schedulerLifecycle{owner: s})
}
```

Proposed:

```go
# Removed
```

Compatibility and rollout: Docs specification only in this lane; T12 applies the private delta in its retained successor. Public Automations API, CLI, configuration, event/status fields, cursor schema/destination, Now-only caller acceptance and persistence fallback policy remain unchanged. No migration or feature flag.

Callers/consumers: `pkg/services/automations/internal/service.go / initialization; runtime activation uses injected reconciliation`.

Generated outputs: None in docs lane; T12 regenerates pkg/wire/wire_gen.go with make generate-wire.

Removal and migration owner: T12 deletes newSchedulerReconciler and schedulerLifecycle{owner:*Service} with Start/Stop/Wait forwarding in the same successor. No registration or lifecycle owner depends on the final root.

### C14 — Canonical root provider consumes completed owner

Authored source: `pkg/wire/session_runtime_providers.go / provideAutomationsRoot`.

Source basis: retained #2683/62cff1c7cfb4d734d462aa1ef08d2d5e0e6c4ba7.

Source locators: `pkg/wire/session_runtime_providers.go:591`.

Owner: AM16 docs: fi-plan-amendment-am16-t12-20261003; implementation/removal: T12; checker unchanged: T20.

Current:

```go
func provideAutomationsRoot(
	hostedSourceInputs automationswire.HostedSourceInputs,
	logger *zap.Logger,
	clock factoryruntime.Clock,
	commandRunner platformprocess.CommandRunner,
	workstationExecution factorydefinitions.WorkstationExecutionPolicyService,
) (automations.Root, error) {
	return automationswire.NewRoot(
		logger,
		clock,
		commandRunner,
		"",
		"",
		hostedSourceInputs,
		workerswire.ResolveTemplateFields,
		workstationExecution,
	)
}
```

Proposed:

```go
func provideAutomationsRoot(service *automationswire.Owner) automations.Root {
	return automationswire.NewRoot(service)
}
```

Compatibility and rollout: Docs specification only in this lane; T12 applies the private delta in its retained successor. Public Automations API, CLI, configuration, event/status fields, cursor schema/destination, Now-only caller acceptance and persistence fallback policy remain unchanged. No migration or feature flag.

Callers/consumers: `pkg/wire servicesSet and generated Wire; initializer/runtime consumers of unchanged automations.Root`.

Generated outputs: None in docs lane; T12 regenerates pkg/wire/wire_gen.go with make generate-wire.

Removal and migration owner: T12 removes hosted effect bag/clock/runner selection from this root projection. Canonical Wire registers the focused owner providers C07/C08/C12/C16 and supplies each once; current-head generation proves actual wiring.

### C15 — Runtime instance retains only scoped data and resource handles

Authored source: `pkg/services/automations/internal/runtime_lifecycle.go / runtimeInstance`.

Source basis: retained #2683/62cff1c7cfb4d734d462aa1ef08d2d5e0e6c4ba7.

Source locators: `pkg/services/automations/internal/runtime_lifecycle.go:19`.

Owner: AM16 docs: fi-plan-amendment-am16-t12-20261003; implementation/removal: T12; checker unchanged: T20.

Current:

```go
type runtimeInstance struct {
	runtimeID        string
	factorySessionID string
	snapshot         interfaces.RuntimeSnapshot
	activationInputs runtimeActivationInputIdentity
	owner            *Service
	runtimeConfig    *runtimeSnapshotConfig
	watcher          automations.FilesystemWatcher
	watcherRoot      string
	submit           automations.WorkRequestSubmitter
	startSchedulers  bool
	ctx              context.Context
	cancel           context.CancelFunc
	sidecars         sync.WaitGroup

	mu       sync.Mutex
	starting bool
	started  bool
	cursorMu sync.Mutex
	released bool
}
```

Proposed:

```go
type runtimeInstance struct {
	runtimeID        string
	factorySessionID string
	snapshot         interfaces.RuntimeSnapshot
	activationInputs runtimeActivationInputIdentity
	runtimeConfig    *runtimeSnapshotConfig
	watcher          automations.FilesystemWatcher
	watcherRoot      string
	submit           automations.WorkRequestSubmitter
	startSchedulers  bool
	ctx              context.Context
	cancel           context.CancelFunc
	sidecars         sync.WaitGroup

	mu       sync.Mutex
	starting bool
	started  bool
	cursorMu sync.Mutex
	released bool
}
```

Compatibility and rollout: Docs specification only in this lane; T12 applies the private delta in its retained successor. Public Automations API, CLI, configuration, event/status fields, cursor schema/destination, Now-only caller acceptance and persistence fallback policy remain unchanged. No migration or feature flag.

Callers/consumers: `pkg/services/automations/internal/runtime_lifecycle.go / buildRuntimeInstance,start,stop; source/cursor routing`.

Generated outputs: None in docs lane; T12 regenerates pkg/wire/wire_gen.go with make generate-wire.

Removal and migration owner: T12 removes runtimeInstance.owner and its newService call together. Build allocates only this scoped state/config/context/watcher resource; root calls injected lifecycle ConfigureRuntimeSource before StartSourceForRuntime/WaitSourceForRuntime. StopSourceForRuntime/WaitSourceForRuntime joins before lifecycle ReleaseRuntimeSource; preserve cursorMu/released old-instance guard. No reusable service construction remains in activation.

### C16 — Hosted leaf has a focused provider instead of effect bag

Authored source: `pkg/services/automations/wire/wire.go / HostedSourceInputs, NewHostedPollers`.

Source basis: retained #2683/62cff1c7cfb4d734d462aa1ef08d2d5e0e6c4ba7.

Source locators: `pkg/services/automations/wire/wire.go:45` (bag); focused NewHostedPollers export absent.

Owner: AM16 docs: fi-plan-amendment-am16-t12-20261003; implementation/removal: T12; checker unchanged: T20.

Current:

```go
type HostedSourceInputs struct {
	Clock            automations.HostedLinearClock
	HTTPClient       automations.HostedLinearHTTPDoer
	SecretResolver   automations.HostedLinearSecretResolver
	LinearEndpoint   string
	CheckpointStore  automations.HostedLinearCheckpointStore
	CursorFileSystem scriptpollerswire.CursorPersistenceFileSystem
}

// Focused NewHostedPollers export is not present.
```

Proposed:

```go
// Removed: HostedSourceInputs.
func NewHostedPollers(logger *zap.Logger, clock automations.HostedLinearClock,
	httpClient automations.HostedLinearHTTPDoer,
	secretResolver automations.HostedLinearSecretResolver,
	linearEndpoint string,
	checkpointStore automations.HostedLinearCheckpointStore) automations.HostedPollers
```

Compatibility and rollout: Docs specification only in this lane; T12 applies the private delta in its retained successor. Public Automations API, CLI, configuration, event/status fields, cursor schema/destination, Now-only caller acceptance and persistence fallback policy remain unchanged. No migration or feature flag.

Callers/consumers: `pkg/wire named Automations hosted provider functions; C08 lifecycle and C09 root`.

Generated outputs: None in docs lane; T12 regenerates pkg/wire/wire_gen.go with make generate-wire.

Removal and migration owner: T12 removes HostedSourceInputs/composeHostedPollers graph construction when migrating NewRoot callers. This focused provider delegates to already-owned hosted source construction with explicit effects; retained required-checkpoint/logger signatures and public hosted policy remain unchanged. Remove the unused composite bag builder in the same PR.

### Required proof and release

T12 retains F01 inert construction and all F10a–m/full T12 obligations in tasks.md. C10 deletes only newly unreachable constructors/helpers after moving live serializer/load/persist behavior and its owner tests. C11 removes only the selected stale script_pollers baseline entry in that same successor PR, without a T20 dependency. Hosted deadcode 3154/zero allowance is failed historical evidence and pkg-structure is unmeasured. Compare main and successor using the same tool/config before attribution; pre-existing main findings are not T12 cleanup scope. Seven local findings are diagnostic, not a fixed removal inventory or an explanation of 3154.

T12-G01/G02 measures boundary/cycle/construction/logging, interface counts, baseline freshness, deadcode, package file counts and generated Wire on the successor head. Independent review rebases current origin/main and immediately runs `make lint pkg-file-count`, plus `make generate-wire` and clean diff when Wire changed. This packet proves no runtime or Project FI-A1–A8 acceptance. Independent docs validation and retained implementation gates remain required.


## T13 — Sessions directory, response state and durable leaves

Authored source: `pkg/services/factory_sessions/internal/sessionservice/assembly.go`.

Current:

```go
func NewAssembly(
	newJavaScriptCheckpointStore factoryruntime.JavaScriptCheckpointStoreFactory,
	sessionResultProjection factoryruntime.SessionResultProjectionOperation,
	interpolation factorydefinitions.InvocationInterpolationService,
	invocationWorkTypes factorydefinitions.InvocationWorkTypeService,
	ttsObservability factorydefinitions.TTSObservabilityService,
	clock factoryruntime.Clock,
	eventIDs factorysessions.ResponseEventIDGenerator,
	sessionIDs factorysessions.SessionIDGenerator,
	resolveHome factorysessions.HomeDirectoryResolver,
	directoryInspection roles.DirectoryInspection,
	namedPaths factorydefinitions.NamedPathResolver,
	invocationInputFiles fileeffects.InvocationInputReader,
	initialWorkFiles fileeffects.InitialWorkReader,
	identityService identity.Service,
	responseStreamService responsestreamservice.Service,
	liveChangeCoordinator factorysessioncontracts.LiveChangeCoordinator,
	recordedSessionInventory recordings.RecordedSessionInventory,
) roles.RuntimeAssembly
```

Proposed:

```go
func NewAssembly(
	gateway roles.SessionGateway,
	registry sessionregistry.Service,
	state *sessionruntime.Service,
	streams StreamManager,
	clock factoryruntime.Clock,
	invoker roles.SessionInvoker,
	activation SessionScopeActivation,
	newJavaScriptCheckpointStore factoryruntime.JavaScriptCheckpointStoreFactory,
	sessionResultProjection factoryruntime.SessionResultProjectionOperation,
	interpolation factorydefinitions.InvocationInterpolationService,
	invocationWorkTypes factorydefinitions.InvocationWorkTypeService,
	ttsObservability factorydefinitions.TTSObservabilityService,
	eventIDs factorysessions.ResponseEventIDGenerator,
	sessionIDs factorysessions.SessionIDGenerator,
	resolveHome factorysessions.HomeDirectoryResolver,
	directoryInspection roles.DirectoryInspection,
	namedPaths factorydefinitions.NamedPathResolver,
	invocationInputFiles fileeffects.InvocationInputReader,
	initialWorkFiles fileeffects.InitialWorkReader,
	identityService identity.Service,
	responseStreamService responsestreamservice.Service,
	liveChangeCoordinator factorysessioncontracts.LiveChangeCoordinator,
	recordedSessionInventory recordings.RecordedSessionInventory,
) roles.RuntimeAssembly
```

Authored source: the same file's stream capability.

Current:

```go
type streamManager interface {
	InferenceProgressPublisherFactory(*zap.Logger) func(string) factorysessions.ProgressPublisher
	DispatchCompletionObserverFactory() func(string) func(string)
}
```

Proposed:

```go
type StreamManager interface {
	InferenceProgressPublisherFactory(*zap.Logger) func(string) factorysessions.ProgressPublisher
	DispatchCompletionObserverFactory() func(string) func(string)
}
```

Expose `SessionGateway`, `SessionRegistry`, `SessionState`, `StreamManager`, `Identity` and `ResponseStreams` aliases through `factory_sessions/wire`. Registry/state/response constructors receive the exact same selected collaborators; Assembly stops invoking them. Preserve registry entries, admission locks, response sequence/cursors and startup singleflight.

T13 removes `BindProcessDurable` gateway replacement and makes the minimal `NewRootFromAssembly` bridge caller adjustment to pass completed durable/gateway roles. T17 owns final removal of `NewRootFromAssembly`, legacy Complete/opening products and factories. Construct the durable owner against directly injected session-state/resolver roles, then supply both completed assembly and durable owner to Root. Persistence-routing and resume-scope queries belong to that resolver, which must not depend on the final Sessions Root. Keep durable error/recovery behavior and raw event histories.


AM07 supersedes the earlier dropped-clock and binding-owner notes. Keep Assembly clock; construct independent authority first. The graph is authority -> scope control/activation -> durable -> invocation -> gateway -> Assembly. Host delegates state/control to authority and durable operations to completed durable; it never depends on invocation/gateway/Root. Failed initialization is unpublished and rolled back. Complete becomes scope registration using completed gateway/invoker in T13; T17 removes that bridge.

### Independent session authority construction

Authored source: `pkg/services/factory_sessions/internal/runtime/service.go:210-229`. Existing direct constructor is retained, moved out of Assembly's nested construction to one focused owner Wire provider.

Current:

```go
func NewWithResponseService(
 registry sessionregistry.Service,
 responses *responsestream.Registry,
 closeSession func(*livesession.LiveSession),
 clock factoryruntime.Clock,
 eventIDs factorysessions.ResponseEventIDGenerator,
 sessionIDs factorysessions.SessionIDGenerator,
 responseEvents responsestreamservice.Service,
) *Service
```

Proposed:

```go
func NewWithResponseService(
 registry sessionregistry.Service,
 responses *responsestream.Registry,
 closeSession func(*livesession.LiveSession),
 clock factoryruntime.Clock,
 eventIDs factorysessions.ResponseEventIDGenerator,
 sessionIDs factorysessions.SessionIDGenerator,
 responseEvents responsestreamservice.Service,
) *Service
```

The authority has no Root/gateway/invoker dependency. closeSession is completed lifecycle cleanup over keyed state/handles, never a closure capturing final Root or re-entering invocation. Existing Register/Resolve, activation lock and canonical identities remain. Scope activation/control and invocation query adapters consume this authority and directly injected Runtime lifecycle/Work/projection roles; peer lookup through final Root is prohibited. Construct durable against its independent resolver before invocation/gateway. Export only focused legal owner Wire aliases, not private stores or a public registry.

### T13 independent scope activation

Authored source: `pkg/services/factory_sessions/internal/roles/contracts.go RuntimeAssembly.Complete`. Current excerpts describe the existing edges; proposed types belong inside the Sessions private owner.

Current:

```go
// Current lifecycle factory edge inside Assembly.Complete:
gateway := NewWithLiveChangeCoordinator(
 SessionServiceHost(runtime), a.state,
 sessionruntime.NewResponseStreamObserver(runtimebinding.ResponseStreamRuntimeFromSessionHandle),
 a.state.ResponseStreams(), runtime.ReconnectCursorValidator(),
 a.sessionResultProjection, a.responseStreams, a.liveChangeCoordinator,
)
gateway = runtime.AttachSessionGateway(gateway)
gateway.bindRecordedSessionHistory(a.ListSessions)
invoker, err := NewInvocationOwner(runtime, a.interpolation, a.invocationWorkTypes, a.ttsObservability, a.invocationInputFiles)
bound.Invoker = invoker
a.registry.Upsert(session, true)
gateway.bindRootCapabilities(invoker, runtime.ActivateNamedFactory, runtime.DefinitionActivationGateway())
```

Proposed:

```go
// New private contracts in factory_sessions/internal/sessionservice.
// Authority is independently constructed sessionruntime.Service; it never
// depends on final Root, gateway or invoker. Existing Register/Resolve and
// response/activation locks retain canonical identity and sequencing.
type SessionScope struct {
 SessionID string
 RuntimeID string
 GenerationID string
 ModelsScope models.RuntimeScopeRef
 Target factorysessions.TargetRef
 Record factoryruntime.RuntimeRecord
 Run factoryruntime.RuntimeRun
}
type SessionScopeActivation interface {
 Activate(context.Context, SessionScope) error
 Retire(context.Context, string, string) error // session ID, generation ID
}
type SessionScopeControl interface {
 CancelLiveFactorySession(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error)
 StopLiveSession(string) error
}
func NewScopeActivation(authority *sessionruntime.Service) SessionScopeActivation
// Interim T13 bridge only: RuntimeRecord still exposes service/logger
// getters today (runtime_session_contract.go:19-36). T15 owns their
// replacement by an opaque RuntimeBinding plus keyed lifecycle state;
// T17 removes legacy opening consumption. This is not final S01 proof.
// T13 bridge: Complete registers scope and uses preconstructed gateway and
// invoker; T17 removes Complete and legacy NewSessionRuntime construction.
```

Activation mutates keyed state under existing locks; failed initialization is unpublished and rolled back. Retire uses session+generation identity to avoid retiring a peer/current replacement. Scope introduces no new process clock/logger default. Existing RuntimeRecord service/logger getters are explicitly temporary compatibility: T15 owns opaque RuntimeBinding/keyed-lifecycle replacement; T17 owns legacy opening removal. The T13 bridge is not final S01 proof.

### T13 gateway receives complete durable owner

Authored source: `pkg/services/factory_sessions/internal/sessionservice/service.go:84-109`. Current excerpts describe the existing edges; proposed types belong inside the Sessions private owner.

Current:

```go
func NewWithLiveChangeCoordinator(
 host Host,
 sessions stream.SessionResolver,
 observer stream.Observer,
 responseStreams *responsestream.Registry,
 reconnects factorysessions.ReconnectCursorValidator,
 results factoryruntime.SessionResultProjectionOperation,
 responseEvents responsestreamservice.Service,
 liveChange factorysessioncontracts.LiveChangeCoordinator,
) *Service
func (a *Assembly) BindProcessDurable(execution durableexecution.Service) error
```

Proposed:

```go
func NewGateway(
 host Host,
 streams *stream.Manager,
 reconnects factorysessions.ReconnectCursorValidator,
 results factoryruntime.SessionResultProjectionOperation,
 responseEvents responsestreamservice.Service,
 liveChange factorysessioncontracts.LiveChangeCoordinator,
 durable durableexecution.Service,
 recordedHistory func(context.Context, factorysessions.ListSessionsRequest) (factorysessions.ListSessionsResult, error),
 invoker roles.SessionInvoker,
 activate func(context.Context, string) error,
 activationGateway factorydefinitions.DefinitionActivationGateway,
) *Service
// Removed in T13: BindProcessDurable, AttachSessionGateway,
// bindRecordedSessionHistory and bindRootCapabilities.
// T17 final root already receives completed assembly/durable/start operation:
func NewRoot(
 assembly roles.RuntimeAssembly,
 durable durableexecution.Service,
 start func(context.Context, factorysessions.SessionStartRequest) (factorysessions.SessionStartResult, error),
 liveChangeCoordinator factorysessioncontracts.LiveChangeCoordinator,
) (*Root, error)
```

Construct stream.Manager separately with existing explicit resolver/observer/registry. Host delegates live state/control to independent authority, durable methods to already constructed durable owner; Host never depends on invoker/gateway/Root. Durable constructor retains persistence and resume errors. T13 bridge passes completed durable owner, never replacing a gateway slot. No public durable semantics change.

### T13 invocation authority breaks cancellation cycle

Authored source: `pkg/services/factory_sessions/internal/sessionservice/runtime_invocation.go:21-74`. Current excerpts describe the existing edges; proposed types belong inside the Sessions private owner.

Current:

```go
func NewInvocationOwner(
 fs *SessionRuntime,
 interpolation factorydefinitions.InvocationInterpolationService,
 invocationWorkTypes factorydefinitions.InvocationWorkTypeService,
 ttsObservability factorydefinitions.TTSObservabilityService,
 inputFiles fileeffects.InvocationInputReader,
) (invocationservice.Service, error)
```

Proposed:

```go
type InvocationAuthority interface {
 FactoryConfig(string) (*factorydefinitions.FactoryConfig, error)
 SubmitWork(context.Context, string, work.SubmitRequest) (work.WorkRequestSubmitResult, error)
 Observe(context.Context, string, sessioninvocation.SessionInvocationWaitInput) (sessioninvocation.SessionInvocationObservation, error)
 WaitSession(context.Context, string) (sessioninvocation.SessionInvocationWaiter, sessioninvocation.ReleaseSessionInvocationWaiter)
}
func NewInvocationOwner(
 authority InvocationAuthority,
 controls SessionScopeControl,
 telemetry sessioninvocation.SessionInvocationTelemetry,
 specialCase sessioninvocation.SessionInvocationSpecialCase,
 interpolation factorydefinitions.InvocationInterpolationService,
 invocationWorkTypes factorydefinitions.InvocationWorkTypeService,
 inputFiles fileeffects.InvocationInputReader,
 workPolicy work.Service,
) (invocationservice.Service, error)
```

Authority resolves named session and generation using existing state, Work and projection owners; it never captures final Root/gateway. ScopeControl directly consumes Runtime lifecycle and session state; timeout cancellation does not re-enter invocation. Telemetry/special-case/Work policy are constructed once, required callback bag is removed with this direct signature. Existing wait/event/result errors and durable history survive.

## T14 — Runtime control and one handle authority

Authored source: `pkg/services/factory_runtime/internal/root.go`.

Current:

```go
func NewRoot(
	newID factoryruntime.IDGenerator,
	workflows factoryruntime.JavaScriptWorkflowDefinitions,
	workflowRuntime factoryruntime.JavaScriptWorkflowRuntime,
	clock factoryruntime.Clock,
	workersPublisher dispatchplanning.WorkersPublisher,
	workersCanceler dispatchplanning.WorkersCanceler,
) (*Root, error)
```

Proposed:

```go
func NewRoot(
	orchestration orchestration.Service,
	instanceHost instancehost.Service,
	dispatchPlan dispatchplanning.Service,
) (*Root, error)
```

Authored destination: `pkg/services/factory_runtime/wire` construction aliases.

Current:

```go
// Not present
```

Proposed:

```go
type Orchestration = orchestration.Service
type InstanceHost = instancehost.Service
type DispatchPlanning = dispatchplanning.Service
```

Focused `NewOrchestration`, `NewInstanceHost` and `NewDispatchPlanning` providers forward existing leaf arguments and return these contracts. Root retains its active/failed/activating/deactivating maps and locks. Use the same handle authority for Root control and runtime lifecycle; do not preserve a second `instancehostwire.New` in Assembly. Characterize handle registration before consolidation, because each current Host owns its own handle map.

Authored source: `factory_runtime/internal/services/instance_host/wire/wire.go`.

Current:

```go
func New(dependencies instancehost.Dependencies) (instancehost.Service, error)
```

Proposed:

```go
// Removed: instancehost.Dependencies.
func New(
 clock factoryruntime.Clock,
 scheduler platformclock.TimerSource,
 lifecycle *factoryhost.LifecycleService,
) (instancehost.Service, error)
```

Authored source: `pkg/services/factory_runtime/internal/host/service.go:16` (current import name `factory`).

Current:

```go
func NewLifecycleService(clock factory.Clock) (*LifecycleService, error)
```

Proposed:

```go
func NewLifecycleService(
 clock factoryruntime.Clock,
 scheduler platformclock.TimerSource,
) (*LifecycleService, error)
```

Authored source: `pkg/services/factory_runtime/internal/host/lifecycle.go:164`.

Current:

```go
func WaitForStart(ctx context.Context, handle *Handle) error
```

Proposed:

```go
func WaitForStart(
 ctx context.Context,
 handle *Handle,
 scheduler platformclock.TimerSource,
) error
```

AM08: delete the one-field instancehost.Dependencies wrapper. LifecycleService stores both Clock and TimerSource; forward scheduler to WaitForStart and all readiness callers (including legacy Assembly host injection). The native pairs above retain the current signatures and separate replay-sensitive timestamps from OS scheduling. Preserve the one-second ceiling and 10ms poll using cancellable NewTimer loops; never fall back to real time or use replay ticks for readiness. Host handles remain keyed by runtime identity. T15/T31 build redesign is not part of this minimal propagation.

## T15 — Runtime activation, replacement and replay

Authored source: `pkg/services/factory_runtime/runtime_activation_contract.go`.

Current:

```go
type Root interface {
	Service
	Activate(context.Context, RuntimeActivationRequest, RuntimeActivationOperation) (RuntimeActivationResult, error)
	Deactivate(context.Context, RuntimeDeactivationRequest) (RuntimeDeactivationResult, error)
}
```

Proposed:

```go
type Root interface {
	Service
	Activate(context.Context, RuntimeActivationRequest) (RuntimeActivationResult, error)
	Deactivate(context.Context, RuntimeDeactivationRequest) (RuntimeDeactivationResult, error)
}
```

The existing operation signature remains `func(context.Context, RuntimeActivationRequest) (*RuntimeActivation, error)` but becomes a fixed Root constructor dependency. T15 extends T14's destination constructor as follows.

Current, T14 destination:

```go
func NewRoot(
	orchestration orchestration.Service,
	instanceHost instancehost.Service,
	dispatchPlan dispatchplanning.Service,
) (*Root, error)
```

Proposed:

```go
func NewRoot(
	orchestration orchestration.Service,
	instanceHost instancehost.Service,
	dispatchPlan dispatchplanning.Service,
	activate factoryruntime.RuntimeActivationOperation,
) (*Root, error)
```

Update Sessions' structurally equivalent `FactoryRuntimeRoot` and Activate callers together. Remove Assembly/RuntimeBuild/RuntimeFactory as composition owners. Activation uses injected definition compilation, engine behavior, dispatch, artifact and sidecar owners; it initializes scoped state/resources and returns the existing activation/cleanup shape. It may not call a removed service factory. Retain per-generation net/marking/buffers, attempts, recording, sinks, replay facts and cleanup. Runtime Bundle loses filesystem/ID-generator service fields; lifecycle handles retain scoped resources. Preserve atomic publication, retryable cleanup, replacement precommit rollback and detached replay tick normalization while keeping canonical events unchanged.

## T16 — Worker Sessions keyed attempt supervision

This is the AM09 first-step documentation contract, not a production cutover or runtime PASS. Source basis: `5c800e49c959b01ae436efaa392d7b4f2cb869cf` (HEAD and origin/main at authoring). Current blocks are native declaration excerpts; imports and function bodies are omitted. The separately reviewed contract PR must merge before characterization and ANY structural T16 PR. T16 owns characterization and complete factory removal; T15 consumes the completed cutover; WSV construction additions wait for FI. No public API, configuration, persistence, event, CLI or identity policy changes are authorized here.

### Native request, operation and construction pairs

Authored destination: `pkg/services/worker_sessions/contracts.go / RuntimeAttemptRequest; destination docs/internal/development/plans/flat-injection/contracts.md §T16`.

Current:

```go
type RuntimeAttemptRequest struct {
	ID        string
	AttemptID string
	Execution workers.WorkstationDispatchRequest
}

type RuntimeAttempt func(context.Context, workers.WorkstationDispatchResult, error) error
```

Proposed:

```go
// RuntimeAttemptKey is a process-local lookup key, never a wire identifier.
type RuntimeAttemptKey struct {
	RuntimeID  string
	DispatchID string
}

// ID is the existing stable Worker Session ID, not a derived key string.
// AttemptID is physical; empty preserves the dispatch-ID default.
// Execution retains the canonical Workers request, including FactorySessionID,
// RuntimeID, RecordingID, GenerationID and Dispatch.DispatchID.
// Key must agree with Execution.Execution.RuntimeID and its Dispatch.DispatchID.
type RuntimeAttemptRequest struct {
	Key       RuntimeAttemptKey
	ID        string
	AttemptID string
	Execution workers.WorkstationDispatchRequest
}

// Unchanged: Complete is idempotent and retains the existing terminal contract.
type RuntimeAttempt func(context.Context, workers.WorkstationDispatchResult, error) error
```

Documentation proposal only. Internal callers later supply an explicit key matching canonical execution correlation. Preserve stable Worker Session and physical attempt IDs, replay associations, empty AttemptID semantics, duplicate Worker Session rejection and canonical Topics. No serialized key, new identity policy or persisted migration.

Authored destination: `pkg/services/factory_runtime/composition_contracts.go / WorkerSessionsFactory; pkg/services/worker_sessions/internal/service/invoke_session.go / structural BeginRuntimeAttempt capability; destination docs/internal/development/plans/flat-injection/contracts.md §T16`.

Current:

```go
type WorkerSessionsFactory func(workers.Service, platformclock.Source) (workersessions.Service, error)

var _ interface {
	BeginRuntimeAttempt(context.Context, workersessions.RuntimeAttemptRequest) (workersessions.RuntimeAttempt, error)
} = (*registry)(nil)
```

Proposed:

```go
// Removed in the later T16 structural PR: WorkerSessionsFactory.
// Request data and behavioral inputs remain separate explicit arguments.
// execution is the already-selected runtime handle carrying provider and
// command-runner overrides, replay-runner precedence and runtime resolution.
// clock supplies scoped fact timestamps; scheduler supplies safety deadlines.
// cancel is the exact Runtime-owned attempt control handle, installed atomically.
type WorkerAttemptOpener interface {
	BeginRuntimeAttempt(
		context.Context,
		workersessions.RuntimeAttemptRequest,
		workers.Service,
		platformclock.Source,
		platformclock.TimerSource,
		func(context.Context) (workers.WorkstationDispatchCancelOutcome, error),
	) (workersessions.RuntimeAttempt, error)

	// Compatibility runtime invocation enters the same keyed supervision.
	// It uses the supplied runtime handle, never the process default executor.
	InvokeRuntimeSession(
		context.Context,
		workersessions.RuntimeAttemptRequest,
		workersessions.RetryPolicy,
		workers.Service,
		platformclock.Source,
		platformclock.TimerSource,
	) (workersessions.InvokeSessionResult, error)

	// Correlation.RuntimeID and DispatchID must match the explicit key.
	// Commit exact provider association and Worker observations before forwarding.
	PublishRuntimeProgress(
		context.Context,
		workersessions.RuntimeAttemptKey,
		workers.ProgressFragment,
		workers.ProgressPublisher,
	) error

	// Join only this runtime's live attempts and recording handles.
	// Preserve terminal metadata, latest dispatch identity and Events retention.
	CloseRuntimeAttempts(context.Context, string) error
}
```

The whole attempt-opening request is the operation input above: domain data plus explicit runtime execution, fact clock, deadline scheduler and cancellation resource arguments. No constructor bag, service getter, secondary graph or shared override setter. Keep direct Service.Start/InvokeSession and control/read contracts unchanged; runtime-only compatibility paths use InvokeRuntimeSession. Existing provider progress fallback/suppression and terminal classifications remain. No production deletion in this documentation PR.

Authored destination: `pkg/services/worker_sessions/wire/wire.go / NewService; destination docs/internal/development/plans/flat-injection/contracts.md §T16`.

Current:

```go
func NewService(
	execution workers.Service,
	eventsAppender EventsAppender,
	logger logging.Logger,
	clock platformclock.Source,
	providerSessions providersessions.Service,
	recording recordings.WorkerSessionRecordingService,
) (workersessions.Service, error)
```

Proposed:

```go
func NewService(
	execution workers.Service,
	eventsAppender EventsAppender,
	logger logging.Logger,
	clock platformclock.Source,
	scheduler platformclock.TimerSource,
	providerSessions providersessions.Service,
	recording recordings.WorkerSessionRecordingService,
) (workersessions.Service, error)
```

Construct one inert supervisor in canonical Wire. The executor and clock here are process defaults for direct Worker Sessions. Runtime operations receive their selected handles explicitly and never mutate these defaults. Scheduler is required and selected at composition; no newSupervisionDeadlineTimer host fallback. Current TimerSource has NewTimer; T01 owns its After addition, not this lane. Preserve explicit legacy clock precedence and scoped replay facts versus OS deadlines.

The complete operation input is the request plus its separate behavior arguments, not a dependency bag. `WorkerAttemptOpener` is the Factory Runtime consuming capability in `pkg/services/factory_runtime/composition_contracts.go`; the shared Worker Sessions implementation satisfies it structurally. Request/key records belong to the Worker Sessions root. Canonical Wire constructs one inert supervisor with the fixed dependencies shown above. No attempt operation constructs a service, returns a service getter, substitutes constructor dependencies or mutates process defaults. The selected runtime handle is an existing execution capability; its eventual T09 replacement must preserve the same selected behavior without introducing another constructor path.

### Identity and source-to-target trace

`RuntimeAttemptKey` is process-local and comparable; neither it nor a concatenated representation is serialized. RuntimeID and logical DispatchID are required and must agree with `Execution.Execution.RuntimeID` and `Execution.Execution.Dispatch.DispatchID` before mutation. Keep the canonical request detached. Runtime callers first preserve the existing correlation fallback (`IW:337-367`) by normalizing the request RuntimeID to that already-resolved canonical value, then derive and validate the key; a blank legacy field is not a new public rejection policy. The stable Worker Session `ID` and physical `AttemptID` remain distinct: empty AttemptID retains the logical-dispatch default, retries retain their current physical allocation, and replay uses the recorded Worker Session association. The execution request's `FactorySessionID`, `RuntimeID`, `RecordingID` and `GenerationID` are all strings (`workers/execution_requests.go:181-198`); dispatch/request/work lineage remains the existing `work.WorkDispatch`. `workers.ExecutionCorrelation` retains string FactorySessionID, RuntimeID, GenerationID, DispatchID, AttemptID, RequestID and TraceID (`execution_requests.go:452-460`). RecordingID stays in the execution request, not a new correlation field.

Paths below are repo-relative. Runtime implementation abbreviations: RB = `pkg/services/factory_runtime/internal/runtime_build.go`; DC = `pkg/services/factory_runtime/internal/services/orchestration/runtime/dispatch_worker_sessions_cutover.go`; IW = the adjacent `invoke_worker.go`; WS = `pkg/services/worker_sessions/internal/service/`. These anchors describe current source, not executed evidence.

| Current input / source witness | Explicit preservation route | Later proof owner / gate |
| --- | --- | --- |
| Runtime-specific execution at `RB:34-45,477-529`; provider, command runner, replay runner, model/permission overrides, resolver, mock policy | Pass that selected `workers.Service` into Begin/InvokeRuntimeSession; retain replay runner precedence (`RB:52-97`) and invocation permission copy. Never fall back to the shared executor or change shared override state. Detached Runtime execution remains Runtime-owned. | T09/T16, U03-T16 selection, F05a-d |
| Factory Session/runtime/recording IDs at `RB:503-517`; normalized request at `IW:337-367` | Key validates canonical runtime/dispatch; Execution carries all session, recording, generation, request and Work facts unchanged. Selected handle retains runtime resolution. | T16, U03-T16, F06 |
| Selected clock passed into factory; late observer binding at `RB:621-631` | Each opening retains its explicit fact clock and scheduler; progress closure captures the runtime key and its downstream publisher. Replace Bind rather than rebinding a shared observer. | T01/T16, U03-T16 selection, U01/F15/F16 |
| Current Begin and later cancellation binding at `DC:80-164`; `WS/invoke_session.go:89-130,240-310` | Begin installs the exact Runtime cancel closure atomically with keyed ownership before return. Closure still calls that runtime's attempts.cancel; no bare-dispatch process lookup. Opening remains a before-worker-effect barrier. | T16, U03-T16 lifecycle, F05d |
| Compatibility `startThroughWorkerSessions` at `DC:25-77`; the compatibility branch of `factoryImpl.InvokeWorker` at `IW:599-660` | Both runtime paths enter InvokeRuntimeSession with key, selected handle, fact clock and scheduler. Direct Service.Start/InvokeSession retains process defaults and existing customer contracts. | T16, U03-T16, F05 |
| `dispatchOwners`, runtime control handles and retained latest IDs at `WS/service.go:68-93`, `WS/invoke_session.go:290-310,435-450` | Runtime reverse owner lookup uses the pair key. Worker-ID registries remain Worker identity-owned; live physical-attempt guards prevent stale completion/control/progress from selecting a replacement. Preserve retained latest logical DispatchID for terminal NOOP responses. | T16, U03-T16 lifecycle, F06 |
| Default Worker ID and replay association at `DC:35-44,177-197` | Preserve existing ID/retry/replay allocation here. Distinct supplied IDs make the synthetic trace feasible; production UUID characterization is separate from collision proof below. | T16-CHAR-PRODUCTION-ID; U03-T16; T16-REAL-PATH-COLLISION; contingent T16-IDENTITY-DELTA |
| Topic at `pkg/services/worker_sessions/topic.go:11-12`; recording request at `WS/service.go:218-229`; recorder validation at `pkg/services/recordings/internal/services/worker_capture/ports.go:30-51` | Keep `Topic(ID)` = `worker-session/<id>/events`. Start capture with the exact RecordingID, FactorySessionID, WorkerSessionID and Topic; own its returned handle per attempt. Never add runtime scope to topic bytes. | T16/Recordings, U03-T16, F07/F08 |
| Now-only fallback and deadlines at `WS/service.go:928-966`; `pkg/platform/clock/clock.go:13-31` | Explicit TimerSource supplies NewTimer; use scheduler.Now for safety elapsed time and the fact clock for records. Stop the selected timer on attempt completion. Preserve existing durations; logical replay facts cannot implicitly select OS scheduling. T01 owns After addition. | T01/T16, U03-T16 selection, F05c/F16 |
| Provider association before forwarding at `pkg/services/worker_sessions/publish.go:84-121,181-199`; windows at `WS/publish_record.go:587-603` and provider binding at `:650-710` | PublishRuntimeProgress validates key against Correlation.RuntimeID/DispatchID and fragment.DispatchID, plus the active physical attempt. Resolve the exact Worker owner, bind its Providers-owned reference, commit source-native observations, then forward to supplied next publisher under existing suppression/fallback. | T16, U03-T16 lifecycle, F06 |

Public unkeyed operations remain unchanged. Runtime progress must not call their bare-dispatch lookups against a shared registry: the keyed operation resolves the immutable owner first, then applies the existing association/publication rules to that owner. Direct supervision keeps its existing dispatch route separate. The runtime-scoped progress closure uses its immutable admitted correlation for compatibility fragments with missing correlation; explicitly conflicting fields are rejected and never overwritten. It must carry the physical attempt at emission, not fill a stale fragment from whichever attempt is currently live. Existing canonical-draft payload and public dispatch bytes remain unchanged; U03-T16 selection/lifecycle must characterize both fully correlated and compatibility fragments. Provider identity, opaque continuation, first-reference acceptance, repeated-reference idempotency and conflicting-reference rejection remain Providers/Worker Sessions contracts. Worker Sessions does not fabricate Provider Session identity or take provider execution policy.

### Opening, control, retention and failure rules

Begin validates the request, matching key and required behavior inputs before any mutation or worker effect. Duplicate Worker identity reservation still returns `ErrSessionAlreadyExists` and leaves the original unchanged; existing reserved-identity invocation/startability rules remain. Claim key and physical attempt without allowing a peer owner to be overwritten. Commit and verify opening Events publication and establish the exact recording association before handing back the immutable completion function. An opening/capture error cannot imply successful admission or start a command; unwind only the owned reservation/route/capture under current failure classification, retaining joined causes where cleanup also fails. Runtime remains admission/execution authority for detached attempts; InvokeRuntimeSession preserves the same supervision/retry/classification state machine and synchronous wait contract as the existing compatibility path.

Controls still address stable Worker Session ID. Resolve its exact retained key and live physical attempt, then invoke the installed cancellation handle. Failed cancellation returns the current failed disposition without fabricating application; unsupported pause/resume stays unsupported. Terminal state is absorbing; completion is idempotent, exactly one terminal classification wins, and Terminate joins the associated callback. Old handles cannot complete or cancel a newer physical attempt. Keep current public logical DispatchID responses and terminal NOOP semantics. Opening/control/completion races must be characterized before changing implementation.

PublishRuntimeProgress commits the exact provider association before forwarding reference-bearing output. Preserve provider agreement checks, bookkeeping-fragment suppression and the existing unassociated-progress fallback selected by the runtime publisher. The key is never inferred from a bare dispatch string. Late or stale progress cannot reopen a terminal publication window or attach to a replacement attempt: retain `ErrPublicationNotOpen`, `ErrOutOfOrderPublication`, exact Events retry/idempotency behavior and safe error logging. Failure to commit an observation does not invent a successful record or a new provider execution.

CloseRuntimeAttempts(ctx, runtimeID) selects only that runtime's live physical attempts, prevents new scoped admission during close, cancels/joins through their exact handles and joins their recording capture handles. Repeated close is idempotent; unsuccessful cleanup retains the owned handle for retry and returns the current cause. Stop owned timers and release live execution/cancel/publisher references after users join, without deleting terminal Worker metadata, immutable observations, latest dispatch identity, provider links or Events retention/cursors. A caller wait cancellation does not substitute for the explicit execution cancel boundary. Runtime owns retained/reused session-generation worktree lifetime; T09 releases attempt resources and nonretained checkouts under existing RetainWorktree policy. Close cannot unconditionally delete retained worktrees or claim P01 retention proof.

Events owns process-local source-native order, retained reads, cursors, subscriptions, gaps and backpressure. Recordings owns durable Factory history and capture/replay. Closing an attempt or a runtime does not erase either history or make Worker Sessions the Factory ledger. Factory dispatch/Worker association remains recorded into the owning runtime ledger; replay resolves the recorded association before opening, rather than allocating a replacement identity. Final completion releases each exact capture handle once and retains its truthful health/classification on failure (`WS/classify.go:49-98`, `WS/publish_record.go:913-937`). The injected logger records opening/control/terminal/close intent and outcome with safe runtime, Factory Session, Worker and physical-attempt identifiers; publication failures retain current safe causes and suppression behavior, without raw provider payloads. High-frequency progress success may use the current sampled/aggregate policy; no new diagnostics or public outcome policy is introduced.

### Equal-dispatch design rehearsal (static only)

Let A carry `(runtime-a, dispatch-1)`, supplied Worker ID `worker-a`, physical attempt `attempt-a`, Factory Session `session-a`, recording `recording-a`, execution handle EA, fact clock CA, scheduler SA and cancel XA. B carries corresponding b values and the same logical dispatch-1. Supplied distinct Worker IDs are valid only for this synthetic witness; they are not a production allocation fix.

| Operation | A route and observable contract | B isolation / remaining proof |
| --- | --- | --- |
| Open/start | Begin(key A, request A, EA, CA, SA, XA) commits worker-a opening/capture before handoff; compatibility InvokeRuntimeSession uses EA. | B uses key B/EB/CB/SB/XB and worker-b; U03-T16/F05a later executes this. |
| Supervise/replay | Immutable A handle keeps physical attempt-a and selected overrides; replay runner wins and recorded Worker association is honored. | EB and recorded B identity remain selected independently; U03-T16 selection/F16. |
| Progress/provider link | Key A plus matching runtime/dispatch/physical correlation selects worker-a; exact provider association precedes next-A output. | Key B selects worker-b and next-B; no shared Bind; U03-T16/F06. |
| Cancel | Cancel(worker-a) resolves A and XA; failed control is truthful; terminal NOOP retains dispatch-1. | XB is untouched, B can complete; F05d and lifecycle races. |
| Observe/retain | Reads by worker-a or exact Factory Session/Work scope retain A observations and provider links after terminal. | worker-b and scoped B reads stay attributable; Events cursor/retention rules unchanged; F06. |
| Record/complete | Topic(worker-a) and recording-a/session-a capture exact source records; A completion normalizes once using current outcome mapping. | Topic(worker-b), recording-b/session-b and B completion are separate; F07/F08 later. |
| Close | CloseRuntimeAttempts(runtime-a) joins only A resources and capture; retains terminal identity/history. | B remains usable and later closes independently; U03-T16 lifecycle, I01/P01. |

D01 happy follows the table. D02 failed control leaves A truthful and B live. D03 duplicate supplied Worker ID rejects before any peer topic/owner mutation. D04 replay keeps the recorded ID, replay runner precedence and separately selected safety scheduler. D05 opening/capture failure produces no worker effect or false handoff and closes only its scope. D06 terminal A rejects late publication under current window rules while retaining readable identity and leaving B live. D07 follows actual allocation: retained default Petri dispatch/Worker IDs and topics differ, so the expected-equal assertion is BLOCKED-premise. Distinct observations neither execute D01-D06 nor prove shared-supervisor safety. Each case needs independent contract review, then its separately owned executable evidence.

### Binding production identity boundary

Verbatim operator response retained from the admitted packet (`docs/temp/operator-mailbox/responses/fi-t16-keyed-attempt-contract-am09-20261003.md`):

```text
# Response — fi-t16-keyed-attempt-contract-am09-20261003 (2026-10-03T10:47Z) — BINDING
**Option A**, with one hard condition. For the synthetic collision witness, distinct Worker Session IDs, already supplied, are valid.
Keep duplicate Worker Session identity rejection and the canonical topics (worker-session/<id>/events) unchanged.
CONDITION: the later T16 characterization MUST prove what production actually allocates, and it is expected to FAIL today.
runtimeWorkerSessionID defaults to the dispatch ID (dispatch_worker_sessions_cutover.go:185-197), and dispatch IDs repeat
across Factory Sessions by contract (decided for the MCP correction at 10:28Z: dispatch-queued IDs = prefix+DispatchID;
equal "dispatch-1" across Sessions observed). With shared process Events and recorder (pkg/wire/worker_sessions_providers.go:95-103),
two runtimes that both dispatch "dispatch-1" would collide on worker-session/dispatch-1/events. Write that characterization
as a red-first witness. When it is red, stop structural cutover work and file the smallest identity delta (e.g. scope the
default Worker Session ID by runtime/Factory Session) to the flat-injection lead as a named prerequisite. Don't paper over it.
Record this boundary verbatim in the plan.
```

**Premise amendment, binding 2026-10-03 17:59Z option A:** the quotation above is historical authority, preserved verbatim. Its natural-equal/expected-red default Petri premise is superseded by current distinct-UUID characterization; plan correction belongs to the Lead. The Lead retains scoped keyed-supervision collision safety. No deterministic allocation edge or substituted identity is authorized. A pair key still cannot fix equal canonical Worker identities: equal admitted IDs address the same `Topic(ID)` bytes. That conditional risk remains a held gate, not an observed default Petri collision or an inferred waiver.

#### Native allocation and reachable consumers

Source pin: `50848bc5900123901733157755463c4098f2dcab`. Compare each file below with amendment starting head `46c26ae6314ad4ca4220fd886feae481fe82cfc5` using `git show <pin>:<path>` and `git diff <pin> <head> -- <path>`: all listed files are unchanged. Function names and literal branches are source anchors, not functional results. Prefixes: RT = `pkg/services/factory_runtime/internal/services/orchestration/runtime/`; JS = `pkg/services/factory_runtime/internal/services/orchestration/javascript/runtime/`; EX = `pkg/services/factory_sessions/internal/execution/`; SS = `pkg/services/factory_sessions/internal/service/`.

| Route | Native allocation and branch | Worker Sessions consumer / proof boundary |
| --- | --- | --- |
| Default Petri | `pkg/services/factory_runtime/internal/services/orchestration/subsystems/subsystem_dispatcher.go`, `buildWorkDispatch`: `DispatchID: d.newID()`. `pkg/wire/session_runtime_providers.go`, `provideFactoryRuntimeIDGenerator`: explicit edge wins; otherwise `return uuid.NewString`. | RT `dispatch_worker_sessions_cutover.go`, `runtimeAttemptPreparation` calls `BeginRuntimeAttempt`; `runtimeWorkerSessionID` starts with trimmed correlation DispatchID outside retry/replay. Initial Worker ID matches the UUID. Routing is not universal uniqueness or equal-ID safety. |
| Petri compatibility | RT `dispatch_worker_sessions_cutover.go`, `startThroughWorkerSessions`: `sessionID := dispatchID`; `WorkerSessionIDForDispatch` replaces it when found. | `Reserve` then `InvokeSession`, with recorded dispatch/Worker association. No naturally equal canonical Worker ID demonstrated. |
| JavaScript runtime-backed | JS `records.go`, collector identity: `fmt.Sprintf("dispatch-%d", index)`; EX `child_worker_executor.go` accepts `ReservedIdentity` and `workerDispatchIdentity` returns `sessionID + "/" + dispatchID` (blank test-composed session retains local text). | `Execute` calls `beginChildWorkerAttempt` before Workers Execute. Its `e.attemptStarter(...)` is bound by SS `open.go`, `runtimeWorkerAttemptStarter` / `setWorkerAttemptStarter`, to Runtime `BeginWorkerAttempt`, including the runtime-service branch. RT `worker_session_control_targets.go` calls `runtimeAttemptPreparation` then `BeginRuntimeAttempt`. Local `dispatch-1` text alone is no canonical-ID collision evidence. |
| JavaScript standalone/direct | EX `direct_child_executor.go`, `directChildExecutor.Execute`, uses local collector or reserved identity. Standalone composition selects this executor without a Factory Runtime. | Calls Workers Execute without an attempt starter; outside Worker Sessions on this route. Source non-reachability proves no functional isolation outcome. |
| JavaScript in-package compatibility | EX `direct_child_executor.go`, `legacyDirectChildExecution.Execute`, maps detached input to `workers.InvocationInput`. | `e.invocation.Execute(...)`, no Worker attempt starter on this direct bridge. A test/in-package survivor outside Wire's standalone production path. |
| JavaScript resume | JS `child_execution.go`, `ResumingChildExecutor.Execute`: increment, `fmt.Sprintf("dispatch-%d", e.next)`, return `CompletedChildResults[dispatchID]` if present; otherwise set `ReservedIdentity` and `return e.base.Execute(ctx, req)`. | Cached completion opens no new attempt; unfinished child follows its selected runtime-backed/direct base route. Resume spelling is no collision or safety result. |
| Runtime InvokeWorker | RT `invoke_worker.go` accepts incoming DispatchID. Stateless physical retries use `fmt.Sprintf("%s/attempt/%d", dispatchID, attemptNumber)`; compatibility `reserveWorkerSession` tries DispatchID then `fmt.Sprintf("%s/resume/%d", dispatchID, attempt)` on duplicate reservation. | Stateless route uses `runtimeAttemptPreparation`; compatibility uses `InvokeSession`. Both reach Worker Sessions. Equal canonical ID behavior remains T16-REAL-PATH-COLLISION. |
| Replay / reopened child | RT `dispatch_worker_sessions_cutover.go`, `runtimeWorkerSessionID` / `startThroughWorkerSessions`, use recorded `WorkerSessionIDForDispatch` when found. `allowRetry` returns nonempty physical AttemptID first. RT `worker_session_control_targets.go`, `BeginWorkerAttempt`, selects retry after `terminalWorkerSessionRequiresRetry` observes prior terminal state. | Preserve recorded association and physical retry allocation. Source routing only; replay/resume isolation still needs owning functional gates. |

Canonical publication remains `pkg/services/worker_sessions/topic.go`, `Topic`: `worker-session/<id>/events`. Worker capture retains the exact RecordingID/FactorySessionID/WorkerSessionID/Topic association (`WS/service.go` capture request; `pkg/services/recordings/internal/services/worker_capture/ports.go` validation). Duplicate Worker ID admission still returns `ErrSessionAlreadyExists` without changing the original. Runtime-selected handles, overrides, clocks, scheduling, retry/replay policy and public topic bytes are unchanged.

#### Retained strict witness and attribution

Artifacts: `C:/Users/andre/work/portos/infinite-you/docs/temp/projects/flat-injection/validation/t16-idchar/strict-isolation.log` and `strict-isolation.patch`. Retained characterization commit: `7909d56f461492628ff4ec91adedfe54b98f26a6`; separately observed active characterization head: `36d850156743f9eaa2c9b717bb139a111ac5d130`, PR #2742, work-task-39. Its unmerged findings remain provisional and separately owned; this amendment claims no task39 delivery.

Strict command:

```text
go test -p 1 ./tests/functional/factory_runtime/root_composition -run '^TestFactorySessionsEqualFirstDispatchDefaultWorkerIdentityCharacterization$' -count=1 -v
```

The retained patch adds `if entryA.dispatchID != entryB.dispatchID` before the distinct-default allocation assertion. The available log ends with this exact failure and a FAIL footer (retained exit status 1):

```text
dispatch_worker_sessions_concurrency_activation_test.go:589: BLOCKED: required natural equal-dispatch premise is false: A=913d7f61-1050-47b9-803c-7a7bce12e8b3 B=02a0aef2-4d09-4d2d-89b0-26dc510f49a3
--- FAIL: TestFactorySessionsEqualFirstDispatchDefaultWorkerIdentityCharacterization (1.78s)
```

| Observation | A | B |
| --- | --- | --- |
| Factory Session | `f2748fae-662a-41de-991e-690310a36ece` | `71168e07-3329-4967-adf0-b81e7971d169` |
| Dispatch = initial Worker ID | `913d7f61-1050-47b9-803c-7a7bce12e8b3` | `02a0aef2-4d09-4d2d-89b0-26dc510f49a3` |
| Exact Worker topic | `worker-session/913d7f61-1050-47b9-803c-7a7bce12e8b3/events` | `worker-session/02a0aef2-4d09-4d2d-89b0-26dc510f49a3/events` |
| Recording association | `worker-recording-8c14977eafec9651bef998d034433c08e9d74d799b97e01f1d37961bc624b6d6` | `worker-recording-578914063681760ca27509749aa4f43f8a43962acd64a35f66e68d14e05dd07a` |
| Retained frames | 6 | 6 |

Classification: **BLOCKED-premise**. Retained public reads and capture snapshots show distinct scoped observations in this run; the strict equality assertion establishes no collision. They prove neither an observed isolation failure nor collision PASS, universal allocator uniqueness, current amendment-head runtime PASS, shared-registry safety or task39 merge.

The admitted packet separately attributes an earlier line590 failure to A `cefea841-8e2a-4248-908e-2a4300a8ead6` / B `bdf0fab5-ab9b-4cf2-b087-0455189de746`. Those bytes differ from the available line589 log above. The earlier artifact remains unavailable for byte-exact verification; retain its attribution separately and keep exact-artifact proof BLOCKED. Do not combine observations or fabricate a replacement run.

#### Held safety gates and release events

| Gate | Owner / public observer / fidelity | Required release event |
| --- | --- | --- |
| T16-CHAR-PRODUCTION-ID | work-task-39 author and ordinary reviewer; explicit Factory Session dispatch/Worker event reads and recording association; functional production allocation/Events/recorder with controlled provider runner. | Independently reviewed characterization PR merges. Distinct-UUID characterization releases only this prerequisite, never shared supervision. |
| U03-T16 | Later structural T16 owner and independent reviewer; Worker Sessions operation/control/read/publication contracts with isolated narrow collaborators; component/unit controlled normal and race evidence. | Characterize current per-runtime factory, then prove approved keyed-supervisor selection/lifecycle/correlation for equal logical dispatch and distinct supplied Worker IDs. D01-D06 and duplicate-ID rejection remain required; synthetic IDs cannot replace production proof. |
| T16-REAL-PATH-COLLISION | Lead owns admission/disposition; later admitted T16 proof owner executes. Supported real entry -> explicit Factory Sessions -> public Worker lifecycle/events/control and recording association; observe typed duplicate admission or exact topic contamination and peer outcome. Functional local-real allocation/routing/Events/recorder with controlled provider edge. | Every applicable existing path has independently reviewed public proof or explicit Lead disposition supported by exact reachability evidence. Source non-reachability alone is no functional isolation proof. No inferred waiver. |
| T16-IDENTITY-DELTA (contingent) | Lead proposes prerequisite; operator decides identity/topic/policy change; separate implementation owner. Same public collision observer plus replay/retry compatibility; later authorized functional/contract proof. | Only an observed real collision requiring correction triggers this gate. Authorized prerequisite must independently merge; premise failure authorizes no allocator or identity change. |
| T16-CUTOVER | Structural T16 and composition steward; T15 consumes completed result. F05a-d/F06 public outcomes plus S01-T16/G01/G02; functional controlled plus static/generation. | Merged AM09 contract, merged production characterization, U03-T16 proof and disposition of every applicable real-path collision, plus any required authorized identity delta. This docs merge releases no cutover. |

F06/F07/F08 retain public session output/events/cursors, durable flush and replay/resume proof with T13/T15/T16/T17 and T03, then independent VAL01. Full T16 and FI-A1-A8 remain unproven. ALLOC-SOURCE/WITNESS are static local-real source/retained-artifact analysis only. Independent ALLOC-LOOPBACK/ALLOC-REVIEW repeats classification from the clean pushed head and reports PASS/FAIL/BLOCKED through `factory/docs/standards/validation-loopback-template.md` without silent repairs. Terminal exact-head CI, current-main rebase and immediate premerge `make lint pkg-file-count` belong to review. Companion consistency and final author handoff belong to story 002.

### Exact later removal and gates

T16 removes the entire `WorkerSessionsFactory` declaration/provider/caller chain together only after the T16-CUTOVER prerequisites above are released. UUID characterization does not discharge U03-T16 or T16-REAL-PATH-COLLISION; T16-IDENTITY-DELTA is contingent on an observed collision and authorized correction. Current production identities:

- Declaration: `pkg/services/factory_runtime/composition_contracts.go:84`.
- Providers: `pkg/wire/worker_sessions_providers.go:61,95` (`provideWorkerSessionsFactory`, `provideWorkerSessionsFactoryWithRecorder`); registration `pkg/wire/wire.go:47`.
- Construction call: `pkg/services/factory_runtime/internal/build.go:400`; threaded factory inputs in NewRuntime/newRuntimeFromBundle (`:140,362`).
- Assembly storage and propagation: `pkg/services/factory_runtime/internal/assembly.go:28,40,164`; owner Wire NewAssembly at `pkg/services/factory_runtime/wire/assembly.go:76-80`.
- Runtime bundle/replay propagation: RB `:258,334,370,385,435,456-475,545-552,605-631`; remove bindProviderSessionProgress and its late publisher.Bind coupling, not a peer's unrelated functionality.
- Generated consumers: `pkg/wire/wire_gen.go:218,242,752`, regenerated only by later T16 via `make generate-wire`; affected factory fixtures/callers must be updated together. Remove the separate BindRuntimeAttemptCancellation structural call at DC `:111-151` when Begin installs cancellation atomically.

The similarly named `workerSessionsFactorySessionScopeResolver` in `pkg/wire/runtime_inputs.go` resolves Factory Session observation scope; it is not a WorkerSessionsFactory constructor and is not authorized for deletion by a name match. Run a fresh symbol/caller inventory on the later structural head; these anchors are a current-source removal checklist, not an exhaustive future path allowlist. T15 waits for completed T16; AM16/T12 and other companion sections are unchanged.

Later T16 owns U03-T16 component-isolated unit cases for distinct supplied IDs with equal dispatch, selected overrides/replay/clocks, opening/capture failures, failed controls, stale retries/late progress, idempotent completion and exact scoped close/retained observations. First characterize the current per-runtime factory; only port the witness after approved cutover. T16-CHAR-PRODUCTION-ID is functional through reusable `root.BuildProcess` + public `Process.Execute`/explicit Factory Sessions with supported external edges, actual default allocation. The retained expected-equal default Petri assertion is BLOCKED-premise, not isolation failure; distinct-UUID characterization remains provisional until its own review/merge. It cannot be replaced by supplied IDs, source scanning or internal-map assertions.

T08/T09/T16 retain every selected F05a success, F05b denial/no unauthorized command, F05c selected-scheduler normalized timeout and F05d one-of-two cancellation witness; F06 overlaps four explicit sessions with attributable output/events/log correlation/cursors. Plan the complete customer matrix before adding tests. Functional cases share one safe root process per immutable edge shape, allocate explicit sessions before runtime construction, own profiles/routes/streams/files/fakes, run with bounded parallelism and synchronize on admission/opening/terminal signals rather than sleeps. Unit tests use isolated narrow owner fakes; later concurrent changes need focused normal/race evidence. Functional tests build no binary; I01 consumes the invoking build lane's prebuilt artifact, P01 stays in dedicated stress, inventories in lint/static checks.

This author slice proves only T16-DOC-SOURCE and T16-DOC-REHEARSAL at local-real source fidelity, plus T16-DOC-LINT (`go run ./cmd/markdown-linter docs/internal/development/plans/flat-injection`, `git diff --check`). Independent review from a clean pushed-head checkout owns T16-CONTRACT-REVIEW/LOOPBACK using `factory/docs/standards/validation-loopback-template.md`: repeat D01-D07 and source/lint procedures, report criterion IDs PASS/FAIL/BLOCKED without silent repairs, and request the smallest delta on failure. Review owns terminal required CI, current-main rebase and immediately premerge `make lint pkg-file-count`; no stale-green merge. Implementation stops at final head pushed, PR open, CI started and blocking feedback addressed.

U03-T16, F05a-d/F06/F07/F08, T16-CHAR-PRODUCTION-ID/T16-REAL-PATH-COLLISION (and any required T16-IDENTITY-DELTA), S01-T16/G01/T16-CUTOVER, U01/F15/F16, U02/F14, S01/G02, I01/P01 and independent integrated VAL01 remain unproven. FI-A1/FI-A4/FI-A8 remain Project obligations; documentation is no runtime, speed, resource-release or Project acceptance claim. Preserve the dedicated P01 baseline/final ten-sample median/p95 nonregression and lifecycle-cycle requirements unchanged.

## T17 — Sessions opening and removal of peer-service bags

Authored source: `pkg/services/factory_sessions/internal/service/factory.go`.

Current:

```go
func NewRoot(
	providerSessions *ProviderSessionsPorts,
	factoryRuntime *FactoryRuntimePorts,
	factoryDefinitions *FactoryDefinitionsPorts,
	factorySessions *FactorySessionsPorts,
	workPorts *WorkPorts,
	automationsPorts *AutomationsPorts,
	modelsPorts *ModelsPorts,
	recordingsPorts *RecordingsPorts,
	webhooksPorts *WebhooksPorts,
	workersPorts *WorkersPorts,
	operatorSettings *OperatorSettingsPorts,
) (*Root, error)
```

Proposed:

```go
func NewRoot(
	assembly roles.RuntimeAssembly,
	durable durableexecution.Service,
	start func(context.Context, factorysessions.SessionStartRequest) (factorysessions.SessionStartResult, error),
	liveChangeCoordinator factorysessioncontracts.LiveChangeCoordinator,
) (*Root, error)
```

The fixed start operation belongs to the preconstructed opening owner. Separate request preparation, definition/replay selection, persistence, runtime activation and observation routing; inject their exact capabilities directly. Never implement start by reconstructing the old graph. Root retains its public behavior via direct assembly/durable operations. The existing mode-neutral `SessionStartRequest`/`SessionStartResult` and sync/async `StartRequest` results remain unchanged.

Delete all eleven `*Ports` structs/aliases and `wire.Struct` registrations; delete execution-construction factories after durable callers migrate. Remove `runtimeProducts` and copied peer services in SessionState. Retain only session/runtime/generation IDs, model scope, worker-setting snapshots, replay warnings/recovery metadata, diagnostics, state and cleanup handles under their actual owners.

Authored source: `factory_sessions/internal/modelinvocation/model_invoker.go`.

Current:

```go
type RuntimeModelInvokerConfig struct {
	Models   models.Service
	Scope    models.RuntimeScopeRef
	Sessions interface {
		GetFactorySession(context.Context, string) (factorysessions.SessionProjection, error)
	}
	Workers          workers.Service
	RuntimeID        string
	GenerationID     string
	FactoryDirectory string
	WorkingDirectory string
}
```

Proposed:

```go
// Removed: RuntimeModelInvokerConfig dependency bag.
type RuntimeModelInvocation struct {
	Scope models.RuntimeScopeRef
	RuntimeID string
	GenerationID string
	FactoryDirectory string
	WorkingDirectory string
}
type RuntimeModelInvocationOperation interface {
	InvokeRuntimeModel(context.Context, RuntimeModelInvocation, string, models.Request) (models.Result, error)
}
```

Inject Models, session projection and Workers into the reusable invocation owner; its operation receives only the scoped facts plus existing model name/request. No new invoker service is constructed for a session. Preserve scope readiness, generation routing and the existing request-scoped Workers execution path.

## T18 — Direct HTTP/MCP/visualization consumers

> Validation review: the `httpRuntimeBinding` and `SessionPresentation` pairs below are implemented by **T28** (depends on T17/T18). T18 implements direct adapters only, without changing these signatures. Five MCP `RootDependencies` wrappers with no production importer are deleted by T24.

Authored source: `pkg/wire/http_runtime_binding.go`.

Current:

```go
type httpRuntimeBinding func(*factorysessionwire.Root, string, initializer.InvocationCancellation) (http.Handler, error)
```

Proposed:

```go
type httpRuntimeBinding func(string, initializer.InvocationCancellation) (http.Handler, error)
```

Authored source: `factory_sessions/internal/service/application_session.go`.

Current:

```go
type SessionPresentation struct {
	FactoryRuntime       factoryruntime.Service
	ModelsScope          models.RuntimeScopeRef
	ModelInvoker         workers.ModelInvoker
	WorkerSessions       workersessions.ObservationService
	Logger               *zap.Logger
	Reader               roles.RuntimeReader
	Projections          recordings.ProjectionService
	Clock                factoryruntime.Clock
	MetricsRootDir       string
	OperatorSettingsPath string
	Recordings           recordings.Service
}
```

Proposed:

```go
type SessionPresentation struct {
	ModelsScope models.RuntimeScopeRef
	RuntimeID string
	GenerationID string
	MetricsRootDir string
	OperatorSettingsPath string
}
```

HTTP binding captures injected prebuilt adapters/mappers and scoped queries, never a Sessions root used for service lookup. Services, clock, base logger, projections, Worker observations and model invocation go directly to consuming adapters. Retain selected session/host facts and per-host cancellation; allocate listener/connection resources under transport lifecycle. Preserve missing/expired errors and SSE retained/live/gap/close behavior. Visualization receives projections and runtime facts; MCP receives operations and selection facts. Remove root peer-service getters only after the last consumer migrates.

## T23 — Strict constructors and explicit disabled roles

Internal constructors presume their required injected collaborators are real and assign/use them directly. The caller-facing construction boundary validates supplied effects and capabilities once; canonical Wire selects optional defaults and explicit disabled implementations. Nil/typed-nil errors belong at that boundary. Internal constructors do not repeat requiredness checks or repair injection by selecting another implementation; operational/domain validation remains required.

Authored source: `pkg/services/factory_runtime/logging.go`.

Current:

```go
func NewSessionLogger(
	base *zap.Logger,
	sessionID string,
	folderPath string,
	factoryDir string,
) *zap.Logger {
	if base == nil {
		base = zap.NewNop()
	}
	return base.With(
		zap.String("session_id", sessionID),
		zap.String("folder_path", folderPath),
		zap.String("factory_dir", factoryDir),
	)
}
```

Proposed:

```go
func NewSessionLogger(
 base *zap.Logger,
 sessionID string,
 folderPath string,
 factoryDir string,
) *zap.Logger {
 return base.With(
  zap.String("session_id", sessionID),
  zap.String("folder_path", folderPath),
  zap.String("factory_dir", factoryDir),
 )
}
```

Keep the existing `SessionLoggerFactory` signature; no new logger-requiredness error or repeated internal guard is introduced. Delete duplicate `internal/build.go:newSessionLogger`; remove nop fallbacks in Sessions root normalization, HTTP handlers/root binding, live-change construction and remaining build owners. One explicit Wire-selected disabled logger is valid. Correlated `.With` derivation and scoped file sinks remain intentional.

Move the required-effect classification currently in Sessions' `factory.go:missingPortDependency` to caller-boundary validation as appropriate; retire the port-bag helper with the bags. That boundary rejects present typed-nil interfaces and nil effect functions without invoking them. Internal constructors directly assign valid dependencies. Preserve fallible construction/resource errors with named causes; do not add `(value, error)` signatures solely to recheck injection.

Disabled progress publisher, completion observer, telemetry recorder and identity mock-runner wrapper are explicit non-nil implementations/functions selected in Wire. They have zero-effect semantics; they do not fabricate records. An absent request provider override means use the already injected Providers service, not no-op provider execution.

Do not classify nil replay artifacts/restored state, empty lists, tri-state permission overrides or domain configuration defaults as missing injection. Replay time remains distinct from process/OS deadlines. Allocation of owned maps, buffers, handles, lifecycle-manager state, artifacts and identity via an injected generator remains legitimate. Documented cancellation detachment for durable cleanup is separate from constructor fallback.

Boundary unit evidence covers nil pointer/function/interface, typed nil, explicit disabled role, valid role, no collaborator call before complete validation, and preserved downstream resource error cause. Internal constructor witnesses use valid direct fakes without requiredness branches. Static lint checks forbidden topology/fallback sites. Functional evidence proves customer behavior and never asserts constructor counts. Characterize previously unprotected fallback semantics inside the affected lane before replacing them.

## T31 — Runtime instance-host build leaves

Authored sources: `pkg/services/factory_runtime/internal/services/instance_host/build/service.go` (`BundleBuilder` / `Service`), `orchestration/runtime/worker_session_control_targets.go` (`WorkstationRequestExecutorConfig`).

**Pending — authored by T31 as its first required step, before any structural change (operator decision, 2026-10-02).** Add the current/proposed Go pair for both types here: injected collaborators separated from a request-resolution value type, with the deferred graph-construction closure removed. If the pair cannot be written within T31's outcome, T31 stops and returns a delta plan.

## v1.1 unchanged public contracts and held proof

AM01: CLI quiet+JSON/explicit-output remains INVOCATION_OUTPUT_CONFLICT; use separate quiet/JSON/response-stream NDJSON/verbose witnesses. Default logging is terminal-muted; typed model failure remains empty stdout and existing stderr, no success artifact and capacity recovery. No public output or logging policy change.

AM03/AM04: no HTTP/MCP Settings load/update routes are created. Existing canonical CLI unknown-field/precedence/failure behavior is F12; dormant adapters are unit compatibility. Canonical AGY selection stays command execution with supplied runner and zero PTY effects; absent-override default composition stays inert and legacy PTY behavior is separate owner-unit compatibility.

AM05: RetainWorktree/reused checkout semantics remain unchanged. T09 releases attempt resources and nonretained checkout; T15 owns retained session/generation checkout disposition. Deleting retained checkouts requires an operator policy decision, not constructor cleanup.

AM10: no public observation API or baseline instrumentation is introduced. FI-PREREQ-BASELINE-OBS must independently prove read-only private handles/leases/capacityHolders observation at original pin; T27/T22 P01 stays blocked if infeasible.

AM11: post-start cancellation plus failing final-flush writer and injected UTC clock preserve terminal causes/metadata; absent clock returns ErrInvalidRecordingTerminalMetadata and leaves FinalizedAt unset. Keep retained T03 head and F07/F08; no successful zero-time finalization claim.

AM12: MCP start_sync completion and ordered read_events for its returned durable session remain M01/F13. FI Sessions/Recordings/MCP owns FI-PREREQ-MCP-DISCOVERY for FI and WSV; diagnose same identity/recording lookup. Public-contract/policy correction needs operator authority; no completion-only/mock substitute.

AM13: no replacement-webhook behavior/policy contract is authorized here. FI Sessions/Runtime/Webhooks owns FI-PREREQ-WEBHOOK-READINESS, authority pending operator; explicit authorization and prerequisite merge precede T26 cutover. Characterization preserves both retained heads and archived reproduction, with F18a-f unchanged.

AM14/AM15: constructor and public contracts gain no quality/acceptance exemption. Authors own evidence and implementation handoff; independent VAL25/VAL01/review owns artifact/Linux/terminal CI/merge, and Factory Reliability owns FI-SHARED-QUALITY release. Retained T01/T06/T25 work is preserved without inventing terminal status.
