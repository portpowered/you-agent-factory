package agentrun

import (
	"fmt"
	"strings"
	"time"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"

	"github.com/portpowered/go-agent-harness/go-agent-loop/pkg/messages"

	"github.com/portpowered/infinite-you/pkg/services/work"
)

const (
	agentRunResponseEventIDPrefix = "factory-event/agent-run-response"

	agentRunTranscriptMaxEntries = 20
	agentRunTranscriptSummaryLen = 160
)

// AgentRunEventRecorder receives worker-owned agent-run boundary facts.
type AgentRunEventRecorder = workerexecution.AgentRunEventRecorder

func agentRunID(dispatchID string) string {
	dispatchID = strings.TrimSpace(dispatchID)
	if dispatchID == "" {
		return "agent-run/1"
	}
	return dispatchID + "/agent-run/1"
}

func agentRunResponseEvent(
	dispatch work.WorkDispatch,
	result workerexecution.WorkResult,
	duration time.Duration,
	transcript []messages.Message,
	eventTime time.Time,
) workerexecution.AgentRunResponseEvent {
	// SafeWorkDiagnostics contains only JSON-compatible typed fields and string
	// maps, so encoding cannot fail for a value produced by this package.
	diagnostics, _ := workerexecution.SafeWorkDiagnosticsEventPayload(agentRunSafeDiagnostics(result.Diagnostics, transcript))
	if string(diagnostics) == "null" {
		diagnostics = nil
	}
	return workerexecution.AgentRunResponseEvent{
		ID:         fmt.Sprintf("%s/%s", agentRunResponseEventIDPrefix, dispatch.DispatchID),
		DispatchID: dispatch.DispatchID,
		EventTime:  eventTime,
		Payload: workerexecution.AgentRunResponseEventPayload{
			AgentRunID:     agentRunID(dispatch.DispatchID),
			Outcome:        string(result.Outcome),
			DurationMillis: duration.Milliseconds(),
			Diagnostics:    diagnostics,
		},
	}
}

func agentRunSafeDiagnostics(
	diagnostics *workerexecution.WorkDiagnostics,
	transcript []messages.Message,
) *workerexecution.SafeWorkDiagnostics {
	safe := workerexecution.SafeWorkDiagnosticsFromWorkDiagnostics(diagnostics)
	if safe == nil {
		safe = &workerexecution.SafeWorkDiagnostics{}
	}
	agentRun := safe.AgentRun
	if agentRun == nil {
		agentRun = &workerexecution.SafeAgentRunDiagnostic{
			ExecutionBehavior: workerexecution.AgentRunExecutionBehavior,
		}
	}
	if entries := boundedAgentRunTranscript(transcript); len(entries) > 0 {
		agentRun.Transcript = entries
	}
	safe.AgentRun = agentRun
	if safe.RenderedPrompt == nil && safe.Provider == nil && safe.AgentRun == nil {
		return nil
	}
	return safe
}

func boundedAgentRunTranscript(history []messages.Message) []workerexecution.AgentRunTranscriptEntry {
	if len(history) == 0 {
		return nil
	}
	start := 0
	if len(history) > agentRunTranscriptMaxEntries {
		start = len(history) - agentRunTranscriptMaxEntries
	}
	out := make([]workerexecution.AgentRunTranscriptEntry, 0, len(history[start:]))
	for _, message := range history[start:] {
		summary := strings.TrimSpace(message.TextContent())
		if summary == "" {
			continue
		}
		if len(summary) > agentRunTranscriptSummaryLen {
			summary = summary[:agentRunTranscriptSummaryLen] + "..."
		}
		out = append(out, workerexecution.AgentRunTranscriptEntry{
			Role:    string(message.Role),
			Summary: summary,
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
