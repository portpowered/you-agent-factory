package events

import (
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestFactoryEventHistory_CurrentSessionProjectionFactsAreDetachedAndIncremental(t *testing.T) {
	t0 := time.Date(2026, 8, 23, 17, 0, 0, 0, time.UTC)
	history := newTestFactoryEventHistory(nil, func() time.Time { return t0 })
	history.RecordSessionLifecycleFromFactoryConfig("session-js", &interfaces.FactoryConfig{
		Name: "factory-js",
		Orchestrator: &interfaces.FactoryOrchestratorConfig{
			Kind: interfaces.OrchestratorKindJavaScript,
		},
	}, 0, t0)
	history.RecordOrchestratorPhaseChanged(OrchestratorPhaseChangedInput{
		SessionID:        "session-js",
		OrchestratorKind: interfaces.OrchestratorKindJavaScript,
		PhaseID:          "phase-plan",
		PhaseName:        "plan",
		Source:           "runtime",
		Tick:             1,
		PhaseStatus:      interfaces.OrchestratorPhaseStatusActive,
	}, t0.Add(time.Second))

	record := interfaces.FactoryDispatchRecord{
		DispatchID:    "dispatch-approval",
		HumanApproval: true,
		Dispatch: work.WorkDispatch{
			DispatchID:      "dispatch-approval",
			TransitionID:    "approval-workstation",
			WorkstationName: "Release Approval",
			Execution:       work.ExecutionMetadata{RequestID: "request-approval"},
			InputTokens: workerexecution.InputTokens(workerexecution.Token{
				ID:    "token-approval",
				Color: workerexecution.Color{WorkID: "work-approval", TraceID: "trace-approval"},
			}),
		},
	}
	history.RecordWorkstationRequest(2, record, t0.Add(2*time.Second))
	history.RecordHumanApprovalRequested(2, record, t0.Add(2*time.Second))

	facts, err := history.CurrentSessionProjectionFacts()
	if err != nil {
		t.Fatalf("CurrentSessionProjectionFacts() error = %v", err)
	}
	if facts.SessionBracket == nil || facts.SessionBracket.SessionID != "session-js" {
		t.Fatalf("session bracket = %#v, want session-js", facts.SessionBracket)
	}
	if facts.JavaScriptRuntime == nil || facts.JavaScriptRuntime.Phase != "plan" {
		t.Fatalf("JavaScript runtime = %#v, want phase plan", facts.JavaScriptRuntime)
	}
	approval, ok := facts.PendingHumanApprovals["approval-dispatch-approval"]
	if !ok || approval.SessionID != "session-js" || approval.RequestID != "request-approval" ||
		approval.WorkstationID != "approval-workstation" || len(approval.WorkItemIDs) != 1 || approval.WorkItemIDs[0] != "work-approval" {
		t.Fatalf("pending approval = %#v, want stable correlated approval", facts.PendingHumanApprovals)
	}

	approval.WorkItemIDs[0] = "mutated"
	approval.Decisions[0] = "MUTATED"
	facts.PendingHumanApprovals["approval-dispatch-approval"] = approval
	facts.JavaScriptRuntime.Phases[0] = "mutated"
	next, err := history.CurrentSessionProjectionFacts()
	if err != nil {
		t.Fatalf("CurrentSessionProjectionFacts() after mutation error = %v", err)
	}
	nextApproval := next.PendingHumanApprovals["approval-dispatch-approval"]
	if nextApproval.WorkItemIDs[0] != "work-approval" || nextApproval.Decisions[0] != interfaces.HumanApprovalDecisionApprove ||
		next.JavaScriptRuntime.Phases[0] != "plan" {
		t.Fatalf("projection leaked mutable read state: %#v / %#v", nextApproval, next.JavaScriptRuntime)
	}

	history.RecordDispatchReconciled(DispatchReconciledInput{
		SessionID:            "session-js",
		DispatchID:           "dispatch-approval",
		Tick:                 3,
		ReconciledStatus:     interfaces.FactoryDispatchStatusCompleted,
		ReconciliationSource: interfaces.DispatchReconciliationSource("RUNTIME_RECONCILER"),
	}, t0.Add(3*time.Second))
	resolved, err := history.CurrentSessionProjectionFacts()
	if err != nil {
		t.Fatalf("CurrentSessionProjectionFacts() after resolution error = %v", err)
	}
	if len(resolved.PendingHumanApprovals) != 0 {
		t.Fatalf("resolved approvals = %#v, want none", resolved.PendingHumanApprovals)
	}
}
