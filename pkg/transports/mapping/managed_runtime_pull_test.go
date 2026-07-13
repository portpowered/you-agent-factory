package apisurface

import (
	"context"
	"errors"
	"net/http"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestManagedRuntimePullHTTPStatus_MapsClassifiedOutcomes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		outcome string
		want    int
	}{
		{outcome: "TIMED_OUT", want: http.StatusGatewayTimeout},
		{outcome: "SOURCE_FETCH_FAILED", want: http.StatusUnprocessableEntity},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.outcome, func(t *testing.T) {
			t.Parallel()
			got := ManagedRuntimePullHTTPStatus(ModelPullResult{ManagedPullOutcome: tc.outcome})
			if got != tc.want {
				t.Fatalf("status = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestModelPullResponseFromService_ProjectsManagedRuntimePullFailure(t *testing.T) {
	t.Parallel()
	response := ModelPullResponseFromService(ModelPullResult{
		ModelName:          "OMNIVOICE_Q4_K_M",
		ProviderLocality:   "LOCAL",
		ManagedPullOutcome: "SOURCE_FETCH_FAILED",
		ReadinessState:     "FAILED",
		LifecycleState:     "NOT_INSTALLED",
	})
	if response.ManagedRuntimePull.PullOutcome != factoryapi.ManagedRuntimePullOutcomeSOURCEFETCHFAILED {
		t.Fatalf("pull outcome = %s, want SOURCE_FETCH_FAILED", response.ManagedRuntimePull.PullOutcome)
	}
	if response.ManagedRuntimePull.ReadinessState != factoryapi.ManagedRuntimeReadinessStateFAILED {
		t.Fatalf("readiness = %s, want FAILED", response.ManagedRuntimePull.ReadinessState)
	}
}

func TestManagedRuntimePullError_UnwrapsCause(t *testing.T) {
	t.Parallel()
	cause := errors.Join(context.DeadlineExceeded, ErrManagedRuntimeSourceFetchFailed)
	err := &ManagedRuntimePullError{
		Result: ModelPullResult{
			ModelName:          "OMNIVOICE_Q4_K_M",
			ManagedPullOutcome: "TIMED_OUT",
			ReadinessState:     "FAILED",
		},
		Cause: cause,
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want deadline exceeded cause", err)
	}
	if !IsManagedRuntimePullError(err) {
		t.Fatalf("IsManagedRuntimePullError = false, want true")
	}
}
