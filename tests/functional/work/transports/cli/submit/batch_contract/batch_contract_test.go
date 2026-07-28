package batch_contract_test

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
)

// TestCLISubmitBatchDryRunEmitsSummaryWithoutMutation proves you submit batch
// --dry-run prints a readable summary of the parsed batch and does not upsert
// Work: an instrumented Factory Session server receives no HTTP traffic while
// output includes request identity, batch source, and dry-run markers.
func TestCLISubmitBatchDryRunEmitsSummaryWithoutMutation(t *testing.T) {
	serverURL, requests := newInstrumentedSubmitBatchServer(t)
	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{})
	process := buildBatchContractProcess(t, serviceedges.Edges{
		ProviderCommandRunner: runner,
	})

	stdout := executeSubmitBatchCLI(t, process, []string{
		"you", "--server", serverURL,
		"submit", "batch", "--dry-run", dryRunInlineBatchJSON(),
	})

	for _, marker := range []string{
		"requestId: " + dryRunSubmitBatchRequestID,
		"batchSource: inline",
		"work count: 1",
		"works: " + dryRunSubmitBatchWorkName,
		"dry-run: no request sent",
	} {
		if !strings.Contains(stdout, marker) {
			t.Fatalf("submit batch dry-run output missing %q: %q", marker, stdout)
		}
	}
	if requests.Load() != 0 {
		t.Fatalf("submit batch dry-run sent %d HTTP requests, want 0", requests.Load())
	}
}

// TestCLISubmitBatchContractHarnessExecutesThroughRootBuildProcess proves the
// Work-owned batch_contract cell constructs a customer process through
// root.BuildProcess, invokes public you submit batch through Process.Execute,
// and replaces external effects only through edges.Edges.
func TestCLISubmitBatchContractHarnessExecutesThroughRootBuildProcess(t *testing.T) {
	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{})
	process := buildBatchContractProcess(t, serviceedges.Edges{
		ProviderCommandRunner: runner,
	})

	stdout := executeSubmitBatchCLI(t, process, []string{
		"you", "--server", "http://127.0.0.1:1",
		"submit", "batch", "--dry-run", harnessInlineBatchJSON(harnessSubmitBatchRequestID),
	})

	for _, marker := range []string{
		"requestId: " + harnessSubmitBatchRequestID,
		"batchSource: inline",
		"dry-run: no request sent",
	} {
		if !strings.Contains(stdout, marker) {
			t.Fatalf("submit batch dry-run output missing %q: %q", marker, stdout)
		}
	}
}
