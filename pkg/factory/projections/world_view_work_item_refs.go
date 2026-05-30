package projections

import "github.com/portpowered/infinite-you/pkg/interfaces"

func workItemRefsForIDs(
	lineage interfaces.WorkPayloadLineageProjection,
	ids []string,
	items map[string]interfaces.FactoryWorkItem,
) []interfaces.FactoryWorldWorkItemRef {
	refs := make([]interfaces.FactoryWorldWorkItemRef, 0, len(ids))
	for _, id := range sortedStrings(ids) {
		item, ok := items[id]
		if !ok || item.ID == "" || interfaces.IsSystemTimeWorkType(item.WorkTypeID) {
			continue
		}
		refs = append(refs, workItemRefWithSelectedPayload(lineage, item))
	}
	return refs
}

func workItemRefsForItems(
	lineage interfaces.WorkPayloadLineageProjection,
	items []interfaces.FactoryWorkItem,
) []interfaces.FactoryWorldWorkItemRef {
	refs := make([]interfaces.FactoryWorldWorkItemRef, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		if item.ID == "" || interfaces.IsSystemTimeWorkType(item.WorkTypeID) {
			continue
		}
		if _, exists := seen[item.ID]; exists {
			continue
		}
		refs = append(refs, workItemRefWithSelectedPayload(lineage, item))
		seen[item.ID] = struct{}{}
	}
	return refs
}

func workItemRefsForInputs(
	lineage interfaces.WorkPayloadLineageProjection,
	inputs []interfaces.WorkstationInput,
) []interfaces.FactoryWorldWorkItemRef {
	refs := make([]interfaces.FactoryWorldWorkItemRef, 0, len(inputs))
	seen := make(map[string]struct{}, len(inputs))
	for _, input := range inputs {
		if input.WorkItem == nil || input.WorkItem.ID == "" || interfaces.IsSystemTimeWorkType(input.WorkItem.WorkTypeID) {
			continue
		}
		if _, exists := seen[input.WorkItem.ID]; exists {
			continue
		}
		refs = append(refs, workItemRefWithSelectedPayload(lineage, *input.WorkItem))
		seen[input.WorkItem.ID] = struct{}{}
	}
	return refs
}

func providerSessionWorkItemRefs(
	lineage interfaces.WorkPayloadLineageProjection,
	session interfaces.FactoryWorldProviderSessionRecord,
) []interfaces.FactoryWorldWorkItemRef {
	refs := make([]interfaces.FactoryWorldWorkItemRef, 0, len(session.ConsumedInputs)+len(session.WorkItemIDs))
	seen := make(map[string]struct{}, len(session.ConsumedInputs)+len(session.WorkItemIDs))
	for _, input := range session.ConsumedInputs {
		if input.WorkItem == nil || input.WorkItem.ID == "" || interfaces.IsSystemTimeWorkType(input.WorkItem.WorkTypeID) {
			continue
		}
		if _, exists := seen[input.WorkItem.ID]; exists {
			continue
		}
		refs = append(refs, workItemRefWithSelectedPayload(lineage, *input.WorkItem))
		seen[input.WorkItem.ID] = struct{}{}
	}
	for _, workID := range sortedStrings(session.WorkItemIDs) {
		if workID == "" {
			continue
		}
		if _, exists := seen[workID]; exists {
			continue
		}
		refs = append(refs, selectedWorkItemRefForID(lineage, workID, nil))
		seen[workID] = struct{}{}
	}
	if len(refs) == 0 {
		return nil
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
		ChainingTraceDepth:       item.ChainingTraceDepth,
		CurrentChainingTraceID:   currentChainingTraceID,
		PreviousChainingTraceIDs: cloneStringSlice(item.PreviousChainingTraceIDs),
		TraceID:                  item.TraceID,
	}
}

func workItemRefWithSelectedPayload(
	lineage interfaces.WorkPayloadLineageProjection,
	item interfaces.FactoryWorkItem,
) interfaces.FactoryWorldWorkItemRef {
	return selectedWorkItemRefForID(lineage, item.ID, &item)
}

func selectedWorkItemRefForID(
	lineage interfaces.WorkPayloadLineageProjection,
	workID string,
	fallback *interfaces.FactoryWorkItem,
) interfaces.FactoryWorldWorkItemRef {
	resolution := lineage.ResolveSelectedWorkSnapshot(workID)
	if resolution.Status == interfaces.WorkPayloadResolutionResolved && resolution.Snapshot != nil {
		return lineageResolvedWorkItemRef(resolution.Snapshot, string(resolution.Status))
	}

	item := interfaces.FactoryWorkItem{ID: workID}
	if fallback != nil {
		item = *fallback
	}
	ref := workItemRef(item)
	if ref.WorkID == "" {
		ref.WorkID = workID
	}
	ref.State = item.State
	ref.PayloadStatus = string(resolution.Status)
	ref.PayloadUnavailableReason = resolution.Reason
	return ref
}

func lineageResolvedWorkItemRef(
	snapshot *interfaces.WorkPayloadSnapshot,
	payloadStatus string,
) interfaces.FactoryWorldWorkItemRef {
	ref := workItemRef(snapshot.WorkItem)
	ref.State = snapshot.WorkItem.State
	ref.Content = interfaces.CloneWorkContentParts(snapshot.WorkItem.Content)
	ref.PayloadStatus = payloadStatus
	ref.LineageLogicalWorkID = snapshot.LogicalWorkID
	ref.LineageSourceKind = string(snapshot.SourceKind)
	ref.LineageContinuity = string(snapshot.Continuity)
	ref.LineageParentWorkIDs = cloneStringSlice(snapshot.ParentWorkIDs)
	return ref
}
