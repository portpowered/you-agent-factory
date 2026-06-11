package workflowruntime_test

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/policy"
	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/result"
	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime"
)

func TestRun_AgentRunFakeChild_EmitsOrderedChildDispatchRecords(t *testing.T) {
	source := readFixture(t, "agent-run-fake-child.workflow.js")
	req := workflowruntime.Request{
		Source:    source,
		SourceRef: "agent-run-fake-child.workflow.js",
		SessionID: "session-agent-run-fake-child",
		Args:      marshalArgs(t, map[string]any{"subject": "workflows"}),
		Metadata: map[string]string{
			"name": "agent-run-fake-child",
		},
		Policy: workflowpolicy.DefaultEffectivePolicy(),
	}

	first := runSuccessful(t, req)
	second := runSuccessful(t, req)

	if len(first.Records) != 3 {
		t.Fatalf("record count = %d, want 3 child dispatch records", len(first.Records))
	}
	assertRecordSequences(t, first.Records)

	wantStatuses := []string{
		workflowruntime.ChildDispatchStatusQueued,
		workflowruntime.ChildDispatchStatusRunning,
		workflowruntime.ChildDispatchStatusCompleted,
	}
	wantDispatchID := "dispatch-1"
	wantArtifactRef := workflowresult.FormatArtifactURI(req.SessionID, "child-artifact-1")
	wantProviderSessionRef := "fake-provider-session-1"
	wantPromptDigest := first.Records[0].ChildDispatch.PromptDigest

	for i, wantStatus := range wantStatuses {
		record := first.Records[i]
		if record.Kind != workflowruntime.RecordKindChildDispatch {
			t.Fatalf("records[%d].kind = %q, want %q", i, record.Kind, workflowruntime.RecordKindChildDispatch)
		}
		child := record.ChildDispatch
		if child == nil {
			t.Fatalf("records[%d] missing child dispatch payload", i)
		}
		if child.Status != wantStatus {
			t.Fatalf("records[%d].status = %q, want %q", i, child.Status, wantStatus)
		}
		if child.DispatchID != wantDispatchID {
			t.Fatalf("records[%d].dispatchId = %q, want %q", i, child.DispatchID, wantDispatchID)
		}
		if child.ChildIndex != 1 {
			t.Fatalf("records[%d].childIndex = %d, want 1", i, child.ChildIndex)
		}
		if child.Label != "summarize-findings" {
			t.Fatalf("records[%d].label = %q", i, child.Label)
		}
		if child.Model != "gpt-test" || child.ReasoningEffort != "medium" {
			t.Fatalf("records[%d] model metadata = %#v", i, child)
		}
		if child.Command != "review" || child.Sandbox != "read-only" {
			t.Fatalf("records[%d] command/sandbox = %#v", i, child)
		}
		if child.ExecutionMode != "fake" {
			t.Fatalf("records[%d].executionMode = %q, want fake", i, child.ExecutionMode)
		}
		if child.ProviderSessionRef != wantProviderSessionRef {
			t.Fatalf("records[%d].providerSessionRef = %q, want %q", i, child.ProviderSessionRef, wantProviderSessionRef)
		}
		if child.ArtifactRef != wantArtifactRef {
			t.Fatalf("records[%d].artifactRef = %q, want %q", i, child.ArtifactRef, wantArtifactRef)
		}
		if child.PromptDigest != wantPromptDigest {
			t.Fatalf("records[%d].promptDigest = %q, want %q", i, child.PromptDigest, wantPromptDigest)
		}
		if child.SchemaDigest == "" {
			t.Fatalf("records[%d].schemaDigest is empty", i)
		}
	}

	projected := projectPrimaryJSON(t, req.SessionID, first.Value)
	if projected["label"] != "agent-run-fake-child" {
		t.Fatalf("projected label = %#v", projected["label"])
	}
	if projected["subject"] != "workflows" {
		t.Fatalf("projected subject = %#v", projected["subject"])
	}
	child, ok := projected["child"].(map[string]any)
	if !ok {
		t.Fatalf("projected child = %#v, want object", projected["child"])
	}
	if child["status"] != workflowruntime.ChildDispatchStatusCompleted {
		t.Fatalf("child status = %#v", child["status"])
	}
	if child["dispatchId"] != wantDispatchID {
		t.Fatalf("child dispatchId = %#v, want %q", child["dispatchId"], wantDispatchID)
	}
	if child["executionMode"] != "fake" {
		t.Fatalf("child executionMode = %#v", child["executionMode"])
	}
	if child["providerSessionRef"] != wantProviderSessionRef {
		t.Fatalf("child providerSessionRef = %#v, want %q", child["providerSessionRef"], wantProviderSessionRef)
	}
	if child["artifactRef"] != wantArtifactRef {
		t.Fatalf("child artifactRef = %#v, want %q", child["artifactRef"], wantArtifactRef)
	}
	if child["label"] != "summarize-findings" || child["model"] != "gpt-test" {
		t.Fatalf("child request metadata = %#v", child)
	}
	if child["reasoningEffort"] != "medium" || child["command"] != "review" || child["sandbox"] != "read-only" {
		t.Fatalf("child options = %#v", child)
	}
	if child["promptDigest"] != wantPromptDigest {
		t.Fatalf("child promptDigest = %#v, want %q", child["promptDigest"], wantPromptDigest)
	}
	output, ok := child["output"].(map[string]any)
	if !ok {
		t.Fatalf("child output = %#v, want object", child["output"])
	}
	if output["text"] != "fake:agent-run-fake-child:summarize-findings:summarize workflows:workflows" {
		t.Fatalf("child output text = %#v", output["text"])
	}
	if output["subject"] != "workflows" {
		t.Fatalf("child output subject = %#v", output["subject"])
	}
	if output["schemaValidated"] != true {
		t.Fatalf("child output schemaValidated = %#v", output["schemaValidated"])
	}

	if recordsJSON(first.Records) != recordsJSON(second.Records) {
		t.Fatalf("record drift across runs:\nfirst=%s\nsecond=%s", recordsJSON(first.Records), recordsJSON(second.Records))
	}
	if string(first.Value.JSON) != string(second.Value.JSON) {
		t.Fatalf("value drift across runs: first=%s second=%s", first.Value.JSON, second.Value.JSON)
	}
}
