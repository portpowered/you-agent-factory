package contractguard

import (
	"bytes"
	"strings"
	"testing"
)

func TestConstructionServiceGetterLocators(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, getter, operation, extra string
		kind                           ConstructionKind
		want                           string
	}{
		{"direct", `func (s *Service) Lookup() Port { return s.port }`, `s.Lookup().Execute()`, "", ConstructionBehavior, "service-getter-locator"},
		{"peer and error", `func (s *Service) Lookup() (Port, error) { return s.port, nil }`, `_, _ = s.Lookup()`, "", ConstructionBehavior, "service-getter-locator"},
		{"peer after error", `func (s *Service) Lookup() (error, Port) { return nil, s.port }`, `_, _ = s.Lookup()`, "", ConstructionBehavior, "service-getter-locator"},
		{"required origin in domain slot only", `func (s *Service) Lookup() (Port, any) { return s.optional, s.port }`, `_, _ = s.Lookup()`, "", ConstructionBehavior, ""},
		{"named explicit return", `func (s *Service) Lookup() (result Port) { return s.port }`, `_ = s.Lookup()`, "", ConstructionBehavior, "service-getter-locator"},
		{"named naked return debt", `func (s *Service) Lookup() (result Port) { result = s.port; return }`, `_ = s.Lookup()`, "", ConstructionBehavior, "unresolved-service-getter-locator"},
		{"named explicit variable debt", `func (s *Service) Lookup() (result Port) { result = s.port; return result }`, `_ = s.Lookup()`, "", ConstructionBehavior, "unresolved-service-getter-locator"},
		{"named parenthesized return debt", `func (s *Service) Lookup() (result Port) { result = s.port; return (result) }`, `_ = s.Lookup()`, "", ConstructionBehavior, "unresolved-service-getter-locator"},
		{"named alias return debt", `func (s *Service) Lookup() (result Port) { result = s.port; alias := result; return alias }`, `_ = s.Lookup()`, "", ConstructionBehavior, "unresolved-service-getter-locator"},
		{"named alias chain debt", `func (s *Service) Lookup() (result Port) { result = s.port; alias := result; second := alias; return second }`, `_ = s.Lookup()`, "", ConstructionBehavior, "unresolved-service-getter-locator"},
		{"named declared alias debt", `func (s *Service) Lookup() (result Port) { result = s.port; var alias Port = result; return alias }`, `_ = s.Lookup()`, "", ConstructionBehavior, "unresolved-service-getter-locator"},
		{"named grouped aliases debt", `func (s *Service) Lookup() (result Port) { result = s.port; var other, alias Port = s.optional, result; _ = other; return alias }`, `_ = s.Lookup()`, "", ConstructionBehavior, "unresolved-service-getter-locator"},
		{"named later alias assignment debt", `func (s *Service) Lookup() (result Port) { result = s.port; alias := s.optional; alias = result; return alias }`, `_ = s.Lookup()`, "", ConstructionBehavior, "unresolved-service-getter-locator"},
		{"named helper alias debt", `func (s *Service) Lookup() (result Port) { result = s.port; alias := identity(result); return alias }`, `_ = s.Lookup()`, `func identity(port Port) Port { return port }`, ConstructionBehavior, "unresolved-service-getter-locator"},
		{"named cross-result chain debt", `func (s *Service) Lookup() (first, second Port) { alias := second; first = alias; second = s.port; return first, s.optional }`, `_, _ = s.Lookup()`, "", ConstructionBehavior, "unresolved-service-getter-locator"},
		{"named closure alias debt", `func (s *Service) Lookup() (result Port) { alias := s.optional; func() { result = s.port; alias = result }(); return alias }`, `_ = s.Lookup()`, "", ConstructionBehavior, "unresolved-service-getter-locator"},
		{"named cyclic aliases debt", `func (s *Service) Lookup() (result Port) { result = s.port; first := result; second := first; first = second; return second }`, `_ = s.Lookup()`, "", ConstructionBehavior, "unresolved-service-getter-locator"},
		{"optional named alias exclusion", `func (s *Service) Lookup() (result Port) { result = s.optional; alias := result; return alias }`, `_ = s.Lookup()`, "", ConstructionBehavior, ""},
		{"shadowed named alias exclusion", `func (s *Service) Lookup() (result Port) { { result := s.port; _ = result }; alias := result; return alias }`, `_ = s.Lookup()`, "", ConstructionBehavior, ""},
		{"named domain alias exclusion", `func (s *Service) Lookup() (result Port, data any) { data = s.port; alias := result; return alias, data }`, `_, _ = s.Lookup()`, "", ConstructionBehavior, ""},
		{"unused named dependency exclusion", `func (s *Service) Lookup() (result Port) { result = s.port; alias := s.optional; return alias }`, `_ = s.Lookup()`, "", ConstructionBehavior, ""},
		{"grouped named result debt", `func (s *Service) Lookup() (first, second Port) { second = s.port; return }`, `_, _ = s.Lookup()`, "", ConstructionBehavior, "unresolved-service-getter-locator"},
		{"named parallel assignment debt", `func (s *Service) Lookup() (err error, result Port) { err, result = nil, s.port; return }`, `_, _ = s.Lookup()`, "", ConstructionBehavior, "unresolved-service-getter-locator"},
		{"named closure write debt", `func (s *Service) Lookup() (result Port) { func() { result = s.port }(); return }`, `_ = s.Lookup()`, "", ConstructionBehavior, "unresolved-service-getter-locator"},
		{"named shadow exclusion", `func (s *Service) Lookup() (result Port) { { result := s.port; _ = result }; return }`, `_ = s.Lookup()`, "", ConstructionBehavior, ""},
		{"named optional result exclusion", `func (s *Service) Lookup() (result Port) { result = s.optional; return }`, `_ = s.Lookup()`, "", ConstructionBehavior, ""},
		{"named zero result exclusion", `func (s *Service) Lookup() (result Port) { return }`, `_ = s.Lookup()`, "", ConstructionBehavior, ""},
		{"tuple helper debt", `func (s *Service) Lookup() (Port, error) { return pair(s.port) }`, `_, _ = s.Lookup()`, `func pair(port Port) (Port, error) { return port, nil }`, ConstructionBehavior, "unresolved-service-getter-locator"},
		{"named tuple assignment debt", `func (s *Service) Lookup() (result Port, err error) { result, err = pair(s.port); return }`, `_, _ = s.Lookup()`, `func pair(port Port) (Port, error) { return port, nil }`, ConstructionBehavior, "unresolved-service-getter-locator"},
		{"named domain slot exclusion", `func (s *Service) Lookup() (result Port, data any) { data = s.port; return }`, `_, _ = s.Lookup()`, "", ConstructionBehavior, ""},
		{"named conflicting writes debt", `func (s *Service) Lookup() (result Port) { result = s.port; result = s.optional; return }`, `_ = s.Lookup()`, "", ConstructionBehavior, "unresolved-service-getter-locator"},
		{"multiple domain results exclusion", `func (s *Service) Lookup() (Port, error) { return s.port, nil }`, `_, _ = s.Lookup()`, "", ConstructionDomain, ""},
		{"multiple optional results exclusion", `func (s *Service) Lookup() (Port, error) { return s.optional, nil }`, `_, _ = s.Lookup()`, "", ConstructionBehavior, ""},
		{"effect", `func (s *Service) Lookup() Port { return s.port }`, `s.Lookup().Execute()`, "", ConstructionEffect, "service-getter-locator"},
		{"renamed field alias", `func (s *Service) Lookup() Port { renamed := s.port; return renamed }`, `s.Lookup().Execute()`, "", ConstructionBehavior, "service-getter-locator"},
		{"receiver alias", `func (s *Service) Lookup() Port { owner := s; return owner.port }`, `s.Lookup().Execute()`, "", ConstructionBehavior, "service-getter-locator"},
		{"caller alias", `func (s *Service) Lookup() Port { return s.port }`, `owner := s; owner.Lookup().Execute()`, "", ConstructionBehavior, "service-getter-locator"},
		{"helper receiver", `func (s *Service) Lookup() Port { return s.port }`, `selectOwner(s).Lookup().Execute()`, `func selectOwner(s *Service) *Service { return s }`, ConstructionBehavior, "service-getter-locator"},
		{"helper receiver alias", `func (s *Service) Lookup() Port { return s.port }`, `owner := selectOwner(s); owner.Lookup().Execute()`, `func selectOwner(s *Service) *Service { return s }`, ConstructionBehavior, "service-getter-locator"},
		{"helper value receiver", `func (s *Service) Lookup() Port { return s.port }`, `selectValue := selectOwner; selectValue(s).Lookup().Execute()`, `func selectOwner(s *Service) *Service { return s }`, ConstructionBehavior, "service-getter-locator"},
		{"helper named receiver result", `func (s *Service) Lookup() Port { return s.port }`, `selectOwner(s).Lookup().Execute()`, `func selectOwner(s *Service) (owner *Service) { return s }`, ConstructionBehavior, "service-getter-locator"},
		{"helper declared receiver alias", `type Alias = Service; func (s *Service) Lookup() Port { return s.port }`, `selectOwner(s).Lookup().Execute()`, `func selectOwner(s *Service) *Alias { return s }`, ConstructionBehavior, "service-getter-locator"},
		{"helper escaped getter", `func (s *Service) Lookup() Port { return s.port }`, `consume(selectOwner(s).Lookup)`, `func selectOwner(s *Service) *Service { return s }; func consume(func() Port) {}`, ConstructionBehavior, "unresolved-service-getter-reference"},
		{"helper unrelated receiver", `func (s *Service) Lookup() Port { return s.port }`, `selectOwner().Lookup().Execute()`, `type Other struct{}; func (*Other) Lookup() Port { return nil }; func selectOwner() *Other { return nil }`, ConstructionBehavior, ""},
		{"shadowed helper binding", `func (s *Service) Lookup() Port { return s.port }`, `selectValue := selectOwner; selectOwner := selectValue; _ = selectOwner; selectValue(s).Lookup().Execute()`, `func selectOwner(s *Service) *Service { return s }`, ConstructionBehavior, "service-getter-locator"},
		{"generic concrete helper result", `func (s *Service) Lookup() Port { return s.port }`, `selectOwner[int](s).Lookup().Execute()`, `func selectOwner[T any](s *Service) *Service { return s }`, ConstructionBehavior, "service-getter-locator"},
		{"helper receiver method chain", `func (s *Service) Lookup() Port { return s.port }`, `selectOwner(s).Next().Lookup().Execute()`, `func selectOwner(s *Service) *Service { return s }; func (s *Service) Next() *Service { return s }`, ConstructionBehavior, "service-getter-locator"},
		{"method value", `func (s *Service) Lookup() Port { return s.port }`, `lookup := s.Lookup; lookup().Execute()`, "", ConstructionBehavior, "service-getter-locator"},
		{"method value chain", `func (s *Service) Lookup() Port { return s.port }`, `lookup := s.Lookup; alias := lookup; alias().Execute()`, "", ConstructionBehavior, "service-getter-locator"},
		{"method expression", `func (s *Service) Lookup() Port { return s.port }`, `(*Service).Lookup(s).Execute()`, "", ConstructionBehavior, "service-getter-locator"},
		{"deferred", `func (s *Service) Lookup() Port { return s.port }`, `defer s.Lookup()`, "", ConstructionBehavior, "service-getter-locator"},
		{"closure call", `func (s *Service) Lookup() Port { return s.port }`, `func() { s.Lookup().Execute() }()`, "", ConstructionBehavior, "service-getter-locator"},
		{"escaped method value", `func (s *Service) Lookup() Port { return s.port }`, `consume(s.Lookup)`, `func consume(func() Port) {}`, ConstructionBehavior, "unresolved-service-getter-reference"},
		{"unused method value", `func (s *Service) Lookup() Port { return s.port }`, `_ = s.Lookup`, "", ConstructionBehavior, "unresolved-service-getter-reference"},
		{"reassigned method value", `func (s *Service) Lookup() Port { return s.port }`, `lookup := s.Lookup; lookup = func() Port { return nil }; lookup()`, "", ConstructionBehavior, "unresolved-service-getter-reference"},
		{"local type alias", `type Alias = Service; func (s *Service) Lookup() Port { return s.port }`, `var owner *Alias; owner.Lookup().Execute()`, "", ConstructionBehavior, "service-getter-locator"},
		{"result alias", `type Alias = Port; func (s *Service) Lookup() Alias { return s.port }`, `s.Lookup().Execute()`, "", ConstructionBehavior, "service-getter-locator"},
		{"helper return", `func (s *Service) Lookup() Port { return identity(s.port) }`, `s.Lookup().Execute()`, `func identity(port Port) Port { return port }`, ConstructionBehavior, "service-getter-locator"},
		{"immutable helper value", `func (s *Service) Lookup() Port { helper := identity; return helper(s.port) }`, `s.Lookup().Execute()`, `func identity(port Port) Port { return port }`, ConstructionBehavior, "service-getter-locator"},
		{"ambiguous helper return", `func (s *Service) Lookup() Port { return identity(s.port) }`, `s.Lookup().Execute()`, `func identity(port Port) Port { if flag { return port }; return nil }; var flag bool`, ConstructionBehavior, "unresolved-service-getter-locator"},
		{"recursive helper return", `func (s *Service) Lookup() Port { return identity(s.port) }`, `s.Lookup().Execute()`, `func identity(port Port) Port { return identity(port) }`, ConstructionBehavior, "unresolved-service-getter-locator"},
		{"field reassignment", `func (s *Service) Lookup() Port { s.port = nil; return s.port }`, `s.Lookup().Execute()`, "", ConstructionBehavior, "unresolved-service-getter-locator"},
		{"helper domain return", `func (s *Service) Lookup() Port { return identity(s.port) }`, `_ = s.Lookup()`, `func identity(port Port) Port { return nil }`, ConstructionBehavior, ""},
		{"domain result", `func (s *Service) Lookup() Port { return s.port }`, `s.Lookup().Execute()`, "", ConstructionDomain, ""},
		{"state result", `func (s *Service) Lookup() Port { return s.port }`, `s.Lookup().Execute()`, "", ConstructionState, ""},
		{"resource result", `func (s *Service) Lookup() Port { return s.port }`, `s.Lookup().Execute()`, "", ConstructionResource, ""},
		{"parameterized view", `func (s *Service) Lookup(scope string) Port { return s.port }`, `s.Lookup("scope").Execute()`, "", ConstructionEffect, ""},
		{"ordinary error outcome", `func (s *Service) Lookup() error { return nil }`, `_ = s.Lookup()`, "", ConstructionBehavior, ""},
		{"optional field", `func (s *Service) Lookup() Port { return s.optional }`, `s.Lookup().Execute()`, "", ConstructionBehavior, ""},
		{"unrelated receiver", `func (other *Other) Lookup() Port { return other.port }`, `var other *Other; other.Lookup().Execute()`, `type Other struct { port Port }`, ConstructionBehavior, ""},
		{"defined type does not inherit method", `type Other Service; func (s *Service) Lookup() Port { return s.port }`, `var other *Other; _ = other`, "", ConstructionBehavior, ""},
		{"nested return is not getter return", `func (s *Service) Lookup() Port { _ = func() Port { return s.port }; return nil }`, `_ = s.Lookup()`, "", ConstructionBehavior, ""},
		{"shadowed method value", `func (s *Service) Lookup() Port { return s.port }`, `lookup := func() Port { return nil }; lookup()`, "", ConstructionBehavior, ""},
		{"unregistered result", `type Payload struct{}; func (s *Service) Lookup() *Payload { return nil }`, `_ = s.Lookup()`, "", ConstructionBehavior, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root, registry := constructionFixture(t)
			registry.CapabilitySets[0].Mode = ConstructionEnforce
			registry.Types = append(registry.Types, ConstructionType{Symbol: ConstructionSymbol{ImportPath: fixtureOwner, Name: "Port"}, CapabilitySet: "owner", Kind: tc.kind})
			writeConstructionFixture(t, root, "pkg/owner/service.go", `package owner
type Port interface { Execute() }
type Service struct { port Port; optional Port }
func New(secretInput Port) (*Service, error) { return &Service{port: secretInput}, nil }
`+tc.getter+`
func (s *Service) Run() { `+tc.operation+` }
`+tc.extra+"\n")
			findings, err := ScanConstruction(root, registry)
			want := 0
			if tc.want != "" {
				want = 1
			}
			if err != nil || len(findings) != want || CountBlockingConstructionFindings(findings) != want {
				t.Fatalf("findings = %+v, error = %v, want %d", findings, err, want)
			}
			for _, finding := range findings {
				if finding.Rule != tc.want || finding.Caller != (ConstructionSymbol{ImportPath: fixtureOwner, Receiver: "Service", Name: "Run"}) ||
					finding.Callee != (ConstructionSymbol{ImportPath: fixtureOwner, Receiver: "Service", Name: "Lookup"}) || finding.FilePath != "pkg/owner/service.go" || finding.Line != 6 {
					t.Fatalf("unexpected getter diagnostic: %+v", finding)
				}
			}
			var output bytes.Buffer
			WriteConstructionFindings(&output, findings)
			if strings.Contains(output.String(), "secretInput") || strings.Contains(output.String(), "s.port") || strings.Contains(output.String(), "renamed") {
				t.Fatalf("diagnostic discloses source: %s", output.String())
			}
			registry.CapabilitySets[0].Mode = ConstructionReport
			findings, err = ScanConstruction(root, registry)
			if err != nil || len(findings) != want || CountBlockingConstructionFindings(findings) != 0 {
				t.Fatalf("report findings = %+v, error = %v", findings, err)
			}
		})
	}
}

func TestConstructionImportedServiceGetter(t *testing.T) {
	t.Parallel()
	root, registry := constructionFixture(t)
	registry.CapabilitySets[0].Mode = ConstructionEnforce
	registry.Types = append(registry.Types, ConstructionType{Symbol: ConstructionSymbol{ImportPath: fixtureOwner, Name: "Port"}, CapabilitySet: "owner", Kind: ConstructionBehavior})
	writeConstructionFixture(t, root, "pkg/owner/service.go", `package owner
type Port interface { Execute() }
type Service struct { port Port }
func New(port Port) (*Service, error) { return &Service{port: port}, nil }
func (s *Service) Lookup() Port { return s.port }
`)
	writeConstructionFixture(t, root, "pkg/consumer/consumer.go", `package consumer
import selected "example.test/factory/pkg/owner"
type Alias = selected.Service
func Run(service *Alias) { service.Lookup().Execute() }
`)
	findings, err := ScanConstruction(root, registry)
	if err != nil || len(findings) != 1 || CountBlockingConstructionFindings(findings) != 1 {
		t.Fatalf("findings = %+v, error = %v", findings, err)
	}
	finding := findings[0]
	if finding.Caller != (ConstructionSymbol{ImportPath: "example.test/factory/pkg/consumer", Name: "Run"}) ||
		finding.Callee != (ConstructionSymbol{ImportPath: fixtureOwner, Receiver: "Service", Name: "Lookup"}) ||
		finding.FilePath != "pkg/consumer/consumer.go" || finding.Line != 4 || finding.Rule != "service-getter-locator" {
		t.Fatalf("unexpected qualified getter: %+v", finding)
	}
}
