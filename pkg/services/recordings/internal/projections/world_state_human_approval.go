package projections

import (
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func (r *factoryWorldReducer) applyHumanApprovalRequestedEvent(event interfaces.FactoryEvent) error {
	var payload interfaces.HumanApprovalRequestedEventPayload
	if err := event.DecodePayload(&payload); err != nil {
		return err
	}
	dispatchID := stringValue(event.Context.DispatchID)
	approvalID := strings.TrimSpace(payload.ApprovalID)
	if approvalID == "" && dispatchID != "" {
		approvalID = "approval-" + dispatchID
	}
	if approvalID == "" {
		return nil
	}
	workstationID := firstNonEmpty(payload.WorkstationID, r.transitionIDForDispatch(dispatchID))
	workstation, found := r.topologyWorkstation(workstationID)
	workstationName := workstationID
	var description *interfaces.NameValueConfig
	if found {
		workstationID = firstNonEmpty(workstation.ID, workstationID)
		workstationName = firstNonEmpty(workstation.Name, workstationID)
		description = cloneNameValue(workstation.Description)
	}
	workIDs := append([]string(nil), sliceValue(event.Context.WorkIDs)...)
	traceIDs := append([]string(nil), sliceValue(event.Context.TraceIDs)...)
	if dispatch, ok := r.stateValue.ActiveDispatches[dispatchID]; ok {
		workIDs = appendUniqueStrings(workIDs, dispatch.WorkItemIDs...)
		traceIDs = appendUniqueStrings(traceIDs, dispatch.TraceIDs...)
	}
	decisions := append([]interfaces.HumanApprovalDecision(nil), payload.Decisions...)
	if len(decisions) == 0 {
		decisions = []interfaces.HumanApprovalDecision{interfaces.HumanApprovalDecisionApprove, interfaces.HumanApprovalDecisionReject}
	}
	status := payload.Status
	if status == "" {
		status = interfaces.HumanApprovalStatusPending
	}
	r.stateValue.PendingHumanApprovalsByID[approvalID] = interfaces.FactoryWorldHumanApproval{
		ApprovalID: approvalID, DispatchID: dispatchID,
		SessionID: stringValue(event.Context.SessionID), RequestID: stringValue(event.Context.RequestID),
		WorkstationID: workstationID, WorkstationName: workstationName,
		WorkstationDescription: description, Decisions: decisions, Status: status,
		WorkItemIDs: workIDs, TraceIDs: traceIDs,
		EventID: event.Id, Tick: event.Context.Tick, EventTime: event.Context.EventTime,
	}
	return nil
}

func appendUniqueStrings(values []string, additions ...string) []string {
	for _, value := range additions {
		if strings.TrimSpace(value) == "" {
			continue
		}
		values = appendUnique(values, value)
	}
	return values
}

func dispatchInputWorkIDs(payload interfaces.DispatchRequestEventPayload, contextWorkIDs *[]string) []string {
	ordered := make([]string, 0, len(payload.Inputs)+len(sliceValue(contextWorkIDs)))
	for _, ref := range payload.Inputs {
		ordered = appendUnique(ordered, ref.WorkID)
	}
	for _, workID := range sliceValue(contextWorkIDs) {
		ordered = appendUnique(ordered, workID)
	}
	return ordered
}
