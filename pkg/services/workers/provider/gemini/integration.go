package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/process"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/adapter"
	inference "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
)

// IntegrationDependencies are the optional execution collaborators used by the
// registry-backed Gemini Integration on the neutral conductor path.
type IntegrationDependencies struct {
	CommandRunner   workerprocess.CommandRunner
	SkipPermissions bool
}

// Integration is Gemini's registry-backed inferencecontract implementation.
// Factory Sessions and worker executors select it by manifest identity and
// invoke it through the provider-neutral conductor.
type Integration struct {
	runner          workerprocess.CommandRunner
	skipPermissions bool
}

// NewIntegration constructs the Gemini Integration. A nil command runner is
// reserved for inert registry composition; Invoke requires an injected runner.
func NewIntegration(deps ...IntegrationDependencies) *Integration {
	integration := &Integration{}
	if len(deps) > 0 {
		integration.runner = deps[0].CommandRunner
		integration.skipPermissions = deps[0].SkipPermissions
	}
	return integration
}

// Identity returns Gemini's stable registry/manifest identity.
func (*Integration) Identity() inference.Identity {
	return inference.Identity(modelprovider.ProviderGemini)
}

// MaximumCapabilities mirrors the authored Gemini manifest maximum.
func (*Integration) MaximumCapabilities() inference.CapabilitySet {
	return inference.NewCapabilitySet(
		inference.CapabilityPromptSubmission,
		inference.CapabilityMessageSnapshots,
	)
}

// Discover reports Gemini as ready for conductor-routed invocation.
func (*Integration) Discover(context.Context) (inference.Discovery, error) {
	return inference.NewDiscovery(inference.ReadinessReady), nil
}

// Capabilities returns the Gemini maximum set for the request.
func (i *Integration) Capabilities(context.Context, inference.InvocationRequest) (inference.CapabilitySet, error) {
	return i.MaximumCapabilities(), nil
}

// Invoke executes Gemini through provider-owned command construction and
// publishes one safe terminal outcome through the conductor response writer.
func (i *Integration) Invoke(
	ctx context.Context,
	request inference.InvocationRequest,
	writer inference.ResponseWriter,
) error {
	runner := i.commandRunner()
	providerRequest := providerRequestFromInvocation(request)
	built, err := NewAdapter().BuildCommand(ctx, adapter.CommandContext{
		Request:         providerRequest,
		SkipPermissions: i.skipPermissionsEnabled(),
	})
	if err != nil {
		return writer.Close(ctx, inference.FailedCompletion(inference.NewFailure(inference.FailureInput{
			Kind:    inference.FailureInvalidRequest,
			Message: err.Error(),
		})))
	}
	result, runErr := runner.Run(ctx, built.Request)
	classified := NewAdapter().ClassifyFailure(ctx, adapter.FailureContext{
		CommandResult: result,
		CommandError:  runErr,
	})
	if classified.Failure != nil {
		return writer.Close(ctx, inference.FailedCompletion(failureFromAdapterFacts(*classified.Failure)))
	}
	if runErr != nil {
		return writer.Close(ctx, inference.FailedCompletion(inference.NewFailure(inference.FailureInput{
			Kind:    inference.FailureUnknown,
			Message: "Gemini command did not complete successfully.",
		})))
	}
	content := string(result.Stdout)
	if err := writeFinalOnlyProgress(ctx, writer, request, content); err != nil {
		return err
	}
	return writer.Close(ctx, inference.SuccessfulCompletion(inference.NewResponse(inference.ResponseInput{
		Content: content,
	})))
}

func (i *Integration) commandRunner() workerprocess.CommandRunner {
	if i != nil && i.runner != nil {
		return i.runner
	}
	return workerprocess.CommandRunnerWithLogging(nil, nil, nil)
}

func (i *Integration) skipPermissionsEnabled() bool {
	return i != nil && i.skipPermissions
}

func providerRequestFromInvocation(request inference.InvocationRequest) workerexecution.ProviderInferenceRequest {
	providerRequest := request.Execution()
	if providerRequest.Dispatch.DispatchID == "" {
		providerRequest.Dispatch.DispatchID = request.InvocationID()
	}
	providerRequest.ModelProvider = string(modelprovider.ProviderGemini)
	providerRequest.Model = request.Model()
	providerRequest.SystemPrompt = request.SystemPrompt()
	providerRequest.UserMessage = request.UserMessage()
	providerRequest.OutputSchema = request.OutputSchema()
	return providerRequest
}

func failureFromAdapterFacts(facts adapter.FailureFacts) inference.Failure {
	return inference.NewFailure(inference.FailureInput{
		Kind:      failureKindFromWorkType(facts.Type),
		Message:   facts.Message,
		Retryable: facts.Retry.Retryable,
	})
}

func failureKindFromWorkType(failureType workerexecution.WorkFailureType) inference.FailureKind {
	switch failureType {
	case workerexecution.WorkFailureTypeTimeout:
		return inference.FailureTimeout
	case workerexecution.WorkFailureTypeThrottled:
		return inference.FailureThrottled
	case workerexecution.WorkFailureTypeAuthFailure:
		return inference.FailureAuthentication
	case workerexecution.WorkFailureTypePermanentBadRequest:
		return inference.FailureInvalidRequest
	case workerexecution.WorkFailureTypeMisconfigured:
		return inference.FailureDependency
	default:
		return inference.FailureUnknown
	}
}

func writeFinalOnlyProgress(
	ctx context.Context,
	writer inference.ResponseWriter,
	request inference.InvocationRequest,
	content string,
) error {
	events, err := finalOnlyProgressEvents(request.InvocationID(), content)
	if err != nil {
		return err
	}
	for _, event := range events {
		if err := writer.WriteEvent(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

func finalOnlyProgressEvents(runID, content string) ([]inference.EventDraft, error) {
	provider := string(modelprovider.ProviderGemini)
	started, err := finalOnlyRunEvent(runID, provider, workerexecution.PhaseStarted)
	if err != nil {
		return nil, err
	}
	message, err := finalOnlyMessageEvent(runID, provider, content)
	if err != nil {
		return nil, err
	}
	completed, err := finalOnlyRunEvent(runID, provider, workerexecution.PhaseCompleted)
	if err != nil {
		return nil, err
	}
	return []inference.EventDraft{started, message, completed}, nil
}

func finalOnlyRunEvent(runID, provider string, phase workerexecution.Phase) (inference.EventDraft, error) {
	payload, err := json.Marshal(workerexecution.RunPayload{Status: string(phase)})
	if err != nil {
		return inference.EventDraft{}, fmt.Errorf("marshal Gemini run payload: %w", err)
	}
	return inference.NewEventDraft(inference.EventDraftInput{
		RunID:   runID,
		Kind:    workerexecution.KindRun,
		Phase:   phase,
		Payload: payload,
		Provenance: workerexecution.Provenance{
			Delivery:        workerexecution.DeliverySynthesized,
			Fidelity:        workerexecution.FidelityLifecycleOnly,
			NativeEventType: "command_completion",
			Provider:        provider,
			Representation:  workerexecution.RepresentationNotification,
		},
	})
}

func finalOnlyMessageEvent(runID, provider, content string) (inference.EventDraft, error) {
	payload, err := json.Marshal(workerexecution.MessagePayload{
		Role: "assistant",
		ContentBlocks: []workerexecution.ContentBlock{{
			Kind: workerexecution.ContentBlockText,
			Text: strings.Clone(content),
		}},
	})
	if err != nil {
		return inference.EventDraft{}, fmt.Errorf("marshal Gemini message payload: %w", err)
	}
	return inference.NewEventDraft(inference.EventDraftInput{
		RunID:   runID,
		Kind:    workerexecution.KindMessage,
		Phase:   workerexecution.PhaseCompleted,
		ItemID:  "gemini-final",
		Payload: payload,
		Provenance: workerexecution.Provenance{
			Delivery:        workerexecution.DeliveryNativeFinal,
			Fidelity:        workerexecution.FidelityFinalOnly,
			NativeEventType: "final_response",
			Provider:        provider,
			Representation:  workerexecution.RepresentationSnapshot,
		},
	})
}

var _ inference.Integration = (*Integration)(nil)
