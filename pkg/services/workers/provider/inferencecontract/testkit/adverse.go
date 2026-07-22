package testkit

import (
	"context"
	"errors"
	"fmt"
	"testing"

	contract "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
)

// FailureIntegrationFactory constructs an integration that completes with the
// requested normalized failure and no competing successful response.
type FailureIntegrationFactory func(contract.Identity, contract.FailureKind) contract.Integration

// AdverseSuite supplies deterministic integrations for interruption, sink
// failure, and malformed terminal behavior. Each factory must return a fresh
// integration so scenarios can run independently and under the race detector.
type AdverseSuite struct {
	Identities          []contract.Identity
	Failures            FailureIntegrationFactory
	Cancellation        IntegrationFactory
	Timeout             IntegrationFactory
	Backpressure        IntegrationFactory
	DoubleClose         IntegrationFactory
	WriteAfter          IntegrationFactory
	MissingClose        IntegrationFactory
	Disagreement        IntegrationFactory
	FailureAfterSuccess IntegrationFactory
	Request             contract.InvocationRequest
}

var failureKinds = []contract.FailureKind{
	contract.FailureAuthentication,
	contract.FailureInvalidRequest,
	contract.FailureThrottled,
	contract.FailureTimeout,
	contract.FailureCanceled,
	contract.FailureDependency,
	contract.FailureMalformedOutput,
	contract.FailureUnknown,
}

// RunAdverse verifies normalized failures, cancellation, deadlines, response
// sink backpressure, and deterministic terminal-state contract violations.
func RunAdverse(t *testing.T, suite AdverseSuite) {
	t.Helper()
	requireAdverseSuite(t, suite)
	for _, identity := range suite.Identities {
		identity := identity
		t.Run(string(identity), func(t *testing.T) {
			runFailureScenarios(t, identity, suite)
			runInterruptedScenario(t, "cancellation", identity, suite.Cancellation, suite.Request, canceledContext, contract.FailureCanceled)
			runInterruptedScenario(t, "timeout", identity, suite.Timeout, suite.Request, expiredContext, contract.FailureTimeout)
			runBackpressureScenario(t, identity, suite)
			runViolationScenario(t, "double-close", identity, suite.DoubleClose, suite.Request, "duplicate_close", outcomeSuccess)
			runViolationScenario(t, "write-after-close", identity, suite.WriteAfter, suite.Request, "write_after_close", outcomeSuccess)
			runViolationScenario(t, "missing-close", identity, suite.MissingClose, suite.Request, "missing_close", outcomeMalformedFailure)
			runViolationScenario(t, "result-event-disagreement", identity, suite.Disagreement, suite.Request, "final_result_agreement", outcomeNone)
			runViolationScenario(t, "failure-after-success-event", identity, suite.FailureAfterSuccess, suite.Request, "final_result_agreement", outcomeNone)
		})
	}
}

func runFailureScenarios(t *testing.T, identity contract.Identity, suite AdverseSuite) {
	t.Helper()
	for _, kind := range failureKinds {
		kind := kind
		t.Run("failure-"+string(kind), func(t *testing.T) {
			destination := &recordingWriter{}
			if err := contract.ExecuteInvocation(context.Background(), suite.Failures(identity, kind), suite.Request, destination); err != nil {
				t.Fatalf("ExecuteInvocation() error = %v", err)
			}
			assertFailureCompletion(t, destination, kind)
		})
	}
}

type contextFactory func() (context.Context, context.CancelFunc)

func canceledContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx, func() {}
}

func expiredContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 0)
}

func runInterruptedScenario(t *testing.T, name string, identity contract.Identity, factory IntegrationFactory, request contract.InvocationRequest, newContext contextFactory, want contract.FailureKind) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		ctx, cancel := newContext()
		defer cancel()
		destination := &recordingWriter{}
		err := contract.ExecuteInvocation(ctx, factory(identity), request, destination)
		if err == nil {
			t.Fatal("ExecuteInvocation() error = nil, want interruption")
		}
		assertFailureCompletion(t, destination, want)
		if len(destination.events) != 0 {
			t.Fatalf("events after interruption = %#v, want none", drafts(destination.events))
		}
	})
}

var errBackpressure = errors.New("testkit response sink backpressure")

type backpressureWriter struct {
	recordingWriter
	writes  int
	entered chan struct{}
	release chan struct{}
}

func (w *backpressureWriter) WriteEvent(context.Context, contract.EventDraft) error {
	w.writes++
	close(w.entered)
	<-w.release
	return errBackpressure
}

func runBackpressureScenario(t *testing.T, identity contract.Identity, suite AdverseSuite) {
	t.Helper()
	t.Run("backpressure", func(t *testing.T) {
		destination := &backpressureWriter{entered: make(chan struct{}), release: make(chan struct{})}
		done := make(chan error, 1)
		go func() {
			done <- contract.ExecuteInvocation(context.Background(), suite.Backpressure(identity), suite.Request, destination)
		}()
		<-destination.entered
		select {
		case err := <-done:
			t.Fatalf("ExecuteInvocation() returned before response sink released backpressure: %v", err)
		default:
		}
		close(destination.release)
		err := <-done
		if !errors.Is(err, errBackpressure) {
			t.Fatalf("ExecuteInvocation() error = %v, want propagated backpressure error", err)
		}
		if destination.writes != 1 || destination.closes != 0 {
			t.Fatalf("destination calls = %d writes, %d closes; want one failed write and no close", destination.writes, destination.closes)
		}
	})
}

type expectedOutcome int

const (
	outcomeNone expectedOutcome = iota
	outcomeSuccess
	outcomeMalformedFailure
)

func runViolationScenario(t *testing.T, name string, identity contract.Identity, factory IntegrationFactory, request contract.InvocationRequest, rule string, outcome expectedOutcome) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		destination := &recordingWriter{}
		err := contract.ExecuteInvocation(context.Background(), factory(identity), request, destination)
		var violation *contract.ProtocolError
		if !errors.As(err, &violation) || violation.Rule != rule {
			t.Fatalf("ExecuteInvocation() error = %v, want protocol rule %q", err, rule)
		}
		switch outcome {
		case outcomeNone:
			if len(destination.events) != 0 || destination.closes != 0 {
				t.Fatalf("destination received %d events and %d closes, want no contradictory terminal outcome", len(destination.events), destination.closes)
			}
		case outcomeSuccess:
			assertSuccessfulClose(t, destination)
		case outcomeMalformedFailure:
			assertFailureCompletion(t, destination, contract.FailureMalformedOutput)
		default:
			t.Fatalf("unsupported expected outcome %d", outcome)
		}
	})
}

func assertSuccessfulClose(t *testing.T, destination *recordingWriter) {
	t.Helper()
	if destination.closes != 1 || destination.completion == nil || destination.completion.Response() == nil || destination.completion.Failure() != nil {
		t.Fatalf("completion = %#v after %d closes, want first successful terminal outcome", destination.completion, destination.closes)
	}
}

func assertFailureCompletion(t *testing.T, destination *recordingWriter, want contract.FailureKind) {
	t.Helper()
	if destination.closes != 1 || destination.completion == nil || destination.completion.Response() != nil {
		t.Fatalf("completion = %#v after %d closes, want one failure", destination.completion, destination.closes)
	}
	failure := destination.completion.Failure()
	if failure == nil || failure.Kind() != want {
		t.Fatalf("failure = %#v, want kind %q", failure, want)
	}
	if err := contract.ValidateFailure(*failure); err != nil {
		t.Fatalf("failure is not safe and valid: %v", err)
	}
}

func requireAdverseSuite(t *testing.T, suite AdverseSuite) {
	t.Helper()
	if err := validateAdverseSuite(suite); err != nil {
		t.Fatalf("invalid adverse provider conformance suite: %v", err)
	}
}

func validateAdverseSuite(suite AdverseSuite) error {
	if suite.Failures == nil {
		return fmt.Errorf("all adverse integration factories are required")
	}
	factories := []IntegrationFactory{
		suite.Cancellation,
		suite.Timeout,
		suite.Backpressure,
		suite.DoubleClose,
		suite.WriteAfter,
		suite.MissingClose,
		suite.Disagreement,
		suite.FailureAfterSuccess,
	}
	for _, factory := range factories {
		if factory == nil {
			return fmt.Errorf("all adverse integration factories are required")
		}
	}
	if len(suite.Identities) == 0 {
		return fmt.Errorf("at least one opaque provider identity is required")
	}
	for _, identity := range suite.Identities {
		if err := contract.ValidateIdentity(identity); err != nil {
			return err
		}
	}
	if suite.Request.InvocationID() == "" || suite.Request.Model() == "" || suite.Request.UserMessage() == "" {
		return fmt.Errorf("request requires invocation ID, model, and user message")
	}
	return nil
}
