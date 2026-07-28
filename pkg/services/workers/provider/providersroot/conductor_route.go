package providersroot

import (
	"context"
	"errors"
	"strings"

	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerrunner "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/runner"
	workerprovider "github.com/portpowered/infinite-you/pkg/services/workers/provider"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/conductor"
	inference "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
)

func (s *Service) shouldRouteConductor(providerID string) bool {
	if s == nil || s.config.Conductor == nil || s.config.ProviderRegistry == nil {
		return false
	}
	identity := conductorIdentity(providerID)
	return !s.config.ProviderRegistry.UsesNativeRunner(identity)
}

func (s *Service) executeViaConductor(
	ctx context.Context,
	request providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	identity := conductorIdentity(request.Provider.String())
	destination := &conductorDestination{
		dispatchID: request.AttemptID,
		publish:    s.config.Publish,
	}
	err := s.config.Conductor.Invoke(
		ctx,
		identity,
		invocationRequestFromExecute(request, s.config.SkipPermissions),
		destination,
	)
	if err != nil {
		return providers.ExecuteResult{}, mapConductorInvokeError(err)
	}
	return destination.result(request.Provider)
}

func conductorIdentity(providerID string) string {
	normalized := workerrunner.NormalizeRunnerID(providerID)
	switch normalized {
	case workers.RunnerIDCursorCLI, "cursor":
		return "cursor"
	case workers.RunnerIDKiro, string(modelprovider.ProviderKiro):
		return "kiro"
	case workers.RunnerIDGemini:
		return "gemini"
	case workers.RunnerIDAgy:
		return "agy"
	default:
		return normalized
	}
}

func invocationRequestFromExecute(
	request providers.ExecuteRequest,
	skipPermissions bool,
) inference.InvocationRequest {
	invocationID := strings.TrimSpace(request.AttemptID)
	if invocationID == "" {
		invocationID = "providers-root-invocation"
	}
	execution := workers.RunnerExecutionRequest{
		Dispatch: work.WorkDispatch{
			DispatchID:      invocationID,
			WorkerType:      strings.TrimSpace(request.WorkerType),
			WorkstationName: strings.TrimSpace(request.WorkstationName),
		},
		RunnerID:           request.Provider.String(),
		WorkerType:         strings.TrimSpace(request.WorkerType),
		WorkstationType:    strings.TrimSpace(request.WorkstationName),
		Model:              strings.TrimSpace(request.Model),
		SystemPrompt:       request.SystemPrompt,
		UserMessage:        request.UserMessage,
		InputTokens:        cloneInputTokens(request.InputTokens),
		OutputSchema:       request.OutputSchema,
		WorkingDirectory:   request.WorkingDirectory,
		Worktree:           request.Worktree,
		EnvVars:            cloneMetadata(request.EnvVars),
		ProcessEnvironment: append([]string(nil), request.ProcessEnvironment...),
		SkipPermissions:    skipPermissions,
	}
	if request.ResumeSession != nil {
		execution.SessionID = request.ResumeSession.ID
	}
	return inference.NewInvocationRequest(inference.InvocationInput{
		InvocationID: invocationID,
		Model:        execution.Model,
		SystemPrompt: execution.SystemPrompt,
		UserMessage:  execution.UserMessage,
		OutputSchema: execution.OutputSchema,
		Required:     inference.NewCapabilitySet(inference.CapabilityPromptSubmission),
		Execution:    execution,
	})
}

func mapConductorInvokeError(err error) error {
	if err == nil {
		return nil
	}
	var rejection *conductor.Rejection
	if errors.As(err, &rejection) {
		return providers.ExecuteFailure{
			Kind:    providers.ExecuteFailureKindInvalidRequest,
			Message: rejection.Error(),
		}
	}
	return providers.ExecuteFailure{
		Kind:    providers.ExecuteFailureKindUnknown,
		Message: err.Error(),
	}
}

type conductorDestination struct {
	completion *inference.Completion
	dispatchID string
	publish    workerprovider.InferenceProgressPublisher
}

func (d *conductorDestination) WriteEvent(_ context.Context, event inference.EventDraft) error {
	if d != nil && d.publish != nil {
		draft := event.Draft()
		draft.DispatchID = d.dispatchID
		d.publish(workerprovider.CanonicalDraftFragment(draft.DispatchID, draft))
	}
	return nil
}

func (d *conductorDestination) Close(_ context.Context, completion inference.Completion) error {
	clone := completion
	d.completion = &clone
	return nil
}

func (d *conductorDestination) result(providerID providers.ID) (providers.ExecuteResult, error) {
	if d == nil || d.completion == nil {
		return providers.ExecuteResult{}, providers.ExecuteFailure{
			Kind:    providers.ExecuteFailureKindUnknown,
			Message: "provider invocation completed without a safe terminal outcome",
		}
	}
	if failure := d.completion.Failure(); failure != nil {
		return providers.ExecuteResult{}, executeFailureFromConductor(*failure)
	}
	response := d.completion.Response()
	if response == nil {
		return providers.ExecuteResult{}, providers.ExecuteFailure{
			Kind:    providers.ExecuteFailureKindUnknown,
			Message: "provider invocation completed without a safe terminal outcome",
		}
	}
	result := providers.ExecuteResult{Content: response.Content()}
	if session := response.ProviderSession(); session != nil {
		result.SessionRef = &providers.SessionRef{
			Provider: providerID,
			Kind:     session.Kind(),
			ID:       session.ID(),
		}
	}
	return result, nil
}

func executeFailureFromConductor(failure inference.Failure) providers.ExecuteFailure {
	executeFailure := providers.ExecuteFailure{
		Kind:    executeFailureKindFromConductor(failure),
		Message: failure.Message(),
	}
	metadata := cloneMetadata(failure.Diagnostics())
	if session := failure.ProviderSession(); session != nil {
		executeFailure.SessionRef = &providers.SessionRef{
			Provider: providers.ID(
				workers.CanonicalProviderSessionProvider(session.Provider()),
			),
			Kind: session.Kind(),
			ID:   session.ID(),
		}
		if metadata == nil {
			metadata = make(map[string]string, 3)
		}
		metadata["provider_session_provider"] = workers.CanonicalProviderSessionProvider(session.Provider())
		metadata["provider_session_kind"] = session.Kind()
		metadata["provider_session_id"] = session.ID()
	}
	if len(metadata) > 0 {
		executeFailure.Diagnostics = &providers.ExecuteDiagnostics{Metadata: metadata}
	}
	return executeFailure
}

func executeFailureKindFromConductor(failure inference.Failure) providers.ExecuteFailureKind {
	switch failure.Kind() {
	case inference.FailureTimeout:
		return providers.ExecuteFailureKindTimeout
	case inference.FailureThrottled:
		return providers.ExecuteFailureKindThrottled
	case inference.FailureAuthentication:
		return providers.ExecuteFailureKindAuthentication
	case inference.FailureInvalidRequest, inference.FailureMalformedOutput:
		return providers.ExecuteFailureKindInvalidRequest
	case inference.FailureDependency:
		return providers.ExecuteFailureKindDependency
	case inference.FailureCanceled:
		return providers.ExecuteFailureKindCanceled
	default:
		if failure.Retryable() {
			return providers.ExecuteFailureKindDependency
		}
		return providers.ExecuteFailureKindUnknown
	}
}
