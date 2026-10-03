package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/contractguard"
)

func TestConstructionReportsPreserveExistingCommandStatus(t *testing.T) {
	t.Parallel()
	for _, blocking := range []bool{false, true} {
		name := "report-only"
		if blocking {
			name = "existing-blocker"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeGoSourceFile(t, root, "go.mod", "module example.test/factory\n\ngo 1.25.0\n")
			writeGoSourceFile(t, root, "pkg/services/example/service.go", `package example
type Service struct{}
func New() *Service { return &Service{} }
func (s *Service) Run() { New() }
`)
			const owner = "example.test/factory/pkg/services/example"
			registry := contractguard.ConstructionRegistry{
				CapabilitySets: []contractguard.ConstructionCapabilitySet{{Name: "example", OwnerTask: "T01", Mode: contractguard.ConstructionReport}},
				Constructors:   []contractguard.ConstructionConstructor{{Symbol: contractguard.ConstructionSymbol{ImportPath: owner, Name: "New"}, CapabilitySet: "example", Results: []contractguard.ConstructionSymbol{{ImportPath: owner, Name: "Service"}}}},
				Types:          []contractguard.ConstructionType{{Symbol: contractguard.ConstructionSymbol{ImportPath: owner, Name: "Service"}, CapabilitySet: "example", Kind: contractguard.ConstructionBehavior}},
			}
			if blocking {
				makeDir(t, root, "pkg/unapproved")
			}
			var stdout, stderr bytes.Buffer
			err := run(config{root: root, packageRoot: defaultScanRoot, constructionRegistry: &registry}, &stdout, &stderr)
			if (err != nil) != blocking {
				t.Fatalf("run status = %v; blocking=%v", err, blocking)
			}
			output := stdout.String()
			if blocking {
				output = stderr.String()
				if !strings.Contains(output, "unapproved root package family") {
					t.Fatalf("existing diagnostic lost: %q", output)
				}
			}
			for _, want := range []string{"pkg/services/example/service.go:4", owner + ".(Service).Run", "callee=" + owner + ".New", "mode=report"} {
				if !strings.Contains(output, want) {
					t.Fatalf("output %q missing %q", output, want)
				}
			}
		})
	}
}

func TestConstructionReportIsNotAdmittedToRecordedBaseline(t *testing.T) {
	t.Parallel()
	finding := contractguard.ConstructionFinding{Mode: contractguard.ConstructionEnforce, Rule: "registered-construction"}
	visible, _ := filterRecordedScanResult(scanResult{constructionFindings: []contractguard.ConstructionFinding{finding}}, recordedBoundaryBaseline{})
	if len(visible.constructionFindings) != 1 || countBlockingViolations(visible) != 1 {
		t.Fatal("construction finding lost to existing baseline partition")
	}
}

func TestFocusedProviderDispatchDebtControlsCommandStatus(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, body string
		mode       contractguard.ConstructionMode
	}{
		{"callback-report", "callback();", contractguard.ConstructionReport},
		{"callback-enforce", "callback();", contractguard.ConstructionEnforce},
		{"returned-report", "factory()();", contractguard.ConstructionReport},
		{"returned-enforce", "factory()();", contractguard.ConstructionEnforce},
		{"field-report", "h := Hook{}; h.Run();", contractguard.ConstructionReport},
		{"field-enforce", "h := Hook{}; h.Run();", contractguard.ConstructionEnforce},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			mode := tc.mode
			root := t.TempDir()
			writeGoSourceFile(t, root, "go.mod", "module example.test/factory\n\ngo 1.25.0\n")
			writeGoSourceFile(t, root, "pkg/services/example/service.go", `package example
type Service struct{}
func New() *Service { return &Service{} }
`)
			writeGoSourceFile(t, root, "pkg/wire/provider.go", `package wire
import "example.test/factory/pkg/services/example"
func Provide(callback func()) { `+tc.body+` example.New() }
func factory() func() { return func() {} }
type Hook struct { Run func() }
`)
			const owner = "example.test/factory/pkg/services/example"
			constructor := contractguard.ConstructionSymbol{ImportPath: owner, Name: "New"}
			provider := contractguard.ConstructionSymbol{ImportPath: "example.test/factory/pkg/wire", Name: "Provide"}
			service := contractguard.ConstructionSymbol{ImportPath: owner, Name: "Service"}
			registry := contractguard.ConstructionRegistry{
				CapabilitySets: []contractguard.ConstructionCapabilitySet{{Name: "example", OwnerTask: "T20", Mode: mode}},
				Constructors:   []contractguard.ConstructionConstructor{{Symbol: constructor, CapabilitySet: "example", Results: []contractguard.ConstructionSymbol{service}}},
				Types:          []contractguard.ConstructionType{{Symbol: service, CapabilitySet: "example", Kind: contractguard.ConstructionBehavior}},
				Allowances: []contractguard.ConstructionAllowance{{Caller: provider, Callee: constructor, FilePath: "pkg/wire/provider.go",
					Kind: "focused-provider", OwnerTask: "T20", Reason: "direct focused construction"}},
			}
			var stdout, stderr bytes.Buffer
			err := run(config{root: root, packageRoot: defaultScanRoot, constructionRegistry: &registry}, &stdout, &stderr)
			if (err != nil) != (mode == contractguard.ConstructionEnforce) {
				t.Fatalf("mode %s: unexpected command status %v; stderr=%s", mode, err, &stderr)
			}
			output := stdout.String() + stderr.String()
			for _, want := range []string{"pkg/wire/provider.go:3", "caller=" + provider.String(), "callee=" + constructor.String(), "mode=" + string(mode), "rule=unresolved-focused-provider-dispatch"} {
				if !strings.Contains(output, want) {
					t.Fatalf("output %q missing %q", output, want)
				}
			}
			if strings.Contains(output, tc.body) {
				t.Fatalf("diagnostic exposed source text: %q", output)
			}
		})
	}
}
