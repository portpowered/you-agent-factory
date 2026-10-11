package invocation

import (
	"context"
	"io"
	"reflect"
	"sync/atomic"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

func TestOpeningPresentationOwnerScopesTransportCollaboratorsAndClosesThem(t *testing.T) {
	owner := NewOpeningPresentationOwner()
	var directObserved atomic.Int32
	var err error
	directID, err := owner.RegisterDirectJavaScript(factorysessions.DirectJavaScriptRunScope{
		Output: io.Discard,
		RuntimeHostObserver: func(factorysessions.RuntimeHostBinding) {
			directObserved.Add(1)
		},
	})
	if err != nil {
		t.Fatalf("RegisterDirectJavaScript: %v", err)
	}
	stdioID, err := owner.RegisterStdio(factorysessions.StdioOpeningScope{Input: nilReader{}, Output: io.Discard})
	if err != nil {
		t.Fatalf("RegisterStdio: %v", err)
	}

	direct, ok := owner.DirectJavaScript(directID)
	if !ok || direct.RuntimeHostObserver == nil {
		t.Fatal("direct JavaScript scope was not retained")
	}
	direct.RuntimeHostObserver(factorysessions.RuntimeHostBinding{Port: 2})
	if directObserved.Load() != 1 {
		t.Fatalf("host observations = %d, want 1", directObserved.Load())
	}
	if _, ok := owner.Stdio(stdioID); !ok {
		t.Fatal("stdio scope was not retained")
	}

	owner.Close(directID)
	owner.Close(stdioID)
	if _, ok := owner.DirectJavaScript(directID); ok {
		t.Fatal("direct JavaScript scope survived Close")
	}
	if _, ok := owner.Stdio(stdioID); ok {
		t.Fatal("stdio scope survived Close")
	}
}

func TestOpeningPresentationOwnerInvocationBridgeStreamsAndReconcilesHistory(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		sessionID string
		finalIDs  []string
	}{
		{name: "live session", finalIDs: []string{"history", "live", "final"}},
		{name: "named live session with unpublished durable dispatch", sessionID: "durable-1", finalIDs: []string{"history", "live", "final"}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			owner := NewOpeningPresentationOwner()
			presented := make(chan string, 8)
			var finalEvent factorydefinitions.FactoryEvent
			committedFailure := factorydefinitions.FactoryEvent{
				Id: "final", Type: factorydefinitions.FactoryEventTypeDispatchResponse,
				Payload: []byte(`{"transitionId":"execute-goal","outcome":"FAILED","error":"deterministic provider rejection"}`),
			}
			scopeID, err := owner.RegisterInvocationEvents(factorysessions.InvocationEventScope{
				FactorySessionID: testCase.sessionID,
				Consume: func(events []factorydefinitions.FactoryEvent) {
					for _, event := range events {
						if event.Id == committedFailure.Id {
							finalEvent = event
						}
						presented <- event.Id
					}
				},
			})
			if err != nil {
				t.Fatalf("RegisterInvocationEvents: %v", err)
			}
			liveEvents := make(chan factorydefinitions.FactoryEvent, 1)
			service := &invocationEventServiceStub{
				live: &factorydefinitions.FactoryEventStream{
					History: []factorydefinitions.FactoryEvent{presentationEvent("history")},
					Events:  liveEvents,
				},
				final: &factorydefinitions.FactoryEventStream{History: []factorydefinitions.FactoryEvent{
					presentationEvent("history"), presentationEvent("live"), committedFailure,
				}},
			}
			bridge, err := owner.StartFactoryEventBridge(t.Context(), service, scopeID)
			if err != nil {
				t.Fatalf("StartFactoryEventBridge: %v", err)
			}
			assertPresentedID(t, presented, "history")
			liveEvents <- presentationEvent("live")
			close(liveEvents)
			// Cancellation can leave the public result without a session ID. The
			// bridge must retain the session selected when it started.
			if err := bridge.Finish(t.Context(), service, factorysessions.FactoryInvocationOutcome{}); err != nil {
				t.Fatalf("Finish: %v", err)
			}
			if service.finalContext == nil || service.finalContext.Err() != context.Canceled {
				t.Fatal("final history read retained its live subscription after Finish")
			}
			if t.Context().Err() != nil {
				t.Fatal("history read canceled its caller context")
			}
			for _, want := range testCase.finalIDs[1:] {
				assertPresentedID(t, presented, want)
			}
			if !reflect.DeepEqual(finalEvent, committedFailure) {
				t.Fatalf("terminal dispatch = %#v, want exact committed failure %#v", finalEvent, committedFailure)
			}
			select {
			case duplicate := <-presented:
				t.Fatalf("duplicate event presented: %q", duplicate)
			default:
			}
			if service.liveCalls != 2 || service.durableCalls != 0 {
				t.Fatalf("live/durable reads = %d/%d, want 2/0", service.liveCalls, service.durableCalls)
			}
			wantSession := testCase.sessionID
			if wantSession == "" {
				wantSession = factorysessions.DefaultSessionID
			}
			if !reflect.DeepEqual(service.sessionIDs, []string{wantSession, wantSession}) {
				t.Fatalf("event scopes = %v, want only %q", service.sessionIDs, wantSession)
			}
		})
	}
}

type invocationEventServiceStub struct {
	factorysessions.Service
	live         *factorydefinitions.FactoryEventStream
	final        *factorydefinitions.FactoryEventStream
	liveCalls    int
	durableCalls int
	finalContext context.Context
	sessionIDs   []string
}

func (stub *invocationEventServiceStub) SubscribeFactoryEventsForSession(
	ctx context.Context,
	sessionID string,
	_ *factorydefinitions.FactoryEventReconnectCursor,
) (*factorydefinitions.FactoryEventStream, error) {
	stub.liveCalls++
	stub.sessionIDs = append(stub.sessionIDs, sessionID)
	if stub.liveCalls == 1 {
		return stub.live, nil
	}
	stub.finalContext = ctx
	return stub.final, nil
}

func (stub *invocationEventServiceStub) ReadDurableFactorySessionEventStream(
	ctx context.Context,
	_ string,
	_ factorysessions.EventReconnectRequest,
) (*factorydefinitions.FactoryEventStream, error) {
	stub.durableCalls++
	stub.finalContext = ctx
	// The durable projection has not published the live terminal dispatch yet.
	return &factorydefinitions.FactoryEventStream{History: []factorydefinitions.FactoryEvent{presentationEvent("history")}}, nil
}

func presentationEvent(id string) factorydefinitions.FactoryEvent {
	return factorydefinitions.FactoryEvent{Id: id}
}

func assertPresentedID(t *testing.T, presented <-chan string, want string) {
	t.Helper()
	select {
	case got := <-presented:
		if got != want {
			t.Fatalf("presented event = %q, want %q", got, want)
		}
	default:
		t.Fatalf("finished presentation lacks event %q", want)
	}
}

type nilReader struct{}

func (nilReader) Read([]byte) (int, error) { return 0, io.EOF }
