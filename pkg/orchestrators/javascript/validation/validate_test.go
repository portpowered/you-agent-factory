package workflowvalidation_test

import (
	"os"
	"path/filepath"
	"testing"

	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/validation"
)

const validWorkflowSource = `
meta({ name: "review", version: 1 });
phase("setup");
log("starting");
workflow.log("step");
workflow.artifact({ kind: "log", label: "step" });
const result = await agent.run({ prompt: "review" });
workflow.final({ ok: true, result });
parallel([pipeline([result])]);
`

func TestValidate_AcceptsSupportedWorkflowGlobals(t *testing.T) {
	result := workflowvalidation.Validate(workflowvalidation.Request{
		Source:    validWorkflowSource,
		SourceRef: "inline",
	})
	if result.HasIssues() {
		t.Fatalf("validation issues = %#v, want none for supported workflow source", result.Issues)
	}
}

func TestValidate_RejectsSyntaxErrorWithSourceLocation(t *testing.T) {
	result := workflowvalidation.Validate(workflowvalidation.Request{
		Source:    "phase(\"setup\";\n",
		SourceRef: "inline",
	})
	if !result.HasIssues() {
		t.Fatal("expected syntax validation issue")
	}
	issue := result.Issues[0]
	if issue.Code != workflowvalidation.CodeSyntaxError {
		t.Fatalf("issue code = %q, want %q", issue.Code, workflowvalidation.CodeSyntaxError)
	}
	if issue.Line <= 0 {
		t.Fatalf("issue line = %d, want source location", issue.Line)
	}
}

func TestValidate_RejectsInvalidArgsSchema(t *testing.T) {
	result := workflowvalidation.Validate(workflowvalidation.Request{
		ConfigPath: "orchestrator.javascript",
		ArgsSchema: []byte(`{"type":"array"}`),
	})
	if !result.HasIssues() {
		t.Fatal("expected args schema validation issue")
	}
	if result.Issues[0].Code != workflowvalidation.CodeInvalidArgsSchema {
		t.Fatalf("issue code = %q, want %q", result.Issues[0].Code, workflowvalidation.CodeInvalidArgsSchema)
	}
}

func TestValidate_RejectsForbiddenHostAccess(t *testing.T) {
	result := workflowvalidation.Validate(workflowvalidation.Request{
		Source:    `const data = require("fs");`,
		SourceRef: "inline",
	})
	if !result.HasIssues() {
		t.Fatal("expected forbidden host-access validation issue")
	}
	if result.Issues[0].Code != workflowvalidation.CodeForbiddenHostAccess {
		t.Fatalf("issue code = %q, want %q", result.Issues[0].Code, workflowvalidation.CodeForbiddenHostAccess)
	}
}

func TestValidate_RejectsDynamicImportHostAccess(t *testing.T) {
	result := workflowvalidation.Validate(workflowvalidation.Request{
		Source:    `import("fs");`,
		SourceRef: "inline",
	})
	if !result.HasIssues() {
		t.Fatal("expected forbidden host-access validation issue for dynamic import")
	}
	if result.Issues[0].Code != workflowvalidation.CodeForbiddenHostAccess {
		t.Fatalf("issue code = %q, want %q", result.Issues[0].Code, workflowvalidation.CodeForbiddenHostAccess)
	}
}

func TestWorkflowSourceTargets_ValidatesFileBackedSource(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "review.js")
	if err := os.WriteFile(sourcePath, []byte(validWorkflowSource), 0o600); err != nil {
		t.Fatalf("write workflow source: %v", err)
	}

	reader := workflowvalidation.FileSourceReader(dir)
	targets := factoryvalidation.WorkflowSourceTargets(&interfaces.FactoryOrchestratorJavaScriptConfig{
		SourceRef: "review.js",
	}, reader)
	if len(targets) > 0 {
		t.Fatalf("workflow source targets = %#v, want none for valid file-backed source", targets)
	}
}

func TestWorkflowSourceTargets_RejectsFileBackedSyntaxError(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "broken.js")
	if err := os.WriteFile(sourcePath, []byte("phase(\"setup\";"), 0o600); err != nil {
		t.Fatalf("write workflow source: %v", err)
	}

	reader := workflowvalidation.FileSourceReader(dir)
	targets := factoryvalidation.WorkflowSourceTargets(&interfaces.FactoryOrchestratorJavaScriptConfig{
		SourceRef: "broken.js",
	}, reader)
	if len(targets) == 0 {
		t.Fatal("expected file-backed syntax validation target")
	}
	if targets[0].Code != workflowvalidation.CodeSyntaxError {
		t.Fatalf("target code = %q, want %q", targets[0].Code, workflowvalidation.CodeSyntaxError)
	}
}

func TestValidate_RejectsInvalidPrimitiveCallShapes(t *testing.T) {
	cases := []struct {
		name string
		src  string
		code string
	}{
		{
			name: "invalid meta",
			src:  `meta("bad");`,
			code: workflowvalidation.CodeInvalidMetadata,
		},
		{
			name: "invalid workflow artifact",
			src:  `workflow.artifact("log");`,
			code: workflowvalidation.CodeUnsupportedPrimitive,
		},
		{
			name: "invalid agent run",
			src:  `agent.run("review");`,
			code: workflowvalidation.CodeUnsupportedPrimitive,
		},
		{
			name: "invalid parallel",
			src:  `parallel("not-array");`,
			code: workflowvalidation.CodeUnsupportedPrimitive,
		},
		{
			name: "invalid pipeline",
			src:  `pipeline("not-array");`,
			code: workflowvalidation.CodeUnsupportedPrimitive,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := workflowvalidation.Validate(workflowvalidation.Request{
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

func TestValidateFactory_RejectsInlineForbiddenHostAccess(t *testing.T) {
	cfg := &interfaces.FactoryConfig{
		Name: "dynamic-workflow",
		Orchestrator: &interfaces.FactoryOrchestratorConfig{
			Kind: interfaces.OrchestratorKindJavaScript,
			JavaScript: &interfaces.FactoryOrchestratorJavaScriptConfig{
				InlineSource: &interfaces.FactoryOrchestratorJavaScriptInlineSource{
					Encoding: interfaces.OrchestratorInlineEncoding,
					Inline:   `fetch("https://example.com");`,
				},
			},
		},
	}

	result := factoryvalidation.Validate(cfg)
	if !result.HasTargets() {
		t.Fatal("expected inline forbidden host-access validation target")
	}
	if result.Targets[0].Code != workflowvalidation.CodeForbiddenHostAccess {
		t.Fatalf("target code = %q, want %q", result.Targets[0].Code, workflowvalidation.CodeForbiddenHostAccess)
	}
}
