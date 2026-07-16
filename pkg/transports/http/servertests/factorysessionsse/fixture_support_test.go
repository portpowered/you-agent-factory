package factorysessionsse

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const (
	factorySessionSSEFixtureSessionID          = "b08-sse-fixture-session"
	factorySessionSSEFixtureStreamGenerationID = "b08-sse-fixture-stream-gen-001"
	factorySessionSSEFixtureNextGenerationID   = "b08-sse-fixture-stream-gen-002"
	factorySessionSSEFixtureBackendScopeID     = "b08-sse-fixture-backend-scope"
	factorySessionSSEFixtureLogicalSessionKey  = "b08-sse-fixture-logical-key"

	factorySessionSSEFixtureRetainedEventOneID   = "b08-sse-fixture/run-started"
	factorySessionSSEFixtureRetainedEventTwoID   = "b08-sse-fixture/initial-structure"
	factorySessionSSEFixtureRetainedEventThreeID = "b08-sse-fixture/work-request"
	factorySessionSSEFixtureLiveEventID          = "b08-sse-fixture/dispatch-live"
	factorySessionSSEFixtureNextRetainedEventID  = "b08-sse-fixture-next/run-started"
)

var factorySessionSSEFixtureEventTime = time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)

// FactorySessionSSEFixture seeds a deterministic live Factory Session with stable
// retained FactoryEvent history and a buffered live-events channel for
// retained-then-live SSE integration coverage.
type FactorySessionSSEFixture struct {
	SessionID string
	Retained  []factoryapi.FactoryEvent
	domain    []interfaces.FactoryEvent
	Live      chan interfaces.FactoryEvent
}

// NewFactorySessionSSEFixture returns the canonical B08 session-scoped SSE fixture
// with three ordered retained public FactoryEvent records and a live channel.
func NewFactorySessionSSEFixture(t *testing.T) *FactorySessionSSEFixture {
	t.Helper()
	retained := factorySessionSSEFixtureRetainedEvents(t)
	live := make(chan interfaces.FactoryEvent, 4)
	return &FactorySessionSSEFixture{
		SessionID: factorySessionSSEFixtureSessionID,
		Retained:  retained,
		domain:    testutil.FactoryEvents(t, retained),
		Live:      live,
	}
}

// RootMockFactory wires the fixture into a session-scoped MockFactory root.
func (f *FactorySessionSSEFixture) RootMockFactory() *testutil.MockFactory {
	return &testutil.MockFactory{
		SessionFactories: map[string]*testutil.MockFactory{
			f.SessionID: {
				FactoryEventStream: &interfaces.FactoryEventStream{
					StreamGenerationID:  factorySessionSSEFixtureStreamGenerationID,
					BackendScopeID:      factorySessionSSEFixtureBackendScopeID,
					LogicalSessionKeyID: factorySessionSSEFixtureLogicalSessionKey,
					FactorySessionID:    f.SessionID,
					History:             append([]interfaces.FactoryEvent(nil), f.domain...),
					Events:              f.Live,
				},
			},
		},
	}
}

// ReplaceStreamGeneration keeps the logical and Factory Session identities but
// installs a new stream generation with its own retained-history boundary.
func (f *FactorySessionSSEFixture) ReplaceStreamGeneration(t *testing.T, root *testutil.MockFactory) []factoryapi.FactoryEvent {
	t.Helper()
	replacement := factorySessionSSEFixtureReplacementRetainedEvents(t)
	root.SessionFactories[f.SessionID] = &testutil.MockFactory{
		FactoryEventStream: &interfaces.FactoryEventStream{
			StreamGenerationID:  factorySessionSSEFixtureNextGenerationID,
			BackendScopeID:      factorySessionSSEFixtureBackendScopeID,
			LogicalSessionKeyID: factorySessionSSEFixtureLogicalSessionKey,
			FactorySessionID:    f.SessionID,
			History:             testutil.FactoryEvents(t, replacement),
			Events:              make(chan interfaces.FactoryEvent, 4),
		},
	}
	return replacement
}

// PublishLive enqueues one live FactoryEvent on the fixture stream channel.
func (f *FactorySessionSSEFixture) PublishLive(event factoryapi.FactoryEvent) {
	converted, err := interfaces.NewFactoryEvent(event)
	if err != nil {
		panic(err)
	}
	f.Live <- converted
}

// LiveDispatchEvent returns the canonical post-retained live fixture event.
func (f *FactorySessionSSEFixture) LiveDispatchEvent(t *testing.T) factoryapi.FactoryEvent {
	t.Helper()
	sessionID := f.SessionID
	return testAPIFactoryEvent(
		t,
		factoryapi.FactoryEventTypeDispatchRequest,
		factorySessionSSEFixtureLiveEventID,
		factoryapi.FactoryEventContext{
			Tick:            2,
			Sequence:        3,
			SessionSequence: factorySessionSSESessionSequencePointer(3),
			EventTime:       factorySessionSSEFixtureEventTime.Add(2 * time.Second),
			SessionId:       &sessionID,
			DispatchId:      stringPointerForAPIServerTest("b08-sse-fixture-dispatch-1"),
		},
		factoryapi.DispatchRequestEventPayload{
			TransitionId: "review",
			Inputs:       []factoryapi.DispatchConsumedWorkRef{},
		},
	)
}

func factorySessionSSEFixtureRetainedEvents(t *testing.T) []factoryapi.FactoryEvent {
	t.Helper()
	sessionID := factorySessionSSEFixtureSessionID
	runStartedFactory := factoryapi.Factory{
		WorkTypes: &[]factoryapi.WorkType{{
			Name: "task",
			States: []factoryapi.WorkState{
				{Name: "init", Type: factoryapi.WorkStateTypeINITIAL},
				{Name: "complete", Type: factoryapi.WorkStateTypeTERMINAL},
			},
		}},
	}
	return []factoryapi.FactoryEvent{
		testAPIFactoryEvent(
			t,
			factoryapi.FactoryEventTypeRunRequest,
			factorySessionSSEFixtureRetainedEventOneID,
			factoryapi.FactoryEventContext{
				Tick:            0,
				Sequence:        0,
				SessionSequence: factorySessionSSESessionSequencePointer(0),
				EventTime:       factorySessionSSEFixtureEventTime,
				SessionId:       &sessionID,
			},
			factoryapi.RunRequestEventPayload{
				RecordedAt: factorySessionSSEFixtureEventTime,
				Factory:    runStartedFactory,
			},
		),
		testAPIFactoryEvent(
			t,
			factoryapi.FactoryEventTypeInitialStructureRequest,
			factorySessionSSEFixtureRetainedEventTwoID,
			factoryapi.FactoryEventContext{
				Tick:            0,
				Sequence:        1,
				SessionSequence: factorySessionSSESessionSequencePointer(1),
				EventTime:       factorySessionSSEFixtureEventTime,
				SessionId:       &sessionID,
			},
			factoryapi.InitialStructureRequestEventPayload{
				Factory: factoryapi.Factory{Name: "b08-sse-fixture-factory"},
			},
		),
		testAPIFactoryEvent(
			t,
			factoryapi.FactoryEventTypeWorkRequest,
			factorySessionSSEFixtureRetainedEventThreeID,
			factoryapi.FactoryEventContext{
				Tick:            1,
				Sequence:        2,
				SessionSequence: factorySessionSSESessionSequencePointer(2),
				EventTime:       factorySessionSSEFixtureEventTime.Add(time.Second),
				SessionId:       &sessionID,
				RequestId:       stringPointerForAPIServerTest("b08-sse-fixture-request-1"),
			},
			factoryapi.WorkRequestEventPayload{
				Type: factoryapi.WorkRequestTypeFactoryRequestBatch,
			},
		),
	}
}

func factorySessionSSEFixtureReplacementRetainedEvents(t *testing.T) []factoryapi.FactoryEvent {
	t.Helper()
	sessionID := factorySessionSSEFixtureSessionID
	return []factoryapi.FactoryEvent{testAPIFactoryEvent(
		t,
		factoryapi.FactoryEventTypeRunRequest,
		factorySessionSSEFixtureNextRetainedEventID,
		factoryapi.FactoryEventContext{
			Tick:            0,
			Sequence:        0,
			SessionSequence: factorySessionSSESessionSequencePointer(0),
			EventTime:       factorySessionSSEFixtureEventTime.Add(time.Minute),
			SessionId:       &sessionID,
		},
		factoryapi.RunRequestEventPayload{
			RecordedAt: factorySessionSSEFixtureEventTime.Add(time.Minute),
			Factory:    factoryapi.Factory{Name: "b08-sse-fixture-replacement-factory"},
		},
	)}
}

func factorySessionSSESessionSequencePointer(sequence int) *int {
	return &sequence
}
