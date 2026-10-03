package contractguard

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fixtureOwner = "example.test/factory/pkg/owner"

func constructionFixture(t *testing.T) (string, ConstructionRegistry) {
	t.Helper()
	root := t.TempDir()
	writeConstructionFixture(t, root, "go.mod", "module example.test/factory\n\ngo 1.25.0\n")
	writeConstructionFixture(t, root, "pkg/owner/service.go", `package owner
type Port interface { Execute() }
type Service struct { port Port }
func New(renamed Port) (*Service, error) { return &Service{port: renamed}, nil }
func (s *Service) Run() { New(s.port) }
`)
	writeConstructionFixture(t, root, "pkg/wire/provider.go", `package wire
import selected "example.test/factory/pkg/owner"
func Provide(p selected.Port) { selected.New(p) }
`)
	symbol := ConstructionSymbol{ImportPath: fixtureOwner, Name: "New"}
	registry := ConstructionRegistry{
		CapabilitySets: []ConstructionCapabilitySet{{Name: "owner", OwnerTask: "T01", Mode: ConstructionReport}},
		Constructors:   []ConstructionConstructor{{Symbol: symbol, CapabilitySet: "owner", RequiredParameters: []ConstructionParameter{{Index: 0, TypeExpr: fixtureOwner + ".Port"}}, Results: []ConstructionSymbol{{ImportPath: fixtureOwner, Name: "Service"}}}},
		Types:          []ConstructionType{{Symbol: ConstructionSymbol{ImportPath: fixtureOwner, Name: "Service"}, CapabilitySet: "owner", Kind: ConstructionBehavior}},
		Allowances:     []ConstructionAllowance{{Caller: ConstructionSymbol{ImportPath: "example.test/factory/pkg/wire", Name: "Provide"}, Callee: symbol, FilePath: "pkg/wire/provider.go", Kind: "focused-provider", OwnerTask: "T01", Reason: "Direct focused provider."}},
	}
	return root, registry
}

func writeConstructionFixture(t *testing.T, root, relative, content string) {
	t.Helper()
	file := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestConstructionReportUsesQualifiedOperationAndExactProvider(t *testing.T) {
	t.Parallel()
	root, registry := constructionFixture(t)
	findings, err := ScanConstruction(root, registry)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 {
		t.Fatalf("findings = %+v, want one operation call", findings)
	}
	finding := findings[0]
	if finding.Caller != (ConstructionSymbol{ImportPath: fixtureOwner, Receiver: "Service", Name: "Run"}) || finding.Callee != registry.Constructors[0].Symbol || finding.FilePath != "pkg/owner/service.go" || finding.Line != 5 {
		t.Fatalf("unexpected qualified observation: %+v", finding)
	}
	if CountBlockingConstructionFindings(findings) != 0 {
		t.Fatal("report observation became blocking")
	}
	var output bytes.Buffer
	WriteConstructionFindings(&output, findings)
	for _, want := range []string{"pkg/owner/service.go:5", fixtureOwner + ".(Service).Run", fixtureOwner + ".New", "mode=report"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("output %q missing %q", output.String(), want)
		}
	}
	if strings.Contains(output.String(), "s.port") {
		t.Fatal("diagnostic disclosed source text")
	}
}

func TestConstructionRegistryRejectsInvalidMetadata(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		change func(*ConstructionRegistry)
	}{
		{"stale constructor", func(r *ConstructionRegistry) { r.Constructors[0].Symbol.Name = "Missing" }},
		{"duplicate constructor", func(r *ConstructionRegistry) { r.Constructors = append(r.Constructors, r.Constructors[0]) }},
		{"conflicting type", func(r *ConstructionRegistry) {
			typ := r.Types[0]
			typ.Kind = ConstructionState
			r.Types = append(r.Types, typ)
		}},
		{"out of range", func(r *ConstructionRegistry) { r.Constructors[0].RequiredParameters[0].Index = 1 }},
		{"negative index", func(r *ConstructionRegistry) { r.Constructors[0].RequiredParameters[0].Index = -1 }},
		{"duplicate parameter", func(r *ConstructionRegistry) {
			r.Constructors[0].RequiredParameters = append(r.Constructors[0].RequiredParameters, r.Constructors[0].RequiredParameters[0])
		}},
		{"wrong type", func(r *ConstructionRegistry) { r.Constructors[0].RequiredParameters[0].TypeExpr = "string" }},
		{"wrong result", func(r *ConstructionRegistry) { r.Constructors[0].Results[0].Name = "Port" }},
		{"invalid mode", func(r *ConstructionRegistry) { r.CapabilitySets[0].Mode = "disabled" }},
		{"unknown set", func(r *ConstructionRegistry) { r.Constructors[0].CapabilitySet = "unknown" }},
		{"wildcard path", func(r *ConstructionRegistry) { r.Allowances[0].FilePath = "pkg/wire/*" }},
		{"directory path", func(r *ConstructionRegistry) { r.Allowances[0].FilePath = "pkg/wire" }},
		{"widened caller", func(r *ConstructionRegistry) { r.Allowances[0].Caller.Name = "*" }},
		{"stale callee", func(r *ConstructionRegistry) { r.Allowances[0].Callee.Name = "Other" }},
		{"empty rationale", func(r *ConstructionRegistry) { r.Allowances[0].Reason = " " }},
		{"duplicate allowance", func(r *ConstructionRegistry) { r.Allowances = append(r.Allowances, r.Allowances[0]) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root, registry := constructionFixture(t)
			tc.change(&registry)
			if _, err := ScanConstruction(root, registry); err == nil {
				t.Fatal("invalid registry accepted")
			}
		})
	}
}

func TestConstructionSourceClassesRetainCompiledSupport(t *testing.T) {
	t.Parallel()
	root, registry := constructionFixture(t)
	for _, file := range []string{"pkg/owner/outer_test.go", "pkg/owner/generated.go", "pkg/owner/servertests/support.go", "pkg/owner/internal/build/support.go"} {
		prefix := ""
		if strings.HasSuffix(file, "generated.go") {
			prefix = "// Code generated by fixture. DO NOT EDIT.\n"
		}
		writeConstructionFixture(t, root, file, prefix+`package support
import selected "example.test/factory/pkg/owner"
func Observe(p selected.Port) { selected.New(p) }
`)
	}
	findings, err := ScanConstruction(root, registry)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 3 || findings[0].FilePath != "pkg/owner/internal/build/support.go" || findings[1].FilePath != "pkg/owner/servertests/support.go" {
		t.Fatalf("source classification = %+v", findings)
	}
}

func TestConstructionScopedKindsAndImportShadowRemainAllowed(t *testing.T) {
	t.Parallel()
	root, registry := constructionFixture(t)
	registry.Types[0].Kind = ConstructionState
	findings, err := ScanConstruction(root, registry)
	if err != nil || len(findings) != 0 {
		t.Fatalf("scoped state = %+v, %v", findings, err)
	}
	registry.Types[0].Kind = ConstructionBehavior
	writeConstructionFixture(t, root, "pkg/wire/shadow.go", `package wire
import selected "example.test/factory/pkg/owner"
func Shadow(selected struct { New func() }) { selected.New() }
`)
	findings, err = ScanConstruction(root, registry)
	if err != nil || len(findings) != 1 {
		t.Fatalf("shadowing = %+v, %v", findings, err)
	}
}

func TestConstructionAliasesReportAnalysisDebt(t *testing.T) {
	t.Parallel()
	root, registry := constructionFixture(t)
	writeConstructionFixture(t, root, "pkg/wire/alias.go", `package wire
import selected "example.test/factory/pkg/owner"
func Later(p selected.Port) { alias := selected.New; alias(p) }
`)
	findings, err := ScanConstruction(root, registry)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 2 || findings[1].Rule != "unresolved-construction-reference" || findings[1].Callee != registry.Constructors[0].Symbol {
		t.Fatalf("constructor alias debt = %+v", findings)
	}
}

func TestConstructionRelativeRootMatchesAbsoluteRoot(t *testing.T) {
	t.Parallel()
	root, registry := constructionFixture(t)
	working, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(working, root)
	if err != nil {
		t.Fatal(err)
	}
	findings, err := ScanConstruction(relative, registry)
	if err != nil || len(findings) != 1 {
		t.Fatalf("relative root = %+v, %v", findings, err)
	}
}
