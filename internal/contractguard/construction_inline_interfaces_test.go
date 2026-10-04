package contractguard

import "testing"

func TestConstructionInlineInterfaceSelectors(t *testing.T) {
	t.Parallel()
	for _, tc := range constructionInlineInterfaceCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root, registry := constructionFixture(t)
			registry.Types = append(registry.Types, ConstructionType{Symbol: ConstructionSymbol{ImportPath: fixtureOwner, Name: "Port"}, CapabilitySet: "owner", Kind: ConstructionBehavior})
			writeConstructionFixture(t, root, "pkg/owner/service.go", `package owner
type Port interface { Execute() }
type Service struct { port Port }
func New(port Port) (*Service, error) { return &Service{port: port}, nil }
func (s *Service) Lookup() Port { return s.port }
func Run(view *View) { `+tc.operation+` }
`)
			writeConstructionFixture(t, root, "pkg/owner/contracts.go", "package owner\n"+tc.contract+"\ntype Inner struct { *Service }; type View struct { Inner; Contract }")
			for _, mode := range []ConstructionMode{ConstructionReport, ConstructionEnforce} {
				registry.CapabilitySets[0].Mode = mode
				findings, err := ScanConstruction(root, registry)
				blocking := 0
				if mode == ConstructionEnforce {
					blocking = tc.want
				}
				if err != nil || len(findings) != tc.want || CountBlockingConstructionFindings(findings) != blocking {
					t.Fatalf("mode %s: findings = %+v, error = %v, want %d", mode, findings, err, tc.want)
				}
				for _, finding := range findings {
					if finding.Rule != "service-getter-locator" || finding.Caller != (ConstructionSymbol{ImportPath: fixtureOwner, Name: "Run"}) || finding.Callee != (ConstructionSymbol{ImportPath: fixtureOwner, Receiver: "Service", Name: "Lookup"}) || finding.FilePath != "pkg/owner/service.go" || finding.Line != 6 {
						t.Fatalf("unexpected promoted getter: %+v", finding)
					}
				}
			}
		})
	}
}

func constructionInlineInterfaceCases() []struct {
	name, contract, operation string
	want                      int
} {
	return []struct {
		name, contract, operation string
		want                      int
	}{
		{"inline empty", `type Contract interface { interface {} }`, `view.Lookup()`, 1},
		{"inline unrelated", `type Contract interface { interface { Observe() } }`, `view.Lookup()`, 1},
		{"inline any", `type Contract interface { interface { any } }`, `view.Lookup()`, 1},
		{"inline error", `type Contract interface { interface { error } }`, `view.Lookup()`, 1},
		{"nested inline", `type Contract interface { interface { interface { Observe() } } }`, `view.Lookup()`, 1},
		{"parenthesized interface", `type Contract (interface { Observe() })`, `view.Lookup()`, 1},
		{"parenthesized named embedding", `type Observer interface { Observe() }; type Contract interface { (Observer) }`, `view.Lookup()`, 1},
		{"parenthesized any", `type Contract interface { (any) }`, `view.Lookup()`, 1},
		{"parenthesized error", `type Contract interface { (error) }`, `view.Lookup()`, 1},
		{"inline alias", `type Contract = interface { interface { Observe() } }`, `view.Lookup()`, 1},
		{"inline method value", `type Contract interface { interface {} }`, `lookup := view.Lookup; lookup()`, 1},
		{"inline method expression", `type Contract interface { interface {} }`, `(*View).Lookup(view)`, 1},
		{"inline matching method", `type Contract interface { interface { Lookup() Port } }`, `view.Lookup()`, 0},
		{"nested matching method", `type Contract interface { interface { interface { Lookup() Port } } }`, `view.Lookup()`, 0},
		{"parenthesized matching method", `type Getter interface { Lookup() Port }; type Contract interface { (Getter) }`, `view.Lookup()`, 0},
		{"inline opaque", `type Contract interface { interface { Unknown } }`, `view.Lookup()`, 0},
		{"inline cycle", `type Contract interface { interface { Contract } }`, `view.Lookup()`, 0},
		{"parenthesized shadowed any", `type any interface { Lookup() Port }; type Contract interface { (any) }`, `view.Lookup()`, 0},
	}
}

func TestConstructionImportedInlineInterfaceSelectors(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, contract, method string
		want                   int
	}{
		{"imported inline unrelated", `type Contract interface { interface { Observe() } }`, "Lookup", 1},
		{"imported inline any", `type Contract interface { interface { any } }`, "Lookup", 1},
		{"imported inline error", `type Contract interface { interface { error } }`, "Lookup", 1},
		{"imported inline alias", `type Contract = interface { interface {} }`, "Lookup", 1},
		{"imported parenthesized alias", `type Base interface { Observe() }; type Contract = (Base)`, "Lookup", 1},
		{"imported parenthesized literal alias", `type Contract = (interface { Observe() })`, "Lookup", 1},
		{"imported private method cannot compete", `type Contract interface { interface { private() } }`, "private", 1},
		{"imported matching method", `type Contract interface { interface { Lookup() } }`, "Lookup", 0},
		{"imported matching literal alias", `type Contract = (interface { Lookup() })`, "Lookup", 0},
		{"imported opaque inline", `type Contract interface { interface { Unknown } }`, "Lookup", 0},
		{"imported cyclic inline", `type Contract interface { interface { Contract } }`, "Lookup", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root, registry := constructionFixture(t)
			registry.Types = append(registry.Types, ConstructionType{Symbol: ConstructionSymbol{ImportPath: fixtureOwner, Name: "Port"}, CapabilitySet: "owner", Kind: ConstructionBehavior})
			writeConstructionFixture(t, root, "pkg/contracts/contracts.go", "package contracts\n"+tc.contract)
			writeConstructionFixture(t, root, "pkg/owner/service.go", `package owner
import selected "example.test/factory/pkg/contracts"
type Port interface { Execute() }
type Service struct { port Port }
func New(port Port) (*Service, error) { return &Service{port: port}, nil }
func (s *Service) `+tc.method+`() Port { return s.port }
type Inner struct { *Service }; type View struct { Inner; selected.Contract }
func Run(view *View) { view.`+tc.method+`() }
`)
			for _, mode := range []ConstructionMode{ConstructionReport, ConstructionEnforce} {
				registry.CapabilitySets[0].Mode = mode
				findings, err := ScanConstruction(root, registry)
				blocking := 0
				if mode == ConstructionEnforce {
					blocking = tc.want
				}
				if err != nil || len(findings) != tc.want || CountBlockingConstructionFindings(findings) != blocking {
					t.Fatalf("mode %s: findings = %+v, error = %v, want %d", mode, findings, err, tc.want)
				}
				for _, finding := range findings {
					if finding.Rule != "service-getter-locator" || finding.Caller != (ConstructionSymbol{ImportPath: fixtureOwner, Name: "Run"}) || finding.Callee != (ConstructionSymbol{ImportPath: fixtureOwner, Receiver: "Service", Name: tc.method}) || finding.FilePath != "pkg/owner/service.go" || finding.Line != 8 {
						t.Fatalf("unexpected imported interface selector: %+v", finding)
					}
				}
			}
		})
	}
}
