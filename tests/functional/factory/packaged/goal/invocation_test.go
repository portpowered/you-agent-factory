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
func TestPackagedGoalQuietCLIBatchReturnsPrimaryResultOnStdout(t *testing.T) {
	scaffoldPackagedGoalBuiltInFactory(t)
	mockWorkersPath := writePackagedGoalBuiltinMockWorkersConfig(t)
	goalText := fmt.Sprintf("functional-packaged-goal-quiet-cli-%d", time.Now().UnixNano())

	stdout, stderr := runPackagedGoalQuietCLIBatch(t, mockWorkersPath, goalText)
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
func TestPackagedGoalQuietCLIBatchExitsWithoutContinuousMode(t *testing.T) {
	scaffoldPackagedGoalBuiltInFactory(t)
	mockWorkersPath := writePackagedGoalBuiltinMockWorkersConfig(t)
	goalText := fmt.Sprintf("functional-packaged-goal-quiet-exit-%d", time.Now().UnixNano())

	if err := runPackagedGoalQuietCLIBatchWithTimeout(t, mockWorkersPath, goalText, 20*time.Second); err != nil {
		t.Fatalf("packaged goal quiet CLI batch invocation: %v", err)
	}
}
