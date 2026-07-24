package conductor

import (
	"context"
	"errors"
	"sync"

	inference "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
)

const (
	safeFailureMessage           = "provider invocation failed"
	collapsedTerminalMessage     = "provider invocation completed without a safe terminal outcome"
	InvariantUnsafeFailureDetail = "unsafe_failure_detail"
	InvariantCollapsedTerminal   = "collapsed_terminal"
)

// terminalGuard wraps the orchestration destination so each conductor Invoke
// yields exactly one terminal Close, or preserves a destination write failure
// as the sole terminal signal without publishing a competing close.
type terminalGuard struct {
	mu          sync.Mutex
	next        inference.ResponseWriter
	closed      bool
	writeFailed bool
	completion  *inference.Completion
}

func newTerminalGuard(next inference.ResponseWriter) *terminalGuard {
	return &terminalGuard{next: next}
}

func (g *terminalGuard) WriteEvent(ctx context.Context, event inference.EventDraft) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := g.next.WriteEvent(ctx, event); err != nil {
		g.writeFailed = true
		return err
	}
	return nil
}

func (g *terminalGuard) Close(ctx context.Context, completion inference.Completion) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.closeLocked(ctx, completion)
}

func (g *terminalGuard) closeLocked(ctx context.Context, completion inference.Completion) error {
	if g.closed {
		return newWriterClosedError("response completion may be closed exactly once")
	}
	safe := sanitizeCompletion(completion)
	if err := g.next.Close(ctx, safe); err != nil {
		g.writeFailed = true
		return err
	}
	g.closed = true
	clone := safe
	g.completion = &clone
	return nil
}

func (g *terminalGuard) finalize(ctx context.Context, invokeErr error) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed || g.writeFailed {
		return invokeErr
	}
	closeErr := g.closeLocked(ctx, inference.FailedCompletion(inference.NewFailure(inference.FailureInput{
		Kind:    inference.FailureMalformedOutput,
		Message: collapsedTerminalMessage,
		Diagnostics: map[string]string{
			"invariant": InvariantCollapsedTerminal,
		},
	})))
	if invokeErr == nil {
		return closeErr
	}
	if closeErr == nil || errors.Is(invokeErr, closeErr) {
		return invokeErr
	}
	return errors.Join(invokeErr, closeErr)
}

func sanitizeCompletion(completion inference.Completion) inference.Completion {
	failure := completion.Failure()
	if failure == nil {
		return completion
	}
	return inference.FailedCompletion(sanitizeFailure(*failure))
}

func sanitizeFailure(failure inference.Failure) inference.Failure {
	if err := inference.ValidateFailure(failure); err == nil {
		return inference.NewFailure(inference.FailureInput{
			Kind:            failure.Kind(),
			Message:         failure.Message(),
			Retryable:       failure.Retryable(),
			ProviderSession: failure.ProviderSession(),
			Diagnostics:     failure.Diagnostics(),
		})
	}
	kind := failure.Kind()
	if err := inference.ValidateFailure(inference.NewFailure(inference.FailureInput{
		Kind:    kind,
		Message: safeFailureMessage,
	})); err != nil {
		kind = inference.FailureUnknown
	}
	return inference.NewFailure(inference.FailureInput{
		Kind:      kind,
		Message:   safeFailureMessage,
		Retryable: failure.Retryable(),
		Diagnostics: map[string]string{
			"invariant": InvariantUnsafeFailureDetail,
		},
	})
}
