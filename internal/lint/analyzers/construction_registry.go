package analyzers

// ConstructionSymbol identifies a declaration, independently of import aliases.
type ConstructionSymbol struct {
	ImportPath string
	Receiver   string
	Name       string
}

func (s ConstructionSymbol) String() string {
	if s.Receiver != "" {
		return s.ImportPath + ".(" + s.Receiver + ")." + s.Name
	}
	return s.ImportPath + "." + s.Name
}

type ConstructionMode string

const (
	ConstructionReport  ConstructionMode = "report"
	ConstructionEnforce ConstructionMode = "enforce"
)

type ConstructionKind string

const (
	ConstructionBehavior ConstructionKind = "behavior"
	ConstructionEffect   ConstructionKind = "effect"
	ConstructionState    ConstructionKind = "scoped-state"
	ConstructionResource ConstructionKind = "resource"
	ConstructionDomain   ConstructionKind = "domain-data"
)

type ConstructionCapabilitySet struct {
	Name      string
	OwnerTask string
	Mode      ConstructionMode
}

// Index counts individual parameters, including grouped names. TypeExpr uses
// qualified named types and canonical structural Go syntax.
type ConstructionParameter struct {
	Index    int
	TypeExpr string
}

type ConstructionConstructor struct {
	Symbol             ConstructionSymbol
	CapabilitySet      string
	RequiredParameters []ConstructionParameter
	Results            []ConstructionSymbol
}

type ConstructionType struct {
	Symbol        ConstructionSymbol
	CapabilitySet string
	Kind          ConstructionKind
}

// ConstructionRegistry is checker metadata; it never constructs runtime values.
type ConstructionRegistry struct {
	CapabilitySets []ConstructionCapabilitySet
	Constructors   []ConstructionConstructor
	Types          []ConstructionType
}

// RepositoryConstructionRegistry supplies source-audited typed classifications.
// Classification coverage is distinct from constructor discovery.
func RepositoryConstructionRegistry() ConstructionRegistry {
	const module = "github.com/portpowered/infinite-you"
	const owner = module + "/pkg/services/chat_sessions/internal/service"
	const events = module + "/pkg/services/events"
	const store = events + "/internal/service"
	constructor := ConstructionSymbol{ImportPath: owner, Name: "New"}
	registry := ConstructionRegistry{
		CapabilitySets: []ConstructionCapabilitySet{
			{Name: "chat-target-catalog", OwnerTask: "T21", Mode: ConstructionEnforce},
			{Name: "events", OwnerTask: "T29", Mode: ConstructionEnforce},
			{Name: "repository", OwnerTask: "T29", Mode: ConstructionEnforce},
		},
		Constructors: []ConstructionConstructor{{
			Symbol: constructor, CapabilitySet: "chat-target-catalog",
			RequiredParameters: []ConstructionParameter{
				{Index: 0, TypeExpr: module + "/pkg/services/operator_settings.Service"},
				{Index: 1, TypeExpr: module + "/pkg/services/factory_definitions.CatalogPathsService"},
				{Index: 2, TypeExpr: module + "/pkg/platform/logging.Logger"},
			},
			Results: []ConstructionSymbol{{ImportPath: owner, Name: "Service"}},
		}, {
			Symbol: ConstructionSymbol{ImportPath: store, Name: "NewWithRetention"}, CapabilitySet: "events",
			RequiredParameters: []ConstructionParameter{{Index: 1, TypeExpr: module + "/pkg/platform/logging.Logger"}},
			Results:            []ConstructionSymbol{{ImportPath: store, Name: "Store"}},
		}, {
			Symbol: ConstructionSymbol{ImportPath: events + "/wire", Name: "NewService"}, CapabilitySet: "events",
			RequiredParameters: []ConstructionParameter{{Index: 0, TypeExpr: module + "/pkg/platform/logging.Logger"}},
			Results:            []ConstructionSymbol{{ImportPath: events, Name: "Service"}},
		}},
		Types: []ConstructionType{
			{Symbol: ConstructionSymbol{ImportPath: owner, Name: "Service"}, CapabilitySet: "chat-target-catalog", Kind: ConstructionBehavior},
			{Symbol: ConstructionSymbol{ImportPath: events, Name: "Service"}, CapabilitySet: "events", Kind: ConstructionBehavior},
			{Symbol: ConstructionSymbol{ImportPath: store, Name: "Store"}, CapabilitySet: "events", Kind: ConstructionBehavior},
			{Symbol: ConstructionSymbol{ImportPath: store, Name: "topicState"}, CapabilitySet: "events", Kind: ConstructionState},
		},
	}
	registry = registerProviderSessionsConstruction(registry)
	registry = registerWorkRootConstruction(registry)
	return registerRuntimeEnablementConstruction(registry)
}

// NewService stores completed inputs; SubmitFile alone validates its public
// reader argument after PR #3160. RuntimeResolver results are selected session
// resources, not fixed injected peers, and are deliberately not required types.
func registerWorkRootConstruction(registry ConstructionRegistry) ConstructionRegistry {
	const root = "github.com/portpowered/infinite-you/pkg/services/work"
	const private = root + "/internal"
	for _, symbol := range []ConstructionSymbol{
		{ImportPath: root, Name: "Service"},
		{ImportPath: root, Name: "FileSubmissionService"},
		{ImportPath: private, Name: "applicationService"},
	} {
		registry.Types = append(registry.Types, ConstructionType{
			Symbol: symbol, CapabilitySet: "repository", Kind: ConstructionBehavior,
		})
	}
	parameters := []ConstructionParameter{
		{Index: 0, TypeExpr: root + ".RuntimeResolver"},
		{Index: 1, TypeExpr: root + ".SubmittedFileReader"},
		{Index: 2, TypeExpr: root + ".SubmittedFilePathInspector"},
		{Index: 3, TypeExpr: root + ".ContentStagingService"},
		{Index: 4, TypeExpr: root + ".ContentMaterializer"},
		{Index: 5, TypeExpr: root + "/internal/services/state_access.Service"},
		{Index: 6, TypeExpr: root + ".RequestPreparationService"},
		{Index: 7, TypeExpr: root + ".InvocationInputPreparation"},
	}
	wireParameters := append([]ConstructionParameter(nil), parameters...)
	wireParameters[5].TypeExpr = root + "/wire.StateAccess"
	registry.Constructors = append(registry.Constructors, ConstructionConstructor{
		Symbol:        ConstructionSymbol{ImportPath: private, Name: "NewService"},
		CapabilitySet: "repository", RequiredParameters: parameters,
		Results: []ConstructionSymbol{{ImportPath: root, Name: "FileSubmissionService"}},
	}, ConstructionConstructor{
		Symbol:        ConstructionSymbol{ImportPath: root + "/wire", Name: "NewRuntimeService"},
		CapabilitySet: "repository", RequiredParameters: wireParameters,
		Results: []ConstructionSymbol{{ImportPath: root, Name: "Service"}},
	})
	return registry
}

// PR #3162 injects reusable enablement into per-runtime termination state.
// Net absence, snapshot/marking/resource facts and empty-mode defaults remain
// domain checks; only the selected logger, time function and evaluator are fixed.
func registerRuntimeEnablementConstruction(registry ConstructionRegistry) ConstructionRegistry {
	const root = "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	const scheduler = root + "/internal/services/orchestration/scheduler"
	const subsystems = root + "/internal/services/orchestration/subsystems"
	for _, name := range []string{"Enablement", "EnablementEvaluator"} {
		registry.Types = append(registry.Types, ConstructionType{
			Symbol:        ConstructionSymbol{ImportPath: scheduler, Name: name},
			CapabilitySet: "repository", Kind: ConstructionBehavior,
		})
	}
	termination := ConstructionSymbol{ImportPath: subsystems, Name: "TerminationCheckSubsystem"}
	registry.Types = append(registry.Types, ConstructionType{
		Symbol: termination, CapabilitySet: "repository", Kind: ConstructionState,
	})
	registry.Constructors = append(registry.Constructors, ConstructionConstructor{
		Symbol:        ConstructionSymbol{ImportPath: scheduler, Name: "NewEnablementEvaluator"},
		CapabilitySet: "repository",
		Results:       []ConstructionSymbol{{ImportPath: scheduler, Name: "EnablementEvaluator"}},
	}, ConstructionConstructor{
		Symbol:        ConstructionSymbol{ImportPath: subsystems, Name: "NewTerminationCheckWithRuntime"},
		CapabilitySet: "repository", Results: []ConstructionSymbol{termination},
		RequiredParameters: []ConstructionParameter{
			{Index: 1, TypeExpr: "github.com/portpowered/infinite-you/pkg/platform/logging.Logger"},
			{Index: 4, TypeExpr: "func() time.Time"},
			{Index: 5, TypeExpr: scheduler + ".Enablement"},
		},
	})
	return registry
}

// The captured-only owner and its HTTP roles were corrected in PR #3132.
// Remaining repository classifications must not be enabled over known debt.
func registerProviderSessionsConstruction(registry ConstructionRegistry) ConstructionRegistry {
	const root = "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	const reader = "github.com/portpowered/infinite-you/pkg/services/recordings.WorkerCapturedActivityReader"
	service := ConstructionSymbol{ImportPath: root, Name: "Service"}
	adapter := ConstructionSymbol{ImportPath: root + "/transports/http", Name: "Adapter"}
	handler := ConstructionSymbol{ImportPath: root + "/transports/http", Name: "Handler"}
	for _, symbol := range []ConstructionSymbol{
		service, adapter, handler,
		{ImportPath: root + "/internal/service", Name: "inspectionService"},
	} {
		registry.Types = append(registry.Types, ConstructionType{
			Symbol: symbol, CapabilitySet: "repository", Kind: ConstructionBehavior,
		})
	}
	for _, symbol := range []ConstructionSymbol{
		{ImportPath: root + "/internal/service", Name: "New"},
		{ImportPath: root + "/wire", Name: "NewService"},
	} {
		registry.Constructors = append(registry.Constructors, ConstructionConstructor{
			Symbol: symbol, CapabilitySet: "repository", Results: []ConstructionSymbol{service},
			RequiredParameters: []ConstructionParameter{{Index: 0, TypeExpr: reader}},
		})
	}
	registry.Constructors = append(registry.Constructors, ConstructionConstructor{
		Symbol:        ConstructionSymbol{ImportPath: adapter.ImportPath, Name: "NewAdapter"},
		CapabilitySet: "repository", Results: []ConstructionSymbol{adapter},
		RequiredParameters: []ConstructionParameter{{Index: 0, TypeExpr: root + ".Service"}},
	}, ConstructionConstructor{
		Symbol:        ConstructionSymbol{ImportPath: handler.ImportPath, Name: "NewHandler"},
		CapabilitySet: "repository", Results: []ConstructionSymbol{handler},
		RequiredParameters: []ConstructionParameter{
			{Index: 0, TypeExpr: "*" + adapter.ImportPath + ".Adapter"},
			{Index: 1, TypeExpr: "*go.uber.org/zap.Logger"},
		},
	})
	return registry
}
