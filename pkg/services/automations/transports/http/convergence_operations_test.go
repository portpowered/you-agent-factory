package http

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/automations"
)

func TestAdapter_ReconcileInvokesFakeRootAndEncodesSuccess(t *testing.T) {
	t.Parallel()

	var invoked bool
	fake := &rootFake{
		reconcile: func(
			_ context.Context,
			request automations.ReconcileRequest,
		) (automations.ReconcileResult, error) {
			invoked = true
			if len(request.Desired) != 1 ||
				request.Desired[0].AutomationID != "automation-1" ||
				request.Desired[0].SourceID != "source-1" {
				t.Fatalf("ReconcileRequest = %#v, want automation-1/source-1 desired spec", request)
			}
			return automations.ReconcileResult{
				Outcomes: []automations.ConvergenceOutcome{
					{
						AutomationID: "automation-1",
						SourceID:     "source-1",
						InstanceID:   "instance-1",
						Action:       automations.ConvergenceActionCreated,
						Desired:      automations.DesiredLifecycleRunning,
						Observed:     automations.ObservedLifecycleRunning,
						Convergence:  automations.ConvergenceStatusConverged,
					},
				},
			}, nil
		},
	}
	adapter := NewAdapterFromRoot(RootBinding{Automations: automations.Root{Operations: fake}})

	response, err := adapter.Reconcile(context.Background(), ReconcileInput{
		Desired: []DesiredSpecInput{
			{
				AutomationID: "automation-1",
				SourceID:     "source-1",
				Kind:         "schedule",
				State:        "running",
			},
		},
	})
	if !invoked {
		t.Fatal("Reconcile did not invoke the injected Automations root")
	}
	if err != nil {
		t.Fatalf("Reconcile error = %v", err)
	}
	if len(response.Outcomes) != 1 || response.Outcomes[0].Convergence != "converged" {
		t.Fatalf("response = %#v, want encoded reconcile outcome", response)
	}
}

func TestAdapter_ReconcileRejectsInvalidInputBeforeFakeRoot(t *testing.T) {
	t.Parallel()

	fake := &rootFake{
		reconcile: func(context.Context, automations.ReconcileRequest) (automations.ReconcileResult, error) {
			t.Fatal("fake root must not be invoked for invalid reconcile input")
			return automations.ReconcileResult{}, nil
		},
	}
	adapter := NewAdapterFromRoot(RootBinding{Automations: automations.Root{Operations: fake}})

	_, err := adapter.Reconcile(context.Background(), ReconcileInput{
		Desired: []DesiredSpecInput{{
			AutomationID: "automation-1",
			SourceID:     "source-1",
			Kind:         "schedule",
			State:        "unknown",
		}},
	})
	if err == nil || !IsConvergenceBadRequest(err) {
		t.Fatalf("Reconcile error = %v, want typed bad request", err)
	}
}

func TestAdapter_GetStatusInvokesFakeRootAndEncodesSuccess(t *testing.T) {
	t.Parallel()

	var invoked bool
	fake := &rootFake{
		getStatus: func(
			_ context.Context,
			request automations.GetStatusRequest,
		) (automations.GetStatusResult, error) {
			invoked = true
			if request.InstanceID != "instance-1" {
				t.Fatalf("GetStatusRequest = %#v, want instance-1", request)
			}
			return automations.GetStatusResult{
				AutomationID: "automation-1",
				InstanceID:   request.InstanceID,
				Status:       automations.ObservedLifecycleRunning,
			}, nil
		},
	}
	adapter := NewAdapterFromRoot(RootBinding{Automations: automations.Root{Operations: fake}})

	response, err := adapter.GetStatus(context.Background(), GetStatusInput{InstanceID: "instance-1"})
	if !invoked {
		t.Fatal("GetStatus did not invoke the injected Automations root")
	}
	if err != nil {
		t.Fatalf("GetStatus error = %v", err)
	}
	if response.Status != string(automations.ObservedLifecycleRunning) {
		t.Fatalf("response = %#v, want encoded instance status", response)
	}
}

func TestAdapter_GetStatusRejectsMissingInstanceBeforeFakeRoot(t *testing.T) {
	t.Parallel()

	fake := &rootFake{
		getStatus: func(context.Context, automations.GetStatusRequest) (automations.GetStatusResult, error) {
			t.Fatal("fake root must not be invoked for invalid status input")
			return automations.GetStatusResult{}, nil
		},
	}
	adapter := NewAdapterFromRoot(RootBinding{Automations: automations.Root{Operations: fake}})

	_, err := adapter.GetStatus(context.Background(), GetStatusInput{})
	if err == nil || !IsConvergenceBadRequest(err) {
		t.Fatalf("GetStatus error = %v, want typed bad request", err)
	}
}

func TestAdapter_GetCursorInvokesFakeRootAndEncodesSuccess(t *testing.T) {
	t.Parallel()

	var invoked bool
	fake := &rootFake{
		getCursor: func(
			_ context.Context,
			request automations.GetCursorRequest,
		) (automations.GetCursorResult, error) {
			invoked = true
			if request.InstanceID != "instance-1" || request.ExpectedCursor != "cursor-expected" {
				t.Fatalf("GetCursorRequest = %#v, want instance-1 with expected cursor", request)
			}
			return automations.GetCursorResult{
				AutomationID: "automation-1",
				InstanceID:   request.InstanceID,
				Cursor:       "cursor-1",
				Checkpoint:   "checkpoint-1",
			}, nil
		},
	}
	adapter := NewAdapterFromRoot(RootBinding{Automations: automations.Root{Operations: fake}})

	response, err := adapter.GetCursor(context.Background(), GetCursorInput{
		InstanceID:     "instance-1",
		ExpectedCursor: "cursor-expected",
	})
	if !invoked {
		t.Fatal("GetCursor did not invoke the injected Automations root")
	}
	if err != nil {
		t.Fatalf("GetCursor error = %v", err)
	}
	if response.Cursor != "cursor-1" || response.Checkpoint != "checkpoint-1" {
		t.Fatalf("response = %#v, want encoded cursor facts", response)
	}
}

func TestAdapter_GetCursorRejectsMissingInstanceBeforeFakeRoot(t *testing.T) {
	t.Parallel()

	fake := &rootFake{
		getCursor: func(context.Context, automations.GetCursorRequest) (automations.GetCursorResult, error) {
			t.Fatal("fake root must not be invoked for invalid cursor input")
			return automations.GetCursorResult{}, nil
		},
	}
	adapter := NewAdapterFromRoot(RootBinding{Automations: automations.Root{Operations: fake}})

	_, err := adapter.GetCursor(context.Background(), GetCursorInput{ExpectedCursor: "cursor-1"})
	if err == nil || !IsConvergenceBadRequest(err) {
		t.Fatalf("GetCursor error = %v, want typed bad request", err)
	}
}
