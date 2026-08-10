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

func TestOpeningPresentationOwnerScopesDynamicCollaboratorsAndClosesThem(t *testing.T) {
	owner := NewOpeningPresentationOwner()
	var appObserved, directObserved atomic.Int32
	appID, err := owner.RegisterApplication(factorysessions.ApplicationOpeningScope{
		RuntimeHostObserver: func(factorysessions.RuntimeHostBinding) { appObserved.Add(1) },
	})
	if err != nil {
		t.Fatalf("RegisterApplication: %v", err)
	}
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

	owner.ObserveHost(appID, factorysessions.RuntimeHostBinding{Port: 1})
	owner.ObserveHost(directID, factorysessions.RuntimeHostBinding{Port: 2})
	if appObserved.Load() != 1 || directObserved.Load() != 1 {
		t.Fatalf("host observations = app:%d direct:%d", appObserved.Load(), directObserved.Load())
	}
	if _, ok := owner.Application(appID); !ok {
		t.Fatal("application scope was not retained")
	}
	if _, ok := owner.DirectJavaScript(directID); !ok {
		t.Fatal("direct JavaScript scope was not retained")
	}
	if _, ok := owner.Stdio(stdioID); !ok {
		t.Fatal("stdio scope was not retained")
	}

	owner.Close(appID)
	owner.Close(directID)
	owner.Close(stdioID)
	if _, ok := owner.Application(appID); ok {
		t.Fatal("application scope survived Close")
	}
	if _, ok := owner.DirectJavaScript(directID); ok {
		t.Fatal("direct JavaScript scope survived Close")
	}
	if _, ok := owner.Stdio(stdioID); ok {
		t.Fatal("stdio scope survived Close")
	}
}

func TestOpeningPresentationOwnerGatesApplicationCompletionOnHostBinding(t *testing.T) {
	var completed atomic.Int32
	started := make(chan struct{}, 1)
	owner := NewOpeningPresentationOwner()
	id, err := owner.RegisterApplication(factorysessions.ApplicationOpeningScope{
		Completion: func(context.Context) error {
			started <- struct{}{}
			completed.Add(1)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("RegisterApplication: %v", err)
	}
	scope, ok := owner.Application(id)
	if !ok {
		t.Fatal("application scope was not registered")
	}
	result := make(chan error, 1)
	go func() { result <- scope.Completion(context.Background()) }()
	select {
	case <-started:
		t.Fatal("completion ran before host binding")
	default:
	}
	owner.ObserveHost(id, factorysessions.RuntimeHostBinding{Port: 7437})
	if err := <-result; err != nil {
		t.Fatalf("completion: %v", err)
	}
	if completed.Load() != 1 {
		t.Fatalf("completion calls = %d, want 1", completed.Load())
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
			if err := bridge.Finish(t.Context(), service, factorysessions.FactoryInvocationOutcome{
				Result: factorydefinitions.FactoryInvocationResult{SessionID: testCase.sessionID},
			}); err != nil {
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
