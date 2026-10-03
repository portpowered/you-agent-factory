package contractguard

import (
	"bytes"
	"strings"
	"testing"
)

func TestConstructionHelperReturnProvenance(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, helpers, constructor, operation, rule string
	}{
		{"identity", `func pass(p Port) Port { return p }`, `return &Service{port: pass(renamed)}, nil`, `if s.port == nil {}`, "required-dependency-guard"},
		{"chain", `func pass(p Port) Port { return next(p) }; func next(p Port) Port { alias := p; return alias }`, `return &Service{port: pass(renamed)}, nil`, `if s.port == nil {}`, "required-dependency-guard"},
		{"operation", `func pass(p Port) Port { return p }`, `return &Service{port: renamed}, nil`, `alias := pass(s.port); if alias == nil {}`, "required-dependency-guard"},
		{"assertion", `func pass(p Port) Port { return p.(Port) }`, `if pass(renamed) == nil {}; return &Service{port: renamed}, nil`, ``, "required-dependency-guard"},
		{"closure return excluded", `func pass(p Port) Port { _ = func() Port { return nil }; return p }`, `return &Service{port: pass(renamed)}, nil`, `if s.port == nil {}`, "required-dependency-guard"},
		{"optional domain", `func payload(p Port) *int { return nil }`, `if payload(renamed) == nil {}; return &Service{port: renamed}, nil`, ``, ""},
		{"shadowed helper", `func pass(p Port) Port { return p }`, `return &Service{port: renamed}, nil`, `pass := func(p *int) *int { return p }; if pass(nil) == nil {}`, ""},
		{"mixed origin", `func pass(p Port) Port { if false { return nil }; return p }`, `return &Service{port: pass(renamed)}, nil`, `if s.port == nil {}`, "unresolved-required-dependency-guard"},
		{"recursive", `func pass(p Port) Port { return pass(p) }`, `if pass(renamed) == nil {}; return &Service{port: renamed}, nil`, ``, "unresolved-required-dependency-guard"},
		{"mutual recursion", `func pass(p Port) Port { return next(p) }; func next(p Port) Port { return pass(p) }`, `if pass(renamed) == nil {}; return &Service{port: renamed}, nil`, ``, "unresolved-required-dependency-guard"},
		{"mutated helper", `func pass(p Port) Port { p = nil; return p }`, `if pass(renamed) == nil {}; return &Service{port: renamed}, nil`, ``, "unresolved-required-dependency-guard"},
		{"mutated argument", `func pass(p Port) Port { return p }`, `renamed = nil; if pass(renamed) == nil {}; return &Service{port: renamed}, nil`, ``, "unresolved-required-dependency-guard"},
		{"named result", `func pass(p Port) (result Port) { result = p; return }`, `if pass(renamed) == nil {}; return &Service{port: renamed}, nil`, ``, "unresolved-required-dependency-guard"},
		{"resolved function value", `func pass(p Port) Port { return p }`, `alias := pass; if alias(renamed) == nil {}; return &Service{port: renamed}, nil`, ``, "required-dependency-guard"},
		{"reassigned function value", `func pass(p Port) Port { return p }`, `alias := pass; alias = pass; if alias(renamed) == nil {}; return &Service{port: renamed}, nil`, ``, "unresolved-required-dependency-guard"},
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
			writeConstructionFixture(t, root, "pkg/owner/helper.go", "package owner\n"+tc.helpers+"\n")
			findings, err := ScanConstruction(root, registry)
			if err != nil {
				t.Fatal(err)
			}
			if tc.rule == "" {
				if len(findings) != 0 {
					t.Fatalf("domain exclusion = %+v, want no findings", findings)
				}
				return
			}
			if len(findings) != 1 || CountBlockingConstructionFindings(findings) != 1 {
				t.Fatalf("findings = %+v, want one blocking guard", findings)
			}
			finding := findings[0]
			if finding.Rule != tc.rule || finding.Callee != registry.Constructors[0].Symbol || finding.FilePath != "pkg/owner/service.go" || finding.Line < 4 {
				t.Fatalf("unexpected helper guard identity: %+v", finding)
			}
		})
	}
}

func TestConstructionRequiredGuardsInsideHelpers(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, helper, constructor, operation, rule string
	}{
		{"constructor helper", `func check(renamed Port) { if renamed == nil {} }`, `check(renamed); return &Service{port: renamed}, nil`, ``, "required-dependency-guard"},
		{"operation helper", `func check(renamed Port) { if renamed != nil { renamed.Execute() } }`, `return &Service{port: renamed}, nil`, `check(s.port)`, "required-dependency-guard"},
		{"helper chain", `func check(renamed Port) { next(renamed) }; func next(alias Port) { if alias == nil {} }`, `check(renamed); return &Service{port: renamed}, nil`, ``, "required-dependency-guard"},
		{"recursive helper", `func check(renamed Port) { if renamed == nil {}; check(renamed) }`, `check(renamed); return &Service{port: renamed}, nil`, ``, "required-dependency-guard"},
		{"helper closure", `func check(renamed Port) { defer func() { if renamed == nil {} }() }`, `check(renamed); return &Service{port: renamed}, nil`, ``, "required-dependency-guard"},
		{"mutated argument", `func check(renamed Port) { if renamed == nil {} }`, `renamed = nil; check(renamed); return &Service{port: renamed}, nil`, ``, "unresolved-required-dependency-guard"},
		{"shadowed helper input", `func check(renamed Port) { { renamed := (*int)(nil); if renamed == nil {} } }`, `check(renamed); return &Service{port: renamed}, nil`, ``, ""},
		{"optional helper input", `func check(optional *int) { if optional == nil {} }`, `check(nil); return &Service{port: renamed}, nil`, ``, ""},
		{"uncalled helper", `func check(renamed Port) { if renamed == nil {} }`, `return &Service{port: renamed}, nil`, ``, ""},
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
			writeConstructionFixture(t, root, "pkg/owner/helper.go", "package owner\n"+tc.helper+"\n")
			findings, err := ScanConstruction(root, registry)
			if err != nil {
				t.Fatal(err)
			}
			if tc.rule == "" {
				if len(findings) != 0 {
					t.Fatalf("unrelated helper = %+v, want no findings", findings)
				}
				return
			}
			if len(findings) != 1 || CountBlockingConstructionFindings(findings) != 1 {
				t.Fatalf("findings = %+v, want one blocking helper guard", findings)
			}
			finding := findings[0]
			if finding.Rule != tc.rule || finding.Callee != registry.Constructors[0].Symbol || finding.FilePath != "pkg/owner/helper.go" || finding.Line != 2 || finding.Caller.ImportPath != "example.test/factory/pkg/owner" {
				t.Fatalf("unexpected helper guard identity: %+v", finding)
			}
			var output bytes.Buffer
			WriteConstructionFindings(&output, findings)
			if strings.Contains(output.String(), "renamed") || strings.Contains(output.String(), "s.port") {
				t.Fatalf("diagnostic includes source text: %s", output.String())
			}
		})
	}
}
