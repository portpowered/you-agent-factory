package projections

import (
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func (r *factoryWorldReducer) factoryRelationsFromGenerated(relations *[]factoryapi.Relation, context factoryapi.FactoryEventContext) []interfaces.FactoryRelation {
	if relations == nil {
		return nil
	}
	out := make([]interfaces.FactoryRelation, 0, len(*relations))
	for _, relation := range *relations {
		converted := r.factoryRelationFromGenerated(relation, context)
		if converted.TargetWorkID != "" {
			out = append(out, converted)
		}
	}
	return out
}

func (r *factoryWorldReducer) factoryRelationFromGenerated(relation factoryapi.Relation, context factoryapi.FactoryEventContext) interfaces.FactoryRelation {
	requestItems := r.requestWorkItems(stringValue(context.RequestId))
	targetWorkID := stringValue(relation.TargetWorkId)
	if targetWorkID == "" {
		targetWorkID = workIDForRequestName(requestItems, relation.TargetWorkName)
	}
	sourceWorkID := workIDForRequestName(requestItems, relation.SourceWorkName)
	if sourceWorkID == "" {
		sourceWorkID = sourceWorkIDFromContext(context, targetWorkID)
	}
	return interfaces.FactoryRelation{
		Type:           string(relation.Type),
		SourceWorkID:   sourceWorkID,
		SourceWorkName: relation.SourceWorkName,
		TargetWorkID:   targetWorkID,
		TargetWorkName: relation.TargetWorkName,
		RequiredState:  stringValue(relation.RequiredState),
		RequestID:      stringValue(context.RequestId),
		TraceID:        firstString(context.TraceIds),
	}
}

func (r *factoryWorldReducer) requestWorkItems(requestID string) []interfaces.FactoryWorkItem {
	if requestID == "" {
		return nil
	}
	return r.stateValue.WorkRequestsByID[requestID].WorkItems
}

func workIDForRequestName(items []interfaces.FactoryWorkItem, workName string) string {
	if workName == "" {
		return ""
	}
	for _, item := range items {
		if item.DisplayName == workName {
			return item.ID
		}
	}
	return ""
}

func sourceWorkIDFromContext(context factoryapi.FactoryEventContext, targetWorkID string) string {
	for _, workID := range sliceValue(context.WorkIds) {
		if workID != "" && workID != targetWorkID {
			return workID
		}
	}
	return ""
}
