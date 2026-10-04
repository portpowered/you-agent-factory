package analyzers

import (
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
)

func registeredGetterFixtureRegistry(mode ConstructionMode) ConstructionRegistry {
	const owner = "m/pkg/registeredgetters"
	registry := ConstructionRegistry{CapabilitySets: []ConstructionCapabilitySet{{Name: "getters", OwnerTask: "T20", Mode: mode}}}
	for _, name := range []string{"Port", "Service", "Domain", "State"} {
		kind := ConstructionBehavior
		if name == "Domain" {
			kind = ConstructionDomain
		}
		if name == "State" {
			kind = ConstructionState
		}
		registry.Types = append(registry.Types, ConstructionType{Symbol: ConstructionSymbol{ImportPath: owner, Name: name}, CapabilitySet: "getters", Kind: kind})
	}
	for _, entry := range []struct{ name, result string }{{"New", "Service"}, {"NewState", "State"}} {
		required := []ConstructionParameter{{Index: 0, TypeExpr: owner + ".Port"}}
		if entry.name == "New" {
			required = append(required, ConstructionParameter{Index: 1, TypeExpr: owner + ".Derived"}, ConstructionParameter{Index: 2, TypeExpr: owner + ".Domain"})
		}
		registry.Constructors = append(registry.Constructors, ConstructionConstructor{Symbol: ConstructionSymbol{ImportPath: owner, Name: entry.name}, CapabilitySet: "getters", Results: []ConstructionSymbol{{ImportPath: owner, Name: entry.result}}, RequiredParameters: required})
	}
	return registry
}

// Static component evidence observes typed diagnostics and imported object facts.
// Shared baseline fixture state requires serial execution.
func TestConstructionRegisteredGetters(t *testing.T) {
	useFixtures(t)
	analyzer := registeredConstructionAnalyzer(registeredGetterFixtureRegistry(ConstructionEnforce))
	run := analyzer.Run
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		findings, err := run(pass)
		if err != nil {
			return nil, err
		}
		if pass.Pkg.Path() != "m/pkg/registeredgetterconsumer" {
			return findings, nil
		}
		report, err := runRegisteredConstruction(pass, registeredGetterFixtureRegistry(ConstructionReport))
		if err != nil {
			return nil, err
		}
		actual, observations := findings.([]ConstructionFinding), report.([]ConstructionFinding)
		if len(actual) != 24 || len(observations) != len(actual) {
			t.Errorf("enforce/report=%d/%d, want 24/24", len(actual), len(observations))
		}
		for i, finding := range actual {
			if finding.Callee.ImportPath != "m/pkg/registeredgetters" || finding.Callee.Receiver != "Service" || finding.Mode != ConstructionEnforce || finding.Line == 0 {
				t.Errorf("invalid finding: %#v", finding)
			}
			finding.Mode = ConstructionReport
			if i < len(observations) && finding != observations[i] {
				t.Errorf("report changed finding: %#v / %#v", finding, observations[i])
			}
		}
		return findings, nil
	}
	analysistest.Run(t, analysistest.TestData(), analyzer, "m/pkg/registeredgetters", "m/pkg/registeredgetterconsumer", "m/pkg/registeredgetterparity")
}

func TestConstructionRegisteredGetterBaseline(t *testing.T) {
	const owner = "m/pkg/registeredgetters"
	const caller = "m/pkg/registeredgetterlisted"
	useFixtures(t,
		"service-getter-locator|pkg/registeredgetterlisted|"+caller+".Direct->"+owner+".(Service).Lookup",
		"unresolved-service-getter-locator|pkg/registeredgetterlisted|"+caller+".Named->"+owner+".(Service).Named",
		"unresolved-service-getter-reference|pkg/registeredgetterlisted|"+caller+".Reference->"+owner+".(Service).Lookup",
		"service-getter-locator|pkg/registeredgetterlisted|"+caller+".Removed->"+owner+".(Service).Lookup")
	analysistest.Run(t, analysistest.TestData(), registeredConstructionAnalyzer(registeredGetterFixtureRegistry(ConstructionEnforce)), "m/pkg/registeredgetterlisted")
}
