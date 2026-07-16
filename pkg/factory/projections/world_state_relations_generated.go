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
		state := ""
		if requestWork.State != nil {
			state = requestWork.State.Name
		}
		content := work.CloneWorkContentParts(requestWork.Content)
		for i := range content {
			content[i].Type = content[i].Type.Normalized()
		}
		item := work.FactoryWorkItem{
			ID:                       requestWork.WorkID,
			WorkTypeID:               requestWork.WorkTypeID,
			State:                    state,
			DisplayName:              requestWork.Name,
			ChainingTraceDepth:       requestWork.ChainingTraceDepth,
			CurrentChainingTraceID:   requestWork.CurrentChainingTraceID,
			PreviousChainingTraceIDs: cloneStringSlice(requestWork.PreviousChainingTraceIDs),
			TraceID:                  requestWork.TraceID,
			Content:                  content,
			Tags:                     cloneStringMap(requestWork.Tags),
		}
		if item.CurrentChainingTraceID == "" {
			item.CurrentChainingTraceID = item.TraceID
		}
		if item.ID != "" {
			out = append(out, item)
		}
	}
	return out
}

func firstWorkRequestID(works []work.WorkRequestEventWork) string {
	for _, requestWork := range works {
		if requestWork.RequestID != "" {
			return requestWork.RequestID
		}
	}
	return ""
}
