package analyzers

import (
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
)

func registeredGuardFixtureRegistry(mode ConstructionMode) ConstructionRegistry {
	const owner = "m/pkg/registeredguards"
	registry := ConstructionRegistry{
		CapabilitySets: []ConstructionCapabilitySet{{Name: "guards", OwnerTask: "T20", Mode: mode}},
		Types: []ConstructionType{
			{Symbol: ConstructionSymbol{ImportPath: owner, Name: "Other"}, CapabilitySet: "guards", Kind: ConstructionBehavior},
			{Symbol: ConstructionSymbol{ImportPath: owner, Name: "Service"}, CapabilitySet: "guards", Kind: ConstructionBehavior},
			{Symbol: ConstructionSymbol{ImportPath: owner, Name: "State"}, CapabilitySet: "guards", Kind: ConstructionState},
		},
	}
	for _, name := range []string{"Direct", "Reverse", "Alias", "Declared", "Grouped", "Shadow", "Optional", "Mutation", "AliasMutation", "Assertion", "AssertionValue", "AssertionAlias", "AssertionMutation", "AssertionObserve", "AssertionHelper", "Closure", "For", "BoolAssertion"} {
		result := "Other"
		if name == "Direct" {
			result = "Service"
		}
		registry.Constructors = append(registry.Constructors, ConstructionConstructor{
			Symbol: ConstructionSymbol{ImportPath: owner, Name: "New" + name}, CapabilitySet: "guards",
			Results:            []ConstructionSymbol{{ImportPath: owner, Name: result}},
			RequiredParameters: []ConstructionParameter{{Index: 0, TypeExpr: "any"}},
		})
	}
	registry.Constructors = append(registry.Constructors, ConstructionConstructor{
		Symbol: ConstructionSymbol{ImportPath: owner, Name: "NewState"}, CapabilitySet: "guards",
		Results: []ConstructionSymbol{{ImportPath: owner, Name: "State"}},
	})
	return registry
}

// Component-isolated static-check fixtures observe typed diagnostics and report
// findings. Shared fixture baseline globals require serial execution.
func TestConstructionRegisteredGuards(t *testing.T) {
	useFixtures(t)
	analyzer := registeredConstructionAnalyzer(registeredGuardFixtureRegistry(ConstructionEnforce))
	run := analyzer.Run
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		findings, err := run(pass)
		if err != nil {
			return nil, err
		}
		report, err := runRegisteredConstruction(pass, registeredGuardFixtureRegistry(ConstructionReport))
		if err != nil {
			return nil, err
		}
		actual, observations := findings.([]ConstructionFinding), report.([]ConstructionFinding)
		if len(actual) != 17 || len(observations) != len(actual) {
			t.Fatalf("enforce/report = %d/%d, want 17/17", len(actual), len(observations))
		}
		for i, finding := range actual {
			if finding.Mode != ConstructionEnforce || finding.CapabilitySet != "guards" || finding.Line == 0 || finding.FilePath != "pkg/registeredguards/guards.go" {
				t.Fatalf("invalid finding: %#v", finding)
			}
			finding.Mode = ConstructionReport
			if finding != observations[i] {
				t.Fatalf("report changed finding: %#v / %#v", finding, observations[i])
			}
		}
		return findings, nil
	}
	analysistest.Run(t, analysistest.TestData(), analyzer, "m/pkg/registeredguards")
}

func TestConstructionRegisteredGuardBaseline(t *testing.T) {
	const owner = "m/pkg/registeredguardlisted"
	useFixtures(t,
		"required-dependency-guard|pkg/registeredguardlisted|"+owner+".New->"+owner+".New",
		"required-dependency-assertion-guard|pkg/registeredguardlisted|"+owner+".New->"+owner+".New",
		"required-receiver-guard|pkg/registeredguardlisted|"+owner+".(Service).Check->"+owner+".New",
		"unresolved-required-dependency-guard|pkg/registeredguardlisted|"+owner+".(Service).Mutated->"+owner+".New",
		"required-dependency-guard|pkg/registeredguardlisted|"+owner+".Removed->"+owner+".New",
	)
	registry := registeredGuardFixtureRegistry(ConstructionEnforce)
	service := ConstructionSymbol{ImportPath: owner, Name: "Service"}
	registry.Types = []ConstructionType{{Symbol: service, CapabilitySet: "guards", Kind: ConstructionBehavior}}
	registry.Constructors = []ConstructionConstructor{{
		Symbol: ConstructionSymbol{ImportPath: owner, Name: "New"}, CapabilitySet: "guards", Results: []ConstructionSymbol{service},
		RequiredParameters: []ConstructionParameter{{Index: 0, TypeExpr: "any"}},
	}}
	analysistest.Run(t, analysistest.TestData(), registeredConstructionAnalyzer(registry), owner)
}
