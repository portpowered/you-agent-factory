package projections

import (
	"sort"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

func workRefsForActiveIDs(
	lineage work.WorkPayloadLineageProjection,
	ids []string,
	items map[string]work.FactoryWorkItem,
) []interfaces.FactoryWorldWorkItemRef {
	refs := workItemRefsForIDs(lineage, ids, items)
	if refs == nil {
		return []interfaces.FactoryWorldWorkItemRef{}
	}
	return refs
}

func mergeWorkRefs(existing []interfaces.FactoryWorldWorkItemRef, additional []interfaces.FactoryWorldWorkItemRef) []interfaces.FactoryWorldWorkItemRef {
	byID := make(map[string]interfaces.FactoryWorldWorkItemRef, len(existing)+len(additional))
	for _, ref := range existing {
		byID[ref.WorkID] = ref
	}
	for _, ref := range additional {
		byID[ref.WorkID] = ref
	}
	ids := sortedMapKeys(byID)
	merged := make([]interfaces.FactoryWorldWorkItemRef, 0, len(ids))
	for _, id := range ids {
		merged = append(merged, byID[id])
	}
	return merged
}

func filterCustomerPlaceIDs(placeIDs []string) []string {
	filtered := make([]string, 0, len(placeIDs))
	for _, placeID := range placeIDs {
		if interfaces.IsSystemTimePlace(placeID) {
			continue
		}
		filtered = append(filtered, placeID)
	}
	return filtered
}

func isSystemTimeWorkstation(workstationID string) bool {
	return workstationID == interfaces.SystemTimeExpiryTransitionID
}

func sortedMapKeys[T any](values map[string]T) []string {
	if len(values) == 0 {
		return nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func workItemRefsForIDs(
	lineage work.WorkPayloadLineageProjection,
	ids []string,
	items map[string]work.FactoryWorkItem,
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
	lineage work.WorkPayloadLineageProjection,
	items []work.FactoryWorkItem,
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
	lineage work.WorkPayloadLineageProjection,
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
	lineage work.WorkPayloadLineageProjection,
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

func workItemRef(item work.FactoryWorkItem) interfaces.FactoryWorldWorkItemRef {
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
	lineage work.WorkPayloadLineageProjection,
	item work.FactoryWorkItem,
) interfaces.FactoryWorldWorkItemRef {
	return selectedWorkItemRefForID(lineage, item.ID, &item)
}

func selectedWorkItemRefForID(
	lineage work.WorkPayloadLineageProjection,
	workID string,
	fallback *work.FactoryWorkItem,
) interfaces.FactoryWorldWorkItemRef {
	resolution := lineage.ResolveSelectedWorkSnapshot(workID)
	if resolution.Status == work.WorkPayloadResolutionResolved && resolution.Snapshot != nil {
		return lineageResolvedWorkItemRef(resolution.Snapshot, string(resolution.Status))
	}

	item := work.FactoryWorkItem{ID: workID}
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
	snapshot *work.WorkPayloadSnapshot,
	payloadStatus string,
) interfaces.FactoryWorldWorkItemRef {
	ref := workItemRef(snapshot.WorkItem)
	ref.State = snapshot.WorkItem.State
	ref.Content = work.CloneWorkContentParts(snapshot.WorkItem.Content)
	ref.PayloadStatus = payloadStatus
	ref.LineageLogicalWorkID = snapshot.LogicalWorkID
	ref.LineageSourceKind = string(snapshot.SourceKind)
	ref.LineageContinuity = string(snapshot.Continuity)
	ref.LineageParentWorkIDs = cloneStringSlice(snapshot.ParentWorkIDs)
	return ref
}
