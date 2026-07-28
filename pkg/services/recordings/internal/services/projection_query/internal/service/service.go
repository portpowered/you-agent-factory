package service

import (
	"fmt"
	"strings"

	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingevents "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/canonical_ledger/events"
	projectionquery "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/projection_query"
	"github.com/portpowered/infinite-you/pkg/services/recordings/projections"
	dashboardprojections "github.com/portpowered/infinite-you/pkg/services/recordings/projections/dashboard"
)

// Service keeps the canonical reducer and derived-query implementation behind
// the Recordings-owned projection-query capability.
type Service struct{}

var _ projectionquery.Service = (*Service)(nil)

// New constructs the stateless projection-query service.
func New() *Service {
	return &Service{}
}

func (*Service) ReconstructFactoryWorldState(
	events []recordings.FactoryEvent,
	selectedTick int,
) (recordings.FactoryWorldState, error) {
	if selectedTick < 0 {
		return recordings.FactoryWorldState{}, recordings.ErrInvalidProjectionInput
	}
	state, err := projections.ReconstructCanonicalFactoryWorldState(events, selectedTick)
	if err != nil {
		return recordings.FactoryWorldState{}, fmt.Errorf(
			"%w: %v",
			recordings.ErrInvalidProjectionInput,
			err,
		)
	}
	return state, nil
}

func (*Service) SimpleDashboardRenderData(
	state recordings.FactoryWorldState,
) recordings.SimpleDashboardRenderData {
	return dashboardprojections.SimpleDashboardRenderDataFromWorldState(state)
}

func (*Service) ProjectActiveThrottlePauses(
	topology recordings.InitialStructurePayload,
	pauses []recordings.ActiveThrottlePause,
) []recordings.FactoryWorldThrottlePause {
	return projections.ProjectActiveThrottlePauses(topology, pauses)
}

func (*Service) ProjectWorkstationRequests(
	state recordings.FactoryWorldState,
) recordings.WorkstationFactoryWorldWorkstationRequestProjectionSlice {
	return recordings.BuildFactoryWorldWorkstationRequestProjectionSlice(state)
}

func (*Service) ValidateReconnectReplay(
	events []recordings.FactoryEvent,
	cursor recordings.FactoryEventReconnectCursor,
	scope recordings.FactoryEventReconnectScope,
) error {
	_, err := reconnectReplay(events, cursor, scope)
	return err
}

// reconnectReplay is the single private continuation path used by validation
// and focused projection-query evidence. The canonical Recordings builder owns
// cursor ordering and dispatch reconciliation; this boundary additionally
// enforces the requested Factory Session scope.
func reconnectReplay(
	events []recordings.FactoryEvent,
	cursor recordings.FactoryEventReconnectCursor,
	scope recordings.FactoryEventReconnectScope,
) ([]recordings.FactoryEvent, error) {
	sessionID := strings.TrimSpace(scope.SessionID)
	if sessionID != "" && !cursorBelongsToSession(events, cursor, sessionID) {
		return nil, reconnectCursorNotFound(cursor, sessionID)
	}

	replay, err := recordingevents.BuildCanonicalReconnectReplay(events, cursor, scope)
	if err != nil {
		return nil, err
	}
	if sessionID == "" {
		return replay, nil
	}

	scoped := make([]recordings.FactoryEvent, 0, len(replay))
	for _, event := range replay {
		if eventBelongsToSession(event, sessionID) {
			scoped = append(scoped, event)
		}
	}
	return scoped, nil
}

func cursorBelongsToSession(
	events []recordings.FactoryEvent,
	cursor recordings.FactoryEventReconnectCursor,
	sessionID string,
) bool {
	if afterEventID := strings.TrimSpace(cursor.AfterEventID); afterEventID != "" {
		for _, event := range events {
			if event.Id == afterEventID && eventBelongsToSession(event, sessionID) {
				return true
			}
		}
		return false
	}
	if cursor.AfterSequence == nil {
		return true
	}
	for _, event := range events {
		if !eventBelongsToSession(event, sessionID) {
			continue
		}
		sequence := event.Context.Sequence
		if event.Context.SessionSequence != nil {
			sequence = *event.Context.SessionSequence
		}
		if sequence == *cursor.AfterSequence {
			return true
		}
	}
	return false
}

func eventBelongsToSession(event recordings.FactoryEvent, sessionID string) bool {
	return event.Context.SessionID != nil &&
		strings.TrimSpace(*event.Context.SessionID) == sessionID
}

func reconnectCursorNotFound(
	cursor recordings.FactoryEventReconnectCursor,
	sessionID string,
) error {
	if afterEventID := strings.TrimSpace(cursor.AfterEventID); afterEventID != "" {
		return fmt.Errorf(
			"%w: after_event_id %q for session %q",
			recordings.ErrReconnectCursorNotFound,
			afterEventID,
			sessionID,
		)
	}
	return fmt.Errorf(
		"%w: after_sequence %d for session %q",
		recordings.ErrReconnectCursorNotFound,
		*cursor.AfterSequence,
		sessionID,
	)
}
