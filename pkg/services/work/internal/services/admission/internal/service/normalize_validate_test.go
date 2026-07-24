package service

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/work/internal/services/admission"
)

func TestNormalizeValidWorkRequestMatchesRootNormalizeShape(t *testing.T) {
	t.Parallel()

	svc := New()
	ctx := context.Background()
	request := work.WorkRequest{
		RequestID:              "request-1",
		CurrentChainingTraceID: "chain-request-1",
		Type:                   work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{
			{Name: "first", WorkTypeID: "task", Payload: map[string]any{"title": "first"}},
			{Name: "second", WorkTypeID: "task", Payload: map[string]any{"title": "second"}},
		},
	}
	opts := work.WorkRequestNormalizeOptions{
		ValidWorkTypes: map[string]bool{"task": true},
	}

	wantNormalized, err := work.NormalizeWorkRequest(request, opts)
	if err != nil {
		t.Fatalf("root NormalizeWorkRequest: %v", err)
	}

	got, err := svc.Normalize(ctx, admission.NormalizeRequest{Request: request, Options: opts})
	if err != nil {
		t.Fatalf("admission Normalize: %v", err)
	}
	if got.RequestID != "request-1" {
		t.Fatalf("RequestID = %q, want request-1", got.RequestID)
	}
	if len(got.Normalized) != len(wantNormalized) {
		t.Fatalf("normalized count = %d, want %d", len(got.Normalized), len(wantNormalized))
	}
	for i := range wantNormalized {
		if got.Normalized[i].RequestID != wantNormalized[i].RequestID ||
			got.Normalized[i].WorkID != wantNormalized[i].WorkID ||
			got.Normalized[i].Name != wantNormalized[i].Name ||
			got.Normalized[i].WorkTypeID != wantNormalized[i].WorkTypeID ||
			got.Normalized[i].CurrentChainingTraceID != wantNormalized[i].CurrentChainingTraceID {
			t.Fatalf("normalized[%d] = %#v, want %#v", i, got.Normalized[i], wantNormalized[i])
		}
	}

	if err := svc.Validate(ctx, admission.ValidateRequest{Request: request, Options: opts}); err != nil {
		t.Fatalf("admission Validate: %v", err)
	}
}

func TestNormalizeAndValidateReturnTypedRejectionFailures(t *testing.T) {
	t.Parallel()

	svc := New()
	ctx := context.Background()
	opts := work.WorkRequestNormalizeOptions{
		ValidWorkTypes: map[string]bool{"task": true},
	}

	cases := []struct {
		name    string
		request work.WorkRequest
	}{
		{
			name: "empty works",
			request: work.WorkRequest{
				RequestID: "request-empty",
				Type:      work.WorkRequestTypeFactoryRequestBatch,
			},
		},
		{
			name: "unsupported type",
			request: work.WorkRequest{
				RequestID: "request-type",
				Type:      work.WorkRequestType("UNSUPPORTED"),
				Works:     []work.Work{{Name: "first", WorkTypeID: "task"}},
			},
		},
		{
			name: "unknown relation identity",
			request: work.WorkRequest{
				RequestID: "request-relation",
				Type:      work.WorkRequestTypeFactoryRequestBatch,
				Works:     []work.Work{{Name: "first", WorkTypeID: "task"}},
				Relations: []work.WorkRelation{{
					Type:           work.WorkRelationDependsOn,
					SourceWorkName: "missing",
					TargetWorkName: "first",
				}},
			},
		},
		{
			name: "unknown work type identity",
			request: work.WorkRequest{
				RequestID: "request-work-type",
				Type:      work.WorkRequestTypeFactoryRequestBatch,
				Works:     []work.Work{{Name: "first", WorkTypeID: "missing"}},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := svc.Normalize(ctx, admission.NormalizeRequest{Request: tc.request, Options: opts})
			assertTypedAdmissionRejection(t, err)

			err = svc.Validate(ctx, admission.ValidateRequest{Request: tc.request, Options: opts})
			assertTypedAdmissionRejection(t, err)
		})
	}
}

func assertTypedAdmissionRejection(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected typed admission rejection, got nil")
	}
	if errors.Is(err, work.ErrInvalidWorkRequest) || errors.Is(err, work.ErrWorkRequestRejected) {
		return
	}
	t.Fatalf("error = %v, want errors.Is ErrInvalidWorkRequest or ErrWorkRequestRejected", err)
}
