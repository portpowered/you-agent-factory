package service

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/internal/services/agent"
)

type service struct {
	providers providers.Service
	publish   workers.ProgressPublisher
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
	if err := ctx.Err(); err != nil {
		return workers.RunnerExecutionResult{}, err
	}
	request = workers.CloneProviderInferenceRequest(request)
	if err := validateRequest(request); err != nil {
		return workers.RunnerExecutionResult{}, err
	}
	result, err := s.providers.Execute(ctx, providerRequest(request))
	if err != nil {
		if failure, ok := providerFailure(err); ok {
			response := runnerFailureResult(failure, request)
			normalizedErr := normalizeProviderFailure(ctx, failure, err, response)
			s.publishFailureProgress(
				request.Dispatch.DispatchID,
				failure,
				response.ProviderSession,
			)
			s.publishTerminalFailure(
				request.Dispatch.DispatchID,
				normalizedErr,
				response.ProviderSession,
				failure.Message,
			)
			return response, normalizedErr
		}
		normalizedErr := normalizeExecutionError(ctx, err)
		s.publishTerminalFailure(request.Dispatch.DispatchID, normalizedErr, nil, "")
		return workers.RunnerExecutionResult{}, normalizedErr
	}
	result = result.Clone()
	response := runnerResult(result, providerIDForRunner(request.RunnerID))
	if response.ProviderSession == nil && strings.TrimSpace(request.SessionID) != "" {
		response.ProviderSession = &workers.ProviderSessionMetadata{
			Provider: workers.CanonicalProviderSessionProvider(providerIDForRunner(request.RunnerID).String()),
			Kind:     providers.SessionIDKind,
			ID:       request.SessionID,
		}
	}
	s.publishProgress(request.Dispatch.DispatchID, result, response.ProviderSession)
	if !hasTerminalRunProgress(result.Diagnostics) {
		s.publish(workers.ProgressFragment{
			DispatchID:         request.Dispatch.DispatchID,
			Kind:               workers.CompletedFragmentKind,
			Type:               "COMPLETED",
			ProviderSessionRef: workers.CloneProviderSessionMetadata(response.ProviderSession),
			ExternalEventType:  "STREAM_COMPLETED",
		})
	}
	return response, nil
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
	dispatchID string,
	err error,
	session *workers.ProviderSessionMetadata,
	providerMessage string,
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
		DispatchID:         dispatchID,
		Kind:               workers.FailedFragmentKind,
		Type:               eventType,
		Payload:            boundedFailureMessage(message),
		ProviderSessionRef: workers.CloneProviderSessionMetadata(session),
		ExternalEventType:  "STREAM_FAILED",
		Metadata:           metadata,
	})
}

func (s *service) publishFailureProgress(
	dispatchID string,
	failure providers.ExecuteFailure,
	session *workers.ProviderSessionMetadata,
) {
	if failure.Diagnostics == nil {
		return
	}
	s.publishProgress(dispatchID, providers.ExecuteResult{
		Diagnostics: failure.Diagnostics,
	}, session)
}

func (s *service) publishProgress(
	dispatchID string,
	result providers.ExecuteResult,
	session *workers.ProviderSessionMetadata,
) {
	var terminalMessages []providers.ExecuteProgress
	if result.Diagnostics != nil {
		for _, progress := range result.Diagnostics.Progress {
			if strings.EqualFold(strings.TrimSpace(progress.Phase), "message.completed") {
				terminalMessages = append(terminalMessages, progress)
				continue
			}
			s.publishProviderProgress(dispatchID, progress, session)
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
		s.publishProviderProgress(dispatchID, progress, session)
	}
}

func (s *service) publishProviderProgress(
	dispatchID string,
	progress providers.ExecuteProgress,
	session *workers.ProviderSessionMetadata,
) {
	s.publish(workers.ProgressFragment{
		DispatchID:         dispatchID,
		Kind:               workers.ProgressFragmentKind,
		Type:               progress.Phase,
		Payload:            progress.Detail,
		ProviderSessionRef: workers.CloneProviderSessionMetadata(session),
		Metadata:           cloneMetadata(progress.Metadata),
	})
}

func validateRequest(request workers.RunnerExecutionRequest) error {
	if err := providers.ID(request.RunnerID).Validate(); err != nil {
		return badRequest("agent provider identity is invalid", err)
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

func providerRequest(request workers.RunnerExecutionRequest) providers.ExecuteRequest {
	providerID := providerIDForRunner(request.RunnerID)
	result := providers.ExecuteRequest{
		Provider:           providerID,
		AttemptID:          request.Dispatch.DispatchID,
		WorkerType:         request.WorkerType,
		WorkstationName:    request.WorkstationType,
		Model:              request.Model,
		SkipPermissions:    request.SkipPermissions,
		SystemPrompt:       request.SystemPrompt,
		UserMessage:        request.UserMessage,
		InputTokens:        cloneInputTokens(request.InputTokens),
		OutputSchema:       request.OutputSchema,
		WorkingDirectory:   request.WorkingDirectory,
		Worktree:           request.Worktree,
		EnvVars:            cloneMetadata(request.EnvVars),
		ProcessEnvironment: append([]string(nil), request.ProcessEnvironment...),
	}
	if strings.TrimSpace(request.SessionID) != "" {
		result.ResumeSession = &providers.SessionRef{
			Provider: providerID,
			Kind:     providers.SessionIDKind,
			ID:       request.SessionID,
		}
	}
	return result
}

// providerIDForRunner translates stable Workers runner identities at the
// Providers boundary.
func providerIDForRunner(runnerID string) providers.ID {
	return providers.ID(workers.NormalizeRunnerID(runnerID))
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
	}, providerIDForRunner(request.RunnerID))
	if response.ProviderSession == nil && failure.SessionRef != nil {
		response.ProviderSession = &workers.ProviderSessionMetadata{
			Provider: workers.CanonicalProviderSessionProvider(
				failure.SessionRef.Provider.String(),
			),
			Kind: failure.SessionRef.Kind,
			ID:   failure.SessionRef.ID,
		}
	}
	if strings.TrimSpace(request.SessionID) != "" {
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
	normalized.ProviderSession = workers.CloneProviderSessionMetadata(
		result.ProviderSession,
	)
	normalized.Diagnostics = workers.CloneWorkDiagnostics(result.Diagnostics)
	return normalized
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
