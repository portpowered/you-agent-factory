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

	wantPromptDigest := assertFakeChildDispatchRecords(t, first.Records, req.SessionID)
	assertFakeChildProjectedValue(t, req.SessionID, first.Value, wantPromptDigest)

	if recordsJSON(first.Records) != recordsJSON(second.Records) {
		t.Fatalf("record drift across runs:\nfirst=%s\nsecond=%s", recordsJSON(first.Records), recordsJSON(second.Records))
	}
	if string(first.Value.JSON) != string(second.Value.JSON) {
		t.Fatalf("value drift across runs: first=%s second=%s", first.Value.JSON, second.Value.JSON)
	}
}

func assertFakeChildDispatchRecords(t *testing.T, records []workflowruntime.RuntimeRecord, sessionID string) string {
	t.Helper()

	wantStatuses := []string{
		workflowruntime.ChildDispatchStatusQueued,
		workflowruntime.ChildDispatchStatusRunning,
		workflowruntime.ChildDispatchStatusCompleted,
	}
	wantPromptDigest := records[0].ChildDispatch.PromptDigest
	wantArtifactRef := workflowresult.FormatArtifactURI(sessionID, "child-artifact-1")

	for i, wantStatus := range wantStatuses {
		assertFakeChildDispatchRecord(t, records[i], i, wantStatus, wantPromptDigest, wantArtifactRef)
	}
	return wantPromptDigest
}

func assertFakeChildDispatchRecord(
	t *testing.T,
	record workflowruntime.RuntimeRecord,
	index int,
	wantStatus string,
	wantPromptDigest string,
	wantArtifactRef string,
) {
	t.Helper()
	if record.Kind != workflowruntime.RecordKindChildDispatch {
		t.Fatalf("records[%d].kind = %q, want %q", index, record.Kind, workflowruntime.RecordKindChildDispatch)
	}
	child := record.ChildDispatch
	if child == nil {
		t.Fatalf("records[%d] missing child dispatch payload", index)
	}
	if child.Status != wantStatus {
		t.Fatalf("records[%d].status = %q, want %q", index, child.Status, wantStatus)
	}
	if child.DispatchID != "dispatch-1" {
		t.Fatalf("records[%d].dispatchId = %q, want dispatch-1", index, child.DispatchID)
	}
	if child.ChildIndex != 1 {
		t.Fatalf("records[%d].childIndex = %d, want 1", index, child.ChildIndex)
	}
	assertFakeChildDispatchRecordMetadata(t, child, index, wantPromptDigest, wantArtifactRef)
}

func assertFakeChildDispatchRecordMetadata(
	t *testing.T,
	child *workflowruntime.ChildDispatchRecord,
	index int,
	wantPromptDigest string,
	wantArtifactRef string,
) {
	t.Helper()
	if child.Label != "summarize-findings" {
		t.Fatalf("records[%d].label = %q", index, child.Label)
	}
	if child.Model != "gpt-test" || child.ReasoningEffort != "medium" {
		t.Fatalf("records[%d] model metadata = %#v", index, child)
	}
	if child.Command != "review" || child.Sandbox != "read-only" {
		t.Fatalf("records[%d] command/sandbox = %#v", index, child)
	}
	if child.ExecutionMode != "fake" {
		t.Fatalf("records[%d].executionMode = %q, want fake", index, child.ExecutionMode)
	}
	if child.ProviderSessionRef != "fake-provider-session-1" {
		t.Fatalf("records[%d].providerSessionRef = %q, want fake-provider-session-1", index, child.ProviderSessionRef)
	}
	if child.ArtifactRef != wantArtifactRef {
		t.Fatalf("records[%d].artifactRef = %q, want %q", index, child.ArtifactRef, wantArtifactRef)
	}
	if child.PromptDigest != wantPromptDigest {
		t.Fatalf("records[%d].promptDigest = %q, want %q", index, child.PromptDigest, wantPromptDigest)
	}
	if child.SchemaDigest == "" {
		t.Fatalf("records[%d].schemaDigest is empty", index)
	}
}

func assertFakeChildProjectedValue(t *testing.T, sessionID string, value workflowresult.TypedValue, wantPromptDigest string) {
	t.Helper()
	projected := projectPrimaryJSON(t, sessionID, value)
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
	assertFakeChildProjectedMetadata(t, child, sessionID, wantPromptDigest)
	assertFakeChildProjectedOutput(t, child)
}

func assertFakeChildProjectedMetadata(t *testing.T, child map[string]any, sessionID string, wantPromptDigest string) {
	t.Helper()
	wantArtifactRef := workflowresult.FormatArtifactURI(sessionID, "child-artifact-1")
	if child["status"] != workflowruntime.ChildDispatchStatusCompleted {
		t.Fatalf("child status = %#v", child["status"])
	}
	if child["dispatchId"] != "dispatch-1" {
		t.Fatalf("child dispatchId = %#v, want dispatch-1", child["dispatchId"])
	}
	if child["executionMode"] != "fake" {
		t.Fatalf("child executionMode = %#v", child["executionMode"])
	}
	if child["providerSessionRef"] != "fake-provider-session-1" {
		t.Fatalf("child providerSessionRef = %#v, want fake-provider-session-1", child["providerSessionRef"])
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
}

func assertFakeChildProjectedOutput(t *testing.T, child map[string]any) {
	t.Helper()
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
}
