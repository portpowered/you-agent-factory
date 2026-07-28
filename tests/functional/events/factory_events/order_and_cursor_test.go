package factory_events

import (
	"strings"
	"testing"
	"time"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const factoryEventsUnknownCursorEventID = "factory-events-invalid-cursor-unknown-event"

// TestAPIGetFactoryEventsReturnsOrderedDurableHistory proves retained Factory
// Event history is returned in durable ascending order through the public
// Factory Events API and that a second retained-history read preserves the same
// relative order for the same session generation.
func TestAPIGetFactoryEventsReturnsOrderedDurableHistory(t *testing.T) {
	dir := support.ScaffoldSingleStepFactory(t, "ordered-durable-history")
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
	})
	t.Cleanup(func() { server.Stop(t) })

	name := "ordered-durable-history-work"
	support.SubmitDefaultSessionWork(t, server.URL(), factoryapi.SubmitWorkRequest{
		Name:         &name,
		WorkTypeName: "task",
		Payload: map[string]string{
			"title": "prove ordered durable history",
		},
	})
	support.WaitForTerminalStatus(t, server.URL(), 10*time.Second)

	firstRead := server.GetFactoryEvents(t)
	if len(firstRead) < 4 {
		t.Fatalf("retained Factory Event count = %d, want at least 4 events after work completion", len(firstRead))
	}
	assertFactoryEventsAscendingOrder(t, firstRead)

	secondRead := server.GetFactoryEvents(t)
	if len(secondRead) != len(firstRead) {
		t.Fatalf(
			"second retained-history read count = %d, want %d for the same session generation",
			len(secondRead),
			len(firstRead),
		)
	}
	assertFactoryEventsSameRelativeOrder(t, firstRead, secondRead)
}

// TestAPIEventCursorReturnsOnlyNewerEvents proves a valid reconnect cursor
// through the public Factory Events API returns only events recorded after the
// acknowledged point and does not re-deliver the acknowledged event itself.
func TestAPIEventCursorReturnsOnlyNewerEvents(t *testing.T) {
	dir := support.ScaffoldSingleStepFactory(t, "cursor-only-newer-events")
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
	})
	t.Cleanup(func() { server.Stop(t) })

	name := "cursor-only-newer-events-work"
	support.SubmitDefaultSessionWork(t, server.URL(), factoryapi.SubmitWorkRequest{
		Name:         &name,
		WorkTypeName: "task",
		Payload: map[string]string{
			"title": "prove cursor returns only newer events",
		},
	})
	support.WaitForTerminalStatus(t, server.URL(), 10*time.Second)

	fullRead := server.GetFactoryEvents(t)
	if len(fullRead) < 4 {
		t.Fatalf("retained Factory Event count = %d, want at least 4 events after work completion", len(fullRead))
	}

	cursorIndex, cursorEvent := pickSessionScopedCursorEvent(t, fullRead, factorysessions.DefaultSessionID)
	wantAfter := append([]factoryapi.FactoryEvent(nil), fullRead[cursorIndex+1:]...)

	afterEventIDRead := server.GetFactoryEventsAfter(t, support.FactoryEventReadCursor{
		AfterEventID: cursorEvent.Id,
	})
	assertFactoryEventsCursorAfterResult(t, cursorEvent, wantAfter, afterEventIDRead)

	reconnectSequence := support.ReconnectSequenceForFactoryEvent(cursorEvent)
	afterSequenceRead := server.GetFactoryEventsAfter(t, support.FactoryEventReadCursor{
		AfterSequence: &reconnectSequence,
	})
	assertFactoryEventsCursorAfterResult(t, cursorEvent, wantAfter, afterSequenceRead)
}

// TestAPIInvalidEventCursorReturnsTypedError proves invalid reconnect cursors
// through the public Factory Events API return typed invalid-cursor handling
// instead of silently skipping events, and that a valid retained-history read
// still works for the same session when cursors are omitted.
func TestAPIInvalidEventCursorReturnsTypedError(t *testing.T) {
	dir := support.ScaffoldSingleStepFactory(t, "invalid-event-cursor")
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
	})
	t.Cleanup(func() { server.Stop(t) })

	name := "invalid-event-cursor-work"
	support.SubmitDefaultSessionWork(t, server.URL(), factoryapi.SubmitWorkRequest{
		Name:         &name,
		WorkTypeName: "task",
		Payload: map[string]string{
			"title": "prove invalid cursor returns typed error",
		},
	})
	support.WaitForTerminalStatus(t, server.URL(), 10*time.Second)

	retained := server.GetFactoryEvents(t)
	if len(retained) < 4 {
		t.Fatalf("retained Factory Event count = %d, want at least 4 events after work completion", len(retained))
	}

	unknownEventIDCursor := support.FactoryEventReadCursor{
		AfterEventID: factoryEventsUnknownCursorEventID,
	}
	unknownEventIDError := support.GetFactoryEventsInvalidCursorErrorAt(t, server.URL(), unknownEventIDCursor)
	assertFactoryEventsInvalidCursorError(t, unknownEventIDError.Response)
	assertFactoryEventsInvalidCursorBodyDoesNotReplayHistory(t, unknownEventIDError.Body, retained)

	unknownSequence := 999999
	unknownSequenceCursor := support.FactoryEventReadCursor{
		AfterSequence: &unknownSequence,
	}
	unknownSequenceError := support.GetFactoryEventsInvalidCursorErrorAt(t, server.URL(), unknownSequenceCursor)
	assertFactoryEventsInvalidCursorError(t, unknownSequenceError.Response)
	assertFactoryEventsInvalidCursorBodyDoesNotReplayHistory(t, unknownSequenceError.Body, retained)

	recovery := support.ProbeFactoryEventStreamRecoveryAt(t, server.URL(), unknownEventIDCursor)
	if recovery.Outcome != factoryapi.FactorySessionEventStreamRecoveryOutcomeCURSORSTALE {
		t.Fatalf("recovery outcome = %q, want CURSOR_STALE", recovery.Outcome)
	}
	if recovery.FactorySessionId != factorysessions.DefaultSessionID {
		t.Fatalf(
			"recovery factorySessionId = %q, want %q",
			recovery.FactorySessionId,
			factorysessions.DefaultSessionID,
		)
	}
	if !recovery.Retry.OmitAfterEventId || !recovery.Retry.OmitAfterSequence {
		t.Fatalf("recovery retry = %#v, want both omit flags true for stale cursor", recovery.Retry)
	}

	validRead := server.GetFactoryEvents(t)
	if len(validRead) != len(retained) {
		t.Fatalf(
			"valid retained-history read count = %d, want %d when cursors are omitted",
			len(validRead),
			len(retained),
		)
	}
	assertFactoryEventsSameRelativeOrder(t, retained, validRead)
}

func assertFactoryEventsInvalidCursorError(t *testing.T, errResp factoryapi.ErrorResponse) {
	t.Helper()

	if errResp.Code != factoryapi.ErrorResponseCodeBADREQUEST {
		t.Fatalf("invalid cursor code = %q, want BAD_REQUEST", errResp.Code)
	}
	if !strings.Contains(errResp.Message, "invalid event reconnect cursor") {
		t.Fatalf("invalid cursor message = %q, want invalid event reconnect cursor guidance", errResp.Message)
	}
}

func assertFactoryEventsInvalidCursorBodyDoesNotReplayHistory(
	t *testing.T,
	bodyText string,
	retained []factoryapi.FactoryEvent,
) {
	t.Helper()

	for _, event := range retained {
		if strings.Contains(bodyText, event.Id) {
			t.Fatalf("invalid cursor response replayed retained event id %q", event.Id)
		}
	}
	if strings.Contains(bodyText, "data: ") {
		t.Fatal("invalid cursor response contained SSE data frames")
	}
}

func pickSessionScopedCursorEvent(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	sessionID string,
) (int, factoryapi.FactoryEvent) {
	t.Helper()

	for index := range events {
		if index >= len(events)-2 {
			break
		}
		event := events[index]
		if !factoryEventBelongsToSession(event, sessionID) {
			continue
		}
		return index, event
	}
	t.Fatalf(
		"retained Factory Event history missing a reusable session-scoped cursor before the final events for session %q",
		sessionID,
	)
	return 0, factoryapi.FactoryEvent{}
}

func factoryEventBelongsToSession(event factoryapi.FactoryEvent, sessionID string) bool {
	return event.Context.SessionId != nil && *event.Context.SessionId == sessionID
}

func assertFactoryEventsCursorAfterResult(
	t *testing.T,
	acknowledged factoryapi.FactoryEvent,
	wantAfter []factoryapi.FactoryEvent,
	got []factoryapi.FactoryEvent,
) {
	t.Helper()

	if len(got) != len(wantAfter) {
		t.Fatalf(
			"cursor-after event count = %d, want %d after acknowledging %q",
			len(got),
			len(wantAfter),
			acknowledged.Id,
		)
	}
	for _, event := range got {
		if event.Id == acknowledged.Id {
			t.Fatalf("cursor-after result re-delivered acknowledged event %q", acknowledged.Id)
		}
	}
	for index := range wantAfter {
		if got[index].Id != wantAfter[index].Id {
			t.Fatalf(
				"cursor-after event at index %d = %q, want %q",
				index,
				got[index].Id,
				wantAfter[index].Id,
			)
		}
		if got[index].Context.Sequence != wantAfter[index].Context.Sequence {
			t.Fatalf(
				"cursor-after sequence at index %d for event %q = %d, want %d",
				index,
				got[index].Id,
				got[index].Context.Sequence,
				wantAfter[index].Context.Sequence,
			)
		}
	}
}

func assertFactoryEventsAscendingOrder(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()

	previousTick := -1
	previousSequence := -1
	for index, event := range events {
		if event.Context.Tick < previousTick {
			t.Fatalf(
				"Factory Event %d (%s) tick %d precedes previous tick %d",
				index,
				event.Id,
				event.Context.Tick,
				previousTick,
			)
		}
		if event.Context.Sequence < previousSequence {
			t.Fatalf(
				"Factory Event %d (%s) sequence %d precedes previous sequence %d",
				index,
				event.Id,
				event.Context.Sequence,
				previousSequence,
			)
		}
		previousTick = event.Context.Tick
		previousSequence = event.Context.Sequence
	}
}

func assertFactoryEventsSameRelativeOrder(
	t *testing.T,
	first []factoryapi.FactoryEvent,
	second []factoryapi.FactoryEvent,
) {
	t.Helper()

	if len(first) != len(second) {
		t.Fatalf("event count mismatch: first=%d second=%d", len(first), len(second))
	}
	for index := range first {
		if first[index].Id != second[index].Id {
			t.Fatalf(
				"retained-history reorder at index %d: first=%q second=%q",
				index,
				first[index].Id,
				second[index].Id,
			)
		}
		if first[index].Context.Sequence != second[index].Context.Sequence {
			t.Fatalf(
				"sequence changed at index %d for event %q between retained-history reads",
				index,
				first[index].Id,
			)
		}
	}
}
