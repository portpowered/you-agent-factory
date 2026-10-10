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
	return ConstructionRegistry{
		CapabilitySets: []ConstructionCapabilitySet{
			{Name: "chat-target-catalog", OwnerTask: "T21", Mode: ConstructionEnforce},
			{Name: "events", OwnerTask: "T29", Mode: ConstructionEnforce},
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
}
