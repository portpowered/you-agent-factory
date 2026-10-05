package admission_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	submissionActivationWorkType = "task"

	submissionActivationHTTPBatchRequestID = "fun-work-submission-http-batch"
	submissionActivationHTTPBatchWorkName  = "fun-work-http-batch-task"
	submissionActivationHTTPBatchWorkID    = "fun-work-http-batch-id"

	submissionActivationHTTPUnaryWorkName = "fun-work-http-unary-task"

	submissionActivationCLIUnaryWorkName  = "fun-work-cli-unary-task"
	submissionActivationCLIBatchRequestID = "fun-work-cli-batch"
	submissionActivationCLIBatchWorkName  = "fun-work-cli-batch-task"
)

// TestCLISubmitCreatesWorkVisibleThroughREST proves CLI and HTTP submissions
// create named Work with identities that a customer can retrieve through REST.
func TestCLISubmitCreatesWorkVisibleThroughREST(t *testing.T) {
	t.Parallel()

	edges := serviceedges.Edges{ProviderCommandRunner: support.NewStaticSuccessCommandRunner("COMPLETE")}

	dir := support.ScaffoldFactory(t, submissionActivationFactoryConfig())
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Edges:                     edges,
	})
	t.Cleanup(func() { server.Stop(t) })

	baseURL := server.URL()

	workTypeName := submissionActivationWorkType
	batchSubmitted := support.UpsertDefaultSessionWorkRequest(t, baseURL, factoryapi.WorkRequest{
		RequestId: submissionActivationHTTPBatchRequestID,
		Type:      factoryapi.WorkRequestTypeFactoryRequestBatch,
		Works: &[]factoryapi.Work{{
			Name:         submissionActivationHTTPBatchWorkName,
			WorkId:       stringPtr(submissionActivationHTTPBatchWorkID),
			WorkTypeName: &workTypeName,
			Payload:      map[string]string{"title": "FUN Work HTTP batch activation"},
		}},
	})
	if batchSubmitted.RequestId != submissionActivationHTTPBatchRequestID {
		t.Fatalf(
			"PUT /work-requests requestId = %q, want %q",
			batchSubmitted.RequestId,
			submissionActivationHTTPBatchRequestID,
		)
	}
	assertSubmissionActivationWorkListed(
		t,
		baseURL,
		submissionActivationHTTPBatchWorkName,
		submissionActivationHTTPBatchWorkID,
	)

	unarySubmitted := support.SubmitDefaultSessionWork(t, baseURL, factoryapi.SubmitWorkRequest{
		Name:         stringPtr(submissionActivationHTTPUnaryWorkName),
		WorkTypeName: submissionActivationWorkType,
		Payload:      map[string]string{"title": "FUN Work HTTP unary activation"},
	})
	unaryWorkID := support.StringPointerValue(unarySubmitted.WorkId)
	if unaryWorkID == "" {
		t.Fatalf("POST /work workId is empty, want customer-visible work identity")
	}
	assertSubmissionActivationWorkListed(
		t,
		baseURL,
		submissionActivationHTTPUnaryWorkName,
		unaryWorkID,
	)

	process := support.BuildProcess(t, edges)
	payloadPath := writeSubmissionActivationPayloadFile(t, "# FUN Work CLI unary activation\n")

	cliUnaryOutput := executeSubmissionActivationUnarySubmitCLI(
		t,
		process,
		baseURL,
		submissionActivationCLIUnaryWorkName,
		payloadPath,
	)
	cliUnarySubmitted := decodeSubmissionActivationUnarySubmitJSON(
		t,
		cliUnaryOutput,
		submissionActivationCLIUnaryWorkName,
	)
	assertSubmissionActivationWorkListed(
		t,
		baseURL,
		submissionActivationCLIUnaryWorkName,
		*cliUnarySubmitted.WorkID,
	)

	cliBatchOutput := executeSubmissionActivationBatchSubmitCLI(
		t,
		process,
		baseURL,
		submissionActivationInlineBatchJSON(),
	)
	cliBatchSubmitted := decodeSubmissionActivationBatchSubmitJSON(t, cliBatchOutput)
	if cliBatchSubmitted.RequestID != submissionActivationCLIBatchRequestID {
		t.Fatalf(
			"CLI submit batch requestId = %q, want %q",
			cliBatchSubmitted.RequestID,
			submissionActivationCLIBatchRequestID,
		)
	}
	assertSubmissionActivationWorkListed(
		t,
		baseURL,
		submissionActivationCLIBatchWorkName,
		cliBatchSubmitted.Works[0].WorkID,
	)

}

func submissionActivationFactoryConfig() map[string]any {
	return map[string]any{
		"name": "work-submission-activation",
		"workTypes": []map[string]any{
			{
				"name": submissionActivationWorkType,
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
				"inputs":    []map[string]string{{"workType": submissionActivationWorkType, "state": "init"}},
				"outputs":   []map[string]string{{"workType": submissionActivationWorkType, "state": "complete"}},
				"onFailure": []map[string]string{{"workType": submissionActivationWorkType, "state": "failed"}},
			},
		},
	}
}

func assertSubmissionActivationWorkListed(t *testing.T, baseURL, workName, workID string) {
	t.Helper()

	listed := support.ListDefaultSessionWork(t, baseURL)
	for _, item := range listed.Results {
		if item.Name != workName {
			continue
		}
		if support.StringPointerValue(item.WorkId) == workID {
			if support.StringPointerValue(item.WorkTypeName) != submissionActivationWorkType {
				t.Fatalf(
					"GET /work list workTypeName = %q, want %q for name=%q workId=%q",
					support.StringPointerValue(item.WorkTypeName),
					submissionActivationWorkType,
					workName,
					workID,
				)
			}
			return
		}
	}
	t.Fatalf(
		"GET /work list missing submitted work name=%q workId=%q: %#v",
		workName,
		workID,
		listed.Results,
	)
}

func writeSubmissionActivationPayloadFile(t *testing.T, content string) string {
	t.Helper()
	payloadPath := filepath.Join(t.TempDir(), "request.md")
	if err := os.WriteFile(payloadPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write unary payload file: %v", err)
	}
	return payloadPath
}

func executeSubmissionActivationUnarySubmitCLI(
	t *testing.T,
	process support.Process,
	serverURL string,
	workName string,
	payloadPath string,
) string {
	t.Helper()

	home := t.TempDir()
	args := []string{
		"you", "--server", serverURL, "--json",
		"submit",
		"--name", workName,
		"--work-type-name", submissionActivationWorkType,
		"--payload", payloadPath,
	}
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = submissionActivationHomeEnvironment(home)
	inputs.Input.WorkingDirectory = home
	stdinIsTTY := true
	inputs.Input.StdinIsTTY = &stdinIsTTY
	inputs.Input.Stdin = strings.NewReader("")
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf(
			"Process.Execute(unary submit) error = %v\nstdout:\n%s\nstderr:\n%s",
			err,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}
	return inputs.Stdout()
}

func executeSubmissionActivationBatchSubmitCLI(
	t *testing.T,
	process support.Process,
	serverURL string,
	batchJSON string,
) string {
	t.Helper()

	home := t.TempDir()
	args := []string{
		"you", "--server", serverURL, "--json",
		"submit", "batch", batchJSON,
	}
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = submissionActivationHomeEnvironment(home)
	inputs.Input.WorkingDirectory = home
	stdinIsTTY := true
	inputs.Input.StdinIsTTY = &stdinIsTTY
	inputs.Input.Stdin = strings.NewReader("")
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf(
			"Process.Execute(batch submit) error = %v\nstdout:\n%s\nstderr:\n%s",
			err,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}
	return inputs.Stdout()
}

type submissionActivationUnarySubmitJSON struct {
	TraceID string  `json:"traceId"`
	WorkID  *string `json:"workId"`
	Name    string  `json:"name"`
}

type submissionActivationBatchSubmitJSON struct {
	RequestID string `json:"requestId"`
	TraceID   string `json:"traceId"`
	WorkCount int    `json:"workCount"`
	Works     []struct {
		Name   string `json:"name"`
		WorkID string `json:"workId"`
	} `json:"works"`
}

func decodeSubmissionActivationUnarySubmitJSON(
	t *testing.T,
	output string,
	workName string,
) submissionActivationUnarySubmitJSON {
	t.Helper()

	var submitted submissionActivationUnarySubmitJSON
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &submitted); err != nil {
		t.Fatalf("decode unary submit JSON: %v\noutput:\n%s", err, output)
	}
	if submitted.TraceID == "" {
		t.Fatalf("unary submit response missing traceId: %#v", submitted)
	}
	if submitted.WorkID == nil || strings.TrimSpace(*submitted.WorkID) == "" {
		t.Fatalf("unary submit response missing workId: %#v", submitted)
	}
	if submitted.Name != workName {
		t.Fatalf("unary submit response name = %q, want %q", submitted.Name, workName)
	}
	return submitted
}

func decodeSubmissionActivationBatchSubmitJSON(
	t *testing.T,
	output string,
) submissionActivationBatchSubmitJSON {
	t.Helper()

	var submitted submissionActivationBatchSubmitJSON
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &submitted); err != nil {
		t.Fatalf("decode batch submit JSON: %v\noutput:\n%s", err, output)
	}
	if submitted.WorkCount != 1 || len(submitted.Works) != 1 || strings.TrimSpace(submitted.Works[0].WorkID) == "" {
		t.Fatalf("batch submit response missing accepted work identity: %#v", submitted)
	}
	if strings.TrimSpace(submitted.RequestID) == "" || strings.TrimSpace(submitted.TraceID) == "" {
		t.Fatalf("batch submit response missing request or trace identity: %#v", submitted)
	}
	return submitted
}

func submissionActivationInlineBatchJSON() string {
	return `{
		"requestId": "` + submissionActivationCLIBatchRequestID + `",
		"type": "FACTORY_REQUEST_BATCH",
		"works": [
			{
				"name": "` + submissionActivationCLIBatchWorkName + `",
				"workTypeName": "` + submissionActivationWorkType + `",
				"payload": {"title": "FUN Work CLI batch activation"}
			}
		]
	}`
}

func submissionActivationHomeEnvironment(home string) []string {
	if runtime.GOOS == "windows" {
		return []string{"USERPROFILE=" + home}
	}
	if runtime.GOOS == "plan9" {
		return []string{"home=" + home}
	}
	return []string{"HOME=" + home}
}

func stringPtr(value string) *string {
	return &value
}
