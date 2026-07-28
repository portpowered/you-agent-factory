package wire

import (
	"encoding/json"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

func readSnapshotFromWorldState(view recordings.WorldStateView) (work.ReadSnapshot, error) {
	if view.SchemaVersion != recordings.WorldStateViewSchemaV1 ||
		strings.TrimSpace(view.Payload) == "" {
		return work.ReadSnapshot{}, recordings.ErrUnsupportedProjectionView
	}
	var state interfaces.FactoryWorldState
	if err := json.Unmarshal([]byte(view.Payload), &state); err != nil {
		return work.ReadSnapshot{}, recordings.ErrInvalidProjectionInput
	}
	return readSnapshotFromFactoryWorldState(state), nil
}

func readSnapshotFromFactoryWorldState(state interfaces.FactoryWorldState) work.ReadSnapshot {
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
	state interfaces.FactoryWorldState,
	names map[string]string,
	activeIDs map[string]struct{},
) work.ReadModel {
	_, inFlight := activeIDs[item.ID]
	read := work.ReadModel{
		CursorID:                 item.ID,
		WorkID:                   item.ID,
		Name:                     firstNonEmpty(item.DisplayName, names[item.ID], item.ID),
		WorkTypeName:             item.WorkTypeID,
		ChainingTraceDepth:       item.ChainingTraceDepth,
		CurrentChainingTraceID:   item.CurrentChainingTraceID,
		PreviousChainingTraceIDs: append([]string(nil), item.PreviousChainingTraceIDs...),
		TraceID:                  item.TraceID,
		Content:                  work.CloneWorkContentParts(item.Content),
		Tags:                     work.CloneTags(item.Tags),
	}
	read.State = workStateFromItem(item, state, inFlight)
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

func workStateFromItem(
	item work.FactoryWorkItem,
	state interfaces.FactoryWorldState,
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
	topology interfaces.InitialStructurePayload,
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
