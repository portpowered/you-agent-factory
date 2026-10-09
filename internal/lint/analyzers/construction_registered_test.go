package analyzers

import (
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func registeredFixtureRegistry(mode ConstructionMode) ConstructionRegistry {
	const owner = "m/pkg/registeredowner"
	service := ConstructionSymbol{ImportPath: owner, Name: "Service"}
	registry := ConstructionRegistry{
		CapabilitySets: []ConstructionCapabilitySet{{Name: "fixture", OwnerTask: "T20", Mode: mode}},
		Types: []ConstructionType{
			{Symbol: service, CapabilitySet: "fixture", Kind: ConstructionBehavior},
			{Symbol: ConstructionSymbol{ImportPath: owner, Name: "Domain"}, CapabilitySet: "fixture", Kind: ConstructionDomain},
		},
	}
	for _, symbol := range []ConstructionSymbol{
		{ImportPath: owner, Name: "New"}, {ImportPath: owner, Name: "Generic"},
		{ImportPath: owner, Receiver: "Maker", Name: "Construct"},
	} {
		registry.Constructors = append(registry.Constructors, ConstructionConstructor{
			Symbol: symbol, CapabilitySet: "fixture", Results: []ConstructionSymbol{service},
			RequiredParameters: []ConstructionParameter{{Index: 0, TypeExpr: owner + ".Dependency"}},
		})
	}
	registry.Constructors = append(registry.Constructors, ConstructionConstructor{
		Symbol: ConstructionSymbol{ImportPath: owner, Name: "Value"}, CapabilitySet: "fixture",
		Results: []ConstructionSymbol{{ImportPath: owner, Name: "Domain"}},
	})
	return registry
}

func TestConstructionRegisteredMetadataErrors(t *testing.T) {
	useFixtures(t)
	results := analysistest.Run(t, analysistest.TestData(), registeredConstructionAnalyzer(registeredFixtureRegistry(ConstructionReport)), "m/pkg/registeredowner")
	if len(results) != 1 {
		t.Fatalf("analysis results = %d, want one defining unit", len(results))
	}
	tests := []struct {
		name   string
		change func(*ConstructionRegistry)
		want   string
	}{
		{"ownerless set", func(r *ConstructionRegistry) { r.CapabilitySets[0].OwnerTask = "" }, "invalid capability set"},
		{"duplicate set", func(r *ConstructionRegistry) { r.CapabilitySets = append(r.CapabilitySets, r.CapabilitySets[0]) }, "duplicate capability set"},
		{"duplicate type", func(r *ConstructionRegistry) { r.Types = append(r.Types, r.Types[0]) }, "duplicate type"},
		{"missing type", func(r *ConstructionRegistry) { r.Types[0].Symbol.Name = "Missing" }, "missing type declaration"},
		{"invalid kind", func(r *ConstructionRegistry) { r.Types[0].Kind = "invalid" }, "invalid construction kind"},
		{"duplicate constructor", func(r *ConstructionRegistry) { r.Constructors = append(r.Constructors, r.Constructors[0]) }, "duplicate constructor"},
		{"missing constructor", func(r *ConstructionRegistry) { r.Constructors[0].Symbol.Name = "Missing" }, "missing constructor declaration"},
		{"wildcard constructor", func(r *ConstructionRegistry) { r.Constructors[0].Symbol.ImportPath = "m/*" }, "invalid constructor metadata"},
		{"unknown set", func(r *ConstructionRegistry) { r.Constructors[0].CapabilitySet = "missing" }, "invalid constructor metadata"},
		{"parameter index", func(r *ConstructionRegistry) { r.Constructors[0].RequiredParameters[0].Index = 9 }, "invalid or duplicate parameter index"},
		{"duplicate parameter", func(r *ConstructionRegistry) {
			r.Constructors[0].RequiredParameters = append(r.Constructors[0].RequiredParameters, r.Constructors[0].RequiredParameters[0])
		}, "invalid or duplicate parameter index"},
		{"result mismatch", func(r *ConstructionRegistry) { r.Constructors[0].Results[0].Name = "Domain" }, "result type mismatch"},
		{"unclassified result", func(r *ConstructionRegistry) { r.Constructors[0].Results[0].Name = "Missing" }, "matching type classification"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			registry := registeredFixtureRegistry(ConstructionReport)
			test.change(&registry)
			err := validateRegisteredConstruction(results[0].Pass, registry)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestConstructionRegisteredAllowanceErrors(t *testing.T) {
	useFixtures(t)
	results := analysistest.Run(t, analysistest.TestData(), registeredConstructionAnalyzer(registeredFixtureRegistry(ConstructionReport)), "m/pkg/registeredallowed")
	if len(results) != 1 {
		t.Fatalf("analysis results = %d, want one calling unit", len(results))
	}
	allowance := ConstructionAllowance{
		Caller:   ConstructionSymbol{ImportPath: "m/pkg/registeredallowed", Name: "Provide"},
		Callee:   ConstructionSymbol{ImportPath: "m/pkg/registeredowner", Name: "New"},
		FilePath: "pkg/registeredallowed/provider.go", Kind: "focused-provider", OwnerTask: "T20", Reason: "Fixture focused provider.",
	}
	tests := []struct {
		name   string
		change func(*ConstructionAllowance)
		want   string
	}{
		{"wildcard", func(a *ConstructionAllowance) { a.FilePath = "pkg/*/provider.go" }, "invalid exact allowance"},
		{"ownerless", func(a *ConstructionAllowance) { a.OwnerTask = "" }, "invalid exact allowance"},
		{"missing reason", func(a *ConstructionAllowance) { a.Reason = "" }, "invalid exact allowance"},
		{"kind", func(a *ConstructionAllowance) { a.Kind = "invalid" }, "invalid allowance kind"},
		{"stale caller", func(a *ConstructionAllowance) { a.Caller.Name = "Missing" }, "stale allowance caller/path"},
		{"stale path", func(a *ConstructionAllowance) { a.FilePath = "pkg/registeredallowed/missing.go" }, "stale allowance caller/path"},
		{"stale callee", func(a *ConstructionAllowance) { a.Callee.Name = "Generic" }, "stale allowance callee"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			registry := registeredFixtureRegistry(ConstructionReport)
			modified := allowance
			test.change(&modified)
			registry.Allowances = []ConstructionAllowance{modified}
			err := validateRegisteredConstruction(results[0].Pass, registry)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
	registry := registeredFixtureRegistry(ConstructionReport)
	registry.Allowances = []ConstructionAllowance{allowance, allowance}
	if err := validateRegisteredConstruction(results[0].Pass, registry); err == nil || !strings.Contains(err.Error(), "duplicate allowance") {
		t.Fatalf("error = %v, want duplicate allowance", err)
	}
}

func TestConstructionRegisteredTypedCallsAndReferences(t *testing.T) {
	useFixtures(t)
	registry := registeredFixtureRegistry(ConstructionEnforce)
	registry.Allowances = []ConstructionAllowance{{
		Caller:   ConstructionSymbol{ImportPath: "m/pkg/registeredallowed", Name: "Provide"},
		Callee:   ConstructionSymbol{ImportPath: "m/pkg/registeredowner", Name: "New"},
		FilePath: "pkg/registeredallowed/provider.go", Kind: "focused-provider", OwnerTask: "T20", Reason: "Fixture focused provider.",
	}}
	analysistest.Run(t, analysistest.TestData(), registeredConstructionAnalyzer(registry),
		"m/pkg/registeredowner", "m/pkg/registeredcalls", "m/pkg/registereddot", "m/pkg/registeredallowed", "m/pkg/registeredexcluded")
}

func TestConstructionRegisteredReportObservations(t *testing.T) {
	useFixtures(t)
	results := analysistest.Run(t, analysistest.TestData(), registeredConstructionAnalyzer(registeredFixtureRegistry(ConstructionReport)), "m/pkg/registeredreport")
	for _, result := range results {
		findings := result.Result.([]ConstructionFinding)
		if len(findings) != 1 {
			t.Fatalf("findings = %#v, want one report-only observation", findings)
		}
		finding := findings[0]
		if finding.Rule != "registered-construction" || finding.Mode != ConstructionReport || finding.CapabilitySet != "fixture" ||
			finding.Callee.String() != "m/pkg/registeredowner.New" || finding.Caller.String() != "m/pkg/registeredreport.Report" ||
			finding.FilePath != "pkg/registeredreport/report.go" || finding.Line != 5 {
			t.Fatalf("observation lost identity: %#v", finding)
		}
	}
}

func TestConstructionRegisteredBaseline(t *testing.T) {
	useFixtures(t,
		"registered-construction|pkg/registeredlisted|m/pkg/registeredlisted.Listed->m/pkg/registeredowner.New",
		"registered-construction|pkg/registeredstale|m/pkg/registeredstale.Listed->m/pkg/registeredowner.New",
	)
	analysistest.Run(t, analysistest.TestData(), registeredConstructionAnalyzer(registeredFixtureRegistry(ConstructionEnforce)), "m/pkg/registeredlisted", "m/pkg/registeredstale")
}

// Real compiler objects prove that omission from the constructor list cannot
// exempt an already classified result, even within the owning service.
func TestConstructionUnlistedClassifiedResults(t *testing.T) {
	useFixtures(t)
	const owner = "m/pkg/services/unlisted"
	registry := ConstructionRegistry{
		CapabilitySets: []ConstructionCapabilitySet{
			{Name: "unlisted", OwnerTask: "T29", Mode: ConstructionEnforce},
			{Name: "reporting", OwnerTask: "T29", Mode: ConstructionReport},
		},
		Types: []ConstructionType{{
			Symbol:        ConstructionSymbol{ImportPath: owner, Name: "Reporting"},
			CapabilitySet: "reporting", Kind: ConstructionBehavior,
		}},
	}
	for name, kind := range map[string]ConstructionKind{
		"Service": ConstructionBehavior, "Dependency": ConstructionEffect,
		"Domain": ConstructionDomain, "Scope": ConstructionState, "Resource": ConstructionResource,
	} {
		registry.Types = append(registry.Types, ConstructionType{
			Symbol: ConstructionSymbol{ImportPath: owner, Name: name}, CapabilitySet: "unlisted", Kind: kind,
		})
	}
	files := map[string]string{
		owner + "/owner.go": `package unlisted
type Dependency interface { Run() }
type Service struct{}
type Reporting struct{}
type Alias = Service
type Domain struct{}
type Scope struct{}
type Resource struct{}
func NewAlternate(dep Dependency) *Alias { return &Service{} }
func NewGeneric[T any](dep Dependency) (*Service, error) { return &Service{}, nil }
func NewMixed() (*Reporting, *Service) { return &Reporting{}, &Service{} }
func NewGuarded(dep Dependency) *Service {
 if dep == nil { return nil } // want "required-dependency-guard:.*NewGuarded"
 return &Service{}
}
func NewDomain(payload *Domain) Domain { if payload == nil { return Domain{} }; return *payload }
func NewScope(previous *Scope) *Scope { if previous == nil { return &Scope{} }; return previous }
func OpenResource(previous *Resource) *Resource { if previous == nil { return &Resource{} }; return previous }
func Run(dep Dependency) {
 NewAlternate(dep) // want "registered-construction:.*Run.*m/pkg/services/unlisted.NewAlternate"
 NewGeneric[int](dep) // want "registered-construction:.*Run.*m/pkg/services/unlisted.NewGeneric"
 NewMixed() // want "registered-construction:.*Run.*m/pkg/services/unlisted.NewMixed"
 _ = NewDomain(nil); _ = NewScope(nil); _ = OpenResource(nil)
}
`,
		"m/pkg/services/unrelated/consumer.go": `package unrelated
import owner "m/pkg/services/unlisted"
func Run(dep owner.Dependency) {
 owner.NewAlternate(dep) // want "registered-construction:.*Run.*m/pkg/services/unlisted.NewAlternate"
 create := owner.NewGeneric[int]
 create(dep) // want "registered-construction:.*Run.*m/pkg/services/unlisted.NewGeneric"
 _ = owner.NewDomain(nil); _ = owner.NewScope(nil); _ = owner.OpenResource(nil)
}
func Escape() any { return owner.NewAlternate } // want "unresolved-construction-reference:.*Escape.*NewAlternate"
func Shadowed() { owner := struct { NewAlternate func() }{func(){}}; owner.NewAlternate() }
`,
	}
	dir, cleanup, err := analysistest.WriteFiles(files)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)
	analysistest.Run(t, dir, registeredConstructionAnalyzer(registry), owner, "m/pkg/services/unrelated")
}

func TestConstructionRegisteredSignatureValidation(t *testing.T) {
	useFixtures(t)
	registry := registeredFixtureRegistry(ConstructionReport)
	registry.Constructors = registry.Constructors[:1]
	registry.Constructors[0].Symbol.ImportPath = "m/pkg/registeredinvalid"
	registry.Constructors[0].Results[0].ImportPath = "m/pkg/registeredinvalid"
	registry.Types = registry.Types[:1]
	registry.Types[0].Symbol.ImportPath = "m/pkg/registeredinvalid"
	analysistest.Run(t, analysistest.TestData(), registeredConstructionAnalyzer(registry), "m/pkg/registeredinvalid")
}

func TestConstructionRegisteredTagged(t *testing.T) {
	useFixtures(t)
	t.Setenv("GOFLAGS", "-tags=backendconformance")
	analysistest.Run(t, analysistest.TestData(), registeredConstructionAnalyzer(registeredFixtureRegistry(ConstructionEnforce)), "m/pkg/registeredtagged")
}
