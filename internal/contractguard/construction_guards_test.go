package contractguard

import (
	"bytes"
	"strings"
	"testing"
)

func TestConstructionRequiredGuardProvenance(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, constructor, operation string
		guards                       int
	}{
		{"constructor error", `if renamed == nil { panic("missing") }; return &Service{port: renamed}, nil`, "", 1},
		{"reversed nil", `if nil != renamed { renamed.Execute() }; return &Service{port: renamed}, nil`, "", 1},
		{"local alias", `alias := renamed; if alias == nil { panic("missing") }; return &Service{port: alias}, nil`, "", 1},
		{"declared alias", `var alias = renamed; if alias != nil { alias.Execute() }; return &Service{port: alias}, nil`, "", 1},
		{"field", `return &Service{port: renamed}, nil`, `if s.port != nil { s.port.Execute() }`, 1},
		{"field alias closure", `alias := renamed; return &Service{port: alias}, nil`, `alias := s.port; defer func() { if alias != nil { alias.Execute() } }()`, 1},
		{"shadowed local", `return &Service{port: renamed}, nil`, `alias := s.port; _ = alias; { alias := (*int)(nil); if alias == nil {} }`, 0},
		{"optional domain", `var payload *int; if payload == nil {}; return &Service{port: renamed}, nil`, `if s.payload == nil {}`, 0},
		{"optional operation input", `return &Service{port: renamed}, nil`, `var err error; if err != nil {}; var resource *int; if resource == nil {}`, 0},
		{"direct required use", `return &Service{port: renamed}, nil`, `s.port.Execute()`, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root, registry := constructionFixture(t)
			registry.CapabilitySets[0].Mode = ConstructionEnforce
			writeConstructionFixture(t, root, "pkg/owner/service.go", `package owner
type Port interface { Execute() }
type Service struct { port Port; payload *int }
func New(renamed Port) (*Service, error) { `+tc.constructor+` }
func (s *Service) Run() { `+tc.operation+` }
`)
			findings, err := ScanConstruction(root, registry)
			if err != nil {
				t.Fatal(err)
			}
			if len(findings) != tc.guards || CountBlockingConstructionFindings(findings) != tc.guards {
				t.Fatalf("findings = %+v, want %d blocking guards", findings, tc.guards)
			}
			for _, finding := range findings {
				if finding.Rule != "required-dependency-guard" || finding.Callee != registry.Constructors[0].Symbol || finding.FilePath != "pkg/owner/service.go" || finding.Line < 4 {
					t.Fatalf("unexpected guard identity: %+v", finding)
				}
			}
		})
	}
}

func TestConstructionGuardMutationReportsBlockingDebt(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, constructor, operation string }{
		{"parameter", `renamed = nil; if renamed == nil {}; return &Service{port: renamed}, nil`, ""},
		{"alias", `alias := renamed; alias = nil; if alias == nil {}; return &Service{port: renamed}, nil`, ""},
		{"stored alias", `alias := renamed; alias = nil; return &Service{port: alias}, nil`, `if s.port == nil {}`},
		{"operation alias", `return &Service{port: renamed}, nil`, `alias := s.port; alias = nil; if alias == nil {}`},
		{"operation field", `return &Service{port: renamed}, nil`, `s.port = nil; if s.port == nil {}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root, registry := constructionFixture(t)
			registry.CapabilitySets[0].Mode = ConstructionEnforce
			writeConstructionFixture(t, root, "pkg/owner/service.go", `package owner
type Port interface { Execute() }
type Service struct { port Port }
func New(renamed Port) (*Service, error) { `+tc.constructor+` }
func (s *Service) Run() { `+tc.operation+` }
`)
			findings, err := ScanConstruction(root, registry)
			if err != nil {
				t.Fatal(err)
			}
			if len(findings) != 1 || findings[0].Rule != "unresolved-required-dependency-guard" || CountBlockingConstructionFindings(findings) != 1 {
				t.Fatalf("mutation debt = %+v, want one blocking unresolved guard", findings)
			}
			var output bytes.Buffer
			WriteConstructionFindings(&output, findings)
			if strings.Contains(output.String(), "renamed") || strings.Contains(output.String(), "s.port") || strings.Contains(output.String(), "alias") {
				t.Fatalf("diagnostic includes source text: %s", output.String())
			}
		})
	}
}

func TestConstructionQualifiedDotAndGenericCalls(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, source string
		calls        int
	}{
		{"dot import", `import . "example.test/factory/pkg/owner"
func Observe(p Port) { New(p) }`, 1},
		{"dot shadow", `import . "example.test/factory/pkg/owner"
func Observe(p Port) { New := func(Port) {}; New(p) }`, 0},
		{"generic registered", `import selected "example.test/factory/pkg/owner"
func Observe(p selected.Port) { selected.New[int](p) }`, 1},
		{"generic local unrelated", `func New[T any](p T) {}
func Observe() { New[int](1) }`, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root, registry := constructionFixture(t)
			if tc.name == "generic registered" {
				writeConstructionFixture(t, root, "pkg/owner/service.go", `package owner
type Port interface { Execute() }
type Service struct { port Port }
func New[T any](renamed Port) (*Service, error) { return &Service{port: renamed}, nil }
func (s *Service) Run() { New[int](s.port) }
`)
				writeConstructionFixture(t, root, "pkg/wire/provider.go", `package wire
import selected "example.test/factory/pkg/owner"
func Provide(p selected.Port) { selected.New[int](p) }
`)
			}
			writeConstructionFixture(t, root, "pkg/consumer/observe.go", "package consumer\n"+tc.source+"\n")
			findings, err := ScanConstruction(root, registry)
			if err != nil {
				t.Fatal(err)
			}
			if len(findings) != tc.calls+1 {
				t.Fatalf("findings = %+v, want %d calls plus existing owner call", findings, tc.calls)
			}
			for _, finding := range findings {
				if finding.Rule != "registered-construction" || finding.Callee != registry.Constructors[0].Symbol {
					t.Fatalf("unexpected resolution: %+v", finding)
				}
			}
		})
	}
}
