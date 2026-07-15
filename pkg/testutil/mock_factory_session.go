package testutil

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/work"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/responseeventstore"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
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

func (m *MockFactory) SubscribeFactoryResponseEventsForSession(ctx context.Context, sessionID string, afterSequence int64, dispatchID string) (apisurface.FactoryResponseEventSubscription, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	session, err := m.sessionFactory(sessionID)
	if err != nil {
		return nil, err
	}
	if session.ResponseEventSubscribeErr != nil {
		return nil, session.ResponseEventSubscribeErr
	}
	if session.ResponseEventStore == nil {
		return nil, fmt.Errorf("response event store unavailable for factory session %q", sessionID)
	}
	options := []responseeventstore.SubscribeOption{}
	if strings.TrimSpace(dispatchID) != "" {
		options = append(options, responseeventstore.WithDispatchFilter(dispatchID))
	}
	subscription, err := session.ResponseEventStore.Subscribe(afterSequence, options...)
	if err != nil {
		if errors.Is(err, responseeventstore.ErrStoreExpired) {
			return nil, fmt.Errorf("%w: %s", apisurface.ErrFactoryResponseEventStreamExpired, sessionID)
		}
		return nil, err
	}
	return &mockFactoryResponseEventSubscription{subscription: subscription}, nil
}

type mockFactoryResponseEventSubscription struct {
	subscription *responseeventstore.Subscription
}

func (s *mockFactoryResponseEventSubscription) Next(ctx context.Context) ([]apisurface.FactoryResponseEventRecord, error) {
	events, err := s.subscription.Next(ctx)
	if err != nil {
		return nil, err
	}
	records := make([]apisurface.FactoryResponseEventRecord, 0, len(events))
	for _, event := range events {
		data, err := json.Marshal(event)
		if err != nil {
			return nil, fmt.Errorf("serialize factory response event: %w", err)
		}
		records = append(records, apisurface.FactoryResponseEventRecord{Sequence: event.Sequence, Kind: string(event.Kind), Data: data})
	}
	return records, nil
}

func (s *mockFactoryResponseEventSubscription) Detach() {
	if s != nil && s.subscription != nil {
		s.subscription.Detach()
	}
}

func (m *MockFactory) GetEngineStateSnapshotForSession(ctx context.Context, sessionID string) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
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
