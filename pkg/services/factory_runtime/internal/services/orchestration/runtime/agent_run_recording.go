package runtime

import (
	"fmt"
	"strings"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

const detachedAgentRunTranscriptSummaryLimit = 160

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
	appendTranscriptEntry(&transcript, "system", request.Target.Prompt.SystemPrompt)
	appendTranscriptEntry(&transcript, "user", request.Target.Prompt.UserMessage)
	appendTranscriptEntry(&transcript, "assistant", primaryOutputText(result.Output.Primary))
	safeDiagnostics := workers.SafeWorkDiagnosticsFromWorkDiagnostics(result.Diagnostics.ToWorkDiagnostics())
	if safeDiagnostics == nil {
		safeDiagnostics = &workers.SafeWorkDiagnostics{}
	}
	safeDiagnostics.AgentRun = &workers.SafeAgentRunDiagnostic{
		ExecutionBehavior: workers.AgentRunExecutionBehavior,
		Transcript:        transcript,
	}
	diagnostics, err := workers.SafeWorkDiagnosticsEventPayload(safeDiagnostics)
	if err != nil {
		return
	}
	outcome := result.Outcome
	if outcome == "" && executeErr != nil {
		outcome = workers.ExecutionOutcomeFailed
	}
	eventTime := time.Now()
	if cfg.clock != nil {
		eventTime = cfg.clock.Now()
	}
	recorder.RecordAgentRunEvent(workers.AgentRunResponseEvent{
		ID:         fmt.Sprintf("factory-event/agent-run-response/%s", dispatchID),
		DispatchID: dispatchID,
		EventTime:  eventTime,
		Payload: workers.AgentRunResponseEventPayload{
			AgentRunID:     fmt.Sprintf("%s/agent-run/1", dispatchID),
			Diagnostics:    diagnostics,
			DurationMillis: result.Metrics.Duration.Milliseconds(),
			Outcome:        string(outcome),
		},
	})
}

func runtimeRequestUsesAgentRun(cfg *runtimeConfig, request workers.ExecuteRequest) bool {
	lookup, ok := runtimeDefinitionLookup(cfg)
	if !ok || lookup == nil {
		return false
	}
	workstation, found := lookup.Workstation(strings.TrimSpace(request.Target.WorkstationName))
	return found && workstation != nil && interfaces.IsAgentRunWorkstationType(workstation.Type)
}

func appendTranscriptEntry(
	transcript *[]workers.AgentRunTranscriptEntry,
	role string,
	content string,
) {
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}
	if len(content) > detachedAgentRunTranscriptSummaryLimit {
		content = content[:detachedAgentRunTranscriptSummaryLimit] + "..."
	}
	*transcript = append(*transcript, workers.AgentRunTranscriptEntry{
		Role:    role,
		Summary: content,
	})
}
