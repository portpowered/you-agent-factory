package projections

import (
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/work"
)

func (r *factoryWorldReducer) factoryRelationsFromRequest(relations []work.WorkRequestEventRelation, context interfaces.FactoryEventContext) []work.FactoryRelation {
	if relations == nil {
		return nil
	}
	out := make([]work.FactoryRelation, 0, len(relations))
	for _, relation := range relations {
		converted := r.factoryRelationFromRequest(relation, context)
		if converted.TargetWorkID != "" {
			out = append(out, converted)
		}
	}
	return out
}

func factoryRelationFromRequest(
	relation work.WorkRequestEventRelation,
	requestID string,
	traceID string,
	sourceWorkID string,
	targetWorkID string,
) work.FactoryRelation {
	return work.FactoryRelation{
		Type:           string(relation.Type),
		SourceWorkID:   sourceWorkID,
		SourceWorkName: relation.SourceWorkName,
		TargetWorkID:   targetWorkID,
		TargetWorkName: relation.TargetWorkName,
		RequiredState:  relation.RequiredState,
		RequestID:      requestID,
		TraceID:        traceID,
	}
}

func factoryWorkItemsFromRequest(works []work.WorkRequestEventWork) []work.FactoryWorkItem {
	if works == nil {
		return nil
	}
	out := make([]work.FactoryWorkItem, 0, len(works))
	for _, requestWork := range works {
		item := factoryWorkItemFromEventWork(requestWork)
		if item.ID != "" {
			out = append(out, item)
		}
	}
	return out
}

func factoryWorkItemFromEventWork(eventWork work.WorkRequestEventWork) work.FactoryWorkItem {
	state := ""
	if eventWork.State != nil {
		state = eventWork.State.Name
	}
	content := work.CloneWorkContentParts(eventWork.Content)
	for i := range content {
		content[i].Type = content[i].Type.Normalized()
	}
	item := work.FactoryWorkItem{
		ID:                       eventWork.WorkID,
		WorkTypeID:               eventWork.WorkTypeID,
		State:                    state,
		DisplayName:              eventWork.Name,
		ChainingTraceDepth:       eventWork.ChainingTraceDepth,
		CurrentChainingTraceID:   eventWork.CurrentChainingTraceID,
		PreviousChainingTraceIDs: cloneStringSlice(eventWork.PreviousChainingTraceIDs),
		TraceID:                  eventWork.TraceID,
		Content:                  content,
		Tags:                     cloneStringMap(eventWork.Tags),
	}
	if item.CurrentChainingTraceID == "" {
		item.CurrentChainingTraceID = item.TraceID
	}
	return item
}

func firstWorkRequestID(works []work.WorkRequestEventWork) string {
	for _, requestWork := range works {
		if requestWork.RequestID != "" {
			return requestWork.RequestID
		}
	}
	return ""
}
