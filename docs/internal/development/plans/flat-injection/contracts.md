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

**AM16 / T12 — approved private behavior-owner decomposition (binding 2026-10-03T10:20Z).** Authority: `C:/Users/andre/work/portos/infinite-you/docs/temp/operator-mailbox/responses/flat-injection.md`, Decision 2026-10-03T10:20Z; this supersedes the 09:05Z deferral only. Retain draft [#2683](https://github.com/portpowered/you-agent-factory/pull/2683) at `62cff1c7cfb4d734d462aa1ef08d2d5e0e6c4ba7`; its stories 002/003 and full F10/S01 remain incomplete. The separately authorized `fi-plan-amendment-am16-t12-20261003` docs lane depends on merged v1.1 #2685 and owns only this six-companion packet. The retained T12 successor depends on this docs lane complete; it does not wait for T20 or WSV. T12 owns implementation, same-PR stale script_pollers baseline deletion and obsolete-helper removal. T20 owns unchanged one-interface enforcement and measurement; deadcode allowance remains 0. No public API, configuration, event, persistence-policy or acceptance change is authorized. Native capability/provider/caller/removal pairs C01–C16 are canonical in [contracts.md](contracts.md#t12--automations-runtimesource-isolation). The docs author handoff requires final head pushed, open PR, CI started and blocking feedback addressed; independent AM16-DOC-VAL/REVIEW owns clean-room loopback, terminal CI and current-main merge. The successor still requires exact-head T12-F10 (F10a–m), T12-S01 and T12-G01/G02, independent review and merge. Historical partial evidence never substitutes for those gates.

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

Authored source: `pkg/services/factory_runtime/composition_contracts.go`.

Current:

```go
type WorkerSessionsFactory func(workers.Service, platformclock.Source) (workersessions.Service, error)
```

Proposed:

```go
// Removed: WorkerSessionsFactory.
type WorkerAttemptOpener interface {
	BeginRuntimeAttempt(context.Context, workersessions.RuntimeAttemptRequest) (workersessions.RuntimeAttempt, error)
}
```

Inject the process Worker Sessions service and its attempt-opening capability directly; remove `provideWorkerSessionsFactoryWithRecorder`. Retain existing attempt request, idempotent completion handle, association and Events contracts. Recorder/progress routing uses explicit session/attempt identities; it must not construct another service or overwrite shared mutable routing when sessions overlap. Observation retention remains owned by Worker Sessions and Events.


> Validation review: the proposed `NewService` above has the same signature as the current one. The real change is per-attempt routing: today each runtime builds Worker Sessions with its own `workers.Service`. That service carries the session's provider and command-runner overrides, replay runner, session/runtime/recording IDs and clock (`factory_runtime/internal/runtime_build.go:477-505,621-631`). State is keyed by bare dispatch ID (`worker_sessions/.../invoke_session.go:294-310`). AM09: T16 removes WorkerSessionsFactory declaration, provider and all call sites after its separately merged keyed-attempt contract PR and characterization. T15 consumes shared supervision and depends on T16; it does not delete the factory.

**Pending — authored by T16 as its first required step, in its own PR (operator decision, 2026-10-02).** Add here the current/proposed Go pair for the attempt request. That request carries the runtime ID, the execution handle, the session overrides and the clock/scheduler, and Worker Sessions state is keyed by (runtime ID, dispatch ID). If keying is not feasible, T16 stops and returns a delta plan (plan §2 replanning trigger); no structural T16 work proceeds first.

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
