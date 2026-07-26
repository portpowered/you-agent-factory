package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerprovider "github.com/portpowered/infinite-you/pkg/services/workers/provider"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/conductor"
	inference "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
	providerregistry "github.com/portpowered/infinite-you/pkg/services/workers/provider/registry"
)

type registryCapabilityRunner struct {
	next      workers.Runner
	providers *providerregistry.Registry
}

func (r registryCapabilityRunner) Execute(
	ctx context.Context,
	request workers.RunnerExecutionRequest,
) (workers.RunnerExecutionResult, error) {
	if err := validateRequestedRunnerCapabilities(r.providers, request); err != nil {
		return workers.RunnerExecutionResult{}, err
	}
	return r.next.Execute(ctx, request)
}

func validateRequestedRunnerCapabilities(
	providers *providerregistry.Registry,
	request workers.RunnerExecutionRequest,
) error {
	if providers == nil {
		return nil
	}
	metadata, err := providers.RunnerMetadata(request.RunnerID)
	if err != nil {
		return err
	}
	supported := make(map[workers.RunnerOptionalCapability]bool, len(metadata.Capabilities.Optional))
	for _, capability := range metadata.Capabilities.Optional {
		supported[capability.Capability] = capability.Status == workers.RunnerOptionalCapabilityStatusSupported
	}
	for _, required := range request.RequiredOptionalCapabilities {
		if supported[required] {
			continue
		}
		return fmt.Errorf(
			"%s is not supported by the %s runner in v1",
			strings.ReplaceAll(string(required), "_", " "),
			metadata.ID,
		)
	}
	return nil
}

// conductorInvocationRunner routes registry-selected external integrations
// through the provider-neutral conductor while preserving the retained
// provider-native runner path for bundled built-ins and ProviderOverride.
type conductorInvocationRunner struct {
	next      workers.Runner
	conductor *conductor.Conductor
	providers *providerregistry.Registry
}

func (r conductorInvocationRunner) Execute(
	ctx context.Context,
	request workers.RunnerExecutionRequest,
) (workers.RunnerExecutionResult, error) {
	if r.conductor == nil || r.providers == nil || r.providers.UsesNativeRunner(request.RunnerID) {
		if r.next == nil {
			return workers.RunnerExecutionResult{}, workerprovider.NewProviderError(
				workers.WorkFailureTypeMisconfigured,
				"runner requires an implementation",
				nil,
			)
		}
		return r.next.Execute(ctx, request)
	}
	destination := &conductorCollectingDestination{}
	err := r.conductor.Invoke(ctx, request.RunnerID, invocationRequestFromRunner(request), destination)
	if err != nil {
		return workers.RunnerExecutionResult{}, mapConductorInvocationError(err)
	}
	return destination.result()
}

func invocationRequestFromRunner(request workers.RunnerExecutionRequest) inference.InvocationRequest {
	invocationID := strings.TrimSpace(request.Dispatch.DispatchID)
	if invocationID == "" {
		invocationID = "conductor-invocation"
	}
	return inference.NewInvocationRequest(inference.InvocationInput{
		InvocationID: invocationID,
		Model:        request.Model,
		SystemPrompt: request.SystemPrompt,
		UserMessage:  request.UserMessage,
		OutputSchema: request.OutputSchema,
		Required:     requiredCapabilitiesFromRunner(request),
		Execution:    request,
	})
}

func requiredCapabilitiesFromRunner(request workers.RunnerExecutionRequest) inference.CapabilitySet {
	capabilities := []inference.Capability{inference.CapabilityPromptSubmission}
	for _, required := range request.RequiredOptionalCapabilities {
		switch required {
		case workers.RunnerOptionalCapabilityImageInput:
			capabilities = append(capabilities, inference.CapabilityImageInput)
		case workers.RunnerOptionalCapabilitySessionResume:
			capabilities = append(capabilities, inference.CapabilitySessionResume)
		case workers.RunnerOptionalCapabilityStructuredOutput:
			capabilities = append(capabilities, inference.CapabilityStructuredOutput)
		}
	}
	return inference.NewCapabilitySet(capabilities...)
}

func mapConductorInvocationError(err error) error {
	if err == nil {
		return nil
	}
	var rejection *conductor.Rejection
	if errors.As(err, &rejection) {
		return workerprovider.NewProviderError(
			workers.WorkFailureTypePermanentBadRequest,
			rejection.Error(),
			rejection,
		)
	}
	return workerprovider.NewProviderError(
		workers.WorkFailureTypeUnknown,
		err.Error(),
		err,
	)
}

type conductorCollectingDestination struct {
	completion *inference.Completion
}

func (d *conductorCollectingDestination) WriteEvent(context.Context, inference.EventDraft) error {
	return nil
}

func (d *conductorCollectingDestination) Close(_ context.Context, completion inference.Completion) error {
	clone := completion
	d.completion = &clone
	return nil
}

func (d *conductorCollectingDestination) result() (workers.RunnerExecutionResult, error) {
	if d == nil || d.completion == nil {
		return workers.RunnerExecutionResult{}, workerprovider.NewProviderError(
			workers.WorkFailureTypeUnknown,
			"provider invocation completed without a safe terminal outcome",
			nil,
		)
	}
	if failure := d.completion.Failure(); failure != nil {
		return workers.RunnerExecutionResult{}, providerErrorFromConductorFailure(*failure)
	}
	response := d.completion.Response()
	if response == nil {
		return workers.RunnerExecutionResult{}, workerprovider.NewProviderError(
			workers.WorkFailureTypeUnknown,
			"provider invocation completed without a safe terminal outcome",
			nil,
		)
	}
	return workers.RunnerExecutionResult{
		Content:         response.Content(),
		ProviderSession: providerSessionFromConductor(response.ProviderSession()),
	}, nil
}

func providerErrorFromConductorFailure(failure inference.Failure) error {
	return workerprovider.NewProviderError(
		workFailureTypeFromConductorFailure(failure.Kind()),
		failure.Message(),
		fmt.Errorf("conductor failure: %s", failure.Kind()),
	)
}

func workFailureTypeFromConductorFailure(kind inference.FailureKind) workers.WorkFailureType {
	switch kind {
	case inference.FailureTimeout:
		return workers.WorkFailureTypeTimeout
	case inference.FailureThrottled:
		return workers.WorkFailureTypeThrottled
	case inference.FailureAuthentication:
		return workers.WorkFailureTypeAuthFailure
	case inference.FailureInvalidRequest, inference.FailureMalformedOutput:
		return workers.WorkFailureTypePermanentBadRequest
	case inference.FailureDependency:
		return workers.WorkFailureTypeMisconfigured
	case inference.FailureCanceled:
		return workers.WorkFailureTypeUnknown
	default:
		return workers.WorkFailureTypeUnknown
	}
}

func providerSessionFromConductor(session *inference.ProviderSession) *workers.ProviderSessionMetadata {
	if session == nil {
		return nil
	}
	return &workers.ProviderSessionMetadata{
		Provider: session.Provider(),
		Kind:     session.Kind(),
		ID:       session.ID(),
	}
}
