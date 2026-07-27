package service

import (
	"fmt"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingevents "github.com/portpowered/infinite-you/pkg/services/recordings/events"
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
	events []factorydefinitions.FactoryEvent,
	selectedTick int,
) (factorydefinitions.FactoryWorldState, error) {
	if selectedTick < 0 {
		return factorydefinitions.FactoryWorldState{}, recordings.ErrInvalidProjectionInput
	}
	state, err := projections.ReconstructCanonicalFactoryWorldState(events, selectedTick)
	if err != nil {
		return factorydefinitions.FactoryWorldState{}, fmt.Errorf(
			"%w: %v",
			recordings.ErrInvalidProjectionInput,
			err,
		)
	}
	return state, nil
}

func (*Service) SimpleDashboardRenderData(
	state factorydefinitions.FactoryWorldState,
) recordings.SimpleDashboardRenderData {
	return dashboardprojections.SimpleDashboardRenderDataFromWorldState(state)
}

func (*Service) ProjectActiveThrottlePauses(
	topology factorydefinitions.InitialStructurePayload,
	pauses []factorydefinitions.ActiveThrottlePause,
) []factorydefinitions.FactoryWorldThrottlePause {
	return projections.ProjectActiveThrottlePauses(topology, pauses)
}

func (*Service) ProjectWorkstationRequests(
	state factorydefinitions.FactoryWorldState,
) recordings.WorkstationFactoryWorldWorkstationRequestProjectionSlice {
	return recordings.BuildFactoryWorldWorkstationRequestProjectionSlice(state)
}

func (*Service) ValidateReconnectReplay(
	events []factorydefinitions.FactoryEvent,
	cursor factorydefinitions.FactoryEventReconnectCursor,
	scope factorydefinitions.FactoryEventReconnectScope,
) error {
	_, err := reconnectReplay(events, cursor, scope)
	return err
}

// reconnectReplay is the single private continuation path used by validation
// and focused projection-query evidence. The canonical Recordings builder owns
// cursor ordering and dispatch reconciliation; this boundary additionally
// enforces the requested Factory Session scope.
func reconnectReplay(
	events []factorydefinitions.FactoryEvent,
	cursor factorydefinitions.FactoryEventReconnectCursor,
	scope factorydefinitions.FactoryEventReconnectScope,
) ([]factorydefinitions.FactoryEvent, error) {
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

	scoped := make([]factorydefinitions.FactoryEvent, 0, len(replay))
	for _, event := range replay {
		if eventBelongsToSession(event, sessionID) {
			scoped = append(scoped, event)
		}
	}
	return scoped, nil
}

func cursorBelongsToSession(
	events []factorydefinitions.FactoryEvent,
	cursor factorydefinitions.FactoryEventReconnectCursor,
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

func eventBelongsToSession(event factorydefinitions.FactoryEvent, sessionID string) bool {
	return event.Context.SessionID != nil &&
		strings.TrimSpace(*event.Context.SessionID) == sessionID
}

func reconnectCursorNotFound(
	cursor factorydefinitions.FactoryEventReconnectCursor,
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
