package projectionquery_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingsservice "github.com/portpowered/infinite-you/pkg/services/recordings/service"
)

type unusedLedger struct {
	recordings.Ledger
}

func TestAcceptedRecordingsRootUsesPrivateProjectionQuery(t *testing.T) {
	t.Parallel()

	root := recordingsservice.NewService(
		&unusedLedger{},
		recordingsservice.NewProjectionService(),
	)
	malformed := factorydefinitions.FactoryEvent{
		Id:            "malformed",
		Type:          factorydefinitions.FactoryEventTypeWorkRequest,
		SchemaVersion: factorydefinitions.FactoryEventSchemaVersionV1,
		Context:       factorydefinitions.FactoryEventContext{Tick: 1},
		Payload:       json.RawMessage(`{"type":`),
	}

	result, err := root.ReconstructWorldState(recordings.ReconstructWorldStateRequest{
		Events:       []factorydefinitions.FactoryEvent{malformed},
		SelectedTick: 1,
	})
	if !errors.Is(err, recordings.ErrInvalidProjectionInput) {
		t.Fatalf("ReconstructWorldState error = %v, want ErrInvalidProjectionInput", err)
	}
	if !reflect.DeepEqual(result, recordings.ReconstructWorldStateResult{}) {
		t.Fatalf("ReconstructWorldState result = %#v, want zero result", result)
	}

	otherSessionID := "factory-session-2"
	history := []factorydefinitions.FactoryEvent{{
		Id: "other-session-cursor",
		Context: factorydefinitions.FactoryEventContext{
			SessionID: &otherSessionID,
			Sequence:  1,
		},
	}}
	err = root.ValidateReconnectReplayFrom(recordings.ValidateReconnectReplayRequest{
		Events: history,
		Cursor: factorydefinitions.FactoryEventReconnectCursor{
			AfterEventID: "other-session-cursor",
		},
		Scope: factorydefinitions.FactoryEventReconnectScope{
			SessionID: "factory-session-1",
		},
	})
	if !errors.Is(err, recordings.ErrReconnectCursorNotFound) {
		t.Fatalf(
			"ValidateReconnectReplayFrom error = %v, want ErrReconnectCursorNotFound",
			err,
		)
	}
}
