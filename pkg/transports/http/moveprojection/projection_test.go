package moveprojection

import (
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/work"
)

func TestBuildFactoryWorldWorkMoveOperationProjectionSlice_ProjectsWorkKeyedGeneratedContract(t *testing.T) {
	t0 := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	state := interfaces.FactoryWorldState{
		WorkStateChangesByWorkID: map[string][]interfaces.FactoryWorldWorkStateChangeRecord{
			"work-bootstrap": {{
				WorkID:       "work-bootstrap",
				WorkTypeName: "task",
				FromState:    "init",
				ToState:      "review",
				FromPlaceID:  "task:init",
				ToPlaceID:    "task:review",
				Source:       work.WorkStateChangeSourceAPI,
				RequestID:    "move-request-1",
				Tick:         2,
				Sequence:     1,
				EventTime:    t0,
			}},
		},
	}

	slice := BuildFactoryWorldWorkMoveOperationProjectionSlice(state)
	if slice.WorkMoveOperationsByWorkId == nil {
		t.Fatal("WorkMoveOperationsByWorkId = nil, want projected map")
	}
	operations := *slice.WorkMoveOperationsByWorkId
	if len(operations) != 1 {
		t.Fatalf("projected work ids = %d, want 1", len(operations))
	}
	records := operations["work-bootstrap"]
	if len(records) != 1 {
		t.Fatalf("work-bootstrap operations = %#v, want one record", records)
	}
	record := records[0]
	if record.WorkId != "work-bootstrap" ||
		record.FromState != "init" ||
		record.ToState != "review" ||
		record.Source != factoryapi.WorkStateChangeSourceAPI ||
		record.Tick != 2 ||
		record.Sequence != 1 {
		t.Fatalf("projected record = %#v, want bootstrap api move", record)
	}
	if record.RequestId == nil || *record.RequestId != "move-request-1" {
		t.Fatalf("request id = %#v, want move-request-1", record.RequestId)
	}
	if record.EventTime == nil || !record.EventTime.Equal(t0) {
		t.Fatalf("event time = %#v, want %s", record.EventTime, t0)
	}
}

func TestBuildFactoryWorldWorkMoveOperationProjectionSlice_OmitsEmptyHistory(t *testing.T) {
	slice := BuildFactoryWorldWorkMoveOperationProjectionSlice(interfaces.FactoryWorldState{})
	if slice.WorkMoveOperationsByWorkId != nil {
		t.Fatalf("WorkMoveOperationsByWorkId = %#v, want omitted empty slice", slice.WorkMoveOperationsByWorkId)
	}
}
