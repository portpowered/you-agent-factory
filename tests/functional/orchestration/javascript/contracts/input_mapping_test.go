package contracts

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

	missingRequiredInputWorkflowSource = `return (async function () {
  const child = await agent.run({
    prompt: "typed-input-child",
    label: "missing-required-child",
    model: "gpt-allowed",
  });
  return {
    label: args.label,
    count: args.count,
    enabled: args.enabled,
    metadata: args.metadata,
    tags: args.tags,
    child: child,
  };
})();`

	stableMissingRequiredInputDiagnostic = `required invocation parameter "label" is missing`
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
func runJavaScriptInvocationReceivesStringNumberBooleanObjectAndArrayInputs(
	t *testing.T,
	fixture *contractFixture,
) {
	dir := scaffoldTypedInputMappingWorkflow(t)
	providerCalls := fixture.providerCallCount()
	started := startTypedInputMappingWorkflow(t, fixture, dir, fixture.nextRequestID("typed-input"))
	if started.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("session status = %q, want SUCCEEDED", started.Status)
	}
	if got := fixture.providerCallCount(); got != providerCalls {
		t.Fatalf("provider command runner call count = %d, want unchanged at %d for typed-input echo workflow", got, providerCalls)
	}

	assertTypedInputMappingPrimaryResult(t, started.Result)
	assertNoPrivateJavaScriptVMDiagnostics(t, marshalPrimaryResultForDiagnostics(t, started.Result))
}

// TestJavaScriptMissingRequiredInputFailsBeforeChildDispatch proves omitting a
// required JavaScript Factory request input fails with an actionable customer
// diagnostic before any child worker or provider dispatch when invoked through
// the public you run customer process boundary after a root-built process run.
func runJavaScriptMissingRequiredInputFailsBeforeChildDispatch(
	t *testing.T,
	fixture *contractFixture,
) {
	dir := scaffoldMissingRequiredInputMappingFactory(t)
	run := runMissingRequiredInputJavaScriptInvocation(t, fixture, dir)

	assertMissingRequiredInputInvocationOutcome(t, run.outcome)
	if got := fixture.providerCallCount(); got != 0 {
		t.Fatalf(
			"provider command runner call count = %d, want 0 before missing-required-input validation",
			got,
		)
	}
	assertNoPrivateJavaScriptVMDiagnostics(t, run.outcome.diagnostic)

	// The rejected CLI request must not poison the next explicit Factory
	// Session. Use a fresh authored root so this is also a direct witness for
	// source and mutable-runtime isolation after the adverse path.
	recoveryDir := scaffoldTypedInputMappingWorkflow(t)
	recovered := startTypedInputMappingWorkflow(
		t,
		fixture,
		recoveryDir,
		fixture.nextRequestID("missing-input-recovery"),
	)
	if recovered.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("post-missing-input session status = %q, want SUCCEEDED", recovered.Status)
	}
	assertTypedInputMappingPrimaryResult(t, recovered.Result)
}

type missingRequiredInputInvocationRun struct {
	outcome missingRequiredInputInvocationOutcome
}

type missingRequiredInputInvocationOutcome struct {
	response      factoryapi.InvocationResponse
	errorResponse factoryapi.ErrorResponse
	diagnostic    string
}

func scaffoldMissingRequiredInputMappingFactory(t *testing.T) string {
	t.Helper()

	dir := support.ScaffoldFactory(t, map[string]any{
		"name": "javascript-input-mapping-missing-required",
		"invocationSignature": map[string]any{
			"parameters": []any{map[string]any{
				"name": "label", "required": true,
				"bindings": []any{map[string]any{"kind": "POSITIONAL", "position": 1}},
			}},
		},
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
				"defaultPolicy": map[string]any{
					"maxAgents":     4,
					"concurrency":   2,
					"allowedModels": []any{"gpt-allowed"},
				},
			},
		},
	})
	if err := os.WriteFile(
		filepath.Join(dir, typedInputWorkflowFileName),
		[]byte(missingRequiredInputWorkflowSource),
		0o600,
	); err != nil {
		t.Fatalf("write missing required input workflow: %v", err)
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

func runMissingRequiredInputJavaScriptInvocation(
	t *testing.T,
	fixture *contractFixture,
	dir string,
) missingRequiredInputInvocationRun {
	t.Helper()

	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "run",
		"--factory", filepath.Join(dir, "factory.json"),
		"--no-record",
		"--with-mock-workers", filepath.Join(dir, "mock-workers.json"),
		"--output", "primary",
	})
	homeDir := t.TempDir()
	inputs.Input.Env = append(inputs.Input.Env, "HOME="+homeDir, "USERPROFILE="+homeDir)
	inputs.Input.WorkingDirectory = dir

	err := fixture.process.Execute(inputs.Input)
	if err == nil {
		t.Fatalf(
			"Process.Execute() error = nil, want missing required input failure before child dispatch\nstdout:\n%s\nstderr:\n%s",
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}
	outcome := missingRequiredInputInvocationOutcome{
		diagnostic: extractMissingRequiredInputDiagnostic(t, inputs.Stdout(), inputs.Stderr(), err.Error()),
	}
	if stdout := strings.TrimSpace(inputs.Stdout()); stdout != "" {
		if decodeErr := json.Unmarshal([]byte(stdout), &outcome.response); decodeErr != nil {
			t.Fatalf("decode InvocationResponse: %v\nstdout:\n%s", decodeErr, stdout)
		}
	}
	if stderr := strings.TrimSpace(inputs.Stderr()); stderr != "" {
		if decodeErr := json.Unmarshal([]byte(stderr), &outcome.errorResponse); decodeErr != nil {
			t.Fatalf("decode ErrorResponse: %v\nstderr:\n%s", decodeErr, stderr)
		}
	}
	if outcome.response.SessionId != nil && strings.TrimSpace(*outcome.response.SessionId) != "" {
		fixture.trackLocalSession(
			t,
			*outcome.response.SessionId,
			outcome.response.RequestId,
			dir,
		)
	}
	return missingRequiredInputInvocationRun{
		outcome: outcome,
	}
}

func assertMissingRequiredInputInvocationOutcome(
	t *testing.T,
	outcome missingRequiredInputInvocationOutcome,
) {
	t.Helper()

	if outcome.response.Status != "" &&
		outcome.response.Status != factoryapi.InvocationTerminalStatusFailed {
		t.Fatalf("invocation status = %q, want FAILED or empty before terminal invocation", outcome.response.Status)
	}
	if outcome.response.PrimaryResult != nil {
		t.Fatalf("primaryResult = %#v, want nil before missing-required-input validation", outcome.response.PrimaryResult)
	}
	if outcome.errorResponse.Code != factoryapi.ErrorResponseCode("INVOCATION_ARGUMENT_MISSING_REQUIRED_INPUT") {
		t.Fatalf(
			"ErrorResponse code = %q, want INVOCATION_ARGUMENT_MISSING_REQUIRED_INPUT",
			outcome.errorResponse.Code,
		)
	}
	if !strings.Contains(outcome.errorResponse.Message, stableMissingRequiredInputDiagnostic) {
		t.Fatalf(
			"ErrorResponse message = %q, want missing required input diagnostic %q",
			outcome.errorResponse.Message,
			stableMissingRequiredInputDiagnostic,
		)
	}
	if !strings.Contains(outcome.diagnostic, `"label"`) {
		t.Fatalf(
			"failure diagnostic = %q, want actionable missing required input field name %q",
			outcome.diagnostic,
			"label",
		)
	}
}

func extractMissingRequiredInputDiagnostic(t *testing.T, stdout, stderr, executeError string) string {
	t.Helper()

	diagnostic := strings.Join([]string{executeError, stdout, stderr}, "\n")
	if strings.Contains(diagnostic, stableMissingRequiredInputDiagnostic) {
		return diagnostic
	}
	t.Fatalf("diagnostic %q does not contain missing required input detail %q", diagnostic, stableMissingRequiredInputDiagnostic)
	return ""
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
	fixture *contractFixture,
	dir, requestID string,
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
	return fixture.startSync(t, factoryapi.FactorySessionExecutionRequest{
		RequestId: requestID,
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowFile,
			WorkflowFile: &workflowPath,
		},
		Args: &args,
	}, dir)
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
