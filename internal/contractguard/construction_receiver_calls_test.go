package contractguard

import (
	"bytes"
	"strings"
	"testing"
)

func TestConstructionImportedHelperReceiver(t *testing.T) {
	t.Parallel()
	for _, operation := range []string{
		`selected.Select(service).Lookup()`,
		`owner := selected.Select(service); owner.Lookup()`,
		`selectValue := selected.Select; selectValue(service).Lookup()`,
		`selected.SelectAlias(service).Lookup()`,
		`selected.Select(service).Next().Lookup()`,
		`owner, _ := selected.Pair(service); owner.Lookup()`,
		`_, owner := selected.Reverse(service); owner.Lookup()`,
		`var owner, _ = selected.Pair(service); owner.Lookup()`,
		`owner, _ := selected.Pair(service); alias := owner; alias.Lookup()`,
		`selectValue := selected.Pair; owner, _ := selectValue(service); owner.Lookup()`,
		`owner, _ := selected.PairAlias(service); owner.Lookup()`,
		`owner, _ := selected.Grouped(service); owner.Lookup()`,
		`owner, _ := selected.Select(service).Pair(); owner.Lookup()`,
		`owner, _ := selected.Pair(service); owner = service; owner.Lookup()`,
		`_, owner := selected.Reverse(service); owner = nil; owner.Lookup()`,
		`var owner, _ = selected.PairAlias(service); owner = service; owner.Lookup()`,
		`owner, _ := selected.Pair(service); alias := owner; alias = service; alias.Lookup()`,
		`owner := selected.Select(service); func() { owner = service }(); owner.Lookup()`,
	} {
		t.Run(operation, func(t *testing.T) {
			t.Parallel()
			root, registry := constructionFixture(t)
			registry.Types = append(registry.Types, ConstructionType{Symbol: ConstructionSymbol{ImportPath: fixtureOwner, Name: "Port"}, CapabilitySet: "owner", Kind: ConstructionBehavior})
			writeConstructionFixture(t, root, "pkg/owner/service.go", `package owner
type Port interface { Execute() }
type Service struct { port Port }
type Alias = Service
func New(port Port) (*Service, error) { return &Service{port: port}, nil }
func (s *Service) Lookup() Port { return s.port }
func Select(s *Service) *Service { return s }
func SelectAlias(s *Service) *Alias { return s }
func (s *Service) Next() *Service { return s }
func Pair(s *Service) (*Service, error) { return s, nil }
func Reverse(s *Service) (error, *Service) { return nil, s }
func PairAlias(s *Service) (*Alias, error) { return s, nil }
func Grouped(s *Service) (first, second *Service) { return s, s }
func (s *Service) Pair() (*Service, error) { return s, nil }
`)
			writeConstructionFixture(t, root, "pkg/consumer/consumer.go", `package consumer
import selected "example.test/factory/pkg/owner"
func Run(service *selected.Service) { `+operation+` }
`)
			for _, mode := range []ConstructionMode{ConstructionReport, ConstructionEnforce} {
				registry.CapabilitySets[0].Mode = mode
				findings, err := ScanConstruction(root, registry)
				blocking := 0
				if mode == ConstructionEnforce {
					blocking = 1
				}
				if err != nil || len(findings) != 1 || CountBlockingConstructionFindings(findings) != blocking {
					t.Fatalf("mode %s: findings = %+v, error = %v", mode, findings, err)
				}
				finding := findings[0]
				if finding.Caller != (ConstructionSymbol{ImportPath: "example.test/factory/pkg/consumer", Name: "Run"}) ||
					finding.Callee != (ConstructionSymbol{ImportPath: fixtureOwner, Receiver: "Service", Name: "Lookup"}) ||
					finding.FilePath != "pkg/consumer/consumer.go" || finding.Line != 3 || finding.Rule != "service-getter-locator" {
					t.Fatalf("unexpected qualified getter: %+v", finding)
				}
				var output bytes.Buffer
				WriteConstructionFindings(&output, findings)
				if strings.Contains(output.String(), "owner :=") || strings.Contains(output.String(), "service);") {
					t.Fatalf("diagnostic discloses source: %s", output.String())
				}
			}
		})
	}
}

func TestConstructionTupleReceiverPositions(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, helper, operation string
		want                    int
	}{
		{"local tuple", `func Pair(s *Service) (*Service, error) { return s, nil }`, `owner, _ := Pair(s); owner.Lookup()`, 1},
		{"parenthesized tuple", `func Pair(s *Service) (*Service, error) { return s, nil }`, `owner, _ := (Pair(s)); owner.Lookup()`, 1},
		{"named results", `func Pair(s *Service) (owner *Service, err error) { return s, nil }`, `owner, _ := Pair(s); owner.Lookup()`, 1},
		{"generic concrete", `func Pair[T any](s *Service, value T) (*Service, T) { return s, value }`, `owner, _ := Pair[int](s, 0); owner.Lookup()`, 1},
		{"reassigned tuple", `func Pair(s *Service) (*Service, error) { return s, nil }`, `owner, _ := Pair(s); owner = s; owner.Lookup()`, 1},
		{"reassigned reverse tuple", `func Pair(s *Service) (error, *Service) { return nil, s }`, `_, owner := Pair(s); owner = nil; owner.Lookup()`, 1},
		{"reassigned grouped declaration", `func Pair(s *Service) (*Service, error) { return s, nil }`, `var owner, _ = Pair(s); owner = s; owner.Lookup()`, 1},
		{"closure writes tuple", `func Pair(s *Service) (*Service, error) { return s, nil }`, `owner, _ := Pair(s); func() { owner = s }(); owner.Lookup()`, 1},
		{"range writes tuple", `func Pair(s *Service) (*Service, error) { return s, nil }`, `owner, _ := Pair(s); for _, owner = range []*Service{s} {}; owner.Lookup()`, 1},
		{"alias of reassigned tuple", `func Pair(s *Service) (*Service, error) { return s, nil }`, `owner, _ := Pair(s); owner = s; alias := owner; alias.Lookup()`, 1},
		{"reassigned tuple alias", `func Pair(s *Service) (*Service, error) { return s, nil }`, `owner, _ := Pair(s); alias := owner; alias = s; alias.Lookup()`, 1},
		{"reassigned single result", `func Select(s *Service) *Service { return s }`, `owner := Select(s); owner = s; owner.Lookup()`, 1},
		{"reassigned concrete generic tuple", `func Pair[T any](s *Service, value T) (*Service, T) { return s, value }`, `owner, _ := Pair[int](s, 0); owner = s; owner.Lookup()`, 1},
		{"reassigned unrelated result", `type Other struct{}; func (*Other) Lookup() Port { return nil }; func Pair(s *Service) (*Service, *Other) { return s, nil }`, `_, owner := Pair(s); owner = &Other{}; owner.Lookup()`, 0},
		{"shadow after tuple write", `type Other struct{}; func (*Other) Lookup() Port { return nil }; func Pair(s *Service) (*Service, error) { return s, nil }`, `owner, _ := Pair(s); owner = s; _ = owner; { owner := &Other{}; owner.Lookup() }`, 0},
		{"reassigned generic parameter", `type Other struct{}; func (*Other) Lookup() Port { return nil }; func Pair[Service any](s *Service) (*Service, error) { return s, nil }`, `owner, _ := Pair[Other](&Other{}); owner = &Other{}; owner.Lookup()`, 0},
		{"unrelated second result", `type Other struct{}; func (*Other) Lookup() Port { return nil }; func Pair(s *Service) (*Service, *Other) { return s, nil }`, `_, owner := Pair(s); owner.Lookup()`, 0},
		{"unrelated first result", `type Other struct{}; func (*Other) Lookup() Port { return nil }; func Pair(s *Service) (*Other, *Service) { return nil, s }`, `owner, _ := Pair(s); owner.Lookup()`, 0},
		{"shadowed owner", `type Other struct{}; func (*Other) Lookup() Port { return nil }; func Pair(s *Service) (*Service, error) { return s, nil }`, `owner, _ := Pair(s); _ = owner; { owner := &Other{}; owner.Lookup() }`, 0},
		{"generic parameter shadows owner", `type Other struct{}; func (*Other) Lookup() Port { return nil }; func Pair[Service any](s *Service) (*Service, error) { return s, nil }`, `owner, _ := Pair[Other](&Other{}); owner.Lookup()`, 0},
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
func (s *Service) Run() { `+tc.operation+` }
`+tc.helper)
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
					if finding.Rule != "service-getter-locator" || finding.Caller != (ConstructionSymbol{ImportPath: fixtureOwner, Receiver: "Service", Name: "Run"}) || finding.Callee != (ConstructionSymbol{ImportPath: fixtureOwner, Receiver: "Service", Name: "Lookup"}) || finding.FilePath != "pkg/owner/service.go" || finding.Line != 6 {
						t.Fatalf("unexpected getter diagnostic: %+v", finding)
					}
				}
				var output bytes.Buffer
				WriteConstructionFindings(&output, findings)
				if strings.Contains(output.String(), tc.operation) || strings.Contains(output.String(), tc.helper) {
					t.Fatalf("diagnostic discloses source: %s", output.String())
				}
			}
		})
	}
}
