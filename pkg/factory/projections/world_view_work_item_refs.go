package projections

import "github.com/portpowered/infinite-you/pkg/interfaces"

func workItemRefsForIDs(
	ids []string,
	items map[string]interfaces.FactoryWorkItem,
) []interfaces.FactoryWorldWorkItemRef {
	refs := make([]interfaces.FactoryWorldWorkItemRef, 0, len(ids))
	for _, id := range sortedStrings(ids) {
		item, ok := items[id]
		if !ok || item.ID == "" || interfaces.IsSystemTimeWorkType(item.WorkTypeID) {
			continue
		}
		refs = append(refs, workItemRef(item))
	}
	return refs
}

func workItemRefsForItems(items []interfaces.FactoryWorkItem) []interfaces.FactoryWorldWorkItemRef {
	refs := make([]interfaces.FactoryWorldWorkItemRef, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		if item.ID == "" || interfaces.IsSystemTimeWorkType(item.WorkTypeID) {
			continue
		}
		if _, exists := seen[item.ID]; exists {
			continue
		}
		refs = append(refs, workItemRef(item))
		seen[item.ID] = struct{}{}
	}
	return refs
}

func workItemRefsForInputs(inputs []interfaces.WorkstationInput) []interfaces.FactoryWorldWorkItemRef {
	refs := make([]interfaces.FactoryWorldWorkItemRef, 0, len(inputs))
	seen := make(map[string]struct{}, len(inputs))
	for _, input := range inputs {
		if input.WorkItem == nil || input.WorkItem.ID == "" || interfaces.IsSystemTimeWorkType(input.WorkItem.WorkTypeID) {
			continue
		}
		if _, exists := seen[input.WorkItem.ID]; exists {
			continue
		}
		refs = append(refs, workItemRef(*input.WorkItem))
		seen[input.WorkItem.ID] = struct{}{}
	}
	return refs
}

func workItemRef(item interfaces.FactoryWorkItem) interfaces.FactoryWorldWorkItemRef {
	currentChainingTraceID := item.CurrentChainingTraceID
	if currentChainingTraceID == "" {
		currentChainingTraceID = item.TraceID
	}
	return interfaces.FactoryWorldWorkItemRef{
		WorkID:                   item.ID,
		WorkTypeID:               item.WorkTypeID,
		DisplayName:              item.DisplayName,
		CurrentChainingTraceID:   currentChainingTraceID,
		PreviousChainingTraceIDs: cloneStringSlice(item.PreviousChainingTraceIDs),
		TraceID:                  item.TraceID,
	}
}
