package callbehavior_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryruntimetestkit "github.com/portpowered/infinite-you/pkg/services/factory_runtime/testkit"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/tooling/javascript/callbehavior"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/tooling/javascript/symbolidentity"
)

func TestVerifyInventory_FailsWhenExpectedPathMissing(t *testing.T) {
	inventory := callbehavior.ProjectInstalledCallBehavior()
	filtered := make([]callbehavior.CallBehaviorRecord, 0, len(inventory.Records)-1)
	for _, record := range inventory.Records {
		if record.Path == "agent.run" {
			continue
		}
		filtered = append(filtered, record)
	}
	inventory.Records = filtered

	err := callbehavior.VerifyInventory(inventory)
	if err == nil {
		t.Fatal("VerifyInventory() error = nil, want missing path failure")
	}
	if got := err.Error(); !strings.Contains(got, `missing record path "agent.run"`) {
		t.Fatalf("VerifyInventory() error = %q, want missing path diagnostic naming agent.run", got)
	}
}

func TestVerifyInventory_FailsWhenUnexpectedPathPresent(t *testing.T) {
	inventory := callbehavior.ProjectInstalledCallBehavior()
	inventory.Records = append(inventory.Records, callbehavior.CallBehaviorRecord{
		IDCandidate: "probe-extra",
		Path:        "probe.extra",
		Kind:        "value",
	})

	err := callbehavior.VerifyInventory(inventory)
	if err == nil {
		t.Fatal("VerifyInventory() error = nil, want unexpected path failure")
	}
	if got := err.Error(); !strings.Contains(got, `unexpected record path "probe.extra"`) {
		t.Fatalf("VerifyInventory() error = %q, want unexpected path diagnostic naming probe.extra", got)
	}
}

func TestVerifyInventory_FailsWhenPathDuplicated(t *testing.T) {
	inventory := callbehavior.ProjectInstalledCallBehavior()
	inventory.Records = append(inventory.Records, inventory.Records[0])

	err := callbehavior.VerifyInventory(inventory)
	if err == nil {
		t.Fatal("VerifyInventory() error = nil, want duplicate path failure")
	}
	if got := err.Error(); !strings.Contains(got, `duplicate record path "agent"`) {
		t.Fatalf("VerifyInventory() error = %q, want duplicate path diagnostic naming agent", got)
	}
}

func TestForbiddenGlobalsAbsentFromInventoryAndInstalledBindingSurface(t *testing.T) {
	inventory := callbehavior.ProjectInstalledCallBehavior()
	for _, forbidden := range callbehavior.ForbiddenRootGlobals {
		for _, record := range inventory.Records {
			path := record.Path
			if path == forbidden || strings.HasPrefix(path, forbidden+".") {
				t.Fatalf("call-behavior inventory exposes forbidden global %q via path %q", forbidden, path)
			}
		}
	}

	installedRoots := symbolidentity.InstalledRootGlobals()
	for _, forbidden := range callbehavior.ForbiddenRootGlobals {
		for _, root := range installedRoots {
			if root == forbidden {
				t.Fatalf("installed binding surface exposes forbidden root global %q", forbidden)
			}
		}
	}

	for _, forbidden := range callbehavior.ForbiddenRootGlobals {
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

func TestVerifyInventory_FailsWhenForbiddenGlobalPresent(t *testing.T) {
	for _, forbidden := range callbehavior.ForbiddenRootGlobals {
		t.Run(forbidden, func(t *testing.T) {
			inventory := callbehavior.ProjectInstalledCallBehavior()
			inventory.Records = append(inventory.Records, callbehavior.CallBehaviorRecord{
				IDCandidate: forbidden,
				Path:        forbidden,
				Kind:        "namespace",
			})

			err := callbehavior.VerifyInventory(inventory)
			if err == nil {
				t.Fatalf("VerifyInventory() error = nil, want forbidden path failure for %q", forbidden)
			}
			if got := err.Error(); !strings.Contains(got, fmt.Sprintf(`forbidden record path %q`, forbidden)) {
				t.Fatalf("VerifyInventory() error = %q, want forbidden path diagnostic naming %q", got, forbidden)
			}
		})
	}
}
