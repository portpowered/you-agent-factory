package service

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func continueLegacyProvider(
	ctx context.Context,
	service providers.Service,
	request providers.ContinueRequest,
) (providers.ContinueResult, error) {
	return service.Continue(ctx, request)
}

func continuedExecuteResultFromOpaque(
	continued providers.ContinueReferenceResult,
	requested providers.SessionRef,
	returned providers.SessionRef,
) (providers.ExecuteResult, error) {
	return continuedExecuteResult(
		providers.ContinueResult{
			Reference: returned,
			Outcome:   continued.Outcome,
			Result:    continued.Result,
		},
		requested,
	)
}

// observeProviderSession forwards only a provider-authored exact reference to
// the session-local progress bridge while the Provider attempt is still live.
// The bridge owns association validation and commits it before allowing the
// later response or terminal fragments for this dispatch to proceed.
func (s *service) observeProviderSession(
	identity progressIdentity,
	live *liveProviderSession,
) providers.SessionObserver {
	return func(reference providers.SessionRef) {
		reference = reference.Clone()
		live.set(reference)
		s.publish(workers.ProgressFragment{
			Correlation:  identity.correlation,
			DispatchID:   identity.dispatchID,
			Kind:         workers.ProviderSessionObservedFragmentKind,
			Continuation: continuationFromSessionRef(&reference),
		})
	}
}

// observeProviderProgress publishes each bounded provider progress fact while
// the attempt is still live, so a Worker's execution trace reaches the
// response-event stream as it is produced rather than in one burst when the
// attempt ends.
//
// Each fact carries whatever provider session the attempt has already
// observed, so a streamed trace is attributed to the same provider and session
// as the buffered diagnostics it replaces. A fact reported before the provider
// authored a session simply carries none.
func (s *service) observeProviderProgress(
	identity progressIdentity,
	live *liveProviderSession,
	provider string,
) providers.ProgressObserver {
	return func(progress providers.ExecuteProgress) {
		continuation := live.snapshot()
		s.publishProviderProgress(identity, progress, continuation, provider)
	}
}

// liveProviderSession retains the exact provider-authored session reference
// observed during one attempt. Progress observation and session observation
// arrive on different callbacks and, for a streaming provider, on different
// goroutines, so the reference is guarded rather than passed by value.
type liveProviderSession struct {
	mu        sync.Mutex
	reference *providers.SessionRef
}

func (l *liveProviderSession) set(reference providers.SessionRef) {
	clone := reference.Clone()
	l.mu.Lock()
	l.reference = &clone
	l.mu.Unlock()
}

// snapshot returns the opaque continuation for the provider-authored session,
// or nil when the provider has not authored a session yet.
func (l *liveProviderSession) snapshot() *workers.ProviderContinuationRef {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.reference == nil {
		return nil
	}
	reference := l.reference.Clone()
	continuation := reference.ContinuationRef()
	return &continuation
}

// continuedExecuteResult admits only a provider response that affirms the
// exact typed Provider Session requested for the continuation. It also carries
// that canonical reference into the result when the provider omits the
// redundant ExecuteResult field, so progress and terminal output retain the
// same identity without rebuilding it from a legacy session ID.
func continuedExecuteResult(continued providers.ContinueResult, reference providers.SessionRef) (providers.ExecuteResult, error) {
	if continued.Reference != reference {
		return providers.ExecuteResult{}, invalidContinuationReference(reference)
	}
	result := continued.Result.Clone()
	if result.SessionRef != nil && *result.SessionRef != reference {
		return providers.ExecuteResult{}, invalidContinuationReference(reference)
	}
	continuedReference := reference.Clone()
	result.SessionRef = &continuedReference
	return result, nil
}

func invalidContinuationReference(reference providers.SessionRef) providers.ContinuationFailure {
	return providers.ContinuationFailure{
		Kind:      providers.ContinuationFailureKindInvalid,
		Message:   "provider continuation returned a different session reference",
		Reference: reference.Clone(),
	}
}

// providerIDForRunner translates stable Workers runner identities at the
// Providers boundary.
func providerIDForRunner(runnerID string) providers.ID {
	return providers.ID(workers.NormalizeRunnerID(runnerID))
}

// providerIDForRequest makes the opaque continuation's provider identity the
// authority for continuation routing. Runner IDs remain the selection input
// for ordinary execution, but cannot redirect or invalidate an admitted
// continuation.
func providerIDForRequest(request workers.RunnerExecutionRequest) providers.ID {
	if request.Continuation != nil {
		return providers.ID(strings.TrimSpace(request.Continuation.Provider))
	}
	return providerIDForRunner(request.RunnerID)
}

func continuationSessionRef(
	request workers.RunnerExecutionRequest,
) *providers.SessionRef {
	if request.Continuation != nil {
		if reference, err := request.Continuation.ToSessionRef(); err == nil {
			return &reference
		}
	}
	if sessionID := strings.TrimSpace(request.SessionID); sessionID != "" {
		return &providers.SessionRef{
			Provider: providerIDForRunner(request.RunnerID),
			Kind:     providers.SessionIDKind,
			ID:       sessionID,
		}
	}
	return nil
}

func runnerResult(
	result providers.ExecuteResult,
	providerID providers.ID,
) workers.RunnerExecutionResult {
	result = result.Clone()
	response := workers.RunnerExecutionResult{
		Content: result.Content,
		Outcome: workers.WorkOutcome(result.Outcome),
	}
	if result.SessionRef != nil {
		response.Continuation = continuationFromSessionRef(result.SessionRef)
	}
	if result.Diagnostics != nil {
		metadata := cloneMetadata(result.Diagnostics.Metadata)
		if result.Diagnostics.DurationMillis != 0 {
			if metadata == nil {
				metadata = make(map[string]string, 1)
			}
			metadata[workers.ProviderResponseMetadataDurationMS] =
				strconv.FormatInt(result.Diagnostics.DurationMillis, 10)
		}
		response.Diagnostics = &workers.WorkDiagnostics{
			Provider: &workers.ProviderDiagnostic{
				Provider:         providerID.String(),
				ResponseMetadata: cloneMetadata(metadata),
			},
			Metadata: metadata,
		}
		if result.Diagnostics.Command != nil {
			response.Diagnostics.Command = &workers.CommandDiagnostic{
				Command:    result.Diagnostics.Command.Command,
				Args:       append([]string(nil), result.Diagnostics.Command.Args...),
				Env:        cloneMetadata(result.Diagnostics.Command.Env),
				Stdin:      result.Diagnostics.Command.Stdin,
				Stdout:     result.Diagnostics.Command.Stdout,
				Stderr:     result.Diagnostics.Command.Stderr,
				ExitCode:   result.Diagnostics.Command.ExitCode,
				TimedOut:   result.Diagnostics.Command.TimedOut,
				Duration:   time.Duration(result.Diagnostics.Command.DurationMS) * time.Millisecond,
				WorkingDir: result.Diagnostics.Command.WorkingDir,
			}
		}
		if result.Diagnostics.Panic != nil {
			response.Diagnostics.Panic = &workers.PanicDiagnostic{
				Message: result.Diagnostics.Panic.Message,
				Stack:   result.Diagnostics.Panic.Stack,
			}
		}
	}
	return response
}

func runnerFailureResult(
	failure providers.ExecuteFailure,
	request workers.RunnerExecutionRequest,
) workers.RunnerExecutionResult {
	response := runnerResult(providers.ExecuteResult{
		SessionRef:  failure.SessionRef,
		Diagnostics: failure.Diagnostics,
	}, providerIDForRequest(request))
	if response.Continuation == nil && failure.SessionRef != nil {
		response.Continuation = continuationFromSessionRef(failure.SessionRef)
	}
	if response.Continuation == nil {
		if reference := continuationSessionRef(request); reference != nil {
			response.Continuation = continuationFromSessionRef(reference)
		}
	}
	if response.Continuation == nil && strings.TrimSpace(request.SessionID) != "" {
		response.Continuation = continuationFromSessionRef(&providers.SessionRef{
			Provider: providerIDForRequest(request),
			Kind:     providers.SessionIDKind,
			ID:       request.SessionID,
		})
	}
	if response.Continuation == nil {
		response.Continuation = &workers.ProviderContinuationRef{
			Provider: providers.ID(request.RunnerID).CanonicalSessionProvider(),
		}
	} else if strings.TrimSpace(response.Continuation.Provider) == "" {
		response.Continuation.Provider = providers.ID(request.RunnerID).CanonicalSessionProvider()
	}
	return response
}

func providerFailure(err error) (providers.ExecuteFailure, bool) {
	var value providers.ExecuteFailure
	if errors.As(err, &value) {
		return value.Clone(), true
	}
	var pointer *providers.ExecuteFailure
	if errors.As(err, &pointer) && pointer != nil {
		return pointer.Clone(), true
	}
	return providers.ExecuteFailure{}, false
}

func normalizeProviderFailure(
	ctx context.Context,
	failure providers.ExecuteFailure,
	cause error,
	result workers.RunnerExecutionResult,
) error {
	interruption := ctx.Err()
	if errors.Is(interruption, context.Canceled) {
		return canceledProviderError(errors.Join(context.Canceled, cause), result)
	}
	if interruption == nil {
		switch failure.Kind {
		case providers.ExecuteFailureKindCanceled:
			interruption = context.Canceled
		case providers.ExecuteFailureKindTimeout:
			interruption = context.DeadlineExceeded
		}
	}
	if failure.Kind == providers.ExecuteFailureKindCanceled {
		return canceledProviderError(errors.Join(interruption, cause), result)
	}
	failureType := failureTypeForProviderFailure(failure)
	if failure.Diagnostics != nil {
		switch failure.Diagnostics.Metadata["work-failure-type"] {
		case string(workers.WorkFailureTypeMissingExecutable):
			failureType = workers.WorkFailureTypeMissingExecutable
		case string(workers.WorkFailureTypeMisconfigured):
			failureType = workers.WorkFailureTypeMisconfigured
		case string(workers.WorkFailureTypeCommandLineTooLong):
			failureType = workers.WorkFailureTypeCommandLineTooLong
		}
	}
	if errors.Is(interruption, context.DeadlineExceeded) {
		failureType = workers.WorkFailureTypeTimeout
	}
	normalized := workers.NewProviderError(
		failureType,
		boundedFailureMessage(canonicalAgentFailureMessage(failure, failureType, failure.Message)),
		errors.Join(interruption, cause),
	)
	// Providers has already normalized and sanitized this message. Retain the
	// provider failure kind on fresh attempts as well as continuations so the
	// downstream invocation/child boundary can distinguish provider-owned safe
	// detail from an arbitrary Worker error.
	normalized.ProviderFailureKind = failure.Kind
	normalized.Continuation = cloneContinuation(result.Continuation)
	normalized.Diagnostics = workers.CloneWorkDiagnostics(result.Diagnostics)
	return normalized
}

// continuationUnsupportedError keeps Providers' successful unsupported
// capability result distinct from an invalid Execute failure until Workers has
// copied that exact classification into its own in-process result boundary.
type continuationUnsupportedError struct {
	reference providers.SessionRef
}

func (continuationUnsupportedError) Error() string {
	return "provider does not support resuming this Provider Session"
}

func continuationFailureResult(
	ctx context.Context,
	request workers.RunnerExecutionRequest,
	err error,
) (workers.RunnerExecutionResult, error, bool) {
	if unsupported, ok := unsupportedContinuation(err); ok {
		response := runnerContinuationFailureResult(request, unsupported.reference)
		normalized := workers.NewProviderError(
			workers.WorkFailureTypePermanentBadRequest,
			"provider session continuation is unsupported",
			err,
		)
		normalized.ProviderContinuationOutcome = providers.ContinuationOutcomeUnsupported
		normalized.Continuation = cloneContinuation(response.Continuation)
		return response, normalized, true
	}
	if failure, ok := continuationFailure(err); ok {
		response := runnerContinuationFailureResult(request, failure.Reference)
		normalized := workers.NewProviderError(
			workers.WorkFailureTypePermanentBadRequest,
			"provider session continuation was rejected",
			errors.Join(ctx.Err(), err),
		)
		normalized.ProviderContinuationFailureKind = failure.Kind
		normalized.Continuation = cloneContinuation(response.Continuation)
		return response, normalized, true
	}
	return workers.RunnerExecutionResult{}, nil, false
}

func runnerContinuationFailureResult(
	request workers.RunnerExecutionRequest,
	fallback providers.SessionRef,
) workers.RunnerExecutionResult {
	reference := fallback.Clone()
	if requested := continuationSessionRef(request); requested != nil {
		reference = requested.Clone()
	}
	result := runnerFailureResult(providers.ExecuteFailure{SessionRef: &reference}, request)
	if request.Continuation != nil {
		result.Continuation = cloneContinuation(request.Continuation)
	}
	return result
}

func unsupportedContinuation(err error) (continuationUnsupportedError, bool) {
	var unsupported continuationUnsupportedError
	if errors.As(err, &unsupported) {
		return unsupported, true
	}
	return continuationUnsupportedError{}, false
}

func continuationFailure(err error) (providers.ContinuationFailure, bool) {
	var value providers.ContinuationFailure
	if errors.As(err, &value) {
		return value.Clone(), true
	}
	var pointer *providers.ContinuationFailure
	if errors.As(err, &pointer) && pointer != nil {
		return pointer.Clone(), true
	}
	return providers.ContinuationFailure{}, false
}

func cloneContinuation(reference *workers.ProviderContinuationRef) *workers.ProviderContinuationRef {
	if reference == nil {
		return nil
	}
	clone := reference.Clone()
	return &clone
}

func continuationFromSessionRef(reference *providers.SessionRef) *workers.ProviderContinuationRef {
	if reference == nil {
		return nil
	}
	continuation := reference.ContinuationRef()
	return &continuation
}

func cloneSessionRef(reference *providers.SessionRef) *providers.SessionRef {
	if reference == nil {
		return nil
	}
	clone := reference.Clone()
	return &clone
}
