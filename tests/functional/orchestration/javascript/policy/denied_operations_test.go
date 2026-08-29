package policy

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
	policyDeniedModelWorkflow = `agent.run({
  prompt: "summarize workflows",
  label: "denied-model",
  model: "gpt-denied",
});
return { ok: true };`

	stablePolicyDeniedModelDiagnostic = `policy denied: model "gpt-denied" is not listed in allowedModels`
)

// TestJavaScriptDeniedChildOperationReturnsStablePolicyDiagnostic preserves
// the original behavior selector while sharing the package-owned process.
func TestJavaScriptDeniedChildOperationReturnsStablePolicyDiagnostic(t *testing.T) {
	fixture := policyFixtureForTest(t)
	before := fixture.trackedSessionCount()
	runJavaScriptDeniedChildOperationReturnsStablePolicyDiagnostic(t, fixture)
	if got := fixture.trackedSessionCount() - before; got != 2 {
		t.Fatalf("stable-denial Factory Sessions = %d, want two fresh sessions", got)
	}
}

// TestJavaScriptPolicyFailureDoesNotDispatchExternalWork preserves the
// original behavior selector while sharing the package-owned process.
func TestJavaScriptPolicyFailureDoesNotDispatchExternalWork(t *testing.T) {
	fixture := policyFixtureForTest(t)
	before := fixture.trackedSessionCount()
	runJavaScriptPolicyFailureDoesNotDispatchExternalWork(t, fixture)
	if got := fixture.trackedSessionCount() - before; got != 1 {
		t.Fatalf("no-dispatch Factory Sessions = %d, want one fresh session", got)
	}
}

// runJavaScriptDeniedChildOperationReturnsStablePolicyDiagnostic proves a
// JavaScript Factory whose child request violates effective policy fails through
// the public invocation boundary with a stable customer-readable policy denial
// diagnostic after a root-built process run with external effects substituted
// only through edges.Edges.
func runJavaScriptDeniedChildOperationReturnsStablePolicyDiagnostic(
	t *testing.T,
	fixture *policyFixture,
) {
	first := runPolicyDeniedJavaScriptInvocation(t, fixture, scaffoldPolicyDeniedJavaScriptFactory(t))
	second := runPolicyDeniedJavaScriptInvocation(t, fixture, scaffoldPolicyDeniedJavaScriptFactory(t))

	assertPolicyDeniedInvocationOutcome(t, first.outcome)
	assertPolicyDeniedInvocationOutcome(t, second.outcome)
	if first.outcome.policyDiagnostic != second.outcome.policyDiagnostic {
		t.Fatalf(
			"policy diagnostic = %q then %q, want identical stable denial across repeated runs",
			first.outcome.policyDiagnostic,
			second.outcome.policyDiagnostic,
		)
	}
}

// runJavaScriptPolicyFailureDoesNotDispatchExternalWork proves a denied child
// operation fails through the public invocation boundary before any external
// worker or provider work is dispatched when external effects are substituted
// only through edges.Edges.
func runJavaScriptPolicyFailureDoesNotDispatchExternalWork(
	t *testing.T,
	fixture *policyFixture,
) {
	run := runPolicyDeniedJavaScriptInvocation(t, fixture, scaffoldPolicyDeniedJavaScriptFactory(t))

	assertPolicyDeniedInvocationOutcome(t, run.outcome)
	if fixture.providerRunner.CallCount() != 0 {
		t.Fatalf(
			"provider command runner call count = %d, want 0 before policy-denied child dispatch",
			fixture.providerRunner.CallCount(),
		)
	}
}

type policyDeniedInvocationRun struct {
	outcome policyDeniedInvocationOutcome
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
					"maxAgents":               4,
					"concurrency":             2,
					"allowedModels":           []any{"gpt-allowed"},
					"allowedReasoningEfforts": []any{"low"},
				},
			},
		},
	})
	if err := os.WriteFile(filepath.Join(dir, "workflow.js"), []byte(policyDeniedModelWorkflow), 0o600); err != nil {
		t.Fatalf("write JavaScript workflow: %v", err)
	}
	return dir
}

func runPolicyDeniedJavaScriptInvocation(
	t *testing.T,
	fixture *policyFixture,
	dir string,
) policyDeniedInvocationRun {
	t.Helper()

	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "run",
		"--factory", filepath.Join(dir, "factory.json"),
		"--no-record",
		"hello",
	})
	inputs.Input.WorkingDirectory = dir
	home := t.TempDir()
	inputs.Input.Env = policyCustomerEnvironment(home)

	err := fixture.process.Execute(inputs.Input)
	if err == nil {
		t.Fatalf("Process.Execute() error = nil; stdout:\n%s\nstderr:\n%s", inputs.Stdout(), inputs.Stderr())
	}

	runOutcome := policyDeniedInvocationOutcome{
		policyDiagnostic: extractPolicyDiagnostic(t, inputs.Stdout(), inputs.Stderr(), err.Error()),
	}
	if stdout := strings.TrimSpace(inputs.Stdout()); stdout != "" {
		runOutcome.response = support.DecodeInvocationResponseJSON(t, stdout)
	}
	if stderr := strings.TrimSpace(inputs.Stderr()); stderr != "" {
		if decodeErr := json.Unmarshal([]byte(stderr), &runOutcome.errorResponse); decodeErr != nil {
			t.Fatalf("decode ErrorResponse: %v\nstderr:\n%s", decodeErr, stderr)
		}
	}
	fixture.trackInvocationSession(t, runOutcome.response, dir, home)
	return policyDeniedInvocationRun{
		outcome: runOutcome,
	}
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
