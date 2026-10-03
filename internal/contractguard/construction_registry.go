package contractguard

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

type ConstructionAllowance struct {
	Caller      ConstructionSymbol
	Callee      ConstructionSymbol
	FilePath    string
	Kind        string
	OwnerTask   string
	Reason      string
	RemovalTask string
}

// ConstructionRegistry is checker metadata; it never constructs runtime values.
type ConstructionRegistry struct {
	CapabilitySets []ConstructionCapabilitySet
	Constructors   []ConstructionConstructor
	Types          []ConstructionType
	Allowances     []ConstructionAllowance
}

// RepositoryConstructionRegistry starts with a report-only, source-audited
// owner. Further owners and enforcement are separate rollout increments.
func RepositoryConstructionRegistry() ConstructionRegistry {
	const module = "github.com/portpowered/infinite-you"
	const owner = module + "/pkg/services/chat_sessions/internal/service"
	constructor := ConstructionSymbol{ImportPath: owner, Name: "New"}
	return ConstructionRegistry{
		CapabilitySets: []ConstructionCapabilitySet{{Name: "chat-target-catalog", OwnerTask: "T21", Mode: ConstructionReport}},
		Constructors: []ConstructionConstructor{{
			Symbol: constructor, CapabilitySet: "chat-target-catalog",
			RequiredParameters: []ConstructionParameter{
				{Index: 0, TypeExpr: module + "/pkg/services/operator_settings.Service"},
				{Index: 1, TypeExpr: module + "/pkg/services/factory_definitions.CatalogPathsService"},
				{Index: 2, TypeExpr: module + "/pkg/platform/logging.Logger"},
			},
			Results: []ConstructionSymbol{{ImportPath: owner, Name: "Service"}},
		}},
		Types: []ConstructionType{{Symbol: ConstructionSymbol{ImportPath: owner, Name: "Service"}, CapabilitySet: "chat-target-catalog", Kind: ConstructionBehavior}},
		Allowances: []ConstructionAllowance{{
			Caller: ConstructionSymbol{ImportPath: module + "/pkg/services/chat_sessions/wire", Name: "NewFactoryTargetCatalogService"},
			Callee: constructor, FilePath: "pkg/services/chat_sessions/wire/wire.go", Kind: "focused-provider",
			OwnerTask: "T21", Reason: "Focused owner provider constructs the catalog implementation directly.",
		}},
	}
}
