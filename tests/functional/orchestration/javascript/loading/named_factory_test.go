// named_factory_test.go holds customer functional scenarios for named JavaScript
// Factory loading through the public CLI, HTTP API, and Factory Session controls.
package loading_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	namedJavaScriptFactoryName     = "named-javascript-loading"
	namedJavaScriptSuccessResult   = "named-javascript-loading:<NAMED_FACTORY_SUCCESS>"
)

// TestNamedJavaScriptFactoryRunsThroughStandardCLI proves a named JavaScript
// Factory resolves by name and completes through the public you run customer
// process boundary with a terminal COMPLETED primary outcome tied to the named
// Factory identity and without private VM internals in success diagnostics.
func TestNamedJavaScriptFactoryRunsThroughStandardCLI(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	sourceDir := scaffoldNamedInlineJavaScriptFactorySource(t)
	support.CreateNamedFactory(
		t,
		homeDir,
		sourceDir,
		namedJavaScriptFactoryName,
		filepath.Join(sourceDir, interfaces.FactoryConfigFile),
	)
	mockWorkersPath := writeEmptyMockWorkersConfig(t, sourceDir)

	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "run",
		"--named", namedJavaScriptFactoryName,
		"--with-mock-workers", mockWorkersPath,
		"--output", "primary",
		"--no-record",
		"hello",
	})
	inputs.Input.Env = append(inputs.Input.Env, "HOME="+homeDir, "USERPROFILE="+homeDir)
	inputs.Input.WorkingDirectory = t.TempDir()

	runner := support.NewRecordingCommandRunner("unexpected live provider execution")
	if err := support.BuildProcess(t, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}).Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute() error = %v\nstdout:\n%s\nstderr:\n%s", err, inputs.Stdout(), inputs.Stderr())
	}
	if inputs.Stderr() != "" {
		t.Fatalf("stderr = %q, want empty stderr on successful JSON invocation", inputs.Stderr())
	}
	if runner.CallCount() != 0 {
		t.Fatalf("provider command runner call count = %d, want 0 for named inline factory without child dispatch", runner.CallCount())
	}

	result := decodeSingleInvocationResponse(t, inputs.Stdout())
	assertNamedJavaScriptSuccessOutcome(t, result)
	assertNoPrivateJavaScriptVMDiagnostics(t, inputs.Stdout(), inputs.Stderr())
}

// TestNamedJavaScriptFactoryRunsThroughAPIInvocation proves the same named
// JavaScript Factory completes through the public HTTP sync execution customer
// entry path with a terminal COMPLETED primary outcome consistent with the CLI
// named-factory success path and without private VM internals in success diagnostics.
func TestNamedJavaScriptFactoryRunsThroughAPIInvocation(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	sourceDir := scaffoldNamedInlineJavaScriptFactorySource(t)

	runner := support.NewRecordingCommandRunner("unexpected live provider execution")
	namedFactoryDir := support.CreateNamedFactory(
		t,
		homeDir,
		sourceDir,
		namedJavaScriptFactoryName,
		filepath.Join(sourceDir, interfaces.FactoryConfigFile),
	)
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

	result := invokeNamedJavaScriptFactoryOverHTTP(t, server.URL(), "hello")
	assertNamedJavaScriptSessionSuccessOutcome(t, result)
	if runner.CallCount() != 0 {
		t.Fatalf("provider command runner call count = %d, want 0 for named inline factory without child dispatch", runner.CallCount())
	}

	responseJSON, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal sync execution response: %v", err)
	}
	assertNoPrivateJavaScriptVMDiagnostics(t, string(responseJSON))
}

func assertNamedJavaScriptSessionSuccessOutcome(
	t *testing.T,
	response factoryapi.FactorySessionSyncExecutionResponse,
) {
	t.Helper()

	if response.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("session status = %s, want SUCCEEDED", response.Status)
	}
	if response.Result == nil {
		t.Fatal("session result = nil, want FINAL primary outcome")
	}
	if response.Result.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("result status = %s, want FINAL", response.Result.ResultStatus)
	}
	if response.Result.PrimaryResult == nil || len(*response.Result.PrimaryResult) != 1 {
		t.Fatalf("primary result = %#v, want exactly one content part", response.Result.PrimaryResult)
	}
	part, err := (*response.Result.PrimaryResult)[0].AsWorkJsonContentPart()
	if err != nil {
		t.Fatalf("decode primary result content part: %v", err)
	}
	if got, ok := part.Json.(string); !ok || got != namedJavaScriptSuccessResult {
		t.Fatalf("primary result = %#v, want exact named-factory string %q", part.Json, namedJavaScriptSuccessResult)
	}
	if response.ResolvedSource.SourceRef == nil ||
		!strings.Contains(*response.ResolvedSource.SourceRef, namedJavaScriptFactoryName) {
		t.Fatalf(
			"resolved source ref = %#v, want named factory reference containing %q",
			response.ResolvedSource.SourceRef,
			namedJavaScriptFactoryName,
		)
	}
}

func scaffoldNamedInlineJavaScriptFactorySource(t *testing.T) string {
	t.Helper()

	return support.ScaffoldFactory(t, map[string]any{
		"name": namedJavaScriptFactoryName,
		"invocationSignature": map[string]any{
			"parameters": []any{map[string]any{
				"name": "prompt", "required": false,
				"bindings": []any{map[string]any{"kind": "POSITIONAL", "position": 1}},
			}},
		},
		"orchestrator": map[string]any{
			"kind": "JAVASCRIPT",
			"javascript": map[string]any{
				"inlineSource": map[string]any{
					"encoding": "utf-8",
					"inline":   `workflow.final("` + namedJavaScriptSuccessResult + `");`,
				},
				"argsSchema": map[string]any{
					"type":                 "object",
					"properties":           map[string]any{"prompt": map[string]any{"type": "string"}},
					"additionalProperties": false,
				},
			},
		},
	})
}

func assertNamedJavaScriptSuccessOutcome(t *testing.T, result factoryapi.InvocationResponse) {
	t.Helper()

	if result.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("terminal outcome = %s, want COMPLETED", result.Status)
	}
	if result.PrimaryResult == nil || len(*result.PrimaryResult) != 1 {
		t.Fatalf("primary result = %#v, want exactly one content part", result.PrimaryResult)
	}
	part, err := (*result.PrimaryResult)[0].AsWorkJsonContentPart()
	if err != nil {
		t.Fatalf("decode primary result content part: %v", err)
	}
	if got, ok := part.Json.(string); !ok || got != namedJavaScriptSuccessResult {
		t.Fatalf("primary result = %#v, want exact named-factory string %q", part.Json, namedJavaScriptSuccessResult)
	}
}

func invokeNamedJavaScriptFactoryOverHTTP(
	t *testing.T,
	baseURL string,
	prompt string,
) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()

	factoryID := namedJavaScriptFactoryName
	payload, err := json.Marshal(factoryapi.FactorySessionExecutionRequest{
		RequestId: "named-javascript-loading-api",
		Args:      &map[string]any{"prompt": prompt},
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
