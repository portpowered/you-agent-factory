package contractguard

import (
	"bytes"
	"strings"
	"testing"
)

func TestConstructionPointerGetterReceivers(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, operation string
		want            int
	}{
		{"dereference", `(*s).Lookup()`, 1},
		{"parenthesized dereference", `((*(s))).Lookup()`, 1},
		{"address of dereference", `(&*s).Lookup()`, 1},
		{"helper dereference", `(*Select(s)).Lookup()`, 1},
		{"helper address", `(&*Select(s)).Lookup()`, 1},
		{"dereferenced alias", `owner := *Select(s); owner.Lookup()`, 1},
		{"addressed alias", `owner := *Select(s); (&owner).Lookup()`, 1},
		{"tuple dereference", `owner, _ := Pair(s); (*owner).Lookup()`, 1},
		{"mutated tuple dereference", `owner, _ := Pair(s); owner = nil; (*owner).Lookup()`, 1},
		{"method value", `lookup := (*Select(s)).Lookup; lookup()`, 1},
		{"deferred getter", `defer (*Select(s)).Lookup()`, 1},
		{"closure getter", `func() { (*Select(s)).Lookup() }()`, 1},
		{"unrelated dereference", `(*other).Lookup()`, 0},
		{"unrelated helper", `(*SelectOther(other)).Lookup()`, 0},
		{"unrelated addressed alias", `owner := *SelectOther(other); (&owner).Lookup()`, 0},
		{"shadow receiver", `_ = s; { s := other; (*s).Lookup() }`, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root, registry := constructionFixture(t)
			registry.Types = append(registry.Types, ConstructionType{Symbol: ConstructionSymbol{ImportPath: fixtureOwner, Name: "Port"}, CapabilitySet: "owner", Kind: ConstructionBehavior})
			writeConstructionFixture(t, root, "pkg/owner/service.go", `package owner
type Port interface { Execute() }
type Service struct { port Port }
type Other struct{}
func New(p Port) (*Service, error) { return &Service{port: p}, nil }
func (s *Service) Lookup() Port { return s.port }
func (*Other) Lookup() Port { return nil }
func Select(s *Service) *Service { return s }
func SelectOther(s *Other) *Other { return s }
func Pair(s *Service) (*Service, error) { return s, nil }
func Run(s *Service, other *Other) { `+tc.operation+` }
`)
			for _, mode := range []ConstructionMode{ConstructionReport, ConstructionEnforce} {
				registry.CapabilitySets[0].Mode = mode
				findings, err := ScanConstruction(root, registry)
				if err != nil || len(findings) != tc.want {
					t.Fatalf("mode %s: findings = %+v, error = %v, want %d", mode, findings, err, tc.want)
				}
				blocking := 0
				if mode == ConstructionEnforce {
					blocking = tc.want
				}
				if CountBlockingConstructionFindings(findings) != blocking {
					t.Fatalf("unexpected blocking findings: %+v", findings)
				}
				for _, finding := range findings {
					if finding.Caller != (ConstructionSymbol{ImportPath: fixtureOwner, Name: "Run"}) || finding.Callee != (ConstructionSymbol{ImportPath: fixtureOwner, Receiver: "Service", Name: "Lookup"}) || finding.FilePath != "pkg/owner/service.go" || finding.Line != 11 || finding.Rule != "service-getter-locator" {
						t.Fatalf("unexpected qualified getter: %+v", finding)
					}
				}
				var output bytes.Buffer
				WriteConstructionFindings(&output, findings)
				if strings.Contains(output.String(), tc.operation) {
					t.Fatalf("source disclosed: %s", output.String())
				}
			}
		})
	}
}
