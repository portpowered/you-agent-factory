package projections

import (
	"fmt"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func topologyPlaceID(workTypeID string, stateValue string) string {
	if workTypeID == "" || stateValue == "" {
		return ""
	}
	return fmt.Sprintf("%s:%s", workTypeID, stateValue)
}

func firstString(values *[]string) string {
	for _, value := range sliceValue(values) {
		if value != "" {
			return value
		}
	}
	return ""
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func enumStringValue[T ~string](value *T) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

func int64Value(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

func intPtrValue(value *int) *int {
	if value == nil {
		return nil
	}
	out := *value
	return &out
}

func sliceValue[T any](values *[]T) []T {
	if values == nil {
		return nil
	}
	return *values
}

// rearmInterruptedDispatch returns process-owned inputs and resources to the
// durable logical state captured by the dispatch request. The interruption
// event is terminal for the old attempt, but it is deliberately not a Work
// state transition: normal guards, dependencies, and capacity decide when a
// fresh attempt may consume the restored tokens.
func (r *factoryWorldReducer) rearmInterruptedDispatch(dispatch interfaces.FactoryWorldDispatch) {
	for _, input := range dispatch.Inputs {
		workID := ""
		if input.WorkItem != nil {
			workID = input.WorkItem.ID
		}
		if workID == "" {
			workID = input.TokenID
		}
		if workID == "" {
			continue
		}
		if _, terminal := r.stateValue.TerminalWorkByID[workID]; terminal {
			continue
		}
		if _, failed := r.stateValue.FailedWorkItemsByID[workID]; failed {
			continue
		}

		item := r.stateValue.WorkItemsByID[workID]
		if input.WorkItem != nil {
			item = mergeFactoryWorkItem(item, *input.WorkItem)
		}
		item.ID = workID
		placeID := input.PlaceID
		if placeID == "" {
			placeID = r.workPlaces[workID]
		}
		if placeID == "" {
			continue
		}
		if item.State == "" {
			item.State = stateFromPlaceID(placeID)
		}
		r.stateValue.WorkItemsByID[workID] = item
		r.addWorkToken(workID, placeID, item)
	}

	for _, resource := range dispatch.Resources {
		if resource.TokenID == "" {
			continue
		}
		placeID := resource.PlaceID
		if placeID == "" {
			placeID = resourceAvailablePlaceID(resource.ResourceID)
		}
		r.addToken(resource.TokenID, placeID, tokenKindResource)
	}
}
