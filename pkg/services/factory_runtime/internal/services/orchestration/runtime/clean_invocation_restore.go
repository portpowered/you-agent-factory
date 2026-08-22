package runtime

import (
	"fmt"
	"sort"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
)

// seedRestoredWork copies only recorded Work into a fresh marking. Resource
// tokens have already been generated from the current topology and clock by
// buildRuntimeMarking, so recorded resource occupancy is intentionally not an
// input to this conversion.
func seedRestoredWork(
	marking *petri.Marking,
	net *state.Net,
	restored *interfaces.FactoryWorldState,
	now time.Time,
	resourcePlaceIDs map[string]struct{},
	excludedWorkIDs map[string]struct{},
) (map[string]struct{}, error) {
	seededWorkIDs := make(map[string]struct{})
	if marking == nil || net == nil || restored == nil {
		return seededWorkIDs, nil
	}
	items := restoredWorkItems(restored)
	placements := restoredWorkPlacements(restored, items)
	if err := validateRestoredWorkState(restored, net, items, placements, resourcePlaceIDs); err != nil {
		return nil, err
	}
	requestIDs := restoredWorkRequestIDs(restored)
	parentIDs := make(map[string]struct{})

	workIDs := make([]string, 0, len(items))
	for workID := range items {
		workIDs = append(workIDs, workID)
	}
	sort.Strings(workIDs)
	for _, workID := range workIDs {
		if _, excluded := excludedWorkIDs[workID]; excluded {
			// Deterministic replay re-materializes Work with recorded dispatch
			// facts from its replay submission at the recorded logical tick. Do
			// not let the detached seed dispatch one tick before that hook.
			continue
		}
		placeID, hasPlacement := placements[workID]
		if !hasPlacement {
			// WorkItemsByID is the durable historical index. Only current
			// occupancy becomes a live runtime token.
			continue
		}
		token, ok := restoredWorkTokenForPlacement(
			marking,
			net,
			restored,
			items[workID],
			placeID,
			requestIDs[workID],
			restored.RelationsByWorkID[workID],
			now,
			resourcePlaceIDs,
		)
		if !ok {
			return nil, fmt.Errorf(
				"restore Work %q at %q: placement cannot be represented by the current Factory topology",
				workID,
				placements[workID],
			)
		}
		marking.AddToken(token)
		seededWorkIDs[token.Color.WorkID] = struct{}{}
		registerRestoredWorkParent(marking, token, parentIDs)
	}

	for _, parentID := range sortedStringKeys(parentIDs) {
		marking.CompleteParentChildRegistration(parentID)
	}
	return seededWorkIDs, nil
}

// restoredWorkIDsWithRecordedDispatch returns the Work identities whose
// restored replay facts include a dispatch. Replay must re-materialize those
// requests so the recorded side effect can run, replacing their seeded token
// at the materialization boundary; Work without a recorded dispatch remains
// seeded in place.
func restoredWorkIDsWithRecordedDispatch(restored *interfaces.FactoryWorldState) map[string]struct{} {
	workIDs := make(map[string]struct{})
	if restored == nil {
		return workIDs
	}
	for _, dispatch := range restored.ActiveDispatches {
		addRestoredDispatchWorkIDs(workIDs, dispatch.WorkItemIDs)
	}
	for _, dispatch := range restored.CompletedDispatches {
		addRestoredDispatchWorkIDs(workIDs, dispatch.WorkItemIDs)
	}
	for _, dispatch := range restored.FailedDispatches {
		addRestoredDispatchWorkIDs(workIDs, dispatch.WorkItemIDs)
	}
	for _, approval := range restored.PendingHumanApprovalsByID {
		addRestoredDispatchWorkIDs(workIDs, approval.WorkItemIDs)
	}
	return workIDs
}

func addRestoredDispatchWorkIDs(destination map[string]struct{}, workIDs []string) {
	for _, workID := range workIDs {
		if workID != "" {
			destination[workID] = struct{}{}
		}
	}
}
