package agentrun

import (
	"fmt"
	"strings"
	"time"

	"github.com/portpowered/go-agent-harness/go-agent-loop/pkg/messages"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

const (
	agentRunResponseEventIDPrefix = "factory-event/agent-run-response"

	agentRunTranscriptMaxEntries = 20
	agentRunTranscriptSummaryLen = 160
)

// AgentRunEventRecorder receives generated agent-run boundary events.
type AgentRunEventRecorder func(factoryapi.FactoryEvent)

func agentRunID(dispatchID string) string {
	dispatchID = strings.TrimSpace(dispatchID)
	if dispatchID == "" {
		return "agent-run/1"
	}
	return dispatchID + "/agent-run/1"
}

func agentRunResponseEvent(
	dispatch interfaces.WorkDispatch,
	result interfaces.WorkResult,
	duration time.Duration,
	transcript []messages.Message,
	eventTime time.Time,
) factoryapi.FactoryEvent {
	payload := factoryapi.AgentRunResponseEventPayload{
		AgentRunId:     agentRunID(dispatch.DispatchID),
		Outcome:        factoryapi.WorkOutcome(result.Outcome),
		DurationMillis: duration.Milliseconds(),
		Diagnostics:    interfaces.GeneratedSafeWorkDiagnostics(agentRunSafeDiagnostics(result.Diagnostics, transcript)),
	}
	return factoryapi.FactoryEvent{
		SchemaVersion: factoryapi.AgentFactoryEventV1,
		Id:            fmt.Sprintf("%s/%s", agentRunResponseEventIDPrefix, dispatch.DispatchID),
		Type:          factoryapi.FactoryEventTypeAgentRunResponse,
		Context: factoryapi.FactoryEventContext{
			EventTime:  interfaces.CanonicalEventTime(eventTime),
			DispatchId: stringPtr(dispatch.DispatchID),
		},
		Payload: agentRunResponseFactoryEventPayload(payload),
	}
}

func agentRunSafeDiagnostics(
	diagnostics *interfaces.WorkDiagnostics,
	transcript []messages.Message,
) *interfaces.SafeWorkDiagnostics {
	safe := interfaces.SafeWorkDiagnosticsFromWorkDiagnostics(diagnostics)
	if safe == nil {
		safe = &interfaces.SafeWorkDiagnostics{}
	}
	agentRun := safe.AgentRun
	if agentRun == nil {
		agentRun = &interfaces.SafeAgentRunDiagnostic{
			ExecutionBehavior: interfaces.AgentRunExecutionBehavior,
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

func boundedAgentRunTranscript(history []messages.Message) []interfaces.AgentRunTranscriptEntry {
	if len(history) == 0 {
		return nil
	}
	start := 0
	if len(history) > agentRunTranscriptMaxEntries {
		start = len(history) - agentRunTranscriptMaxEntries
	}
	out := make([]interfaces.AgentRunTranscriptEntry, 0, len(history[start:]))
	for _, message := range history[start:] {
		summary := strings.TrimSpace(message.TextContent())
		if summary == "" {
			continue
		}
		if len(summary) > agentRunTranscriptSummaryLen {
			summary = summary[:agentRunTranscriptSummaryLen] + "..."
		}
		out = append(out, interfaces.AgentRunTranscriptEntry{
			Role:    string(message.Role),
			Summary: summary,
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func agentRunResponseFactoryEventPayload(payload factoryapi.AgentRunResponseEventPayload) factoryapi.FactoryEvent_Payload {
	var out factoryapi.FactoryEvent_Payload
	_ = out.FromAgentRunResponseEventPayload(payload)
	return out
}

func stringPtr(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
