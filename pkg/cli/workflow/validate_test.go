package workflow_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/cli/workflow"
	workflowpolicy "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/policy"
	workflowpreview "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/preview"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
	workflowvalidation "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/validation"
)

func TestValidate_ValidWorkflowNameHumanOutput(t *testing.T) {
	projectRoot := t.TempDir()
	writeWorkflow(t, projectRoot, "review.js", validWorkflowSource)

	var output bytes.Buffer
	err := workflow.Validate(workflow.ValidateConfig{
		SourceConfig: workflow.SourceConfig{
			Dir:         projectRoot,
			SourceKind:  string(workflowsource.KindWorkflowName),
			SourceValue: "review",
		},
		Output: &output,
	})
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	text := output.String()
	for _, want := range []string{"Workflow validation passed.", "Source hash:"} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
	wantRef := workflowsource.ProjectClaudeWorkflowsDir + "/review.js"
	if !strings.Contains(text, wantRef) {
		t.Fatalf("output missing source ref %q:\n%s", wantRef, text)
	}
}

func TestValidate_JSONOutputMatchesCanonicalValidationResult(t *testing.T) {
	projectRoot := t.TempDir()
	writeWorkflow(t, projectRoot, "review.js", validWorkflowSource)

	ctx, err := workflowsource.DefaultContext(projectRoot)
	if err != nil {
		t.Fatalf("DefaultContext: %v", err)
	}
	input := workflowpreview.Request{
		Source: workflowsource.Request{
			Kind:  workflowsource.KindWorkflowName,
			Value: "review",
		},
		Context: ctx,
	}
	want := apisurface.FactoryWorkflowValidationResultFromPreview(
		apisurface.BuildFactoryWorkflowValidation(input),
	)

	var output bytes.Buffer
	if err := workflow.Validate(workflow.ValidateConfig{
		SourceConfig: workflow.SourceConfig{
			Dir:         projectRoot,
			SourceKind:  string(workflowsource.KindWorkflowName),
			SourceValue: "review",
		},
		JSON:   true,
		Output: &output,
	}); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal canonical validation result: %v", err)
	}
	gotJSON := bytes.TrimSpace(output.Bytes())
	if !bytes.Equal(gotJSON, wantJSON) {
		var got apisurface.FactoryWorkflowValidationResult
		if err := json.Unmarshal(gotJSON, &got); err != nil {
			t.Fatalf("decode validation JSON: %v", err)
		}
		t.Fatalf("validation JSON = %#v, want %#v", got, want)
	}
	if !want.Valid || len(want.BlockingDiagnostics) != 0 {
		t.Fatalf("want valid result with empty blocking diagnostics, got valid=%v diagnostics=%d",
			want.Valid, len(want.BlockingDiagnostics))
	}
}

func TestValidate_InlineWorkflowSource(t *testing.T) {
	projectRoot := t.TempDir()

	var output bytes.Buffer
	err := workflow.Validate(workflow.ValidateConfig{
		SourceConfig: workflow.SourceConfig{
			Dir:          projectRoot,
			SourceKind:   string(workflowsource.KindInlineWorkflow),
			InlineSource: `meta({ name: "inline", version: 1 }); phase("setup");`,
		},
		Output: &output,
	})
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if !strings.Contains(output.String(), "Workflow validation passed.") {
		t.Fatalf("output = %q, want validation passed summary", output.String())
	}
}

func TestValidate_SyntaxErrorHumanOutput(t *testing.T) {
	projectRoot := t.TempDir()
	writeWorkflow(t, projectRoot, "broken.js", `meta({ name: "broken" );`)

	var output bytes.Buffer
	err := workflow.Validate(workflow.ValidateConfig{
		SourceConfig: workflow.SourceConfig{
			Dir:         projectRoot,
			SourceKind:  string(workflowsource.KindWorkflowName),
			SourceValue: "broken",
		},
		Output: &output,
	})
	if err == nil {
		t.Fatal("expected validation failure")
	}
	text := output.String()
	for _, want := range []string{
		"Workflow validation failed.",
		"Blocking diagnostics:",
		workflowvalidation.CodeSyntaxError,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
	wantPath := workflowsource.ProjectClaudeWorkflowsDir + "/broken.js"
	if !strings.Contains(text, wantPath+":") {
		t.Fatalf("output = %q, want path-aware diagnostic prefix %q", text, wantPath)
	}
}

func TestValidate_UnsupportedGlobalHumanOutput(t *testing.T) {
	projectRoot := t.TempDir()
	writeWorkflow(t, projectRoot, "unsafe.js", `console.log("unsupported global");`)

	var output bytes.Buffer
	err := workflow.Validate(workflow.ValidateConfig{
		SourceConfig: workflow.SourceConfig{
			Dir:         projectRoot,
			SourceKind:  string(workflowsource.KindWorkflowName),
			SourceValue: "unsafe",
		},
		Output: &output,
	})
	if err == nil {
		t.Fatal("expected validation failure")
	}
	if !strings.Contains(output.String(), workflowvalidation.CodeUnsupportedGlobal) {
		t.Fatalf("output = %q, want unsupported global diagnostic", output.String())
	}
}

func TestValidate_ForbiddenHostAccessHumanOutput(t *testing.T) {
	projectRoot := t.TempDir()
	writeWorkflow(t, projectRoot, "unsafe.js", `require('fs');`)

	var output bytes.Buffer
	err := workflow.Validate(workflow.ValidateConfig{
		SourceConfig: workflow.SourceConfig{
			Dir:         projectRoot,
			SourceKind:  string(workflowsource.KindWorkflowName),
			SourceValue: "unsafe",
		},
		Output: &output,
	})
	if err == nil {
		t.Fatal("expected validation failure")
	}
	if !strings.Contains(output.String(), workflowvalidation.CodeForbiddenHostAccess) {
		t.Fatalf("output = %q, want forbidden host access diagnostic", output.String())
	}
}

func TestValidate_InvalidArgsSchemaHumanOutput(t *testing.T) {
	projectRoot := t.TempDir()
	writeWorkflow(t, projectRoot, "review.js", validWorkflowSource)

	var output bytes.Buffer
	err := workflow.Validate(workflow.ValidateConfig{
		SourceConfig: workflow.SourceConfig{
			Dir:         projectRoot,
			SourceKind:  string(workflowsource.KindWorkflowName),
			SourceValue: "review",
			ArgsSchema:  `{"type":"array"}`,
		},
		Output: &output,
	})
	if err == nil {
		t.Fatal("expected validation failure")
	}
	text := output.String()
	if !strings.Contains(text, workflowvalidation.CodeInvalidArgsSchema) {
		t.Fatalf("output = %q, want invalid args schema diagnostic", text)
	}
	if !strings.Contains(text, "argsSchema.type") {
		t.Fatalf("output = %q, want args schema path detail", text)
	}
}

func TestValidate_PolicyDeniedHostAccessHumanOutput(t *testing.T) {
	projectRoot := t.TempDir()
	writeWorkflow(t, projectRoot, "review.js", validWorkflowSource)

	var output bytes.Buffer
	err := workflow.Validate(workflow.ValidateConfig{
		SourceConfig: workflow.SourceConfig{
			Dir:                 projectRoot,
			SourceKind:          string(workflowsource.KindWorkflowName),
			SourceValue:         "review",
			RequestedPolicyJSON: `{"allowNetwork":true}`,
		},
		Output: &output,
	})
	if err == nil {
		t.Fatal("expected validation failure")
	}
	text := output.String()
	if !strings.Contains(text, workflowpolicy.CodeDeniedCapability) {
		t.Fatalf("output = %q, want policy denied capability diagnostic", text)
	}
	if strings.Contains(text, workflowvalidation.CodeSyntaxError) {
		t.Fatalf("output = %q, want policy denial rather than syntax failure", text)
	}
}

func TestValidate_JSONBlockingDiagnosticsMatchCanonicalResult(t *testing.T) {
	cases := []struct {
		name      string
		setup     func(projectRoot string) workflow.SourceConfig
		wantCode  string
		wantLine  bool
	}{
		{
			name: "syntax error",
			setup: func(projectRoot string) workflow.SourceConfig {
				writeWorkflow(t, projectRoot, "broken.js", `meta({ name: "broken" );`)
				return workflow.SourceConfig{
					Dir:         projectRoot,
					SourceKind:  string(workflowsource.KindWorkflowName),
					SourceValue: "broken",
				}
			},
			wantCode: workflowvalidation.CodeSyntaxError,
			wantLine: true,
		},
		{
			name: "unsupported global",
			setup: func(projectRoot string) workflow.SourceConfig {
				writeWorkflow(t, projectRoot, "unsafe.js", `console.log("unsupported global");`)
				return workflow.SourceConfig{
					Dir:         projectRoot,
					SourceKind:  string(workflowsource.KindWorkflowName),
					SourceValue: "unsafe",
				}
			},
			wantCode: workflowvalidation.CodeUnsupportedGlobal,
		},
		{
			name: "forbidden host access",
			setup: func(projectRoot string) workflow.SourceConfig {
				writeWorkflow(t, projectRoot, "unsafe.js", `require('fs');`)
				return workflow.SourceConfig{
					Dir:         projectRoot,
					SourceKind:  string(workflowsource.KindWorkflowName),
					SourceValue: "unsafe",
				}
			},
			wantCode: workflowvalidation.CodeForbiddenHostAccess,
		},
		{
			name: "invalid args schema",
			setup: func(projectRoot string) workflow.SourceConfig {
				writeWorkflow(t, projectRoot, "review.js", validWorkflowSource)
				return workflow.SourceConfig{
					Dir:         projectRoot,
					SourceKind:  string(workflowsource.KindWorkflowName),
					SourceValue: "review",
					ArgsSchema:  `{"type":"array"}`,
				}
			},
			wantCode: workflowvalidation.CodeInvalidArgsSchema,
		},
		{
			name: "policy denied host access",
			setup: func(projectRoot string) workflow.SourceConfig {
				writeWorkflow(t, projectRoot, "review.js", validWorkflowSource)
				return workflow.SourceConfig{
					Dir:                 projectRoot,
					SourceKind:          string(workflowsource.KindWorkflowName),
					SourceValue:         "review",
					RequestedPolicyJSON: `{"allowNetwork":true}`,
				}
			},
			wantCode: workflowpolicy.CodeDeniedCapability,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			projectRoot := t.TempDir()
			sourceCfg := tc.setup(projectRoot)
			want := canonicalValidationResult(t, projectRoot, sourceCfg)
			if want.Valid {
				t.Fatal("expected invalid canonical validation result")
			}

			var output bytes.Buffer
			err := workflow.Validate(workflow.ValidateConfig{
				SourceConfig: sourceCfg,
				JSON:         true,
				Output:       &output,
			})
			if err == nil {
				t.Fatal("expected validation failure exit")
			}

			wantJSON, err := json.Marshal(want)
			if err != nil {
				t.Fatalf("marshal canonical validation result: %v", err)
			}
			gotJSON := bytes.TrimSpace(output.Bytes())
			if !bytes.Equal(gotJSON, wantJSON) {
				var got apisurface.FactoryWorkflowValidationResult
				if err := json.Unmarshal(gotJSON, &got); err != nil {
					t.Fatalf("decode validation JSON: %v", err)
				}
				t.Fatalf("validation JSON = %#v, want %#v", got, want)
			}

			diagnostic := findBlockingDiagnostic(want.BlockingDiagnostics, tc.wantCode)
			if diagnostic == nil {
				t.Fatalf("blocking diagnostics = %#v, want code %q", want.BlockingDiagnostics, tc.wantCode)
			}
			if strings.TrimSpace(diagnostic.Message) == "" {
				t.Fatalf("diagnostic = %#v, want message", diagnostic)
			}
			if tc.wantLine && (diagnostic.Line == nil || *diagnostic.Line <= 0) {
				t.Fatalf("diagnostic = %#v, want source line", diagnostic)
			}
		})
	}
}

func canonicalValidationResult(t *testing.T, projectRoot string, sourceCfg workflow.SourceConfig) apisurface.FactoryWorkflowValidationResult {
	t.Helper()
	ctx, err := workflowsource.DefaultContext(projectRoot)
	if err != nil {
		t.Fatalf("DefaultContext: %v", err)
	}
	input := workflowpreview.Request{
		Source: workflowsource.Request{
			Kind:         workflowsource.Kind(strings.TrimSpace(sourceCfg.SourceKind)),
			Value:        strings.TrimSpace(sourceCfg.SourceValue),
			InlineSource: strings.TrimSpace(sourceCfg.InlineSource),
			ArtifactRoot: strings.TrimSpace(sourceCfg.ArtifactRoot),
		},
		Context: ctx,
	}
	if trimmed := strings.TrimSpace(sourceCfg.ArgsSchema); trimmed != "" {
		input.ArgsSchema = []byte(trimmed)
	}
	if trimmed := strings.TrimSpace(sourceCfg.RequestedPolicyJSON); trimmed != "" {
		if err := json.Unmarshal([]byte(trimmed), &input.RequestedPolicy); err != nil {
			t.Fatalf("unmarshal requested policy: %v", err)
		}
	}
	return apisurface.FactoryWorkflowValidationResultFromPreview(
		apisurface.BuildFactoryWorkflowValidation(input),
	)
}

func findBlockingDiagnostic(diagnostics []factoryapi.WorkflowDiagnostic, code string) *factoryapi.WorkflowDiagnostic {
	for i := range diagnostics {
		if diagnostics[i].Code == code {
			return &diagnostics[i]
		}
	}
	return nil
}
