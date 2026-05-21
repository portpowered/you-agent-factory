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

func factoryRelationFromGenerated(
	relation factoryapi.Relation,
	requestID string,
	traceID string,
	sourceWorkID string,
	targetWorkID string,
) interfaces.FactoryRelation {
	return interfaces.FactoryRelation{
		Type:           string(relation.Type),
		SourceWorkID:   sourceWorkID,
		SourceWorkName: relation.SourceWorkName,
		TargetWorkID:   targetWorkID,
		TargetWorkName: relation.TargetWorkName,
		RequiredState:  stringValue(relation.RequiredState),
		RequestID:      requestID,
		TraceID:        traceID,
	}
}
