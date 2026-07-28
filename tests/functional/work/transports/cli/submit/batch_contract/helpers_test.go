package batch_contract_test

import (
	"runtime"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	harnessSubmitBatchRequestID = "work-cli-submit-batch-contract-harness"
	harnessSubmitBatchWorkName  = "harness-review"
	harnessSubmitBatchWorkType  = "task"
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
	return `{
		"requestId": "` + requestID + `",
		"type": "FACTORY_REQUEST_BATCH",
		"works": [
			{"name": "` + harnessSubmitBatchWorkName + `", "workTypeName": "` + harnessSubmitBatchWorkType + `", "payload": {"title": "Harness"}}
		]
	}`
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
