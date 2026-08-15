package providersroot

import (
	"context"
	"errors"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerprovider "github.com/portpowered/infinite-you/pkg/services/workers/providercompat"
	"github.com/portpowered/infinite-you/pkg/services/workers/providercompat/conductor"
	inference "github.com/portpowered/infinite-you/pkg/services/workers/providercompat/inferencecontract"
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
	normalized := workers.NormalizeRunnerID(providerID)
	switch normalized {
	case workers.RunnerIDAntigravity:
		return "antigravity"
	default:
		return normalized
	}
}

func invocationRequestFromExecute(
	request providers.ExecuteRequest,
	configuredSkipPermissions bool,
) inference.InvocationRequest {
	skipPermissions := request.SkipPermissions || configuredSkipPermissions
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
		ReasoningEffort:    canonicalReasoningEffort(request.ReasoningEffort),
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
	required := inference.NewCapabilitySet(inference.CapabilityPromptSubmission)
	if skipPermissions {
		required = inference.NewCapabilitySet(
			inference.CapabilityPromptSubmission,
			inference.CapabilityPermissionBypass,
		)
	}
	return inference.NewInvocationRequest(inference.InvocationInput{
		InvocationID: invocationID,
		Model:        execution.Model,
		SystemPrompt: execution.SystemPrompt,
		UserMessage:  execution.UserMessage,
		OutputSchema: execution.OutputSchema,
		Required:     required,
		Execution:    execution,
	})
}

func mapConductorInvokeError(err error) error {
	if err == nil {
		return nil
	}
	var rejection *conductor.Rejection
	if errors.As(err, &rejection) {
		kind := providers.ExecuteFailureKindInvalidRequest
		if rejection.Invariant() == conductor.InvariantCapabilityEscalation {
			kind = providers.ExecuteFailureKindCapabilityMismatch
		}
		return providers.ExecuteFailure{
			Kind:    kind,
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
	result := providers.ExecuteResult{
		Content: response.Content(),
		Diagnostics: &providers.ExecuteDiagnostics{Metadata: map[string]string{
			workers.ProviderResponseMetadataCompletionEvidence: "provider_response",
		}},
	}
	if session := response.ProviderSession(); session != nil {
		result.SessionRef = &providers.SessionRef{
			Provider: providerID,
			Kind:     session.Kind(),
			ID:       session.ID(),
		}
	}
	if metadata := response.Metadata(); len(metadata) > 0 {
		result.Diagnostics = &providers.ExecuteDiagnostics{
			Metadata: cloneMetadata(metadata),
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
				providers.CanonicalProviderSessionProvider(session.Provider()),
			),
			Kind: session.Kind(),
			ID:   session.ID(),
		}
		if metadata == nil {
			metadata = make(map[string]string, 3)
		}
		metadata["provider_session_provider"] = providers.CanonicalProviderSessionProvider(session.Provider())
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
