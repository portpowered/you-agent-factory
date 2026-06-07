package apisurface

import (
	"errors"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

func TestInvocationErrorFromManagedRuntime_ReadyAllowsInvocation(t *testing.T) {
	err := InvocationErrorFromManagedRuntime(factoryapi.ManagedRuntime{
		Identity:       "OMNIVOICE_Q4_K_M",
		ReadinessState: factoryapi.ManagedRuntimeReadinessStateREADY,
		LifecycleState: factoryapi.ManagedRuntimeLifecycleStateINSTALLED,
	})
	if err != nil {
		t.Fatalf("error = %v, want nil for READY", err)
	}
}

func TestInvocationErrorFromManagedRuntime_MissingUsesManagedVocabulary(t *testing.T) {
	err := InvocationErrorFromManagedRuntime(factoryapi.ManagedRuntime{
		Identity:       "OMNIVOICE_Q4_K_M",
		ReadinessState: factoryapi.ManagedRuntimeReadinessStateMISSING,
		LifecycleState: factoryapi.ManagedRuntimeLifecycleStateNOTINSTALLED,
	})
	if err == nil {
		t.Fatal("error = nil, want managed runtime missing")
	}
	if !errors.Is(err, ErrManagedRuntimeMissing) {
		t.Fatalf("error = %v, want ErrManagedRuntimeMissing", err)
	}
	var readinessErr *ManagedRuntimeInvocationError
	if !errors.As(err, &readinessErr) {
		t.Fatalf("error = %T, want *ManagedRuntimeInvocationError", err)
	}
	if readinessErr.ReadinessState != factoryapi.ManagedRuntimeReadinessStateMISSING ||
		readinessErr.LifecycleState != factoryapi.ManagedRuntimeLifecycleStateNOTINSTALLED {
		t.Fatalf("readiness = (%s, %s), want MISSING NOT_INSTALLED", readinessErr.ReadinessState, readinessErr.LifecycleState)
	}
}

func TestInvocationErrorFromManagedRuntime_LoadingAndFailed(t *testing.T) {
	loadingErr := InvocationErrorFromManagedRuntime(factoryapi.ManagedRuntime{
		Identity:       "OMNIVOICE_Q4_K_M",
		ReadinessState: factoryapi.ManagedRuntimeReadinessStateLOADING,
		LifecycleState: factoryapi.ManagedRuntimeLifecycleStateLOADING,
	})
	if !errors.Is(loadingErr, ErrManagedRuntimeLoading) {
		t.Fatalf("loading error = %v, want ErrManagedRuntimeLoading", loadingErr)
	}

	failedErr := InvocationErrorFromManagedRuntime(factoryapi.ManagedRuntime{
		Identity:       "OMNIVOICE_Q4_K_M",
		ReadinessState: factoryapi.ManagedRuntimeReadinessStateFAILED,
		LifecycleState: factoryapi.ManagedRuntimeLifecycleStateNOTINSTALLED,
	})
	if !errors.Is(failedErr, ErrManagedRuntimeFailed) {
		t.Fatalf("failed error = %v, want ErrManagedRuntimeFailed", failedErr)
	}
}
