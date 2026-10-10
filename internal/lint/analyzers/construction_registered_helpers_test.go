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

// A public validator does not excuse a repeated check on the constructed path.
// Controlled metadata proves the provenance rule without enabling an owner
// over unresolved production debt. No runtime behavior is inferred here.
func TestConstructionRequiredReaderThroughSharedValidator(t *testing.T) {
	useFixtures(t)
	const owner = "m/pkg/requiredreader"
	service := ConstructionSymbol{ImportPath: owner, Name: "Service"}
	registry := ConstructionRegistry{
		CapabilitySets: []ConstructionCapabilitySet{{Name: "reader", OwnerTask: "T29", Mode: ConstructionEnforce}},
		Types:          []ConstructionType{{Symbol: service, CapabilitySet: "reader", Kind: ConstructionBehavior}},
		Constructors: []ConstructionConstructor{{
			Symbol: ConstructionSymbol{ImportPath: owner, Name: "New"}, CapabilitySet: "reader",
			Results:            []ConstructionSymbol{service},
			RequiredParameters: []ConstructionParameter{{Index: 0, TypeExpr: owner + ".Reader"}},
		}},
	}
	dir, cleanup, err := analysistest.WriteFiles(map[string]string{
		owner + "/reader.go": `package requiredreader
type Reader func(string) ([]byte, error)
type Service struct { read Reader }
func New(read Reader) *Service { return &Service{read: read} }
func (s *Service) Submit(path string) ([]byte, error) { return validatedRead(path, s.read) }
func PublicSubmit(path string, read Reader) ([]byte, error) { return validatedRead(path, read) }
func validatedRead(path string, read Reader) ([]byte, error) {
 if read == nil { return nil, nil } // want "required-dependency-guard:.*requiredreader.validatedRead.*requiredreader.New"
 return read(path)
}
// Caller validation is lawful when it is separate from the injected path.
func PublicOnly(path string, read Reader) ([]byte, error) {
 if read == nil { return nil, nil }
 return read(path)
}
func (s *Service) Direct(path string) ([]byte, error) { return s.read(path) }
// Absence of the selected resource is distinct from absence of the resolver.
func (s *Service) Selected(resolve func() any) bool {
 resource := resolve()
 return resource != nil
}
`,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)
	analysistest.Run(t, dir, registeredConstructionAnalyzer(registry), owner)
}
