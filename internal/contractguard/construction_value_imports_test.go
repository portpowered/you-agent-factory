package contractguard

import "testing"

func TestConstructionImportedValuesAndExactAllowances(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, body string
		allowed    bool
	}{
		{"import alias", `alias := selected.New; alias(p)`, false},
		{"import alias shadow", `alias := selected.New; selected := alias; selected(p)`, false},
		{"provider alias", `alias := selected.New; alias(p)`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root, registry := constructionFixture(t)
			writeConstructionFixture(t, root, "pkg/owner/service.go", `package owner
type Port interface { Execute() }
type Service struct { port Port }
func New(p Port) (*Service, error) { return &Service{port: p}, nil }
`)
			writeConstructionFixture(t, root, "pkg/wire/provider.go", `package wire
import selected "example.test/factory/pkg/owner"
func Provide(p selected.Port) { `+tc.body+` }
`)
			registry.CapabilitySets[0].Mode = ConstructionEnforce
			if !tc.allowed {
				registry.Allowances = nil
			}
			findings, err := ScanConstruction(root, registry)
			if err != nil {
				t.Fatal(err)
			}
			if tc.allowed {
				if len(findings) != 0 {
					t.Fatalf("exact provider alias = %+v", findings)
				}
			} else if len(findings) != 1 || findings[0].Rule != "registered-construction" || findings[0].Callee != registry.Constructors[0].Symbol || CountBlockingConstructionFindings(findings) != 1 {
				t.Fatalf("qualified imported value = %+v", findings)
			}
		})
	}
}

func TestConstructionInstantiatedValue(t *testing.T) {
	t.Parallel()
	root, registry := constructionFixture(t)
	writeConstructionFixture(t, root, "pkg/owner/service.go", `package owner
type Port interface { Execute() }
type Service struct { port Port }
func New[T any](p Port) (*Service, error) { return &Service{port: p}, nil }
func Run(p Port) { first := New[string]; second := first; second(p) }
`)
	// Generic constructors need explicit arguments in the focused provider too.
	writeConstructionFixture(t, root, "pkg/wire/provider.go", `package wire
import selected "example.test/factory/pkg/owner"
func Provide(p selected.Port) { selected.New[string](p) }
`)
	registry.CapabilitySets[0].Mode = ConstructionEnforce
	findings, err := ScanConstruction(root, registry)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || findings[0].Rule != "registered-construction" || findings[0].Callee != registry.Constructors[0].Symbol || findings[0].Caller.Name != "Run" || CountBlockingConstructionFindings(findings) != 1 {
		t.Fatalf("instantiated constructor value = %+v", findings)
	}
}
