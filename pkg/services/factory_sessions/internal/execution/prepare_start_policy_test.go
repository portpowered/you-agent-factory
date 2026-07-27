package factorysessionexecution

import (
	"encoding/json"
	"testing"

	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

func TestApplyInlineFactoryDeclarationPreservesWorkflowFileDefaultPolicy(t *testing.T) {
	t.Parallel()

	defaultPolicy := json.RawMessage(`{"allowedModels":["gpt-allowed"],"mode":"READ_ONLY"}`)
	resolution := factory.WorkflowSourceResolution{Found: true}
	applyInlineFactoryDeclaration(&resolution, Source{
		Kind:         factory.WorkflowSourceKindWorkflowFile,
		WorkflowFile: "/tmp/workflow.js",
		InlineWorkflow: &InlineWorkflowSource{
			DefaultPolicy: defaultPolicy,
		},
	})
	if string(resolution.DefaultPolicy) != string(defaultPolicy) {
		t.Fatalf("resolution defaultPolicy = %s, want %s", resolution.DefaultPolicy, defaultPolicy)
	}
}
