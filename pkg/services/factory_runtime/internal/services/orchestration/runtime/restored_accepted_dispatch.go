package runtime

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// These detached values exist only during opening. The canonical dispatch
// request and accepted response remain in the session's recording.
type restoredAcceptedDispatch struct {
	dispatch work.WorkDispatch
	entry    interfaces.DispatchEntry
	result   workers.WorkResult
}

func prepareRestoredAcceptedDispatches(cfg *runtimeConfig, marking *petri.Marking) error {
	if cfg.restoredWorldState == nil || cfg.skipRestoredDispatchReconciliation {
		return nil
	}
	events := cfg.restoredEventPrefix
	if err := validateRestoredAcceptedResponses(cfg, events); err != nil {
		return err
	}
	cfg.restoredAcceptedDispatches = make(map[string]restoredAcceptedDispatch)
	for _, id := range sortedRestoredKeys(cfg.restoredWorldState.ActiveDispatches) {
		dispatch := cfg.restoredWorldState.ActiveDispatches[id]
		index, accepted := restoredAcceptanceIndex(events, id)
		if !accepted || restoredDispatchHasTerminalEvent(events, id) {
			continue
		}
		content, _ := restoredAcceptedOutput(events[:index], events[index].Context, dispatch)
		restored, err := buildRestoredAcceptedDispatch(cfg, marking, dispatch, events[index].Context, content)
		if err != nil {
			return err
		}
		cfg.restoredAcceptedDispatches[id] = restored
	}
	return nil
}

func restoredAcceptanceIndex(events []interfaces.FactoryEvent, dispatchID string) (int, bool) {
	for index := len(events) - 1; index >= 0; index-- {
		event := events[index]
		if event.Type != interfaces.FactoryEventTypeAgentRunResponse || stringPointerValue(event.Context.DispatchID) != dispatchID {
			continue
		}
		var response workers.AgentRunResponseEventPayload
		if event.DecodePayload(&response) == nil && response.Outcome == string(workers.OutcomeAccepted) {
			return index, true
		}
	}
	return 0, false
}

func buildRestoredAcceptedDispatch(cfg *runtimeConfig, marking *petri.Marking, recorded interfaces.FactoryWorldDispatch, acceptance interfaces.FactoryEventContext, content []work.WorkContentPart) (restoredAcceptedDispatch, error) {
	transition := cfg.net.Transitions[recorded.TransitionID]
	if transition == nil {
		return restoredAcceptedDispatch{}, restoredAcceptedResponseError(recorded.DispatchID, "recorded workstation is unavailable in the current Factory")
	}
	tokens, err := claimRestoredAcceptedTokens(marking, recorded)
	if err != nil {
		return restoredAcceptedDispatch{}, err
	}
	dispatch := work.WorkDispatch{
		DispatchID: recorded.DispatchID, TransitionID: recorded.TransitionID,
		WorkstationName: transition.Name, WorkerType: transition.WorkerType,
		ExpectedArtifactContext:  recorded.ExpectedArtifactContext.Clone(),
		CurrentChainingTraceID:   recorded.CurrentChainingTraceID,
		PreviousChainingTraceIDs: append([]string(nil), recorded.PreviousChainingTraceIDs...),
		Execution: work.ExecutionMetadata{
			DispatchCreatedTick: recorded.StartedTick, WorkIDs: restoredDispatchWorkIDs(recorded),
			RequestID: stringPointerValue(acceptance.RequestID), TraceID: recorded.CurrentChainingTraceID,
		},
	}
	for _, token := range tokens {
		dispatch.InputTokens = append(dispatch.InputTokens, factorytoken.ToWorker(token))
	}
	dispatch.Execution = restoredAcceptedExecutionMetadata(cfg.restoredEventPrefix, acceptance, dispatch)
	result, err := restoredAcceptedWorkResult(cfg, dispatch, acceptance, content)
	if err != nil {
		return restoredAcceptedDispatch{}, err
	}
	// Text follows the existing compatibility parser, including Work envelopes.
	// Typed content follows the detached proposal/materialization boundary.
	var proposal *workers.ProposedOutput
	if len(content) != 1 || content[0].Type.Normalized() != work.WorkContentPartTypeText {
		detached := workers.ProposedOutputFromLegacyWorkResult(result)
		detached.Primary = work.CloneWorkContentParts(content)
		proposal = &detached
	}
	result = materializeWorkerOutputForDispatchWithProposal(context.Background(), cfg.workService, cfg.net,
		cfg.workRequestIDs, workers.WorkstationDispatchRequest{Execution: workers.WorkstationExecutionRequest{Dispatch: dispatch}}, result, proposal)
	if result.Outcome != workers.OutcomeAccepted {
		return restoredAcceptedDispatch{}, restoredAcceptedResponseError(recorded.DispatchID, "recorded output cannot be materialized as Work")
	}
	return restoredAcceptedDispatch{dispatch: dispatch, result: result, entry: interfaces.DispatchEntry{
		DispatchID: dispatch.DispatchID, TransitionID: dispatch.TransitionID, WorkstationName: dispatch.WorkstationName,
		StartTime: recorded.StartedAt, ConsumedTokens: factorytoken.ToWorkerSlice(tokens),
		ExpectedArtifactContext: dispatch.ExpectedArtifactContext.Clone(),
	}}, nil
}

func restoredAcceptedWorkResult(cfg *runtimeConfig, dispatch work.WorkDispatch, acceptance interfaces.FactoryEventContext, content []work.WorkContentPart) (workers.WorkResult, error) {
	result := workers.WorkResult{DispatchID: dispatch.DispatchID, TransitionID: dispatch.TransitionID,
		Outcome: workers.OutcomeAccepted, Output: primaryOutputText(content)}
	if cfg.runtimeConfig == nil {
		return result, nil
	}
	station, found := cfg.runtimeConfig.Workstation(dispatch.WorkstationName)
	if !found || station == nil {
		return result, nil
	}
	if strings.TrimSpace(station.OutcomeFormat) == interfaces.WorkstationOutcomeFormatDecisionEnvelope {
		raw, ok := restoredAcceptedInferenceResponse(cfg.restoredEventPrefix, acceptance)
		if !ok || cfg.decisionEnvelopes == nil {
			return workers.WorkResult{}, restoredAcceptedResponseError(dispatch.DispatchID, "complete decision envelope is unavailable")
		}
		if cfg.decisionEnvelopes.UsesGoalRoutingDecisionEnvelope(station) {
			result = cfg.decisionEnvelopes.WorkResultFromGoalRoutingDecisionEnvelopeJSONOrFailed(dispatch.DispatchID, dispatch.TransitionID, raw)
		} else {
			result = cfg.decisionEnvelopes.WorkResultFromDecisionEnvelopeJSONOrFailed(dispatch.DispatchID, dispatch.TransitionID, raw)
		}
		if result.Outcome != workers.OutcomeAccepted || (strings.TrimSpace(result.Output) != strings.TrimSpace(primaryOutputText(content)) && strings.TrimSpace(raw) != strings.TrimSpace(primaryOutputText(content))) {
			return workers.WorkResult{}, restoredAcceptedResponseError(dispatch.DispatchID, "decision envelope conflicts with normalized accepted output")
		}
	}
	if strings.TrimSpace(station.OutputSchema) != "" {
		if json.Unmarshal([]byte(result.Output), &result.StructuredResult) != nil {
			return workers.WorkResult{}, restoredAcceptedResponseError(dispatch.DispatchID, "recorded structured output is malformed")
		}
		result.StructuredResultPresent = true
	}
	return result, nil
}

// Agent decision envelopes are interpreted before MODEL_RESPONSE records its
// primary output. Recover their feedback and proposed Work only from a complete
// matching provider response; primary output alone cannot authorize those facts.
func restoredAcceptedInferenceResponse(events []interfaces.FactoryEvent, acceptance interfaces.FactoryEventContext) (string, bool) {
	requests := make(map[string]workers.InferenceRequestEventPayload)
	var raw string
	for _, event := range events {
		if event.Type == interfaces.FactoryEventTypeAgentRunResponse && sameRestoredResponseContext(event.Context, acceptance) {
			break
		}
		if !sameRestoredResponseContext(event.Context, acceptance) {
			continue
		}
		switch event.Type {
		case interfaces.FactoryEventTypeModelRequest:
			// A new normalized invocation cannot borrow an earlier provider
			// envelope, even when both primary outputs happen to match.
			requests = make(map[string]workers.InferenceRequestEventPayload)
			raw = ""
		case interfaces.FactoryEventTypeInferenceRequest:
			var request workers.InferenceRequestEventPayload
			if event.DecodePayload(&request) == nil {
				requests[request.InferenceRequestID] = request
			}
			raw = ""
		case interfaces.FactoryEventTypeInferenceResponse:
			var response workers.InferenceResponseEventPayload
			raw = ""
			if event.DecodePayload(&response) != nil {
				continue
			}
			request, exists := requests[response.InferenceRequestID]
			if exists && request.Attempt == response.Attempt && response.Outcome == workers.InferenceOutcomeSucceeded && completeRestoredInferenceOutput(response.Response) {
				raw = *response.Response
			}
		}
	}
	return raw, raw != ""
}

func restoredAcceptedExecutionMetadata(events []interfaces.FactoryEvent, acceptance interfaces.FactoryEventContext, dispatch work.WorkDispatch) work.ExecutionMetadata {
	metadata := dispatch.Execution
	for _, event := range events {
		if event.Type != interfaces.FactoryEventTypeDispatchRequest || !sameRestoredResponseContext(event.Context, acceptance) {
			continue
		}
		metadata.RequestID = stringPointerValue(event.Context.RequestID)
		if event.Context.TraceIDs != nil && len(*event.Context.TraceIDs) > 0 {
			metadata.TraceID = (*event.Context.TraceIDs)[0]
		}
		var request interfaces.DispatchRequestEventPayload
		if event.DecodePayload(&request) == nil && request.Metadata != nil {
			metadata.ReplayKey = stringPointerValue(request.Metadata.ReplayKey)
		}
	}
	if metadata.ReplayKey == "" {
		parts := []string{dispatch.TransitionID}
		if metadata.TraceID != "" {
			parts = append(parts, metadata.TraceID)
		}
		metadata.ReplayKey = strings.Join(append(parts, metadata.WorkIDs...), "/")
	}
	return metadata
}

func claimRestoredAcceptedTokens(marking *petri.Marking, dispatch interfaces.FactoryWorldDispatch) ([]factorytoken.Token, error) {
	var tokens []factorytoken.Token
	for _, workID := range restoredDispatchWorkIDs(dispatch) {
		var matches []*factorytoken.Token
		for _, token := range marking.Tokens {
			if token.Color.DataType != factorytoken.DataTypeResource && token.Color.WorkID == workID {
				matches = append(matches, token)
			}
		}
		if len(matches) != 1 {
			return nil, restoredAcceptedResponseError(dispatch.DispatchID, "recorded input Work has no unique restored claim")
		}
		tokens = append(tokens, factorytoken.Clone(*matches[0]))
		marking.RemoveToken(matches[0].ID)
	}
	for _, resource := range dispatch.Resources {
		available := marking.TokensInPlace(resource.ResourceID + ":" + interfaces.ResourceStateAvailable)
		sort.Slice(available, func(i, j int) bool { return available[i].ID < available[j].ID })
		if len(available) == 0 {
			return nil, restoredAcceptedResponseError(dispatch.DispatchID, "recorded resource claim exceeds current Factory capacity")
		}
		tokens = append(tokens, factorytoken.Clone(available[0]))
		marking.RemoveToken(available[0].ID)
	}
	if len(tokens) == 0 {
		return nil, restoredAcceptedResponseError(dispatch.DispatchID, "recorded dispatch has no input claims")
	}
	return tokens, nil
}

func (f *factoryImpl) restoreAcceptedDispatches() error {
	for _, id := range sortedRestoredKeys(f.cfg.restoredAcceptedDispatches) {
		restored := f.cfg.restoredAcceptedDispatches[id]
		if err := f.engine.SeedRestoredDispatch(restored.entry); err != nil {
			return err
		}
		if err := f.dispatchFlow.SubmitDispatch(context.Background(), restored.dispatch); err != nil {
			return restoredAcceptedResponseError(id, "recorded completion could not enter Runtime result delivery")
		}
		delete(f.cfg.restoredAcceptedDispatches, id)
		f.logger.Info("restored accepted dispatch queued for completion", "dispatchID", id)
	}
	return nil
}
