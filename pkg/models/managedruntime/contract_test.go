package managedruntime_test

import (
	"errors"
	"fmt"
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

type readinessError struct {
	state managedruntime.ReadinessState
}

func (e readinessError) Error() string { return "runtime readiness" }

func (e readinessError) ManagedRuntimeReadinessState() managedruntime.ReadinessState {
	return e.state
}
