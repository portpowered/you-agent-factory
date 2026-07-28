package runtime_api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	jsStructuredInputFactoryName   = "js-structured-input-factory"
	jsStructuredInputSuccessResult = "js-structured-input:<SYNC_SUCCESS>"
)

// TestNamedJavaScriptFactoryRunResolvesInvocationInputThroughCLI exercises
// ResolveInvocationInput on the invocation service when a named JavaScript Factory
// runs through the public you run customer boundary with structured args.
func TestNamedJavaScriptFactoryRunResolvesInvocationInputThroughCLI(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	sourceDir := scaffoldJavaScriptStructuredInputFactory(t)
	support.CreateNamedFactory(
		t,
		homeDir,
		sourceDir,
		jsStructuredInputFactoryName,
		filepath.Join(sourceDir, interfaces.FactoryConfigFile),
	)
	mockWorkersPath := writeEmptyMockWorkersConfig(t, sourceDir)

	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "run",
		"--named", jsStructuredInputFactoryName,
		"--with-mock-workers", mockWorkersPath,
		"--output", "primary",
		"--no-record",
		"structured-cli",
	})
	inputs.Input.Env = append(inputs.Input.Env, "HOME="+homeDir, "USERPROFILE="+homeDir)
	inputs.Input.WorkingDirectory = t.TempDir()

	runner := support.NewRecordingCommandRunner("unexpected live provider execution")
	if err := support.BuildProcess(t, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}).Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute() error = %v\nstdout:\n%s\nstderr:\n%s", err, inputs.Stdout(), inputs.Stderr())
	}
	if runner.CallCount() != 0 {
		t.Fatalf("provider command runner call count = %d, want 0 for inline JavaScript workflow", runner.CallCount())
	}

	result := decodeJavaScriptInvocationResponse(t, inputs.Stdout())
	if result.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("invocation status = %q, want COMPLETED", result.Status)
	}
	if result.PrimaryResult == nil || len(*result.PrimaryResult) != 1 {
		t.Fatalf("primary result = %#v, want one content part", result.PrimaryResult)
	}
	part, err := (*result.PrimaryResult)[0].AsWorkJsonContentPart()
	if err != nil {
		t.Fatalf("decode primary result: %v", err)
	}
	if got, ok := part.Json.(string); !ok || got != jsStructuredInputSuccessResult {
		t.Fatalf("primary result = %#v, want %q", part.Json, jsStructuredInputSuccessResult)
	}
}

func writeEmptyMockWorkersConfig(t *testing.T, factoryDir string) string {
	t.Helper()
	path := filepath.Join(factoryDir, "mock-workers.json")
	if err := os.WriteFile(path, []byte(`{"mockWorkers":[]}`), 0o600); err != nil {
		t.Fatalf("write mock-workers config: %v", err)
	}
	return path
}

func decodeJavaScriptInvocationResponse(t *testing.T, stdout string) factoryapi.InvocationResponse {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) == 0 {
		t.Fatalf("stdout is empty, want JSON invocation response")
	}
	var response factoryapi.InvocationResponse
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &response); err != nil {
		t.Fatalf("decode invocation response: %v\noutput:\n%s", err, stdout)
	}
	return response
}

// TestJavaScriptSyncExecutionResolvesStructuredInvocationInput exercises
// invocation input normalization through ResolveInvocationInput on JavaScript
// orchestrator sync execution when structured args are supplied.
func TestJavaScriptSyncExecutionResolvesStructuredInvocationInput(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	sourceDir := scaffoldJavaScriptStructuredInputFactory(t)
	namedFactoryDir := support.CreateNamedFactory(
		t,
		homeDir,
		sourceDir,
		jsStructuredInputFactoryName,
		filepath.Join(sourceDir, interfaces.FactoryConfigFile),
	)

	runner := support.NewRecordingCommandRunner("unexpected live provider execution")
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                namedFactoryDir,
		WorkingDirectory:          t.TempDir(),
		WaitForServiceModeRuntime: true,
		UseMockWorkers:            true,
		Env:                       []string{"HOME=" + homeDir, "USERPROFILE=" + homeDir},
		Edges: serviceedges.Edges{
			ProviderCommandRunner: runner,
			FactoryRuntimeWorkflowHome: func() (string, error) {
				return homeDir, nil
			},
		},
	})
	t.Cleanup(func() { server.Stop(t) })

	response := postJavaScriptSyncExecution(t, server.URL(), map[string]any{"input": "structured-sync"})
	if response.Result == nil || response.Result.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("sync result = %#v, want FINAL primary outcome", response.Result)
	}
	if response.Result.PrimaryResult == nil || len(*response.Result.PrimaryResult) != 1 {
		t.Fatalf("primary result = %#v, want one content part", response.Result.PrimaryResult)
	}
	part, err := (*response.Result.PrimaryResult)[0].AsWorkJsonContentPart()
	if err != nil {
		t.Fatalf("decode primary result: %v", err)
	}
	if got, ok := part.Json.(string); !ok || got != jsStructuredInputSuccessResult {
		t.Fatalf("primary result = %#v, want %q", part.Json, jsStructuredInputSuccessResult)
	}
	if runner.CallCount() != 0 {
		t.Fatalf("provider command runner call count = %d, want 0 for inline JavaScript workflow", runner.CallCount())
	}
}

func scaffoldJavaScriptStructuredInputFactory(t *testing.T) string {
	t.Helper()

	return support.ScaffoldFactory(t, map[string]any{
		"name": jsStructuredInputFactoryName,
		"invocationSignature": map[string]any{
			"parameters": []any{map[string]any{
				"name":     "input",
				"required": true,
				"bindings": []any{map[string]any{"kind": "POSITIONAL", "position": 1}},
			}},
		},
		"orchestrator": map[string]any{
			"kind": "JAVASCRIPT",
			"javascript": map[string]any{
				"inlineSource": map[string]any{
					"encoding": "utf-8",
					"inline":   `workflow.final("` + jsStructuredInputSuccessResult + `");`,
				},
				"argsSchema": map[string]any{
					"type":                 "object",
					"properties":           map[string]any{"input": map[string]any{"type": "string"}},
					"additionalProperties": false,
				},
			},
		},
	})
}

func postJavaScriptSyncExecution(
	t *testing.T,
	baseURL string,
	args map[string]any,
) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()

	factoryID := jsStructuredInputFactoryName
	payload, err := json.Marshal(factoryapi.FactorySessionExecutionRequest{
		RequestId: "js-structured-input-sync",
		Args:      &args,
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:      factoryapi.FactorySessionExecutionSourceKindFactoryId,
			FactoryId: &factoryID,
		},
	})
	if err != nil {
		t.Fatalf("marshal sync execution request: %v", err)
	}

	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/sync"
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build sync execution request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		var responseBody bytes.Buffer
		_, _ = responseBody.ReadFrom(response.Body)
		t.Fatalf("POST %s status = %d, want 200: %s", endpoint, response.StatusCode, responseBody.String())
	}

	var result factoryapi.FactorySessionSyncExecutionResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("decode sync execution response: %v", err)
	}
	return result
}
