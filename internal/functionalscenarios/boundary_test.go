package functionalscenarios

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckFunctionalBoundaryFile_AcceptsPublicClientAndSupportSeams(t *testing.T) {
	violations := checkBoundarySource(t, `package scenario
import (
	generated "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)
func observe(client *generated.ClientWithResponses) { _ = client; _ = support.DefaultSessionEventsURL }
`)
	if len(violations) != 0 {
		t.Fatalf("public boundary source produced violations: %v", violations)
	}
}

func TestCheckFunctionalBoundaryFile_RejectsConstructionInspectionAndAliases(t *testing.T) {
	tests := []struct {
		name         string
		source       string
		wantBoundary string
		wantSymbol   string
	}{
		{
			name: "wire composition",
			source: `package scenario
import "github.com/portpowered/infinite-you/pkg/wire"
func run() { wire.InjectFactoryService(nil, nil) }
`,
			wantBoundary: "composition",
			wantSymbol:   "pkg/wire.InjectFactoryService",
		},
		{
			name: "API surface composite",
			source: `package scenario
import mapping "github.com/portpowered/infinite-you/pkg/transports/mapping"
var surface = mapping.APISurface{}
`,
			wantBoundary: "API surface",
			wantSymbol:   "pkg/transports/mapping.APISurface",
		},
		{
			name: "service configuration composite",
			source: `package scenario
import "github.com/portpowered/infinite-you/pkg/service"
var config = service.FactoryServiceConfig{}
`,
			wantBoundary: "service",
			wantSymbol:   "pkg/service.FactoryServiceConfig",
		},
		{
			name: "runtime generated configuration lookup",
			source: `package scenario
import runtime "github.com/portpowered/infinite-you/pkg/factory/runtime"
func run() { runtime.GeneratedConfigForDefinition() }
`,
			wantBoundary: "runtime",
			wantSymbol:   "pkg/factory/runtime.GeneratedConfigForDefinition",
		},
		{
			name: "projection reconstruction",
			source: `package scenario
import projections "github.com/portpowered/infinite-you/pkg/factory/projections"
func run() { projections.ReconstructWorld() }
`,
			wantBoundary: "projection",
			wantSymbol:   "pkg/factory/projections.ReconstructWorld",
		},
		{
			name: "internal event history replay",
			source: `package scenario
import replay "github.com/portpowered/infinite-you/pkg/factory/replay"
func run() { replay.FromEventHistory() }
`,
			wantBoundary: "recorder",
			wantSymbol:   "pkg/factory/replay.FromEventHistory",
		},
		{
			name: "constructor alias",
			source: `package scenario
import "github.com/portpowered/infinite-you/pkg/service"
func run() { build := service.NewFactoryService; build() }
`,
			wantBoundary: "service",
			wantSymbol:   "pkg/service.NewFactoryService",
		},
		{
			name: "inferred receiver",
			source: `package scenario
import mapping "github.com/portpowered/infinite-you/pkg/transports/mapping"
func run() { surface := mapping.NewAPISurface(); surface.Status() }
`,
			wantBoundary: "API surface",
			wantSymbol:   "pkg/transports/mapping.APISurface.Status",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			violations := checkBoundarySource(t, test.source)
			if !containsBoundaryViolation(violations, test.wantBoundary, test.wantSymbol) {
				t.Fatalf("violations = %#v, want boundary %q symbol containing %q", violations, test.wantBoundary, test.wantSymbol)
			}
			diagnostic := violations[0].Error()
			if !strings.Contains(diagnostic, "functional test boundary [direct-product-boundary]") ||
				!strings.Contains(diagnostic, "invoke or observe the product through REST, CLI, MCP, or SSE") {
				t.Fatalf("diagnostic = %q, want stable category and remediation", diagnostic)
			}
		})
	}
}

func checkBoundarySource(t *testing.T, source string) []FunctionalBoundaryViolation {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, "tests", "functional", "scenario_test.go")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create functional fixture directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatalf("write functional fixture: %v", err)
	}
	violations, err := checkFunctionalBoundaryFile(root, path)
	if err != nil {
		t.Fatalf("check functional boundary fixture: %v", err)
	}
	return violations
}

func containsBoundaryViolation(violations []FunctionalBoundaryViolation, boundary, symbol string) bool {
	for _, violation := range violations {
		if violation.Boundary == boundary && strings.Contains(violation.Symbol, symbol) {
			return true
		}
	}
	return false
}
