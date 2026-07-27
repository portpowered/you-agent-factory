package policy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	policyDeniedModelWorkflow = `agent.run({
  prompt: "summarize workflows",
  label: "denied-model",
  model: "gpt-denied",
});
return { ok: true };`

	stablePolicyDeniedModelDiagnostic = `policy denied: model "gpt-denied" is not listed in allowedModels`
)

// TestJavaScriptDeniedChildOperationReturnsStablePolicyDiagnostic proves a
// JavaScript Factory whose child request violates effective policy fails through
// the public invocation boundary with a stable customer-readable policy denial
// diagnostic after a root-built process run with external effects substituted
// only through edges.Edges.
func TestJavaScriptDeniedChildOperationReturnsStablePolicyDiagnostic(t *testing.T) {
	t.Parallel()

	dir := scaffoldPolicyDeniedJavaScriptFactory(t)
	first := runPolicyDeniedJavaScriptInvocation(t, dir)
	second := runPolicyDeniedJavaScriptInvocation(t, dir)

	assertPolicyDeniedInvocationOutcome(t, first)
	assertPolicyDeniedInvocationOutcome(t, second)
	if first.policyDiagnostic != second.policyDiagnostic {
		t.Fatalf(
			"policy diagnostic = %q then %q, want identical stable denial across repeated runs",
			first.policyDiagnostic,
			second.policyDiagnostic,
		)
	}
}

type policyDeniedInvocationOutcome struct {
	response         factoryapi.InvocationResponse
	errorResponse    factoryapi.ErrorResponse
	policyDiagnostic string
}

func scaffoldPolicyDeniedJavaScriptFactory(t *testing.T) string {
	t.Helper()

	dir := support.ScaffoldFactory(t, map[string]any{
		"name": "javascript-policy-denied",
		"invocationSignature": map[string]any{
			"parameters": []any{map[string]any{
				"name": "prompt", "required": true,
				"bindings": []any{map[string]any{"kind": "POSITIONAL", "position": 1}},
			}},
		},
		"orchestrator": map[string]any{
			"kind": "JAVASCRIPT",
			"javascript": map[string]any{
				"sourceRef": "workflow.js",
				"argsSchema": map[string]any{
					"type": "object", "required": []any{"prompt"},
					"properties":           map[string]any{"prompt": map[string]any{"type": "string"}},
					"additionalProperties": false,
				},
				"defaultPolicy": map[string]any{
					"mode":                    "READ_ONLY",
					"maxAgents":               4,
					"concurrency":             2,
					"allowNetwork":            false,
					"allowedModels":           []any{"gpt-allowed"},
					"allowedReasoningEfforts": []any{"low"},
				},
			},
		},
	})
	if err := os.WriteFile(filepath.Join(dir, "workflow.js"), []byte(policyDeniedModelWorkflow), 0o600); err != nil {
		t.Fatalf("write JavaScript workflow: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mock-workers.json"), []byte(`{"mockWorkers":[]}`), 0o600); err != nil {
		t.Fatalf("write mock-workers config: %v", err)
	}
	return dir
}

func runPolicyDeniedJavaScriptInvocation(t *testing.T, dir string) policyDeniedInvocationOutcome {
	t.Helper()

	runner := support.NewRecordingCommandRunner("unexpected live provider execution")
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "run",
		"--factory", filepath.Join(dir, "factory.json"),
		"--no-record",
		"--with-mock-workers", filepath.Join(dir, "mock-workers.json"),
		"hello",
	})
	inputs.Input.WorkingDirectory = dir
	home := t.TempDir()
	inputs.Input.Env = append(inputs.Input.Env, "HOME="+home, "USERPROFILE="+home)

	err := support.BuildProcess(t, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}).Execute(inputs.Input)
	if err == nil {
		t.Fatalf("Process.Execute() error = nil; stdout:\n%s\nstderr:\n%s", inputs.Stdout(), inputs.Stderr())
	}

	outcome := policyDeniedInvocationOutcome{
		policyDiagnostic: extractPolicyDiagnostic(t, inputs.Stdout(), inputs.Stderr(), err.Error()),
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
	return outcome
}

func assertPolicyDeniedInvocationOutcome(t *testing.T, outcome policyDeniedInvocationOutcome) {
	t.Helper()

	if outcome.response.Status != factoryapi.InvocationTerminalStatusFailed {
		t.Fatalf("invocation status = %q, want FAILED", outcome.response.Status)
	}
	if outcome.response.ErrorCode == nil ||
		*outcome.response.ErrorCode != factoryapi.InvocationResponseErrorCode("INVOCATION_RUNTIME_FAILURE") {
		t.Fatalf("invocation errorCode = %#v, want INVOCATION_RUNTIME_FAILURE", outcome.response.ErrorCode)
	}
	if outcome.response.PrimaryResult != nil {
		t.Fatalf("primaryResult = %#v, want nil on policy denial", outcome.response.PrimaryResult)
	}
	if !strings.Contains(outcome.policyDiagnostic, stablePolicyDeniedModelDiagnostic) {
		t.Fatalf("policy diagnostic = %q, want substring %q", outcome.policyDiagnostic, stablePolicyDeniedModelDiagnostic)
	}
	if !strings.Contains(outcome.policyDiagnostic, `label="denied-model"`) {
		t.Fatalf("policy diagnostic = %q, want actionable child label context", outcome.policyDiagnostic)
	}
	if outcome.errorResponse.Code != factoryapi.ErrorResponseCode("INVOCATION_RUNTIME_FAILURE") {
		t.Fatalf("ErrorResponse code = %q, want INVOCATION_RUNTIME_FAILURE", outcome.errorResponse.Code)
	}
	if !strings.Contains(outcome.errorResponse.Message, stablePolicyDeniedModelDiagnostic) {
		t.Fatalf("ErrorResponse message = %q, want policy denial diagnostic", outcome.errorResponse.Message)
	}
}

func extractPolicyDiagnostic(t *testing.T, stdout, stderr, executeError string) string {
	t.Helper()

	diagnostic := strings.Join([]string{executeError, stdout, stderr}, "\n")
	for _, fragment := range []string{
		`policy denied: model "gpt-denied" is not listed in allowedModels (label="denied-model")`,
		stablePolicyDeniedModelDiagnostic,
	} {
		if strings.Contains(diagnostic, fragment) {
			return fragment
		}
	}
	t.Fatalf("diagnostic %q does not contain stable policy denial", diagnostic)
	return ""
}
