package wire

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/work/internal/requestadmission"
)

type selectedRequest struct{}

func (selectedRequest) PrepareWorkRequest(context.Context, requestadmission.WorkRequestPreparation) (requestadmission.Request, error) {
	return requestadmission.Request{RequestID: "selected"}, nil
}

type selectedPrivateContent struct{}

func (selectedPrivateContent) PrepareWorkContent(context.Context, []requestadmission.ContentPart) ([]requestadmission.ContentPart, error) {
	return []requestadmission.ContentPart{{Type: requestadmission.ContentPartTypeText, Text: "selected"}}, nil
}

type selectedPublicContent struct{ failure error }

func (p selectedPublicContent) PrepareWorkContent(context.Context, []work.WorkContentPart) ([]work.WorkContentPart, error) {
	return []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "public"}}, p.failure
}
func TestRequestAdapterUsesCompletedPolicy(t *testing.T) {
	t.Parallel()
	got, err := NewRequestPreparationService(selectedRequest{}).PrepareWorkRequest(t.Context(), work.WorkRequestPreparation{})
	if err != nil || got.RequestID != "selected" {
		t.Fatalf("prepared=%#v,error=%v", got, err)
	}
}
func TestContentAdapterUsesCompletedPolicy(t *testing.T) {
	t.Parallel()
	got, err := NewContentPreparation(selectedPrivateContent{}).PrepareWorkContent(t.Context(), nil)
	if err != nil || len(got) != 1 || got[0].Text != "selected" {
		t.Fatalf("content=%#v,error=%v", got, err)
	}
}
func TestContentBridgePreservesPublicRoleResultAndError(t *testing.T) {
	t.Parallel()
	for _, failure := range []error{nil, context.Canceled} {
		got, err := NewRequestContentBridge(selectedPublicContent{failure: failure}).PrepareWorkContent(t.Context(), nil)
		if !errors.Is(err, failure) {
			t.Fatalf("bridge error=%v", err)
		}
		if failure == nil && (len(got) != 1 || got[0].Text != "public") {
			t.Fatalf("bridge content=%#v", got)
		}
	}
}
