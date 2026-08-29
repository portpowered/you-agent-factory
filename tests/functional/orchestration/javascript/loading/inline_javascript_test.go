// Package loading holds customer functional scenarios for inline JavaScript
// Factory loading through the public CLI and customer process boundary.
package loading_test

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	orderedJavaScriptPipelineResult   = "ordered-pipeline-complete"
	orderedJavaScriptPipelineWorkflow = `return (async function () {
  const first = await agent.run({
    prompt: "stage-one-input",
    label: "stage-one",
  });
  const stageTwoPrompt =
    "stage-two-after:" + first.dispatchId + ":" + first.status + ":" + first.output.text;
  const second = await agent.run({
    prompt: stageTwoPrompt,
    label: "stage-two",
  });

  if (first.status !== "COMPLETED" || second.status !== "COMPLETED") {
    throw new Error("ordered pipeline did not complete both stages");
  }
  return {
    finalValue: "` + orderedJavaScriptPipelineResult + `",
    stages: [
      {
        stage: first.label,
        childIndex: first.childIndex,
        dispatchId: first.dispatchId,
        resultStatus: first.status,
        response: first.output.text,
      },
      {
        stage: second.label,
        childIndex: second.childIndex,
        dispatchId: second.dispatchId,
        resultStatus: second.status,
        response: second.output.text,
      },
    ],
    dependency: {
      priorDispatchId: first.dispatchId,
      observedByStageTwo: second.output.text.indexOf(stageTwoPrompt) !== -1,
    },
  };
})();`
)

const (
	inlineJavaScriptSuccessResult     = "<SUCCESS>"
	inlineJavaScriptSyntaxErrorSource = "workflow.final(\"ok\");\nphase(\"setup\";\n"
	inlineJavaScriptSyntaxErrorLine   = 2
	workflowSourceSyntaxErrorCode     = "workflow.source.syntaxError"
)

var privateJavaScriptVMDiagnosticMarkers = []string{
	"goja",
	"goja.",
	"stack frame",
	"heap dump",
}

// TestInlineJavaScriptFactoryRunsFromCLI proves an inline JavaScript Factory
// definition completes through the public you run customer process boundary
// with mock workers, a terminal COMPLETED primary outcome that returns the
// authored primary result, and without private VM internals in success
// diagnostics.
func runInlineJavaScriptFactoryRunsFromCLI(t *testing.T, fixture *loadingFixture) {
	dir := support.ScaffoldFactory(t, map[string]any{
		"name": "inline-javascript-loading",
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
					"inline":   `workflow.final("` + inlineJavaScriptSuccessResult + `");`,
				},
				"argsSchema": map[string]any{
					"type":                 "object",
					"properties":           map[string]any{"prompt": map[string]any{"type": "string"}},
					"additionalProperties": false,
				},
			},
		},
	})
	mockWorkersPath := writeEmptyMockWorkersConfig(t, dir)
	result, inputs := fixture.runCLIInvocation(t, []string{
		"you", "--json", "run",
		"--factory", filepath.Join(dir, "factory.json"),
		"--with-mock-workers", mockWorkersPath,
		"--output", "primary",
		"--no-record",
		"hello",
	}, dir, t.TempDir())
	if got := fixture.provider.CallCount(); got != 0 {
		t.Fatalf("provider command runner call count = %d, want 0 for inline factory without child dispatch", got)
	}
	assertInlineJavaScriptSuccessOutcome(t, result)
	assertNoPrivateJavaScriptVMDiagnostics(t, inputs.Stdout(), inputs.Stderr())
}

// TestInlineJavaScriptFactoryRunsOrderedTwoStagePipeline proves an inline
// JavaScript Factory with sequential agent.run child dispatches completes
// through the public you run customer process boundary with ordered stage
// evidence, stage-two dependency on stage-one output, and a terminal COMPLETED
// primary outcome without live provider execution.
func runInlineJavaScriptFactoryRunsOrderedTwoStagePipeline(t *testing.T, fixture *loadingFixture) {
	dir := scaffoldOrderedInlineJavaScriptPipelineFactory(t)
	mockWorkersPath := writeEmptyMockWorkersConfig(t, dir)
	result, inputs := fixture.runCLIInvocation(t, []string{
		"you", "--json", "run",
		"--factory", filepath.Join(dir, "factory.json"),
		"--with-mock-workers", mockWorkersPath,
		"--output", "primary",
		"--no-record",
		"hello",
	}, dir, t.TempDir())
	if got := fixture.provider.CallCount(); got != 0 {
		t.Fatalf("provider command runner call count = %d, want 0 for mock-worker child execution", got)
	}
	assertOrderedInlineJavaScriptPipelineOutcome(t, result)
	assertNoPrivateJavaScriptVMDiagnostics(t, inputs.Stdout(), inputs.Stderr())
}

// TestInlineJavaScriptSyntaxErrorReturnsSourceLocation proves an inline
// JavaScript Factory with a deliberate syntax error fails before work starts
// through the public you run customer process boundary with an actionable
// authored source location and without private VM internals or external
// worker dispatch.
func runInlineJavaScriptSyntaxErrorReturnsSourceLocation(t *testing.T, fixture *loadingFixture) {
	dir := support.ScaffoldFactory(t, map[string]any{
		"name": "inline-javascript-syntax-error",
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
					"inline":   inlineJavaScriptSyntaxErrorSource,
				},
				"argsSchema": map[string]any{
					"type":                 "object",
					"properties":           map[string]any{"prompt": map[string]any{"type": "string"}},
					"additionalProperties": false,
				},
			},
		},
	})
	mockWorkersPath := writeEmptyMockWorkersConfig(t, dir)
	inputs, err := fixture.executeCLI(t, []string{
		"you", "--json", "run",
		"--factory", filepath.Join(dir, "factory.json"),
		"--with-mock-workers", mockWorkersPath,
		"--output", "primary",
		"--no-record",
		"hello",
	}, dir, t.TempDir())
	assertInlineJavaScriptSyntaxFailureOutcome(
		t,
		err,
		inputs.Stdout(),
		inputs.Stderr(),
		inlineJavaScriptSyntaxErrorLine,
	)
	if got := fixture.provider.CallCount(); got != 0 {
		t.Fatalf("provider command runner call count = %d, want 0 for syntax error before dispatch", got)
	}
	assertNoPrivateJavaScriptVMDiagnostics(t, inputs.Stdout(), inputs.Stderr())
	fixture.recoverAfterLoadFailure(t, "inline-syntax")
}

func scaffoldOrderedInlineJavaScriptPipelineFactory(t *testing.T) string {
	t.Helper()

	dir := support.ScaffoldFactory(t, map[string]any{
		"name": "inline-javascript-ordered-pipeline",
		"invocationSignature": map[string]any{
			"parameters": []any{map[string]any{
				"name": "prompt", "required": false,
				"bindings": []any{map[string]any{"kind": "POSITIONAL", "position": 1}},
			}},
		},
		"orchestrator": map[string]any{
			"kind": "JAVASCRIPT",
			"javascript": map[string]any{
				"sourceRef": "workflow.js",
				"argsSchema": map[string]any{
					"type":                 "object",
					"properties":           map[string]any{"prompt": map[string]any{"type": "string"}},
					"additionalProperties": false,
				},
			},
		},
	})
	if err := os.WriteFile(filepath.Join(dir, "workflow.js"), []byte(orderedJavaScriptPipelineWorkflow), 0o600); err != nil {
		t.Fatalf("write ordered pipeline workflow: %v", err)
	}
	return dir
}

func writeEmptyMockWorkersConfig(t *testing.T, dir string) string {
	t.Helper()

	path := filepath.Join(dir, "mock-workers.json")
	if err := os.WriteFile(path, []byte(`{"mockWorkers":[]}`), 0o600); err != nil {
		t.Fatalf("write mock-workers config: %v", err)
	}
	return path
}

func decodeSingleInvocationResponse(t *testing.T, stdout string) factoryapi.InvocationResponse {
	t.Helper()

	decoder := json.NewDecoder(strings.NewReader(stdout))
	var result factoryapi.InvocationResponse
	if err := decoder.Decode(&result); err != nil {
		t.Fatalf("decode CLI stdout as InvocationResponse: %v\nstdout:\n%s", err, stdout)
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		t.Fatalf("stdout contained more than one terminal JSON result: %v\nstdout:\n%s", err, stdout)
	}
	return result
}

type orderedInlineJavaScriptPipelineEvidence struct {
	FinalValue string `json:"finalValue"`
	Stages     []struct {
		Stage        string `json:"stage"`
		ChildIndex   int    `json:"childIndex"`
		DispatchID   string `json:"dispatchId"`
		ResultStatus string `json:"resultStatus"`
		Response     string `json:"response"`
	} `json:"stages"`
	Dependency struct {
		PriorDispatchID    string `json:"priorDispatchId"`
		ObservedByStageTwo bool   `json:"observedByStageTwo"`
	} `json:"dependency"`
}

func assertOrderedInlineJavaScriptPipelineOutcome(t *testing.T, result factoryapi.InvocationResponse) {
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

	var evidence orderedInlineJavaScriptPipelineEvidence
	encoded, err := json.Marshal(part.Json)
	if err != nil {
		t.Fatalf("encode ordered pipeline evidence: %v", err)
	}
	if err := json.Unmarshal(encoded, &evidence); err != nil {
		t.Fatalf("decode ordered pipeline evidence: %v", err)
	}
	if evidence.FinalValue != orderedJavaScriptPipelineResult {
		t.Fatalf("final customer value = %q, want %q", evidence.FinalValue, orderedJavaScriptPipelineResult)
	}
	if len(evidence.Stages) != 2 {
		t.Fatalf("stage evidence count = %d, want exactly 2", len(evidence.Stages))
	}
	for index, wantStage := range []string{"stage-one", "stage-two"} {
		stage := evidence.Stages[index]
		if stage.Stage != wantStage || stage.ChildIndex != index+1 || stage.DispatchID == "" || stage.ResultStatus != "COMPLETED" {
			t.Fatalf("stage evidence[%d] = %#v, want %s child %d with one completed dispatch result", index, stage, wantStage, index+1)
		}
		if !strings.Contains(stage.Response, ":"+wantStage+":") {
			t.Fatalf("stage evidence[%d] response = %q, want deterministic mock response for %s", index, stage.Response, wantStage)
		}
	}
	if evidence.Stages[0].DispatchID == evidence.Stages[1].DispatchID {
		t.Fatalf("stage dispatch IDs are duplicated: %q", evidence.Stages[0].DispatchID)
	}
	if evidence.Dependency.PriorDispatchID != evidence.Stages[0].DispatchID || !evidence.Dependency.ObservedByStageTwo {
		t.Fatalf("stage-two dependency evidence = %#v, want completed stage-one dispatch %q", evidence.Dependency, evidence.Stages[0].DispatchID)
	}
}

func assertInlineJavaScriptSuccessOutcome(t *testing.T, result factoryapi.InvocationResponse) {
	t.Helper()

	if result.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("terminal outcome = %s, want COMPLETED", result.Status)
	}
	if result.PrimaryResult == nil || len(*result.PrimaryResult) != 1 {
		t.Fatalf("primary result = %#v, want exactly one content part", result.PrimaryResult)
	}
	part, err := (*result.PrimaryResult)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("decode primary result content part: %v", err)
	}
	if part.Text != inlineJavaScriptSuccessResult {
		t.Fatalf("primary result = %q, want exact string %q", part.Text, inlineJavaScriptSuccessResult)
	}
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

func assertInlineJavaScriptSyntaxFailureOutcome(
	t *testing.T,
	runErr error,
	stdout string,
	stderr string,
	wantLine int,
) {
	t.Helper()

	if runErr == nil {
		t.Fatalf("Process.Execute() error = nil, want inline syntax failure before invocation\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty stdout before terminal invocation on syntax failure", stdout)
	}

	failureText := strings.Join([]string{runErr.Error(), stderr}, "\n")
	if !strings.Contains(failureText, workflowSourceSyntaxErrorCode) {
		t.Fatalf("failure output = %q, want customer-stable code %q", failureText, workflowSourceSyntaxErrorCode)
	}
	if !strings.Contains(failureText, fmt.Sprintf("line %d", wantLine)) {
		t.Fatalf("failure output = %q, want authored source line %d indicator", failureText, wantLine)
	}
	if !strings.Contains(stderr, "blocking validation targets") {
		t.Fatalf("stderr = %q, want customer-visible invocation failure surface", stderr)
	}
}
