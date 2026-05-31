package projections

import "github.com/portpowered/infinite-you/pkg/interfaces"

func buildWorkMoveOperationsByWorkID(
	state interfaces.FactoryWorldState,
) map[string][]interfaces.FactoryWorldWorkStateChangeRecord {
	if len(state.WorkStateChangesByWorkID) == 0 {
		return nil
	}

	operationsByWorkID := make(
		map[string][]interfaces.FactoryWorldWorkStateChangeRecord,
		len(state.WorkStateChangesByWorkID),
	)
	for workID, records := range state.WorkStateChangesByWorkID {
		if len(records) == 0 {
			continue
		}
		cloned := make([]interfaces.FactoryWorldWorkStateChangeRecord, len(records))
		copy(cloned, records)
		operationsByWorkID[workID] = cloned
	}
	if len(operationsByWorkID) == 0 {
		return nil
	}
	return operationsByWorkID
}
