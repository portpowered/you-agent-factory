package wire

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/portpowered/infinite-you/pkg/platform/jsonvalue"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func readSnapshotFromWorldState(view recordings.WorldStateView) (work.ReadSnapshot, error) {
	if view.SchemaVersion != recordings.WorldStateViewSchemaV1 ||
		strings.TrimSpace(view.Payload) == "" {
		return work.ReadSnapshot{}, recordings.ErrUnsupportedProjectionView
	}
	var state factorydefinitions.FactoryWorldState
	if err := json.Unmarshal([]byte(view.Payload), &state); err != nil {
		return work.ReadSnapshot{}, recordings.ErrInvalidProjectionInput
	}
	return readSnapshotFromFactoryWorldState(state), nil
}

func readSnapshotFromFactoryWorldState(state factorydefinitions.FactoryWorldState) work.ReadSnapshot {
	itemsByID := make(map[string]work.FactoryWorkItem, len(state.WorkItemsByID))
	for id, item := range state.WorkItemsByID {
		itemsByID[id] = item
	}
	for id, item := range state.ActiveWorkItemsByID {
		itemsByID[id] = item
	}
	names := workDisplayNames(itemsByID)
	activeIDs := activeWorkItemIDs(state.ActiveWorkItemsByID)
	items := make([]work.ReadModel, 0, len(itemsByID))
	for _, item := range itemsByID {
		if !isPublicWorkItem(item) {
			continue
		}
		items = append(items, readModelFromWorkItem(item, state, names, activeIDs))
	}
	return work.ReadSnapshot{Items: items}
}

func readModelFromWorkItem(
	item work.FactoryWorkItem,
	state factorydefinitions.FactoryWorldState,
	names map[string]string,
	activeIDs map[string]struct{},
) work.ReadModel {
	_, inFlight := activeIDs[item.ID]
	read := work.ReadModel{
		CursorID:                 item.ID,
		WorkID:                   item.ID,
		RequestID:                requestIDForWork(state, item.ID),
		Name:                     firstNonEmpty(item.DisplayName, names[item.ID], item.ID),
		WorkTypeName:             item.WorkTypeID,
		ChainingTraceDepth:       item.ChainingTraceDepth,
		CurrentChainingTraceID:   item.CurrentChainingTraceID,
		PreviousChainingTraceIDs: append([]string(nil), item.PreviousChainingTraceIDs...),
		TraceID:                  item.TraceID,
		Content:                  work.CloneWorkContentParts(item.Content),
		StructuredResult:         jsonvalue.Clone(item.StructuredResult),
		StructuredResultPresent:  jsonvalue.Present(item.StructuredResult, item.StructuredResultPresent),
		Tags:                     work.CloneTags(item.Tags),
		ExpectedArtifacts:        expectedArtifactsFromWorldItem(item, state),
	}
	read.State = workStateFromItem(item, state, inFlight)
	read.HumanApproval = humanApprovalForWork(item.ID, state)
	for _, relation := range state.RelationsByWorkID[item.ID] {
		read.Relations = append(read.Relations, work.ReadRelation{
			Type:           work.RelationType(relation.Type),
			SourceWorkName: read.Name,
			TargetWorkName: firstNonEmpty(relation.TargetWorkName, names[relation.TargetWorkID], relation.TargetWorkID),
			TargetWorkID:   relation.TargetWorkID,
			RequiredState:  relation.RequiredState,
		})
	}
	return read
}

func humanApprovalForWork(workID string, state factorydefinitions.FactoryWorldState) *work.HumanApprovalReadModel {
	approvalIDs := make([]string, 0, len(state.PendingHumanApprovalsByID))
	for approvalID := range state.PendingHumanApprovalsByID {
		approvalIDs = append(approvalIDs, approvalID)
	}
	sort.Strings(approvalIDs)
	for _, approvalID := range approvalIDs {
		approval := state.PendingHumanApprovalsByID[approvalID]
		for _, approvalWorkID := range approval.WorkItemIDs {
			if approvalWorkID != workID {
				continue
			}
			result := &work.HumanApprovalReadModel{
				ApprovalID: approval.ApprovalID, SessionID: approval.SessionID, DispatchID: approval.DispatchID,
				WorkstationID: approval.WorkstationID, WorkstationName: approval.WorkstationName,
				Decisions: make([]string, 0, len(approval.Decisions)), Status: string(approval.Status),
			}
			if approval.WorkstationDescription != nil {
				result.Description = factorydefinitions.ResolveNameValue(*approval.WorkstationDescription, "")
			}
			for _, decision := range approval.Decisions {
				result.Decisions = append(result.Decisions, string(decision))
			}
			return result
		}
	}
	return nil
}

func workStateFromItem(
	item work.FactoryWorkItem,
	state factorydefinitions.FactoryWorldState,
	inFlight bool,
) *work.State {
	stateName := strings.TrimSpace(item.State)
	if stateName == "" {
		return nil
	}
	if _, failed := state.FailedWorkItemsByID[item.ID]; failed {
		return &work.State{Name: stateName, Type: work.StateTypeFailed}
	}
	if terminal, ok := state.TerminalWorkByID[item.ID]; ok {
		stateName = firstNonEmpty(terminal.WorkItem.State, stateName)
	}
	category := workStateCategory(state.Topology, item.WorkTypeID, stateName)
	if inFlight && category != work.StateTypeTerminal && category != work.StateTypeFailed {
		category = work.StateTypeProcessing
	}
	if category == "" {
		category = work.StateTypeInitial
	}
	return &work.State{Name: stateName, Type: category}
}

func workStateCategory(
	topology factorydefinitions.InitialStructurePayload,
	workTypeID string,
	stateName string,
) string {
	for _, workType := range topology.WorkTypes {
		if workType.ID != workTypeID && workType.Name != workTypeID {
			continue
		}
		for _, state := range workType.States {
			if state.Value == stateName {
				return state.Category
			}
		}
	}
	return ""
}

func workDisplayNames(items map[string]work.FactoryWorkItem) map[string]string {
	names := make(map[string]string, len(items))
	for id, item := range items {
		names[id] = firstNonEmpty(item.DisplayName, id)
	}
	return names
}

func activeWorkItemIDs(items map[string]work.FactoryWorkItem) map[string]struct{} {
	active := make(map[string]struct{}, len(items))
	for id := range items {
		active[id] = struct{}{}
	}
	return active
}

func requestIDForWork(state factorydefinitions.FactoryWorldState, workID string) string {
	if workID == "" || len(state.WorkRequestsByID) == 0 {
		return ""
	}
	requestIDs := make([]string, 0, len(state.WorkRequestsByID))
	for requestID := range state.WorkRequestsByID {
		requestIDs = append(requestIDs, requestID)
	}
	sort.Strings(requestIDs)
	for _, requestKey := range requestIDs {
		request := state.WorkRequestsByID[requestKey]
		requestID := firstNonEmpty(request.RequestID, requestKey)
		for _, item := range request.WorkItems {
			if item.ID == workID {
				return requestID
			}
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func expectedArtifactsFromWorldItem(
	item work.FactoryWorkItem,
	state factorydefinitions.FactoryWorldState,
) []work.ExpectedArtifactReadModel {
	workTypeDeclarations := worldWorkTypeArtifactDeclarations(state.Topology, item.WorkTypeID)
	workstationDeclarations, inputs, observation, templateContext, workstationFound := worldDispatchArtifactFacts(item, state)
	if !workstationFound {
		workstationDeclarations = worldCandidateWorkstationArtifactDeclarations(item, state.Topology)
		inputs = []work.ExpectedArtifactInput{worldExpectedArtifactInput(item)}
	}
	return work.ExpectedArtifactReadModelProjector{}.Project(
		workTypeDeclarations,
		workstationDeclarations,
		inputs,
		observation,
		worldExpectedArtifactTemplateContext(templateContext)...,
	)
}

func worldWorkTypeArtifactDeclarations(
	topology factorydefinitions.InitialStructurePayload,
	workTypeID string,
) []work.ExpectedArtifactDeclaration {
	for _, workType := range topology.WorkTypes {
		if workType.ID == workTypeID || workType.Name == workTypeID {
			return append([]work.ExpectedArtifactDeclaration(nil), workType.ExpectedArtifacts...)
		}
	}
	return nil
}

func worldCandidateWorkstationArtifactDeclarations(
	item work.FactoryWorkItem,
	topology factorydefinitions.InitialStructurePayload,
) []work.ExpectedArtifactDeclaration {
	placeID := ""
	if item.WorkTypeID != "" && item.State != "" {
		placeID = item.WorkTypeID + ":" + item.State
	}
	var declarations []work.ExpectedArtifactDeclaration
	for _, workstation := range topology.Workstations {
		if placeID == "" || containsString(workstation.InputPlaceIDs, placeID) {
			declarations = append(declarations, workstation.ExpectedArtifacts...)
		}
	}
	return declarations
}

type worldArtifactDispatchFacts struct {
	workstationName string
	transitionID    string
	inputs          []work.FactoryWorkItem
	observation     work.ExpectedArtifactObservation
	templateContext *work.ExpectedArtifactTemplateContext
}

func worldDispatchArtifactFacts(
	item work.FactoryWorkItem,
	state factorydefinitions.FactoryWorldState,
) ([]work.ExpectedArtifactDeclaration, []work.ExpectedArtifactInput, work.ExpectedArtifactObservation, *work.ExpectedArtifactTemplateContext, bool) {
	if dispatch, ok := latestWorldActiveArtifactDispatch(item, state.ActiveDispatches); ok {
		return worldWorkstationArtifactDeclarations(state.Topology, dispatch.transitionID, dispatch.workstationName),
			worldExpectedArtifactInputs(dispatch.inputs, item), dispatch.observation, dispatch.templateContext, true
	}
	if dispatch, ok := latestWorldCompletedArtifactDispatch(item, state.CompletedDispatches); ok {
		return worldWorkstationArtifactDeclarations(state.Topology, dispatch.transitionID, dispatch.workstationName),
			worldExpectedArtifactInputs(dispatch.inputs, item), dispatch.observation, dispatch.templateContext, true
	}
	if dispatch, ok := latestWorldCompletedArtifactDispatch(item, state.FailedDispatches); ok {
		return worldWorkstationArtifactDeclarations(state.Topology, dispatch.transitionID, dispatch.workstationName),
			worldExpectedArtifactInputs(dispatch.inputs, item), dispatch.observation, dispatch.templateContext, true
	}
	if detail, ok := state.FailureDetailsByWorkID[item.ID]; ok && detail.ArtifactVerification != nil {
		return worldWorkstationArtifactDeclarations(state.Topology, detail.TransitionID, detail.WorkstationName),
			[]work.ExpectedArtifactInput{worldExpectedArtifactInput(detail.WorkItem)},
			worldArtifactObservation(detail.ArtifactVerification), cloneExpectedArtifactTemplateContext(detail.ExpectedArtifactContext), true
	}
	return nil, nil, work.ExpectedArtifactObservation{}, nil, false
}

func latestWorldActiveArtifactDispatch(
	item work.FactoryWorkItem,
	dispatches map[string]factorydefinitions.FactoryWorldDispatch,
) (worldArtifactDispatchFacts, bool) {
	ids := make([]string, 0, len(dispatches))
	for id := range dispatches {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		dispatch := dispatches[id]
		if !worldDispatchContainsWork(dispatch.WorkItemIDs, dispatch.Inputs, nil, nil, item.ID) {
			continue
		}
		return worldArtifactDispatchFacts{
			workstationName: dispatch.Workstation.Name,
			transitionID:    dispatch.TransitionID,
			inputs:          worldWorkstationInputs(dispatch.Inputs),
			templateContext: cloneExpectedArtifactTemplateContext(dispatch.ExpectedArtifactContext),
		}, true
	}
	return worldArtifactDispatchFacts{}, false
}

func latestWorldCompletedArtifactDispatch(
	item work.FactoryWorkItem,
	dispatches []factorydefinitions.FactoryWorldDispatchCompletion,
) (worldArtifactDispatchFacts, bool) {
	for index := len(dispatches) - 1; index >= 0; index-- {
		dispatch := dispatches[index]
		if !worldDispatchContainsWork(dispatch.WorkItemIDs, dispatch.ConsumedInputs, dispatch.InputWorkItems, dispatch.OutputWorkItems, item.ID) {
			continue
		}
		return worldArtifactDispatchFacts{
			workstationName: dispatch.Workstation.Name,
			transitionID:    dispatch.TransitionID,
			inputs:          append([]work.FactoryWorkItem(nil), dispatch.InputWorkItems...),
			observation:     worldArtifactObservationFromResult(dispatch.Result),
			templateContext: cloneExpectedArtifactTemplateContext(dispatch.ExpectedArtifactContext),
		}, true
	}
	return worldArtifactDispatchFacts{}, false
}

func cloneExpectedArtifactTemplateContext(
	context *work.ExpectedArtifactTemplateContext,
) *work.ExpectedArtifactTemplateContext {
	return context.Clone()
}

func worldDispatchContainsWork(
	workItemIDs []string,
	inputs []factorydefinitions.WorkstationInput,
	inputWorkItems []work.FactoryWorkItem,
	outputWorkItems []work.FactoryWorkItem,
	workID string,
) bool {
	if containsString(workItemIDs, workID) {
		return true
	}
	for _, input := range inputs {
		if input.WorkItem != nil && input.WorkItem.ID == workID {
			return true
		}
	}
	for _, workItem := range inputWorkItems {
		if workItem.ID == workID {
			return true
		}
	}
	for _, workItem := range outputWorkItems {
		if workItem.ID == workID {
			return true
		}
	}
	return false
}

func worldWorkstationArtifactDeclarations(
	topology factorydefinitions.InitialStructurePayload,
	transitionID string,
	workstationName string,
) []work.ExpectedArtifactDeclaration {
	for _, workstation := range topology.Workstations {
		if workstation.ID == transitionID || workstation.Name == transitionID || workstation.Name == workstationName {
			return append([]work.ExpectedArtifactDeclaration(nil), workstation.ExpectedArtifacts...)
		}
	}
	return nil
}

func worldWorkstationInputs(inputs []factorydefinitions.WorkstationInput) []work.FactoryWorkItem {
	items := make([]work.FactoryWorkItem, 0, len(inputs))
	for _, input := range inputs {
		if input.WorkItem != nil {
			items = append(items, *input.WorkItem)
		}
	}
	return items
}

func worldExpectedArtifactInputs(items []work.FactoryWorkItem, fallback work.FactoryWorkItem) []work.ExpectedArtifactInput {
	if len(items) == 0 {
		items = []work.FactoryWorkItem{fallback}
	}
	inputs := make([]work.ExpectedArtifactInput, 0, len(items))
	for _, item := range items {
		inputs = append(inputs, worldExpectedArtifactInput(item))
	}
	return inputs
}

func worldExpectedArtifactInput(item work.FactoryWorkItem) work.ExpectedArtifactInput {
	return work.ExpectedArtifactInput{
		Name:       firstNonEmpty(item.DisplayName, item.ID),
		WorkID:     item.ID,
		WorkTypeID: item.WorkTypeID,
		DataType:   "work",
		TraceID:    item.TraceID,
		ParentID:   item.ParentID,
		Project:    item.Tags["project"],
		Tags:       work.CloneTags(item.Tags),
	}
}

func worldArtifactObservation(
	verification *workers.ExpectedArtifactVerification,
) work.ExpectedArtifactObservation {
	if verification == nil {
		return work.ExpectedArtifactObservation{}
	}
	observation := work.ExpectedArtifactObservation{Verified: true}
	for _, entry := range verification.Entries {
		observation.Entries = append(observation.Entries, work.ExpectedArtifactVerificationEntry{
			DeclarationIndex: entry.DeclarationIndex,
			Name:             entry.Name, Pattern: entry.Pattern, Reason: work.ExpectedArtifactVerificationReason(entry.Reason),
		})
	}
	return observation
}

func worldExpectedArtifactTemplateContext(
	context *work.ExpectedArtifactTemplateContext,
) []work.ExpectedArtifactTemplateContext {
	if context == nil {
		return nil
	}
	return []work.ExpectedArtifactTemplateContext{*context}
}

func worldArtifactObservationFromResult(
	result workers.WorkstationResult,
) work.ExpectedArtifactObservation {
	if result.ArtifactVerification != nil {
		return worldArtifactObservation(result.ArtifactVerification)
	}
	if result.Outcome == string(workers.OutcomeAccepted) {
		return work.ExpectedArtifactObservation{Verified: true}
	}
	return work.ExpectedArtifactObservation{}
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
