package projections

import (
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	workdomain "github.com/portpowered/infinite-you/pkg/work"
	contentcontract "github.com/portpowered/infinite-you/pkg/work/content/contract"
)

func factoryWorkItemsFromGenerated(works *[]factoryapi.Work) []workdomain.FactoryWorkItem {
	if works == nil {
		return nil
	}
	out := make([]workdomain.FactoryWorkItem, 0, len(*works))
	for _, work := range *works {
		item := factoryWorkItemFromGenerated(work)
		if item.ID != "" {
			out = append(out, item)
		}
	}
	return out
}

func factoryWorkItemFromGenerated(work factoryapi.Work) workdomain.FactoryWorkItem {
	currentChainingTraceID := stringValue(work.CurrentChainingTraceId)
	traceID := stringValue(work.TraceId)
	if currentChainingTraceID == "" {
		currentChainingTraceID = traceID
	}
	return workdomain.FactoryWorkItem{
		ID:                       stringValue(work.WorkId),
		WorkTypeID:               stringValue(work.WorkTypeName),
		State:                    generatedWorkStateName(work.State),
		DisplayName:              work.Name,
		ChainingTraceDepth:       intValue(work.ChainingTraceDepth),
		CurrentChainingTraceID:   currentChainingTraceID,
		PreviousChainingTraceIDs: cloneStringSlice(sliceValue(work.PreviousChainingTraceIds)),
		TraceID:                  traceID,
		Content:                  generatedWorkContentToDomain(work.Content),
		Tags:                     stringMapFromGenerated(work.Tags),
	}
}

func generatedWorkContentToDomain(content *factoryapi.WorkContent) []workdomain.WorkContentPart {
	return contentcontract.PartsFromGenerated(content)
}
