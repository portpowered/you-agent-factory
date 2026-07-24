package service

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/work/internal/services/admission"
)

func TestNewReturnsAdmissionServiceShell(t *testing.T) {
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

	normalized, err := svc.Normalize(ctx, admission.NormalizeRequest{Request: request})
	if err == nil {
		t.Fatal("Normalize shell should not claim full admission behavior yet")
	}
	if normalized.RequestID != "" || len(normalized.Normalized) != 0 {
		t.Fatalf("Normalize shell result = %#v, want empty outcome on stub failure", normalized)
	}

	if err := svc.Validate(ctx, admission.ValidateRequest{Request: request}); err == nil {
		t.Fatal("Validate shell should not claim full admission behavior yet")
	}

	accepted, err := svc.Accept(ctx, admission.AcceptRequest{RequestID: request.RequestID})
	if err == nil {
		t.Fatal("Accept shell should not claim full admission behavior yet")
	}
	if accepted.Accepted || accepted.RequestID != "" {
		t.Fatalf("Accept shell result = %#v, want empty outcome on stub failure", accepted)
	}
}
