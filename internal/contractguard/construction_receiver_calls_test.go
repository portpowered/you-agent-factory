package contractguard

import "testing"

func TestConstructionImportedHelperReceiver(t *testing.T) {
	t.Parallel()
	for _, operation := range []string{
		`selected.Select(service).Lookup()`,
		`owner := selected.Select(service); owner.Lookup()`,
		`selectValue := selected.Select; selectValue(service).Lookup()`,
		`selected.SelectAlias(service).Lookup()`,
		`selected.Select(service).Next().Lookup()`,
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
			}
		})
	}
}
