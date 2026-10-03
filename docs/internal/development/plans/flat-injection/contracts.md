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

Authored source: `pkg/services/automations/internal/service.go`.

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
) *Service
```

Proposed:

```go
func newService(
	logger *zap.Logger,
	clock Clock,
	reconciler reconciliation.Service,
	scriptPollers scriptpollers.Service,
	cronService cron.Service,
	filesystemWatchers filesystemwatchers.Service,
	hostedPollers automations.HostedPollers,
) *Service
```

Runner, template, policy and cursor effects move to their consuming leaves. Workflow/directory values come from runtime activation snapshots; they no longer select a newly constructed runtime service tree. Preserve existing default-workflow resolution in request preparation.

Authored source: `pkg/services/automations/internal/services/reconciliation/service.go`.

Current:

```go
type Effects struct {
	Start func(context.Context, StartEffect) error
	Stop  func(context.Context, StopEffect) error
	Wait  func(context.Context, WaitEffect) (automations.SourceObservation, error)
}
```

Proposed:

```go
// Removed: Effects dependency bag.
type SourceLifecycle interface {
	Start(context.Context, StartEffect) error
	Stop(context.Context, StopEffect) error
	Wait(context.Context, WaitEffect) (automations.SourceObservation, error)
}
```

Extract `schedulerSources` and its start/stop/wait behavior into the source-lifecycle owner. Inject that owner into reconciliation, rather than callbacks into Automations Root. Expose construction aliases `Reconciliation`, `ScriptPollers`, `Cron`, `FilesystemWatchers` and `SourceLifecycle` through `automations/wire`. They alias the corresponding private contracts, not concrete implementations.

Keep `buildRuntimeInstance(context.Context, automations.RuntimeActivationRequest) (*runtimeInstance, error)` as scoped state/resource allocation. Delete its `NewWithCursorFileSystem` call and `runtimeInstance.owner *Service`. Retain runtime/session/source identities, snapshots, cursor values, watcher handles, contexts, cancel functions, wait groups and startup flags. Inject cursor behavior; select the existing authored destination by runtime/source identity. Characterize and preserve the existing durable-cursor-error-to-memory behavior during structural migration. A separately reviewed persistence policy delta is required before replacing that fallback with a surfaced error.

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
func NewLifecycleService(
 clock factoryruntime.Clock,
 scheduler platformclock.TimerSource,
) (*LifecycleService, error)
func WaitForStart(
 ctx context.Context,
 handle *Handle,
 scheduler platformclock.TimerSource,
) error
```

AM08: delete the one-field instancehost.Dependencies wrapper. LifecycleService stores both Clock and TimerSource; forward scheduler to WaitForStart and all readiness callers (including legacy Assembly host injection). Current lifecycle signature is `NewLifecycleService(clock factoryruntime.Clock) (*LifecycleService, error)` and current readiness helper is `WaitForStart(ctx context.Context, handle *Handle) error` (`host/service.go:16`; `host/lifecycle.go:164`). Proposed signatures above separate replay-sensitive timestamps from OS scheduling. Preserve the one-second ceiling and 10ms poll using cancellable NewTimer loops; never fall back to real time or use replay ticks for readiness. Host handles remain keyed by runtime identity. T15/T31 build redesign is not part of this minimal propagation.

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
