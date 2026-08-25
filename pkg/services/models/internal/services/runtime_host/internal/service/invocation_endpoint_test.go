package service

import (
	"context"
	"errors"
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
)

func TestInvocationEndpointExposesOnlyReadyRuntimeEndpoints(t *testing.T) {
	t.Parallel()

	scope, err := (models.RuntimeScopeRef{}).Parse("scope:endpoint-test")
	if err != nil {
		t.Fatalf("scope.Parse: %v", err)
	}
	host := &service{runtimeSlots: map[string]*supervisedRuntime{
		runtimeSlotKey(scope, "llm"): {
			state:    supervisedStateReady,
			endpoint: "  grpc://127.0.0.1:45731  ",
		},
	}}

	endpoint, err := host.InvocationEndpoint(context.Background(), scope, "llm")
	if err != nil {
		t.Fatalf("ready InvocationEndpoint: %v", err)
	}
	if endpoint != "grpc://127.0.0.1:45731" {
		t.Fatalf("ready endpoint = %q, want trimmed endpoint", endpoint)
	}

	if _, err := host.InvocationEndpoint(context.Background(), scope, "missing"); !errors.Is(err, models.ErrHostRuntimeNotReady) {
		t.Fatalf("missing endpoint error = %v, want ErrHostRuntimeNotReady", err)
	}

	for _, state := range []supervisedState{supervisedStateAbsent, supervisedStateLoading, supervisedStateFailed} {
		host.runtimeSlots[runtimeSlotKey(scope, "llm")] = &supervisedRuntime{
			state: state, endpoint: "grpc://127.0.0.1:45731",
		}
		if _, err := host.InvocationEndpoint(context.Background(), scope, "llm"); !errors.Is(err, models.ErrHostRuntimeNotReady) {
			t.Fatalf("state %q endpoint error = %v, want ErrHostRuntimeNotReady", state, err)
		}
	}

	var nilHost *service
	if _, err := nilHost.InvocationEndpoint(context.Background(), scope, "llm"); !errors.Is(err, models.ErrUnavailable) {
		t.Fatalf("nil host endpoint error = %v, want ErrUnavailable", err)
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := host.InvocationEndpoint(cancelled, scope, "llm"); !errors.Is(err, models.ErrHostCancelled) {
		t.Fatalf("cancelled endpoint error = %v, want ErrHostCancelled", err)
	}
}
