package service

import (
	"context"
	"errors"
	"strings"
	"sync"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers/internal/execution"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/internal/services/agent"
)

type service struct {
	providers providers.Service
	publish   workers.ProgressPublisher
	// decisionEnvelopes is the injected Factory Definitions owner of decision
	// envelope interpretation. Workers never parses the envelope itself: the
	// contract, its vocabulary, and its malformed-envelope failure shape all
	// belong to invocation policy.
	decisionEnvelopes interfaces.DecisionEnvelopeService
}

// progressIdentity keeps the detached correlation emitted by Execute separate
// from the legacy dispatch fallback needed by direct RunnerExecutionRequest
// callers that predate the correlation field.
type progressIdentity struct {
	correlation workers.ExecutionCorrelation
	dispatchID  string
}

const (
	suppressedTerminalErrorKindMetadata = "suppressed_terminal_error_kind"
	noUsableAgentResultMessage          = "provider returned no usable result"
)

// terminalPublication is scoped to one runner attempt. It serializes the
// attempt's observation edge, closes that edge before publishing a terminal
// fragment, and ignores any provider callback that arrives after terminal
// selection. The runner service itself is reused concurrently, so this state
// must never live on service.
type terminalPublication struct {
	mu      sync.Mutex
	closed  bool
	publish workers.ProgressPublisher
}

func newTerminalPublication(publish workers.ProgressPublisher) *terminalPublication {
	return &terminalPublication{publish: publish}
}

func (publication *terminalPublication) progress(fragment workers.ProgressFragment) {
	if publication == nil || publication.publish == nil {
		return
	}
	publication.mu.Lock()
	defer publication.mu.Unlock()
	if publication.closed {
		return
	}
	publication.publish(fragment)
}

func (publication *terminalPublication) terminal(fragment workers.ProgressFragment) {
	if publication == nil || publication.publish == nil {
		return
	}
	publication.mu.Lock()
	defer publication.mu.Unlock()
	if publication.closed {
		return
	}
	publication.closed = true
	publication.publish(fragment)
}

func (publication *terminalPublication) close() {
	if publication == nil {
		return
	}
	publication.mu.Lock()
	publication.closed = true
	publication.mu.Unlock()
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

// New validates and captures the singular Providers root, the Workers-owned
// observation edge, and the injected decision-envelope owner without starting
// an attempt or constructing another graph.
func New(
	providersService providers.Service,
	publish workers.ProgressPublisher,
	decisionEnvelopes ...interfaces.DecisionEnvelopeService,
) (agent.Service, error) {
	if providersService == nil {
		return nil, misconfigured("agent Providers service is required", nil)
	}
	if publish == nil {
		return nil, misconfigured("agent progress publisher is required", nil)
	}
	return &service{
		providers:         providersService,
		publish:           publish,
		decisionEnvelopes: firstDecisionEnvelopeService(decisionEnvelopes),
	}, nil
}

func firstDecisionEnvelopeService(
	services []interfaces.DecisionEnvelopeService,
) interfaces.DecisionEnvelopeService {
	for _, service := range services {
		if service != nil {
			return service
		}
	}
	return nil
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
	publication := newTerminalPublication(s.publish)
	effective := *s
	effective.publish = publication.progress
	return effective.executeWithTerminalPublication(ctx, request, publication)
}

func (s *service) executeWithTerminalPublication(
	ctx context.Context,
	request workers.RunnerExecutionRequest,
	publication *terminalPublication,
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
	resumeReference := continuationSessionRef(request)
	result, attemptErr := s.executeProviderAttempt(ctx, request, identity)
	result = result.Clone()
	response := runnerResult(result, providerIDForRequest(request))
	response = preserveContinuation(response, request, resumeReference)
	var resultErr error
	// An empty result does not carry a candidate to normalize. Skipping the
	// semantic decoder in that one case preserves the provider's original
	// typed failure and avoids replacing a process/dependency route with a
	// malformed empty decision envelope.
	if hasAgentCandidate(result) || attemptErr == nil {
		response, resultErr = s.normalizeAgentResponse(response, request)
	}
	if resultErr == nil && hasAgentCandidate(result) &&
		(attemptErr == nil || usableAgentResult(response, request)) {
		response.Diagnostics = mergeSuppressedAttemptDiagnostics(response.Diagnostics, attemptErr)
		s.publishProgress(identity, result, response.Continuation, provider)
		if hasTerminalRunProgress(result.Diagnostics) {
			// Native provider lifecycle progress is already the authoritative
			// terminal observation. Close the callback edge without adding a
			// duplicate synthetic COMPLETED fragment.
			publication.close()
		} else {
			s.publishCompletedOnce(
				publication,
				identity,
				response.Continuation,
				provider,
				response.Diagnostics,
			)
		}
		return response, nil
	}
	if resultErr != nil {
		attemptErr = resultErr
	}
	return s.publishAttemptFailure(
		ctx,
		publication,
		identity,
		request,
		response,
		attemptErr,
		resumeReference,
		provider,
	)
}

func (s *service) normalizeAgentResponse(
	response workers.RunnerExecutionResult,
	request workers.RunnerExecutionRequest,
) (workers.RunnerExecutionResult, error) {
	if request.DecisionEnvelope ||
		strings.EqualFold(
			strings.TrimSpace(request.OutputFormat),
			interfaces.DecisionEnvelopeOutcomeFormat,
		) {
		return s.normalizeDecisionEnvelope(response, request)
	}
	if strings.TrimSpace(request.StopToken) == "" {
		return response, nil
	}
	response.Outcome = evaluateAgentOutcome(response.Content, request.StopToken)
	return response, nil
}

// normalizeDecisionEnvelope delegates every envelope decision to the injected
// Factory Definitions owner and copies its canonical WorkResult onto the runner
// result. A malformed envelope keeps its canonical failure outcome and
// completion-validation diagnostics, and is surfaced as a terminal runner error
// so an agent loop stops instead of replaying an unreadable turn.
func (s *service) normalizeDecisionEnvelope(
	response workers.RunnerExecutionResult,
	request workers.RunnerExecutionRequest,
) (workers.RunnerExecutionResult, error) {
	if s.decisionEnvelopes == nil {
		return response, misconfigured(
			"agent decision-envelope service is required for decision-envelope output",
			nil,
		)
	}
	raw := response.Content
	result := s.decisionEnvelopeWorkResult(request, raw)
	response.Outcome = result.Outcome
	response.Feedback = strings.TrimSpace(result.Feedback)
	response.Classification = result.SelectedClassificationLabel
	response.RecordedOutputWork = result.RecordedOutputWork
	response.Diagnostics = mergeDecisionEnvelopeDiagnostics(response.Diagnostics, result.Diagnostics)
	if strings.TrimSpace(result.Error) != "" {
		// The unreadable response stays on Content so operators still see what
		// the worker actually produced.
		return response, malformedDecisionEnvelopeError(result, response)
	}
	response.Content = strings.TrimSpace(result.Output)
	return response, nil
}

func (s *service) decisionEnvelopeWorkResult(
	request workers.RunnerExecutionRequest,
	raw string,
) workers.WorkResult {
	if request.GoalRoutingDecisionEnvelope {
		return s.decisionEnvelopes.WorkResultFromGoalRoutingDecisionEnvelopeJSONOrFailed(
			request.Dispatch.DispatchID,
			request.Dispatch.TransitionID,
			raw,
		)
	}
	return s.decisionEnvelopes.WorkResultFromDecisionEnvelopeJSONOrFailed(
		request.Dispatch.DispatchID,
		request.Dispatch.TransitionID,
		raw,
	)
}

// malformedDecisionEnvelopeError preserves the failure classification the
// decision-envelope owner assigned instead of restating it as a Workers-local
// bad-request decision.
func malformedDecisionEnvelopeError(
	result workers.WorkResult,
	response workers.RunnerExecutionResult,
) error {
	failureType := workers.WorkFailureTypeUnknown
	if result.FailureMetadata != nil && strings.TrimSpace(string(result.FailureMetadata.Type)) != "" {
		failureType = result.FailureMetadata.Type
	}
	normalized := workers.NewProviderError(
		failureType,
		boundedFailureMessage(result.Error),
		nil,
	)
	normalized.Continuation = cloneContinuation(response.Continuation)
	normalized.Diagnostics = workers.CloneWorkDiagnostics(response.Diagnostics)
	return normalized
}

// mergeDecisionEnvelopeDiagnostics layers the owner's failure facts over the
// provider diagnostics the attempt already produced, so the provider identity,
// duration, and response metadata survive alongside the envelope verdict.
func mergeDecisionEnvelopeDiagnostics(
	base *workers.WorkDiagnostics,
	envelope *workers.WorkDiagnostics,
) *workers.WorkDiagnostics {
	if envelope == nil {
		return base
	}
	merged := workers.CloneWorkDiagnostics(base)
	if merged == nil {
		return workers.CloneWorkDiagnostics(envelope)
	}
	overlay := workers.CloneWorkDiagnostics(envelope)
	if overlay.Provider != nil {
		if merged.Provider == nil {
			merged.Provider = overlay.Provider
		} else {
			if merged.Provider.ResponseMetadata == nil {
				merged.Provider.ResponseMetadata = make(map[string]string, len(overlay.Provider.ResponseMetadata))
			}
			for key, value := range overlay.Provider.ResponseMetadata {
				merged.Provider.ResponseMetadata[key] = value
			}
		}
	}
	if overlay.Metadata != nil {
		if merged.Metadata == nil {
			merged.Metadata = make(map[string]string, len(overlay.Metadata))
		}
		for key, value := range overlay.Metadata {
			merged.Metadata[key] = value
		}
	}
	return merged
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

func hasAgentCandidate(result providers.ExecuteResult) bool {
	return strings.TrimSpace(result.Content) != "" || result.Outcome != ""
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

// usableAgentResult is the only semantic success predicate in this runner.
// Providers may return a normalized result beside a later attempt error; a
// classified decision-envelope result or a native output that satisfies the
// configured stop policy is still the authoritative attempt outcome. A native
// REJECTED result means the stop token was absent, so partial output beside a
// provider error remains unusable instead of becoming a false success.
func usableAgentResult(
	response workers.RunnerExecutionResult,
	request workers.RunnerExecutionRequest,
) bool {
	switch response.Outcome {
	case workers.OutcomeAccepted, workers.OutcomeContinue:
		return true
	case workers.OutcomeRejected:
		return request.DecisionEnvelope || strings.EqualFold(
			strings.TrimSpace(request.OutputFormat),
			interfaces.DecisionEnvelopeOutcomeFormat,
		)
	case workers.OutcomeFailed, workers.OutcomeCanceled:
		return false
	default:
		return strings.TrimSpace(response.Content) != ""
	}
}

func preserveContinuation(
	response workers.RunnerExecutionResult,
	request workers.RunnerExecutionRequest,
	resumeReference *providers.SessionRef,
) workers.RunnerExecutionResult {
	if request.Continuation != nil {
		response.Continuation = cloneContinuation(request.Continuation)
	}
	if response.Continuation == nil && resumeReference != nil {
		continuation := resumeReference.ContinuationRef()
		response.Continuation = &continuation
	}
	if response.Continuation == nil && strings.TrimSpace(request.SessionID) != "" {
		continuation := (providers.SessionRef{
			Provider: providerIDForRequest(request),
			Kind:     providers.SessionIDKind,
			ID:       request.SessionID,
		}).ContinuationRef()
		response.Continuation = &continuation
	}
	return response
}

func (s *service) publishCompletedOnce(
	publication *terminalPublication,
	identity progressIdentity,
	continuation *workers.ProviderContinuationRef,
	provider string,
	diagnostics *workers.WorkDiagnostics,
) {
	fragment := workers.ProgressFragment{
		Correlation:       identity.correlation,
		DispatchID:        identity.dispatchID,
		Kind:              workers.CompletedFragmentKind,
		Type:              "COMPLETED",
		Provider:          provider,
		Continuation:      cloneContinuation(continuation),
		ExternalEventType: "STREAM_COMPLETED",
		Metadata:          terminalSuppressedMetadata(diagnostics),
	}
	if publication != nil {
		publication.terminal(fragment)
		return
	}
	s.publish(fragment)
}

func (s *service) publishAttemptFailure(
	ctx context.Context,
	publication *terminalPublication,
	identity progressIdentity,
	request workers.RunnerExecutionRequest,
	response workers.RunnerExecutionResult,
	attemptErr error,
	resumeReference *providers.SessionRef,
	provider string,
) (workers.RunnerExecutionResult, error) {
	if attemptErr == nil {
		attemptErr = errors.New(noUsableAgentResultMessage)
	}
	response = preserveContinuation(response, request, resumeReference)

	if continuationResponse, normalizedErr, handled := continuationFailureResult(ctx, request, attemptErr); handled {
		response = mergeFailureResponse(response, continuationResponse)
		markFailureOutcome(&response, normalizedErr)
		s.publishTerminalFailureOnce(
			publication,
			identity,
			normalizedErr,
			response.Continuation,
			resumeReference,
			"",
			provider,
		)
		return response, normalizedErr
	}
	if failure, ok := providerFailure(attemptErr); ok {
		response = mergeFailureResponse(response, runnerFailureResult(failure, request))
		normalizedErr := normalizeProviderFailure(ctx, failure, attemptErr, response)
		markFailureOutcome(&response, normalizedErr)
		s.publishFailureProgress(
			identity,
			failure,
			response.Continuation,
			provider,
		)
		s.publishTerminalFailureOnce(
			publication,
			identity,
			normalizedErr,
			response.Continuation,
			failure.SessionRef,
			failure.Message,
			provider,
		)
		return response, normalizedErr
	}

	var providerErr *workers.ProviderError
	if errors.As(attemptErr, &providerErr) && providerErr != nil {
		response = mergeFailureResponse(response, runnerResultFromProviderError(providerErr))
		markFailureOutcome(&response, providerErr)
		s.publishTerminalFailureOnce(
			publication,
			identity,
			providerErr,
			response.Continuation,
			nil,
			"",
			provider,
		)
		return response, providerErr
	}

	normalizedErr := normalizeExecutionError(ctx, attemptErr)
	markFailureOutcome(&response, normalizedErr)
	s.publishTerminalFailureOnce(
		publication,
		identity,
		normalizedErr,
		response.Continuation,
		nil,
		"",
		provider,
	)
	return response, normalizedErr
}

func mergeFailureResponse(
	response workers.RunnerExecutionResult,
	failure workers.RunnerExecutionResult,
) workers.RunnerExecutionResult {
	if failure.Continuation != nil {
		response.Continuation = cloneContinuation(failure.Continuation)
	}
	if response.Diagnostics == nil {
		response.Diagnostics = workers.CloneWorkDiagnostics(failure.Diagnostics)
	} else if failure.Diagnostics != nil {
		response.Diagnostics = mergeDecisionEnvelopeDiagnostics(response.Diagnostics, failure.Diagnostics)
	}
	if response.Outcome == "" {
		response.Outcome = failure.Outcome
	}
	if strings.TrimSpace(response.Content) == "" {
		response.Content = failure.Content
	}
	return response
}

func runnerResultFromProviderError(
	providerErr *workers.ProviderError,
) workers.RunnerExecutionResult {
	if providerErr == nil {
		return workers.RunnerExecutionResult{}
	}
	return workers.RunnerExecutionResult{
		Continuation: cloneContinuation(providerErr.Continuation),
		Diagnostics:  workers.CloneWorkDiagnostics(providerErr.Diagnostics),
	}
}

func markFailureOutcome(
	response *workers.RunnerExecutionResult,
	err error,
) {
	if response == nil || response.Outcome != "" {
		return
	}
	if errors.Is(err, context.Canceled) {
		response.Outcome = workers.OutcomeCanceled
		return
	}
	response.Outcome = workers.OutcomeFailed
}

func mergeSuppressedAttemptDiagnostics(
	base *workers.WorkDiagnostics,
	attemptErr error,
) *workers.WorkDiagnostics {
	if attemptErr == nil {
		return base
	}
	kind := suppressedAttemptErrorKind(attemptErr)
	stage := suppressedAttemptFailureStage(attemptErr)
	merged := workers.CloneWorkDiagnostics(base)
	if merged == nil {
		merged = &workers.WorkDiagnostics{}
	}
	if merged.Metadata == nil {
		merged.Metadata = make(map[string]string, 2)
	}
	merged.Metadata[suppressedTerminalErrorKindMetadata] = kind
	if stage != "" {
		merged.Metadata[workers.ProviderResponseMetadataFailureStage] = stage
	}
	if merged.Provider == nil {
		merged.Provider = &workers.ProviderDiagnostic{}
	}
	if merged.Provider.ResponseMetadata == nil {
		merged.Provider.ResponseMetadata = make(map[string]string, 2)
	}
	merged.Provider.ResponseMetadata[suppressedTerminalErrorKindMetadata] = kind
	if stage != "" {
		merged.Provider.ResponseMetadata[workers.ProviderResponseMetadataFailureStage] = stage
	}
	return merged
}

func terminalSuppressedMetadata(
	diagnostics *workers.WorkDiagnostics,
) map[string]string {
	if diagnostics == nil {
		return nil
	}
	metadata := make(map[string]string, 2)
	if diagnostics.Metadata != nil {
		if value := strings.TrimSpace(diagnostics.Metadata[suppressedTerminalErrorKindMetadata]); value != "" {
			metadata[suppressedTerminalErrorKindMetadata] = value
		}
		if value := strings.TrimSpace(diagnostics.Metadata[workers.ProviderResponseMetadataFailureStage]); value != "" {
			metadata[workers.ProviderResponseMetadataFailureStage] = value
		}
	}
	if diagnostics.Provider != nil && diagnostics.Provider.ResponseMetadata != nil {
		for _, key := range []string{
			suppressedTerminalErrorKindMetadata,
			workers.ProviderResponseMetadataFailureStage,
		} {
			if _, exists := metadata[key]; exists {
				continue
			}
			if value := strings.TrimSpace(diagnostics.Provider.ResponseMetadata[key]); value != "" {
				metadata[key] = value
			}
		}
	}
	if len(metadata) == 0 {
		return nil
	}
	return metadata
}

func suppressedAttemptErrorKind(err error) string {
	if failure, ok := providerFailure(err); ok {
		return boundedExecuteFailureKind(failure.Kind)
	}
	var providerErr *workers.ProviderError
	if errors.As(err, &providerErr) && providerErr != nil && providerErr.ProviderFailureKind != "" {
		return boundedExecuteFailureKind(providerErr.ProviderFailureKind)
	}
	if errors.Is(err, context.Canceled) {
		return string(providers.ExecuteFailureKindCanceled)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return string(providers.ExecuteFailureKindTimeout)
	}
	return string(providers.ExecuteFailureKindUnknown)
}

func boundedExecuteFailureKind(kind providers.ExecuteFailureKind) string {
	switch kind {
	case providers.ExecuteFailureKindCanceled,
		providers.ExecuteFailureKindTimeout,
		providers.ExecuteFailureKindAuthentication,
		providers.ExecuteFailureKindInvalidRequest,
		providers.ExecuteFailureKindMisconfigured,
		providers.ExecuteFailureKindThrottled,
		providers.ExecuteFailureKindDependency,
		providers.ExecuteFailureKindCapabilityMismatch,
		providers.ExecuteFailureKindSessionNotFound:
		return string(kind)
	case providers.ExecuteFailureKindUnknown:
		fallthrough
	default:
		return string(providers.ExecuteFailureKindUnknown)
	}
}

func suppressedAttemptFailureStage(err error) string {
	stage := ""
	if failure, ok := providerFailure(err); ok && failure.Diagnostics != nil {
		stage = failure.Diagnostics.Metadata[workers.ProviderResponseMetadataFailureStage]
	}
	if stage == "" {
		var providerErr *workers.ProviderError
		if errors.As(err, &providerErr) && providerErr != nil && providerErr.Diagnostics != nil {
			if providerErr.Diagnostics.Metadata != nil {
				stage = providerErr.Diagnostics.Metadata[workers.ProviderResponseMetadataFailureStage]
			}
			if stage == "" && providerErr.Diagnostics.Provider != nil {
				stage = providerErr.Diagnostics.Provider.ResponseMetadata[workers.ProviderResponseMetadataFailureStage]
			}
		}
	}
	return boundedFailureStage(stage)
}

func boundedFailureStage(stage string) string {
	switch strings.ToLower(strings.TrimSpace(stage)) {
	case "native", "decode", "flush", "final_parse", "process", "stream", "teardown":
		return strings.ToLower(strings.TrimSpace(stage))
	default:
		return ""
	}
}

func (s *service) publishTerminalFailureOnce(
	publication *terminalPublication,
	identity progressIdentity,
	err error,
	continuation *workers.ProviderContinuationRef,
	reference *providers.SessionRef,
	providerMessage string,
	provider string,
) {
	s.publishTerminalFailureTo(
		publication,
		identity,
		err,
		continuation,
		reference,
		providerMessage,
		provider,
	)
}

func (s *service) publishTerminalFailureTo(
	publication *terminalPublication,
	identity progressIdentity,
	err error,
	continuation *workers.ProviderContinuationRef,
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
	if strings.TrimSpace(providerMessage) != "" &&
		!errors.Is(err, context.Canceled) &&
		!hasUnrecognizedProviderRefusalMarker(providerErr) {
		message = providerMessage
	}
	if len(metadata) == 0 {
		metadata = nil
	}
	fragment := workers.ProgressFragment{
		Correlation:       identity.correlation,
		DispatchID:        identity.dispatchID,
		Kind:              workers.FailedFragmentKind,
		Type:              eventType,
		Payload:           boundedFailureMessage(message),
		Provider:          provider,
		Continuation:      cloneContinuation(continuation),
		ExternalEventType: "STREAM_FAILED",
		Metadata:          metadata,
	}
	if publication != nil {
		publication.terminal(fragment)
		return
	}
	s.publish(fragment)
}

func (s *service) publishFailureProgress(
	identity progressIdentity,
	failure providers.ExecuteFailure,
	continuation *workers.ProviderContinuationRef,
	provider string,
) {
	if failure.Diagnostics == nil {
		return
	}
	s.publishProgress(identity, providers.ExecuteResult{
		SessionRef:  cloneSessionRef(failure.SessionRef),
		Diagnostics: failure.Diagnostics,
	}, continuation, provider)
}

func (s *service) publishProgress(
	identity progressIdentity,
	result providers.ExecuteResult,
	continuation *workers.ProviderContinuationRef,
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
			s.publishProviderProgress(identity, progress, continuation, provider)
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
		s.publishProviderProgress(identity, progress, continuation, provider)
	}
}

func (s *service) publishProviderProgress(
	identity progressIdentity,
	progress providers.ExecuteProgress,
	continuation *workers.ProviderContinuationRef,
	provider string,
) {
	s.publish(workers.ProgressFragment{
		Correlation:  identity.correlation,
		DispatchID:   identity.dispatchID,
		Kind:         workers.ProgressFragmentKind,
		Type:         progress.Phase,
		Payload:      progress.Detail,
		Provider:     provider,
		Continuation: cloneContinuation(continuation),
		Metadata:     cloneMetadata(progress.Metadata),
	})
}

func validateRequest(request workers.RunnerExecutionRequest) error {
	if request.Continuation == nil {
		if err := providers.ID(request.RunnerID).Validate(); err != nil {
			return badRequest("agent provider identity is invalid", err)
		}
	} else if request.Continuation != nil {
		if _, err := request.Continuation.ToSessionRef(); err != nil {
			return invalidContinuationRequestError(request.Continuation, err)
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

func invalidContinuationRequestError(
	continuation *workers.ProviderContinuationRef,
	cause error,
) *workers.ProviderError {
	normalized := continuation.Normalize()
	identity := strings.TrimSpace(normalized.ProviderSessionID)
	if identity == "" {
		identity = strings.TrimSpace(normalized.ExternalRef)
	}
	failure := providers.ContinuationFailure{
		Kind:    providers.ContinuationFailureKindInvalid,
		Message: cause.Error(),
		Reference: providers.SessionRef{
			Provider: providers.ID(normalized.Provider),
			Kind:     normalized.Kind,
			ID:       identity,
		},
	}
	result := workers.NewProviderError(
		workers.WorkFailureTypePermanentBadRequest,
		"agent provider continuation is invalid",
		failure,
	)
	result.ProviderContinuationFailureKind = providers.ContinuationFailureKindInvalid
	result.Continuation = cloneContinuation(continuation)
	return result
}

// executeProviderAttempt runs one provider attempt for request. An admitted
// opaque continuation is passed unchanged to Providers.ContinueReference;
// legacy configuration SessionID resumes retain their established compatibility
// path. Providers.Execute is used only when neither continuation value is
// present.
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
	if request.Continuation != nil {
		reference, err := request.Continuation.ToSessionRef()
		if err != nil {
			normalized := request.Continuation.Normalize()
			identity := strings.TrimSpace(normalized.ProviderSessionID)
			if identity == "" {
				identity = strings.TrimSpace(normalized.ExternalRef)
			}
			return providers.ExecuteResult{}, providers.ContinuationFailure{
				Kind:    providers.ContinuationFailureKindInvalid,
				Message: err.Error(),
				Reference: providers.SessionRef{
					Provider: providers.ID(normalized.Provider),
					Kind:     normalized.Kind,
					ID:       identity,
				},
			}
		}
		continued, err := s.providers.ContinueReference(ctx, providers.ContinueReferenceRequest{
			Reference: request.Continuation.Clone(),
			Attempt:   attempt,
		})
		if err != nil {
			return providers.ExecuteResult{}, err
		}
		if continued.Outcome == providers.ContinuationOutcomeUnsupported {
			return providers.ExecuteResult{}, continuationUnsupportedError{reference: reference}
		}
		continuedReference, referenceErr := continued.Reference.ToSessionRef()
		if referenceErr != nil {
			return providers.ExecuteResult{}, referenceErr
		}
		return continuedExecuteResultFromOpaque(continued, reference, continuedReference)
	}
	if strings.TrimSpace(request.SessionID) == "" {
		return s.providers.Execute(ctx, attempt)
	}
	reference := providers.SessionRef{
		Provider: attempt.Provider,
		Kind:     providers.SessionIDKind,
		ID:       request.SessionID,
	}
	continued, err := continueLegacyProvider(ctx, s.providers, providers.ContinueRequest{
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
