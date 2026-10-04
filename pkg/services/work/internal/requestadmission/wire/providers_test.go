package wire

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work/internal/requestadmission"
)

type selectedContent struct{}

func (selectedContent) PrepareWorkContent(context.Context, []requestadmission.ContentPart) ([]requestadmission.ContentPart, error) {
	return []requestadmission.ContentPart{{Type: requestadmission.ContentPartTypeText, Text: "selected"}}, nil
}
func TestRequestPolicyUsesSelectedContent(t *testing.T) {
	t.Parallel()
	policy := NewRequestPolicy(selectedContent{})
	got, err := policy.PrepareWorkRequest(t.Context(), requestadmission.WorkRequestPreparation{Request: requestadmission.Request{RequestID: "request", Works: []requestadmission.Work{{Name: "draft", WorkTypeID: "task"}}}})
	if err != nil || got.Works[0].Content[0].Text != "selected" {
		t.Fatalf("prepared=%#v,error=%v", got, err)
	}
}
func TestContentPolicyRejectsCanceledPreparation(t *testing.T) {
	t.Parallel()
	policy := NewContentPolicy()
	got, err := policy.PrepareWorkContent(t.Context(), []requestadmission.ContentPart{{Type: requestadmission.ContentPartTypeText, Text: "selected"}})
	if err != nil || len(got) != 1 || got[0].Text != "selected" {
		t.Fatalf("content=%#v,error=%v", got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := policy.PrepareWorkContent(ctx, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled content=%v", err)
	}
}
