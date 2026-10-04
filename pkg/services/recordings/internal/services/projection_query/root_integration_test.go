package projectionquery_test

import (
	"errors"
	"reflect"
	"testing"

	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"

	"github.com/portpowered/infinite-you/pkg/services/recordings/internal/canonical"
	projectionquerywire "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/projection_query/wire"
)

func TestProjectionQueryRejectsMalformedPayloadAndValidatesScopedReconnect(t *testing.T) {
	t.Parallel()

	root := projectionquerywire.NewService()
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

	result, err := root.ReconstructFactoryWorldState([]recordings.FactoryEvent{
		canonical.FactoryEventFromCanonical(malformed),
	}, 1)
	if !errors.Is(err, recordings.ErrInvalidProjectionInput) {
		t.Fatalf("ReconstructWorldState error = %v, want ErrInvalidProjectionInput", err)
	}
	if !reflect.DeepEqual(result, recordings.FactoryWorldState{}) {
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
	events := []recordings.FactoryEvent{
		canonical.FactoryEventFromCanonical(history[0]),
		canonical.FactoryEventFromCanonical(history[1]),
	}
	afterSequence := 0
	cursor := recordings.FactoryEventReconnectCursor{AfterSequence: &afterSequence}
	reconnectScope := recordings.FactoryEventReconnectScope{SessionID: string(scope.FactorySessionID)}
	err = root.ValidateReconnectReplay(events, cursor, reconnectScope)
	if err != nil {
		t.Fatalf("ValidateReconnectReplayFrom interleaved scoped history: %v", err)
	}

	err = root.ValidateReconnectReplay(events[1:], cursor, reconnectScope)
	if !errors.Is(err, recordings.ErrReconnectCursorNotFound) {
		t.Fatalf(
			"ValidateReconnectReplayFrom continuation-only error = %v, want ErrReconnectCursorNotFound",
			err,
		)
	}
}
