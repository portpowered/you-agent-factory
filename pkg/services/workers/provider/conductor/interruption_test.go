package conductor_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/workers/provider/conductor"
	inference "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
)

func TestConductorCanceledContextYieldsCanceledTerminal(t *testing.T) {
	t.Parallel()

	providers, recording := newLimitedCapabilityRegistry(t)
	recording.invoke = func(ctx context.Context, _ inference.InvocationRequest, _ inference.ResponseWriter) error {
		<-ctx.Done()
		return ctx.Err()
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	destination := &terminalDestination{}
	err := conductor.New(providers).Invoke(ctx, "conductor.fixture", acceptedRequest("inv-canceled"), destination)
	if err == nil {
		t.Fatal("Invoke() error = nil, want cancellation")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Invoke() error = %v, want context.Canceled", err)
	}
	assertExactlyOneTerminal(t, destination)
	assertInterruptionFailure(t, destination.completion.Failure(), inference.FailureCanceled, conductor.InvariantCanceled, false)
}

func TestConductorDeadlineExpiryYieldsTimeoutTerminal(t *testing.T) {
	t.Parallel()

	providers, recording := newLimitedCapabilityRegistry(t)
	recording.invoke = func(ctx context.Context, _ inference.InvocationRequest, _ inference.ResponseWriter) error {
		<-ctx.Done()
		return ctx.Err()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()

	destination := &terminalDestination{}
	err := conductor.New(providers).Invoke(ctx, "conductor.fixture", acceptedRequest("inv-timeout"), destination)
	if err == nil {
		t.Fatal("Invoke() error = nil, want timeout")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Invoke() error = %v, want context.DeadlineExceeded", err)
	}
	assertExactlyOneTerminal(t, destination)
	assertInterruptionFailure(t, destination.completion.Failure(), inference.FailureTimeout, conductor.InvariantTimeout, true)
}

func TestConductorInterruptionDiagnosticsRemainCustomerSafe(t *testing.T) {
	t.Parallel()

	const secret = "sk-conductor-interrupt-secret"
	providers, recording := newLimitedCapabilityRegistry(t)
	recording.invoke = func(ctx context.Context, _ inference.InvocationRequest, writer inference.ResponseWriter) error {
		return writer.Close(ctx, inference.FailedCompletion(inference.NewFailure(inference.FailureInput{
			Kind:    inference.FailureTimeout,
			Message: "timed out with token=" + secret,
			Diagnostics: map[string]string{
				"detail": "Authorization: Bearer " + secret,
			},
		})))
	}

	destination := &terminalDestination{}
	if err := conductor.New(providers).Invoke(
		context.Background(),
		"conductor.fixture",
		acceptedRequest("inv-unsafe-timeout"),
		destination,
	); err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	assertExactlyOneTerminal(t, destination)
	failure := destination.completion.Failure()
	assertInterruptionFailure(t, failure, inference.FailureTimeout, conductor.InvariantTimeout, true)
	if strings.Contains(failure.Message(), secret) {
		t.Fatalf("failure message leaked secret: %q", failure.Message())
	}
	for key, value := range failure.Diagnostics() {
		if strings.Contains(key, secret) || strings.Contains(value, secret) ||
			strings.Contains(strings.ToLower(value), "bearer") ||
			strings.Contains(strings.ToLower(value), "authorization") {
			t.Fatalf("failure diagnostics leaked unsafe detail: %s=%q", key, value)
		}
	}
}

func TestConductorRetryHandoffExposesNeutralRetryability(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		kind      inference.FailureKind
		retryable bool
		want      bool
	}{
		{name: "canceled", kind: inference.FailureCanceled, retryable: false, want: false},
		{name: "timeout", kind: inference.FailureTimeout, retryable: true, want: true},
		{name: "throttled", kind: inference.FailureThrottled, retryable: true, want: true},
		{name: "auth", kind: inference.FailureAuthentication, retryable: false, want: false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			providers, recording := newLimitedCapabilityRegistry(t)
			recording.invoke = func(ctx context.Context, _ inference.InvocationRequest, writer inference.ResponseWriter) error {
				return writer.Close(ctx, inference.FailedCompletion(inference.NewFailure(inference.FailureInput{
					Kind:      test.kind,
					Message:   "safe normalized failure",
					Retryable: test.retryable,
				})))
			}
			destination := &terminalDestination{}
			if err := conductor.New(providers).Invoke(
				context.Background(),
				"conductor.fixture",
				acceptedRequest("inv-retry-"+test.name),
				destination,
			); err != nil {
				t.Fatalf("Invoke() error = %v", err)
			}
			assertExactlyOneTerminal(t, destination)
			failure := destination.completion.Failure()
			if failure == nil {
				t.Fatal("failure = nil, want normalized failure")
			}
			handoff := conductor.RetryHandoffFromFailure(*failure)
			if handoff.Retryable != test.want {
				t.Fatalf("RetryHandoff.Retryable = %t, want %t", handoff.Retryable, test.want)
			}
		})
	}
}

func TestConductorCanceledInvocationsRemainExclusiveUnderConcurrentLoad(t *testing.T) {
	t.Parallel()

	const workers = 8
	var wait sync.WaitGroup
	errs := make([]error, workers)
	destinations := make([]*terminalDestination, workers)
	for index := range workers {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			providers, recording := newLimitedCapabilityRegistry(t)
			recording.invoke = func(ctx context.Context, _ inference.InvocationRequest, _ inference.ResponseWriter) error {
				<-ctx.Done()
				return ctx.Err()
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			destination := &terminalDestination{}
			destinations[index] = destination
			errs[index] = conductor.New(providers).Invoke(
				ctx,
				"conductor.fixture",
				acceptedRequest("inv-race-cancel"),
				destination,
			)
		}(index)
	}
	wait.Wait()

	for index := range workers {
		if !errors.Is(errs[index], context.Canceled) {
			t.Fatalf("worker %d error = %v, want context.Canceled", index, errs[index])
		}
		assertExactlyOneTerminal(t, destinations[index])
		assertInterruptionFailure(
			t,
			destinations[index].completion.Failure(),
			inference.FailureCanceled,
			conductor.InvariantCanceled,
			false,
		)
	}
}

func assertInterruptionFailure(
	t *testing.T,
	failure *inference.Failure,
	wantKind inference.FailureKind,
	wantInvariant string,
	wantRetryable bool,
) {
	t.Helper()
	assertSafeFailure(t, failure, wantKind)
	if failure.Retryable() != wantRetryable {
		t.Fatalf("Retryable() = %t, want %t", failure.Retryable(), wantRetryable)
	}
	diagnostics := failure.Diagnostics()
	if got := diagnostics["invariant"]; got != wantInvariant {
		t.Fatalf("diagnostics[invariant] = %q, want %q", got, wantInvariant)
	}
	handoff := conductor.RetryHandoffFromFailure(*failure)
	if handoff.Retryable != wantRetryable {
		t.Fatalf("RetryHandoff.Retryable = %t, want %t", handoff.Retryable, wantRetryable)
	}
}
