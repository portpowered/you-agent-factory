package analyzers

import (
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
)

func registeredHelperFixtureRegistry(mode ConstructionMode) ConstructionRegistry {
	const owner = "m/pkg/registeredhelpers"
	service := ConstructionSymbol{ImportPath: owner, Name: "Service"}
	return ConstructionRegistry{
		CapabilitySets: []ConstructionCapabilitySet{{Name: "helpers", OwnerTask: "T20", Mode: mode}},
		Types:          []ConstructionType{{Symbol: service, CapabilitySet: "helpers", Kind: ConstructionBehavior}},
		Constructors: []ConstructionConstructor{{Symbol: ConstructionSymbol{ImportPath: owner, Name: "New"},
			CapabilitySet: "helpers", Results: []ConstructionSymbol{service},
			RequiredParameters: []ConstructionParameter{{Index: 0, TypeExpr: "any"}}}},
	}
}

// Compiler-unit fixtures observe guard diagnostics and report identities, with
// no application assembly. Shared fixture baselines require serial execution.
func TestConstructionRegisteredHelpers(t *testing.T) {
	useFixtures(t)
	analyzer := registeredConstructionAnalyzer(registeredHelperFixtureRegistry(ConstructionEnforce))
	run := analyzer.Run
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		findings, err := run(pass)
		if err != nil {
			return nil, err
		}
		report, err := runRegisteredConstruction(pass, registeredHelperFixtureRegistry(ConstructionReport))
		if err != nil {
			return nil, err
		}
		actual, observations := findings.([]ConstructionFinding), report.([]ConstructionFinding)
		if len(actual) != 22 || len(observations) != len(actual) {
			t.Errorf("enforce/report = %d/%d, want 22/22", len(actual), len(observations))
		}
		for i, finding := range actual {
			if finding.Mode != ConstructionEnforce || finding.CapabilitySet != "helpers" || finding.Line == 0 || finding.Callee.Name != "New" {
				t.Errorf("invalid helper finding: %#v", finding)
			}
			finding.Mode = ConstructionReport
			if i < len(observations) && finding != observations[i] {
				t.Errorf("report changed helper finding: %#v / %#v", finding, observations[i])
			}
		}
		return findings, nil
	}
	analysistest.Run(t, analysistest.TestData(), analyzer, "m/pkg/registeredhelpers")
}

func TestConstructionRegisteredHelperBaseline(t *testing.T) {
	const owner = "m/pkg/registeredhelperlisted"
	useFixtures(t,
		"required-dependency-guard|pkg/registeredhelperlisted|"+owner+".check->"+owner+".New",
		"unresolved-required-dependency-guard|pkg/registeredhelperlisted|"+owner+".ambiguous->"+owner+".New",
		"required-dependency-guard|pkg/registeredhelperlisted|"+owner+".Removed->"+owner+".New",
	)
	registry := registeredHelperFixtureRegistry(ConstructionEnforce)
	registry.Types[0].Symbol.ImportPath = owner
	registry.Constructors[0].Symbol.ImportPath = owner
	registry.Constructors[0].Results[0].ImportPath = owner
	analysistest.Run(t, analysistest.TestData(), registeredConstructionAnalyzer(registry), owner)
}
