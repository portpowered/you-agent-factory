package contracts

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	typedInputWorkflowFileName = "typed-input.workflow.js"
	typedInputWorkflowSource   = `return {
  label: args.label,
  count: args.count,
  enabled: args.enabled,
  metadata: args.metadata,
  tags: args.tags,
};`

	typedInputLabelValue    = "hello"
	typedInputRegionValue   = "us-west"
	typedInputTagAlphaValue = "alpha"
	typedInputTagBetaValue  = "beta"
)

var privateJavaScriptVMDiagnosticMarkers = []string{
	"goja",
	"goja.",
	"stack frame",
	"heap dump",
}

// TestJavaScriptInvocationReceivesStringNumberBooleanObjectAndArrayInputs proves
// string, number, boolean, object, and array Work Request inputs reach a
// JavaScript Factory invocation with preserved types on the public primary
// Factory Session result surface after a root-built process run.
func TestJavaScriptInvocationReceivesStringNumberBooleanObjectAndArrayInputs(t *testing.T) {
	t.Parallel()

	dir := scaffoldTypedInputMappingWorkflow(t)
	runner := support.NewRecordingCommandRunner("unexpected live provider execution")
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		UseMockWorkers:            true,
		Edges:                     serviceedges.Edges{ProviderCommandRunner: runner},
	})
	t.Cleanup(func() { server.Stop(t) })

	started := startTypedInputMappingWorkflow(t, server.URL(), dir)
	if started.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("session status = %q, want SUCCEEDED", started.Status)
	}
	if runner.CallCount() != 0 {
		t.Fatalf("provider command runner call count = %d, want 0 for typed-input echo workflow", runner.CallCount())
	}

	assertTypedInputMappingPrimaryResult(t, started.Result)
	assertNoPrivateJavaScriptVMDiagnostics(t, marshalPrimaryResultForDiagnostics(t, started.Result))
}

func scaffoldTypedInputMappingWorkflow(t *testing.T) string {
	t.Helper()

	dir := support.ScaffoldFactory(t, map[string]any{
		"name": "javascript-input-mapping",
		"orchestrator": map[string]any{
			"kind": "JAVASCRIPT",
			"javascript": map[string]any{
				"sourceRef": typedInputWorkflowFileName,
				"argsSchema": map[string]any{
					"type":     "object",
					"required": []any{"label", "count", "enabled", "metadata", "tags"},
					"properties": map[string]any{
						"label":    map[string]any{"type": "string"},
						"count":    map[string]any{"type": "number"},
						"enabled":  map[string]any{"type": "boolean"},
						"metadata": map[string]any{"type": "object"},
						"tags": map[string]any{
							"type":  "array",
							"items": map[string]any{"type": "string"},
						},
					},
					"additionalProperties": false,
				},
			},
		},
	})
	if err := os.WriteFile(
		filepath.Join(dir, typedInputWorkflowFileName),
		[]byte(typedInputWorkflowSource),
		0o600,
	); err != nil {
		t.Fatalf("write typed input workflow: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(dir, "mock-workers.json"),
		[]byte(`{"mockWorkers":[]}`),
		0o600,
	); err != nil {
		t.Fatalf("write mock-workers config: %v", err)
	}
	return dir
}

func startTypedInputMappingWorkflow(
	t *testing.T,
	serverURL, dir string,
) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()

	workflowPath := filepath.Join(dir, typedInputWorkflowFileName)
	args := map[string]any{
		"label":    typedInputLabelValue,
		"count":    42,
		"enabled":  true,
		"metadata": map[string]any{"region": typedInputRegionValue},
		"tags":     []any{typedInputTagAlphaValue, typedInputTagBetaValue},
	}
	payload, err := json.Marshal(factoryapi.FactorySessionExecutionRequest{
		RequestId: "javascript-typed-input-mapping",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowFile,
			WorkflowFile: &workflowPath,
		},
		Args: &args,
	})
	if err != nil {
		t.Fatalf("marshal typed input workflow request: %v", err)
	}
	endpoint := strings.TrimSuffix(serverURL, "/") + "/factory-sessions/sync"
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build typed input workflow request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("start typed input workflow: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		var body bytes.Buffer
		_, _ = body.ReadFrom(response.Body)
		t.Fatalf("start typed input workflow status = %d: %s", response.StatusCode, body.String())
	}
	var started factoryapi.FactorySessionSyncExecutionResponse
	if err := json.NewDecoder(response.Body).Decode(&started); err != nil {
		t.Fatalf("decode typed input workflow response: %v", err)
	}
	return started
}

func assertTypedInputMappingPrimaryResult(t *testing.T, result *factoryapi.FactorySessionResult) {
	t.Helper()

	if result == nil || result.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("result = %#v, want FINAL Factory Session result", result)
	}
	if result.PrimaryResult == nil || len(*result.PrimaryResult) != 1 {
		t.Fatalf("primary result = %#v, want exactly one content part", result.PrimaryResult)
	}
	part, err := (*result.PrimaryResult)[0].AsWorkJsonContentPart()
	if err != nil {
		t.Fatalf("decode primary result content part: %v", err)
	}
	encoded, err := json.Marshal(part.Json)
	if err != nil {
		t.Fatalf("encode primary result JSON: %v", err)
	}
	var evidence struct {
		Label    string         `json:"label"`
		Count    float64        `json:"count"`
		Enabled  bool           `json:"enabled"`
		Metadata map[string]any `json:"metadata"`
		Tags     []string       `json:"tags"`
	}
	if err := json.Unmarshal(encoded, &evidence); err != nil {
		t.Fatalf("decode typed input primary result: %v", err)
	}
	if evidence.Label != typedInputLabelValue {
		t.Fatalf("mapped label = %q, want %q", evidence.Label, typedInputLabelValue)
	}
	if evidence.Count != 42 {
		t.Fatalf("mapped count = %v, want 42", evidence.Count)
	}
	if !evidence.Enabled {
		t.Fatalf("mapped enabled = %v, want true", evidence.Enabled)
	}
	if evidence.Metadata == nil || evidence.Metadata["region"] != typedInputRegionValue {
		t.Fatalf("mapped metadata = %#v, want region %q", evidence.Metadata, typedInputRegionValue)
	}
	if len(evidence.Tags) != 2 ||
		evidence.Tags[0] != typedInputTagAlphaValue ||
		evidence.Tags[1] != typedInputTagBetaValue {
		t.Fatalf("mapped tags = %#v, want [%q, %q]", evidence.Tags, typedInputTagAlphaValue, typedInputTagBetaValue)
	}
}

func marshalPrimaryResultForDiagnostics(t *testing.T, result *factoryapi.FactorySessionResult) string {
	t.Helper()

	if result == nil || result.PrimaryResult == nil {
		return ""
	}
	encoded, err := json.Marshal(result.PrimaryResult)
	if err != nil {
		t.Fatalf("marshal primary result for diagnostics: %v", err)
	}
	return string(encoded)
}

func assertNoPrivateJavaScriptVMDiagnostics(t *testing.T, outputs ...string) {
	t.Helper()

	combined := strings.ToLower(strings.Join(outputs, "\n"))
	for _, marker := range privateJavaScriptVMDiagnosticMarkers {
		if strings.Contains(combined, strings.ToLower(marker)) {
			t.Fatalf("diagnostics exposed private VM detail %q in %q", marker, strings.Join(outputs, "\n---\n"))
		}
	}
}
