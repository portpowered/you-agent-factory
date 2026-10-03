package contractguard

import "testing"

func TestConstructionRequiredStorage(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, constructor, rule string
	}{
		{"positional", `return &Service{renamed, nil}, nil`, "required-dependency-guard"},
		{"positional alias", `alias := renamed; return &Service{alias, nil}, nil`, "required-dependency-guard"},
		{"allocated assignment", `s := new(Service); s.port = renamed; return s, nil`, "required-dependency-guard"},
		{"literal assignment", `s := &Service{}; s.port = renamed; return s, nil`, "required-dependency-guard"},
		{"declared assignment", `var s Service; s.port = renamed; return &s, nil`, "required-dependency-guard"},
		{"declared initialized", `var s = &Service{}; s.port = renamed; return s, nil`, "required-dependency-guard"},
		{"storage alias", `s := &Service{}; alias := s; alias.port = renamed; return s, nil`, "required-dependency-guard"},
		{"parallel storage", `s, other := &Service{}, 1; _ = other; s.port, s.payload = renamed, nil; return s, nil`, "required-dependency-guard"},
		{"shadowed storage", `s := &Service{}; { s := &Other{}; s.port = renamed }; return s, nil`, ""},
		{"optional storage", `s := &Service{}; s.port = optional(); return s, nil`, ""},
		{"replaced field", `s := &Service{port: renamed}; s.port = nil; return s, nil`, "unresolved-required-dependency-guard"},
		{"replaced then required", `s := &Service{port: nil}; s.port = renamed; return s, nil`, "unresolved-required-dependency-guard"},
		{"reassigned storage", `s := &Service{}; s = &Service{}; s.port = renamed; return s, nil`, "unresolved-required-dependency-guard"},
		{"reassigned storage alias", `s := &Service{}; alias := s; s = &Service{}; alias.port = renamed; return alias, nil`, "unresolved-required-dependency-guard"},
		{"mixed return storage", `if true { return &Service{port: renamed}, nil }; return &Service{port: nil}, nil`, "unresolved-required-dependency-guard"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root, registry := constructionFixture(t)
			registry.CapabilitySets[0].Mode = ConstructionEnforce
			writeConstructionFixture(t, root, "pkg/owner/service.go", `package owner
type Port interface { Execute() }
type Service struct { port Port; payload *int }
type Other struct { port Port }
func optional() Port { return nil }
func New(renamed Port) (*Service, error) { `+tc.constructor+` }
func (s *Service) Run() { if s.port == nil {}; if s.payload == nil {} }
`)
			findings, err := ScanConstruction(root, registry)
			if err != nil {
				t.Fatal(err)
			}
			want := 0
			if tc.rule != "" {
				want = 1
			}
			if len(findings) != want || CountBlockingConstructionFindings(findings) != want {
				t.Fatalf("findings = %+v, want %d blocking observations", findings, want)
			}
			if want == 1 && (findings[0].Rule != tc.rule || findings[0].Caller.Receiver != "Service" || findings[0].Caller.Name != "Run" || findings[0].Callee != registry.Constructors[0].Symbol || findings[0].Line != 7) {
				t.Fatalf("unexpected stored-field identity: %+v", findings)
			}
		})
	}
}

func TestConstructionPositionalGroupedAndEmbeddedFields(t *testing.T) {
	t.Parallel()
	root, registry := constructionFixture(t)
	writeConstructionFixture(t, root, "pkg/owner/service.go", `package owner
type Port interface { Execute() }
type Service struct { *Domain; payload, other *int; port Port }
type Domain struct {}
func New(renamed Port) (*Service, error) { return &Service{nil, nil, nil, renamed}, nil }
func (s *Service) Run() { if s.port == nil {}; if s.Domain == nil {}; if s.other == nil {} }
`)
	findings, err := ScanConstruction(root, registry)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || findings[0].Rule != "required-dependency-guard" || findings[0].Caller.Name != "Run" {
		t.Fatalf("grouped/embedded positional storage = %+v, want one required guard", findings)
	}
}
