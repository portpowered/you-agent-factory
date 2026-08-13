package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/internal/services/agent"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/execution"
)

type service struct {
	providers providers.Service
	publish   workers.ProgressPublisher
}

// progressIdentity keeps the detached correlation emitted by Execute separate
// from the legacy dispatch fallback needed by direct RunnerExecutionRequest
// callers that predate the correlation field.
type progressIdentity struct {
	correlation workers.ExecutionCorrelation
	dispatchID  string
}

func progressIdentityForRequest(request workers.RunnerExecutionRequest) progressIdentity {
	identity := progressIdentity{
		correlation: request.Correlation,
		dispatchID:  request.Correlation.DispatchID,
	}
	if strings.TrimSpace(identity.dispatchID) == "" {
		identity.dispatchID = request.Dispatch.DispatchID
	}
	return identity
}

var _ agent.Service = (*service)(nil)

// New validates and captures the singular Providers root and Workers-owned
// observation edge without starting an attempt or constructing another graph.
func New(
	providersService providers.Service,
	publish workers.ProgressPublisher,
) (agent.Service, error) {
	if providersService == nil {
		return nil, misconfigured("agent Providers service is required", nil)
	}
	if publish == nil {
		return nil, misconfigured("agent progress publisher is required", nil)
	}
	return &service{providers: providersService, publish: publish}, nil
}

// Execute snapshots one common Runner request and delegates exactly one
// provider attempt. Retry, backoff, and scheduling policy remain caller-owned.
func (s *service) Execute(
	ctx context.Context,
	request workers.RunnerExecutionRequest,
) (workers.RunnerExecutionResult, error) {
	effective := *s
	effective.publish = workerexecution.ProgressPublisherFromContext(ctx, s.publish)
	return effective.execute(ctx, request)
}

func (s *service) execute(
	ctx context.Context,
	request workers.RunnerExecutionRequest,
) (workers.RunnerExecutionResult, error) {
	if err := ctx.Err(); err != nil {
		return workers.RunnerExecutionResult{}, err
	}
	request = workers.CloneProviderInferenceRequest(request)
	if err := validateRequest(request); err != nil {
		return workers.RunnerExecutionResult{}, err
	}
	identity := progressIdentityForRequest(request)
	provider := providerIDForRequest(request).String()
	result, err := s.executeProviderAttempt(ctx, request, identity)
	if err != nil {
		if response, normalizedErr, handled := continuationFailureResult(ctx, request, err); handled {
			s.publishTerminalFailure(
				identity,
				normalizedErr,
				response.ProviderSession,
				request.ResumeSession,
				"",
				provider,
			)
			return response, normalizedErr
		}
		if failure, ok := providerFailure(err); ok {
			response := runnerFailureResult(failure, request)
			normalizedErr := normalizeProviderFailure(ctx, failure, err, response, request.ResumeSession != nil)
			s.publishFailureProgress(
				identity,
				failure,
				response.ProviderSession,
				provider,
			)
			s.publishTerminalFailure(
				identity,
				normalizedErr,
				response.ProviderSession,
				failure.SessionRef,
				failure.Message,
				provider,
			)
			return response, normalizedErr
		}
		normalizedErr := normalizeExecutionError(ctx, err)
		s.publishTerminalFailure(identity, normalizedErr, nil, nil, "", provider)
		return workers.RunnerExecutionResult{}, normalizedErr
	}
	result = result.Clone()
	response := runnerResult(result, providerIDForRequest(request))
	if response.ProviderSession == nil && request.ResumeSession != nil {
		reference := request.ResumeSession.Clone()
		response.ProviderSession = &workers.ProviderSessionMetadata{
			Provider: workers.CanonicalProviderSessionProvider(reference.Provider.String()),
			Kind:     reference.Kind,
			ID:       reference.ID,
		}
	}
	if response.ProviderSession == nil && strings.TrimSpace(request.SessionID) != "" {
		response.ProviderSession = &workers.ProviderSessionMetadata{
			Provider: workers.CanonicalProviderSessionProvider(providerIDForRequest(request).String()),
			Kind:     providers.SessionIDKind,
			ID:       request.SessionID,
		}
	}
	response, err = normalizeAgentResponse(response, request)
	if err != nil {
		return response, err
	}
	s.publishProgress(identity, result, response.ProviderSession, provider)
	if !hasTerminalRunProgress(result.Diagnostics) {
		s.publish(workers.ProgressFragment{
			Correlation:              identity.correlation,
			DispatchID:               identity.dispatchID,
			Kind:                     workers.CompletedFragmentKind,
			Type:                     "COMPLETED",
			Provider:                 provider,
			ProviderSessionReference: workers.CloneProviderSessionReference(result.SessionRef),
			ProviderSessionRef:       workers.CloneProviderSessionMetadata(response.ProviderSession),
			ExternalEventType:        "STREAM_COMPLETED",
		})
	}
	return response, nil
}

type decisionEnvelope struct {
	Decision string `json:"decision"`
	Feedback string `json:"feedback"`
	Output   string `json:"output,omitempty"`
}

func normalizeAgentResponse(
	response workers.RunnerExecutionResult,
	request workers.RunnerExecutionRequest,
) (workers.RunnerExecutionResult, error) {
	if request.DecisionEnvelope || strings.EqualFold(strings.TrimSpace(request.OutputFormat), "decision-envelope") {
		return normalizeDecisionEnvelope(response, request)
	}
	if strings.TrimSpace(request.StopToken) == "" {
		return response, nil
	}
	response.Outcome = evaluateAgentOutcome(response.Content, request.StopToken)
	return response, nil
}

func normalizeDecisionEnvelope(
	response workers.RunnerExecutionResult,
	request workers.RunnerExecutionRequest,
) (workers.RunnerExecutionResult, error) {
	var envelope decisionEnvelope
	if err := json.Unmarshal([]byte(strings.TrimSpace(response.Content)), &envelope); err != nil {
		return response, workers.NewProviderError(
			workers.WorkFailureTypePermanentBadRequest,
			"reviewer decision envelope is invalid",
			err,
		)
	}
	decision := strings.ToUpper(strings.TrimSpace(envelope.Decision))
	if request.GoalRoutingDecisionEnvelope {
		classification, err := normalizeGoalDecision(decision)
		if err != nil {
			return response, workers.NewProviderError(
				workers.WorkFailureTypePermanentBadRequest,
				"goal decision envelope is invalid",
				err,
			)
		}
		response.Outcome = workers.OutcomeAccepted
		response.Classification = classification
	} else {
		switch workers.WorkOutcome(decision) {
		case workers.OutcomeAccepted, workers.OutcomeContinue, workers.OutcomeRejected:
			response.Outcome = workers.WorkOutcome(decision)
		default:
			return response, workers.NewProviderError(
				workers.WorkFailureTypePermanentBadRequest,
				"reviewer decision envelope has an unsupported decision",
				fmt.Errorf("decision %q", envelope.Decision),
			)
		}
	}
	response.Content = strings.TrimSpace(envelope.Output)
	response.Feedback = strings.TrimSpace(envelope.Feedback)
	return response, nil
}

func normalizeGoalDecision(decision string) (string, error) {
	classification := strings.ToLower(strings.ReplaceAll(decision, "-", "_"))
	switch classification {
	case "accepted", "needs_changes", "tests_failed", "needs_human", "blocked", "interrupted", "failed":
		return classification, nil
	default:
		return "", fmt.Errorf("decision %q", decision)
	}
}

func evaluateAgentOutcome(content, stopToken string) workers.WorkOutcome {
	if strings.TrimSpace(stopToken) == "" || strings.Contains(content, stopToken) {
		return workers.OutcomeAccepted
	}
	if strings.Contains(content, "<CONTINUE>") {
		return workers.OutcomeContinue
	}
	return workers.OutcomeRejected
}

func hasTerminalRunProgress(diagnostics *providers.ExecuteDiagnostics) bool {
	if diagnostics == nil {
		return false
	}
	for _, progress := range diagnostics.Progress {
		switch strings.ToLower(strings.TrimSpace(progress.Phase)) {
		case "run.completed", "turn.completed":
			return true
		}
	}
	return false
}

func (s *service) publishTerminalFailure(
	identity progressIdentity,
	err error,
	session *workers.ProviderSessionMetadata,
	reference *providers.SessionRef,
	providerMessage string,
	provider string,
) {
	eventType := "FAILED"
	message := "provider invocation failed"
	metadata := make(map[string]string, 2)
	if errors.Is(err, context.Canceled) {
		eventType = "CANCELED"
		message = agentCanceledFailureMessage
	}
	var providerErr *workers.ProviderError
	if errors.As(err, &providerErr) {
		metadata["work_failure_type"] = string(providerErr.Type)
		if providerErr.Family == workers.WorkFailureFamilyRetryable {
			metadata["retryable"] = "true"
		}
		if strings.TrimSpace(providerErr.Message) != "" {
			message = providerErr.Message
		}
	}
	if strings.TrimSpace(providerMessage) != "" && !errors.Is(err, context.Canceled) {
		message = providerMessage
	}
	if len(metadata) == 0 {
		metadata = nil
	}
	s.publish(workers.ProgressFragment{
		Correlation:              identity.correlation,
		DispatchID:               identity.dispatchID,
		Kind:                     workers.FailedFragmentKind,
		Type:                     eventType,
		Payload:                  boundedFailureMessage(message),
		Provider:                 provider,
		ProviderSessionReference: workers.CloneProviderSessionReference(reference),
		ProviderSessionRef:       workers.CloneProviderSessionMetadata(session),
		ExternalEventType:        "STREAM_FAILED",
		Metadata:                 metadata,
	})
}

func (s *service) publishFailureProgress(
	identity progressIdentity,
	failure providers.ExecuteFailure,
	session *workers.ProviderSessionMetadata,
	provider string,
) {
	if failure.Diagnostics == nil {
		return
	}
	s.publishProgress(identity, providers.ExecuteResult{
		SessionRef:  workers.CloneProviderSessionReference(failure.SessionRef),
		Diagnostics: failure.Diagnostics,
	}, session, provider)
}

func (s *service) publishProgress(
	identity progressIdentity,
	result providers.ExecuteResult,
	session *workers.ProviderSessionMetadata,
	provider string,
) {
	var terminalMessages []providers.ExecuteProgress
	// A provider that streamed its facts live has already delivered every
	// entry in Progress, in this order, through the attempt's ProgressObserver.
	// Replaying the slice here would publish each fact a second time.
	//
	// The terminal-message reordering below is unaffected: it only ever
	// matches the dotted "message.completed" phase the native adapters
	// produce, and a streaming provider reports its message facts as
	// started/delta/completed instead, so a streamed turn contributes no
	// terminal messages here in the first place.
	alreadyObserved := result.Diagnostics != nil && result.Diagnostics.ProgressAlreadyObserved
	if result.Diagnostics != nil && !alreadyObserved {
		for _, progress := range result.Diagnostics.Progress {
			if strings.EqualFold(strings.TrimSpace(progress.Phase), "message.completed") {
				terminalMessages = append(terminalMessages, progress)
				continue
			}
			s.publishProviderProgress(identity, progress, session, result.SessionRef, provider)
		}
	}
	if len(terminalMessages) == 0 && strings.TrimSpace(result.Content) != "" {
		terminalMessages = append(terminalMessages, providers.ExecuteProgress{
			Phase:  "message.completed",
			Detail: result.Content,
		})
	}
	// Publish authoritative completed messages after provider run/turn lifecycle
	// completion so all transports observe the same terminal ordering.
	for _, progress := range terminalMessages {
		s.publishProviderProgress(identity, progress, session, result.SessionRef, provider)
	}
}

func (s *service) publishProviderProgress(
	identity progressIdentity,
	progress providers.ExecuteProgress,
	session *workers.ProviderSessionMetadata,
	reference *providers.SessionRef,
	provider string,
) {
	s.publish(workers.ProgressFragment{
		Correlation:              identity.correlation,
		DispatchID:               identity.dispatchID,
		Kind:                     workers.ProgressFragmentKind,
		Type:                     progress.Phase,
		Payload:                  progress.Detail,
		Provider:                 provider,
		ProviderSessionReference: workers.CloneProviderSessionReference(reference),
		ProviderSessionRef:       workers.CloneProviderSessionMetadata(session),
		Metadata:                 cloneMetadata(progress.Metadata),
	})
}

func validateRequest(request workers.RunnerExecutionRequest) error {
	if request.ResumeSession == nil {
		if err := providers.ID(request.RunnerID).Validate(); err != nil {
			return badRequest("agent provider identity is invalid", err)
		}
	}
	if strings.TrimSpace(request.Dispatch.DispatchID) == "" {
		return badRequest("agent dispatch identity is required", nil)
	}
	if strings.TrimSpace(request.SystemPrompt) == "" &&
		strings.TrimSpace(request.UserMessage) == "" {
		return badRequest("agent prompt is required", nil)
	}
	return nil
}

// executeProviderAttempt runs one provider attempt for request. A
// Worker-Session-owned ResumeSession is always passed unchanged to
// Providers.Continue, preserving provider-specific kind and opaque identity
// without consulting runner/default selection. Legacy configuration SessionID
// resumes retain their established compatibility path. Providers.Execute is
// used only when neither continuation value is present.
func (s *service) executeProviderAttempt(
	ctx context.Context,
	request workers.RunnerExecutionRequest,
	identity progressIdentity,
) (providers.ExecuteResult, error) {
	attempt := providerRequest(request)
	// Both observers share one holder so live progress can be attributed to
	// the same provider-authored session the association fragment committed.
	live := &liveProviderSession{}
	attempt.SessionObserver = s.observeProviderSession(identity, live)
	attempt.ProgressObserver = s.observeProviderProgress(
		identity,
		live,
		providerIDForRequest(request).String(),
	)
	if request.ResumeSession != nil {
		reference := request.ResumeSession.Clone()
		continued, err := s.providers.Continue(ctx, providers.ContinueRequest{
			Reference: reference,
			Attempt:   attempt,
		})
		if err != nil {
			return providers.ExecuteResult{}, err
		}
		if continued.Outcome == providers.ContinuationOutcomeUnsupported {
			return providers.ExecuteResult{}, continuationUnsupportedError{reference: reference}
		}
		return continuedExecuteResult(continued, reference)
	}
	if strings.TrimSpace(request.SessionID) == "" {
		return s.providers.Execute(ctx, attempt)
	}
	reference := providers.SessionRef{
		Provider: attempt.Provider,
		Kind:     providers.SessionIDKind,
		ID:       request.SessionID,
	}
	continued, err := s.providers.Continue(ctx, providers.ContinueRequest{
		Reference: reference,
		Attempt:   attempt,
	})
	if err != nil {
		return providers.ExecuteResult{}, err
	}
	if continued.Outcome == providers.ContinuationOutcomeUnsupported {
		return providers.ExecuteResult{}, continuationUnsupportedError{reference: reference}
	}
	return continued.Result, nil
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
			Correlation:              identity.correlation,
			DispatchID:               identity.dispatchID,
			Kind:                     workers.ProviderSessionObservedFragmentKind,
			ProviderSessionReference: &reference,
			ProviderSessionRef: &workers.ProviderSessionMetadata{
				Provider: workers.CanonicalProviderSessionProvider(reference.Provider.String()),
				Kind:     reference.Kind,
				ID:       reference.ID,
			},
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
		reference, session := live.snapshot()
		s.publishProviderProgress(identity, progress, session, reference, provider)
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

// snapshot returns a detached reference and its matching response metadata, or
// two nils when the provider has not authored a session yet.
func (l *liveProviderSession) snapshot() (*providers.SessionRef, *workers.ProviderSessionMetadata) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.reference == nil {
		return nil, nil
	}
	reference := l.reference.Clone()
	return &reference, &workers.ProviderSessionMetadata{
		Provider: workers.CanonicalProviderSessionProvider(reference.Provider.String()),
		Kind:     reference.Kind,
		ID:       reference.ID,
	}
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

func providerRequest(request workers.RunnerExecutionRequest) providers.ExecuteRequest {
	providerID := providerIDForRequest(request)
	result := providers.ExecuteRequest{
		Provider:           providerID,
		AttemptID:          request.Dispatch.DispatchID,
		WorkerType:         request.WorkerType,
		WorkstationName:    request.WorkstationType,
		Model:              request.Model,
		ReasoningEffort:    request.ReasoningEffort,
		SkipPermissions:    request.SkipPermissions,
		PrintTimeout:       request.PrintTimeout,
		SystemPrompt:       request.SystemPrompt,
		UserMessage:        request.UserMessage,
		InputTokens:        cloneInputTokens(request.InputTokens),
		OutputSchema:       request.OutputSchema,
		WorkingDirectory:   request.WorkingDirectory,
		Worktree:           request.Worktree,
		EnvVars:            cloneMetadata(request.EnvVars),
		ProcessEnvironment: append([]string(nil), request.ProcessEnvironment...),
	}
	return result
}

// providerIDForRunner translates stable Workers runner identities at the
// Providers boundary.
func providerIDForRunner(runnerID string) providers.ID {
	return providers.ID(workers.NormalizeRunnerID(runnerID))
}

// providerIDForRequest makes the Worker Session-owned exact reference the
// authority for continuation routing. Runner IDs remain the selection input
// for ordinary execution, but cannot redirect or invalidate an admitted
// continuation whose provider identity is already recorded in ResumeSession.
func providerIDForRequest(request workers.RunnerExecutionRequest) providers.ID {
	if request.ResumeSession != nil {
		return request.ResumeSession.Provider
	}
	return providerIDForRunner(request.RunnerID)
}

func runnerResult(
	result providers.ExecuteResult,
	providerID providers.ID,
) workers.RunnerExecutionResult {
	result = result.Clone()
	response := workers.RunnerExecutionResult{Content: result.Content}
	if result.SessionRef != nil {
		response.ProviderSession = &workers.ProviderSessionMetadata{
			Provider: workers.CanonicalProviderSessionProvider(
				result.SessionRef.Provider.String(),
			),
			Kind: result.SessionRef.Kind,
			ID:   result.SessionRef.ID,
		}
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
	if response.ProviderSession == nil && failure.SessionRef != nil {
		response.ProviderSession = &workers.ProviderSessionMetadata{
			Provider: workers.CanonicalProviderSessionProvider(
				failure.SessionRef.Provider.String(),
			),
			Kind: failure.SessionRef.Kind,
			ID:   failure.SessionRef.ID,
		}
	}
	if response.ProviderSession == nil && request.ResumeSession != nil {
		response.ProviderSession = &workers.ProviderSessionMetadata{
			Provider: workers.CanonicalProviderSessionProvider(request.ResumeSession.Provider.String()),
			Kind:     request.ResumeSession.Kind,
			ID:       request.ResumeSession.ID,
		}
	}
	if response.ProviderSession == nil && strings.TrimSpace(request.SessionID) != "" {
		response.ProviderSession = &workers.ProviderSessionMetadata{
			Provider: workers.CanonicalProviderSessionProvider(request.RunnerID),
			Kind:     providers.SessionIDKind,
			ID:       request.SessionID,
		}
	}
	if response.ProviderSession == nil {
		response.ProviderSession = &workers.ProviderSessionMetadata{
			Provider: workers.CanonicalProviderSessionProvider(request.RunnerID),
		}
	} else if strings.TrimSpace(response.ProviderSession.Provider) == "" {
		response.ProviderSession.Provider = workers.CanonicalProviderSessionProvider(
			request.RunnerID,
		)
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
	continuation bool,
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
	failureType := failureTypeForProviderKind(failure.Kind)
	if failure.Diagnostics != nil {
		switch failure.Diagnostics.Metadata["work-failure-type"] {
		case string(workers.WorkFailureTypeMissingExecutable):
			failureType = workers.WorkFailureTypeMissingExecutable
		case string(workers.WorkFailureTypeMisconfigured):
			failureType = workers.WorkFailureTypeMisconfigured
		}
	}
	if errors.Is(interruption, context.DeadlineExceeded) {
		failureType = workers.WorkFailureTypeTimeout
	}
	normalized := workers.NewProviderError(
		failureType,
		boundedFailureMessage(canonicalAgentFailureMessage(failureType, failure.Message)),
		errors.Join(interruption, cause),
	)
	if continuation {
		normalized.ProviderFailureKind = failure.Kind
	}
	normalized.ProviderSession = workers.CloneProviderSessionMetadata(
		result.ProviderSession,
	)
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
		normalized.ProviderSession = workers.CloneProviderSessionMetadata(response.ProviderSession)
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
		normalized.ProviderSession = workers.CloneProviderSessionMetadata(response.ProviderSession)
		return response, normalized, true
	}
	return workers.RunnerExecutionResult{}, nil, false
}

func runnerContinuationFailureResult(
	request workers.RunnerExecutionRequest,
	fallback providers.SessionRef,
) workers.RunnerExecutionResult {
	reference := fallback.Clone()
	if request.ResumeSession != nil {
		reference = request.ResumeSession.Clone()
	}
	return runnerFailureResult(providers.ExecuteFailure{SessionRef: &reference}, request)
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

func canceledProviderError(cause error, result workers.RunnerExecutionResult) *workers.ProviderError {
	normalized := workers.NewProviderError(
		workers.WorkFailureTypeUnknown,
		agentCanceledFailureMessage,
		cause,
	)
	normalized.ProviderSession = workers.CloneProviderSessionMetadata(result.ProviderSession)
	normalized.Diagnostics = workers.CloneWorkDiagnostics(result.Diagnostics)
	return normalized
}

func normalizeExecutionError(ctx context.Context, err error) error {
	if contextErr := ctx.Err(); contextErr != nil {
		return errors.Join(contextErr, err)
	}
	return workers.NewProviderError(
		workers.WorkFailureTypeInternalServerError,
		boundedFailureMessage(err.Error()),
		err,
	)
}

func failureTypeForProviderKind(
	kind providers.ExecuteFailureKind,
) workers.WorkFailureType {
	switch kind {
	case providers.ExecuteFailureKindAuthentication:
		return workers.WorkFailureTypeAuthFailure
	case providers.ExecuteFailureKindInvalidRequest:
		return workers.WorkFailureTypePermanentBadRequest
	case providers.ExecuteFailureKindCapabilityMismatch:
		return workers.WorkFailureTypePermanentBadRequest
	case providers.ExecuteFailureKindMisconfigured:
		return workers.WorkFailureTypeMisconfigured
	case providers.ExecuteFailureKindThrottled:
		return workers.WorkFailureTypeThrottled
	case providers.ExecuteFailureKindDependency:
		return workers.WorkFailureTypeInternalServerError
	case providers.ExecuteFailureKindTimeout:
		return workers.WorkFailureTypeTimeout
	default:
		return workers.WorkFailureTypeUnknown
	}
}

const failureMessageRuneLimit = 512

const (
	agentTimeoutFailureMessage  = "provider invocation timed out"
	agentCanceledFailureMessage = "provider invocation was canceled"
)

func canonicalAgentFailureMessage(
	failureType workers.WorkFailureType,
	providerMessage string,
) string {
	switch failureType {
	case workers.WorkFailureTypeTimeout:
		return agentTimeoutFailureMessage
	case workers.WorkFailureTypeUnknown:
		if strings.TrimSpace(providerMessage) == "" {
			return "provider invocation failed"
		}
	}
	return providerMessage
}

func boundedFailureMessage(message string) string {
	message = strings.TrimSpace(message)
	runes := []rune(message)
	if len(runes) <= failureMessageRuneLimit {
		return message
	}
	return string(runes[:failureMessageRuneLimit])
}

func cloneMetadata(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func cloneInputTokens(values []any) []any {
	if values == nil {
		return nil
	}
	return append([]any(nil), values...)
}

func badRequest(message string, cause error) error {
	return workers.NewProviderError(
		workers.WorkFailureTypePermanentBadRequest,
		message,
		cause,
	)
}

func misconfigured(message string, cause error) error {
	return workers.NewProviderError(
		workers.WorkFailureTypeMisconfigured,
		message,
		cause,
	)
}
