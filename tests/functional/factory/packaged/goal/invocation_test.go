package goal

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestPackagedGoalQuietCLIBatchReturnsPrimaryResultOnStdout proves packaged
// @you/goal batch invocation through the public you run CLI with --quiet writes
// only the primary result to stdout without echoing the submitted goal text.
// Isolation is intentional: this fixture must observe the finite Process.Execute
// stdout/stderr boundary without a shared continuous server. Dependency fidelity
// is local-real root.BuildProcess/Process.Execute and the packaged Factory, with
// only ProviderCommandRunner controlled at the external provider-command edge.
func TestPackagedGoalQuietCLIBatchReturnsPrimaryResultOnStdout(t *testing.T) {
	providerRunner := newPackagedGoalAcceptedProviderRunner(t)
	goalText := fmt.Sprintf("functional-packaged-goal-quiet-cli-%d", time.Now().UnixNano())

	stdout, stderr := runPackagedGoalQuietCLIBatch(t, providerRunner, goalText)
	if stdout != packagedGoalMockWorkerAcceptedSummary {
		t.Fatalf("stdout = %q, want only primary result %q", stdout, packagedGoalMockWorkerAcceptedSummary)
	}
	if strings.Contains(stdout, goalText) {
		t.Fatalf("stdout echoed submitted goal text %q", goalText)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty stderr on successful batch invocation", stderr)
	}
}
