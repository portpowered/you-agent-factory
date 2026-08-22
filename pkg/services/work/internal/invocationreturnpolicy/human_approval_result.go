package invocationreturnpolicy

import (
	"fmt"
	"sort"
	"strings"
)

// ClassifyMissingPrimaryResult inspects the selected-tick world state for
// authored non-success work states that explain why no primary result exists.
func ClassifyMissingPrimaryResult(input PrimaryResultSelectionInput) (*PrimaryResultError, bool) {
	requestID := strings.TrimSpace(input.RequestID)
	if requestID == "" {
		return nil, false
	}
	request, ok := invocationWorldState(input).WorkRequestsByID[requestID]
	if !ok || len(request.WorkItems) == 0 {
		return nil, false
	}

	scope := invocationScopeWorkIDs(invocationWorldState(input).PayloadLineage, request.WorkItems)
	if approval, found := scopedPendingHumanApproval(invocationWorldState(input).PendingHumanApprovals, scope); found {
		item, itemFound := scopedCurrentWorkItem(invocationWorldState(input).WorkItemsByID, scope)
		if !itemFound {
			if len(request.WorkItems) == 0 {
				return nil, false
			}
			item = request.WorkItems[0]
		}
		return humanApprovalPrimaryResultError(requestID, resolvedInvocationReturnPolicy(input.InvocationReturn), item, approval), true
	}
	for _, stateName := range []string{"blocked", "needs-human"} {
		item, found := scopedWorkItemInState(invocationWorldState(input).WorkItemsByID, scope, stateName)
		if found {
			return classifiedPrimaryResultError(requestID, resolvedInvocationReturnPolicy(input.InvocationReturn), item), true
		}
	}
	return nil, false
}

func scopedPendingHumanApproval(approvals []InvocationHumanApproval, scope map[string]struct{}) (InvocationHumanApproval, bool) {
	if len(approvals) == 0 || len(scope) == 0 {
		return InvocationHumanApproval{}, false
	}
	sorted := append([]InvocationHumanApproval(nil), approvals...)
	sort.SliceStable(sorted, func(left, right int) bool {
		return strings.Compare(sorted[left].ApprovalID, sorted[right].ApprovalID) < 0
	})
	for _, approval := range sorted {
		if strings.TrimSpace(approval.Status) != "" && strings.TrimSpace(approval.Status) != "PENDING" {
			continue
		}
		for _, workID := range approval.WorkItemIDs {
			if _, ok := scope[strings.TrimSpace(workID)]; ok {
				return approval, true
			}
		}
	}
	return InvocationHumanApproval{}, false
}

func humanApprovalPrimaryResultError(requestID, policy string, item WorkItem, approval InvocationHumanApproval) *PrimaryResultError {
	context := invocationFailureContextFromWorkItem(approval.SessionID, item)
	context.ApprovalID = strings.TrimSpace(approval.ApprovalID)
	context.DispatchID = strings.TrimSpace(approval.DispatchID)
	context.WorkstationID = strings.TrimSpace(approval.WorkstationID)
	context.WorkstationName = strings.TrimSpace(approval.WorkstationName)
	context.Decisions = append([]string(nil), approval.Decisions...)
	workstationLabel := context.WorkstationName
	if workstationLabel == "" {
		workstationLabel = context.WorkstationID
	}
	return &PrimaryResultError{
		Code: PrimaryResultErrorCodeNeedsHuman, RequestID: requestID, Policy: policy,
		Message: fmt.Sprintf("invocation needs human input: work %q is waiting for approval at workstation %q", workDisplayLabel(item), workstationLabel),
		Context: context,
	}
}
