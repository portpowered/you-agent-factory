package opencode

import (
	"context"
	"errors"
	"strings"

	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	inference "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
)

// IntegrationDependencies are the Providers collaborators used by OpenCode's
// registry-backed integration on the neutral conductor path.
type IntegrationDependencies struct {
	ProvidersService providers.Service
}

// Integration routes OpenCode through the Providers root execution boundary.
type Integration struct {
	providers providers.Service
}

// NewIntegration constructs OpenCode's registry-backed integration.
func NewIntegration(deps ...IntegrationDependencies) *Integration {
	integration := &Integration{}
	if len(deps) > 0 {
		integration.providers = deps[0].ProvidersService
	}
	return integration
}

func (*Integration) Identity() inference.Identity {
	return inference.Identity(modelprovider.ProviderOpenCode)
}

func (*Integration) MaximumCapabilities() inference.CapabilitySet {
	return inference.NewCapabilitySet(
		inference.CapabilityPromptSubmission,
		inference.CapabilitySessionResume,
		inference.CapabilityNativeStreaming,
		inference.CapabilityMessageSnapshots,
		inference.CapabilityUsage,
		inference.CapabilityStableItemIDs,
	)
}

func (*Integration) Discover(context.Context) (inference.Discovery, error) {
	return inference.NewDiscovery(inference.ReadinessReady), nil
}

func (i *Integration) Capabilities(context.Context, inference.InvocationRequest) (inference.CapabilitySet, error) {
	return i.MaximumCapabilities(), nil
}

// Invoke executes OpenCode through providers.Service and publishes progress
// before closing with exactly one terminal outcome.
func (i *Integration) Invoke(
	ctx context.Context,
	request inference.InvocationRequest,
	writer inference.ResponseWriter,
) error {
	if i == nil || i.providers == nil {
		failure := inference.NewFailure(inference.FailureInput{
			Kind:    inference.FailureDependency,
			Message: "OpenCode Providers service is unavailable",
		})
		if err := writeFailureProgress(ctx, writer, request.InvocationID(), failure); err != nil {
			return err
		}
		return writer.Close(ctx, inference.FailedCompletion(failure))
	}
	result, err := i.providers.Execute(ctx, executeRequestFromInvocation(request))
	if errors.Is(ctx.Err(), context.Canceled) && !errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return ctx.Err()
	}
	if err != nil {
		failure := failureFromExecuteError(request, err)
		if writeErr := writeFailureProgress(ctx, writer, request.InvocationID(), failure); writeErr != nil {
			return writeErr
		}
		return writer.Close(ctx, inference.FailedCompletion(failure))
	}
	if err := writeProgressEvents(
		ctx,
		writer,
		request.InvocationID(),
		result.Diagnostics,
		result.Content,
	); err != nil {
		return err
	}
	return writer.Close(ctx, inference.SuccessfulCompletion(inference.NewResponse(inference.ResponseInput{
		Content:         result.Content,
		ProviderSession: sessionRefToInference(result.SessionRef),
	})))
}

func executeRequestFromInvocation(request inference.InvocationRequest) providers.ExecuteRequest {
	execution := request.Execution()
	executeRequest := providers.ExecuteRequest{
		Provider:           providers.IDOpenCode,
		AttemptID:          request.InvocationID(),
		Model:              request.Model(),
		SkipPermissions:    execution.SkipPermissions,
		SystemPrompt:       request.SystemPrompt(),
		UserMessage:        request.UserMessage(),
		OutputSchema:       request.OutputSchema(),
		WorkingDirectory:   execution.WorkingDirectory,
		Worktree:           execution.Worktree,
		ProcessEnvironment: append([]string(nil), execution.ProcessEnvironment...),
		EnvVars:            cloneStringMap(execution.EnvVars),
		WorkerType:         workerNameFromExecution(execution),
		WorkstationName:    workstationNameFromExecution(execution),
	}
	if agent := strings.TrimSpace(execution.OpenCodeAgent); agent != "" {
		if executeRequest.EnvVars == nil {
			executeRequest.EnvVars = map[string]string{}
		}
		executeRequest.EnvVars["opencode_agent"] = agent
	}
	if session := requestedSession(request); session != nil {
		executeRequest.ResumeSession = session
	}
	return executeRequest
}

func workerNameFromExecution(execution workers.ProviderInferenceRequest) string {
	if execution.WorkerType != "" {
		return execution.WorkerType
	}
	return execution.Dispatch.WorkerType
}

func workstationNameFromExecution(execution workers.ProviderInferenceRequest) string {
	if execution.WorkstationType != "" {
		return execution.WorkstationType
	}
	return execution.Dispatch.WorkstationName
}

func requestedSession(request inference.InvocationRequest) *providers.SessionRef {
	if session := request.ProviderSession(); session != nil &&
		strings.TrimSpace(session.Provider()) == string(providers.IDOpenCode) &&
		strings.TrimSpace(session.Kind()) == providers.SessionIDKind {
		return &providers.SessionRef{
			Provider: providers.IDOpenCode,
			Kind:     providers.SessionIDKind,
			ID:       strings.TrimSpace(session.ID()),
		}
	}
	if sessionID := strings.TrimSpace(request.Execution().SessionID); sessionID != "" {
		return &providers.SessionRef{
			Provider: providers.IDOpenCode,
			Kind:     providers.SessionIDKind,
			ID:       sessionID,
		}
	}
	return nil
}

func failureFromExecuteError(request inference.InvocationRequest, err error) inference.Failure {
	var failure providers.ExecuteFailure
	if errors.As(err, &failure) {
		return inference.NewFailure(inference.FailureInput{
			Kind:            executeFailureKind(failure.Kind),
			Message:         failure.Message,
			ProviderSession: failureSessionForInvocation(request, failure.SessionRef),
		})
	}
	if errors.Is(err, providers.ErrExecuteCancelled) {
		return inference.NewFailure(inference.FailureInput{
			Kind:    inference.FailureCanceled,
			Message: "provider invocation was canceled",
		})
	}
	if errors.Is(err, providers.ErrExecuteTimeout) {
		return inference.NewFailure(inference.FailureInput{
			Kind:    inference.FailureTimeout,
			Message: "provider execution timed out",
		})
	}
	return inference.NewFailure(inference.FailureInput{
		Kind:            inference.FailureUnknown,
		Message:         "OpenCode invocation failed.",
		ProviderSession: failureSessionForInvocation(request, nil),
	})
}

func executeFailureKind(kind providers.ExecuteFailureKind) inference.FailureKind {
	switch kind {
	case providers.ExecuteFailureKindCanceled:
		return inference.FailureCanceled
	case providers.ExecuteFailureKindTimeout:
		return inference.FailureTimeout
	case providers.ExecuteFailureKindAuthentication:
		return inference.FailureAuthentication
	case providers.ExecuteFailureKindInvalidRequest:
		return inference.FailureInvalidRequest
	case providers.ExecuteFailureKindThrottled:
		return inference.FailureThrottled
	case providers.ExecuteFailureKindDependency:
		return inference.FailureDependency
	default:
		return inference.FailureUnknown
	}
}

func sessionRefFromResult(ref *providers.SessionRef) *inference.ProviderSession {
	return sessionRefToInference(ref)
}

func sessionRefToInference(ref *providers.SessionRef) *inference.ProviderSession {
	if ref == nil || strings.TrimSpace(ref.ID) == "" {
		return nil
	}
	session := inference.NewProviderSession(string(ref.Provider), ref.Kind, ref.ID, nil)
	return &session
}

func failureSessionForInvocation(
	request inference.InvocationRequest,
	ref *providers.SessionRef,
) *inference.ProviderSession {
	if session := sessionRefToInference(ref); session != nil {
		return session
	}
	return sessionRefToInference(requestedSession(request))
}

func cloneStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

var _ inference.Integration = (*Integration)(nil)
