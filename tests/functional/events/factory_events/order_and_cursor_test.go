package factory_events

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
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
	t.Parallel()

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
	t.Parallel()

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
	t.Parallel()

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

// TestAPISubmitWorkEmitsCanonicalTraceAwareBatchEvent proves explicit trace and
// chaining trace identities on submit are preserved in the emitted WORK_REQUEST
// batch event and the public Work projection.
func TestAPISubmitWorkEmitsCanonicalTraceAwareBatchEvent(t *testing.T) {
	t.Parallel()

	dir := support.ScaffoldSingleStepFactory(t, "trace-aware-submit")
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:     dir,
		UseMockWorkers: true,
	})
	defer server.Stop(t)

	const traceID = "trace-request"
	name := "trace-aware-submit"
	submitted := support.SubmitDefaultSessionWork(t, server.URL(), factoryapi.SubmitWorkRequest{
		Name:                   &name,
		WorkTypeName:           "task",
		CurrentChainingTraceId: stringPtr(traceID),
		TraceId:                stringPtr(traceID),
		Payload:                map[string]string{"title": "explicit current"},
	})

	event := waitForWorkRequestEvent(t, server, submitted.RequestId, 5*time.Second)
	if got := support.StringPointerValue(event.Context.RequestId); got != submitted.RequestId {
		t.Fatalf("WORK_REQUEST context request ID = %q, want %q", got, submitted.RequestId)
	}

	payload, err := event.Payload.AsWorkRequestEventPayload()
	if err != nil {
		t.Fatalf("decode WORK_REQUEST payload: %v", err)
	}
	if payload.Type != factoryapi.WorkRequestTypeFactoryRequestBatch {
		t.Fatalf("WORK_REQUEST payload type = %q, want FACTORY_REQUEST_BATCH", payload.Type)
	}

	works := support.FactoryWorksValue(payload.Works)
	if len(works) != 1 {
		t.Fatalf("WORK_REQUEST payload work count = %d, want 1", len(works))
	}
	if works[0].Name != name {
		t.Fatalf("work name = %q, want %s", works[0].Name, name)
	}
	if got := support.StringPointerValue(works[0].CurrentChainingTraceId); got != traceID {
		t.Fatalf("work current chaining trace ID = %q, want %s", got, traceID)
	}
	if got := support.StringPointerValue(works[0].TraceId); got != traceID {
		t.Fatalf("work trace ID = %q, want %s", got, traceID)
	}

	support.WaitForTerminalStatus(t, server.URL(), 10*time.Second)
	listed := support.ListDefaultSessionWork(t, server.URL())
	if len(listed.Results) != 1 ||
		support.StringPointerValue(listed.Results[0].CurrentChainingTraceId) != traceID {
		t.Fatalf("public work projection = %#v, want chaining trace identity", listed.Results)
	}
}

func stringPtr(value string) *string {
	return &value
}

func waitForWorkRequestEvent(
	t *testing.T,
	server *support.FunctionalAPIServer,
	requestID string,
	timeout time.Duration,
) factoryapi.FactoryEvent {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		events := server.GetFactoryEvents(t)
		for _, event := range events {
			if event.Type == factoryapi.FactoryEventTypeWorkRequest &&
				support.StringPointerValue(event.Context.RequestId) == requestID {
				return event
			}
		}
		time.Sleep(50 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for WORK_REQUEST event for %q", requestID)
	return factoryapi.FactoryEvent{}
}

// TestFactoryEventStreamIsOrderedAndClosesAtSessionTermination proves a live
// Factory Event SSE stream delivers events in ascending order while the Factory
// Session is active and closes when the session terminates through the public
// session boundary.
func TestFactoryEventStreamIsOrderedAndClosesAtSessionTermination(t *testing.T) {
	t.Parallel()

	dir := support.ScaffoldSingleStepFactory(t, "stream-order-close")
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
	})
	t.Cleanup(func() { server.Stop(t) })

	opened := support.OpenFactorySessionAt(t, server.URL(), dir)
	sessionID := opened.Session.Id

	stream := support.OpenFactoryEventStreamAt(t, support.SessionEventsURL(server.URL(), sessionID))

	name := "stream-order-close-work"
	support.SubmitSessionWorkAt(t, server.URL(), sessionID, factoryapi.SubmitWorkRequest{
		Name:         &name,
		WorkTypeName: "task",
		Payload: map[string]string{
			"title": "prove ordered stream closes at session termination",
		},
	})

	collected := collectFactoryEventStreamUntilQuiet(t, stream, 15*time.Second)
	if len(collected) < 4 {
		t.Fatalf("live Factory Event count = %d, want at least 4 events before session close", len(collected))
	}
	assertFactoryEventsAscendingOrder(t, collected)

	support.CloseFactorySessionAt(t, server.URL(), sessionID)
	stream.WaitClosed(5 * time.Second)
}

// TestFactoryEventStreamReconnectHasNoGapOrDuplicate proves a dropped Factory
// Event stream can reconnect from an acknowledged cursor and resume the live
// timeline without gaps or duplicate deliveries.
func TestFactoryEventStreamReconnectHasNoGapOrDuplicate(t *testing.T) {
	t.Parallel()

	dir := support.ScaffoldSingleStepFactory(t, "stream-reconnect-continuity")
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
	})
	t.Cleanup(func() { server.Stop(t) })

	firstStream := support.OpenFactoryEventStreamAt(
		t,
		support.SessionEventsURL(server.URL(), factorysessions.DefaultSessionID),
	)
	firstPrefix := collectFactoryEventStreamUntilCount(t, firstStream, 4, 10*time.Second)
	cursorIndex, cursorEvent := pickMidStreamCursorEvent(t, firstPrefix)
	acknowledgedPrefix := append([]factoryapi.FactoryEvent(nil), firstPrefix[:cursorIndex+1]...)
	firstStream.Close()

	name := "stream-reconnect-continuity-work"
	support.SubmitDefaultSessionWork(t, server.URL(), factoryapi.SubmitWorkRequest{
		Name:         &name,
		WorkTypeName: "task",
		Payload: map[string]string{
			"title": "prove reconnect has no gap or duplicate",
		},
	})
	support.WaitForTerminalStatus(t, server.URL(), 15*time.Second)

	reconnectStream := support.OpenFactoryEventStreamAt(
		t,
		support.SessionEventsURLWithCursor(
			server.URL(),
			factorysessions.DefaultSessionID,
			support.FactoryEventReadCursor{AfterEventID: cursorEvent.Id},
		),
	)
	reconnectEvents := collectFactoryEventStreamUntilQuiet(t, reconnectStream, 10*time.Second)
	reconnectStream.Close()

	fullRetained := server.GetFactoryEvents(t)
	assertFactoryEventStreamReconnectContinuity(
		t,
		acknowledgedPrefix,
		reconnectEvents,
		fullRetained,
		cursorEvent,
	)
}

func collectFactoryEventStreamUntilQuiet(
	t *testing.T,
	stream *support.FactoryEventStream,
	timeout time.Duration,
) []factoryapi.FactoryEvent {
	t.Helper()

	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	var collected []factoryapi.FactoryEvent
	var quiet *time.Timer
	var quietC <-chan time.Time
	for {
		select {
		case <-deadline.C:
			return collected
		case <-quietC:
			return collected
		default:
			event, ok := stream.TryNextEvent(50 * time.Millisecond)
			if !ok {
				continue
			}
			collected = append(collected, event)
			if quiet == nil {
				quiet = time.NewTimer(250 * time.Millisecond)
			} else {
				if !quiet.Stop() {
					select {
					case <-quiet.C:
					default:
					}
				}
				quiet.Reset(250 * time.Millisecond)
			}
			quietC = quiet.C
		}
	}
}

func collectFactoryEventStreamUntilCount(
	t *testing.T,
	stream *support.FactoryEventStream,
	wantCount int,
	timeout time.Duration,
) []factoryapi.FactoryEvent {
	t.Helper()

	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	var collected []factoryapi.FactoryEvent
	for len(collected) < wantCount {
		select {
		case <-deadline.C:
			t.Fatalf(
				"timed out collecting %d Factory Events from stream; got %d within %s",
				wantCount,
				len(collected),
				timeout,
			)
		default:
			event, ok := stream.TryNextEvent(50 * time.Millisecond)
			if !ok {
				continue
			}
			collected = append(collected, event)
		}
	}
	return collected
}

func assertFactoryEventStreamReconnectContinuity(
	t *testing.T,
	acknowledgedPrefix []factoryapi.FactoryEvent,
	reconnectEvents []factoryapi.FactoryEvent,
	fullRetained []factoryapi.FactoryEvent,
	acknowledged factoryapi.FactoryEvent,
) {
	t.Helper()

	for _, event := range reconnectEvents {
		if event.Id == acknowledged.Id {
			t.Fatalf("reconnect re-delivered acknowledged event %q", acknowledged.Id)
		}
	}
	for _, event := range acknowledgedPrefix {
		for _, reconnectEvent := range reconnectEvents {
			if reconnectEvent.Id == event.Id {
				t.Fatalf("reconnect duplicated event %q already present in acknowledged prefix", event.Id)
			}
		}
	}

	combined := append(append([]factoryapi.FactoryEvent(nil), acknowledgedPrefix...), reconnectEvents...)
	if len(combined) != len(fullRetained) {
		t.Fatalf(
			"combined reconnect timeline count = %d, want %d retained events",
			len(combined),
			len(fullRetained),
		)
	}
	for index := range fullRetained {
		if combined[index].Id != fullRetained[index].Id {
			t.Fatalf(
				"combined reconnect timeline at index %d = %q, want retained event %q",
				index,
				combined[index].Id,
				fullRetained[index].Id,
			)
		}
		if combined[index].Context.Sequence != fullRetained[index].Context.Sequence {
			t.Fatalf(
				"combined reconnect sequence at index %d for event %q = %d, want %d",
				index,
				combined[index].Id,
				combined[index].Context.Sequence,
				fullRetained[index].Context.Sequence,
			)
		}
	}

	assertFactoryEventsAscendingOrder(t, combined)

	acknowledgedSequence := acknowledged.Context.Sequence
	for _, event := range reconnectEvents {
		if event.Context.Sequence <= acknowledgedSequence {
			t.Fatalf(
				"reconnect event %q sequence %d is not after acknowledged sequence %d",
				event.Id,
				event.Context.Sequence,
				acknowledgedSequence,
			)
		}
	}
	for index := 1; index < len(reconnectEvents); index++ {
		previous := reconnectEvents[index-1]
		current := reconnectEvents[index]
		if current.Context.Sequence <= previous.Context.Sequence {
			t.Fatalf(
				"reconnect gap or reorder between %q (sequence %d) and %q (sequence %d)",
				previous.Id,
				previous.Context.Sequence,
				current.Id,
				current.Context.Sequence,
			)
		}
	}
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

func pickMidStreamCursorEvent(
	t *testing.T,
	events []factoryapi.FactoryEvent,
) (int, factoryapi.FactoryEvent) {
	t.Helper()

	for index := range events {
		if index >= len(events)-2 {
			break
		}
		return index, events[index]
	}
	t.Fatalf(
		"stream prefix missing a reusable mid-stream cursor before the final events; count = %d",
		len(events),
	)
	return 0, factoryapi.FactoryEvent{}
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

// TestCanonicalTopologySnapshotsPreservePublicIdentityAndResourceEvidence proves
// durable Factory Event topology snapshots keep stable public entity IDs and
// resource evidence across InitialStructureRequest and FactoryChange events
// even when customer-facing names change.
func TestCanonicalTopologySnapshotsPreservePublicIdentityAndResourceEvidence(t *testing.T) {
	t.Parallel()

	dir := scaffoldCanonicalTopologyFactory(
		t,
		"gpu",
		"writer",
		"task",
		"queued",
		"review",
	)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
	})
	t.Cleanup(func() { server.Stop(t) })

	initialStructure := requireInitialStructureFactoryEvent(t, server.GetFactoryEvents(t))
	initialPayload, err := initialStructure.Payload.AsInitialStructureRequestEventPayload()
	if err != nil {
		t.Fatalf("decode initial-structure payload: %v", err)
	}
	assertCanonicalTopologyFactoryEvidence(t, initialPayload.Factory)

	initialEvents := server.GetFactoryEvents(t)
	current := getCurrentFactoryAt(t, server.URL())
	saveCurrentFactoryAt(
		t,
		server.URL(),
		current.Version,
		"accelerator",
		"author",
		"job",
		"waiting",
		"approval",
	)

	factoryChange := requireFactoryChangeAfterEvents(t, initialEvents, server.GetFactoryEvents(t))
	changePayload, err := factoryChange.Payload.AsFactoryChangeEventPayload()
	if err != nil {
		t.Fatalf("decode factory-change payload: %v", err)
	}
	assertCanonicalTopologyFactoryEvidence(t, changePayload.Factory)
}

func scaffoldCanonicalTopologyFactory(
	t *testing.T,
	resourceName,
	workerName,
	workTypeName,
	initialStateName,
	workstationName string,
) string {
	t.Helper()

	dir := support.ScaffoldFactory(t, map[string]any{
		"name": "canonical-topology",
		"id":   "canonical-topology-runtime",
		"resources": []any{
			map[string]any{"id": "gpu-stable", "name": resourceName, "capacity": 2},
		},
		"workers": []any{
			map[string]any{
				"id":               "writer-stable",
				"name":             workerName,
				"type":             "MODEL_WORKER",
				"modelProvider":    "CLAUDE",
				"executorProvider": "SCRIPT_WRAP",
				"model":            "claude-sonnet-4-20250514",
				"resources": []any{
					map[string]any{"name": resourceName, "capacity": 1},
				},
			},
		},
		"workTypes": []any{
			map[string]any{
				"id":   "task-stable",
				"name": workTypeName,
				"states": []any{
					map[string]any{"id": "queued-stable", "name": initialStateName, "type": "INITIAL"},
					map[string]any{"id": "done-stable", "name": "done", "type": "TERMINAL"},
					map[string]any{"id": "failed-stable", "name": "failed", "type": "FAILED"},
				},
			},
		},
		"workstations": []map[string]any{{
			"id":       "review-stable",
			"name":     workstationName,
			"behavior": "STANDARD",
			"type":     "MODEL_WORKSTATION",
			"worker":   workerName,
			"body":     "Review work.",
			"inputs": []any{
				map[string]any{"workType": workTypeName, "state": initialStateName},
			},
			"outputs": []any{
				map[string]any{"workType": workTypeName, "state": "done"},
			},
			"onFailure": []any{
				map[string]any{"workType": workTypeName, "state": "failed"},
			},
			"resources": []any{
				map[string]any{"name": resourceName, "capacity": 1},
			},
		}},
	})
	return dir
}

func getCurrentFactoryAt(t *testing.T, serverURL string) factoryapi.Factory {
	t.Helper()

	resp, err := http.Get(serverURL + "/factory-sessions/~default/factory")
	if err != nil {
		t.Fatalf("GET /factory-sessions/~default/factory: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /factory-sessions/~default/factory status = %d, want 200", resp.StatusCode)
	}
	var current factoryapi.Factory
	if err := json.NewDecoder(resp.Body).Decode(&current); err != nil {
		t.Fatalf("decode current factory response: %v", err)
	}
	return current
}

func saveCurrentFactoryAt(
	t *testing.T,
	serverURL string,
	version *factoryapi.HybridLogicalTimestamp,
	resourceName,
	workerName,
	workTypeName,
	initialStateName,
	workstationName string,
) {
	t.Helper()

	if version == nil {
		t.Fatal("factory version = nil, want version metadata for save")
	}
	nextVersion := factoryapi.HybridLogicalTimestamp{
		Logical:  version.Logical + 1,
		Physical: version.Physical.UTC().Add(time.Nanosecond),
	}
	body := canonicalTopologyFactorySaveBody(
		nextVersion,
		resourceName,
		workerName,
		workTypeName,
		initialStateName,
		workstationName,
	)

	req, err := http.NewRequest(
		http.MethodPut,
		serverURL+"/factory-sessions/~default/factory",
		bytes.NewReader([]byte(body)),
	)
	if err != nil {
		t.Fatalf("new current factory save request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT /factory-sessions/~default/factory: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(resp.Body)
		t.Fatalf(
			"PUT /factory-sessions/~default/factory status = %d, want 200: %s",
			resp.StatusCode,
			responseBody,
		)
	}
}

func canonicalTopologyFactorySaveBody(
	version factoryapi.HybridLogicalTimestamp,
	resourceName,
	workerName,
	workTypeName,
	initialStateName,
	workstationName string,
) string {
	document := map[string]any{
		"name": "UNDEFINED",
		"id":   "canonical-topology-runtime",
		"version": map[string]string{
			"logical":  strconv.FormatInt(version.Logical.Int64(), 10),
			"physical": version.Physical.UTC().Format(time.RFC3339Nano),
		},
		"resources": []map[string]any{
			{"id": "gpu-stable", "name": resourceName, "capacity": 2},
		},
		"workers": []map[string]any{
			{
				"id":               "writer-stable",
				"name":             workerName,
				"type":             "MODEL_WORKER",
				"modelProvider":    "CLAUDE",
				"executorProvider": "SCRIPT_WRAP",
				"model":            "claude-sonnet-4-20250514",
				"resources": []map[string]any{
					{"name": resourceName, "capacity": 1},
				},
			},
		},
		"workTypes": []map[string]any{
			{
				"id":   "task-stable",
				"name": workTypeName,
				"states": []map[string]string{
					{"id": "queued-stable", "name": initialStateName, "type": "INITIAL"},
					{"id": "done-stable", "name": "done", "type": "TERMINAL"},
					{"id": "failed-stable", "name": "failed", "type": "FAILED"},
				},
			},
		},
		"workstations": []map[string]any{
			{
				"id":       "review-stable",
				"name":     workstationName,
				"behavior": "STANDARD",
				"type":     "MODEL_WORKSTATION",
				"body":     "Review work.",
				"worker":   workerName,
				"inputs": []map[string]string{
					{"workType": workTypeName, "state": initialStateName},
				},
				"outputs": []map[string]string{
					{"workType": workTypeName, "state": "done"},
				},
				"onFailure": []map[string]string{
					{"workType": workTypeName, "state": "failed"},
				},
				"resources": []map[string]any{
					{"name": resourceName, "capacity": 1},
				},
			},
		},
	}
	body, err := json.Marshal(document)
	if err != nil {
		panic(fmt.Sprintf("marshal canonical topology save document: %v", err))
	}
	return fmt.Sprintf(`{"factory":%s}`, body)
}

func requireInitialStructureFactoryEvent(
	t *testing.T,
	events []factoryapi.FactoryEvent,
) factoryapi.FactoryEvent {
	t.Helper()

	for _, event := range events {
		if event.Type == factoryapi.FactoryEventTypeInitialStructureRequest {
			return event
		}
	}
	t.Fatal("retained Factory Event history missing InitialStructureRequest")
	return factoryapi.FactoryEvent{}
}

func requireFactoryChangeAfterEvents(
	t *testing.T,
	before []factoryapi.FactoryEvent,
	after []factoryapi.FactoryEvent,
) factoryapi.FactoryEvent {
	t.Helper()

	minSequence := -1
	for _, event := range before {
		if event.Context.Sequence > minSequence {
			minSequence = event.Context.Sequence
		}
	}
	for _, event := range after {
		if event.Context.Sequence > minSequence && event.Type == factoryapi.FactoryEventTypeFactoryChange {
			return event
		}
	}
	t.Fatal("retained Factory Event history missing FactoryChange after save")
	return factoryapi.FactoryEvent{}
}

func assertCanonicalTopologyFactoryEvidence(t *testing.T, factory factoryapi.Factory) {
	t.Helper()

	if factory.Resources == nil || len(*factory.Resources) != 1 {
		t.Fatalf("Factory snapshot resources = %#v, want one durable resource", factory.Resources)
	}
	resource := (*factory.Resources)[0]
	if resource.Id == nil || *resource.Id != "gpu-stable" {
		t.Fatalf("Factory snapshot resource id = %#v, want gpu-stable", resource.Id)
	}

	if factory.Workers == nil || len(*factory.Workers) != 1 {
		t.Fatalf("Factory snapshot workers = %#v, want one durable worker", factory.Workers)
	}
	worker := (*factory.Workers)[0]
	if worker.Id == nil || *worker.Id != "writer-stable" {
		t.Fatalf("Factory snapshot worker id = %#v, want writer-stable", worker.Id)
	}
	if worker.Resources == nil || len(*worker.Resources) != 1 {
		t.Fatalf("Factory snapshot worker resources = %#v, want one resource requirement", worker.Resources)
	}

	if factory.WorkTypes == nil || len(*factory.WorkTypes) != 1 {
		t.Fatalf("Factory snapshot work types = %#v, want one durable work type", factory.WorkTypes)
	}
	workType := (*factory.WorkTypes)[0]
	if workType.Id == nil || *workType.Id != "task-stable" {
		t.Fatalf("Factory snapshot work type id = %#v, want task-stable", workType.Id)
	}
	if len(workType.States) != 3 {
		t.Fatalf("Factory snapshot work type states = %#v, want three durable states", workType.States)
	}
	if workType.States[0].Id == nil || *workType.States[0].Id != "queued-stable" {
		t.Fatalf("Factory snapshot initial state id = %#v, want queued-stable", workType.States[0].Id)
	}

	if factory.Workstations == nil || len(*factory.Workstations) != 1 {
		t.Fatalf("Factory snapshot workstations = %#v, want one durable workstation", factory.Workstations)
	}
	workstation := (*factory.Workstations)[0]
	if workstation.Id == nil || *workstation.Id != "review-stable" {
		t.Fatalf("Factory snapshot workstation id = %#v, want review-stable", workstation.Id)
	}
	if workstation.Resources == nil || len(*workstation.Resources) != 1 {
		t.Fatalf("Factory snapshot workstation resources = %#v, want one resource requirement", workstation.Resources)
	}
}
