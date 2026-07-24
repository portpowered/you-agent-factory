package service

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/work/internal/services/admission"
)

func TestAcceptFirstTimeReturnsDetachedAcceptedResult(t *testing.T) {
	t.Parallel()

	svc := New()
	ctx := context.Background()
	normalized := []work.SubmitRequest{{
		RequestID:              "request-accept-1",
		WorkID:                 "work-accept-1",
		Name:                   "story-1",
		WorkTypeID:             "story",
		CurrentChainingTraceID: "chain-accept-1",
		TraceID:                "chain-accept-1",
		Payload:                []byte(`{"title":"accept"}`),
		Tags:                   map[string]string{},
		Relations:              []work.Relation{},
	}}

	got, err := svc.Accept(ctx, admission.AcceptRequest{
		RequestID:  "request-accept-1",
		Normalized: normalized,
	})
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}

	want := work.SubmitResultFromNormalized("request-accept-1", normalized)
	if got.RequestID != want.RequestID ||
		!got.Accepted ||
		got.WorkID != want.WorkID ||
		got.Name != want.Name ||
		got.WorkTypeName != want.WorkTypeName ||
		got.TraceID != want.TraceID ||
		len(got.Works) != 1 ||
		got.Works[0].WorkID != want.Works[0].WorkID {
		t.Fatalf("Accept result = %#v, want %#v", got, want)
	}
}

func TestAcceptDuplicateReturnsTypedConflictFailure(t *testing.T) {
	t.Parallel()

	svc := New()
	ctx := context.Background()
	request := admission.AcceptRequest{
		RequestID: "request-dup-1",
		Normalized: []work.SubmitRequest{{
			RequestID:  "request-dup-1",
			WorkID:     "work-dup-1",
			Name:       "story-dup",
			WorkTypeID: "story",
			TraceID:    "chain-dup-1",
			Payload:    []byte(`{"title":"dup"}`),
			Tags:       map[string]string{},
			Relations:  []work.Relation{},
		}},
	}

	if _, err := svc.Accept(ctx, request); err != nil {
		t.Fatalf("first Accept: %v", err)
	}

	_, err := svc.Accept(ctx, request)
	if !errors.Is(err, work.ErrWorkRequestConflict) {
		t.Fatalf("duplicate Accept error = %v, want ErrWorkRequestConflict", err)
	}
}

func TestAcceptIncompatibleReplayReturnsTypedConflictFailure(t *testing.T) {
	t.Parallel()

	svc := New()
	ctx := context.Background()
	first := admission.AcceptRequest{
		RequestID: "request-conflict-1",
		Normalized: []work.SubmitRequest{{
			RequestID:  "request-conflict-1",
			WorkID:     "work-conflict-a",
			Name:       "story-a",
			WorkTypeID: "story",
			TraceID:    "chain-conflict-1",
			Payload:    []byte(`{"title":"a"}`),
			Tags:       map[string]string{},
			Relations:  []work.Relation{},
		}},
	}
	if _, err := svc.Accept(ctx, first); err != nil {
		t.Fatalf("first Accept: %v", err)
	}

	_, err := svc.Accept(ctx, admission.AcceptRequest{
		RequestID: "request-conflict-1",
		Normalized: []work.SubmitRequest{{
			RequestID:  "request-conflict-1",
			WorkID:     "work-conflict-b",
			Name:       "story-b",
			WorkTypeID: "story",
			TraceID:    "chain-conflict-1",
			Payload:    []byte(`{"title":"b"}`),
			Tags:       map[string]string{},
			Relations:  []work.Relation{},
		}},
	})
	if !errors.Is(err, work.ErrWorkRequestConflict) {
		t.Fatalf("conflict Accept error = %v, want ErrWorkRequestConflict", err)
	}
}

func TestAcceptRejectionShapedReturnsTypedRejectionFailure(t *testing.T) {
	t.Parallel()

	svc := New()
	ctx := context.Background()

	cases := []struct {
		name    string
		request admission.AcceptRequest
	}{
		{
			name:    "empty request id",
			request: admission.AcceptRequest{Normalized: []work.SubmitRequest{{Name: "story", WorkTypeID: "story"}}},
		},
		{
			name:    "empty normalized batch",
			request: admission.AcceptRequest{RequestID: "request-rejected-1"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := svc.Accept(ctx, tc.request)
			if !errors.Is(err, work.ErrWorkRequestRejected) && !errors.Is(err, work.ErrInvalidWorkRequest) {
				t.Fatalf("Accept error = %v, want ErrWorkRequestRejected or ErrInvalidWorkRequest", err)
			}
		})
	}
}
