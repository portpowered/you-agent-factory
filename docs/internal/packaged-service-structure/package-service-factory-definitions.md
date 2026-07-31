## Recommended convergence

Factory Definitions should become one deep service with seven private capability modules:

1. Catalog
2. Authoring layout
3. Compilation
4. Validation
5. Snapshot portability
6. Distribution
7. Invocation policy

The current root [`Service`](C:/Users/andre/work/portos/infinite-you/pkg/services/factory_definitions/service_contract.go:15) is directionally correct for the first six. It is not yet the final contract because:

- Five legacy lifecycle/session methods precede the request/result slices.
- Invocation policy is exposed through eight parallel interfaces.
- The root contains 31 named interfaces, including filesystem, clock, validation, loading, persistence, and Sessions callback ports.
- `definition/` and `service/` remain public implementation shims.
- The seven private subservices still have noncanonical sibling implementation directories.
- Mapping and Wire still construct individual capabilities instead of consuming the root consistently.

## 1. Converged root interface

A practical convergence target is:

```go
package factorydefinitions

type Service interface {
	// Catalog and discovery.
	ListEffectiveFactories(
		context.Context,
		ListEffectiveFactoriesRequest,
	) (ListEffectiveFactoriesResult, error)

	ListNamedFactories(
		context.Context,
		ListNamedFactoriesRequest,
	) (ListNamedFactoriesResult, error)

	GetNamedFactory(
		context.Context,
		GetNamedFactoryRequest,
	) (GetNamedFactoryResult, error)

	ResolveNamedFactory(
		context.Context,
		ResolveNamedFactoryRequest,
	) (ResolveNamedFactoryResult, error)

	DeleteNamedFactory(
		context.Context,
		DeleteNamedFactoryRequest,
	) (DeleteNamedFactoryResult, error)

	GetCurrentFactoryPointer(
		context.Context,
		GetCurrentFactoryPointerRequest,
	) (GetCurrentFactoryPointerResult, error)

	SetCurrentFactoryPointer(
		context.Context,
		SetCurrentFactoryPointerRequest,
	) (SetCurrentFactoryPointerResult, error)

	// Authoring and persistence.
	PrepareFactoryLayout(
		context.Context,
		PrepareFactoryLayoutRequest,
	) (PrepareFactoryLayoutResult, error)

	FlattenFactoryLayout(
		context.Context,
		FlattenFactoryLayoutRequest,
	) (FlattenFactoryLayoutResult, error)

	ExpandFactoryLayout(
		context.Context,
		ExpandFactoryLayoutRequest,
	) (ExpandFactoryLayoutResult, error)

	CreateNamedFactory(
		context.Context,
		CreateNamedFactoryRequest,
	) (CreateNamedFactoryResult, error)

	ReplaceNamedFactory(
		context.Context,
		ReplaceNamedFactoryRequest,
	) (ReplaceNamedFactoryResult, error)

	// Compile and validate.
	CompileEffectiveFactorySource(
		context.Context,
		CompileEffectiveFactorySourceRequest,
	) (CompileEffectiveFactorySourceResult, error)

	ValidateStructuralFactoryDefinition(
		context.Context,
		ValidateStructuralFactoryDefinitionRequest,
	) (ValidateStructuralFactoryDefinitionResult, error)

	ValidateEffectiveFactoryDefinition(
		context.Context,
		ValidateEffectiveFactoryDefinitionRequest,
	) (ValidateEffectiveFactoryDefinitionResult, error)

	// Portable snapshots.
	CaptureFactorySnapshot(
		context.Context,
		CaptureFactorySnapshotRequest,
	) (CaptureFactorySnapshotResult, error)

	PrepareFactorySnapshotImport(
		context.Context,
		PrepareFactorySnapshotImportRequest,
	) (PrepareFactorySnapshotImportResult, error)

	MaterializeFactorySnapshot(
		context.Context,
		MaterializeFactorySnapshotRequest,
	) (MaterializeFactorySnapshotResult, error)

	// Built-in distribution.
	ListBuiltInPackagedFactories(
		context.Context,
		ListBuiltInPackagedFactoriesRequest,
	) (ListBuiltInPackagedFactoriesResult, error)

	ResolveBuiltInPackagedFactory(
		context.Context,
		ResolveBuiltInPackagedFactoryRequest,
	) (ResolveBuiltInPackagedFactoryResult, error)

	InstallPackagedFactory(
		context.Context,
		InstallPackagedFactoryRequest,
	) (InstallPackagedFactoryResult, error)

	CreateFactoryScaffold(
		context.Context,
		CreateFactoryScaffoldRequest,
	) (CreateFactoryScaffoldResult, error)

	// Invocation-time projection of authored policy.
	ResolveInvocationDefinition(
		context.Context,
		ResolveInvocationDefinitionRequest,
	) (ResolveInvocationDefinitionResult, error)
}
```

The first 22 methods already approximately exist on the root interface in grouped slices ([service_contract.go](C:/Users/andre/work/portos/infinite-you/pkg/services/factory_definitions/service_contract.go:22)). The new conceptual operation is `ResolveInvocationDefinition`.

### Why one invocation operation

Today the service publishes eight separate policy interfaces:

- `DecisionEnvelopeService`
- `InvocationInterpolationService`
- `InvocationOutputShapingService`
- `InvocationWorkTypeService`
- `QuorumPolicyService`
- `WorkPropagationPolicyService`
- `WorkstationExecutionPolicyService`
- `TTSObservabilityService`

These are defined in files such as [decision_envelope_contract.go](C:/Users/andre/work/portos/infinite-you/pkg/services/factory_definitions/decision_envelope_contract.go:39) and independently reconstructed in `pkg/wire`.

That arrangement violates the singular service-root model and makes Runtime, Workers, Sessions, and Automations depend on implementation-shaped slices.

Instead, `ResolveInvocationDefinition` should return an immutable, detached projection such as:

```go
type ResolveInvocationDefinitionRequest struct {
	Definition        EffectiveFactorySource
	Arguments         InvocationArguments
	ResolvedFileInput map[string][]byte
}

type ResolveInvocationDefinitionResult struct {
	Factory      FactoryConfig
	DefaultWork  string
	Workstations map[string]ResolvedWorkstationPolicy
	FactoryKind  FactoryBehaviorKind
}

type ResolvedWorkstationPolicy struct {
	ExecutionTimeout time.Duration
	PropagationMode  WorkPropagationMode
	OutputMode       InvocationOutputMode
	DecisionMode     DecisionEnvelopeMode
}
```

The precise fields need caller-by-caller characterization, but the ownership principle is:

- Factory Definitions interprets authored configuration.
- Workers executes workers.
- Runtime applies transition/output mechanics.
- Work owns Work content and result construction.
- Sessions owns invocation lifecycle and observations.

Factory Definitions should not return `workerexecution.WorkResult`, inspect Factory world state, or implement waiting/observability algorithms. Those pieces of the current policy interfaces belong to their execution owners.

## 2. Legacy root methods to remove

These current methods should not survive as-is:

```go
ActivateNamedFactory(context.Context, string) error
Save(context.Context, string, SaveMode, EditableFactory) (EditableFactory, error)
GetCurrentNamedFactory(context.Context) (*FactorySnapshot, error)
GetCurrentFactoryForSession(context.Context, string) (EditableFactory, error)
CurrentFactoryDefinitionVersionAtRoot(string, string) (FactoryVersion, error)
```

Recommended ownership:

| Current method | Final owner |
|---|---|
| `ActivateNamedFactory` | Factory Sessions command; Definitions resolves the named definition |
| `Save(... sessionID ...)` | Sessions coordinates session state; Definitions performs validate/create/replace |
| `GetCurrentFactoryForSession` | Factory Sessions |
| `GetCurrentNamedFactory` | Replace with catalog pointer + `GetNamedFactory` |
| `CurrentFactoryDefinitionVersionAtRoot` | Catalog result/version field or a request/result catalog operation |

This completes the planned activation-cycle cut described by `CUT-DEF-SES` ([planner-wave-def-migration-repair-20260728.json](C:/Users/andre/work/portos/infinite-you/docs/temp/projects/packaged-service-structure/batches/planner-wave-def-migration-repair-20260728.json:182)).

## 3. Root contract files

At convergence, the Factory Definitions root should contain:

- Exactly one named interface: `Service`.
- Plain request/result structs.
- Definition-owned value types:
  - `FactoryConfig`
  - `FactorySnapshot`
  - authored/effective definition values
  - catalog identities
  - validation findings
  - portability facts
  - distribution facts
  - resolved invocation-definition policy
- Typed errors and error-detail structs.
- Constants/enums genuinely owned by Factory Definitions.

The root should not contain:

- Filesystem interfaces.
- Clocks.
- HTTP/process adapters.
- persistence interfaces.
- loader interfaces.
- validator interfaces.
- Sessions callback interfaces.
- Runtime/Petri contracts.
- Workers or Work result construction.
- constructors or exported helper functions.
- eight policy service interfaces.

Existing pure public helpers should either:

- Become behavior on `Service`.
- Become unexported implementation helpers.
- Move to the domain that owns the returned type.
- Remain exported only as plain data conversion when mechanically required by a public contract—and ideally be removed by the final structure gate.

## 4. Final Factory Definitions tree

```text
pkg/services/factory_definitions/
├── service.go
├── catalog_contracts.go
├── authoring_contracts.go
├── compilation_contracts.go
├── validation_contracts.go
├── snapshot_contracts.go
├── distribution_contracts.go
├── invocation_contracts.go
├── definition_types.go
├── errors.go
│
├── internal/
│   ├── service.go
│   ├── lifecycle/
│   │   ├── activate.go
│   │   ├── save.go
│   │   └── version.go
│   ├── testutil/
│   └── services/
│       ├── catalog/
│       │   ├── service.go
│       │   ├── internal/
│       │   │   ├── service/
│       │   │   ├── namedpaths/
│       │   │   ├── namedfactories/
│       │   │   ├── persistence/
│       │   │   └── resource/
│       │   └── wire/
│       │
│       ├── authoring_layout/
│       │   ├── service.go
│       │   ├── internal/
│       │   │   ├── service/
│       │   │   ├── codec/
│       │   │   ├── prepare/
│       │   │   ├── flatten/
│       │   │   ├── expand/
│       │   │   └── persist/
│       │   └── wire/
│       │
│       ├── compilation/
│       │   ├── service.go
│       │   ├── internal/
│       │   │   ├── service/
│       │   │   ├── canonical/
│       │   │   ├── loading/
│       │   │   ├── loadedsource/
│       │   │   └── runtimeconfig/
│       │   └── wire/
│       │
│       ├── validation/
│       │   ├── service.go
│       │   ├── internal/
│       │   │   ├── service/
│       │   │   ├── authoredmodel/
│       │   │   ├── structural/
│       │   │   ├── topology/
│       │   │   ├── requiredtools/
│       │   │   └── orchestrator/
│       │   └── wire/
│       │
│       ├── snapshots_portability/
│       │   ├── service.go
│       │   ├── internal/
│       │   │   ├── service/
│       │   │   ├── capture/
│       │   │   ├── editable/
│       │   │   ├── prepare/
│       │   │   ├── materialize/
│       │   │   ├── portableconfig/
│       │   │   └── replayconfig/
│       │   └── wire/
│       │
│       ├── distribution/
│       │   ├── service.go
│       │   ├── internal/
│       │   │   ├── service/
│       │   │   ├── packagedcatalog/
│       │   │   ├── packagedinstallation/
│       │   │   ├── packageassets/
│       │   │   ├── promptassets/
│       │   │   ├── scaffold/
│       │   │   └── templates/
│       │   └── wire/
│       │
│       └── invocation_policy/
│           ├── service.go
│           ├── internal/
│           │   ├── service/
│           │   ├── interpolation/
│           │   ├── worktype/
│           │   ├── execution/
│           │   ├── propagation/
│           │   ├── output/
│           │   ├── decision/
│           │   └── quorum/
│           └── wire/
│
├── transports/
│   ├── cli/
│   ├── http/
│   └── mcp/
│
└── wire/
    └── wire.go
```

Every subservice root has exactly:

- `service.go`
- `internal/`
- `wire/`

No implementation siblings sit directly beside `internal` and `wire`.

## 5. Existing-package destination map

### Root-level packages

| Existing path | Destination |
|---|---|
| `factory_definitions/definition` | Fold into `internal/lifecycle` and root composition; delete shim |
| `factory_definitions/service` | Fold into `internal/service.go`; delete package |
| `factory_definitions/clonetests` | Move tests beside owning internal/root contracts |
| `factory_definitions/systeminitializationtests` | Move behavior tests to System Initialization or service-local integration tests |
| `factory_definitions/internal/testcomposition` | `internal/testutil` or package-local `_test.go` helpers |
| `factory_definitions/internal/contracts` | Split Definition-owned data into root contracts; move foreign vocabulary to owning services; delete barrel |
| `factory_definitions/internal/lifecycle` | Retain as private root implementation |
| `factory_definitions/transports/*` | Retain, after domain policy is removed |
| `factory_definitions/wire` | Retain as sole public constructor |

Deleting the contracts mega-barrel and parallel injection paths is already explicitly planned ([planner-wave-def-migration-repair-20260728.json](C:/Users/andre/work/portos/infinite-you/docs/temp/projects/packaged-service-structure/batches/planner-wave-def-migration-repair-20260728.json:153)).

### Catalog

| Existing | Destination |
|---|---|
| `catalog/namedpaths` | `catalog/internal/namedpaths` |
| `catalog/namedfactories` | `catalog/internal/namedfactories` |
| `catalog/persistence` | `catalog/internal/persistence` |
| `catalog/resource` | `catalog/internal/resource` |
| `catalog/internal/service` | Retain |
| `catalog/wire` | Retain |

This exact fold is already captured by `CLN-DEF-FOLD-CATALOG` ([residual packet](C:/Users/andre/work/portos/infinite-you/docs/temp/projects/packaged-service-structure/batches/planner-wave-def-residual-folds-20260728.json:69)).

### Authoring layout

| Existing | Destination |
|---|---|
| `authoring_layout/authoredlayout` | `authoring_layout/internal/codec` or `internal/authoredlayout` |
| `authoring_layout/prepare` | `authoring_layout/internal/prepare` |
| `authoring_layout/flatten` | `authoring_layout/internal/flatten` |
| `authoring_layout/expand` | `authoring_layout/internal/expand` |
| `authoring_layout/persist` | `authoring_layout/internal/persist` |
| `authoring_layout/internal/service` | Retain |
| `authoring_layout/wire` | Retain |

The OpenAPI-free canonical/authored codec belongs here or in Compilation—not in shared transport mapping.

### Compilation

| Existing | Destination |
|---|---|
| `compilation/canonical` | `compilation/internal/canonical` |
| `compilation/loading` | `compilation/internal/loading` |
| `compilation/loadedsource` | `compilation/internal/loadedsource` |
| `compilation/runtimeconfig` | `compilation/internal/runtimeconfig` |
| `compilation/runtimetests` | Tests beside `internal` implementation |
| `compilation/internal/service` | Retain |
| `compilation/wire` | Retain |

### Validation

| Existing | Destination |
|---|---|
| `validation/authoredmodel` | `validation/internal/authoredmodel` |
| `authoredmodel/namevalue` | Retain beneath `internal/authoredmodel` |
| `authoredmodel/taxonomy` | Retain beneath `internal/authoredmodel` |
| `authoredmodel/workers` | Retain beneath `internal/authoredmodel` |
| `validation/impl` | Merge into `validation/internal/service` |
| `validation/internal/structural` | Retain |
| `validation/internal/topology` | Retain |
| `validation/internal/requiredtools` | Retain |
| `validation/internal/orchestrator` | Retain |
| `validation/wire` | Retain |

Definitions may call a Runtime-root semantic validation edge, but neither the root result nor validation contracts should expose Petri types.

### Snapshot portability

| Existing | Destination |
|---|---|
| `snapshots_portability/capture` | `snapshots_portability/internal/capture` |
| `snapshots_portability/editable` | `snapshots_portability/internal/editable` |
| `snapshots_portability/prepare` | `snapshots_portability/internal/prepare` |
| `snapshots_portability/materialize` | `snapshots_portability/internal/materialize` |
| `snapshots_portability/portableconfig` | `snapshots_portability/internal/portableconfig` |
| `snapshots_portability/replayconfig` | `snapshots_portability/internal/replayconfig` |
| `snapshots_portability/internal/service` | Retain |
| `snapshots_portability/wire` | Retain |

### Distribution

| Existing | Destination |
|---|---|
| `distribution/packagedcatalog` | `distribution/internal/packagedcatalog` |
| `distribution/packagedinstallation` | `distribution/internal/packagedinstallation` |
| `distribution/packageassets` | `distribution/internal/packageassets` |
| `distribution/promptassets` | `distribution/internal/promptassets` |
| `distribution/scaffoldfacts` | `distribution/internal/scaffold` |
| `distribution/goal` | `distribution/internal/templates` if asset-only; otherwise invocation policy |
| `distribution/review` | Same decision |
| `distribution/subagent` | Same decision |
| `distribution/tts` | Same decision |
| `distribution/internal/service` | Retain |
| `distribution/wire` | Retain |
| `wire/defaultscaffold` | Fold into distribution construction/scaffold implementation |

The rule here is: packaging and template assets belong to Distribution; interpreting their runtime behavior belongs to Invocation Policy or the execution owner.

### Invocation policy

| Existing | Destination |
|---|---|
| `decisionenvelope` | `invocation_policy/internal/decision` |
| `invocationinterpolation` | `invocation_policy/internal/interpolation` |
| `invocationoutput` | `invocation_policy/internal/output` |
| `invocationworktype` | `invocation_policy/internal/worktype` |
| `quorumpolicy` | `invocation_policy/internal/quorum` |
| `workpropagation` | `invocation_policy/internal/propagation` |
| `workstationexecution` | `invocation_policy/internal/execution` |
| `ttsobservability` | Split: authored TTS classification stays; session/model wait logic moves out |
| `invocation_policy/internal/service` | Rewrite to return one resolved invocation projection |
| `invocation_policy/wire` | Retain |

The project already plans this cluster fold ([planner-wave-def-residual-folds-20260728.json](C:/Users/andre/work/portos/infinite-you/docs/temp/projects/packaged-service-structure/batches/planner-wave-def-residual-folds-20260728.json:409)), but it currently preserves parallel root policy contracts. I would tighten that outcome to remove those interfaces entirely.

## 6. Wire convergence

Final construction should be:

```go
func NewService(deps Dependencies) (factorydefinitions.Service, error)
```

`factory_definitions/wire` should:

- Construct each private subservice exactly once.
- Construct the private root implementation.
- Return only `factorydefinitions.Service`.
- Never return policy ports independently.
- Never expose private subservice interfaces to `pkg/wire`.
- Accept external effects through explicit construction dependencies.
- Avoid `SessionHost`; use a narrow function-based activation edge until the Sessions cycle is fully removed.

The current independent providers in `pkg/wire/factory_definition_services.go` should disappear. Runtime, Sessions, Workers, and Automations should receive either:

- The root `factorydefinitions.Service`, or
- An immutable resolved policy/definition produced by it.

They should not receive eight independently constructed Factory Definitions services.

## 7. Remaining migration sequence

Recommended order:

1. Seal the intended root contract.

   Add the final policy projection operation and characterize every current policy consumer.

2. Cut the Sessions lifecycle ownership.

   Move activation/current-session behavior to Factory Sessions and remove `SessionHost` from the Definitions root ([service_contract.go](C:/Users/andre/work/portos/infinite-you/pkg/services/factory_definitions/service_contract.go:651)).

3. Fold root composition.

   Merge `definition/` and `service/` into `internal/`; retarget `factory_definitions/wire`.

4. Fold the seven subservice implementation trees.

   Move all public sibling packages beneath each subservice’s `internal/`.

5. Remove the contracts barrel.

   Move foreign event, world-state, Work, Workers, Provider, and Runtime vocabulary to their owners.

6. Move codec and normalization policy out of transports.

   Authoring/Compilation should own canonical Factory decoding and normalization. Transports should only map generated API fields.

7. Retarget peer consumers.

   Remove individual policy service dependencies from Runtime, Sessions, Workers, and Automations.

8. Delete shims and lower baselines.

   Delete `definition/`, `service/`, emptied siblings, stale aliases, and parallel constructors.

9. Verify behavior and structure.

   At minimum:

   - Factory Definitions focused unit/integration tests.
   - Catalog precedence and failure isolation.
   - Authored layout round trips and atomic failure preservation.
   - Compile equivalence.
   - Structural/effective validation failures.
   - Snapshot detached round trips.
   - Packaged install/scaffold equivalence.
   - Invocation policy parity across Runtime, Sessions, Workers, and Automations.
   - `make pkg-structure`
   - `make package-target-manifest-check`
   - `make ownership-boundary-check`
   - `make verify-fast`
   - `make lint`

10. Continue the implementation/review cycle through terminal green CI, resolved blocking feedback and conflicts, and actual PR merge.

The most important architectural correction is this: the seven internal subservices are the right decomposition, but they must be private implementation details. The only peer-visible behavioral object should be `factorydefinitions.Service`; session lifecycle and execution mechanics must leave the service rather than being retained as “policy contracts.”