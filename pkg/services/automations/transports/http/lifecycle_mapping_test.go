package http

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/automations"
)

func TestStartSourceRequestFromHTTP_MapsRootRequest(t *testing.T) {
	t.Parallel()

	request, err := StartSourceRequestFromHTTP(StartSourceInput{
		AutomationID: "automation-1",
		SourceID:     "source-1",
		Kind:         "schedule",
	})
	if err != nil {
		t.Fatalf("StartSourceRequestFromHTTP: %v", err)
	}
	if request.Identity.AutomationID != "automation-1" ||
		request.Identity.SourceID != "source-1" ||
		request.Kind != "schedule" ||
		request.Resume != nil {
		t.Fatalf("request = %#v, want start-source root request", request)
	}
}

func TestStartSourceRequestFromHTTP_MapsResumeObservation(t *testing.T) {
	t.Parallel()

	request, err := StartSourceRequestFromHTTP(StartSourceInput{
		AutomationID: "automation-1",
		SourceID:     "source-1",
		Kind:         "watcher",
		Resume: &SourceObservationResponse{
			AutomationID: "automation-1",
			SourceID:     "source-1",
			InstanceID:   "instance-1",
			State:        string(automations.ObservedLifecycleRunning),
			Cursor:       "cursor-1",
		},
	})
	if err != nil {
		t.Fatalf("StartSourceRequestFromHTTP: %v", err)
	}
	if request.Resume == nil ||
		request.Resume.InstanceID != "instance-1" ||
		request.Resume.State != automations.ObservedLifecycleRunning ||
		request.Resume.Cursor != "cursor-1" {
		t.Fatalf("request = %#v, want resume observation", request)
	}
}

func TestStartSourceRequestFromHTTP_RejectsMalformedInputsBeforeRoot(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input StartSourceInput
	}{
		{
			name: "missing automation id",
			input: StartSourceInput{
				SourceID: "source-1",
				Kind:     "schedule",
			},
		},
		{
			name: "missing source id",
			input: StartSourceInput{
				AutomationID: "automation-1",
				Kind:         "schedule",
			},
		},
		{
			name: "missing kind",
			input: StartSourceInput{
				AutomationID: "automation-1",
				SourceID:     "source-1",
			},
		},
		{
			name: "resume identity mismatch",
			input: StartSourceInput{
				AutomationID: "automation-1",
				SourceID:     "source-1",
				Kind:         "schedule",
				Resume: &SourceObservationResponse{
					AutomationID: "other",
					SourceID:     "source-1",
					InstanceID:   "instance-1",
					State:        string(automations.ObservedLifecycleRunning),
				},
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := StartSourceRequestFromHTTP(test.input)
			if err == nil || !IsLifecycleBadRequest(err) {
				t.Fatalf("StartSourceRequestFromHTTP = %v, want typed bad request", err)
			}
		})
	}
}

func TestStopSourceRequestFromHTTP_MapsRootRequest(t *testing.T) {
	t.Parallel()

	request, err := StopSourceRequestFromHTTP(StopSourceInput{
		AutomationID: "automation-1",
		SourceID:     "source-1",
	})
	if err != nil {
		t.Fatalf("StopSourceRequestFromHTTP: %v", err)
	}
	if request.Identity.AutomationID != "automation-1" || request.Identity.SourceID != "source-1" {
		t.Fatalf("request = %#v, want stop-source root request", request)
	}
}

func TestWaitSourceRequestFromHTTP_MapsDesiredLifecycle(t *testing.T) {
	t.Parallel()

	request, err := WaitSourceRequestFromHTTP(WaitSourceInput{
		AutomationID: "automation-1",
		SourceID:     "source-1",
		Desired:      "running",
	})
	if err != nil {
		t.Fatalf("WaitSourceRequestFromHTTP: %v", err)
	}
	if request.Desired != automations.DesiredLifecycleRunning {
		t.Fatalf("request = %#v, want desired running", request)
	}
}

func TestWaitSourceRequestFromHTTP_RejectsInvalidDesiredBeforeRoot(t *testing.T) {
	t.Parallel()

	_, err := WaitSourceRequestFromHTTP(WaitSourceInput{
		AutomationID: "automation-1",
		SourceID:     "source-1",
		Desired:      "paused",
	})
	if err == nil || !IsLifecycleBadRequest(err) {
		t.Fatalf("WaitSourceRequestFromHTTP = %v, want typed bad request", err)
	}
}

func TestSourceStatusRequestFromHTTP_MapsRootRequest(t *testing.T) {
	t.Parallel()

	request, err := SourceStatusRequestFromHTTP(SourceStatusInput{
		AutomationID: "automation-1",
		SourceID:     "source-1",
	})
	if err != nil {
		t.Fatalf("SourceStatusRequestFromHTTP: %v", err)
	}
	if request.Identity.AutomationID != "automation-1" || request.Identity.SourceID != "source-1" {
		t.Fatalf("request = %#v, want source-status root request", request)
	}
}

func TestLifecycleResponsesToHTTP_EncodeFakeRootSuccess(t *testing.T) {
	t.Parallel()

	observation := automations.SourceObservation{
		Identity: automations.SourceIdentity{
			AutomationID: "automation-1",
			SourceID:     "source-1",
		},
		InstanceID: "instance-1",
		State:      automations.ObservedLifecycleRunning,
		Cursor:     "cursor-1",
	}
	outcome := automations.LifecycleOutcome{
		Desired:     automations.DesiredLifecycleRunning,
		Observation: observation,
		Convergence: automations.ConvergenceStatusConverged,
		Idempotent:  true,
	}

	started := StartSourceResponseToHTTP(automations.StartSourceResult{Outcome: outcome})
	if started.Outcome.Desired != "running" ||
		started.Outcome.Observation.InstanceID != "instance-1" ||
		!started.Outcome.Idempotent {
		t.Fatalf("start response = %#v, want encoded lifecycle outcome", started)
	}

	stopped := StopSourceResponseToHTTP(automations.StopSourceResult{Outcome: outcome})
	if stopped.Outcome.Observation.State != string(automations.ObservedLifecycleRunning) {
		t.Fatalf("stop response = %#v, want encoded observation", stopped)
	}

	waited := WaitSourceResponseToHTTP(automations.WaitSourceResult{Outcome: outcome})
	if waited.Outcome.Convergence != string(automations.ConvergenceStatusConverged) {
		t.Fatalf("wait response = %#v, want encoded convergence", waited)
	}

	status := SourceStatusResponseToHTTP(automations.SourceStatusResult{Observation: observation})
	if status.Observation.Cursor != "cursor-1" {
		t.Fatalf("status response = %#v, want encoded observation", status)
	}
}
