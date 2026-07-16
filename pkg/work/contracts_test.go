package work

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestWorkRequestJSONPreservesPublicContract(t *testing.T) {
	request := WorkRequest{
		RequestID: "request-1",
		Type:      WorkRequestTypeFactoryRequestBatch,
		Works: []Work{{
			Name:       "draft",
			WorkTypeID: "story",
			Content: []WorkContentPart{
				{Type: WorkContentPartTypeText, Text: "hello"},
				{Type: WorkContentPartTypeJSON, JSON: json.RawMessage(`{"answer":42}`)},
			},
		}},
	}

	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal WorkRequest: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("decode WorkRequest JSON: %v", err)
	}
	if decoded["requestId"] != "request-1" || decoded["type"] != "FACTORY_REQUEST_BATCH" {
		t.Fatalf("public request fields changed: %#v", decoded)
	}
	if _, present := decoded["currentChainingTraceId"]; present {
		t.Fatalf("omitted chaining trace was serialized: %#v", decoded)
	}
}

func TestCloneWorkDispatchDetachesMutableState(t *testing.T) {
	dispatch := WorkDispatch{
		PreviousChainingTraceIDs: []string{"trace-1"},
		Execution:                ExecutionMetadata{WorkIDs: []string{"work-1"}},
		InputTokens:              []any{"input"},
		InputBindings:            map[string][]string{"slot": {"work-1"}},
	}
	clone := CloneWorkDispatch(dispatch)

	clone.PreviousChainingTraceIDs[0] = "changed"
	clone.Execution.WorkIDs[0] = "changed"
	clone.InputTokens[0] = "changed"
	clone.InputBindings["slot"][0] = "changed"

	if !reflect.DeepEqual(dispatch.PreviousChainingTraceIDs, []string{"trace-1"}) ||
		!reflect.DeepEqual(dispatch.Execution.WorkIDs, []string{"work-1"}) ||
		!reflect.DeepEqual(dispatch.InputTokens, []any{"input"}) ||
		!reflect.DeepEqual(dispatch.InputBindings, map[string][]string{"slot": {"work-1"}}) {
		t.Fatalf("clone mutated source dispatch: %#v", dispatch)
	}
}

func TestPayloadLineageSnapshotsDetachContent(t *testing.T) {
	item := FactoryWorkItem{
		ID:      "work-1",
		TraceID: "trace-1",
		Content: []WorkContentPart{{
			Type:     WorkContentPartTypeText,
			Text:     "original",
			Metadata: map[string]any{"source": "request"},
		}},
	}
	var projection WorkPayloadLineageProjection
	projection.RecordWorkRequestSnapshot(1, "request-1", item)
	item.Content[0].Text = "changed"
	item.Content[0].Metadata["source"] = "changed"

	resolved := projection.ResolveInitialSubmittedSnapshot("work-1")
	if resolved.Status != WorkPayloadResolutionResolved || resolved.Snapshot == nil {
		t.Fatalf("resolution = %#v", resolved)
	}
	got := resolved.Snapshot.WorkItem.Content[0]
	if got.Text != "original" || got.Metadata["source"] != "request" {
		t.Fatalf("snapshot content was not detached: %#v", got)
	}
}
