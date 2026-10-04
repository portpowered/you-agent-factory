package analyzers

import (
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
)

func registeredBagFixtureRegistry(mode ConstructionMode) ConstructionRegistry {
	const owner = "m/pkg/registeredbags"
	const child = "m/pkg/registeredbagchild"
	registry := ConstructionRegistry{
		CapabilitySets: []ConstructionCapabilitySet{{Name: "bags", OwnerTask: "T20", Mode: mode}},
		Types: []ConstructionType{
			{Symbol: ConstructionSymbol{ImportPath: owner, Name: "Service"}, CapabilitySet: "bags", Kind: ConstructionBehavior},
			{Symbol: ConstructionSymbol{ImportPath: owner, Name: "Classified"}, CapabilitySet: "bags", Kind: ConstructionDomain},
		},
	}
	for _, typ := range []struct {
		name string
		kind ConstructionKind
	}{
		{"Child", ConstructionBehavior}, {"Effect", ConstructionEffect}, {"Domain", ConstructionDomain},
		{"State", ConstructionState}, {"Resource", ConstructionResource},
	} {
		registry.Types = append(registry.Types, ConstructionType{
			Symbol: ConstructionSymbol{ImportPath: child, Name: typ.name}, CapabilitySet: "bags", Kind: typ.kind,
		})
	}
	for _, test := range []struct{ name, param string }{
		{"Record", owner + ".Record"}, {"Alias", owner + ".RecordAlias"}, {"Defined", owner + ".RecordDefined"},
		{"Pointer", "*" + owner + ".Record"}, {"Embedded", owner + ".Embedded"}, {"Nested", owner + ".Nested"},
		{"RecursiveBag", owner + ".RecursiveBag"}, {"Slice", "[]" + owner + ".Record"}, {"Array", "[2]" + owner + ".Record"},
		{"Map", "map[string]" + owner + ".Record"}, {"MapKey", "map[*" + owner + ".Record]string"},
		{"Channel", "chan " + owner + ".Record"}, {"Variadic", "[]" + owner + ".Record"},
		{"Imported", child + ".Bag"}, {"ImportedAlias", child + ".BagAlias"}, {"ImportedDefined", child + ".BagDefined"},
		{"DefinedCollaborator", owner + ".DefinedContainer"}, {"Container", owner + ".ChildContainer"}, {"Multiple", owner + ".Multiple"},
		{"Inline", "struct{dep *" + child + ".Child}"}, {"Effect", "struct{effect " + child + ".Effect}"},
		{"FieldMapKey", "struct{deps map[*" + child + ".Child]string}"}, {"FieldChannel", "struct{deps chan *" + child + ".Child}"},
		{"FieldArray", "struct{deps [2]*" + child + ".Child}"}, {"Grouped", owner + ".Record"}, {"Unnamed", owner + ".Record"},
		{"Domain", child + ".Domain"}, {"State", child + ".State"}, {"Resource", child + ".Resource"},
		{"DomainContainer", owner + ".DomainContainer"}, {"Classified", owner + ".Classified"}, {"Recursive", owner + ".Recursive"},
		{"Direct", "*" + child + ".Child"}, {"DirectAlias", "*" + owner + ".Alias"}, {"DirectDefined", "*" + child + ".DerivedAgain"},
		{"DirectSlice", child + ".Slice"}, {"Optional", owner + ".Record"},
	} {
		constructor := ConstructionConstructor{
			Symbol: ConstructionSymbol{ImportPath: owner, Name: "New" + test.name}, CapabilitySet: "bags",
			Results: []ConstructionSymbol{{ImportPath: owner, Name: "Service"}},
		}
		switch test.name {
		case "Optional":
		case "Grouped":
			constructor.RequiredParameters = []ConstructionParameter{{Index: 1, TypeExpr: test.param}, {Index: 2, TypeExpr: test.param}}
		default:
			constructor.RequiredParameters = []ConstructionParameter{{Index: 0, TypeExpr: test.param}}
		}
		registry.Constructors = append(registry.Constructors, constructor)
	}
	return registry
}

// Unit evidence observes diagnostics, report observations and imported facts
// against real Go types and controlled registry metadata. No process or binary
// is built; the fixture baseline helper requires serial tests.
func TestConstructionRegisteredDependencyBags(t *testing.T) {
	useFixtures(t)
	analyzer := registeredConstructionAnalyzer(registeredBagFixtureRegistry(ConstructionEnforce))
	run := analyzer.Run
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		report := pass.Report
		defer func() { pass.Report = report }()
		pass.Report = func(d analysis.Diagnostic) {
			if strings.Contains(d.Message, "secretInput") || strings.Contains(d.Message, "children") {
				t.Errorf("diagnostic discloses field contents: %s", d.Message)
			}
			report(d)
		}
		findings, err := run(pass)
		if err != nil {
			return nil, err
		}
		if pass.Pkg.Path() == "m/pkg/registeredbags" {
			reports, err := runRegisteredConstruction(pass, registeredBagFixtureRegistry(ConstructionReport))
			if err != nil {
				return nil, err
			}
			assertRegisteredBagObservations(t, findings.([]ConstructionFinding), reports.([]ConstructionFinding))
		}
		return findings, nil
	}
	analysistest.Run(t, analysistest.TestData(), analyzer, "m/pkg/registeredbagchild", "m/pkg/registeredbags")
}

func assertRegisteredBagObservations(t *testing.T, findings, reports []ConstructionFinding) {
	t.Helper()
	if len(findings) != 27 || len(reports) != len(findings) {
		t.Fatalf("enforced/report findings = %d/%d, want 27/27", len(findings), len(reports))
	}
	grouped := 0
	for i, finding := range findings {
		if finding.Rule != "required-dependency-bag" || finding.Mode != ConstructionEnforce || finding.CapabilitySet != "bags" ||
			finding.Caller != finding.Callee || finding.FilePath != "pkg/registeredbags/bags.go" || finding.Line == 0 {
			t.Fatalf("invalid bag observation: %#v", finding)
		}
		if finding.Caller.Name == "NewGrouped" {
			grouped++
		}
		finding.Mode = ConstructionReport
		if finding != reports[i] {
			t.Fatalf("report changed observation: %#v / %#v", finding, reports[i])
		}
	}
	if grouped != 2 {
		t.Fatalf("grouped observations = %d, want two parameter positions", grouped)
	}
}

func TestConstructionRegisteredDependencyBagBaseline(t *testing.T) {
	useFixtures(t,
		"required-dependency-bag|pkg/registeredbaglisted|m/pkg/registeredbaglisted.New->m/pkg/registeredbaglisted.New",
		"required-dependency-bag|pkg/registeredbaglisted|m/pkg/registeredbaglisted.Removed->m/pkg/registeredbaglisted.Removed",
	)
	registry := registeredBagFixtureRegistry(ConstructionEnforce)
	const owner = "m/pkg/registeredbaglisted"
	service := ConstructionSymbol{ImportPath: owner, Name: "Service"}
	registry.Types = append(registry.Types, ConstructionType{Symbol: service, CapabilitySet: "bags", Kind: ConstructionBehavior})
	registry.Constructors = []ConstructionConstructor{{
		Symbol: ConstructionSymbol{ImportPath: owner, Name: "New"}, CapabilitySet: "bags", Results: []ConstructionSymbol{service},
		RequiredParameters: []ConstructionParameter{{Index: 0, TypeExpr: "m/pkg/registeredbagchild.Bag"}},
	}}
	analysistest.Run(t, analysistest.TestData(), registeredConstructionAnalyzer(registry), owner)
}
