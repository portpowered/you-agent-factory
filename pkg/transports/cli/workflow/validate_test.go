package workflow_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/transports/cli/workflow"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

func TestValidate_ValidWorkflowNameHumanOutput(t *testing.T) {
	projectRoot := t.TempDir()
	writeWorkflow(t, projectRoot, "review.js", validWorkflowSource)

	var output bytes.Buffer
	err := workflow.Validate(workflowDefinitions, workflow.ValidateConfig{Context: context.Background(),
		SourceConfig: workflow.SourceConfig{
			Dir:         projectRoot,
			SourceKind:  string(factory.WorkflowSourceKindWorkflowName),
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
	wantRef := factory.WorkflowSourceProjectClaudeWorkflowsDir + "/review.js"
	if !strings.Contains(text, wantRef) {
		t.Fatalf("output missing source ref %q:\n%s", wantRef, text)
	}
}

func TestValidate_JSONOutputMatchesCanonicalValidationResult(t *testing.T) {
	projectRoot := t.TempDir()
	writeWorkflow(t, projectRoot, "review.js", validWorkflowSource)

	ctx, err := workflowDefinitions.DefaultSourceContext(projectRoot)
	if err != nil {
		t.Fatalf("DefaultContext: %v", err)
	}
	input := factory.WorkflowPreviewRequest{
		Source: factory.WorkflowSourceRequest{
			Kind:  factory.WorkflowSourceKindWorkflowName,
			Value: "review",
		},
		Context: ctx,
	}
	want := apisurface.FactoryWorkflowValidationResultFromPreview(
		workflowDefinitions.BuildPreview(input),
	)

	var output bytes.Buffer
	if err := workflow.Validate(workflowDefinitions, workflow.ValidateConfig{Context: context.Background(),
		SourceConfig: workflow.SourceConfig{
			Dir:         projectRoot,
			SourceKind:  string(factory.WorkflowSourceKindWorkflowName),
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
	err := workflow.Validate(workflowDefinitions, workflow.ValidateConfig{Context: context.Background(),
		SourceConfig: workflow.SourceConfig{
			Dir:          projectRoot,
			SourceKind:   string(factory.WorkflowSourceKindInlineWorkflow),
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

func TestValidate_UnsupportedGlobalHumanOutput(t *testing.T) {
	projectRoot := t.TempDir()
	writeWorkflow(t, projectRoot, "unsupported-global.js", `console.log("unsupported global");`)

	var output bytes.Buffer
	err := workflow.Validate(workflowDefinitions, workflow.ValidateConfig{Context: context.Background(),
		SourceConfig: workflow.SourceConfig{
			Dir:         projectRoot,
			SourceKind:  string(factory.WorkflowSourceKindWorkflowName),
			SourceValue: "unsupported-global",
		},
		Output: &output,
	})

	if err == nil {
		t.Fatal("expected validation failure")
	}
	if !strings.Contains(output.String(), factory.WorkflowValidationCodeUnsupportedGlobal) {
		t.Fatalf("output = %q, want unsupported global diagnostic", output.String())
	}
}

func TestValidate_ForbiddenHostAccessHumanOutput(t *testing.T) {
	projectRoot := t.TempDir()
	writeWorkflow(t, projectRoot, "forbidden-host-access.js", `require('fs');`)

	var output bytes.Buffer
	err := workflow.Validate(workflowDefinitions, workflow.ValidateConfig{Context: context.Background(),
		SourceConfig: workflow.SourceConfig{
			Dir:         projectRoot,
			SourceKind:  string(factory.WorkflowSourceKindWorkflowName),
			SourceValue: "forbidden-host-access",
		},
		Output: &output,
	})

	if err == nil {
		t.Fatal("expected validation failure")
	}
	if !strings.Contains(output.String(), factory.WorkflowValidationCodeForbiddenHostAccess) {
		t.Fatalf("output = %q, want forbidden host access diagnostic", output.String())
	}
}

func TestValidate_InvalidArgsSchemaHumanOutput(t *testing.T) {
	projectRoot := t.TempDir()
	writeWorkflow(t, projectRoot, "review.js", validWorkflowSource)

	var output bytes.Buffer
	err := workflow.Validate(workflowDefinitions, workflow.ValidateConfig{Context: context.Background(),
		SourceConfig: workflow.SourceConfig{
			Dir:         projectRoot,
			SourceKind:  string(factory.WorkflowSourceKindWorkflowName),
			SourceValue: "review",
			ArgsSchema:  `{"type":"array"}`,
		},
		Output: &output,
	})

	if err == nil {
		t.Fatal("expected validation failure")
	}
	text := output.String()
	if !strings.Contains(text, factory.WorkflowValidationCodeInvalidArgsSchema) {
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
	err := workflow.Validate(workflowDefinitions, workflow.ValidateConfig{Context: context.Background(),
		SourceConfig: workflow.SourceConfig{
			Dir:                 projectRoot,
			SourceKind:          string(factory.WorkflowSourceKindWorkflowName),
			SourceValue:         "review",
			RequestedPolicyJSON: `{"allowNetwork":true}`,
		},
		Output: &output,
	})

	if err == nil {
		t.Fatal("expected validation failure")
	}
	text := output.String()
	if !strings.Contains(text, factory.JavaScriptPolicyCodeDeniedCapability) {
		t.Fatalf("output = %q, want policy denied capability diagnostic", text)
	}
	if strings.Contains(text, factory.WorkflowValidationCodeSyntaxError) {
		t.Fatalf("output = %q, want policy denial rather than syntax failure", text)
	}
}

func TestValidate_MissingWorkflowNameValueReturnsCommandError(t *testing.T) {
	projectRoot := t.TempDir()

	var output bytes.Buffer
	err := workflow.Validate(workflowDefinitions, workflow.ValidateConfig{Context: context.Background(),
		SourceConfig: workflow.SourceConfig{
			Dir:        projectRoot,
			SourceKind: string(factory.WorkflowSourceKindWorkflowName),
		},
		Output: &output,
	})

	if err == nil {
		t.Fatal("expected missing workflow name value to fail")
	}
	if !strings.Contains(err.Error(), "value is required when kind is WORKFLOW_NAME") {
		t.Fatalf("error = %q", err.Error())
	}
	if output.Len() != 0 {
		t.Fatalf("stdout = %q, want empty output before validation", output.String())
	}
}

func TestValidate_MissingInlineWorkflowSourceReturnsCommandError(t *testing.T) {
	projectRoot := t.TempDir()

	var output bytes.Buffer
	err := workflow.Validate(workflowDefinitions, workflow.ValidateConfig{Context: context.Background(),
		SourceConfig: workflow.SourceConfig{
			Dir:        projectRoot,
			SourceKind: string(factory.WorkflowSourceKindInlineWorkflow),
		},
		Output: &output,
	})

	if err == nil {
		t.Fatal("expected missing inline workflow source to fail")
	}
	if !strings.Contains(err.Error(), "inline is required when kind is INLINE_WORKFLOW") {
		t.Fatalf("error = %q", err.Error())
	}
	if output.Len() != 0 {
		t.Fatalf("stdout = %q, want empty output before validation", output.String())
	}
}

func TestValidate_ConflictingWorkflowNameAndInlineReturnsCommandError(t *testing.T) {
	projectRoot := t.TempDir()

	var output bytes.Buffer
	err := workflow.Validate(workflowDefinitions, workflow.ValidateConfig{Context: context.Background(),
		SourceConfig: workflow.SourceConfig{
			Dir:          projectRoot,
			SourceKind:   string(factory.WorkflowSourceKindWorkflowName),
			SourceValue:  "review",
			InlineSource: `phase("setup");`,
		},
		Output: &output,
	})

	if err == nil {
		t.Fatal("expected conflicting source inputs to fail")
	}
	if !strings.Contains(err.Error(), "--inline cannot be used when kind is WORKFLOW_NAME") {
		t.Fatalf("error = %q", err.Error())
	}
	if output.Len() != 0 {
		t.Fatalf("stdout = %q, want empty output before validation", output.String())
	}
}

func TestValidate_InvalidSourceKindReturnsCommandError(t *testing.T) {
	projectRoot := t.TempDir()

	var output bytes.Buffer
	err := workflow.Validate(workflowDefinitions, workflow.ValidateConfig{Context: context.Background(),
		SourceConfig: workflow.SourceConfig{
			Dir:        projectRoot,
			SourceKind: "UNSUPPORTED",
		},
		Output: &output,
	})

	if err == nil {
		t.Fatal("expected invalid source kind to fail")
	}
	if !strings.Contains(err.Error(), "source kind must be one of") {
		t.Fatalf("error = %q", err.Error())
	}
	if output.Len() != 0 {
		t.Fatalf("stdout = %q, want empty output before validation", output.String())
	}
}

func TestValidate_InvalidRequestedPolicyJSONReturnsCommandError(t *testing.T) {
	projectRoot := t.TempDir()
	writeWorkflow(t, projectRoot, "review.js", validWorkflowSource)

	var output bytes.Buffer
	err := workflow.Validate(workflowDefinitions, workflow.ValidateConfig{Context: context.Background(),
		SourceConfig: workflow.SourceConfig{
			Dir:                 projectRoot,
			SourceKind:          string(factory.WorkflowSourceKindWorkflowName),
			SourceValue:         "review",
			RequestedPolicyJSON: `{`,
		},
		Output: &output,
	})

	if err == nil {
		t.Fatal("expected invalid requested policy JSON to fail")
	}
	if !strings.Contains(err.Error(), "requested policy must be valid JSON") {
		t.Fatalf("error = %q", err.Error())
	}
	if output.Len() != 0 {
		t.Fatalf("stdout = %q, want empty output before validation", output.String())
	}
}

func TestValidate_JSONCommandErrorsKeepStdoutEmpty(t *testing.T) {
	projectRoot := t.TempDir()
	cases := []struct {
		name string
		cfg  workflow.SourceConfig
		want string
	}{
		{
			name: "missing workflow name value",
			cfg: workflow.SourceConfig{
				Dir:        projectRoot,
				SourceKind: string(factory.WorkflowSourceKindWorkflowName),
			},
			want: "value is required when kind is WORKFLOW_NAME",
		},
		{
			name: "conflicting workflow name and inline",
			cfg: workflow.SourceConfig{
				Dir:          projectRoot,
				SourceKind:   string(factory.WorkflowSourceKindWorkflowName),
				SourceValue:  "review",
				InlineSource: `phase("setup");`,
			},
			want: "--inline cannot be used when kind is WORKFLOW_NAME",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var output bytes.Buffer
			err := workflow.Validate(workflowDefinitions, workflow.ValidateConfig{Context: context.Background(),
				SourceConfig: tc.cfg,
				JSON:         true,
				Output:       &output,
			})

			if err == nil {
				t.Fatal("expected command error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tc.want)
			}
			if output.Len() != 0 {
				t.Fatalf("stdout = %q, want empty JSON output on command error", output.String())
			}
		})
	}
}

type jsonBlockingDiagnosticCase struct {
	name     string
	setup    func(t *testing.T, projectRoot string) workflow.SourceConfig
	wantCode string
	wantLine bool
}

func jsonBlockingDiagnosticCases() []jsonBlockingDiagnosticCase {
	return []jsonBlockingDiagnosticCase{
		{
			name: "syntax error",
			setup: func(t *testing.T, projectRoot string) workflow.SourceConfig {
				writeWorkflow(t, projectRoot, "syntax-error.js", `meta({ name: "broken" );`)
				return workflow.SourceConfig{
					Dir:         projectRoot,
					SourceKind:  string(factory.WorkflowSourceKindWorkflowName),
					SourceValue: "syntax-error",
				}
			},
			wantCode: factory.WorkflowValidationCodeSyntaxError,
			wantLine: true,
		},
		{
			name: "unsupported global",
			setup: func(t *testing.T, projectRoot string) workflow.SourceConfig {
				writeWorkflow(t, projectRoot, "unsupported-global.js", `console.log("unsupported global");`)
				return workflow.SourceConfig{
					Dir:         projectRoot,
					SourceKind:  string(factory.WorkflowSourceKindWorkflowName),
					SourceValue: "unsupported-global",
				}
			},
			wantCode: factory.WorkflowValidationCodeUnsupportedGlobal,
		},
		{
			name: "forbidden host access",
			setup: func(t *testing.T, projectRoot string) workflow.SourceConfig {
				writeWorkflow(t, projectRoot, "forbidden-host-access.js", `require('fs');`)
				return workflow.SourceConfig{
					Dir:         projectRoot,
					SourceKind:  string(factory.WorkflowSourceKindWorkflowName),
					SourceValue: "forbidden-host-access",
				}
			},
			wantCode: factory.WorkflowValidationCodeForbiddenHostAccess,
		},
		{
			name: "invalid args schema",
			setup: func(t *testing.T, projectRoot string) workflow.SourceConfig {
				writeWorkflow(t, projectRoot, "review.js", validWorkflowSource)
				return workflow.SourceConfig{
					Dir:         projectRoot,
					SourceKind:  string(factory.WorkflowSourceKindWorkflowName),
					SourceValue: "review",
					ArgsSchema:  `{"type":"array"}`,
				}
			},
			wantCode: factory.WorkflowValidationCodeInvalidArgsSchema,
		},
		{
			name: "policy denied host access",
			setup: func(t *testing.T, projectRoot string) workflow.SourceConfig {
				writeWorkflow(t, projectRoot, "review.js", validWorkflowSource)
				return workflow.SourceConfig{
					Dir:                 projectRoot,
					SourceKind:          string(factory.WorkflowSourceKindWorkflowName),
					SourceValue:         "review",
					RequestedPolicyJSON: `{"allowNetwork":true}`,
				}
			},
			wantCode: factory.JavaScriptPolicyCodeDeniedCapability,
		},
	}
}

func TestValidate_JSONBlockingDiagnosticsMatchCanonicalResult(t *testing.T) {
	for _, tc := range jsonBlockingDiagnosticCases() {
		t.Run(tc.name, func(t *testing.T) {
			assertValidateJSONBlockingDiagnostics(t, tc)
		})
	}
}

func assertValidateJSONBlockingDiagnostics(t *testing.T, tc jsonBlockingDiagnosticCase) {
	t.Helper()
	projectRoot := t.TempDir()
	sourceCfg := tc.setup(t, projectRoot)
	want := canonicalValidationResult(t, projectRoot, sourceCfg)
	if want.Valid {
		t.Fatal("expected invalid canonical validation result")
	}

	var output bytes.Buffer
	err := workflow.Validate(workflowDefinitions, workflow.ValidateConfig{Context: context.Background(),
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
}

func canonicalValidationResult(t *testing.T, projectRoot string, sourceCfg workflow.SourceConfig) apisurface.FactoryWorkflowValidationResult {
	t.Helper()
	ctx, err := workflowDefinitions.DefaultSourceContext(projectRoot)
	if err != nil {
		t.Fatalf("DefaultContext: %v", err)
	}
	input := factory.WorkflowPreviewRequest{
		Source: factory.WorkflowSourceRequest{
			Kind:         factory.WorkflowSourceKind(strings.TrimSpace(sourceCfg.SourceKind)),
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
		workflowDefinitions.BuildPreview(input),
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
