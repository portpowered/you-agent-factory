package agentrun

import (
	"strings"
	"testing"
	"time"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"

	"github.com/portpowered/go-agent-harness/go-agent-loop/pkg/messages"

	"github.com/portpowered/infinite-you/pkg/services/work"
)

func TestAgentRunID_DefaultsWhenDispatchMissing(t *testing.T) {
	t.Parallel()

	if got := agentRunID(""); got != "agent-run/1" {
		t.Fatalf("agentRunID(\"\") = %q, want agent-run/1", got)
	}
	if got := agentRunID("dispatch-1"); got != "dispatch-1/agent-run/1" {
		t.Fatalf("agentRunID(dispatch-1) = %q", got)
	}
}

func TestBoundedAgentRunTranscript_TrimsAndSummarizes(t *testing.T) {
	t.Parallel()

	longText := strings.Repeat("x", agentRunTranscriptSummaryLen+20)
	history := make([]messages.Message, 0, agentRunTranscriptMaxEntries+2)
	for i := 0; i < agentRunTranscriptMaxEntries+1; i++ {
		history = append(history, messages.NewTextMessage(messages.RoleUser, "entry"))
	}
	history = append(history, messages.NewTextMessage(messages.RoleAssistant, longText))
	history = append(history, messages.NewTextMessage(messages.RoleTool, ""))

	entries := boundedAgentRunTranscript(history)
	if len(entries) == 0 || len(entries) > agentRunTranscriptMaxEntries {
		t.Fatalf("entries = %d, want 1..%d retained non-empty messages", len(entries), agentRunTranscriptMaxEntries)
	}
	if !strings.HasSuffix(entries[len(entries)-1].Summary, "...") {
		t.Fatalf("last summary = %q, want truncated suffix", entries[len(entries)-1].Summary)
	}
	if entries[0].Role != string(messages.RoleUser) {
		t.Fatalf("first role = %q, want user", entries[0].Role)
	}
}

func TestAgentRunSafeDiagnostics_IncludesTranscript(t *testing.T) {
	t.Parallel()

	transcript := []messages.Message{
		messages.NewTextMessage(messages.RoleAssistant, "final answer"),
	}
	safe := agentRunSafeDiagnostics(nil, transcript)
	if safe == nil || safe.AgentRun == nil || len(safe.AgentRun.Transcript) != 1 {
		t.Fatalf("safe diagnostics = %#v, want transcript entry", safe)
	}
	if safe.AgentRun.Transcript[0].Summary != "final answer" {
		t.Fatalf("summary = %q, want final answer", safe.AgentRun.Transcript[0].Summary)
	}
}

func TestAgentRunResponseEvent_MapsDispatchAndOutcome(t *testing.T) {
	t.Parallel()

	dispatch := work.WorkDispatch{DispatchID: "dispatch-42"}
	result := workerexecution.WorkResult{
		Outcome: workerexecution.OutcomeAccepted,
		Output:  "done",
	}
	event := agentRunResponseEvent(
		dispatch,
		result,
		1500*time.Millisecond,
		nil,
		time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
	)
	if event.ID != "factory-event/agent-run-response/dispatch-42" || event.DispatchID != "dispatch-42" {
		t.Fatalf("event identity = %#v", event)
	}
	if event.Payload.DurationMillis != 1500 || event.Payload.AgentRunID != "dispatch-42/agent-run/1" {
		t.Fatalf("payload = %#v", event.Payload)
	}
}
