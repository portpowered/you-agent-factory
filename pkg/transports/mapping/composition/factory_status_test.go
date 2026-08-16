package composition

import (
	"context"
	"errors"
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

type factoryStatusRuntimeRole struct {
	factoryruntime.Service
	observation  factoryruntime.Observation
	observations int
}

type factoryStatusProjectorRole struct {
	status      factoryruntime.FactoryStatus
	projections int
}

type factoryStatusSessionRole struct {
	observations int
	sessionIDs   []string
}

func (role *factoryStatusRuntimeRole) Observe(_ context.Context, _ factoryruntime.ObserveRequest) (factoryruntime.ObserveResult, error) {
	role.observations++
	return factoryruntime.ObserveResult{Observation: role.observation}, nil
}

func (role *factoryStatusProjectorRole) ProjectFactoryStatusFromObservation(factoryruntime.Observation) factoryruntime.FactoryStatus {
	role.projections++
	return role.status
}

func (role *factoryStatusSessionRole) ObserveForSession(
	_ context.Context,
	sessionID string,
	_ factoryruntime.ObserveRequest,
) (factoryruntime.ObserveResult, error) {
	role.observations++
	role.sessionIDs = append(role.sessionIDs, sessionID)
	if sessionID == "missing" {
		return factoryruntime.ObserveResult{}, factorysessions.ErrSessionNotFound
	}
	return factoryruntime.ObserveResult{
		Observation: factoryruntime.Observation{
			Health: factoryruntime.ObservationHealth{FactoryState: "SESSION_" + sessionID},
		},
	}, nil
}

func TestFactoryStatusAPIUsesBoundSessionRuntimeForCurrentAndSessionRoutes(t *testing.T) {
	scopedObservation := factoryruntime.Observation{
		Status: factoryruntime.ObservationStatusActive,
		Progress: factoryruntime.ObservationProgress{
			TotalWorkCount: 99,
			WorkCategories: factoryruntime.ObservationWorkCategories{Processing: 7, Terminal: 92},
		},
		Health: factoryruntime.ObservationHealth{FactoryState: "OBSERVATION_SOURCE"},
	}
	runtime := &factoryStatusRuntimeRole{observation: scopedObservation}
	sessions := &factoryStatusSessionRole{}
	projector := &factoryStatusProjectorRole{status: factoryruntime.FactoryStatus{
		FactoryState:  "CURRENT",
		RuntimeStatus: "ACTIVE",
		TotalTokens:   2,
		Categories:    factoryruntime.FactoryStatusCategories{Processing: 1},
	}}
	api := newFactoryStatusAPI(sessions, projector)

	got, err := api.ProjectFactoryStatus(context.Background(), "")
	if err != nil || got.FactoryState != "CURRENT" || got.RuntimeStatus != "ACTIVE" ||
		got.TotalTokens != 2 || got.Categories.Processing != 1 || runtime.observations != 0 || sessions.observations != 1 || projector.projections != 1 {
		t.Fatalf("default status = (%#v, %v), observations = %d, projections = %d", got, err, runtime.observations, projector.projections)
	}
	got, err = api.ProjectFactoryStatus(context.Background(), "session-beta")
	if err != nil || got.FactoryState != "CURRENT" || got.TotalTokens != 2 || runtime.observations != 0 || sessions.observations != 2 || projector.projections != 2 {
		t.Fatalf("scoped status = (%#v, %v), runtime observations = %d, session observations = %d, projections = %d", got, err, runtime.observations, sessions.observations, projector.projections)
	}
	if len(sessions.sessionIDs) != 2 || sessions.sessionIDs[0] != factorysessions.DefaultSessionID || sessions.sessionIDs[1] != "session-beta" {
		t.Fatalf("session IDs = %#v, want [~default session-beta]", sessions.sessionIDs)
	}
}

func TestFactoryStatusAPIReturnsMissingSessionError(t *testing.T) {
	t.Parallel()

	api := newFactoryStatusAPI(
		&factoryStatusSessionRole{},
		&factoryStatusProjectorRole{},
	)
	_, err := api.ProjectFactoryStatus(context.Background(), "missing")
	if err == nil || !errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("missing session error = %v, want ErrSessionNotFound", err)
	}
}
