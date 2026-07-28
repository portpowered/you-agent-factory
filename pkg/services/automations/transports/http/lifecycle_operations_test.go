package http

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/automations"
)

func TestAdapter_StartSourceInvokesFakeRootAndEncodesSuccess(t *testing.T) {
	t.Parallel()

	var invoked bool
	fake := &rootFake{
		startSource: func(
			_ context.Context,
			request automations.StartSourceRequest,
		) (automations.StartSourceResult, error) {
			invoked = true
			if request.Identity.AutomationID != "automation-1" ||
				request.Identity.SourceID != "source-1" ||
				request.Kind != "schedule" {
				t.Fatalf("StartSourceRequest = %#v, want automation-1/source-1 schedule", request)
			}
			return automations.StartSourceResult{
				Outcome: automations.LifecycleOutcome{
					Desired: automations.DesiredLifecycleRunning,
					Observation: automations.SourceObservation{
						Identity:   request.Identity,
						InstanceID: "instance-1",
						State:      automations.ObservedLifecycleRunning,
					},
					Convergence: automations.ConvergenceStatusConverged,
				},
			}, nil
		},
	}
	adapter := NewAdapterFromRoot(RootBinding{Automations: automations.Root{Operations: fake}})

	response, err := adapter.StartSource(context.Background(), StartSourceInput{
		AutomationID: "automation-1",
		SourceID:     "source-1",
		Kind:         "schedule",
	})
	if !invoked {
		t.Fatal("StartSource did not invoke the injected Automations root")
	}
	if err != nil {
		t.Fatalf("StartSource error = %v", err)
	}
	if response.Outcome.Observation.InstanceID != "instance-1" ||
		response.Outcome.Desired != string(automations.DesiredLifecycleRunning) {
		t.Fatalf("response = %#v, want encoded start outcome", response)
	}
}

func TestAdapter_StartSourceRejectsInvalidInputBeforeFakeRoot(t *testing.T) {
	t.Parallel()

	fake := &rootFake{
		startSource: func(context.Context, automations.StartSourceRequest) (automations.StartSourceResult, error) {
			t.Fatal("fake root must not be invoked for invalid start input")
			return automations.StartSourceResult{}, nil
		},
	}
	adapter := NewAdapterFromRoot(RootBinding{Automations: automations.Root{Operations: fake}})

	_, err := adapter.StartSource(context.Background(), StartSourceInput{
		AutomationID: "automation-1",
		SourceID:     "source-1",
	})
	if err == nil || !IsLifecycleBadRequest(err) {
		t.Fatalf("StartSource error = %v, want typed bad request", err)
	}
}

func TestAdapter_StopSourceInvokesFakeRootAndEncodesSuccess(t *testing.T) {
	t.Parallel()

	var invoked bool
	fake := &rootFake{
		stopSource: func(
			_ context.Context,
			request automations.StopSourceRequest,
		) (automations.StopSourceResult, error) {
			invoked = true
			return automations.StopSourceResult{
				Outcome: automations.LifecycleOutcome{
					Desired: automations.DesiredLifecycleStopped,
					Observation: automations.SourceObservation{
						Identity:   request.Identity,
						InstanceID: "instance-1",
						State:      automations.ObservedLifecycleStopped,
					},
					Convergence: automations.ConvergenceStatusConverged,
					Idempotent:  true,
				},
			}, nil
		},
	}
	adapter := NewAdapterFromRoot(RootBinding{Automations: automations.Root{Operations: fake}})

	response, err := adapter.StopSource(context.Background(), StopSourceInput{
		AutomationID: "automation-1",
		SourceID:     "source-1",
	})
	if !invoked {
		t.Fatal("StopSource did not invoke the injected Automations root")
	}
	if err != nil {
		t.Fatalf("StopSource error = %v", err)
	}
	if !response.Outcome.Idempotent || response.Outcome.Desired != "stopped" {
		t.Fatalf("response = %#v, want encoded stop outcome", response)
	}
}

func TestAdapter_WaitSourceInvokesFakeRootAndEncodesSuccess(t *testing.T) {
	t.Parallel()

	var invoked bool
	fake := &rootFake{
		waitSource: func(
			_ context.Context,
			request automations.WaitSourceRequest,
		) (automations.WaitSourceResult, error) {
			invoked = true
			if request.Desired != automations.DesiredLifecycleRunning {
				t.Fatalf("WaitSourceRequest desired = %q, want running", request.Desired)
			}
			return automations.WaitSourceResult{
				Outcome: automations.LifecycleOutcome{
					Desired: automations.DesiredLifecycleRunning,
					Observation: automations.SourceObservation{
						Identity:   request.Identity,
						InstanceID: "instance-1",
						State:      automations.ObservedLifecycleRunning,
					},
					Convergence: automations.ConvergenceStatusConverged,
				},
			}, nil
		},
	}
	adapter := NewAdapterFromRoot(RootBinding{Automations: automations.Root{Operations: fake}})

	response, err := adapter.WaitSource(context.Background(), WaitSourceInput{
		AutomationID: "automation-1",
		SourceID:     "source-1",
		Desired:      "running",
	})
	if !invoked {
		t.Fatal("WaitSource did not invoke the injected Automations root")
	}
	if err != nil {
		t.Fatalf("WaitSource error = %v", err)
	}
	if response.Outcome.Observation.State != string(automations.ObservedLifecycleRunning) {
		t.Fatalf("response = %#v, want encoded wait outcome", response)
	}
}

func TestAdapter_SourceStatusInvokesFakeRootAndEncodesSuccess(t *testing.T) {
	t.Parallel()

	var invoked bool
	fake := &rootFake{
		sourceStatus: func(
			_ context.Context,
			request automations.SourceStatusRequest,
		) (automations.SourceStatusResult, error) {
			invoked = true
			return automations.SourceStatusResult{
				Observation: automations.SourceObservation{
					Identity:   request.Identity,
					InstanceID: "instance-1",
					State:      automations.ObservedLifecycleRunning,
				},
			}, nil
		},
	}
	adapter := NewAdapterFromRoot(RootBinding{Automations: automations.Root{Operations: fake}})

	response, err := adapter.SourceStatus(context.Background(), SourceStatusInput{
		AutomationID: "automation-1",
		SourceID:     "source-1",
	})
	if !invoked {
		t.Fatal("SourceStatus did not invoke the injected Automations root")
	}
	if err != nil {
		t.Fatalf("SourceStatus error = %v", err)
	}
	if response.Observation.InstanceID != "instance-1" {
		t.Fatalf("response = %#v, want encoded source status", response)
	}
}

func TestAdapter_WaitSourceRejectsInvalidDesiredBeforeFakeRoot(t *testing.T) {
	t.Parallel()

	fake := &rootFake{
		waitSource: func(context.Context, automations.WaitSourceRequest) (automations.WaitSourceResult, error) {
			t.Fatal("fake root must not be invoked for invalid wait desired state")
			return automations.WaitSourceResult{}, nil
		},
	}
	adapter := NewAdapterFromRoot(RootBinding{Automations: automations.Root{Operations: fake}})

	_, err := adapter.WaitSource(context.Background(), WaitSourceInput{
		AutomationID: "automation-1",
		SourceID:     "source-1",
		Desired:      "unknown",
	})
	if err == nil || !IsLifecycleBadRequest(err) {
		t.Fatalf("WaitSource error = %v, want typed bad request", err)
	}
}
