package contractguard

import "testing"

func TestConstructionPredeclaredErrorSelector(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, contract string
		want           int
	}{
		{"error method competes", `type Contract interface { error }`, 0},
		{"error alias method competes", `type Failure = error; type Contract interface { Failure }`, 0},
		{"any has no error method", `type Contract interface { any }`, 1},
		{"shadowed error has no error method", `type error interface { Observe() }; type Contract interface { error }`, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root, registry := constructionFixture(t)
			registry.Types = append(registry.Types, ConstructionType{Symbol: ConstructionSymbol{ImportPath: fixtureOwner, Name: "Port"}, CapabilitySet: "owner", Kind: ConstructionBehavior})
			writeConstructionFixture(t, root, "pkg/owner/service.go", `package owner
type Port interface { Execute() }
type Service struct { port Port }
func New(port Port) (*Service, error) { return &Service{port: port}, nil }
func (s *Service) Error() Port { return s.port }
type Inner struct { *Service }
type View struct { Inner; Contract }
func Run(view *View) { view.Error() }
`+tc.contract)
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
					if finding.Rule != "service-getter-locator" || finding.Caller != (ConstructionSymbol{ImportPath: fixtureOwner, Name: "Run"}) || finding.Callee != (ConstructionSymbol{ImportPath: fixtureOwner, Receiver: "Service", Name: "Error"}) || finding.FilePath != "pkg/owner/service.go" || finding.Line != 8 {
						t.Fatalf("unexpected promoted getter: %+v", finding)
					}
				}
			}
		})
	}
}

func TestConstructionPredeclaredInterfaceSelectors(t *testing.T) {
	t.Parallel()
	for _, tc := range constructionPredeclaredInterfaceCases() {
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
			writeConstructionFixture(t, root, "pkg/owner/contracts.go", "package owner\n"+tc.declarations)
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

func constructionPredeclaredInterfaceCases() []struct {
	name, declarations, operation string
	want                          int
} {
	return []struct {
		name, declarations, operation string
		want                          int
	}{
		{"any embedding", `type Contract interface { any }; type View struct { *Service; Contract }`, `view.Lookup()`, 1},
		{"error embedding", `type Contract interface { error }; type View struct { *Service; Contract }`, `view.Lookup()`, 1},
		{"alias to any", `type Empty = any; type Contract interface { Empty }; type View struct { *Service; Contract }`, `view.Lookup()`, 1},
		{"alias to error", `type Failure = error; type Contract interface { Failure }; type View struct { *Service; Contract }`, `view.Lookup()`, 1},
		{"defined any", `type Empty any; type Contract interface { Empty }; type View struct { *Service; Contract }`, `view.Lookup()`, 1},
		{"defined error", `type Failure error; type Contract interface { Failure }; type View struct { *Service; Contract }`, `view.Lookup()`, 1},
		{"nested interface", `type Base interface { any; error }; type Contract interface { Base }; type View struct { *Service; Contract }`, `view.Lookup()`, 1},
		{"shallower interface", `type Contract interface { error }; type Inner struct { *Service }; type View struct { Inner; Contract }`, `view.Lookup()`, 1},
		{"method value", `type Contract interface { any }; type View struct { *Service; Contract }`, `lookup := view.Lookup; lookup()`, 1},
		{"method expression", `type Contract interface { error }; type View struct { *Service; Contract }`, `(*View).Lookup(view)`, 1},
		{"shadowed any unrelated", `type any interface { Observe() }; type Contract interface { any }; type View struct { *Service; Contract }`, `view.Lookup()`, 1},
		{"shadowed error unrelated", `type error interface { Observe() }; type Contract interface { error }; type View struct { *Service; Contract }`, `view.Lookup()`, 1},
		{"shadowed any collision", `type any interface { Lookup() Port }; type Contract interface { any }; type View struct { *Service; Contract }`, `view.Lookup()`, 0},
		{"shadowed error collision", `type error interface { Lookup() Port }; type Contract interface { error }; type View struct { *Service; Contract }`, `view.Lookup()`, 0},
		{"own selector competes", `type Contract interface { any; Lookup() Port }; type View struct { *Service; Contract }`, `view.Lookup()`, 0},
		{"unknown alongside any", `type Contract interface { any; Unknown }; type View struct { *Service; Contract }`, `view.Lookup()`, 0},
		{"cycle alongside error", `type Contract interface { error; Contract }; type View struct { *Service; Contract }`, `view.Lookup()`, 0},
	}
}
