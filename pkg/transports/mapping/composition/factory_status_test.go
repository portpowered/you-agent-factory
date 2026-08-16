package composition

import (
	"context"
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

type factoryStatusSessionRole struct {
	observation factoryruntime.Observation
	sessionID   string
}

func (role *factoryStatusSessionRole) ObserveForSession(_ context.Context, sessionID string, _ factoryruntime.ObserveRequest) (factoryruntime.ObserveResult, error) {
	role.sessionID = sessionID
	return factoryruntime.ObserveResult{Observation: role.observation}, nil
}

func TestFactoryStatusAPIRoutesCurrentAndSessionObservationsThroughNeutralProjection(t *testing.T) {
	scopedObservation := factoryruntime.Observation{
		Status: factoryruntime.ObservationStatusActive,
		Progress: factoryruntime.ObservationProgress{
			TotalWorkCount: 2,
			WorkCategories: factoryruntime.ObservationWorkCategories{Processing: 1, Terminal: 1},
		},
		Health: factoryruntime.ObservationHealth{FactoryState: "SCOPED"},
	}
	sessions := &factoryStatusSessionRole{observation: scopedObservation}
	api := newFactoryStatusAPI(sessions)

	got, err := api.ProjectFactoryStatus(context.Background(), "")
	if err != nil || got.FactoryState != "SCOPED" || got.RuntimeStatus != "ACTIVE" ||
		got.TotalTokens != 2 || got.Categories.Processing != 1 || sessions.sessionID != factorysessions.DefaultSessionID {
		t.Fatalf("default status = (%#v, %v), session = %q", got, err, sessions.sessionID)
	}
	got, err = api.ProjectFactoryStatus(context.Background(), "session-beta")
	if err != nil || got.FactoryState != "SCOPED" || got.TotalTokens != 2 || sessions.sessionID != "session-beta" {
		t.Fatalf("scoped status = (%#v, %v), session = %q", got, err, sessions.sessionID)
	}
}
