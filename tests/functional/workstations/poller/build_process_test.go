package poller

import (
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestScriptPollerAutomationRemainsInertThroughRootBuildProcessConstruction proves
// root.BuildProcess composes Automations script-poller supervision without
// launching command, admission, or cursor effects during construction.
func TestScriptPollerAutomationRemainsInertThroughRootBuildProcessConstruction(t *testing.T) {
	t.Parallel()

	runner := newPollerIngressCommandRunner(t, nil)
	_ = support.BuildProcess(t, serviceedges.Edges{
		ScriptCommandRunner: runner,
	})
	if runner.callCount() != 0 {
		t.Fatalf("script command runner calls = %d during BuildProcess, want 0", runner.callCount())
	}
}
