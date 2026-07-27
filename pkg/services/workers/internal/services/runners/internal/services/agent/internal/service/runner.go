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
		if contextErr := ctx.Err(); contextErr != nil {
			return workers.RunnerExecutionResult{}, contextErr
		}
		return workers.RunnerExecutionResult{}, normalizeExecutionError(err)
	}
	result = result.Clone()
	response := runnerResult(result, providers.ID(request.RunnerID))
	s.publishProgress(request.Dispatch.DispatchID, result, response.ProviderSession)
	return response, nil
}

func (s *service) publishProgress(
	dispatchID string,
	result providers.ExecuteResult,
	session *workers.ProviderSessionMetadata,
) {
	if result.Diagnostics == nil {
		return
	}
	for _, progress := range result.Diagnostics.Progress {
		s.publish(workers.ProgressFragment{
			DispatchID:         dispatchID,
			Kind:               workers.ProgressFragmentKind,
			Type:               progress.Phase,
			Payload:            progress.Detail,
			ProviderSessionRef: workers.CloneProviderSessionMetadata(session),
			Metadata:           cloneMetadata(progress.Metadata),
		})
	}
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
	for _, capability := range request.RequiredOptionalCapabilities {
		if capability == workers.RunnerOptionalCapabilityImageInput {
			return &workers.UnsupportedRunnerCapabilityError{
				RunnerID:   request.RunnerID,
				Capability: capability,
			}
		}
	}
	return nil
}

func providerRequest(request workers.RunnerExecutionRequest) providers.ExecuteRequest {
	providerID := providers.ID(request.RunnerID)
	result := providers.ExecuteRequest{
		Provider:         providerID,
		AttemptID:        request.Dispatch.DispatchID,
		SystemPrompt:     request.SystemPrompt,
		UserMessage:      request.UserMessage,
		OutputSchema:     request.OutputSchema,
		WorkingDirectory: request.WorkingDirectory,
		Worktree:         request.Worktree,
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

func runnerResult(
	result providers.ExecuteResult,
	providerID providers.ID,
) workers.RunnerExecutionResult {
	result = result.Clone()
	response := workers.RunnerExecutionResult{Content: result.Content}
	if result.SessionRef != nil {
		response.ProviderSession = &workers.ProviderSessionMetadata{
			Provider: result.SessionRef.Provider.String(),
			Kind:     result.SessionRef.Kind,
			ID:       result.SessionRef.ID,
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

func normalizeExecutionError(err error) error {
	var failure providers.ExecuteFailure
	if errors.As(err, &failure) {
		return workers.NewProviderError(
			workers.WorkFailureTypeInternalServerError,
			strings.TrimSpace(failure.Message),
			err,
		)
	}
	return workers.NewProviderError(
		workers.WorkFailureTypeInternalServerError,
		"agent provider execution failed",
		err,
	)
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
