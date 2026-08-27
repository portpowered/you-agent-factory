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

// TestPackagedGoalQuietCLIBatchExitsWithoutContinuousMode proves packaged
// @you/goal batch invocation through the public you run CLI exits after the
// invocation completes instead of staying in continuous service mode.
// Isolation is intentional: the termination property belongs to one finite
// Process.Execute call, while the shared parent intentionally keeps a daemon
// alive. Dependency fidelity is the local-real root/CLI path plus a controlled
// ProviderCommandRunner edge; no mock-worker mode substitutes for the worker
// boundary.
func TestPackagedGoalQuietCLIBatchExitsWithoutContinuousMode(t *testing.T) {
	providerRunner := newPackagedGoalAcceptedProviderRunner(t)
	goalText := fmt.Sprintf("functional-packaged-goal-quiet-exit-%d", time.Now().UnixNano())

	if err := runPackagedGoalQuietCLIBatchWithTimeout(t, providerRunner, goalText, 20*time.Second); err != nil {
		t.Fatalf("packaged goal quiet CLI batch invocation: %v", err)
	}
	if got := providerRunner.CallCount(); got != 1 {
		t.Fatalf("provider invocation count = %d, want 1 for one-shot batch exit", got)
	}
}
