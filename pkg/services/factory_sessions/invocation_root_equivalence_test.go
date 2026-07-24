package factorysessions_test

import (
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	. "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

// TestInvocationRootResultStaysInterchangeableWithEngineOutcome proves the
// published CTR-SES InvocationResult vocabulary remains the same concrete
// type family as FactoryInvocationResult after the private invocation move.
// Peers assert outcomes through this root slice without importing
// factory_sessions/internal/services/invocation.
func TestInvocationRootResultStaysInterchangeableWithEngineOutcome(t *testing.T) {
	t.Parallel()

	engine := factorydefinitions.FactoryInvocationResult{
		RequestID: "req-invoke-1",
		TraceID:   "trace-invoke-1",
		Status:    factorydefinitions.InvocationTerminalStatusCompleted,
		SessionID: "sess-invoke-alpha",
		WorkID:    "work-1",
		WorkName:  "task",
		WorkState: "done",
		PrimaryResult: []work.WorkContentPart{
			{Type: work.WorkContentPartTypeText, Text: "ok"},
		},
	}

	// Assignability is the equivalence contract: private implementation outcomes
	// must be publishable as the root InvocationResult without a second shape.
	var root InvocationResult = engine
	if root.Status != InvocationTerminalStatusCompleted ||
		root.SessionID != "sess-invoke-alpha" ||
		root.RequestID != "req-invoke-1" {
		t.Fatalf("root InvocationResult = %#v, want session-scoped completed CTR-SES outcome", root)
	}
	if root.Status != factorydefinitions.InvocationTerminalStatusCompleted {
		t.Fatalf("root Status = %q, want engine COMPLETED vocabulary", root.Status)
	}

	timedOut := InvocationResult{
		RequestID: "req-timeout",
		Status:    InvocationTerminalStatusTimedOut,
		ErrorCode: string(InvocationErrorCodeTimedOut),
		SessionID: "sess-invoke-beta",
	}
	canceled := InvocationResult{
		RequestID: "req-cancel",
		Status:    InvocationTerminalStatusCanceled,
		ErrorCode: string(InvocationErrorCodeCanceled),
		SessionID: "sess-invoke-beta",
	}
	if timedOut.Status == canceled.Status ||
		timedOut.ErrorCode == canceled.ErrorCode ||
		timedOut.Status == InvocationTerminalStatusCompleted {
		t.Fatal("timeout and cancellation typed outcomes must stay distinguishable from each other and success")
	}
}
