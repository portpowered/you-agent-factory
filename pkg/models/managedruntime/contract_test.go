package managedruntime_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	managedruntime "github.com/portpowered/infinite-you/pkg/models/managedruntime"
)

func TestReadinessAndInvocationErrorsRemainDistinct(t *testing.T) {
	t.Parallel()

	cases := []struct {
		state managedruntime.ReadinessState
		err   error
	}{
		{state: managedruntime.ReadinessStateMissing, err: managedruntime.ErrMissing},
		{state: managedruntime.ReadinessStateLoading, err: managedruntime.ErrLoading},
		{state: managedruntime.ReadinessStateFailed, err: managedruntime.ErrFailed},
		{state: managedruntime.ReadinessStateUnsupported, err: managedruntime.ErrUnsupported},
	}
	for index, tc := range cases {
		if tc.state == "" {
			t.Fatalf("case %d readiness state is empty", index)
		}
		for otherIndex, other := range cases {
			if index != otherIndex && errors.Is(tc.err, other.err) {
				t.Fatalf("case %d error aliases case %d", index, otherIndex)
			}
		}
		state, ok := managedruntime.ReadinessStateFromError(fmt.Errorf("wrapped: %w", tc.err))
		if !ok || state != tc.state {
			t.Fatalf("case %d readiness = %q, %t; want %q, true", index, state, ok, tc.state)
		}
	}
	if state, ok := managedruntime.ReadinessStateFromError(errors.New("unrelated")); ok || state != "" {
		t.Fatalf("unrelated readiness = %q, %t; want empty, false", state, ok)
	}
}

func TestReadinessStateFromErrorPrefersReadinessSeam(t *testing.T) {
	t.Parallel()

	err := readinessError{state: managedruntime.ReadinessStateFailed}
	state, ok := managedruntime.ReadinessStateFromError(err)
	if !ok || state != managedruntime.ReadinessStateFailed {
		t.Fatalf("readiness = %q, %t; want %q, true", state, ok, managedruntime.ReadinessStateFailed)
	}
}

func TestErrNotFoundHasStableIdentity(t *testing.T) {
	t.Parallel()

	if managedruntime.ErrNotFound == nil || managedruntime.ErrNotFound.Error() != "model not found" {
		t.Fatalf("ErrNotFound = %v, want stable model-not-found identity", managedruntime.ErrNotFound)
	}
}

func TestInvocationErrorForRuntimePreservesReadinessAndRecoveryGuidance(t *testing.T) {
	t.Parallel()

	err := managedruntime.InvocationErrorForRuntime(managedruntime.Runtime{
		Identity:       "OMNIVOICE_Q4_K_M",
		ReadinessState: managedruntime.ReadinessStateMissing,
		LifecycleState: managedruntime.LifecycleStateNotInstalled,
	})
	if !errors.Is(err, managedruntime.ErrMissing) {
		t.Fatalf("error = %v, want ErrMissing", err)
	}
	var readinessErr *managedruntime.InvocationError
	if !errors.As(err, &readinessErr) {
		t.Fatalf("error = %T, want *InvocationError", err)
	}
	if readinessErr.Identity != "OMNIVOICE_Q4_K_M" ||
		readinessErr.ManagedRuntimeReadinessState() != managedruntime.ReadinessStateMissing ||
		!strings.Contains(readinessErr.Error(), "pull or install") {
		t.Fatalf("readiness error = %#v (%q), want identity, MISSING, and recovery guidance", readinessErr, readinessErr.Error())
	}
	if err := managedruntime.InvocationErrorForRuntime(managedruntime.Runtime{
		ReadinessState: managedruntime.ReadinessStateReady,
	}); err != nil {
		t.Fatalf("ready error = %v, want nil", err)
	}
}

type readinessError struct {
	state managedruntime.ReadinessState
}

func (e readinessError) Error() string { return "runtime readiness" }

func (e readinessError) ManagedRuntimeReadinessState() managedruntime.ReadinessState {
	return e.state
}
