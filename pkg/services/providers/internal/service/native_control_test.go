package service_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/service"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog/wire"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
	executionwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/wire"
)

// ctxAwareAttempt is a channel-gated fake native adapter attempt that reports
// when it started, then blocks until either its context is cancelled (in
// which case it returns the same ExecuteFailureKindCanceled outcome every
// real native adapter returns for a killed process) or the test explicitly
// releases it with a plain success.
type ctxAwareAttempt struct {
	started chan struct{}
	release chan struct{}
}

func newCtxAwareAttempt() *ctxAwareAttempt {
	return &ctxAwareAttempt{started: make(chan struct{}), release: make(chan struct{})}
}

func (a *ctxAwareAttempt) attempt(ctx context.Context, request providers.ExecuteRequest) (providers.ExecuteResult, error) {
	close(a.started)
	select {
	case <-ctx.Done():
		return providers.ExecuteResult{}, providers.ExecuteFailure{Kind: providers.ExecuteFailureKindCanceled}
	case <-a.release:
		return providers.ExecuteResult{Content: request.AttemptID}, nil
	}
}

func mustControlCapableRootService(t *testing.T, attempt execution.Attempt) providers.Service {
	t.Helper()

	catalogService, err := catalogwire.NewService()
	if err != nil {
		t.Fatalf("catalogwire.NewService() = %v", err)
	}
	executionService, err := executionwire.NewService(
		catalogService,
		execution.Registration{Provider: providers.IDCodex, Attempt: attempt},
		execution.Registration{Provider: providers.IDClaude, Attempt: attempt},
	)
	if err != nil {
		t.Fatalf("executionwire.NewService() = %v", err)
	}
	root, err := providerservice.New(catalogService, executionService, logging.NoopLogger{})
	if err != nil {
		t.Fatalf("New() = %v", err)
	}
	return root
}

func TestControlAttempt_CancelReachesExactInFlightNativeAttempt(t *testing.T) {
	t.Parallel()

	fake := newCtxAwareAttempt()
	root := mustControlCapableRootService(t, fake.attempt)

	executeDone := make(chan error, 1)
	go func() {
		_, err := root.Execute(context.Background(), providers.ExecuteRequest{
			Provider:  providers.IDCodex,
			AttemptID: "in-flight-cancel",
		})
		executeDone <- err
	}()
	<-fake.started

	result, err := root.ControlAttempt(context.Background(), providers.ControlAttemptRequest{
		Provider:  providers.IDCodex,
		AttemptID: "in-flight-cancel",
		Action:    providers.ControlActionCancel,
	})
	if err != nil {
		t.Fatalf("ControlAttempt() error = %v, want nil", err)
	}
	if result.Outcome != providers.ControlOutcomeCompleted {
		t.Fatalf("ControlAttempt() outcome = %q, want completed", result.Outcome)
	}

	executeErr := <-executeDone
	if !errors.Is(executeErr, providers.ErrExecuteCancelled) {
		t.Fatalf("Execute() error = %v, want ErrExecuteCancelled", executeErr)
	}
}

func TestControlAttempt_TerminateReachesExactInFlightNativeAttempt(t *testing.T) {
	t.Parallel()

	fake := newCtxAwareAttempt()
	root := mustControlCapableRootService(t, fake.attempt)

	executeDone := make(chan error, 1)
	go func() {
		_, err := root.Execute(context.Background(), providers.ExecuteRequest{
			Provider:  providers.IDCodex,
			AttemptID: "in-flight-terminate",
		})
		executeDone <- err
	}()
	<-fake.started

	result, err := root.ControlAttempt(context.Background(), providers.ControlAttemptRequest{
		Provider:  providers.IDCodex,
		AttemptID: "in-flight-terminate",
		Action:    providers.ControlActionTerminate,
	})
	if err != nil {
		t.Fatalf("ControlAttempt() error = %v, want nil", err)
	}
	if result.Outcome != providers.ControlOutcomeCompleted {
		t.Fatalf("ControlAttempt() outcome = %q, want completed", result.Outcome)
	}

	executeErr := <-executeDone
	if !errors.Is(executeErr, providers.ErrExecuteCancelled) {
		t.Fatalf("Execute() error = %v, want ErrExecuteCancelled", executeErr)
	}
}

func TestControlAttempt_PauseHasNoNativeSeamAndLeavesAttemptRunning(t *testing.T) {
	t.Parallel()

	fake := newCtxAwareAttempt()
	root := mustControlCapableRootService(t, fake.attempt)

	executeDone := make(chan error, 1)
	go func() {
		_, err := root.Execute(context.Background(), providers.ExecuteRequest{
			Provider:  providers.IDCodex,
			AttemptID: "in-flight-pause",
		})
		executeDone <- err
	}()
	<-fake.started

	result, err := root.ControlAttempt(context.Background(), providers.ControlAttemptRequest{
		Provider:  providers.IDCodex,
		AttemptID: "in-flight-pause",
		Action:    providers.ControlActionPause,
	})
	if err != nil {
		t.Fatalf("ControlAttempt() error = %v, want nil", err)
	}
	if result.Outcome != providers.ControlOutcomeUnsupported {
		t.Fatalf("ControlAttempt(pause) outcome = %q, want unsupported", result.Outcome)
	}

	// A supported cancel on the same identity must still reach the attempt:
	// the unsupported pause must not have consumed or corrupted the live
	// registration.
	result, err = root.ControlAttempt(context.Background(), providers.ControlAttemptRequest{
		Provider:  providers.IDCodex,
		AttemptID: "in-flight-pause",
		Action:    providers.ControlActionCancel,
	})
	if err != nil {
		t.Fatalf("ControlAttempt(cancel) error = %v, want nil", err)
	}
	if result.Outcome != providers.ControlOutcomeCompleted {
		t.Fatalf("ControlAttempt(cancel) outcome = %q, want completed", result.Outcome)
	}

	executeErr := <-executeDone
	if !errors.Is(executeErr, providers.ErrExecuteCancelled) {
		t.Fatalf("Execute() error = %v, want ErrExecuteCancelled", executeErr)
	}
}

func TestControlAttempt_UnrelatedAttemptsAreUnaffectedByControl(t *testing.T) {
	t.Parallel()

	targetFake := newCtxAwareAttempt()
	bystanderFake := newCtxAwareAttempt()
	calls := 0
	root := mustControlCapableRootService(t, func(ctx context.Context, request providers.ExecuteRequest) (providers.ExecuteResult, error) {
		calls++
		if request.AttemptID == "target" {
			return targetFake.attempt(ctx, request)
		}
		return bystanderFake.attempt(ctx, request)
	})

	targetDone := make(chan error, 1)
	go func() {
		_, err := root.Execute(context.Background(), providers.ExecuteRequest{
			Provider:  providers.IDCodex,
			AttemptID: "target",
		})
		targetDone <- err
	}()
	<-targetFake.started

	bystanderDone := make(chan error, 1)
	go func() {
		_, err := root.Execute(context.Background(), providers.ExecuteRequest{
			Provider:  providers.IDCodex,
			AttemptID: "bystander",
		})
		bystanderDone <- err
	}()
	<-bystanderFake.started

	result, err := root.ControlAttempt(context.Background(), providers.ControlAttemptRequest{
		Provider:  providers.IDCodex,
		AttemptID: "target",
		Action:    providers.ControlActionCancel,
	})
	if err != nil {
		t.Fatalf("ControlAttempt() error = %v, want nil", err)
	}
	if result.Outcome != providers.ControlOutcomeCompleted {
		t.Fatalf("ControlAttempt() outcome = %q, want completed", result.Outcome)
	}
	if !errors.Is(<-targetDone, providers.ErrExecuteCancelled) {
		t.Fatal("target Execute() did not observe cancellation")
	}

	close(bystanderFake.release)
	if err := <-bystanderDone; err != nil {
		t.Fatalf("bystander Execute() error = %v, want nil (unrelated attempt must be unaffected)", err)
	}
	if calls != 2 {
		t.Fatalf("adapter calls = %d, want 2", calls)
	}
}

// TestControlAttempt_BlocksUntilSignaledNativeAttemptReturns proves
// ControlAttempt does not report ControlOutcomeCompleted merely because it
// requested cancellation: it must block until the controlled execution has
// actually observed its terminal behavior. The fake attempt here only
// returns after the test explicitly releases it post-cancellation, so if
// ControlAttempt returned early this test's ordering assertion would fail
// deterministically (no sleep-based timing involved).
func TestControlAttempt_BlocksUntilSignaledNativeAttemptReturns(t *testing.T) {
	t.Parallel()

	cancelledSeen := make(chan struct{})
	releaseAttempt := make(chan struct{})
	started := make(chan struct{})
	root := mustControlCapableRootService(t, func(ctx context.Context, request providers.ExecuteRequest) (providers.ExecuteResult, error) {
		close(started)
		<-ctx.Done()
		close(cancelledSeen)
		<-releaseAttempt
		return providers.ExecuteResult{}, providers.ExecuteFailure{Kind: providers.ExecuteFailureKindCanceled}
	})

	executeDone := make(chan error, 1)
	go func() {
		_, err := root.Execute(context.Background(), providers.ExecuteRequest{
			Provider:  providers.IDCodex,
			AttemptID: "blocks-until-terminal",
		})
		executeDone <- err
	}()
	<-started

	controlDone := make(chan providers.ControlAttemptResult, 1)
	go func() {
		result, err := root.ControlAttempt(context.Background(), providers.ControlAttemptRequest{
			Provider:  providers.IDCodex,
			AttemptID: "blocks-until-terminal",
			Action:    providers.ControlActionCancel,
		})
		if err != nil {
			t.Errorf("ControlAttempt() error = %v, want nil", err)
		}
		controlDone <- result
	}()

	<-cancelledSeen
	select {
	case <-controlDone:
		t.Fatal("ControlAttempt() returned before the attempt observed its terminal behavior")
	default:
	}

	close(releaseAttempt)
	result := <-controlDone
	if result.Outcome != providers.ControlOutcomeCompleted {
		t.Fatalf("ControlAttempt() outcome = %q, want completed", result.Outcome)
	}
	if !errors.Is(<-executeDone, providers.ErrExecuteCancelled) {
		t.Fatal("Execute() did not observe cancellation")
	}
}

// TestControlAttempt_DuplicateConcurrentControlsExactlyOneCompletes proves
// story 004's "at most one supported control can claim a live attempt"
// requirement at the root ControlAttempt level (registry.claim's exclusivity
// is already unit-tested directly; this proves ControlAttempt actually wires
// that guarantee through to real concurrent callers and the underlying
// adapter observes exactly one cancellation, not one per duplicate caller).
func TestControlAttempt_DuplicateConcurrentControlsExactlyOneCompletes(t *testing.T) {
	t.Parallel()

	fake := newCtxAwareAttempt()
	root := mustControlCapableRootService(t, fake.attempt)

	executeDone := make(chan error, 1)
	go func() {
		_, err := root.Execute(context.Background(), providers.ExecuteRequest{
			Provider:  providers.IDCodex,
			AttemptID: "duplicate-control",
		})
		executeDone <- err
	}()
	<-fake.started

	const callers = 8
	results := make(chan providers.ControlAttemptResult, callers)
	var wg sync.WaitGroup
	wg.Add(callers)
	for range callers {
		go func() {
			defer wg.Done()
			result, err := root.ControlAttempt(context.Background(), providers.ControlAttemptRequest{
				Provider:  providers.IDCodex,
				AttemptID: "duplicate-control",
				Action:    providers.ControlActionCancel,
			})
			if err != nil {
				t.Errorf("ControlAttempt() error = %v, want nil", err)
			}
			results <- result
		}()
	}
	wg.Wait()
	close(results)

	var completed, unsupported int
	for result := range results {
		switch result.Outcome {
		case providers.ControlOutcomeCompleted:
			completed++
		case providers.ControlOutcomeUnsupported:
			unsupported++
		default:
			t.Fatalf("unexpected outcome %q", result.Outcome)
		}
	}
	if completed != 1 {
		t.Fatalf("completed count = %d, want exactly 1", completed)
	}
	if unsupported != callers-1 {
		t.Fatalf("unsupported count = %d, want %d", unsupported, callers-1)
	}
	if !errors.Is(<-executeDone, providers.ErrExecuteCancelled) {
		t.Fatal("Execute() did not observe exactly one cancellation")
	}
}

// TestControlAttempt_UnsupportedForAlreadyTerminalAttempt proves an attempt
// that has already completed naturally (no control ever claimed it) answers
// a later control with the deterministic unsupported outcome, not an error
// or a stale/leaked live registration.
func TestControlAttempt_UnsupportedForAlreadyTerminalAttempt(t *testing.T) {
	t.Parallel()

	fake := newCtxAwareAttempt()
	root := mustControlCapableRootService(t, fake.attempt)

	executeDone := make(chan error, 1)
	go func() {
		_, err := root.Execute(context.Background(), providers.ExecuteRequest{
			Provider:  providers.IDCodex,
			AttemptID: "already-terminal",
		})
		executeDone <- err
	}()
	<-fake.started
	close(fake.release)
	if err := <-executeDone; err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	result, err := root.ControlAttempt(context.Background(), providers.ControlAttemptRequest{
		Provider:  providers.IDCodex,
		AttemptID: "already-terminal",
		Action:    providers.ControlActionCancel,
	})
	if err != nil {
		t.Fatalf("ControlAttempt() error = %v, want nil", err)
	}
	if result.Outcome != providers.ControlOutcomeUnsupported {
		t.Fatalf("ControlAttempt() outcome = %q, want unsupported for an already-terminal attempt", result.Outcome)
	}
}
