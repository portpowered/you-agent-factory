package factory_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryruntimeservice "github.com/portpowered/infinite-you/pkg/services/factory_runtime/service"
	factoryruntimetestkit "github.com/portpowered/infinite-you/pkg/services/factory_runtime/testkit"
)

const validWorkflowSource = `
meta({ name: "review", version: 1 });
phase("setup");
log("starting");
workflow.log("step");
workflow.artifact({ kind: "log", label: "step" });
const result = await agent.run({ prompt: "review" });
workflow.final({ ok: true, result });
pipeline([], function () {}, function () {});
`

var validationWorkflows = factoryruntimetestkit.JavaScriptWorkflows()

func TestValidate_AcceptsSupportedWorkflowGlobals(t *testing.T) {
	t.Parallel()
	result := validationWorkflows.Validate(factory.WorkflowValidationRequest{
		Source:    validWorkflowSource,
		SourceRef: "inline",
	})
	if result.HasIssues() {
		t.Fatalf("validation issues = %#v, want none for supported workflow source", result.Issues)
	}
}

func TestValidate_RejectsSyntaxErrorWithSourceLocation(t *testing.T) {
	t.Parallel()
	result := validationWorkflows.Validate(factory.WorkflowValidationRequest{
		Source:    "phase(\"setup\";\n",
		SourceRef: "inline",
	})
	if !result.HasIssues() {
		t.Fatal("expected syntax validation issue")
	}
	issue := result.Issues[0]
	if issue.Code != factory.WorkflowValidationCodeSyntaxError {
		t.Fatalf("issue code = %q, want %q", issue.Code, factory.WorkflowValidationCodeSyntaxError)
	}
	if issue.Line <= 0 {
		t.Fatalf("issue line = %d, want source location", issue.Line)
	}
}

func TestValidate_RejectsInvalidArgsSchema(t *testing.T) {
	t.Parallel()
	result := validationWorkflows.Validate(factory.WorkflowValidationRequest{
		ConfigPath: "orchestrator.javascript",
		ArgsSchema: []byte(`{"type":"array"}`),
	})
	if !result.HasIssues() {
		t.Fatal("expected args schema validation issue")
	}
	if result.Issues[0].Code != factory.WorkflowValidationCodeInvalidArgsSchema {
		t.Fatalf("issue code = %q, want %q", result.Issues[0].Code, factory.WorkflowValidationCodeInvalidArgsSchema)
	}
}

func TestValidate_RejectsForbiddenHostAccess(t *testing.T) {
	t.Parallel()
	result := validationWorkflows.Validate(factory.WorkflowValidationRequest{
		Source:    `const data = require("fs");`,
		SourceRef: "inline",
	})
	if !result.HasIssues() {
		t.Fatal("expected forbidden host-access validation issue")
	}
	if result.Issues[0].Code != factory.WorkflowValidationCodeForbiddenHostAccess {
		t.Fatalf("issue code = %q, want %q", result.Issues[0].Code, factory.WorkflowValidationCodeForbiddenHostAccess)
	}
}

func TestValidate_AllowsLocalBindingMemberAccessAndResumeState(t *testing.T) {
	t.Parallel()
	result := validationWorkflows.Validate(factory.WorkflowValidationRequest{
		Source: `(function(){
const resumed = workflow.resumeState();
return resumed && resumed.step >= 1;
})();`,
		SourceRef: "inline",
	})
	if result.HasIssues() {
		t.Fatalf("validation issues = %#v, want none for local binding member access", result.Issues)
	}
}

func TestValidate_RejectsShadowedLocalBindingDoesNotBypassHostMemberAccess(t *testing.T) {
	t.Parallel()
	result := validationWorkflows.Validate(factory.WorkflowValidationRequest{
		Source: `(function(){
  function shadow(foo) {
    return foo && foo.bar;
  }
  return foo.bar;
})();`,
		SourceRef: "inline",
	})
	if !result.HasIssues() {
		t.Fatal("expected forbidden host-access validation issue for outer foo.bar")
	}
	found := false
	for _, issue := range result.Issues {
		if issue.Code == factory.WorkflowValidationCodeForbiddenHostAccess && strings.Contains(issue.Message, "foo.bar") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("issues = %#v, want forbidden host access for outer foo.bar", result.Issues)
	}
}

func TestValidate_RejectsDynamicImportHostAccess(t *testing.T) {
	t.Parallel()
	result := validationWorkflows.Validate(factory.WorkflowValidationRequest{
		Source:    `import("fs");`,
		SourceRef: "inline",
	})
	if !result.HasIssues() {
		t.Fatal("expected forbidden host-access validation issue for dynamic import")
	}
	if result.Issues[0].Code != factory.WorkflowValidationCodeForbiddenHostAccess {
		t.Fatalf("issue code = %q, want %q", result.Issues[0].Code, factory.WorkflowValidationCodeForbiddenHostAccess)
	}
}

func TestWorkflowSourceTargets_ValidatesFileBackedSource(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "review.js")
	if err := os.WriteFile(sourcePath, []byte(validWorkflowSource), 0o600); err != nil {
		t.Fatalf("write workflow source: %v", err)
	}

	reader := factoryruntimetestkit.NewFileWorkflowSourceReader(dir, localWorkflowSourceFiles{})
	targets := factoryruntimeservice.NewOrchestratorDefinitionValidator(testJavaScriptWorkflows()).ValidateJavaScriptFactoryDefinition(
		t.Context(),
		&interfaces.FactoryOrchestratorJavaScriptConfig{SourceRef: "review.js"},
		reader,
	)
	if len(targets) > 0 {
		t.Fatalf("workflow source targets = %#v, want none for valid file-backed source", targets)
	}
}

func TestWorkflowSourceTargets_RejectsFileBackedSyntaxError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "broken.js")
	if err := os.WriteFile(sourcePath, []byte("phase(\"setup\";"), 0o600); err != nil {
		t.Fatalf("write workflow source: %v", err)
	}

	reader := factoryruntimetestkit.NewFileWorkflowSourceReader(dir, localWorkflowSourceFiles{})
	targets := factoryruntimeservice.NewOrchestratorDefinitionValidator(testJavaScriptWorkflows()).ValidateJavaScriptFactoryDefinition(
		t.Context(),
		&interfaces.FactoryOrchestratorJavaScriptConfig{SourceRef: "broken.js"},
		reader,
	)
	if len(targets) == 0 {
		t.Fatal("expected file-backed syntax validation target")
	}
	if targets[0].Code != factory.WorkflowValidationCodeSyntaxError {
		t.Fatalf("target code = %q, want %q", targets[0].Code, factory.WorkflowValidationCodeSyntaxError)
	}
}

func TestValidate_RejectsInvalidPrimitiveCallShapes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		src  string
		code string
	}{
		{
			name: "invalid meta",
			src:  `meta("bad");`,
			code: factory.WorkflowValidationCodeInvalidMetadata,
		},
		{
			name: "invalid workflow artifact",
			src:  `workflow.artifact("log");`,
			code: factory.WorkflowValidationCodeUnsupportedPrimitive,
		},
		{
			name: "invalid agent run",
			src:  `agent.run("review");`,
			code: factory.WorkflowValidationCodeUnsupportedPrimitive,
		},
		{
			name: "invalid parallel",
			src:  `parallel("not-array");`,
			code: factory.WorkflowValidationCodeUnsupportedPrimitive,
		},
		{
			name: "invalid pipeline",
			src:  `pipeline("not-array");`,
			code: factory.WorkflowValidationCodeUnsupportedPrimitive,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := validationWorkflows.Validate(factory.WorkflowValidationRequest{
				Source:    tc.src,
				SourceRef: "inline",
			})
			if !result.HasIssues() {
				t.Fatal("expected primitive shape validation issue")
			}
			found := false
			for _, issue := range result.Issues {
				if issue.Code == tc.code {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("issues = %#v, want code %q", result.Issues, tc.code)
			}
		})
	}
}

func TestValidate_AcceptsEverySupportedAgentRunField(t *testing.T) {
	t.Parallel()
	result := validationWorkflows.Validate(factory.WorkflowValidationRequest{
		Source: `agent.run({
  prompt: "review",
  label: "reviewer",
  preset: "careful",
  modelProvider: "codex",
  model: "gpt-test",
  reasoningEffort: "high",
});`,
		SourceRef: "inline",
	})
	if result.HasIssues() {
		t.Fatalf("validation issues = %#v, want canonical agent.run fields accepted", result.Issues)
	}
}

func TestValidate_RejectsInvalidSupportedAgentRunFieldValues(t *testing.T) {
	t.Parallel()
	for _, field := range []string{"prompt", "label", "preset", "modelProvider", "model", "reasoningEffort"} {
		t.Run(field, func(t *testing.T) {
			source := fmt.Sprintf(`agent.run({ prompt: "review", %s: 42 });`, field)
			if field == "prompt" {
				source = `agent.run({ prompt: 42 });`
			}
			result := validationWorkflows.Validate(factory.WorkflowValidationRequest{
				Source:    source,
				SourceRef: "inline",
			})
			if !result.HasIssues() || !strings.Contains(result.Issues[0].Message, field) {
				t.Fatalf("validation issues = %#v, want field-specific shape issue", result.Issues)
			}
		})
	}
}

func TestValidate_AcceptsComputedAgentRunPromptForLeadSynthesis(t *testing.T) {
	t.Parallel()
	result := validationWorkflows.Validate(factory.WorkflowValidationRequest{
		Source:    `const findings = [{ output: { text: "specialist finding" } }]; agent.run({ prompt: "Synthesize findings: " + findings[0].output.text, label: "lead" });`,
		SourceRef: "inline",
	})
	if result.HasIssues() {
		t.Fatalf("validation issues = %#v, want computed lead-synthesis prompt accepted", result.Issues)
	}
}

func TestValidate_RejectsUnsupportedAgentRunFieldsWithoutExposingValues(t *testing.T) {
	t.Parallel()
	unsupported := []string{
		"writableRoots", "allowNetwork", "network", "allowDangerFullAccess", "dangerFullAccess",
		"schema", "outputSchema", "concurrency", "maxAgents", "duration", "timeout", "timeoutMs",
	}
	for _, field := range unsupported {
		t.Run(field, func(t *testing.T) {
			source := fmt.Sprintf(`agent.run({ prompt: "prompt-secret", %s: "value-secret" });`, field)
			result := validationWorkflows.Validate(factory.WorkflowValidationRequest{Source: source, SourceRef: "inline"})
			want := `agent.run() does not support field "` + field + `"`
			if len(result.Issues) != 1 || result.Issues[0].Code != factory.WorkflowValidationCodeUnsupportedPrimitive || result.Issues[0].Message != want {
				t.Fatalf("validation issues = %#v, want one field-specific unsupported-primitive issue", result.Issues)
			}
			if strings.Contains(result.Issues[0].Message, "value-secret") || strings.Contains(result.Issues[0].Message, "prompt-secret") {
				t.Fatalf("validation message = %q, want redacted diagnostic", result.Issues[0].Message)
			}
		})
	}
}

func TestValidate_RejectsQuotedUnsupportedAgentRunField(t *testing.T) {
	t.Parallel()
	result := validationWorkflows.Validate(factory.WorkflowValidationRequest{
		Source:    `agent.run({ prompt: "review", "outputSchema": "secret" });`,
		SourceRef: "inline",
	})
	if len(result.Issues) != 1 || result.Issues[0].Message != `agent.run() does not support field "outputSchema"` {
		t.Fatalf("validation issues = %#v, want quoted field rejection", result.Issues)
	}
}

func TestValidateFactory_RejectsInlineForbiddenHostAccess(t *testing.T) {
	t.Parallel()
	result := validationWorkflows.Validate(factory.WorkflowValidationRequest{
		Source:    `fetch("https://example.com");`,
		SourceRef: "inline",
	})
	if len(result.Issues) == 0 {
		t.Fatal("expected inline forbidden host-access validation target")
	}
	if result.Issues[0].Code != factory.WorkflowValidationCodeForbiddenHostAccess {
		t.Fatalf("target code = %q, want %q", result.Issues[0].Code, factory.WorkflowValidationCodeForbiddenHostAccess)
	}
}
