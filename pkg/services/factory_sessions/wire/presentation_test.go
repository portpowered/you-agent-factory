package wire

import (
	"context"
	"io"
	"sync/atomic"
	"testing"
	"time"

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
		durable   bool
		finalIDs  []string
	}{
		{name: "live session", finalIDs: []string{"history", "live", "final"}},
		{name: "durable session", sessionID: "durable-1", durable: true, finalIDs: []string{"history", "live", "final"}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			owner := NewOpeningPresentationOwner()
			presented := make(chan string, 8)
			scopeID, err := owner.RegisterInvocationEvents(factorysessions.InvocationEventScope{
				FactorySessionID: testCase.sessionID,
				Consume: func(events []factorydefinitions.FactoryEvent) {
					for _, event := range events {
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
					presentationEvent("history"), presentationEvent("live"), presentationEvent("final"),
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
			for _, want := range testCase.finalIDs[1:] {
				assertPresentedID(t, presented, want)
			}
			select {
			case duplicate := <-presented:
				t.Fatalf("duplicate event presented: %q", duplicate)
			default:
			}
			if testCase.durable {
				if service.durableCalls != 1 || service.liveCalls != 1 {
					t.Fatalf("durable/live reads = %d/%d, want 1/1", service.durableCalls, service.liveCalls)
				}
			} else if service.liveCalls != 2 || service.durableCalls != 0 {
				t.Fatalf("live/durable reads = %d/%d, want 2/0", service.liveCalls, service.durableCalls)
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
}

func (stub *invocationEventServiceStub) SubscribeFactoryEventsForSession(
	context.Context,
	string,
	*factorydefinitions.FactoryEventReconnectCursor,
) (*factorydefinitions.FactoryEventStream, error) {
	stub.liveCalls++
	if stub.liveCalls == 1 {
		return stub.live, nil
	}
	return stub.final, nil
}

func (stub *invocationEventServiceStub) ReadDurableFactorySessionEventStream(
	context.Context,
	string,
	factorysessions.EventReconnectRequest,
) (*factorydefinitions.FactoryEventStream, error) {
	stub.durableCalls++
	return stub.final, nil
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
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for event %q", want)
	}
}

type nilReader struct{}

func (nilReader) Read([]byte) (int, error) { return 0, io.EOF }
