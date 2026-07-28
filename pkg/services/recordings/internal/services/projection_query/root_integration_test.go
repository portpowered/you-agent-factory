package projectionquery_test

import (
	"errors"
	"reflect"
	"testing"

	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingsinternal "github.com/portpowered/infinite-you/pkg/services/recordings/internal"
)

type unusedLedger struct {
	recordings.Ledger
}

func TestAcceptedRecordingsRootUsesPrivateProjectionQuery(t *testing.T) {
	t.Parallel()

	root := recordingsinternal.NewService(
		&unusedLedger{},
		recordingsinternal.NewProjectionService(),
	)
	malformed := recordings.CanonicalEvent{
		ID:       "malformed",
		Kind:     "WORK_REQUEST",
		Sequence: 0,
		Cursor: recordings.CanonicalEventCursor{
			StreamGenerationID: "generation-1",
			Sequence:           0,
		},
		FactoryTick: 1,
		Payload:     `{"type":`,
	}

	result, err := root.ReconstructWorldState(recordings.ReconstructWorldStateRequest{
		Events:       []recordings.CanonicalEvent{malformed},
		SelectedTick: 1,
	})
	if !errors.Is(err, recordings.ErrInvalidProjectionInput) {
		t.Fatalf("ReconstructWorldState error = %v, want ErrInvalidProjectionInput", err)
	}
	if !reflect.DeepEqual(result, recordings.ReconstructWorldStateResult{}) {
		t.Fatalf("ReconstructWorldState result = %#v, want zero result", result)
	}

	scope := recordings.CanonicalEventScope{FactorySessionID: "factory-session-1"}
	history := []recordings.CanonicalEvent{
		{
			ID:       "acknowledged",
			Sequence: 0,
			Scope:    scope,
			Cursor: recordings.CanonicalEventCursor{
				StreamGenerationID: "generation-1",
				Sequence:           0,
			},
		},
		{
			ID:       "continuation",
			Sequence: 2,
			Scope:    scope,
			Cursor: recordings.CanonicalEventCursor{
				StreamGenerationID: "generation-1",
				Sequence:           2,
			},
		},
	}
	err = root.ValidateReconnectReplayFrom(recordings.ValidateReconnectReplayRequest{
		Events: history,
		Cursor: history[0].Cursor,
		Scope:  scope,
	})
	if err != nil {
		t.Fatalf("ValidateReconnectReplayFrom interleaved scoped history: %v", err)
	}

	err = root.ValidateReconnectReplayFrom(recordings.ValidateReconnectReplayRequest{
		Events: history[1:],
		Cursor: history[0].Cursor,
		Scope:  scope,
	})
	if !errors.Is(err, recordings.ErrReconnectCursorNotFound) {
		t.Fatalf(
			"ValidateReconnectReplayFrom continuation-only error = %v, want ErrReconnectCursorNotFound",
			err,
		)
	}
}
