package testutil

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	petri "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

func (m *MockFactory) WaitToComplete() <-chan struct{} {
	ch := make(chan struct{})
	return ch
}

func (m *MockFactory) SubmitWorkRequestForSession(ctx context.Context, sessionID string, request work.WorkRequest) (work.WorkRequestSubmitResult, error) {
	session, err := m.sessionFactory(sessionID)
	if err != nil {
		return work.WorkRequestSubmitResult{}, err
	}
	return session.SubmitWorkRequest(ctx, request)
}

func (m *MockFactory) SubscribeFactoryEventsForSession(ctx context.Context, sessionID string, reconnect *interfaces.FactoryEventReconnectCursor) (*interfaces.FactoryEventStream, error) {
	session, err := m.sessionFactory(sessionID)
	if err != nil {
		return nil, err
	}
	return session.SubscribeFactoryEvents(ctx, reconnect, interfaces.FactoryEventReconnectScope{SessionID: sessionID})
}

func (m *MockFactory) ProbeFactoryEventsForSession(ctx context.Context, sessionID string, reconnect *interfaces.FactoryEventReconnectCursor) error {
	probeCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	_, err := m.SubscribeFactoryEventsForSession(probeCtx, sessionID, reconnect)
	return err
}

func (m *MockFactory) SubscribeFactoryResponseEventsForSession(
	ctx context.Context,
	request factorysessions.ResponseEventSubscriptionRequest,
) (apisurface.FactoryResponseEventSubscription, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if m != nil && m.SubscribeFactoryResponseEventsFunc != nil {
		return m.SubscribeFactoryResponseEventsFunc(ctx, request)
	}
	return nil, factorysessions.ErrRuntimeNotAvailable
}

func (m *MockFactory) GetEngineStateSnapshotForSession(ctx context.Context, sessionID string) (*interfaces.EngineStateSnapshot[petri.PetriMarkingSnapshot, *petri.Net], error) {
	session, err := m.sessionFactory(sessionID)
	if err != nil {
		return nil, err
	}
	return session.GetEngineStateSnapshot(ctx)
}

func (m *MockFactory) GetCurrentFactoryForSession(ctx context.Context, sessionID string) (factoryapi.Factory, error) {
	session, err := m.sessionFactory(sessionID)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	return session.GetCurrentFactory(ctx)
}

func (m *MockFactory) InvokeFactorySession(_ context.Context, sessionID string, request factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
	if m.InvokeFactoryErr != nil {
		return apisurface.FactoryInvocationResult{}, m.InvokeFactoryErr
	}
	if _, err := m.sessionFactory(sessionID); err != nil {
		return apisurface.FactoryInvocationResult{}, err
	}
	m.InvokedFactorySessionIDs = append(m.InvokedFactorySessionIDs, sessionID)
	m.InvokedFactorySessions = append(m.InvokedFactorySessions, request)
	return m.InvokeFactoryResult, nil
}

func (m *MockFactory) SaveCurrentFactoryForSession(
	ctx context.Context,
	sessionID string,
	request factoryapi.Factory,
) (factoryapi.Factory, error) {
	return m.SaveFactoryForSession(ctx, sessionID, factoryapi.FactorySaveModeReplaceCurrent, request)
}

func (m *MockFactory) sessionFactory(sessionID string) (*MockFactory, error) {
	if m == nil {
		return nil, apisurface.ErrFactorySessionNotFound
	}
	if sessionID == "" || sessionID == "~default" {
		if m.SessionFactories == nil {
			return m, nil
		}
		if session, ok := m.SessionFactories["~default"]; ok && session != nil {
			return session, nil
		}
		return m, nil
	}
	if m.SessionFactories == nil {
		return nil, apisurface.ErrFactorySessionNotFound
	}
	session, ok := m.SessionFactories[sessionID]
	if !ok || session == nil {
		return nil, apisurface.ErrFactorySessionNotFound
	}
	return session, nil
}
