package symbolidentity_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryruntimetestkit "github.com/portpowered/infinite-you/pkg/services/factory_runtime/testkit"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/tooling/javascript/symbolidentity"
)

func TestVerifyProjectedInstalledBindings_PassesForLiveDescriptor(t *testing.T) {
	if err := symbolidentity.VerifyProjectedInstalledBindings(); err != nil {
		t.Fatalf("VerifyProjectedInstalledBindings() error = %v", err)
	}
}

func TestClassifySurface_CentralizesUnsupportedSymbolPolicy(t *testing.T) {
	tests := []struct {
		path string
		kind string
		want symbolidentity.SurfaceClassification
	}{
		{path: "workflow.final", kind: "function", want: symbolidentity.SurfaceSupported},
		{path: "context.session", kind: "property", want: symbolidentity.SurfaceForbiddenHostGlobal},
		{path: "orchestrator", kind: "object", want: symbolidentity.SurfaceForbiddenHostGlobal},
		{path: "workflow.sleep", kind: "function", want: symbolidentity.SurfaceComparisonProjectHelper},
		{path: "agent.verify", kind: "function", want: symbolidentity.SurfaceComparisonProjectHelper},
		{path: "agent.parallel", kind: "function", want: symbolidentity.SurfaceComparisonProjectHelper},
		{path: "agent", kind: "function", want: symbolidentity.SurfaceCallableAgentGlobal},
		{path: "agent", kind: "object", want: symbolidentity.SurfaceSupported},
	}

	for _, test := range tests {
		t.Run(test.path+"/"+test.kind, func(t *testing.T) {
			if got := symbolidentity.ClassifySurface(test.path, test.kind); got != test.want {
				t.Fatalf("ClassifySurface(%q, %q) = %q, want %q", test.path, test.kind, got, test.want)
			}
		})
	}
}

func TestVerifyInventory_FailsWhenExpectedPathMissing(t *testing.T) {
	inventory := symbolidentity.ProjectInstalledBindings()
	filtered := make([]symbolidentity.SymbolRecord, 0, len(inventory.Symbols)-1)
	for _, record := range inventory.Symbols {
		if record.Path == "agent.run" {
			continue
		}
		filtered = append(filtered, record)
	}
	inventory.Symbols = filtered

	err := symbolidentity.VerifyInventory(inventory)
	if err == nil {
		t.Fatal("VerifyInventory() error = nil, want missing path failure")
	}
	if got := err.Error(); !strings.Contains(got, `missing symbol path "agent.run"`) {
		t.Fatalf("VerifyInventory() error = %q, want missing path diagnostic naming agent.run", got)
	}
}

func TestVerifyInventory_FailsWhenUnexpectedPathPresent(t *testing.T) {
	inventory := symbolidentity.ProjectInstalledBindings()
	inventory.Symbols = append(inventory.Symbols, symbolidentity.SymbolRecord{
		IDCandidate: "probe-extra",
		Name:        "probe-extra",
		Path:        "probe.extra",
		Kind:        "value",
	})

	err := symbolidentity.VerifyInventory(inventory)
	if err == nil {
		t.Fatal("VerifyInventory() error = nil, want unexpected path failure")
	}
	if got := err.Error(); !strings.Contains(got, `unexpected symbol path "probe.extra"`) {
		t.Fatalf("VerifyInventory() error = %q, want unexpected path diagnostic naming probe.extra", got)
	}
}

func TestVerifyInventory_FailsWhenPathDuplicated(t *testing.T) {
	inventory := symbolidentity.ProjectInstalledBindings()
	inventory.Symbols = append(inventory.Symbols, inventory.Symbols[0])

	err := symbolidentity.VerifyInventory(inventory)
	if err == nil {
		t.Fatal("VerifyInventory() error = nil, want duplicate path failure")
	}
	if got := err.Error(); !strings.Contains(got, `duplicate symbol path "agent"`) {
		t.Fatalf("VerifyInventory() error = %q, want duplicate path diagnostic naming agent", got)
	}
}

func TestVerifyInventory_FailsWhenForbiddenGlobalPresent(t *testing.T) {
	for _, forbidden := range symbolidentity.ForbiddenRootGlobals {
		t.Run(forbidden, func(t *testing.T) {
			inventory := symbolidentity.ProjectInstalledBindings()
			inventory.Symbols = append(inventory.Symbols, symbolidentity.SymbolRecord{
				IDCandidate: forbidden,
				Name:        forbidden,
				Path:        forbidden,
				Kind:        "namespace",
			})

			err := symbolidentity.VerifyInventory(inventory)
			if err == nil {
				t.Fatalf("VerifyInventory() error = nil, want forbidden path failure for %q", forbidden)
			}
			if got := err.Error(); !strings.Contains(got, fmt.Sprintf(`forbidden symbol path %q`, forbidden)) {
				t.Fatalf("VerifyInventory() error = %q, want forbidden path diagnostic naming %q", got, forbidden)
			}
		})
	}
}

func TestForbiddenGlobalsAbsentFromInventoryAndInstalledBindingSurface(t *testing.T) {
	inventory := symbolidentity.ProjectInstalledBindings()
	paths := pathsFromInventory(inventory)
	for _, forbidden := range symbolidentity.ForbiddenRootGlobals {
		for _, path := range paths {
			if path == forbidden || strings.HasPrefix(path, forbidden+".") {
				t.Fatalf("inventory exposes forbidden global %q via path %q", forbidden, path)
			}
		}
	}

	installedRoots := symbolidentity.InstalledRootGlobals()
	for _, forbidden := range symbolidentity.ForbiddenRootGlobals {
		for _, root := range installedRoots {
			if root == forbidden {
				t.Fatalf("installed binding surface exposes forbidden root global %q", forbidden)
			}
		}
	}

	for _, forbidden := range symbolidentity.ForbiddenRootGlobals {
		result := factoryruntimetestkit.JavaScriptWorkflows().Validate(factory.WorkflowValidationRequest{
			Source:    fmt.Sprintf("%s({}); workflow.final({ ok: true });", forbidden),
			SourceRef: "inline",
		})
		if !result.HasIssues() {
			t.Fatalf("validation accepted forbidden global %q", forbidden)
		}
		if result.Issues[0].Code != factory.WorkflowValidationCodeUnsupportedGlobal {
			t.Fatalf("validation issue for %q = %#v, want unsupported global", forbidden, result.Issues[0])
		}
	}
}
