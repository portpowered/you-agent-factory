package batch_contract_test

import (
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	harnessSubmitBatchRequestID = "work-cli-submit-batch-contract-harness"
	harnessSubmitBatchWorkName  = "harness-review"
	harnessSubmitBatchWorkType  = "task"

	dryRunSubmitBatchRequestID = "work-cli-submit-batch-dry-run"
	dryRunSubmitBatchWorkName  = "dry-run-review"
	dryRunSubmitBatchWorkType  = "task"
)

func buildBatchContractProcess(t *testing.T, edges serviceedges.Edges) support.Process {
	t.Helper()
	return support.BuildProcess(t, edges)
}

func executeSubmitBatchCLI(t *testing.T, process support.Process, args []string) string {
	t.Helper()
	home := t.TempDir()
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = batchContractHomeEnvironment(home)
	inputs.Input.WorkingDirectory = home
	stdinIsTTY := true
	inputs.Input.StdinIsTTY = &stdinIsTTY
	inputs.Input.Stdin = strings.NewReader("")
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(%v) error = %v\nstderr:\n%s", args, err, inputs.Stderr())
	}
	return inputs.Stdout()
}

func executeSubmitBatchCLIExpectError(t *testing.T, process support.Process, args []string) (stdout, stderr string, err error) {
	t.Helper()
	home := t.TempDir()
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = batchContractHomeEnvironment(home)
	inputs.Input.WorkingDirectory = home
	stdinIsTTY := true
	inputs.Input.StdinIsTTY = &stdinIsTTY
	inputs.Input.Stdin = strings.NewReader("")
	err = process.Execute(inputs.Input)
	return inputs.Stdout(), inputs.Stderr(), err
}

func harnessInlineBatchJSON(requestID string) string {
	return inlineBatchJSON(requestID, harnessSubmitBatchWorkName, harnessSubmitBatchWorkType, "Harness")
}

func dryRunInlineBatchJSON() string {
	return inlineBatchJSON(dryRunSubmitBatchRequestID, dryRunSubmitBatchWorkName, dryRunSubmitBatchWorkType, "Dry run")
}

func inlineBatchJSON(requestID, workName, workType, title string) string {
	return `{
		"requestId": "` + requestID + `",
		"type": "FACTORY_REQUEST_BATCH",
		"works": [
			{"name": "` + workName + `", "workTypeName": "` + workType + `", "payload": {"title": "` + title + `"}}
		]
	}`
}

func newInstrumentedSubmitBatchServer(t *testing.T) (serverURL string, requests *atomic.Int32) {
	t.Helper()
	var count atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		t.Errorf("submit batch dry-run must not send HTTP requests; got %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	return server.URL, &count
}

func batchContractHomeEnvironment(home string) []string {
	if runtime.GOOS == "windows" {
		return []string{"USERPROFILE=" + home}
	}
	if runtime.GOOS == "plan9" {
		return []string{"home=" + home}
	}
	return []string{"HOME=" + home}
}
