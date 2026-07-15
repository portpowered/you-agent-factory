package moveprojection

import (
	"sort"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/optional"
)

// BuildFactoryWorldWorkMoveOperationProjectionSlice keeps the additive
// work-move contract at the API boundary while deriving it from the canonical
// selected-tick FactoryWorldState model.
func BuildFactoryWorldWorkMoveOperationProjectionSlice(
	state interfaces.FactoryWorldState,
) factoryapi.FactoryWorldWorkMoveOperationProjectionSlice {
	operationsByWorkID := buildFactoryWorldWorkMoveOperationsByWorkID(state.WorkStateChangesByWorkID)
	if len(operationsByWorkID) == 0 {
		return factoryapi.FactoryWorldWorkMoveOperationProjectionSlice{}
	}
	return factoryapi.FactoryWorldWorkMoveOperationProjectionSlice{
		WorkMoveOperationsByWorkId: &operationsByWorkID,
	}
}

func buildFactoryWorldWorkMoveOperationsByWorkID(
	recordsByWorkID map[string][]interfaces.FactoryWorldWorkStateChangeRecord,
) map[string][]factoryapi.FactoryWorldWorkMoveOperationView {
	if len(recordsByWorkID) == 0 {
		return nil
	}

	workIDs := sortedWorkIDs(recordsByWorkID)
	operationsByWorkID := make(map[string][]factoryapi.FactoryWorldWorkMoveOperationView, len(workIDs))
	for _, workID := range workIDs {
		records := recordsByWorkID[workID]
		if len(records) == 0 {
			continue
		}
		views := make([]factoryapi.FactoryWorldWorkMoveOperationView, len(records))
		for i, record := range records {
			views[i] = factoryWorldWorkMoveOperationView(record)
		}
		operationsByWorkID[workID] = views
	}
	if len(operationsByWorkID) == 0 {
		return nil
	}
	return operationsByWorkID
}

func factoryWorldWorkMoveOperationView(
	record interfaces.FactoryWorldWorkStateChangeRecord,
) factoryapi.FactoryWorldWorkMoveOperationView {
	return factoryapi.FactoryWorldWorkMoveOperationView{
		WorkId:       record.WorkID,
		WorkTypeName: optional.NonEmptyStringPtr(record.WorkTypeName),
		FromState:    record.FromState,
		ToState:      record.ToState,
		FromPlaceId:  record.FromPlaceID,
		ToPlaceId:    record.ToPlaceID,
		Source:       factoryapi.WorkStateChangeSource(record.Source),
		RequestId:    optional.NonEmptyStringPtr(record.RequestID),
		Tick:         record.Tick,
		Sequence:     record.Sequence,
		EventTime:    workMoveEventTimePtr(record.EventTime),
	}
}

func workMoveEventTimePtr(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	utc := value.UTC()
	return &utc
}

func sortedWorkIDs(recordsByWorkID map[string][]interfaces.FactoryWorldWorkStateChangeRecord) []string {
	workIDs := make([]string, 0, len(recordsByWorkID))
	for workID := range recordsByWorkID {
		workIDs = append(workIDs, workID)
	}
	sort.Strings(workIDs)
	return workIDs
}
