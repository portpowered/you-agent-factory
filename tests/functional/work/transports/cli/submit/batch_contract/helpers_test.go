package batch_contract_test

import (
	"bytes"
	"encoding/json"
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

	successHumanSubmitBatchRequestID = "work-cli-submit-batch-success-human"
	successJSONSubmitBatchRequestID  = "work-cli-submit-batch-success-json"
	successSubmitBatchWorkName       = "success-review"
	successSubmitBatchWorkType       = "task"
)

type batchContractSubmitJSON struct {
	RequestID     string `json:"requestId"`
	TraceID       string `json:"traceId"`
	WorkCount     int    `json:"workCount"`
	SessionID     string `json:"sessionId"`
	EndpointPath  string `json:"endpointPath"`
	BatchSource   string `json:"batchSource"`
	Works         []struct {
		Name         string `json:"name"`
		WorkTypeName string `json:"workTypeName"`
		WorkID       string `json:"workId"`
	} `json:"works"`
}

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

func successHumanInlineBatchJSON() string {
	return inlineBatchJSON(
		successHumanSubmitBatchRequestID,
		successSubmitBatchWorkName,
		successSubmitBatchWorkType,
		"Success human",
	)
}

func successJSONInlineBatchJSON() string {
	return inlineBatchJSON(
		successJSONSubmitBatchRequestID,
		successSubmitBatchWorkName,
		successSubmitBatchWorkType,
		"Success JSON",
	)
}

func invalidInlineBatchJSON() string {
	return `{not-json`
}

func successSubmitBatchFactoryConfig() map[string]any {
	return map[string]any{
		"name": "work-cli-submit-batch-contract-success",
		"workTypes": []map[string]any{
			{
				"name": successSubmitBatchWorkType,
				"states": []map[string]any{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []map[string]string{
			{"name": "mock-worker"},
		},
		"workstations": []map[string]any{
			{
				"name":      "process-task",
				"worker":    "mock-worker",
				"inputs":    []map[string]string{{"workType": successSubmitBatchWorkType, "state": "init"}},
				"outputs":   []map[string]string{{"workType": successSubmitBatchWorkType, "state": "complete"}},
				"onFailure": []map[string]string{{"workType": successSubmitBatchWorkType, "state": "failed"}},
			},
		},
	}
}

func decodeSubmitBatchJSONResult(t *testing.T, output string) batchContractSubmitJSON {
	t.Helper()

	var submitted batchContractSubmitJSON
	if err := json.Unmarshal(bytes.TrimSpace([]byte(output)), &submitted); err != nil {
		t.Fatalf("decode submit batch JSON: %v\noutput:\n%s", err, output)
	}
	return submitted
}

func assertSubmitBatchHumanSuccess(t *testing.T, output, requestID, workName, workType string) {
	t.Helper()

	for _, marker := range []string{
		"requestId: " + requestID,
		"traceId:",
		"work count: 1",
		workName + " (" + workType + ")",
		"workId=",
	} {
		if !strings.Contains(output, marker) {
			t.Fatalf("submit batch human output missing %q:\n%s", marker, output)
		}
	}
}

func assertSubmitBatchJSONSuccess(t *testing.T, submitted batchContractSubmitJSON, requestID, workName, workType string) {
	t.Helper()

	if submitted.RequestID != requestID {
		t.Fatalf("submit batch JSON requestId = %q, want %q", submitted.RequestID, requestID)
	}
	if strings.TrimSpace(submitted.TraceID) == "" {
		t.Fatalf("submit batch JSON missing traceId: %#v", submitted)
	}
	if submitted.WorkCount != 1 {
		t.Fatalf("submit batch JSON workCount = %d, want 1", submitted.WorkCount)
	}
	if strings.TrimSpace(submitted.SessionID) == "" {
		t.Fatalf("submit batch JSON missing sessionId: %#v", submitted)
	}
	if strings.TrimSpace(submitted.EndpointPath) == "" {
		t.Fatalf("submit batch JSON missing endpointPath: %#v", submitted)
	}
	if submitted.BatchSource != "inline" {
		t.Fatalf("submit batch JSON batchSource = %q, want inline", submitted.BatchSource)
	}
	if len(submitted.Works) != 1 {
		t.Fatalf("submit batch JSON works = %#v, want one accepted work", submitted.Works)
	}
	work := submitted.Works[0]
	if work.Name != workName {
		t.Fatalf("submit batch JSON work name = %q, want %q", work.Name, workName)
	}
	if work.WorkTypeName != workType {
		t.Fatalf("submit batch JSON work type = %q, want %q", work.WorkTypeName, workType)
	}
	if strings.TrimSpace(work.WorkID) == "" {
		t.Fatalf("submit batch JSON missing accepted workId: %#v", work)
	}
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
