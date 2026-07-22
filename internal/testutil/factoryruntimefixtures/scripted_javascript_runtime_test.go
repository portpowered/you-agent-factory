package factoryruntimefixtures

import (
	"context"
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

func TestScriptedJavaScriptWorkflowsPreviewRequiresExplicitScript(t *testing.T) {
	_, err := (ScriptedJavaScriptWorkflows{}).PreviewWorkflow(context.Background(), factoryruntime.WorkflowPreviewInput{})
	if err == nil || err.Error() != "unexpected PreviewWorkflow call" {
		t.Fatalf("PreviewWorkflow error = %v, want unexpected-call failure", err)
	}
}

func TestScriptedJavaScriptWorkflowsPreviewForwardsExactInput(t *testing.T) {
	want := factoryruntime.WorkflowPreviewInput{ProjectRoot: "/project"}
	service := ScriptedJavaScriptWorkflows{
		PreviewWorkflowFunc: func(_ context.Context, got factoryruntime.WorkflowPreviewInput) (factoryruntime.WorkflowPreview, error) {
			if got.ProjectRoot != want.ProjectRoot {
				t.Fatalf("input = %#v, want %#v", got, want)
			}
			return factoryruntime.WorkflowPreview{Valid: true}, nil
		},
	}
	preview, err := service.PreviewWorkflow(context.Background(), want)
	if err != nil || !preview.Valid {
		t.Fatalf("PreviewWorkflow = %#v, %v, want valid preview", preview, err)
	}
}
