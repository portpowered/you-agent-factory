package analyzers

import (
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
)

func registeredProviderFixtureRegistry(mode ConstructionMode, unit string, names []string) ConstructionRegistry {
	registry := registeredFixtureRegistry(mode)
	for _, name := range names {
		registry.Allowances = append(registry.Allowances, ConstructionAllowance{
			Caller: ConstructionSymbol{ImportPath: "m/" + unit, Name: name}, Callee: registry.Constructors[0].Symbol,
			FilePath: unit + "/provider.go", Kind: "focused-provider", OwnerTask: "T20", Reason: "Fixture focused provider.",
		})
	}
	return registry
}

// Compiler-loaded fixtures exercise only construction lint. Serial execution
// preserves the shared baseline fixture state; no application is assembled.
func TestConstructionRegisteredProviders(t *testing.T) {
	useFixtures(t)
	names := []string{"Direct", "Deferred", "Async", "ClosureConstruction", "Argument", "Recursive", "Mutual", "Generic", "Method", "Closure", "PackageClosure", "PackageMutation", "PackageAddress", "Callback", "Field", "Interface", "Mutable", "ReturnedDeclaration", "ReturnedClosure", "ReturnedAlias", "ReturnedNested", "SameAlternatives", "MixedAlternatives", "DistinctClosures", "NamedReturn", "TupleReturn", "ReturnCycle", "Escaped", "EscapedAlias", "Acyclic", "Uncalled", "Shadowed", "UnrelatedCycle", "Builtins", "ReturnedUncalled", "NestedReturn", "PromotedInterface", "PromotedMethod", "ReturnedDeferred", "ReturnedGo", "SameClosure", "ReturnedCallback", "ReturnedField", "MutualReturnCycle", "MutatedResult", "ReturnedEscaped", "UncalledEvaluation", "RecursionAndDebt", "PackageParenWrite", "PackageParenAddress", "ConstructorAlias"}
	registry := registeredProviderFixtureRegistry(ConstructionEnforce, "pkg/registeredproviders", names)
	analyzer := registeredConstructionAnalyzer(registry)
	run := analyzer.Run
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		findings, err := run(pass)
		if err != nil || pass.Pkg.Path() != "m/pkg/registeredproviders" {
			return findings, err
		}
		reportRegistry := registry
		reportRegistry.CapabilitySets = []ConstructionCapabilitySet{{Name: "fixture", OwnerTask: "T20", Mode: ConstructionReport}}
		report, err := runRegisteredConstruction(pass, reportRegistry)
		if err != nil {
			return nil, err
		}
		actual, observations := findings.([]ConstructionFinding), report.([]ConstructionFinding)
		if len(actual) != 39 || len(observations) != len(actual) {
			t.Errorf("enforce/report=%d/%d, want 39/39", len(actual), len(observations))
		}
		for i, finding := range actual {
			if finding.Callee != registry.Constructors[0].Symbol || finding.Mode != ConstructionEnforce || finding.Line == 0 {
				t.Errorf("invalid finding: %#v", finding)
			}
			finding.Mode = ConstructionReport
			if i < len(observations) && finding != observations[i] {
				t.Errorf("report changed finding: %#v / %#v", finding, observations[i])
			}
		}
		return findings, nil
	}
	analysistest.Run(t, analysistest.TestData(), analyzer, "m/pkg/registeredproviders")
}

func TestConstructionRegisteredProviderBaseline(t *testing.T) {
	const unit = "pkg/registeredproviderlisted"
	const edge = "m/" + unit + "."
	const target = "->m/pkg/registeredowner.New"
	useFixtures(t,
		"registered-construction|"+unit+"|"+edge+"Recursive"+target,
		"unresolved-focused-provider-dispatch|"+unit+"|"+edge+"Callback"+target,
		"unresolved-focused-provider-dispatch|"+unit+"|"+edge+"Removed"+target)
	registry := registeredProviderFixtureRegistry(ConstructionEnforce, unit, []string{"Recursive", "Callback"})
	analysistest.Run(t, analysistest.TestData(), registeredConstructionAnalyzer(registry), "m/"+unit)
}
