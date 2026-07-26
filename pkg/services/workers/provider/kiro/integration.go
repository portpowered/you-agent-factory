package kiro

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/process"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/adapter"
	inference "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
)

// IntegrationDependencies are the execution collaborators used by Kiro's
// registry-backed integration.
type IntegrationDependencies struct {
	CommandRunner workerprocess.CommandRunner
}

// Integration is Kiro's provider-owned neutral-conductor implementation.
type Integration struct {
	runner workerprocess.CommandRunner
}

// NewIntegration constructs Kiro's integration. A nil runner is reserved for
// inert registry composition; invocation requires a composed command runner.
func NewIntegration(deps ...IntegrationDependencies) *Integration {
	integration := &Integration{}
	if len(deps) > 0 {
		integration.runner = deps[0].CommandRunner
	}
	return integration
}

func (*Integration) Identity() inference.Identity {
	return inference.Identity(providerIdentity)
}

func (*Integration) MaximumCapabilities() inference.CapabilitySet {
	return inference.NewCapabilitySet(
		inference.CapabilityPromptSubmission,
		inference.CapabilitySessionResume,
		inference.CapabilityMessageSnapshots,
	)
}

func (*Integration) Discover(context.Context) (inference.Discovery, error) {
	return inference.NewDiscovery(inference.ReadinessReady), nil
}

func (i *Integration) Capabilities(
	context.Context,
	inference.InvocationRequest,
) (inference.CapabilitySet, error) {
	return i.MaximumCapabilities(), nil
}

// Invoke runs Kiro once, translates its final-only response, and closes the
// response writer with exactly one authoritative completion.
func (i *Integration) Invoke(
	ctx context.Context,
	request inference.InvocationRequest,
	writer inference.ResponseWriter,
) error {
	providerRequest := kiroRequestFromInvocation(request)
	built, err := NewAdapter().BuildCommand(ctx, adapter.CommandContext{
		Request:         providerRequest,
		SkipPermissions: providerRequest.SkipPermissions,
	})
	if err != nil {
		return writer.Close(ctx, inference.FailedCompletion(inference.NewFailure(inference.FailureInput{
			Kind:    inference.FailureInvalidRequest,
			Message: err.Error(),
		})))
	}

	result, runErr := i.commandRunner().Run(ctx, built.Request)
	if errors.Is(runErr, context.Canceled) || errors.Is(runErr, context.DeadlineExceeded) {
		return runErr
	}
	if runErr != nil || result.ExitCode != 0 {
		return writer.Close(ctx, inference.FailedCompletion(inference.NewFailure(inference.FailureInput{
			Kind:    inference.FailureUnknown,
			Message: "Kiro command did not complete successfully.",
		})))
	}

	response := responseFromOutput(result.Stdout, result.Stderr, request.ProviderSession())
	if err := writeFinalOnlyProgress(ctx, writer, request.InvocationID(), response.Content()); err != nil {
		return err
	}
	return writer.Close(ctx, inference.SuccessfulCompletion(response))
}

func (i *Integration) commandRunner() workerprocess.CommandRunner {
	if i != nil && i.runner != nil {
		return i.runner
	}
	return workerprocess.CommandRunnerWithLogging(nil, nil, nil)
}

func kiroRequestFromInvocation(request inference.InvocationRequest) workerexecution.ProviderInferenceRequest {
	providerRequest := request.Execution()
	if providerRequest.Dispatch.DispatchID == "" {
		providerRequest.Dispatch.DispatchID = request.InvocationID()
	}
	providerRequest.ModelProvider = string(modelprovider.ProviderKiro)
	providerRequest.Model = request.Model()
	providerRequest.SystemPrompt = request.SystemPrompt()
	providerRequest.UserMessage = request.UserMessage()
	providerRequest.OutputSchema = request.OutputSchema()
	if session := validRequestedSession(request.ProviderSession()); session != nil {
		providerRequest.SessionID = session.ID()
	}
	return providerRequest
}

func writeFinalOnlyProgress(
	ctx context.Context,
	writer inference.ResponseWriter,
	runID, content string,
) error {
	events, err := finalOnlyProgressEvents(runID, content)
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
	started, err := finalOnlyRunEvent(runID, workerexecution.PhaseStarted)
	if err != nil {
		return nil, err
	}
	message, err := finalOnlyMessageEvent(runID, content)
	if err != nil {
		return nil, err
	}
	completed, err := finalOnlyRunEvent(runID, workerexecution.PhaseCompleted)
	if err != nil {
		return nil, err
	}
	return []inference.EventDraft{started, message, completed}, nil
}

func finalOnlyRunEvent(runID string, phase workerexecution.Phase) (inference.EventDraft, error) {
	payload, err := json.Marshal(workerexecution.RunPayload{Status: string(phase)})
	if err != nil {
		return inference.EventDraft{}, fmt.Errorf("marshal Kiro run payload: %w", err)
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
			Provider:        providerIdentity,
			Representation:  workerexecution.RepresentationNotification,
		},
	})
}

func finalOnlyMessageEvent(runID, content string) (inference.EventDraft, error) {
	payload, err := json.Marshal(workerexecution.MessagePayload{
		Role: "assistant",
		ContentBlocks: []workerexecution.ContentBlock{{
			Kind: workerexecution.ContentBlockText,
			Text: strings.Clone(content),
		}},
	})
	if err != nil {
		return inference.EventDraft{}, fmt.Errorf("marshal Kiro message payload: %w", err)
	}
	return inference.NewEventDraft(inference.EventDraftInput{
		RunID:   runID,
		Kind:    workerexecution.KindMessage,
		Phase:   workerexecution.PhaseCompleted,
		ItemID:  "kiro-final",
		Payload: payload,
		Provenance: workerexecution.Provenance{
			Delivery:        workerexecution.DeliveryNativeFinal,
			Fidelity:        workerexecution.FidelityFinalOnly,
			NativeEventType: "final_response",
			Provider:        providerIdentity,
			Representation:  workerexecution.RepresentationSnapshot,
		},
	})
}

var _ inference.Integration = (*Integration)(nil)
