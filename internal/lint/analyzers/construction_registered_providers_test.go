package analyzers

import (
	"os"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
	"golang.org/x/tools/go/packages"
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

// Historical source-only examples that are invalid Go belong to the compiler
// boundary, rather than successful construction-analysis observations.
func TestConstructionCompilerRejectsInvalidHistoricalCases(t *testing.T) {
	for _, test := range []struct{ unit, message string }{
		{"registeredinvalidcycle", "initialization cycle"},
		{"registeredinvalidselector", "ambiguous selector"},
		{"registeredinvalidalias", "invalid recursive type"},
		{"registeredinvalidunknown", "undefined: Unknown"},
	} {
		t.Run(test.unit, func(t *testing.T) {
			loaded, err := packages.Load(&packages.Config{
				Mode: packages.NeedName | packages.NeedFiles | packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo,
				Dir:  analysistest.TestData(),
				Env:  append(os.Environ(), "GO111MODULE=off", "GOPATH="+analysistest.TestData(), "GOWORK=off"),
			}, "m/pkg/"+test.unit)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, pkg := range loaded {
				for _, diagnostic := range pkg.Errors {
					if diagnostic.Kind == packages.TypeError && strings.Contains(diagnostic.Msg, test.message) {
						found = true
						if diagnostic.Pos == "" {
							t.Error("compiler error lost its position")
						}
					}
				}
			}
			if !found {
				t.Fatalf("compiler did not reject %s with %q", test.unit, test.message)
			}
		})
	}
}

// Compiler-loaded fixtures exercise only construction lint. Serial execution
// preserves the shared baseline fixture state; no application is assembled.
func TestConstructionRegisteredProviders(t *testing.T) {
	useFixtures(t)
	names := []string{"Direct", "Deferred", "Async", "ClosureConstruction", "Argument", "Recursive", "Mutual", "Generic", "Method", "Closure", "PackageClosure", "PackageMutation", "PackageAddress", "Callback", "Field", "Interface", "Mutable", "ReturnedDeclaration", "ReturnedClosure", "ReturnedAlias", "ReturnedNested", "SameAlternatives", "MixedAlternatives", "DistinctClosures", "NamedReturn", "TupleReturn", "ReturnCycle", "Escaped", "EscapedAlias", "Acyclic", "Uncalled", "Shadowed", "UnrelatedCycle", "Builtins", "ReturnedUncalled", "NestedReturn", "PromotedInterface", "PromotedMethod", "ReturnedDeferred", "ReturnedGo", "SameClosure", "ReturnedCallback", "ReturnedField", "MutualReturnCycle", "MutatedResult", "ReturnedEscaped", "UncalledEvaluation", "RecursionAndDebt", "PackageParenWrite", "PackageParenAddress", "ConstructorAlias"}
	names = append(names, "InterfaceAlias", "PromotedInterfaceAlias", "ConcreteMethodAlias")
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
		if len(actual) != 41 || len(observations) != len(actual) {
			t.Errorf("enforce/report=%d/%d, want 41/41", len(actual), len(observations))
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

// Compile-valid retained provider cases have exact allowances and diagnostic
// controls. Callable returns use the operator-authorized typed rule. Shared
// baseline fixture state requires serial execution.
func TestConstructionRegisteredProviderParity(t *testing.T) {
	useFixtures(t)
	cases := registeredProviderParityCases()
	registry := registeredFixtureRegistry(ConstructionEnforce)
	expected := map[ConstructionSymbol]int{}
	for _, test := range cases {
		caller := ConstructionSymbol{ImportPath: "m/" + test.unit, Name: test.name}
		expected[caller] = test.count
		registry.Allowances = append(registry.Allowances, ConstructionAllowance{
			Caller: caller, Callee: registry.Constructors[0].Symbol, FilePath: test.unit + "/provider.go",
			Kind: "focused-provider", OwnerTask: "T20", Reason: "Retained compile-valid provider behavior.",
		})
	}
	analyzer := registeredConstructionAnalyzer(registry)
	run := analyzer.Run
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		result, err := run(pass)
		if err != nil {
			return result, err
		}
		reportRegistry := registry
		reportRegistry.CapabilitySets = []ConstructionCapabilitySet{{Name: "fixture", OwnerTask: "T20", Mode: ConstructionReport}}
		report, err := runRegisteredConstruction(pass, reportRegistry)
		if err != nil {
			return result, err
		}
		findings, observations := result.([]ConstructionFinding), report.([]ConstructionFinding)
		if len(findings) != len(observations) {
			t.Errorf("enforce/report count mismatch: %d/%d", len(findings), len(observations))
		}
		counts := map[ConstructionSymbol]int{}
		for i, finding := range findings {
			counts[finding.Caller]++
			if finding.Callee != registry.Constructors[0].Symbol || finding.Line == 0 || finding.Mode != ConstructionEnforce {
				t.Errorf("invalid finding: %#v", finding)
			}
			finding.Mode = ConstructionReport
			if i < len(observations) && finding != observations[i] {
				t.Errorf("report changed finding: %#v / %#v", finding, observations[i])
			}
		}
		for caller, count := range expected {
			if caller.ImportPath == pass.Pkg.Path() && counts[caller] != count {
				t.Errorf("%s count=%d, want %d", caller, counts[caller], count)
			}
		}
		return result, nil
	}
	analysistest.Run(t, analysistest.TestData(), analyzer,
		"m/pkg/registeredprovidercycles", "m/pkg/registeredproviderdebt", "m/pkg/registeredproviderexecution")
}

type registeredProviderParityCase struct {
	unit, name string
	count      int
}

func registeredProviderParityCases() []registeredProviderParityCase {
	return []registeredProviderParityCase{
		{"pkg/registeredprovidercycles", "Case01", 1},
		{"pkg/registeredprovidercycles", "Case02", 1},
		{"pkg/registeredprovidercycles", "Case03", 1},
		{"pkg/registeredprovidercycles", "Case04", 1},
		{"pkg/registeredprovidercycles", "Case05", 1},
		{"pkg/registeredprovidercycles", "Case06", 1},
		{"pkg/registeredprovidercycles", "Case07", 1},
		{"pkg/registeredprovidercycles", "Case08", 1},
		{"pkg/registeredprovidercycles", "Case09", 1},
		{"pkg/registeredprovidercycles", "Case10", 1},
		{"pkg/registeredprovidercycles", "Case11", 1},
		{"pkg/registeredprovidercycles", "Case12", 1},
		{"pkg/registeredprovidercycles", "Case13", 1},
		{"pkg/registeredprovidercycles", "Case14", 1},
		{"pkg/registeredprovidercycles", "Case15", 1},
		{"pkg/registeredprovidercycles", "Case16", 1},
		{"pkg/registeredprovidercycles", "Case17", 1},
		{"pkg/registeredprovidercycles", "Case18", 1},
		{"pkg/registeredprovidercycles", "Case19", 1},
		{"pkg/registeredprovidercycles", "Case20", 1},
		{"pkg/registeredprovidercycles", "Case21", 1},
		{"pkg/registeredprovidercycles", "Case22", 0},
		{"pkg/registeredprovidercycles", "Case23", 0},
		{"pkg/registeredprovidercycles", "Case24", 0},
		{"pkg/registeredprovidercycles", "Case25", 0},
		{"pkg/registeredprovidercycles", "Case32", 0},
		{"pkg/registeredprovidercycles", "Case33", 0},
		{"pkg/registeredprovidercycles", "Case34", 0},
		{"pkg/registeredprovidercycles", "Case38", 0},
		{"pkg/registeredprovidercycles", "Case39", 0},
		{"pkg/registeredprovidercycles", "Case40", 0},
		{"pkg/registeredprovidercycles", "Case41", 0},
		{"pkg/registeredprovidercycles", "Case42", 0},
		{"pkg/registeredprovidercycles", "Case43", 0},
		{"pkg/registeredproviderdebt", "Case06", 1},
		{"pkg/registeredproviderdebt", "Case07", 1},
		{"pkg/registeredproviderdebt", "Case08", 1},
		{"pkg/registeredproviderdebt", "Case09", 1},
		{"pkg/registeredproviderdebt", "Case10", 1},
		{"pkg/registeredproviderdebt", "Case11", 1},
		{"pkg/registeredproviderdebt", "Case12", 1},
		{"pkg/registeredproviderdebt", "Case13", 1},
		{"pkg/registeredproviderdebt", "Case14", 1},
		{"pkg/registeredproviderdebt", "Case15", 1},
		{"pkg/registeredproviderdebt", "Case16", 1},
		{"pkg/registeredproviderdebt", "Case17", 1},
		{"pkg/registeredproviderdebt", "Case18", 0},
		{"pkg/registeredproviderdebt", "Case19", 1},
		{"pkg/registeredproviderdebt", "Case20", 1},
		{"pkg/registeredproviderdebt", "Case21", 1},
		{"pkg/registeredproviderdebt", "Case22", 1},
		{"pkg/registeredproviderdebt", "Case23", 1},
		{"pkg/registeredproviderdebt", "Case24", 1},
		{"pkg/registeredproviderdebt", "Case25", 1},
		{"pkg/registeredproviderdebt", "Case26", 1},
		{"pkg/registeredproviderdebt", "Case27", 1},
		{"pkg/registeredproviderdebt", "Case28", 1},
		{"pkg/registeredproviderdebt", "Case29", 1},
		{"pkg/registeredproviderdebt", "Case30", 1},
		{"pkg/registeredproviderdebt", "Case31", 0},
		{"pkg/registeredproviderdebt", "Case32", 0},
		{"pkg/registeredproviderdebt", "Case33", 0},
		{"pkg/registeredproviderdebt", "Case34", 0},
		{"pkg/registeredproviderdebt", "Case35", 0},
		{"pkg/registeredproviderdebt", "Case36", 0},
		{"pkg/registeredproviderdebt", "Case37", 0},
		{"pkg/registeredproviderdebt", "Case38", 0},
		{"pkg/registeredproviderdebt", "Case39", 0},
		{"pkg/registeredproviderdebt", "Case40", 0},
		{"pkg/registeredproviderdebt", "Case41", 0},
		{"pkg/registeredproviderexecution", "Case01", 0},
		{"pkg/registeredproviderexecution", "Case02", 0},
		{"pkg/registeredproviderexecution", "Case03", 0},
		{"pkg/registeredproviderexecution", "Case04", 1},
		{"pkg/registeredproviderexecution", "Case05", 1},
		{"pkg/registeredproviderexecution", "Case06", 1},
		{"pkg/registeredproviderexecution", "Case07", 1},
		{"pkg/registeredproviderexecution", "Case08", 1},
		{"pkg/registeredproviderexecution", "Case09", 1},
		{"pkg/registeredproviderexecution", "Case10", 1},
		{"pkg/registeredproviderexecution", "Case11", 0},
		{"pkg/registeredproviderexecution", "Case12", 0},
	}
}
