package workflow_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/transports/cli/workflow"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/spf13/cobra"
)

var workflowDefinitions = testutil.ScriptedJavaScriptWorkflowDefinitions{
	DefaultSourceContextFunc: func(root string) (factory.WorkflowSourceContext, error) {
		return factory.WorkflowSourceContext{ProjectRoot: root}, nil
	},
	BuildPreviewFunc: scriptedTransportWorkflowPreview,
}

type strictPreviewOperation func(context.Context, factory.WorkflowPreviewInput) (factory.WorkflowPreview, error)

func (operation strictPreviewOperation) PreviewWorkflow(ctx context.Context, input factory.WorkflowPreviewInput) (factory.WorkflowPreview, error) {
	return operation(ctx, input)
}

func TestPreview_ForwardsOnlyDecodedExternalEdgeFields(t *testing.T) {
	called := false
	operation := strictPreviewOperation(func(_ context.Context, input factory.WorkflowPreviewInput) (factory.WorkflowPreview, error) {
		called = true
		if input.ProjectRoot != "/customer/project" ||
			input.Source.Kind != factory.WorkflowSourceKindWorkflowName ||
			input.Source.Value != "review" ||
			string(input.ArgsSchema) != `{"type":"object"}` ||
			input.RequestedPolicy["maxConcurrency"] != float64(2) {
			t.Fatalf("preview input = %#v, want exact decoded edge fields", input)
		}
		return factory.WorkflowPreview{Valid: true}, nil
	})

	var output bytes.Buffer
	err := workflow.Preview(operation, workflow.PreviewConfig{Context: context.Background(),
		SourceConfig: workflow.SourceConfig{
			Dir:                 "/customer/project",
			SourceKind:          string(factory.WorkflowSourceKindWorkflowName),
			SourceValue:         "review",
			ArgsSchema:          `{"type":"object"}`,
			RequestedPolicyJSON: `{"maxConcurrency":2}`,
		},
		JSON: true, Output: &output,
	})
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if !called {
		t.Fatal("PreviewWorkflow was not called")
	}
}

const validWorkflowSource = `
meta({ name: "review", version: 1 });
phase("setup");
log("starting");
`

func scriptedTransportWorkflowPreview(
	request factory.WorkflowPreviewRequest,
) factory.WorkflowPreview {
	sourceRef := "inline.workflow.js"
	if request.Source.Kind == factory.WorkflowSourceKindWorkflowName {
		sourceRef = factory.WorkflowSourceProjectClaudeWorkflowsDir + "/" + request.Source.Value + ".js"
	}
	preview := factory.WorkflowPreview{
		Valid: true,
		SourceResolution: factory.WorkflowSourceResolution{
			RequestKind:  request.Source.Kind,
			RequestValue: request.Source.Value,
			ResolvedKind: request.Source.Kind,
			SourceRef:    sourceRef,
			SourceHash:   "sha256:scripted-source",
			Dialect:      "you-workflow-v1",
			ArtifactRoot: factory.WorkflowSourceArtifactRootDecision{Allowed: true},
			Found:        true,
		},
		PolicyPreview: factory.JavaScriptPolicyPreview{
			EffectivePolicy: factory.DefaultJavaScriptPolicy(),
			PolicyHash:      "sha256:scripted-policy",
		},
		ResultConstraints: factory.WorkflowResultConstraints{
			RequiresStructuredCloneableJSON: true,
			ArtifactURIScheme:               "you-artifact",
			MaxEmbeddedBytes:                1024,
		},
	}

	if len(request.ArgsSchema) > 0 {
		preview.Valid = false
		preview.SourceValidationIssues = []factory.WorkflowPreviewSourceValidationIssue{{
			Code:    factory.WorkflowValidationCodeInvalidArgsSchema,
			Message: "argsSchema.type must be object",
			Path:    "argsSchema.type",
		}}
		return preview
	}
	if request.RequestedPolicy["allowNetwork"] == true {
		preview.Valid = false
		preview.PolicyPreview.ValidationIssues = []factory.JavaScriptPolicyIssue{{
			Code:    factory.JavaScriptPolicyCodeDeniedCapability,
			Message: "network capability denied",
			Path:    "allowNetwork",
		}}
		return preview
	}

	code := ""
	line := 0
	switch request.Source.Value {
	case "review":
		return preview
	case "broken":
		code = factory.WorkflowValidationCodeForbiddenHostAccess
	case "syntax-error":
		code = factory.WorkflowValidationCodeSyntaxError
		line = 1
	case "unsupported-global":
		code = factory.WorkflowValidationCodeUnsupportedGlobal
	case "forbidden-host-access":
		code = factory.WorkflowValidationCodeForbiddenHostAccess
	default:
		if request.Source.Kind == factory.WorkflowSourceKindInlineWorkflow &&
			strings.TrimSpace(request.Source.InlineSource) != "" {
			return preview
		}
		panic("unexpected scripted workflow preview request: " + request.Source.Value)
	}
	preview.Valid = false
	preview.SourceValidationIssues = []factory.WorkflowPreviewSourceValidationIssue{{
		Code:    code,
		Message: "scripted " + code + " diagnostic",
		Path:    sourceRef,
		Line:    line,
	}}
	return preview
}

func TestPreview_ValidWorkflowNameHumanOutput(t *testing.T) {
	projectRoot := t.TempDir()
	writeWorkflow(t, projectRoot, "review.js", validWorkflowSource)

	var output bytes.Buffer
	err := workflow.Preview(workflowDefinitions, workflow.PreviewConfig{Context: context.Background(),
		SourceConfig: workflow.SourceConfig{
			Dir:         projectRoot,
			SourceKind:  string(factory.WorkflowSourceKindWorkflowName),
			SourceValue: "review",
		},
		Output: &output,
	})

	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	text := output.String()
	for _, want := range []string{"Factory preview passed.", "Source hash:", "Policy hash:"} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
}

func TestPreview_JSONOutputMatchesCanonicalFactoryPreview(t *testing.T) {
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
	want := apisurface.FactoryPreviewResultFromPreview(workflowDefinitions.BuildPreview(input))

	var output bytes.Buffer
	if err := workflow.Preview(workflowDefinitions, workflow.PreviewConfig{Context: context.Background(),
		SourceConfig: workflow.SourceConfig{
			Dir:         projectRoot,
			SourceKind:  string(factory.WorkflowSourceKindWorkflowName),
			SourceValue: "review",
		},
		JSON:   true,
		Output: &output,
	}); err != nil {
		t.Fatalf("Preview: %v", err)
	}

	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal canonical preview: %v", err)
	}
	gotJSON := bytes.TrimSpace(output.Bytes())
	if !bytes.Equal(gotJSON, wantJSON) {
		var got factoryapi.FactoryPreviewResult
		if err := json.Unmarshal(gotJSON, &got); err != nil {
			t.Fatalf("decode preview JSON: %v", err)
		}
		t.Fatalf("preview JSON = %#v, want %#v", got, want)
	}
}

func TestPreview_InvalidWorkflowReportsDiagnostics(t *testing.T) {
	projectRoot := t.TempDir()
	writeWorkflow(t, projectRoot, "broken.js", "require('fs');")

	var output bytes.Buffer
	err := workflow.Preview(workflowDefinitions, workflow.PreviewConfig{Context: context.Background(),
		SourceConfig: workflow.SourceConfig{
			Dir:         projectRoot,
			SourceKind:  string(factory.WorkflowSourceKindWorkflowName),
			SourceValue: "broken",
		},
		Output: &output,
	})

	if err == nil {
		t.Fatal("expected preview failure")
	}
	if !strings.Contains(output.String(), "Factory preview failed.") {
		t.Fatalf("output = %q, want failure summary", output.String())
	}
	wantPath := factory.WorkflowSourceProjectClaudeWorkflowsDir + "/broken.js"
	if !strings.Contains(output.String(), wantPath+":") {
		t.Fatalf("output = %q, want path-aware diagnostic prefix %q", output.String(), wantPath)
	}
}

func TestGeneratedMetadataRunEAdaptersUseCommandOutputAndGlobalJSON(t *testing.T) {
	projectRoot := t.TempDir()
	writeWorkflow(t, projectRoot, "review.js", validWorkflowSource)
	jsonOutput := true

	for _, tc := range []struct {
		name string
		run  func(*cobra.Command, []string) error
	}{
		{
			name: "preview",
			run: workflow.PreviewRunE(workflowDefinitions, &workflow.PreviewConfig{Context: context.Background(), SourceConfig: workflow.SourceConfig{
				Dir: projectRoot, SourceKind: string(factory.WorkflowSourceKindWorkflowName), SourceValue: "review",
			}}, &jsonOutput),
		},
		{
			name: "validate",
			run: workflow.ValidateRunE(workflowDefinitions, &workflow.ValidateConfig{Context: context.Background(), SourceConfig: workflow.SourceConfig{
				Dir: projectRoot, SourceKind: string(factory.WorkflowSourceKindWorkflowName), SourceValue: "review",
			}}, &jsonOutput),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var output bytes.Buffer
			cmd := &cobra.Command{}
			cmd.SetContext(context.Background())
			cmd.SetOut(&output)
			if err := tc.run(cmd, nil); err != nil {
				t.Fatalf("RunE: %v", err)
			}
			if !json.Valid(output.Bytes()) {
				t.Fatalf("RunE output = %q, want global-JSON document", output.String())
			}
		})
	}
}

func writeWorkflow(t *testing.T, projectRoot, name, content string) {
	t.Helper()
	workflowDir := filepath.Join(projectRoot, factory.WorkflowSourceProjectClaudeWorkflowsDir)
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, name), []byte(content), 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
}
