package projections

import (
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
		if got[i] != want[i] {
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
