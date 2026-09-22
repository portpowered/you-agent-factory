package service

import (
	"context"
	"errors"
	"reflect"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

func TestServiceSubscribeFactoryEventsForSession_UsesResolvedSessionIdentity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		requestedID      string
		sessionID        string
		runtimeSessionID string
		eventScopeID     string
		logicalKeyID     string
	}{
		{
			name:             "exact id",
			requestedID:      "session-exact-001",
			sessionID:        "session-exact-001",
			runtimeSessionID: "session-exact-001",
			eventScopeID:     "session-exact-001",
			logicalKeyID:     "logical-exact-001",
		},
		{
			name:             "default alias",
			requestedID:      factorysessions.DefaultSessionID,
			sessionID:        factorysessions.DefaultSessionID,
			runtimeSessionID: "runtime-default-001",
			eventScopeID:     "restored-event-scope-001",
			logicalKeyID:     "logical-default-001",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			events := make(chan interfaces.FactoryEvent)
			wantHistory := []interfaces.FactoryEvent{{Id: "event-001"}}
			stream := &interfaces.FactoryEventStream{
				StreamGenerationID: "generation-001",
				History:            wantHistory,
				Events:             events,
			}
			runtime := &gatewayEventRuntimeFake{stream: stream}
			session := &livesession.LiveSession{
				ID:                      test.sessionID,
				RuntimeFactorySessionID: test.runtimeSessionID,
				RuntimeEventSessionID:   test.eventScopeID,
			}
			service := &Service{
				host: dependencyHost{
					requireSession: func(string) (*livesession.LiveSession, error) {
						return session, nil
					},
					sessionFactory: func(string) (factoryruntime.Service, error) {
						return runtime, nil
					},
					backendScopeID: func() string { return "backend-001" },
					logicalSessionKeyID: func(got *livesession.LiveSession) string {
						if got != session {
							t.Fatalf("logical key resolved session = %p, want %p", got, session)
						}
						return test.logicalKeyID
					},
				},
			}

			sequence := 3
			reconnect := &interfaces.FactoryEventReconnectCursor{AfterSequence: &sequence}
			got, err := service.SubscribeFactoryEventsForSession(
				context.Background(), test.requestedID, reconnect,
			)
			if err != nil {
				t.Fatalf("SubscribeFactoryEventsForSession() error = %v", err)
			}
			if got != stream {
				t.Fatalf("stream pointer = %p, want original %p", got, stream)
			}
			if got.BackendScopeID != "backend-001" {
				t.Fatalf("backend scope = %q, want backend-001", got.BackendScopeID)
			}
			if got.LogicalSessionKeyID != test.logicalKeyID {
				t.Fatalf("logical session key = %q, want %q", got.LogicalSessionKeyID, test.logicalKeyID)
			}
			if got.FactorySessionID != test.runtimeSessionID {
				t.Fatalf("Factory Session ID = %q, want %q", got.FactorySessionID, test.runtimeSessionID)
			}
			if got.StreamGenerationID != "generation-001" || !reflect.DeepEqual(got.History, wantHistory) || got.Events != events {
				t.Fatalf("stream behavior changed: got generation=%q history=%#v events=%p", got.StreamGenerationID, got.History, got.Events)
			}
			if runtime.scope.SessionID != test.eventScopeID {
				t.Fatalf("event scope = %q, want %q", runtime.scope.SessionID, test.eventScopeID)
			}
			if runtime.reconnect != reconnect {
				t.Fatalf("reconnect cursor pointer = %p, want %p", runtime.reconnect, reconnect)
			}
		})
	}
}

func TestServiceSubscribeFactoryEventsForSession_DoesNotFabricateIdentityWhenResolutionFails(t *testing.T) {
	t.Parallel()

	stream := &interfaces.FactoryEventStream{}
	runtime := &gatewayEventRuntimeFake{stream: stream}
	service := &Service{
		host: dependencyHost{
			requireSession: func(string) (*livesession.LiveSession, error) {
				return nil, errors.New("session resolution failed")
			},
			sessionFactory: func(string) (factoryruntime.Service, error) {
				return runtime, nil
			},
			backendScopeID: func() string { return "backend-001" },
			logicalSessionKeyID: func(*livesession.LiveSession) string {
				return "must-not-be-called"
			},
		},
	}

	got, err := service.SubscribeFactoryEventsForSession(context.Background(), "missing-session", nil)
	if err != nil {
		t.Fatalf("SubscribeFactoryEventsForSession() error = %v", err)
	}
	if got != stream {
		t.Fatalf("stream pointer = %p, want original %p", got, stream)
	}
	if got.LogicalSessionKeyID != "" || got.FactorySessionID != "" {
		t.Fatalf("resolution failure fabricated identity: %#v", got)
	}
}

type gatewayEventRuntimeFake struct {
	factoryruntime.Service
	stream    *interfaces.FactoryEventStream
	err       error
	scope     interfaces.FactoryEventReconnectScope
	reconnect *interfaces.FactoryEventReconnectCursor
}

func (fake *gatewayEventRuntimeFake) SubmitWorkRequest(
	context.Context,
	work.WorkRequest,
) (work.WorkRequestSubmitResult, error) {
	return work.WorkRequestSubmitResult{}, nil
}

func (fake *gatewayEventRuntimeFake) SubscribeFactoryEvents(
	_ context.Context,
	reconnect *interfaces.FactoryEventReconnectCursor,
	scope interfaces.FactoryEventReconnectScope,
) (*interfaces.FactoryEventStream, error) {
	fake.scope = scope
	fake.reconnect = reconnect
	return fake.stream, fake.err
}
