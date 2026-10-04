package contractguard

import (
	"bytes"
	"strings"
	"testing"
)

func TestConstructionRequiredDependencyBags(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, declarations, parameter string
		kind                          ConstructionKind
		want                          int
	}{
		{"behavior field", `type Inputs struct { child *Child }`, "Inputs", ConstructionBehavior, 1},
		{"effect field", `type Inputs struct { child *Child }`, "Inputs", ConstructionEffect, 1},
		{"embedded", `type Inputs struct { *Child }`, "Inputs", ConstructionBehavior, 1},
		{"nested", `type Nested struct { child *Child }; type Inputs struct { nested Nested }`, "Inputs", ConstructionBehavior, 1},
		{"aliased record", `type Bag struct { child *Child }; type Inputs = Bag`, "Inputs", ConstructionBehavior, 1},
		{"defined record", `type Bag struct { child *Child }; type Inputs Bag`, "Inputs", ConstructionBehavior, 1},
		{"aliased collaborator", `type Alias = Child; type Inputs struct { child *Alias }`, "Inputs", ConstructionBehavior, 1},
		{"defined collaborator", `type Alias Child; type Inputs struct { child *Alias }`, "Inputs", ConstructionBehavior, 1},
		{"pointer record", `type Inputs struct { child *Child }`, "*Inputs", ConstructionBehavior, 1},
		{"slice field", `type Inputs struct { children []*Child }`, "Inputs", ConstructionBehavior, 1},
		{"map field", `type Inputs struct { children map[string]*Child }`, "Inputs", ConstructionBehavior, 1},
		{"map key", `type Inputs struct { children map[*Child]string }`, "Inputs", ConstructionBehavior, 1},
		{"nested map key", `type Nested struct { child *Child }; type Inputs struct { children map[*Nested]string }`, "Inputs", ConstructionBehavior, 1},
		{"channel field", `type Inputs struct { children chan *Child }`, "Inputs", ConstructionBehavior, 1},
		{"recursive record", `type Inputs struct { next *Inputs; child *Child }`, "Inputs", ConstructionBehavior, 1},
		{"multiple collaborators one parameter", `type Inputs struct { first, second *Child }`, "Inputs", ConstructionBehavior, 1},
		{"domain field", `type Inputs struct { child *Child }`, "Inputs", ConstructionDomain, 0},
		{"state field", `type Inputs struct { child *Child }`, "Inputs", ConstructionState, 0},
		{"resource field", `type Inputs struct { child *Child }`, "Inputs", ConstructionResource, 0},
		{"ordinary configuration", `type Inputs struct { enabled bool; names []string }`, "Inputs", ConstructionBehavior, 0},
		{"recursive domain", `type Inputs struct { next *Inputs; value int }`, "Inputs", ConstructionBehavior, 0},
		{"name collision", `type DomainChild struct {}; type Inputs struct { child *DomainChild }`, "Inputs", ConstructionBehavior, 0},
		{"direct collaborator", `type Inputs = Child`, "Inputs", ConstructionBehavior, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root, registry := constructionFixture(t)
			registry.CapabilitySets[0].Mode = ConstructionEnforce
			registry.Allowances = nil
			registry.Constructors[0].RequiredParameters[0].TypeExpr = strings.ReplaceAll(tc.parameter, "Inputs", fixtureOwner+".Inputs")
			registry.Types = append(registry.Types, ConstructionType{
				Symbol: ConstructionSymbol{ImportPath: fixtureOwner, Name: "Child"}, CapabilitySet: "owner", Kind: tc.kind,
			})
			writeConstructionFixture(t, root, "pkg/wire/provider.go", "package wire\n")
			writeConstructionFixture(t, root, "pkg/owner/service.go", `package owner
type Child struct {}
`+tc.declarations+`
type Service struct { input `+tc.parameter+` }
func New(secretInput `+tc.parameter+`) (*Service, error) { return &Service{input: secretInput}, nil }
`)
			findings, err := ScanConstruction(root, registry)
			if err != nil {
				t.Fatal(err)
			}
			if len(findings) != tc.want || CountBlockingConstructionFindings(findings) != tc.want {
				t.Fatalf("findings = %+v, want %d blocking bags", findings, tc.want)
			}
			for _, finding := range findings {
				if finding.Rule != "required-dependency-bag" || finding.Caller != registry.Constructors[0].Symbol || finding.Callee != finding.Caller || finding.FilePath != "pkg/owner/service.go" || finding.Line != 5 {
					t.Fatalf("unexpected bag identity: %+v", finding)
				}
			}
			var output bytes.Buffer
			WriteConstructionFindings(&output, findings)
			if strings.Contains(output.String(), "secretInput") || strings.Contains(output.String(), "children") {
				t.Fatalf("diagnostic discloses source: %s", output.String())
			}
			registry.CapabilitySets[0].Mode = ConstructionReport
			findings, err = ScanConstruction(root, registry)
			if err != nil || len(findings) != tc.want || CountBlockingConstructionFindings(findings) != 0 {
				t.Fatalf("report findings = %+v, error = %v", findings, err)
			}
		})
	}
}

func TestConstructionImportedDependencyBag(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, declarations string
		want               int
	}{
		{"import alias", `type Inputs struct { child *chosen.Child }`, 1},
		{"imported type alias", `type Alias = chosen.Child; type Inputs struct { child *Alias }`, 1},
		{"imported defined record", `type Inputs chosen.Bag`, 1},
		{"imported record alias", `type Inputs = chosen.Bag`, 1},
		{"qualified collision", `type Child struct {}; type Inputs struct { child *Child; payload chosen.Payload }`, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root, registry := constructionFixture(t)
			registry.Allowances = nil
			registry.CapabilitySets[0].Mode = ConstructionEnforce
			registry.Constructors[0].RequiredParameters[0].TypeExpr = fixtureOwner + ".Inputs"
			registry.Types = append(registry.Types, ConstructionType{
				Symbol: ConstructionSymbol{ImportPath: "example.test/factory/pkg/child", Name: "Child"}, CapabilitySet: "owner", Kind: ConstructionBehavior,
			})
			writeConstructionFixture(t, root, "pkg/wire/provider.go", "package wire\n")
			writeConstructionFixture(t, root, "pkg/child/child.go", `package child
type Child struct {}
type Payload struct { value int }
type Bag struct { child *Child }
`)
			writeConstructionFixture(t, root, "pkg/owner/service.go", `package owner
import chosen "example.test/factory/pkg/child"
`+tc.declarations+`
type Service struct {}
func New(input Inputs) (*Service, error) { return &Service{}, nil }
`)
			findings, err := ScanConstruction(root, registry)
			if err != nil || len(findings) != tc.want || CountBlockingConstructionFindings(findings) != tc.want {
				t.Fatalf("findings = %+v, error = %v, want %d", findings, err, tc.want)
			}
		})
	}
}

func TestConstructionBagAuthorityAndParameterPositions(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		kind    ConstructionKind
		indices []int
		want    int
	}{
		{"grouped required", "", []int{1, 2}, 2},
		{"unnamed required", "", []int{0}, 1},
		{"optional record", "", nil, 0},
		{"classified domain", ConstructionDomain, []int{1}, 0},
		{"classified state", ConstructionState, []int{1}, 0},
		{"classified resource", ConstructionResource, []int{1}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root, registry := constructionFixture(t)
			registry.Allowances = nil
			registry.CapabilitySets[0].Mode = ConstructionEnforce
			registry.Constructors[0].RequiredParameters = nil
			for _, position := range tc.indices {
				registry.Constructors[0].RequiredParameters = append(registry.Constructors[0].RequiredParameters, ConstructionParameter{Index: position, TypeExpr: fixtureOwner + ".Inputs"})
			}
			registry.Types = append(registry.Types, ConstructionType{Symbol: ConstructionSymbol{ImportPath: fixtureOwner, Name: "Child"}, CapabilitySet: "owner", Kind: ConstructionBehavior})
			if tc.kind != "" {
				registry.Types = append(registry.Types, ConstructionType{Symbol: ConstructionSymbol{ImportPath: fixtureOwner, Name: "Inputs"}, CapabilitySet: "owner", Kind: tc.kind})
			}
			parameters := "optional int, first, second Inputs"
			if tc.name == "unnamed required" {
				parameters = "Inputs"
			}
			writeConstructionFixture(t, root, "pkg/wire/provider.go", "package wire\n")
			writeConstructionFixture(t, root, "pkg/owner/service.go", `package owner
type Child struct {}
type Inputs struct { child *Child }
type Service struct {}
func New(`+parameters+`) (*Service, error) { return &Service{}, nil }
`)
			findings, err := ScanConstruction(root, registry)
			if err != nil || len(findings) != tc.want || CountBlockingConstructionFindings(findings) != tc.want {
				t.Fatalf("findings = %+v, error = %v, want %d", findings, err, tc.want)
			}
		})
	}
}
