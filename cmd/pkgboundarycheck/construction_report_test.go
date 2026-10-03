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
