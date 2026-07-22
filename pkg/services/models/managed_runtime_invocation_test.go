package models

import (
	"errors"
	"testing"
)

func TestInvocationErrorFromManagedRuntime_ReadyAllowsInvocation(t *testing.T) {
	t.Parallel()

	err := (Runtime{
		Identity:       "OMNIVOICE_Q4_K_M",
		ReadinessState: ReadinessStateReady,
		LifecycleState: LifecycleStateInstalled,
	}).InvocationError()
	if err != nil {
		t.Fatalf("error = %v, want nil for READY", err)
	}
}

func TestInvocationErrorFromManagedRuntime_MissingUsesManagedVocabulary(t *testing.T) {
	t.Parallel()

	err := (Runtime{
		Identity:       "OMNIVOICE_Q4_K_M",
		ReadinessState: ReadinessStateMissing,
		LifecycleState: LifecycleStateNotInstalled,
	}).InvocationError()
	if err == nil {
		t.Fatal("error = nil, want managed runtime missing")
	}
	if !errors.Is(err, ErrMissing) {
		t.Fatalf("error = %v, want ErrMissing", err)
	}
	var readinessErr *InvocationError
	if !errors.As(err, &readinessErr) {
		t.Fatalf("error = %T, want *InvocationError", err)
	}
	if readinessErr.ReadinessState != ReadinessStateMissing ||
		readinessErr.LifecycleState != LifecycleStateNotInstalled {
		t.Fatalf("readiness = (%s, %s), want MISSING NOT_INSTALLED", readinessErr.ReadinessState, readinessErr.LifecycleState)
	}
}

func TestInvocationErrorFromManagedRuntime_LoadingAndFailed(t *testing.T) {
	t.Parallel()

	loadingErr := (Runtime{
		Identity:       "OMNIVOICE_Q4_K_M",
		ReadinessState: ReadinessStateLoading,
		LifecycleState: LifecycleStateLoading,
	}).InvocationError()
	if !errors.Is(loadingErr, ErrLoading) {
		t.Fatalf("loading error = %v, want ErrLoading", loadingErr)
	}

	failedErr := (Runtime{
		Identity:       "OMNIVOICE_Q4_K_M",
		ReadinessState: ReadinessStateFailed,
		LifecycleState: LifecycleStateNotInstalled,
	}).InvocationError()
	if !errors.Is(failedErr, ErrFailed) {
		t.Fatalf("failed error = %v, want ErrFailed", failedErr)
	}
}
