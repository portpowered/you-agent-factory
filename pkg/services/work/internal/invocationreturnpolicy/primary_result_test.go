package invocationreturnpolicy

import "testing"

func TestClassifyMissingPrimaryResultReturnsPendingHumanApprovalContext(t *testing.T) {
	state := InvocationWorldState{
		WorkRequestsByID: map[string]InvocationWorkRequest{
			"request-1": {WorkItems: []WorkItem{{ID: "work-1", WorkTypeID: "release", DisplayName: "Release package", State: "processing"}}},
		},
		WorkItemsByID: map[string]WorkItem{
			"work-1": {ID: "work-1", WorkTypeID: "release", DisplayName: "Release package", State: "processing"},
		},
		PendingHumanApprovals: []InvocationHumanApproval{{
			ApprovalID:      "approval-1",
			SessionID:       "session-1",
			DispatchID:      "dispatch-1",
			WorkstationID:   "approval-workstation",
			WorkstationName: "Release Approval",
			Decisions:       []string{"APPROVE", "REJECT"},
			Status:          "PENDING",
			WorkItemIDs:     []string{"work-1"},
		}},
	}

	got, classified := ClassifyMissingPrimaryResult(PrimaryResultSelectionInput{
		RequestID:  "request-1",
		WorldState: state,
	})
	if !classified || got == nil {
		t.Fatalf("ClassifyMissingPrimaryResult = (%#v, %t), want needs-human classification", got, classified)
	}
	if got.Code != PrimaryResultErrorCodeNeedsHuman || got.Context.SessionID != "session-1" || got.Context.ApprovalID != "approval-1" ||
		got.Context.DispatchID != "dispatch-1" || got.Context.WorkID != "work-1" ||
		got.Context.WorkstationID != "approval-workstation" || got.Context.WorkstationName != "Release Approval" {
		t.Fatalf("needs-human error = %#v, want approval/workstation/work context", got)
	}
	if got.Message != `invocation needs human input: work "Release package" is waiting for approval at workstation "Release Approval"` {
		t.Fatalf("needs-human message = %q, want stable operator diagnostic", got.Message)
	}
}
