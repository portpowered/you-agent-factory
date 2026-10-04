package analyzers

import (
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
)

func registeredStorageFixtureRegistry(mode ConstructionMode) ConstructionRegistry {
	const owner = "m/pkg/registeredstorage"
	registry := ConstructionRegistry{CapabilitySets: []ConstructionCapabilitySet{{Name: "storage", OwnerTask: "T20", Mode: mode}}}
	for _, name := range []string{"Keyed", "Positional", "Embedded", "Assigned", "Replaced", "Mutated", "Optional", "State", "Shadowed", "Asserted"} {
		kind := ConstructionBehavior
		if name == "State" {
			kind = ConstructionState
		}
		result := ConstructionSymbol{ImportPath: owner, Name: name}
		registry.Types = append(registry.Types, ConstructionType{Symbol: result, CapabilitySet: "storage", Kind: kind})
		param := "any"
		if name == "Embedded" {
			param = owner + ".Port"
		}
		registry.Constructors = append(registry.Constructors, ConstructionConstructor{
			Symbol: ConstructionSymbol{ImportPath: owner, Name: "New" + name}, CapabilitySet: "storage",
			Results: []ConstructionSymbol{result}, RequiredParameters: []ConstructionParameter{{Index: 0, TypeExpr: param}},
		})
	}
	return registry
}

// This is component-isolated static-check evidence, not a runtime topology test.
// Shared fixture baselines require serial execution.
func TestConstructionRegisteredStorage(t *testing.T) {
	useFixtures(t)
	analyzer := registeredConstructionAnalyzer(registeredStorageFixtureRegistry(ConstructionEnforce))
	run := analyzer.Run
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		findings, err := run(pass)
		if err != nil {
			return nil, err
		}
		report, err := runRegisteredConstruction(pass, registeredStorageFixtureRegistry(ConstructionReport))
		if err != nil {
			return nil, err
		}
		actual, observations := findings.([]ConstructionFinding), report.([]ConstructionFinding)
		if len(actual) != 14 || len(observations) != len(actual) {
			t.Errorf("enforce/report = %d/%d, want 14/14", len(actual), len(observations))
		}
		for i, finding := range actual {
			if finding.Mode != ConstructionEnforce || finding.CapabilitySet != "storage" || finding.Line == 0 || finding.FilePath != "pkg/registeredstorage/storage.go" {
				t.Errorf("invalid finding: %#v", finding)
			}
			finding.Mode = ConstructionReport
			if finding != observations[i] {
				t.Errorf("report changed finding: %#v / %#v", finding, observations[i])
			}
		}
		return findings, nil
	}
	analysistest.Run(t, analysistest.TestData(), analyzer, "m/pkg/registeredstorage")
}

func TestConstructionRegisteredStorageBaseline(t *testing.T) {
	const owner = "m/pkg/registeredstoragelisted"
	useFixtures(t,
		"required-dependency-guard|pkg/registeredstoragelisted|"+owner+".(Service).Guard->"+owner+".New",
		"required-dependency-assertion-guard|pkg/registeredstoragelisted|"+owner+".(Service).Assertion->"+owner+".New",
		"unresolved-required-dependency-guard|pkg/registeredstoragelisted|"+owner+".(Service).Mutated->"+owner+".New",
		"required-dependency-guard|pkg/registeredstoragelisted|"+owner+".Removed->"+owner+".New",
	)
	service := ConstructionSymbol{ImportPath: owner, Name: "Service"}
	registry := ConstructionRegistry{
		CapabilitySets: []ConstructionCapabilitySet{{Name: "storage", OwnerTask: "T20", Mode: ConstructionEnforce}},
		Types:          []ConstructionType{{Symbol: service, CapabilitySet: "storage", Kind: ConstructionBehavior}},
		Constructors: []ConstructionConstructor{{Symbol: ConstructionSymbol{ImportPath: owner, Name: "New"},
			CapabilitySet: "storage", Results: []ConstructionSymbol{service},
			RequiredParameters: []ConstructionParameter{{Index: 0, TypeExpr: "any"}}}},
	}
	analysistest.Run(t, analysistest.TestData(), registeredConstructionAnalyzer(registry), owner)
}
