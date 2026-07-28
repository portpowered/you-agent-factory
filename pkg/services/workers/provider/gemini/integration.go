package gemini

import (
	"context"
	"errors"
	"strings"

	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	inference "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
)

// IntegrationDependencies are the Providers collaborators used by Gemini's
// registry-backed integration on the neutral conductor path.
type IntegrationDependencies struct {
	ProvidersService providers.Service
}

// Integration routes Gemini through the Providers root execution boundary.
type Integration struct {
	providers providers.Service
}

// NewIntegration constructs Gemini's registry-backed integration.
func NewIntegration(deps ...IntegrationDependencies) *Integration {
	integration := &Integration{}
	if len(deps) > 0 {
		integration.providers = deps[0].ProvidersService
	}
	return integration
}

func (*Integration) Identity() inference.Identity {
	return inference.Identity(modelprovider.ProviderGemini)
}

func (*Integration) MaximumCapabilities() inference.CapabilitySet {
	return inference.NewCapabilitySet(
		inference.CapabilityPromptSubmission,
		inference.CapabilityMessageSnapshots,
	)
}

func (*Integration) Discover(context.Context) (inference.Discovery, error) {
	return inference.NewDiscovery(inference.ReadinessReady), nil
}

func (i *Integration) Capabilities(context.Context, inference.InvocationRequest) (inference.CapabilitySet, error) {
	return i.MaximumCapabilities(), nil
}

// Invoke executes Gemini through providers.Service and publishes final-only
// progress before closing with exactly one terminal outcome.
func (i *Integration) Invoke(
	ctx context.Context,
	request inference.InvocationRequest,
	writer inference.ResponseWriter,
) error {
	if i == nil || i.providers == nil {
		failure := inference.NewFailure(inference.FailureInput{
			Kind:    inference.FailureDependency,
			Message: "Gemini Providers service is unavailable",
		})
		return writer.Close(ctx, inference.FailedCompletion(failure))
	}
	result, err := i.providers.Execute(ctx, executeRequestFromInvocation(request))
	if errors.Is(ctx.Err(), context.Canceled) && !errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return ctx.Err()
	}
	if err != nil {
		return writer.Close(ctx, inference.FailedCompletion(failureFromExecuteError(err)))
	}
	if err := writeFinalOnlyProgress(ctx, writer, request.InvocationID(), result.Content); err != nil {
		return err
	}
	return writer.Close(ctx, inference.SuccessfulCompletion(inference.NewResponse(inference.ResponseInput{
		Content: result.Content,
	})))
}

func executeRequestFromInvocation(request inference.InvocationRequest) providers.ExecuteRequest {
	execution := request.Execution()
	return providers.ExecuteRequest{
		Provider:           providers.IDGemini,
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

func failureFromExecuteError(err error) inference.Failure {
	var failure providers.ExecuteFailure
	if errors.As(err, &failure) {
		return inference.NewFailure(inference.FailureInput{
			Kind:      executeFailureKind(failure.Kind),
			Message:   strings.TrimSpace(failure.Message),
			Retryable: failure.Kind == providers.ExecuteFailureKindThrottled,
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
			Kind:      inference.FailureTimeout,
			Message:   "Gemini request timed out.",
			Retryable: true,
		})
	}
	return inference.NewFailure(inference.FailureInput{
		Kind:    inference.FailureUnknown,
		Message: "Gemini invocation failed.",
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
