package script_pollers_test

import (
	"testing"

	scriptpollers "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/script_pollers"
)

func TestSourceIDForWorkstation(t *testing.T) {
	t.Parallel()

	if got := scriptpollers.SourceIDForWorkstation("linear-ingress"); got != "script-poller:linear-ingress" {
		t.Fatalf("SourceIDForWorkstation() = %q, want script-poller:linear-ingress", got)
	}
	if got := scriptpollers.SourceIDForWorkstation(" "); got != "" {
		t.Fatalf("SourceIDForWorkstation() = %q, want empty for blank workstation", got)
	}
}

func TestStableInstanceID_IsDeterministicAndNamespaced(t *testing.T) {
	t.Parallel()

	first := scriptpollers.StableInstanceID("workflow-a", "script-poller:ingress")
	second := scriptpollers.StableInstanceID("workflow-a", "script-poller:ingress")
	if first != second {
		t.Fatalf("StableInstanceID() = %q and %q, want deterministic value", first, second)
	}
	if !scriptpollers.IsScriptPollerInstanceID(first) {
		t.Fatalf("IsScriptPollerInstanceID(%q) = false, want true", first)
	}
	if scriptpollers.IsScriptPollerInstanceID("automation-instance:deadbeef") {
		t.Fatal("IsScriptPollerInstanceID() = true for reconciliation instance, want false")
	}
}

func TestSupervisionFor_WiresAutomationSourceAndInstanceIdentity(t *testing.T) {
	t.Parallel()

	supervision := scriptpollers.SupervisionFor("workflow-cursor", "linear-ingress")
	if supervision.AutomationID != "workflow-cursor" {
		t.Fatalf("AutomationID = %q, want workflow-cursor", supervision.AutomationID)
	}
	if supervision.SourceID != "script-poller:linear-ingress" {
		t.Fatalf("SourceID = %q, want script-poller:linear-ingress", supervision.SourceID)
	}
	if supervision.InstanceID == "" || !scriptpollers.IsScriptPollerInstanceID(supervision.InstanceID) {
		t.Fatalf("InstanceID = %q, want non-empty script-poller instance identity", supervision.InstanceID)
	}
}
