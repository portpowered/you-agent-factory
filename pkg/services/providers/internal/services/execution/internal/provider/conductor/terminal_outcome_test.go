package conductor_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/provider/conductor"
	inference "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/provider/inferencecontract"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestConductorYieldsExactlyOneSuccessfulTerminal(t *testing.T) {
	t.Parallel()

	providers, recording := newLimitedCapabilityRegistry(t)
	recording.invoke = func(ctx context.Context, request inference.InvocationRequest, writer inference.ResponseWriter) error {
		return writer.Close(ctx, inference.SuccessfulCompletion(inference.NewResponse(inference.ResponseInput{
			Content: "authoritative success",
		})))
	}
	destination := &terminalDestination{}
	if err := conductor.New(providers).Invoke(
		context.Background(),
		"conductor.fixture",
		acceptedRequest("inv-one-success"),
		destination,
	); err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	assertExactlyOneTerminal(t, destination)
	if destination.completion.Response() == nil || destination.completion.Failure() != nil {
		t.Fatalf("completion = %#v, want success without failure", destination.completion)
	}
	if got := destination.completion.Response().Content(); got != "authoritative success" {
		t.Fatalf("response content = %q, want authoritative success", got)
	}
}

func TestConductorYieldsExactlyOneNormalizedFailure(t *testing.T) {
	t.Parallel()

	providers, recording := newLimitedCapabilityRegistry(t)
	recording.invoke = func(ctx context.Context, request inference.InvocationRequest, writer inference.ResponseWriter) error {
		return writer.Close(ctx, inference.FailedCompletion(inference.NewFailure(inference.FailureInput{
			Kind:    inference.FailureDependency,
			Message: "upstream provider unavailable",
		})))
	}
	destination := &terminalDestination{}
	if err := conductor.New(providers).Invoke(
		context.Background(),
		"conductor.fixture",
		acceptedRequest("inv-one-failure"),
		destination,
	); err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	assertExactlyOneTerminal(t, destination)
	assertSafeFailure(t, destination.completion.Failure(), inference.FailureDependency)
}

func TestConductorMissingCloseCollapsesToOneSafeFailure(t *testing.T) {
	t.Parallel()

	providers, recording := newLimitedCapabilityRegistry(t)
	recording.invoke = func(context.Context, inference.InvocationRequest, inference.ResponseWriter) error {
		return nil
	}
	destination := &terminalDestination{}
	err := conductor.New(providers).Invoke(
		context.Background(),
		"conductor.fixture",
		acceptedRequest("inv-missing-close"),
		destination,
	)
	if err == nil {
		t.Fatal("Invoke() error = nil, want missing-close protocol failure")
	}
	assertExactlyOneTerminal(t, destination)
	assertSafeFailure(t, destination.completion.Failure(), inference.FailureMalformedOutput)
}

func TestConductorDuplicateClosePreservesFirstTerminal(t *testing.T) {
	t.Parallel()

	providers, recording := newLimitedCapabilityRegistry(t)
	var secondClose error
	recording.invoke = func(ctx context.Context, request inference.InvocationRequest, writer inference.ResponseWriter) error {
		if err := writer.Close(ctx, inference.SuccessfulCompletion(inference.NewResponse(inference.ResponseInput{
			Content: "first terminal",
		}))); err != nil {
			return err
		}
		secondClose = writer.Close(ctx, inference.FailedCompletion(inference.NewFailure(inference.FailureInput{
			Kind:    inference.FailureUnknown,
			Message: "competing second terminal",
		})))
		return secondClose
	}
	destination := &terminalDestination{}
	err := conductor.New(providers).Invoke(
		context.Background(),
		"conductor.fixture",
		acceptedRequest("inv-duplicate-close"),
		destination,
	)
	if secondClose == nil {
		t.Fatal("second Close() error = nil, want duplicate-close rejection")
	}
	if err == nil {
		t.Fatal("Invoke() error = nil, want duplicate-close rejection")
	}
	assertExactlyOneTerminal(t, destination)
	if destination.completion.Response() == nil || destination.completion.Failure() != nil {
		t.Fatalf("completion = %#v, want preserved first successful terminal", destination.completion)
	}
	if got := destination.completion.Response().Content(); got != "first terminal" {
		t.Fatalf("response content = %q, want first terminal", got)
	}
}

func TestConductorContradictoryTerminalCollapsesToOneSafeOutcome(t *testing.T) {
	t.Parallel()

	providers, recording := newLimitedCapabilityRegistry(t)
	recording.invoke = func(ctx context.Context, request inference.InvocationRequest, writer inference.ResponseWriter) error {
		provider := string(recording.identity)
		if err := writer.WriteEvent(ctx, messageEvent(t, request.InvocationID(), provider, "message-1", "represented success")); err != nil {
			return err
		}
		return writer.Close(ctx, inference.FailedCompletion(inference.NewFailure(inference.FailureInput{
			Kind:    inference.FailureUnknown,
			Message: "contradictory failure after represented success",
		})))
	}
	destination := &terminalDestination{}
	err := conductor.New(providers).Invoke(
		context.Background(),
		"conductor.fixture",
		acceptedRequest("inv-contradictory"),
		destination,
	)
	if err == nil {
		t.Fatal("Invoke() error = nil, want contradictory terminal rejection")
	}
	assertExactlyOneTerminal(t, destination)
	failure := destination.completion.Failure()
	if failure == nil || destination.completion.Response() != nil {
		t.Fatalf("completion = %#v, want one normalized failure without success", destination.completion)
	}
	assertSafeFailure(t, failure, failure.Kind())
}

func TestConductorRejectsSecretLeakingFailureDetail(t *testing.T) {
	t.Parallel()

	const secret = "sk-conductor-fixture-secret"
	providers, recording := newLimitedCapabilityRegistry(t)
	recording.invoke = func(ctx context.Context, request inference.InvocationRequest, writer inference.ResponseWriter) error {
		return writer.Close(ctx, inference.FailedCompletion(inference.NewFailure(inference.FailureInput{
			Kind:    inference.FailureAuthentication,
			Message: "auth failed with token=" + secret,
			Diagnostics: map[string]string{
				"detail": "Authorization: Bearer " + secret,
			},
		})))
	}
	destination := &terminalDestination{}
	if err := conductor.New(providers).Invoke(
		context.Background(),
		"conductor.fixture",
		acceptedRequest("inv-unsafe-failure"),
		destination,
	); err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	assertExactlyOneTerminal(t, destination)
	failure := destination.completion.Failure()
	if failure == nil || destination.completion.Response() != nil {
		t.Fatalf("completion = %#v, want one sanitized failure", destination.completion)
	}
	if err := inference.ValidateFailure(*failure); err != nil {
		t.Fatalf("sanitized failure failed ValidateFailure: %v", err)
	}
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

func TestConductorPreservesWriterFailureWithoutCompetingTerminal(t *testing.T) {
	t.Parallel()

	providers, recording := newLimitedCapabilityRegistry(t)
	sinkErr := errors.New("response sink failed")
	destination := &failingDestination{err: sinkErr}
	recording.invoke = func(ctx context.Context, request inference.InvocationRequest, writer inference.ResponseWriter) error {
		return writer.WriteEvent(ctx, runEvent(t, request.InvocationID(), string(recording.identity), workers.PhaseStarted))
	}
	err := conductor.New(providers).Invoke(
		context.Background(),
		"conductor.fixture",
		acceptedRequest("inv-writer-failure-terminal"),
		destination,
	)
	if !errors.Is(err, sinkErr) {
		t.Fatalf("Invoke() error = %v, want preserved destination failure", err)
	}
	if destination.closes != 0 {
		t.Fatalf("destination closes = %d, want 0 so writer failure stays the sole terminal signal", destination.closes)
	}
}

type terminalDestination struct {
	drafts     []inference.EventDraft
	completion *inference.Completion
	closes     int
}

func (w *terminalDestination) WriteEvent(_ context.Context, event inference.EventDraft) error {
	w.drafts = append(w.drafts, event)
	return nil
}

func (w *terminalDestination) Close(_ context.Context, completion inference.Completion) error {
	w.closes++
	clone := completion
	w.completion = &clone
	return nil
}

func assertExactlyOneTerminal(t *testing.T, destination *terminalDestination) {
	t.Helper()
	if destination.closes != 1 || destination.completion == nil {
		t.Fatalf("closes=%d completion=%#v, want exactly one terminal close", destination.closes, destination.completion)
	}
	response := destination.completion.Response()
	failure := destination.completion.Failure()
	if (response == nil) == (failure == nil) {
		t.Fatalf("completion has response=%v failure=%v, want exactly one outcome", response != nil, failure != nil)
	}
}

func assertSafeFailure(t *testing.T, failure *inference.Failure, want inference.FailureKind) {
	t.Helper()
	if failure == nil {
		t.Fatal("failure = nil, want normalized failure")
	}
	if failure.Kind() != want {
		t.Fatalf("failure kind = %q, want %q", failure.Kind(), want)
	}
	if err := inference.ValidateFailure(*failure); err != nil {
		t.Fatalf("failure is not customer-safe: %v", err)
	}
}
