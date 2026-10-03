package contractguard

import "testing"

func TestConstructionRequiredAssertionGuards(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, operation, rule string
	}{
		{"failed assertion", `_, ok := s.port.(Port); if !ok {}`, "required-dependency-assertion-guard"},
		{"successful assertion", `p, ok := s.port.(Port); if ok { p.Execute() }`, "required-dependency-assertion-guard"},
		{"status equal false", `_, ok := s.port.(Port); if ok == false {}`, "required-dependency-assertion-guard"},
		{"reversed true", `_, ok := s.port.(Port); if true != ok {}`, "required-dependency-assertion-guard"},
		{"alias closure", `_, ok := s.port.(Port); alias := ok; defer func() { if !alias {} }()`, "required-dependency-assertion-guard"},
		{"declared status alias", `_, ok := s.port.(Port); var alias = ok; if alias {}`, "required-dependency-assertion-guard"},
		{"compound", `_, ok := s.port.(Port); if true && !ok {}`, "required-dependency-assertion-guard"},
		{"for guard", `_, ok := s.port.(Port); for ok { break }`, "required-dependency-assertion-guard"},
		{"status reassignment", `_, ok := s.port.(Port); ok = false; if !ok {}`, "unresolved-required-dependency-guard"},
		{"field reassignment", `s.port = nil; _, ok := s.port.(Port); if !ok {}`, "unresolved-required-dependency-guard"},
		{"status observation", `_, ok := s.port.(Port); _ = ok`, ""},
		{"optional assertion", `_, ok := s.payload.(Port); if !ok {}`, ""},
		{"optional boolean", `ok := true; if !ok {}`, ""},
		{"shadowed status", `_, ok := s.port.(Port); _ = ok; { ok := false; if !ok {} }`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root, registry := constructionFixture(t)
			registry.CapabilitySets[0].Mode = ConstructionEnforce
			writeConstructionFixture(t, root, "pkg/owner/service.go", `package owner
type Port interface { Execute() }
type Service struct { port Port; payload any }
func New(renamed Port) (*Service, error) { return &Service{port: renamed}, nil }
func (s *Service) Run() { `+tc.operation+` }
`)
			assertConstructionGuardRule(t, root, registry, tc.rule)
		})
	}
}

func TestConstructionRequiredReceiverGuards(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, operation, rule string }{
		{"receiver", `if s == nil {}`, "required-receiver-guard"},
		{"receiver alias", `self := s; if self != nil {}`, "required-receiver-guard"},
		{"receiver alias field", `self := s; if self.port == nil {}`, "required-dependency-guard"},
		{"receiver alias mutation", `self := s; self.port = nil; if s.port == nil {}`, "unresolved-required-dependency-guard"},
		{"receiver mutation", `s = nil; if s == nil {}`, "unresolved-required-dependency-guard"},
		{"alias replaced", `self := s; self = nil; if self.port == nil {}`, "unresolved-required-dependency-guard"},
		{"domain receiver", `domain := (*Domain)(nil); if domain == nil {}`, ""},
		{"shadow receiver", `{ s := (*Domain)(nil); if s == nil {} }`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root, registry := constructionFixture(t)
			registry.CapabilitySets[0].Mode = ConstructionEnforce
			writeConstructionFixture(t, root, "pkg/owner/service.go", `package owner
type Port interface { Execute() }
type Service struct { port Port }
type Domain struct {}
func New(renamed Port) (*Service, error) { return &Service{port: renamed}, nil }
func (s *Service) Run() { `+tc.operation+` }
func (d *Domain) Validate() { if d == nil {} }
`)
			assertConstructionGuardRule(t, root, registry, tc.rule)
		})
	}
}

func TestConstructionReceiverHelperGuard(t *testing.T) {
	t.Parallel()
	root, registry := constructionFixture(t)
	registry.CapabilitySets[0].Mode = ConstructionEnforce
	writeConstructionFixture(t, root, "pkg/owner/service.go", `package owner
type Port interface { Execute() }
type Service struct { port Port }
func New(renamed Port) (*Service, error) { return &Service{port: renamed}, nil }
func (s *Service) Run() { check(s) }
func check(owner *Service) { if owner == nil {} }
`)
	findings, err := ScanConstruction(root, registry)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || findings[0].Rule != "required-receiver-guard" || findings[0].Caller.Name != "check" || CountBlockingConstructionFindings(findings) != 1 {
		t.Fatalf("receiver helper = %+v, want one qualified blocking guard", findings)
	}
}

func TestConstructionScopedStateReceiverIsOptional(t *testing.T) {
	t.Parallel()
	root, registry := constructionFixture(t)
	registry.CapabilitySets[0].Mode = ConstructionEnforce
	registry.Types[0].Kind = ConstructionState
	writeConstructionFixture(t, root, "pkg/owner/service.go", `package owner
type Port interface { Execute() }
type Service struct { port Port }
func New(renamed Port) (*Service, error) { return &Service{port: renamed}, nil }
func (s *Service) Run() { if s == nil {}; check(s) }
func check(state *Service) { if state == nil {} }
`)
	findings, err := ScanConstruction(root, registry)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("scoped state receiver guards = %+v, want none", findings)
	}
}

func assertConstructionGuardRule(t *testing.T, root string, registry ConstructionRegistry, rule string) {
	t.Helper()
	findings, err := ScanConstruction(root, registry)
	if err != nil {
		t.Fatal(err)
	}
	want := 0
	if rule != "" {
		want = 1
	}
	if len(findings) != want || CountBlockingConstructionFindings(findings) != want {
		t.Fatalf("findings = %+v, want %d blocking observations", findings, want)
	}
	if want == 1 && (findings[0].Rule != rule || findings[0].Caller.Receiver != "Service" || findings[0].Caller.Name != "Run" || findings[0].Callee != registry.Constructors[0].Symbol || findings[0].FilePath != "pkg/owner/service.go" || findings[0].Line < 5) {
		t.Fatalf("unexpected guard identity: %+v", findings)
	}
}
