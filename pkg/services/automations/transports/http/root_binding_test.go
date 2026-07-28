package http

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/automations"
)

func TestAdapter_BindsAutomationsRootViaFakeRootSeam(t *testing.T) {
	t.Parallel()

	var invoked bool
	fake := &rootFake{
		sourceStatus: func(
			_ context.Context,
			request automations.SourceStatusRequest,
		) (automations.SourceStatusResult, error) {
			invoked = true
			if request.Identity.AutomationID != "automation-1" || request.Identity.SourceID != "source-1" {
				t.Fatalf("SourceStatusRequest identity = %#v, want automation-1/source-1", request.Identity)
			}
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
	if adapter.Root().Operations != fake {
		t.Fatal("adapter must expose the injected Automations root")
	}

	result, err := adapter.invokeSourceStatus(context.Background(), automations.SourceStatusRequest{
		Identity: automations.SourceIdentity{
			AutomationID: "automation-1",
			SourceID:     "source-1",
		},
	})
	if !invoked {
		t.Fatal("adapter-owned operation did not invoke the injected Automations root")
	}
	if err != nil {
		t.Fatalf("invokeSourceStatus error = %v", err)
	}
	if result.Observation.InstanceID != "instance-1" {
		t.Fatalf("SourceStatusResult = %#v, want instance-1 observation", result)
	}
}

func TestNewAdapter_RejectsNilRootOperations(t *testing.T) {
	t.Parallel()

	if NewAdapter(automations.Root{}) != nil {
		t.Fatal("NewAdapter with nil Operations must return nil")
	}
	if NewAdapterFromRoot(RootBinding{}) != nil {
		t.Fatal("NewAdapterFromRoot with nil Operations must return nil")
	}
}

func TestAdapter_PropagatesTypedRootFailures(t *testing.T) {
	t.Parallel()

	fake := &rootFake{
		sourceStatus: func(
			context.Context,
			automations.SourceStatusRequest,
		) (automations.SourceStatusResult, error) {
			return automations.SourceStatusResult{}, &automations.Error{
				Op:   "SourceStatus",
				Code: automations.ErrorCodeNotFound,
				Err:  automations.ErrNotFound,
			}
		},
	}
	adapter := NewAdapterFromRoot(RootBinding{Automations: automations.Root{Operations: fake}})

	_, err := adapter.invokeSourceStatus(context.Background(), automations.SourceStatusRequest{
		Identity: automations.SourceIdentity{AutomationID: "missing", SourceID: "source"},
	})
	if !errors.Is(err, automations.ErrNotFound) {
		t.Fatalf("invokeSourceStatus error = %v, want ErrNotFound", err)
	}
}

func TestAdapter_RequiresInjectedRoot(t *testing.T) {
	t.Parallel()

	var adapter *Adapter

	_, err := adapter.invokeSourceStatus(context.Background(), automations.SourceStatusRequest{})
	if err == nil {
		t.Fatal("invokeSourceStatus on nil adapter = nil, want error")
	}
}
