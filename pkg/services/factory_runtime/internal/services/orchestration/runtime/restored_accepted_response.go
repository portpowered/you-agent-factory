package runtime

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// Validate the whole recovery batch before reconciliation writes any events.
// Bounded agent diagnostics and model previews are never executable output.
// The source prefix is scoped by Recordings to the restored logical session;
// matching individual facts also requires their original session identity.
func validateRestoredAcceptedResponses(cfg *runtimeConfig, events []interfaces.FactoryEvent) error {
	dispatchIDs := make([]string, 0, len(cfg.restoredWorldState.ActiveDispatches))
	for dispatchID := range cfg.restoredWorldState.ActiveDispatches {
		dispatchIDs = append(dispatchIDs, dispatchID)
	}
	sort.Strings(dispatchIDs)
	for _, dispatchID := range dispatchIDs {
		dispatch := cfg.restoredWorldState.ActiveDispatches[dispatchID]
		if restoredDispatchHasTerminalEvent(events, dispatchID) {
			continue
		}
		if err := validateRestoredAcceptedResponse(events, dispatch); err != nil {
			return err
		}
	}
	return nil
}

func validateRestoredAcceptedResponse(events []interfaces.FactoryEvent, dispatch interfaces.FactoryWorldDispatch) error {
	sawOtherOutcome := false
	for index := len(events) - 1; index >= 0; index-- {
		event := events[index]
		if event.Type != interfaces.FactoryEventTypeAgentRunResponse ||
			stringPointerValue(event.Context.DispatchID) != dispatch.DispatchID {
			continue
		}
		var accepted workers.AgentRunResponseEventPayload
		if err := event.DecodePayload(&accepted); err != nil {
			return restoredAcceptedResponseError(dispatch.DispatchID, "agent acceptance record is malformed")
		}
		if accepted.Outcome != string(workers.OutcomeAccepted) {
			sawOtherOutcome = true
			continue
		}
		if sawOtherOutcome {
			return restoredAcceptedResponseError(dispatch.DispatchID, "agent acceptance conflicts with a later agent response")
		}
		if _, ok := restoredAcceptedOutput(events[:index], event.Context, dispatch); !ok {
			return restoredAcceptedResponseError(dispatch.DispatchID, "complete output and matching request facts are unavailable or inconsistent")
		}
		return nil
	}
	return nil
}

func restoredAcceptedResponseError(dispatchID, reason string) error {
	return fmt.Errorf("cannot recover accepted dispatch %q: %s. Preserve the source recording. Recover the complete response from an intact recording before reopening the Factory Session. The accepted turn will not be retried", dispatchID, reason)
}

// Return detached executable content, rather than a preview or a transcript.
// Model output is the normalized boundary when present; an earlier successful
// inference cannot authorize recovery of a later failed or incomplete model run.
func restoredAcceptedOutput(events []interfaces.FactoryEvent, context interfaces.FactoryEventContext, dispatch interfaces.FactoryWorldDispatch) ([]work.WorkContentPart, bool) {
	modelRequests := make(map[string]workers.ModelRequestEventPayload)
	inferenceRequests := make(map[string]workers.InferenceRequestEventPayload)
	hasDispatchRequest := false
	var modelOutput, inferenceOutput []work.WorkContentPart
	sawModel := false
	for _, event := range events {
		if !sameRestoredResponseContext(event.Context, context) {
			continue
		}
		switch event.Type {
		case interfaces.FactoryEventTypeDispatchRequest:
			var request interfaces.DispatchRequestEventPayload
			hasDispatchRequest = event.DecodePayload(&request) == nil && request.TransitionID == dispatch.TransitionID && request.TransitionID != ""
		case interfaces.FactoryEventTypeModelRequest, interfaces.FactoryEventTypeModelResponse:
			sawModel = true
			modelOutput = restoredModelOutput(event, modelRequests)
		case interfaces.FactoryEventTypeInferenceRequest, interfaces.FactoryEventTypeInferenceResponse:
			inferenceOutput = restoredInferenceOutput(event, inferenceRequests)
		}
	}
	if !hasDispatchRequest {
		return nil, false
	}
	if sawModel {
		return modelOutput, len(modelOutput) > 0
	}
	return inferenceOutput, len(inferenceOutput) > 0
}

func restoredModelOutput(event interfaces.FactoryEvent, requests map[string]workers.ModelRequestEventPayload) []work.WorkContentPart {
	if event.Type == interfaces.FactoryEventTypeModelRequest {
		var request workers.ModelRequestEventPayload
		if event.DecodePayload(&request) == nil && request.ModelRequestID != "" {
			requests[request.ModelRequestID] = request
		}
		return nil
	}
	var response workers.ModelResponseEventPayload
	if event.DecodePayload(&response) != nil {
		return nil
	}
	request, exists := requests[response.ModelRequestID]
	if !exists || !matchingRestoredModelResponse(request, response) {
		return nil
	}
	return work.CloneWorkContentParts(*response.OutputContent)
}

func restoredInferenceOutput(event interfaces.FactoryEvent, requests map[string]workers.InferenceRequestEventPayload) []work.WorkContentPart {
	if event.Type == interfaces.FactoryEventTypeInferenceRequest {
		var request workers.InferenceRequestEventPayload
		if event.DecodePayload(&request) == nil && request.InferenceRequestID != "" {
			requests[request.InferenceRequestID] = request
		}
		return nil
	}
	var response workers.InferenceResponseEventPayload
	if event.DecodePayload(&response) != nil {
		return nil
	}
	request, exists := requests[response.InferenceRequestID]
	if !exists || request.Attempt != response.Attempt ||
		response.Outcome != workers.InferenceOutcomeSucceeded || !completeRestoredInferenceOutput(response.Response) {
		return nil
	}
	content, err := work.ContentFromWorkerOutput(*response.Response)
	if err != nil {
		return nil
	}
	return work.CloneWorkContentParts(content)
}

func sameRestoredResponseContext(left, right interfaces.FactoryEventContext) bool {
	return stringPointerValue(left.DispatchID) == stringPointerValue(right.DispatchID) &&
		stringPointerValue(left.SessionID) == stringPointerValue(right.SessionID)
}

func matchingRestoredModelResponse(request workers.ModelRequestEventPayload, response workers.ModelResponseEventPayload) bool {
	if request.Attempt != response.Attempt || request.Model != response.Model ||
		request.Worker != response.Worker || request.Operation != response.Operation ||
		response.Outcome != workers.InferenceOutcomeSucceeded || response.OutputContent == nil {
		return false
	}
	content := *response.OutputContent
	return completeRestoredContent(content)
}

func completeRestoredContent(content []work.WorkContentPart) bool {
	if len(content) == 0 {
		return false
	}
	for _, part := range content {
		switch part.Type.Normalized() {
		case work.WorkContentPartTypeText:
			if strings.TrimSpace(part.Text) == "" {
				return false
			}
			raw := strings.TrimSpace(part.Text)
			if (strings.HasPrefix(raw, "{") || strings.HasPrefix(raw, "[")) && !json.Valid([]byte(raw)) {
				return false
			}
		case work.WorkContentPartTypeJSON:
			if !json.Valid(part.JSON) {
				return false
			}
		case work.WorkContentPartTypeImage, work.WorkContentPartTypeVideo, work.WorkContentPartTypeAudio, work.WorkContentPartTypeBinary:
			if part.URL == "" && part.File == "" && part.ArtifactID == "" {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func completeRestoredInferenceOutput(response *string) bool {
	if response == nil || strings.TrimSpace(*response) == "" {
		return false
	}
	raw := strings.TrimSpace(*response)
	// A cut-off JSON envelope must not be reinterpreted as plain text by the
	// legacy output mapper. Plain text remains a supported response shape.
	if (strings.HasPrefix(raw, "{") || strings.HasPrefix(raw, "[")) && !json.Valid([]byte(raw)) {
		return false
	}
	content, err := work.ContentFromWorkerOutput(raw)
	return err == nil && completeRestoredContent(content)
}
