package service

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/work/internal/services/admission"
)

func TestNewReturnsAdmissionService(t *testing.T) {
	t.Parallel()

	svc := New()
	if svc == nil {
		t.Fatal("New() returned nil")
	}
	var _ admission.Service = svc

	ctx := context.Background()
	request := work.WorkRequest{
		RequestID: "shell-request-1",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{{
			Name:       "shell-work",
			WorkTypeID: "story",
		}},
	}
	opts := work.WorkRequestNormalizeOptions{
		ValidWorkTypes: map[string]bool{"story": true},
	}

	normalized, err := svc.Normalize(ctx, admission.NormalizeRequest{Request: request, Options: opts})
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if normalized.RequestID != "shell-request-1" || len(normalized.Normalized) != 1 {
		t.Fatalf("Normalize result = %#v, want shell-request-1 with one work", normalized)
	}

	if err := svc.Validate(ctx, admission.ValidateRequest{Request: request, Options: opts}); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	accepted, err := svc.Accept(ctx, admission.AcceptRequest{RequestID: request.RequestID})
	if err == nil {
		t.Fatal("Accept shell should not claim full admission accept behavior yet")
	}
	if accepted.Accepted || accepted.RequestID != "" {
		t.Fatalf("Accept shell result = %#v, want empty outcome on stub failure", accepted)
	}
}
