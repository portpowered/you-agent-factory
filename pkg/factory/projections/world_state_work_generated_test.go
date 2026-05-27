package projections

import (
	"encoding/json"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestGeneratedWorkContentToDomain_PreservesNilEmptyAndOrderedParts(t *testing.T) {
	if got := generatedWorkContentToDomain(nil); got != nil {
		t.Fatalf("nil content = %#v, want nil", got)
	}

	empty := factoryapi.WorkContent{}
	if got := generatedWorkContentToDomain(&empty); got != nil {
		t.Fatalf("empty content = %#v, want nil", got)
	}

	content := factoryapi.WorkContent{
		workTextContentPartForProjectionTest(t, "outline"),
		workImageContentPartForProjectionTest(t, "diagram.png"),
	}

	got := generatedWorkContentToDomain(&content)
	want := []interfaces.WorkContentPart{
		{Type: interfaces.WorkContentPartTypeText, Text: "outline"},
		{Type: interfaces.WorkContentPartTypeImage, File: "diagram.png"},
	}
	if len(got) != len(want) {
		t.Fatalf("content part count = %d, want %d (%#v)", len(got), len(want), got)
	}
	for i := range want {
		if !projectionWorkContentPartEqual(got[i], want[i]) {
			t.Fatalf("content part %d = %#v, want %#v", i, got[i], want[i])
		}
	}
}

func TestFactoryWorkItemFromGenerated_FallsBackCurrentTraceAndPreservesOptionalFields(t *testing.T) {
	work := factoryapi.Work{
		Name:               "Write docs",
		WorkId:             stringPtrForProjectionTest("work-1"),
		WorkTypeName:       stringPtrForProjectionTest("task"),
		TraceId:            stringPtrForProjectionTest("trace-1"),
		ChainingTraceDepth: intPtrForProjectionTest(2),
		PreviousChainingTraceIds: stringSlicePtrForProjectionTest([]string{
			"chain-a",
			"chain-b",
		}),
		Content: workContentPtrForProjectionTest(t,
			workTextContentPartForProjectionTest(t, "draft"),
			workImageContentPartForProjectionTest(t, "draft.png"),
		),
		Tags: generatedStringMapForProjectionTest(map[string]string{"priority": "high"}),
	}

	got := factoryWorkItemFromGenerated(work)
	if got.CurrentChainingTraceID != "trace-1" {
		t.Fatalf("current chaining trace ID = %q, want trace fallback", got.CurrentChainingTraceID)
	}
	if got.TraceID != "trace-1" {
		t.Fatalf("trace ID = %q, want trace-1", got.TraceID)
	}
	if len(got.PreviousChainingTraceIDs) != 2 || got.PreviousChainingTraceIDs[0] != "chain-a" || got.PreviousChainingTraceIDs[1] != "chain-b" {
		t.Fatalf("previous chaining trace IDs = %#v, want [chain-a chain-b]", got.PreviousChainingTraceIDs)
	}
	if len(got.Content) != 2 || got.Content[0].Text != "draft" || got.Content[1].File != "draft.png" {
		t.Fatalf("content = %#v, want preserved ordered parts", got.Content)
	}
	if got.Tags["priority"] != "high" {
		t.Fatalf("tags = %#v, want priority=high", got.Tags)
	}
}

func workContentPtrForProjectionTest(t *testing.T, parts ...factoryapi.WorkContentPart) *factoryapi.WorkContent {
	t.Helper()
	content := factoryapi.WorkContent(parts)
	return &content
}

func workTextContentPartForProjectionTest(t *testing.T, text string) factoryapi.WorkContentPart {
	t.Helper()
	var part factoryapi.WorkContentPart
	if err := part.FromWorkTextContentPart(factoryapi.WorkTextContentPart{
		Type: factoryapi.WorkContentPartTypeText,
		Text: text,
	}); err != nil {
		t.Fatalf("build text part: %v", err)
	}
	return part
}

func workImageContentPartForProjectionTest(t *testing.T, file string) factoryapi.WorkContentPart {
	t.Helper()
	var part factoryapi.WorkContentPart
	if err := part.FromWorkImageContentPart(factoryapi.WorkImageContentPart{
		Type: factoryapi.WorkContentPartTypeImage,
		File: file,
	}); err != nil {
		t.Fatalf("build image part: %v", err)
	}
	return part
}

func projectionWorkContentPartEqual(left, right interfaces.WorkContentPart) bool {
	if left.Type != right.Type ||
		left.Text != right.Text ||
		left.File != right.File ||
		left.Label != right.Label ||
		left.Role != right.Role ||
		left.ContentType != right.ContentType ||
		left.ArtifactID != right.ArtifactID ||
		string(left.JSON) != string(right.JSON) {
		return false
	}
	leftMetadata, _ := json.Marshal(left.Metadata)
	rightMetadata, _ := json.Marshal(right.Metadata)
	return string(leftMetadata) == string(rightMetadata)
}

func TestWorkItemRefsForProjectionOwners_FilterCustomerWorkAndPreserveLineage(t *testing.T) {
	itemsByID := map[string]interfaces.FactoryWorkItem{
		"work-2": {ID: "work-2", WorkTypeID: "task", DisplayName: "Second", CurrentChainingTraceID: "chain-2", PreviousChainingTraceIDs: []string{"chain-0", "chain-1"}, TraceID: "trace-2"},
		"work-1": {ID: "work-1", WorkTypeID: "task", DisplayName: "First", CurrentChainingTraceID: "chain-1", PreviousChainingTraceIDs: []string{"chain-0"}, TraceID: "trace-1"},
		"time-1": {ID: "time-1", WorkTypeID: interfaces.SystemTimeWorkTypeID, DisplayName: "tick"},
	}

	refsByID := workItemRefsForIDs([]string{"work-2", "time-1", "work-1", "work-2"}, itemsByID)
	if len(refsByID) != 2 || refsByID[0].WorkID != "work-1" || refsByID[1].WorkID != "work-2" {
		t.Fatalf("workItemRefsForIDs = %#v, want sorted customer refs", refsByID)
	}
	if refsByID[0].CurrentChainingTraceID != "chain-1" || len(refsByID[1].PreviousChainingTraceIDs) != 2 {
		t.Fatalf("workItemRefsForIDs lineage = %#v, want explicit chaining fields", refsByID)
	}
	if refsByID[0].ChainingTraceDepth != 0 || refsByID[1].ChainingTraceDepth != 0 {
		t.Fatalf("workItemRefsForIDs unexpected implicit depth = %#v, want zero when source depth absent", refsByID)
	}

	refsForItems := workItemRefsForItems([]interfaces.FactoryWorkItem{
		itemsByID["work-2"],
		itemsByID["time-1"],
		itemsByID["work-2"],
		itemsByID["work-1"],
	})
	if len(refsForItems) != 2 || refsForItems[0].WorkID != "work-2" || refsForItems[1].WorkID != "work-1" {
		t.Fatalf("workItemRefsForItems = %#v, want first-occurrence customer refs", refsForItems)
	}

	refsForInputs := workItemRefsForInputs([]interfaces.WorkstationInput{
		{WorkItem: &interfaces.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", DisplayName: "First", CurrentChainingTraceID: "chain-1", PreviousChainingTraceIDs: []string{"chain-0"}}},
		{WorkItem: &interfaces.FactoryWorkItem{ID: "time-1", WorkTypeID: interfaces.SystemTimeWorkTypeID, DisplayName: "tick"}},
		{WorkItem: &interfaces.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", DisplayName: "First", CurrentChainingTraceID: "chain-1", PreviousChainingTraceIDs: []string{"chain-0"}}},
		{WorkItem: &interfaces.FactoryWorkItem{ID: "work-2", WorkTypeID: "task", DisplayName: "Second", CurrentChainingTraceID: "chain-2", PreviousChainingTraceIDs: []string{"chain-0", "chain-1"}}},
	})
	if len(refsForInputs) != 2 || refsForInputs[0].WorkID != "work-1" || refsForInputs[1].WorkID != "work-2" {
		t.Fatalf("workItemRefsForInputs = %#v, want first-occurrence customer refs", refsForInputs)
	}
}
