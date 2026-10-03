package contractguard

import "testing"

func TestConstructionPromotedGetter(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, declarations, operation string
		want                          int
	}{
		{"embedded pointer", `type View struct { *Service }`, `view.Lookup()`, 1},
		{"embedded value", `type View struct { Service }`, `view.Lookup()`, 1},
		{"alias embedding", `type Alias = Service; type View struct { *Alias }`, `view.Lookup()`, 1},
		{"nested embedding", `type Inner struct { *Service }; type View struct { Inner }`, `view.Lookup()`, 1},
		{"defined struct", `type Inner struct { *Service }; type View Inner`, `view.Lookup()`, 1},
		{"method expression", `type View struct { *Service }`, `(*View).Lookup(view)`, 1},
		{"method value", `type View struct { *Service }`, `lookup := view.Lookup; lookup()`, 1},
		{"deferred call", `type View struct { *Service }`, `defer view.Lookup()`, 1},
		{"closure call", `type View struct { *Service }`, `func() { view.Lookup() }()`, 1},
		{"receiver alias", `type View struct { *Service }`, `alias := view; alias.Lookup()`, 1},
		{"helper result", `type View struct { *Service }; func Select(v *View) *View { return v }`, `Select(view).Lookup()`, 1},
		{"shallower method", `type Inner struct { *Other }; type View struct { *Service; Inner }; type Other struct {}; func (*Other) Lookup() Port { return nil }`, `view.Lookup()`, 1},
		{"shallower method before opaque embedding", `type Generic[T any] struct { value T }; type Inner struct { Generic[int] }; type View struct { *Service; Inner }`, `view.Lookup()`, 1},
		{"own method before opaque embedding", `type Generic[T any] struct { value T }; type View struct { Generic[int] }; func (v *View) Lookup() Port { return nil }`, `view.Lookup()`, 0},
		{"scalar sibling", `type Scalar int; type View struct { *Service; Scalar }`, `view.Lookup()`, 1},
		{"named function sibling", `type Callback func(); type View struct { *Service; Callback }`, `view.Lookup()`, 1},
		{"own method shadows", `type View struct { *Service }; func (*View) Lookup() Port { return nil }`, `view.Lookup()`, 0},
		{"field shadows", `type View struct { *Service; Lookup func() Port }`, `view.Lookup()`, 0},
		{"shallower field", `type Inner struct { *Service }; type View struct { Inner; Lookup func() Port }`, `view.Lookup()`, 0},
		{"ambiguous methods", `type Other struct {}; func (*Other) Lookup() Port { return nil }; type View struct { *Service; *Other }`, `view.Lookup()`, 0},
		{"diamond ambiguity", `type Left struct { *Service }; type Right struct { *Service }; type View struct { Left; Right }`, `view.Lookup()`, 0},
		{"same depth field collision", `type Other struct { Lookup func() Port }; type View struct { *Service; Other }`, `view.Lookup()`, 0},
		{"defined service does not inherit methods", `type Copy Service; type View struct { Copy }`, `view.Lookup()`, 0},
		{"recursive embedding terminates", `type View struct { *View }`, `view.Lookup()`, 0},
		{"recursive sibling", `type View struct { *View; *Service }`, `view.Lookup()`, 1},
		{"unrelated receiver shadow", `type View struct { *Service }; type Other struct {}; func (*Other) Lookup() Port { return nil }`, `{ view := &Other{}; view.Lookup() }`, 0},
	} {
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
`+tc.declarations)
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

func TestConstructionImportedPromotedGetter(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, declarations, operation string
		want                          int
	}{
		{"imported pointer", `type View struct { *selected.Service }`, `view.Lookup()`, 1},
		{"imported alias", `type Alias = selected.Service; type View struct { *Alias }`, `view.Lookup()`, 1},
		{"imported wrapper", `type View selected.View`, `view.Lookup()`, 1},
		{"imported wrapper alias", `type View = selected.View`, `view.Lookup()`, 1},
		{"imported method expression", `type View struct { *selected.Service }`, `(*View).Lookup(view)`, 1},
		{"imported method value", `type View struct { *selected.Service }`, `lookup := view.Lookup; lookup()`, 1},
		{"imported defined service", `type Copy selected.Service; type View struct { Copy }`, `view.Lookup()`, 0},
		{"imported private method", `type View struct { *selected.Service }`, `view.private()`, 0},
		{"imported shadow", `type View struct { *selected.Service }; func (*View) Lookup() selected.Port { return nil }`, `view.Lookup()`, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root, registry := constructionFixture(t)
			registry.Types = append(registry.Types, ConstructionType{Symbol: ConstructionSymbol{ImportPath: fixtureOwner, Name: "Port"}, CapabilitySet: "owner", Kind: ConstructionBehavior})
			writeConstructionFixture(t, root, "pkg/owner/service.go", `package owner
type Port interface { Execute() }
type Service struct { port Port }
type View struct { *Service }
func New(port Port) (*Service, error) { return &Service{port: port}, nil }
func (s *Service) Lookup() Port { return s.port }
func (s *Service) private() Port { return s.port }
`)
			writeConstructionFixture(t, root, "pkg/consumer/consumer.go", `package consumer
import selected "example.test/factory/pkg/owner"
func Run(view *View) { `+tc.operation+` }
`+tc.declarations)
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
					if finding.Rule != "service-getter-locator" || finding.Caller != (ConstructionSymbol{ImportPath: "example.test/factory/pkg/consumer", Name: "Run"}) || finding.Callee != (ConstructionSymbol{ImportPath: fixtureOwner, Receiver: "Service", Name: "Lookup"}) || finding.FilePath != "pkg/consumer/consumer.go" || finding.Line != 3 {
						t.Fatalf("unexpected imported promoted getter: %+v", finding)
					}
				}
			}
		})
	}
}
