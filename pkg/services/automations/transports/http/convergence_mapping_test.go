package http

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/automations"
)

func TestReconcileRequestFromHTTP_MapsRootRequest(t *testing.T) {
	t.Parallel()

	request, err := ReconcileRequestFromHTTP(ReconcileInput{
		Desired: []DesiredSpecInput{
			{
				AutomationID: "automation-1",
				SourceID:     "source-1",
				Kind:         "schedule",
				State:        "running",
			},
		},
		Observed: []ObservedInstanceInput{
			{
				AutomationID: "automation-1",
				SourceID:     "source-1",
				InstanceID:   "instance-1",
				State:        string(automations.ObservedLifecycleRunning),
			},
		},
	})
	if err != nil {
		t.Fatalf("ReconcileRequestFromHTTP: %v", err)
	}
	if len(request.Desired) != 1 || len(request.Observed) != 1 {
		t.Fatalf("request = %#v, want one desired and one observed entry", request)
	}
	if request.Desired[0].State != automations.DesiredLifecycleRunning {
		t.Fatalf("desired state = %q, want running", request.Desired[0].State)
	}
	if request.Observed[0].InstanceID != "instance-1" {
		t.Fatalf("observed = %#v, want instance-1", request.Observed[0])
	}
}

func TestReconcileRequestFromHTTP_RejectsMalformedInputsBeforeRoot(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input ReconcileInput
	}{
		{
			name: "missing desired automation id",
			input: ReconcileInput{
				Desired: []DesiredSpecInput{{
					SourceID: "source-1",
					Kind:     "schedule",
					State:    "running",
				}},
			},
		},
		{
			name: "invalid desired state",
			input: ReconcileInput{
				Desired: []DesiredSpecInput{{
					AutomationID: "automation-1",
					SourceID:     "source-1",
					Kind:         "schedule",
					State:        "paused",
				}},
			},
		},
		{
			name: "missing observed instance id",
			input: ReconcileInput{
				Observed: []ObservedInstanceInput{{
					AutomationID: "automation-1",
					SourceID:     "source-1",
					State:        string(automations.ObservedLifecycleRunning),
				}},
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := ReconcileRequestFromHTTP(test.input)
			if err == nil || !IsConvergenceBadRequest(err) {
				t.Fatalf("ReconcileRequestFromHTTP = %v, want typed bad request", err)
			}
		})
	}
}

func TestGetStatusRequestFromHTTP_MapsRootRequest(t *testing.T) {
	t.Parallel()

	request, err := GetStatusRequestFromHTTP(GetStatusInput{InstanceID: "instance-1"})
	if err != nil {
		t.Fatalf("GetStatusRequestFromHTTP: %v", err)
	}
	if request.InstanceID != "instance-1" {
		t.Fatalf("request = %#v, want instance-1", request)
	}
}

func TestGetStatusRequestFromHTTP_RejectsMissingInstanceBeforeRoot(t *testing.T) {
	t.Parallel()

	_, err := GetStatusRequestFromHTTP(GetStatusInput{})
	if err == nil || !IsConvergenceBadRequest(err) {
		t.Fatalf("GetStatusRequestFromHTTP = %v, want typed bad request", err)
	}
}

func TestGetCursorRequestFromHTTP_MapsRootRequest(t *testing.T) {
	t.Parallel()

	request, err := GetCursorRequestFromHTTP(GetCursorInput{
		InstanceID:     "instance-1",
		ExpectedCursor: "cursor-1",
	})
	if err != nil {
		t.Fatalf("GetCursorRequestFromHTTP: %v", err)
	}
	if request.InstanceID != "instance-1" || request.ExpectedCursor != "cursor-1" {
		t.Fatalf("request = %#v, want instance-1 with expected cursor", request)
	}
}

func TestGetCursorRequestFromHTTP_RejectsMissingInstanceBeforeRoot(t *testing.T) {
	t.Parallel()

	_, err := GetCursorRequestFromHTTP(GetCursorInput{ExpectedCursor: "cursor-1"})
	if err == nil || !IsConvergenceBadRequest(err) {
		t.Fatalf("GetCursorRequestFromHTTP = %v, want typed bad request", err)
	}
}

func TestConvergenceResponsesToHTTP_EncodeFakeRootSuccess(t *testing.T) {
	t.Parallel()

	reconciled := ReconcileResponseToHTTP(automations.ReconcileResult{
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
		GeneratedWorkRequests: []automations.GeneratedWorkRequestOutcome{
			{
				Request: automations.GeneratedWorkRequest{
					Identity: automations.GeneratedWorkRequestIdentity{
						AutomationID: "automation-1",
						SourceID:     "source-1",
						RequestID:    "request-1",
					},
					Payload: []byte("payload"),
				},
				Status: automations.WorkRequestAdmissionAccepted,
			},
		},
	})
	if len(reconciled.Outcomes) != 1 || reconciled.Outcomes[0].Action != "created" {
		t.Fatalf("reconcile response = %#v, want encoded outcome", reconciled)
	}
	if len(reconciled.GeneratedWorkRequests) != 1 ||
		reconciled.GeneratedWorkRequests[0].Status != "accepted" {
		t.Fatalf("generated work requests = %#v, want encoded admission", reconciled.GeneratedWorkRequests)
	}

	status := GetStatusResponseToHTTP(automations.GetStatusResult{
		AutomationID: "automation-1",
		InstanceID:   "instance-1",
		Status:       automations.ObservedLifecycleRunning,
	})
	if status.Status != string(automations.ObservedLifecycleRunning) {
		t.Fatalf("status response = %#v, want encoded status", status)
	}

	cursor := GetCursorResponseToHTTP(automations.GetCursorResult{
		AutomationID: "automation-1",
		InstanceID:   "instance-1",
		Cursor:       "cursor-1",
		Checkpoint:   "checkpoint-1",
	})
	if cursor.Cursor != "cursor-1" || cursor.Checkpoint != "checkpoint-1" {
		t.Fatalf("cursor response = %#v, want encoded cursor facts", cursor)
	}
}
