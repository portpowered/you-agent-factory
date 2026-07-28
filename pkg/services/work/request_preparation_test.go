package work

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestPrepareWorkRequestOwnsLineageContentAndMutationDetachment(t *testing.T) {
	service := mustRequestPreparationService(t)
	original := WorkRequest{
		RequestID: "request-1",
		Type:      WorkRequestTypeFactoryRequestBatch,
		Works: []Work{
			{
				Name:       "  draft  ",
				WorkTypeID: "task",
				Content: []WorkContentPart{
					{
						Type: WorkContentPartTypeImage,
						File: "fixtures/ui.png",
						Metadata: map[string]any{
							"source": "customer",
						},
					},
				},
				Payload: map[string]any{"title": "Draft"},
				Tags:    map[string]string{"priority": "high"},
			},
			{
				Name:                     "review",
				WorkTypeID:               "review",
				CurrentChainingTraceID:   "chain-review",
				PreviousChainingTraceIDs: []string{"chain-parent"},
				Content: []WorkContentPart{
					{Type: WorkContentPartTypeText, Text: "Review"},
				},
			},
		},
	}

	prepared, err := service.PrepareWorkRequest(context.Background(), WorkRequestPreparation{
		Request: original,
	})
	if err != nil {
		t.Fatalf("PrepareWorkRequest: %v", err)
	}
	if prepared.CurrentChainingTraceID != "chain-review" {
		t.Fatalf("request lineage = %q, want chain-review", prepared.CurrentChainingTraceID)
	}
	if prepared.Works[0].CurrentChainingTraceID != "chain-review" ||
		prepared.Works[0].TraceID != "chain-review" {
		t.Fatalf("inherited first Work lineage = %#v", prepared.Works[0])
	}
	if prepared.Works[1].TraceID != "chain-review" {
		t.Fatalf("second Work trace = %q, want chain-review", prepared.Works[1].TraceID)
	}
	if prepared.Works[0].Name != "draft" {
		t.Fatalf("normalized name = %q, want draft", prepared.Works[0].Name)
	}
	content := prepared.Works[0].Content[0]
	if content.File != "" || content.URL != "file://fixtures/ui.png" {
		t.Fatalf("prepared file content = %#v, want canonical file URL", content)
	}

	prepared.Works[0].Tags["priority"] = "changed"
	prepared.Works[0].Payload.(map[string]any)["title"] = "Changed"
	prepared.Works[0].Content[0].Metadata["source"] = "changed"
	prepared.Works[1].PreviousChainingTraceIDs[0] = "changed"
	if original.Works[0].Tags["priority"] != "high" ||
		original.Works[0].Payload.(map[string]any)["title"] != "Draft" ||
		original.Works[0].Content[0].Metadata["source"] != "customer" ||
		original.Works[1].PreviousChainingTraceIDs[0] != "chain-parent" {
		t.Fatalf("preparation mutated caller-owned request: %#v", original)
	}
}

func TestPrepareWorkRequestUsesStableRequestLineageFallback(t *testing.T) {
	service := mustRequestPreparationService(t)
	prepared, err := service.PrepareWorkRequest(context.Background(), WorkRequestPreparation{
		Request: WorkRequest{
			RequestID: "request-stable",
			Type:      WorkRequestTypeFactoryRequestBatch,
			Works:     []Work{{Name: "draft", WorkTypeID: "task"}},
		},
	})
	if err != nil {
		t.Fatalf("PrepareWorkRequest: %v", err)
	}
	if prepared.CurrentChainingTraceID != "trace-request-stable" ||
		prepared.Works[0].CurrentChainingTraceID != "trace-request-stable" ||
		prepared.Works[0].TraceID != "trace-request-stable" {
		t.Fatalf("stable lineage = %#v", prepared)
	}
}

func TestPrepareWorkRequestFillsMissingBatchWorkTypeFromDefault(t *testing.T) {
	service := mustRequestPreparationService(t)
	prepared, err := service.PrepareWorkRequest(context.Background(), WorkRequestPreparation{
		Request: WorkRequest{
			RequestID: "request-default-type",
			Type:      WorkRequestTypeFactoryRequestBatch,
			Works:     []Work{{Name: "draft", Payload: map[string]string{"title": "Draft"}}},
		},
		DefaultWorkTypeID: "task",
	})
	if err != nil {
		t.Fatalf("PrepareWorkRequest: %v", err)
	}
	if prepared.Works[0].WorkTypeID != "task" {
		t.Fatalf("work type = %q, want task", prepared.Works[0].WorkTypeID)
	}
}

func TestPrepareWorkRequestOwnsPublicContentAliasNormalization(t *testing.T) {
	original := WorkRequest{
		RequestID: "request-aliases",
		Type:      WorkRequestTypeFactoryRequestBatch,
		Works: []Work{{
			Name: "draft", WorkTypeID: "task",
			Content: []WorkContentPart{
				{Type: "TEXT", Text: "Draft"},
				{Type: "IMAGE", URL: "file://fixtures/draft.png"},
			},
		}},
	}

	prepared, err := mustRequestPreparationService(t).PrepareWorkRequest(
		context.Background(),
		WorkRequestPreparation{Request: original},
	)
	if err != nil {
		t.Fatalf("PrepareWorkRequest: %v", err)
	}
	if prepared.Works[0].Content[0].Type != WorkContentPartTypeText ||
		prepared.Works[0].Content[1].Type != WorkContentPartTypeImage {
		t.Fatalf("prepared aliases = %#v, want canonical text and image", prepared.Works[0].Content)
	}
	if original.Works[0].Content[0].Type != "TEXT" || original.Works[0].Content[1].Type != "IMAGE" {
		t.Fatalf("preparation mutated caller aliases: %#v", original.Works[0].Content)
	}
}

func TestPrepareWorkContentOwnsAdmissionAndReturnsDetachedCanonicalValues(t *testing.T) {
	original := []WorkContentPart{{
		Type: "IMAGE", File: "fixtures/draft.png",
		Metadata: map[string]any{"source": "customer"},
	}}
	prepared, err := NewContentPreparation().PrepareWorkContent(context.Background(), original)
	if err != nil {
		t.Fatalf("PrepareWorkContent: %v", err)
	}
	if len(prepared) != 1 || prepared[0].Type != WorkContentPartTypeImage ||
		prepared[0].URL != "file://fixtures/draft.png" || prepared[0].File != "" {
		t.Fatalf("prepared content = %#v, want canonical image URL", prepared)
	}
	prepared[0].Metadata["source"] = "changed"
	if original[0].Type != "IMAGE" || original[0].File != "fixtures/draft.png" ||
		original[0].Metadata["source"] != "customer" {
		t.Fatalf("content preparation mutated caller values: %#v", original)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = NewContentPreparation().PrepareWorkContent(ctx, original)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled content preparation error = %v, want context.Canceled", err)
	}
}

func TestPrepareWorkRequestOwnsCanonicalAliasAndSubmissionConflicts(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		message string
	}{
		{
			name:    "retired work type",
			raw:     `{"work_type_id":"legacy"}`,
			message: "work_type_id is not supported; use workTypeName",
		},
		{
			name:    "retired nested target state",
			raw:     `{"works":[{"target_state":"queued"}]}`,
			message: "works[0].target_state is not supported; use state",
		},
		{
			name:    "conflicting trace aliases",
			raw:     `{"currentChainingTraceId":"chain-a","traceId":"chain-b"}`,
			message: "currentChainingTraceId and traceId must match",
		},
		{
			name:    "items and content",
			raw:     `{"items":[],"content":[]}`,
			message: "items cannot be combined with content",
		},
		{
			name:    "items and payload",
			raw:     `{"items":[],"payload":"text"}`,
			message: "items cannot be combined with payload",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := mustRequestPreparationService(t).PrepareWorkRequest(
				context.Background(),
				WorkRequestPreparation{CanonicalJSON: []byte(test.raw)},
			)
			assertRequestPreparationErrorContains(t, err, test.message)
		})
	}
}

func TestPrepareWorkRequestOwnsContentURLMediaAndMeaningfulnessPolicy(t *testing.T) {
	tests := []struct {
		name    string
		content []WorkContentPart
		message string
	}{
		{
			name: "unsupported URL",
			content: []WorkContentPart{{
				Type: WorkContentPartTypeImage, URL: "ftp://example.com/ui.png",
			}},
			message: "url scheme must be one of file, http, https, or data",
		},
		{
			name: "URL and legacy file conflict",
			content: []WorkContentPart{{
				Type: WorkContentPartTypeImage,
				URL:  "file://fixtures/ui.png",
				File: "fixtures/ui.png",
			}},
			message: "url and file cannot both be set",
		},
		{
			name: "image media mismatch",
			content: []WorkContentPart{{
				Type: WorkContentPartTypeImage, URL: "file://fixtures/ui.png",
				ContentType: "audio/wav",
			}},
			message: "contentType must start with image/",
		},
		{
			name: "audio media mismatch",
			content: []WorkContentPart{{
				Type: WorkContentPartTypeAudio, URL: "file://fixtures/audio.wav",
				ContentType: "image/png",
			}},
			message: "contentType must start with audio/",
		},
		{
			name: "blank only content",
			content: []WorkContentPart{{
				Type: WorkContentPartTypeText, Text: " \t ",
			}},
			message: "content must contain at least one non-empty part",
		},
		{
			name: "invalid JSON content",
			content: []WorkContentPart{{
				Type: WorkContentPartTypeJSON, JSON: json.RawMessage(`{`),
			}},
			message: "json must contain valid JSON",
		},
		{
			name: "unsupported content type",
			content: []WorkContentPart{{
				Type: "VIDEO", URL: "file://fixtures/video.mp4",
			}},
			message: "type must be one of",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := mustRequestPreparationService(t).PrepareWorkRequest(
				context.Background(),
				WorkRequestPreparation{Request: WorkRequest{
					RequestID: "request-content",
					Type:      WorkRequestTypeFactoryRequestBatch,
					Works: []Work{{
						Name: "draft", WorkTypeID: "task", Content: test.content,
					}},
				}},
			)
			assertRequestPreparationErrorContains(t, err, test.message)
		})
	}
}

func TestPrepareWorkRequestRejectsInvalidIdentityAndContext(t *testing.T) {
	if service, err := NewRequestPreparationService(nil); err == nil || service != nil {
		t.Fatalf("NewRequestPreparationService(nil) = (%T, %v), want required-dependency error", service, err)
	}
	service := mustRequestPreparationService(t)
	_, err := service.PrepareWorkRequest(context.Background(), WorkRequestPreparation{
		Request: WorkRequest{Works: []Work{{WorkTypeID: "task"}}},
	})
	assertRequestPreparationErrorContains(t, err, "name is required")

	_, err = service.PrepareWorkRequest(context.Background(), WorkRequestPreparation{
		Request: WorkRequest{Works: []Work{{Name: "draft"}}},
	})
	assertRequestPreparationErrorContains(t, err, "workTypeName is required")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = service.PrepareWorkRequest(ctx, WorkRequestPreparation{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled preparation error = %v, want context.Canceled", err)
	}
}

func assertRequestPreparationErrorContains(t *testing.T, err error, message string) {
	t.Helper()
	var validation *RequestPreparationError
	if !errors.As(err, &validation) || !strings.Contains(validation.Message, message) {
		t.Fatalf("preparation error = %v, want typed error containing %q", err, message)
	}
}

func mustRequestPreparationService(t *testing.T) RequestPreparationService {
	t.Helper()
	service, err := NewRequestPreparationService(NewContentPreparation())
	if err != nil {
		t.Fatalf("NewRequestPreparationService: %v", err)
	}
	return service
}
