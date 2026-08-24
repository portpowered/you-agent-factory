package runtime

import (
	"fmt"
	"strconv"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// recordedTokenUsageFromDiagnostics preserves provider usage in the
// event-first Worker Session projection. Provider-session transcript storage
// is optional, but normalized response metadata is already part of the
// replay-safe dispatch diagnostics and must remain observable when that
// provider source is unavailable.
func recordedTokenUsageFromDiagnostics(diagnostics *workers.SafeWorkDiagnostics) *workersessions.TokenUsage {
	if diagnostics == nil || diagnostics.Provider == nil {
		return nil
	}
	metadata := diagnostics.Provider.ResponseMetadata
	input := recordedOptionalInt(metadata, workers.ProviderResponseMetadataInputTokens)
	cachedInput := recordedOptionalInt(metadata, workers.ProviderResponseMetadataCachedInputTokens)
	output := recordedOptionalInt(metadata, workers.ProviderResponseMetadataOutputTokens)
	reasoningOutput := recordedOptionalInt(metadata, workers.ProviderResponseMetadataReasoningOutputTokens)
	if input == nil && cachedInput == nil && output == nil && reasoningOutput == nil {
		return nil
	}
	var total *int
	if input != nil && output != nil && *input <= int(^uint(0)>>1)-*output {
		value := *input + *output
		total = &value
	}
	return &workersessions.TokenUsage{
		InputTokens:           input,
		CachedInputTokens:     cachedInput,
		OutputTokens:          output,
		ReasoningOutputTokens: reasoningOutput,
		TotalTokens:           total,
	}
}

func recordedOptionalInt(metadata map[string]string, key string) *int {
	value, ok := metadata[key]
	if !ok {
		return nil
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed < 0 {
		return nil
	}
	return &parsed
}

func recordedDispatchFact(
	dispatchID string,
	association recordedDispatchAssociation,
	requests map[string]recordedDispatchRequest,
	completed map[string]interfaces.FactoryWorldDispatchCompletion,
	providerSessions []interfaces.FactoryWorldProviderSessionRecord,
	active map[string]interfaces.FactoryWorldDispatch,
	events []interfaces.FactoryEvent,
) recordedDispatchObservation {
	fact := recordedDispatchObservation{
		workerSessionID: association.workerSessionID,
		dispatchID:      dispatchID,
		turnID:          association.turnID,
		model:           cloneRecordedString(association.model),
		reasoningEffort: cloneRecordedString(association.reasoningEffort),
		startedAt:       association.eventTime,
		state:           workersessions.StateStarting,
	}
	if request, ok := requests[dispatchID]; ok {
		fact.workIDs = append([]string(nil), request.workIDs...)
		fact.startedAt = request.startedAt
	}
	if dispatch, ok := active[dispatchID]; ok {
		fact.state = workersessions.StateRunning
		fact.startedAt = firstRecordedTime(dispatch.StartedAt, fact.startedAt)
		fact.workIDs = firstRecordedWorkIDs(dispatch.WorkItemIDs, fact.workIDs)
	}
	if dispatch, ok := completed[dispatchID]; ok {
		fact.state = recordedObservationState(dispatch.Result.Outcome)
		fact.startedAt = firstRecordedTime(dispatch.StartedAt, fact.startedAt)
		fact.endedAt = recordedDispatchEnd(dispatch, events, dispatchID)
		fact.workIDs = firstRecordedWorkIDs(dispatch.WorkItemIDs, fact.workIDs)
		fact.failure = recordedFailureWithDiagnostics(
			workers.WorkOutcome(dispatch.Result.Outcome),
			dispatch.Result.FailureDetail,
			dispatch.Result.FailureMetadata,
			fact.state,
			dispatch.Diagnostics,
		)
		fact.tokenUsage = recordedTokenUsageFromDiagnostics(dispatch.Diagnostics)
		fact.provider = cloneProviderMetadata(dispatch.ProviderSession)
	}
	for _, provider := range providerSessions {
		if provider.DispatchID != dispatchID {
			continue
		}
		fact.provider = cloneProviderMetadata(&provider.ProviderSession)
		fact.workIDs = firstRecordedWorkIDs(provider.WorkItemIDs, fact.workIDs)
		fact.failure = firstRecordedFailure(fact.failure, recordedFailureWithDiagnostics(
			workers.OutcomeFailed,
			provider.FailureDetail,
			nil,
			fact.state,
			provider.Diagnostics,
		))
		if fact.tokenUsage == nil {
			fact.tokenUsage = recordedTokenUsageFromDiagnostics(provider.Diagnostics)
		}
		break
	}
	if interruption, ok := recordedDispatchInterruption(events, dispatchID); ok && !fact.state.Terminal() {
		fact.state = workersessions.StateFailed
		fact.workIDs = firstRecordedWorkIDs(interruption.workIDs, fact.workIDs)
		endedAt := interruption.interruptedAt
		if endedAt.IsZero() {
			endedAt = interruption.eventTime
		}
		if !endedAt.IsZero() {
			endedAt = endedAt.UTC()
			fact.endedAt = &endedAt
		}
		reason := strings.TrimSpace(interruption.reason)
		if reason == "" {
			reason = "dispatch interrupted"
		}
		fact.failure = &workersessions.FailureCause{
			Kind:   workersessions.FailureCauseProcessGone,
			Detail: reason,
		}
	}
	return fact
}

func recordedObservationFromFact(fact recordedDispatchObservation, clock factory.Clock) workersessions.Observation {
	state := fact.state
	if state == "" {
		state = workersessions.StateStarting
	}
	observation := workersessions.Observation{
		WorkerSessionID:          fact.workerSessionID,
		Model:                    cloneRecordedString(fact.model),
		ReasoningEffort:          cloneRecordedString(fact.reasoningEffort),
		TokenUsage:               cloneRecordedTokenUsage(fact.tokenUsage),
		ProviderSessionAvailable: fact.provider != nil && fact.provider.ID != "",
		WorkIDs:                  append([]string(nil), fact.workIDs...),
		TurnID:                   fact.turnID,
		AttemptID:                fact.dispatchID,
		State:                    state,
		DurationBasis:            workersessions.DurationBasisUnavailable,
		Transcript:               workersessions.TranscriptAvailabilityUnavailable,
	}
	if fact.provider != nil {
		observation.ProviderSession = providerSessionRef(*fact.provider)
	}
	if !fact.startedAt.IsZero() {
		started := fact.startedAt.UTC()
		observation.StartedAt = &started
		if fact.endedAt != nil {
			ended := fact.endedAt.UTC()
			observation.EndedAt = &ended
			duration := ended.Sub(started)
			if duration < 0 {
				duration = 0
			}
			observation.Duration = &duration
			observation.DurationBasis = workersessions.DurationBasisRecordedTimestamps
		} else if !state.Terminal() && clock != nil {
			duration := clock.Now().Sub(started)
			if duration < 0 {
				duration = 0
			}
			observation.Duration = &duration
			observation.DurationBasis = workersessions.DurationBasisActiveClock
		}
	}
	if fact.failure != nil {
		failure := *fact.failure
		observation.Failure = &failure
	}
	return observation
}

func cloneRecordedTokenUsage(usage *workersessions.TokenUsage) *workersessions.TokenUsage {
	if usage == nil {
		return nil
	}
	clone := usage.Clone()
	return &clone
}

func recordDetachedAgentRunResponse(
	cfg *runtimeConfig,
	request workers.ExecuteRequest,
	result workers.ExecuteResult,
	executeErr error,
) {
	if cfg == nil || cfg.eventHistory == nil || !runtimeRequestUsesAgentRun(cfg, request) {
		return
	}
	recorder, ok := cfg.eventHistory.(recordings.WorkerEventRecorder)
	if !ok || recorder == nil {
		return
	}
	dispatchID := strings.TrimSpace(request.Correlation.DispatchID)
	if dispatchID == "" {
		return
	}

	transcript := make([]workers.AgentRunTranscriptEntry, 0, 3)
	appendPromptTranscriptEntry(&transcript, "system", request.Target.Prompt.SystemPrompt)
	appendPromptTranscriptEntry(&transcript, "user", request.Target.Prompt.UserMessage)
	appendTranscriptEntry(&transcript, "assistant", primaryOutputText(result.Output.Primary))
	safeDiagnostics := workers.SafeWorkDiagnosticsFromWorkDiagnostics(result.Diagnostics.ToWorkDiagnostics())
	if safeDiagnostics == nil {
		safeDiagnostics = &workers.SafeWorkDiagnostics{}
	}
	agentRun := safeDiagnostics.AgentRun
	if agentRun == nil {
		agentRun = &workers.SafeAgentRunDiagnostic{}
	}
	agentRun.ExecutionBehavior = workers.AgentRunExecutionBehavior
	agentRun.Transcript = transcript
	safeDiagnostics.AgentRun = agentRun
	diagnostics, err := workers.SafeWorkDiagnosticsEventPayload(safeDiagnostics)
	if err != nil {
		return
	}
	outcome := result.Outcome
	if outcome == "" && executeErr != nil {
		outcome = workers.ExecutionOutcomeFailed
	}
	recorder.RecordAgentRunEvent(workers.AgentRunResponseEvent{
		ID:                         fmt.Sprintf("factory-event/agent-run-response/%s", dispatchID),
		DispatchID:                 dispatchID,
		EventTime:                  cfg.clock.Now(),
		Tick:                       detachedExecutionTick(request.Input.Dispatch.Execution),
		DeclaredSecretJSONPointers: agentRunSecretJSONPointers(request, transcript),
		Payload: workers.AgentRunResponseEventPayload{
			AgentRunID:     fmt.Sprintf("%s/agent-run/1", dispatchID),
			Diagnostics:    diagnostics,
			DurationMillis: result.Metrics.Duration.Milliseconds(),
			Outcome:        string(outcome),
		},
	})
}

func agentRunSecretJSONPointers(
	request workers.ExecuteRequest,
	transcript []workers.AgentRunTranscriptEntry,
) []string {
	if !hasDeclaredSecretInvocationParameter(request.Input.Invocation) {
		return nil
	}
	pointers := make([]string, 0, len(transcript))
	for index, entry := range transcript {
		if entry.Role != "system" && entry.Role != "user" {
			continue
		}
		pointers = append(pointers, fmt.Sprintf("/diagnostics/agentRun/transcript/%d/summary", index))
	}
	return pointers
}

func hasDeclaredSecretInvocationParameter(arguments work.InvocationArguments) bool {
	for _, argument := range arguments.Arguments {
		if argument.Sensitive {
			return true
		}
	}
	return false
}

func detachedExecutionTick(metadata work.ExecutionMetadata) int {
	if metadata.CurrentTick != 0 {
		return metadata.CurrentTick
	}
	return metadata.DispatchCreatedTick
}

func runtimeRequestUsesAgentRun(cfg *runtimeConfig, request workers.ExecuteRequest) bool {
	lookup, ok := runtimeDefinitionLookup(cfg)
	if !ok || lookup == nil {
		return false
	}
	workstation, found := lookup.Workstation(strings.TrimSpace(request.Target.WorkstationName))
	return found && workstation != nil && interfaces.IsAgentRunWorkstationType(workstation.Type)
}
