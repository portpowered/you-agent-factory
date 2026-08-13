# Runtime opening ownership matrix

Status: P0 characterization, reconciled against current main at 558e9ac47 on 2026-08-10.

This is the reviewable ownership inventory for the current Factory Sessions
runtime-opening seam. It describes where each input or opening operation should
retire, without changing runtime behavior. The matrix is deliberately
maintained as documentation: it is not generated code and no source-inventory
test should be added for it.

## Scope and source reconciliation

The source of truth for the first four sections is intentionally narrow:

- D rows enumerate the fields of the ten grouped dependency structs in
  pkg/services/factory_sessions/internal/runtimeopening/factory.go. The
  flattened private Factory fields are projections of these fields and are not
  counted again.
- E rows enumerate every field of ExternalEffects in
  pkg/services/factory_sessions/internal/runtimeopening/factories.go.
- F rows enumerate the deferred construction, callback, and adapter roles
  declared by the runtime-opening package and its durable-provider helper.
- L rows enumerate the late completion, attachment, and binding operations in
  pkg/services/factory_sessions/internal/runtimeopening/open.go and the
  adjacent application and Wire binding surfaces.

| Source surface | Count | Matrix rows | Reconciliation |
| --- | ---: | --- | --- |
| Grouped Dependencies fields | 44 | D-01 through D-44 | 1 + 8 + 8 + 9 + 2 + 2 + 1 + 7 + 5 + 1 |
| Invocation-local ExternalEffects fields | 15 | E-01 through E-15 | Every declared ExternalEffects field appears once |
| Deferred construction and callback roles | 38 | F-01 through F-38 | Every retained role is listed once; explicit compatibility/deletion roles are visible |
| Late completion, attachment, and binding operations | 13 | L-01 through L-13 | Every current late operation is listed once |
| **Total characterized rows** | **110** | **D, E, F, and L** | **44 + 15 + 38 + 13 = 110** |

Classification is exclusive. Each row has exactly one classification and one
target owner or deletion outcome:

- **build-time dependency** — selected once by canonical Wire composition and
  passed through a narrow owner contract.
- **operation value** — supplied for one invocation, runtime, or hosted source
  and not retained as a service-valued dependency bag.
- **private owner state** — state that belongs inside the target owner and may
  require an explicit lifecycle or cleanup boundary.
- **delete** — a compatibility bridge, locator, callback injector, or exposed
  private assembly role that has no destination owner.

Packet references use FSE for the Factory Sessions decomposition packets, WSE
for the Workers stateless-execution packets, PSS-I01 for root/Wire/initializer
composition, and BTRC-P1 for the follow-on characterization packet.

## D. Grouped dependency fields

| ID | Stable name | Current role | Classification | Target owner or deletion | Lifecycle / cleanup | Retirement packet |
| --- | --- | --- | --- | --- | --- | --- |
| D-01 | ProviderSessions.Service | Provider Sessions service root used during opening | build-time dependency | Provider Sessions | Process-scoped root; closed by process lifecycle | FSE-07 |
| D-02 | FactoryRuntime.FactoryWorkflows | JavaScript workflow definition provider | build-time dependency | Factory Runtime | Process-scoped immutable capability | FSE-05 |
| D-03 | FactoryRuntime.WorkflowPreview | Workflow preview operation | build-time dependency | Factory Runtime | Process-scoped operation | FSE-05 |
| D-04 | FactoryRuntime.WorkersRuntimeExecutorsFactory | Workers runtime-executor constructor supplied to Runtime | build-time dependency | Factory Runtime | Constructor owned by process composition | WSE-03 |
| D-05 | FactoryRuntime.ProviderInvocationFactory | Provider invocation-executor constructor | build-time dependency | Factory Runtime | Constructor owned by process composition | FSE-03 |
| D-06 | FactoryRuntime.WorkersMockCommandRunnerFactory | Mock command-runner constructor used for durable execution | build-time dependency | Factory Runtime | Constructor owned by process composition | WSE-07 |
| D-07 | FactoryRuntime.FactoryRuntimeAssembler | Runtime assembly capability | build-time dependency | Factory Runtime | Assembled runtime closes through runtime opening cleanup | FSE-05 |
| D-08 | FactoryRuntime.ResolveClock | Runtime clock resolver | build-time dependency | Factory Runtime | Selected clock belongs to the opened runtime | FSE-05 |
| D-09 | FactoryRuntime.NewSessionLogger | Session logger factory | build-time dependency | Factory Runtime | Logger lifetime follows opened session and process logger | PSS-I01 |
| D-10 | FactoryDefinitions.Validator | Factory definition validator | build-time dependency | Factory Definitions | Process-scoped validator | FSE-02 |
| D-11 | FactoryDefinitions.NamedPaths | Named-path resolver | build-time dependency | Factory Definitions | Process-scoped resolver | FSE-02 |
| D-12 | FactoryDefinitions.Factory | Factory Definitions service constructor | build-time dependency | Factory Definitions | Constructed service is owned by Definitions | FSE-02 |
| D-13 | FactoryDefinitions.InitialFactorySnapshotFactory | Initial snapshot constructor | build-time dependency | Factory Definitions | Snapshot belongs to definition lifecycle | FSE-02 |
| D-14 | FactoryDefinitions.LoadFactory | Loaded-factory loader | build-time dependency | Factory Definitions | Loaded source is closed by its owning runtime | FSE-02 |
| D-15 | FactoryDefinitions.NewLoadedFactory | Loaded-factory source constructor | build-time dependency | Factory Definitions | Loaded source lifecycle follows opened runtime | FSE-02 |
| D-16 | FactoryDefinitions.DecodeReplayConfig | Replay configuration decoder | build-time dependency | Factory Definitions | Stateless process-scoped operation | BTRC-P1 |
| D-17 | FactoryDefinitions.CaptureLoadedFactorySnapshot | Loaded-factory snapshot capturer | build-time dependency | Factory Definitions | Snapshot artifact owned by recording/definition flow | FSE-02 |
| D-18 | FactorySessions.Service | Factory Sessions service root | build-time dependency | Factory Sessions | Root service owns runtime session lifecycle | FSE-01 |
| D-19 | FactorySessions.DurableExecutionFactory | Durable execution service constructor | build-time dependency | Factory Sessions | Durable execution closes with runtime cleanup | FSE-05 |
| D-20 | FactorySessions.FactorySessionExecutionFactory | Session execution constructor | build-time dependency | Factory Sessions | Execution service owns its close boundary | FSE-05 |
| D-21 | FactorySessions.FactoryScaffoldInitializer | Factory scaffold initializer | build-time dependency | Factory Sessions | Initialization/rollback owned by initializer/session boundary | FSE-02 |
| D-22 | FactorySessions.EditableFactoryValidator | Editable factory validator | build-time dependency | Factory Sessions | Stateless validation operation | FSE-02 |
| D-23 | FactorySessions.ProcessRuntimeFactory | Process runtime binding constructor | build-time dependency | initializer | Process lifecycle owns binding and unwind | PSS-I01 |
| D-24 | FactorySessions.GenerateRuntimeInstanceID | Runtime instance ID generator | build-time dependency | Factory Sessions | ID is created per opened runtime; no cleanup | FSE-01 |
| D-25 | FactorySessions.ResolveHome | Home-directory resolver | build-time dependency | initializer | Process-scoped environment resolution | PSS-I01 |
| D-26 | FactorySessions.ProviderIdentities | Provider identity resolver | build-time dependency | Providers | Resolver belongs to provider identity policy | FSE-07 |
| D-27 | Work.Factory | Work service constructor from runtime resolver | delete | delete: runtime-opening WorkFactory seam | Removed; canonical Wire owns the Work root | WSE-04 |
| D-28 | Work.ContentMaterializer | Work content materialization capability | build-time dependency | Work | Materialized content cleanup belongs to Work | WSE-04 |
| D-29 | Automations.Factory | Automation service constructor | build-time dependency | Automations | Automation service closes with runtime scope | WSE-08 |
| D-30 | Automations.HostedSourcesFactory | Hosted-source constructor | build-time dependency | Automations | Hosted pollers close with Automations | WSE-08 |
| D-31 | Models.Service | Models service root used for runtime scope binding | build-time dependency | Models | Model runtime scope cleanup is reverse-unwound by opener | WSE-03 |
| D-32 | Recordings.ProjectionFactory | Recording projection constructor | delete | delete: per-opening Recordings projection factory | Shared stateless projection is owned by the canonical root | FSE-06 |
| D-33 | Recordings.LifecycleFactory | Recording lifecycle constructor | delete | delete: per-opening Recordings lifecycle factory | Opaque recording scopes own lifecycle state in Recordings | FSE-06 |
| D-34 | Recordings.RuntimeLedgerFactory | Runtime ledger constructor | delete | delete: per-opening Recordings runtime-ledger factory | Runtime ledgers are opened and routed by the canonical root | FSE-06 |
| D-35 | Recordings.RuntimeRecorderFactory | Runtime recorder constructor | delete | delete: per-opening Recordings runtime-recorder factory | Scope owner finalizes runtime recorders exactly once | FSE-06 |
| D-36 | Recordings.ReplayClockFactory | Replay clock constructor | delete | delete: per-opening Recordings replay-clock factory | Replay clock is a root capability | FSE-06 |
| D-37 | Recordings.ReplayExecutionFactory | Replay execution constructor | delete | delete: per-opening Recordings replay-execution factory | Replay execution is a root capability | FSE-06 |
| D-38 | Recordings.ReplayInputs | Replay input loader | delete | delete: runtime-opening replay-input field | Replay input loading is retained by the canonical root | FSE-06 |
| D-39 | Workers.ExecutionFactory | Workers execution constructor | build-time dependency | Workers | Workers runtime/session build closes through runtime cleanup | WSE-02 |
| D-40 | Workers.RuntimeFactory | Workers runtime constructor | build-time dependency | Workers | Workers runtime is closed by session build cleanup | WSE-02 |
| D-41 | Workers.LocalRuntimeHooksFactory | Unused local-runtime hook constructor retained in opening dependencies | delete | delete: unused runtime-opening bridge | No lifecycle; remove validation, storage, and composition | WSE-03 |
| D-42 | Workers.AdaptCommandRunner | Process command-runner adapter | build-time dependency | Workers | Adapter is process-scoped; adapted runners are operation values | WSE-06 |
| D-43 | Workers.ProviderFromCommandRunnerFactory | Provider fallback constructor from command runner | build-time dependency | Providers | Provider closes with durable execution | WSE-07 |
| D-44 | OperatorSettings.EnsureBackendScope | Backend scope resolver/ensurer | build-time dependency | Operator Settings | Scope rollback belongs to process/session initialization | PSS-I01 |

## E. Invocation-local ExternalEffects

These values are projected by pkg/wire/runtime_inputs.go from the broader
process edge aggregate. They are operation values, not a service locator or
long-lived owner dependency.

| ID | Stable name | Current role | Classification | Target owner or deletion | Lifecycle / cleanup | Retirement packet |
| --- | --- | --- | --- | --- | --- | --- |
| E-01 | Clock | Explicit clock for this opening operation | operation value | Factory Runtime | Runtime scope owns the selected clock | FSE-05 |
| E-02 | ProviderOverride | Per-invocation provider override | operation value | Providers | Used by one opening; provider runtime closes normally | FSE-07 |
| E-03 | ModelPullMetricsRecorder | Model-pull metrics sink | operation value | Models | No ownership; recorder lifecycle remains process-scoped | WSE-03 |
| E-04 | InvocationMetricsRecorder | Invocation metrics sink | operation value | platform | No ownership; process metrics lifecycle | PSS-I01 |
| E-05 | ProviderCommandRunner | Provider command process effect | operation value | Providers | Commands are operation-scoped and wait/close at execution boundary | WSE-07 |
| E-06 | ScriptCommandRunner | Script command process effect | operation value | Automations | Commands are operation-scoped and wait/close at execution boundary | WSE-08 |
| E-07 | SubmissionRecorder | Work submission event sink | operation value | Recordings | Recording lifecycle owns flush/close | FSE-06 |
| E-08 | DispatchRecorder | Dispatch event sink | operation value | Recordings | Recording lifecycle owns flush/close | FSE-06 |
| E-09 | RuntimeHostObserver | Runtime-host readiness observer | operation value | initializer | Process opening owns callback registration and completion | PSS-I01 |
| E-10 | FactoryVisualizationSink | Factory visualization event sink | operation value | Factory Visualization | Visualization sink owns its subscription lifecycle | FSE-08 |
| E-11 | FactoryVisualizationRootObserver | Root visualization attachment observer | operation value | Factory Visualization | Attachment is unwound with visualization runtime | FSE-08 |
| E-12 | HostedClock | Hosted-source clock | operation value | Automations | Hosted pollers own clock use for their runtime | WSE-08 |
| E-13 | HostedHTTPClient | Hosted-source HTTP effect | operation value | Automations | Request/client lifecycle belongs to hosted poller | WSE-08 |
| E-14 | HostedSecretResolver | Hosted-source secret effect | operation value | Automations | Resolver is used by hosted pollers; no opener ownership | WSE-08 |
| E-15 | HostedLinearEndpoint | Hosted-source endpoint configuration | operation value | Automations | Immutable per hosted opening; no cleanup | WSE-08 |

## F. Deferred construction and callback roles

These rows make the remaining role-shaped seam visible before it is
retired. A role may also appear as a D field when its current implementation
is stored in grouped Dependencies; the two inventories answer different
questions and are both reconciled explicitly.

| ID | Stable name | Current role | Classification | Target owner or deletion | Lifecycle / cleanup | Retirement packet |
| --- | --- | --- | --- | --- | --- | --- |
| F-01 | FactoryRuntimeAssembler | Assembles the Runtime graph from opening inputs | build-time dependency | Factory Runtime | Runtime owns assembled graph cleanup | FSE-05 |
| F-02 | WorkersRuntimeExecutorsFactory | Builds Runtime-facing Workers executors | build-time dependency | Factory Runtime | Runtime owns executor lifecycle | WSE-03 |
| F-03 | ProviderInvocationFactory | Builds provider invocation executors | build-time dependency | Factory Runtime | Invocation executor closes with invocation | FSE-03 |
| F-04 | WorkersMockCommandRunnerFactory | Builds mock command runners | build-time dependency | Factory Runtime | Mock runner lifecycle follows durable execution | WSE-07 |
| F-05 | ResolveClock | Resolves live or replay clock | build-time dependency | Factory Runtime | Selected clock belongs to opened scope | FSE-05 |
| F-06 | NewSessionLogger | Builds the per-session logger | build-time dependency | Factory Runtime | Logger follows session/process lifecycle | PSS-I01 |
| F-07 | FactoryDefinitionsFactory | Builds the Factory Definitions service after runtime completion | build-time dependency | Factory Definitions | Definitions service owns its close boundary | FSE-02 |
| F-08 | InitialFactorySnapshotFactory | Builds initial definition snapshot | build-time dependency | Factory Definitions | Snapshot artifact follows definition lifecycle | FSE-02 |
| F-09 | LoadFactory | Loads a factory source | build-time dependency | Factory Definitions | Loaded source closes with opening scope | FSE-02 |
| F-10 | NewLoadedFactory | Builds a loaded factory source | build-time dependency | Factory Definitions | Loaded source lifecycle follows runtime | FSE-02 |
| F-11 | DecodeReplayConfig | Decodes replay configuration | build-time dependency | Factory Definitions | Stateless operation | BTRC-P1 |
| F-12 | CaptureLoadedFactorySnapshot | Captures a loaded factory snapshot | build-time dependency | Factory Definitions | Snapshot artifact lifecycle is explicit | FSE-02 |
| F-13 | DurableExecutionFactory | Builds durable execution service | build-time dependency | Factory Sessions | Durable execution closes during runtime unwind | FSE-05 |
| F-14 | FactorySessionExecutionFactory | Builds session execution internals | build-time dependency | Factory Sessions | Execution internals own their close boundary | FSE-05 |
| F-15 | FactoryScaffoldInitializer | Initializes factory scaffold state | build-time dependency | Factory Sessions | Initialization rollback is owned by initializer | FSE-02 |
| F-16 | EditableFactoryValidator | Validates editable factory state | build-time dependency | Factory Sessions | Stateless operation | FSE-02 |
| F-17 | ProcessRuntimeFactory | Binds process runtime roles | build-time dependency | initializer | Initializer owns bind/unwind | PSS-I01 |
| F-18 | GenerateRuntimeInstanceID | Generates runtime instance identity | build-time dependency | Factory Sessions | Per-runtime value; no separate cleanup | FSE-01 |
| F-19 | ResolveHome | Resolves the process home directory | build-time dependency | initializer | Process-scoped resolution | PSS-I01 |
| F-20 | ProviderIdentities | Resolves provider identities | build-time dependency | Providers | Provider identity policy owns capability | FSE-07 |
| F-21 | WorkFactory | Builds Work from a runtime resolver | delete | delete: runtime-opening WorkFactory seam | Canonical Wire supplies one Work root | WSE-04 |
| F-22 | ContentMaterializer | Materializes Work content | build-time dependency | Work | Work owns materialized-content cleanup | WSE-04 |
| F-23 | AutomationFactory | Builds Automations service | build-time dependency | Automations | Automation service closes with runtime | WSE-08 |
| F-24 | HostedSourcesFactory | Builds hosted automation pollers | build-time dependency | Automations | Pollers close with Automations | WSE-08 |
| F-25 | ProjectionFactory | Builds Recordings projections | delete | delete: per-opening Recordings projection factory | Canonical root owns the shared projection capability | FSE-06 |
| F-26 | LifecycleFactory | Builds recording lifecycle | delete | delete: per-opening Recordings lifecycle factory | Opaque scope owner owns lifecycle state | FSE-06 |
| F-27 | RuntimeLedgerFactory | Builds runtime ledger | delete | delete: per-opening Recordings runtime-ledger factory | Canonical root opens private ledgers | FSE-06 |
| F-28 | RuntimeRecorderFactory | Builds runtime recorder | delete | delete: per-opening Recordings runtime-recorder factory | Scope finalization owns recorder cleanup | FSE-06 |
| F-29 | ReplayClockFactory | Builds replay clock | delete | delete: per-opening Recordings replay-clock factory | Canonical root exposes replay clock capability | FSE-06 |
| F-30 | ReplayExecutionFactory | Builds replay execution | delete | delete: per-opening Recordings replay-execution factory | Canonical root exposes replay execution capability | FSE-06 |
| F-31 | ReplayInputs | Loads replay inputs | delete | delete: runtime-opening replay-input field | Canonical root retains the replay-input capability | FSE-06 |
| F-32 | ExecutionFactory | Builds Workers execution runtime | build-time dependency | Workers | Workers cleanup closes runtime and session build | WSE-02 |
| F-33 | RuntimeFactory | Builds Workers runtime implementation | build-time dependency | Workers | Workers runtime cleanup | WSE-02 |
| F-34 | LocalRuntimeHooksFactory | Builds unused local-runtime hooks | delete | delete: unused runtime-opening bridge | No lifecycle; remove from composition | WSE-03 |
| F-35 | AdaptCommandRunner | Adapts process command runners into Workers ports | build-time dependency | Workers | Adapted runner is an operation value | WSE-06 |
| F-36 | ProviderFromCommandRunnerFactory | Builds provider fallback from a command runner | build-time dependency | Providers | Provider lifecycle follows durable execution | WSE-07 |
| F-37 | EnsureBackendScope | Establishes backend operator scope | build-time dependency | Operator Settings | Scope rollback follows process initialization | PSS-I01 |
| F-38 | ConductorInvocationWithProgressFactory | Wire-only legacy provider invocation constructor | delete | delete: compatibility invocation bridge | No runtime-opening ownership; remove alias and provider fallback | FSE-03 |

## L. Late completion, attachment, and binding operations

These operations are separate from construction inputs because they currently
cross ownership boundaries after several services have been assembled.

| ID | Stable name | Current role | Classification | Target owner or deletion | Lifecycle / cleanup | Retirement packet |
| --- | --- | --- | --- | --- | --- | --- |
| L-01 | RuntimeAssembly.Complete | Completes Runtime assembly and returns session/invocation roles | private owner state | Factory Sessions | Completion owns the resulting runtime scope | FSE-05 |
| L-02 | DefinitionActivationGateway type assertion | Extracts a Definitions activation gateway from completed runtime | delete | delete: private runtime-to-definitions extraction | Remove assertion by making activation an explicit Definitions capability | FSE-03 |
| L-03 | AttachFactoryDefinitionService | Attaches Definitions service to completed runtime | delete | delete: runtime attachment bridge | Definitions owns its service; no late attachment | FSE-03 |
| L-04 | RecordingLifecycleFactory plus BindRecordingLifecycle | Creates and attaches recording lifecycle after recorder creation | delete | delete: runtime-opening lifecycle binding bridge | Recordings binds and finalizes its opaque scope internally | FSE-06 |
| L-05 | ProcessRuntimeFactory.Bind | Binds process runtime to the opened session runtime | delete | delete: process lifecycle binding bridge | initializer owns process binding and unwind | PSS-I01 |
| L-06 | BindWorkerInvoker | Injects durable execution into the runtime root | delete | delete: runtime invoker callback injector | Runtime receives its invocation capability at construction | FSE-03 |
| L-07 | fanOutWorkerProgress | Injects worker progress publisher into runtime execution | delete | delete: progress callback injector | Runtime/Workers own progress channel at construction | WSE-03 |
| L-08 | InferenceProgressPublisherFactory | Obtains a session progress publisher from private runtime assembly | private owner state | Factory Sessions | Session owns publisher lifecycle | WSE-03 |
| L-09 | DispatchCompletionObserverFactory | Obtains dispatch completion observer from private runtime assembly | private owner state | Factory Sessions | Session owns observer lifecycle | FSE-06 |
| L-10 | FactoryVisualizationRootObserver callback | Attaches visualized runtime root after opening | private owner state | Factory Visualization | Visualization owns attachment and stop/drain cleanup | FSE-08 |
| L-11 | RuntimeHTTPServicesBound | Signals HTTP service-table binding after assembly | delete | delete: application binding callback | Wire owns HTTP service-table composition | PSS-I01 |
| L-12 | HistoricalReplayBound | Signals replay-only service-table binding | operation value | Recordings | Replay view is immutable and has no live cleanup | FSE-06 |
| L-13 | gateCompletionOnRuntimeHost | Gates application completion on runtime-host readiness callback | delete | delete: application opening callback gate | initializer owns readiness and completion ordering | PSS-I01 |

## Ownership invariants for the next packet

- Factory Sessions opens a runtime from explicit inputs; it does not become a
  generic factory, service locator, callback injector, or compatibility graph.
- Service-valued operation bags and exposed private runtime state are not
  target owners. Where a row is deleted, the replacement is a narrow
  constructor or operation capability owned by the destination service.
- Runtime-created resources unwind in reverse acquisition order and cleanup is
  idempotent. Replay does not acquire live collaborators or invoke live
  cleanup paths.
- The D, E, F, and L totals are a reviewer-visible reconciliation artifact.
  No production or test filesystem inventory checker is part of this packet.
- BTRC-P1 must start from this matrix and reconcile current main again before
  selecting one characterization seam for implementation.
