// pkgmaintcheck:ignore-file-lines consolidated same-package factory-session and provider-session transport tests remain together until HTTP transport package-count pressure is relieved.
// backendsizecheck:ignore-file consolidated same-package factory-session and provider-session transport tests remain together until HTTP transport package-count pressure is relieved.
package http

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/work"

	"github.com/portpowered/infinite-you/internal/testutil"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factorysessions "github.com/portpowered/infinite-you/pkg/factory/sessions"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/responseevents"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/responseeventstore"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	factorytoken "github.com/portpowered/infinite-you/pkg/factory/token"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
	invocations "github.com/portpowered/infinite-you/pkg/work/invocation"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestFactoryResponseEventsBySessionID_RetainedThenLiveUsesExactSessionAndFlushesEachMessage(t *testing.T) {
	defaultStore := responseeventstore.NewSessionResponseEventStore("session-default")
	betaStore := responseeventstore.NewSessionResponseEventStore("session-beta")
	publishResponseProgress(t, defaultStore, "default-retained")
	wantRetained := publishResponseProgress(t, betaStore, "beta-retained")

	srv := newTestServer(&testutil.MockFactory{SessionFactories: map[string]*testutil.MockFactory{
		"~default":     {ResponseEventStore: defaultStore},
		"session-beta": {ResponseEventStore: betaStore},
	}})
	httpServer := httptest.NewServer(srv.Handler())
	defer httpServer.Close()

	ctx, cancel := context.WithCancel(context.Background())
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, httpServer.URL+"/factory-sessions/session-beta/response-events", nil)
	if err != nil {
		t.Fatalf("new response-event request: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("open response-event stream before live publication: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("response-event status = %d, want 200", response.StatusCode)
	}
	if got := response.Header.Get("Content-Type"); !strings.Contains(got, "text/event-stream") {
		t.Fatalf("Content-Type = %q, want text/event-stream", got)
	}

	reader := bufio.NewReader(response.Body)
	retained := readSSEFactoryResponseEvent(t, reader)
	if retained.EventID != wantRetained.EventID || retained.FactorySessionID != "session-beta" {
		t.Fatalf("retained response event = %#v, want beta event %q", retained, wantRetained.EventID)
	}
	wantLive := publishResponseProgress(t, betaStore, "beta-live")
	live := readSSEFactoryResponseEvent(t, reader)
	if live.EventID != wantLive.EventID || live.Sequence <= retained.Sequence {
		t.Fatalf("live response event = %#v, want event %q after sequence %d", live, wantLive.EventID, retained.Sequence)
	}
	if defaultStore.SubscriberCount() != 0 {
		t.Fatalf("default response-event subscribers = %d, want 0", defaultStore.SubscriberCount())
	}

	cancel()
	deadline := time.Now().Add(time.Second)
	for betaStore.SubscriberCount() != 0 && time.Now().Before(deadline) {
		runtime.Gosched()
	}
	if betaStore.SubscriberCount() != 0 {
		t.Fatalf("beta response-event subscribers after disconnect = %d, want 0", betaStore.SubscriberCount())
	}
}

func TestFactoryResponseEventsBySessionID_DisconnectDoesNotStopPublicationOrCatchUp(t *testing.T) {
	store := responseeventstore.NewSessionResponseEventStore("session-beta")
	retained := publishResponseProgress(t, store, "before-disconnect")
	srv := newTestServer(&testutil.MockFactory{SessionFactories: map[string]*testutil.MockFactory{
		"session-beta": {ResponseEventStore: store},
	}})
	httpServer := httptest.NewServer(srv.Handler())
	defer httpServer.Close()

	ctx, cancel := context.WithCancel(context.Background())
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, httpServer.URL+"/factory-sessions/session-beta/response-events", nil)
	if err != nil {
		t.Fatalf("new response-event request: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("open response-event stream: %v", err)
	}
	defer response.Body.Close()
	got := readSSEFactoryResponseEvent(t, bufio.NewReader(response.Body))
	if got.EventID != retained.EventID {
		t.Fatalf("retained event = %q, want %q", got.EventID, retained.EventID)
	}

	cancel()
	waitForNoResponseEventSubscribers(t, store)
	afterDisconnect := publishResponseProgress(t, store, "after-disconnect")
	catchUp, err := store.Subscribe(retained.Sequence)
	if err != nil {
		t.Fatalf("subscribe after observer disconnect: %v", err)
	}
	defer catchUp.Detach()
	events, err := catchUp.Next(context.Background())
	if err != nil {
		t.Fatalf("read event published after observer disconnect: %v", err)
	}
	if len(events) != 1 || events[0].EventID != afterDisconnect.EventID {
		t.Fatalf("events after observer disconnect = %#v, want continued event %q", events, afterDisconnect.EventID)
	}
}

func waitForNoResponseEventSubscribers(t *testing.T, store *responseeventstore.SessionResponseEventStore) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for store.SubscriberCount() != 0 && time.Now().Before(deadline) {
		runtime.Gosched()
	}
	if store.SubscriberCount() != 0 {
		t.Fatalf("response-event subscribers after disconnect = %d, want 0", store.SubscriberCount())
	}
}

func TestFactoryResponseEventsBySessionID_StaleCursorGetsGapBeforeRetainedAndLiveEvents(t *testing.T) {
	store, err := responseeventstore.NewSessionResponseEventStoreWithLimits(
		"session-beta",
		responseeventstore.RetentionLimits{MaxEvents: 2, MaxBytes: 1 << 20},
	)
	if err != nil {
		t.Fatalf("new response-event store: %v", err)
	}
	publishResponseProgress(t, store, "dropped-1")
	publishResponseProgress(t, store, "dropped-2")
	retainedFirst := publishResponseProgress(t, store, "retained-3")
	retainedSecond := publishResponseProgress(t, store, "retained-4")

	srv := newTestServer(&testutil.MockFactory{SessionFactories: map[string]*testutil.MockFactory{
		"session-beta": {ResponseEventStore: store},
	}})
	httpServer := httptest.NewServer(srv.Handler())
	defer httpServer.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		httpServer.URL+"/factory-sessions/session-beta/response-events?after_sequence=1&kind=PROGRESS",
		nil,
	)
	if err != nil {
		t.Fatalf("new stale response-event request: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("open stale response-event stream: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("stale response-event status = %d, want 200", response.StatusCode)
	}

	reader := bufio.NewReader(response.Body)
	gap := readSSEFactoryResponseEvent(t, reader)
	gapPayload := decodeSSEStreamGap(t, gap)
	if gapPayload.FromSequence != 2 || gapPayload.ToSequence != 2 || gapPayload.FirstAvailableSequence != 3 {
		t.Fatalf("STREAM_GAP payload = %#v, want unavailable 2..2 and first available 3", gapPayload)
	}
	retained := []responseevents.FactoryResponseEvent{
		readSSEFactoryResponseEvent(t, reader),
		readSSEFactoryResponseEvent(t, reader),
	}
	if retained[0].EventID != retainedFirst.EventID || retained[1].EventID != retainedSecond.EventID {
		t.Fatalf("retained events = [%q %q], want [%q %q]", retained[0].EventID, retained[1].EventID, retainedFirst.EventID, retainedSecond.EventID)
	}
	liveWant := publishResponseProgress(t, store, "live-5")
	live := readSSEFactoryResponseEvent(t, reader)
	if live.EventID != liveWant.EventID || live.Sequence != 5 {
		t.Fatalf("live event = %#v, want sequence 5 event %q", live, liveWant.EventID)
	}
}

func TestFactoryResponseEventsBySessionID_CursorInsideRetainedSuffixDoesNotGetGap(t *testing.T) {
	store, err := responseeventstore.NewSessionResponseEventStoreWithLimits(
		"session-beta",
		responseeventstore.RetentionLimits{MaxEvents: 2, MaxBytes: 1 << 20},
	)
	if err != nil {
		t.Fatalf("new response-event store: %v", err)
	}
	publishResponseProgress(t, store, "dropped-1")
	publishResponseProgress(t, store, "dropped-2")
	retainedFirst := publishResponseProgress(t, store, "retained-3")
	retainedSecond := publishResponseProgress(t, store, "retained-4")
	store.Complete()
	srv := newTestServer(&testutil.MockFactory{SessionFactories: map[string]*testutil.MockFactory{
		"session-beta": {ResponseEventStore: store},
	}})

	request := httptest.NewRequest(http.MethodGet, "/factory-sessions/session-beta/response-events?after_sequence=3", nil)
	recorder := httptest.NewRecorder()
	srv.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("current response-event status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	got := readSSEFactoryResponseEvent(t, bufio.NewReader(recorder.Body))
	if got.Kind == responseevents.KindStreamGap || got.EventID != retainedSecond.EventID {
		t.Fatalf("current cursor event = %#v, want retained event %q without gap after %q", got, retainedSecond.EventID, retainedFirst.EventID)
	}
}

func TestFactoryResponseEventsBySessionID_KnownCursorEmitsOnlyNewerEvents(t *testing.T) {
	store := responseeventstore.NewSessionResponseEventStore("session-beta")
	first := publishResponseProgress(t, store, "first")
	second := publishResponseProgress(t, store, "second")
	third := publishResponseProgress(t, store, "third")
	store.Complete()
	srv := newTestServer(&testutil.MockFactory{SessionFactories: map[string]*testutil.MockFactory{
		"session-beta": {ResponseEventStore: store},
	}})

	request := httptest.NewRequest(http.MethodGet, "/factory-sessions/session-beta/response-events?after_sequence=1", nil)
	recorder := httptest.NewRecorder()
	srv.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("response-event reconnect status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	reader := bufio.NewReader(recorder.Body)
	gotSecond := readSSEFactoryResponseEvent(t, reader)
	gotThird := readSSEFactoryResponseEvent(t, reader)
	if gotSecond.EventID != second.EventID || gotThird.EventID != third.EventID {
		t.Fatalf("reconnect events = [%q %q], want [%q %q] after %q", gotSecond.EventID, gotThird.EventID, second.EventID, third.EventID, first.EventID)
	}
	if remaining, err := io.ReadAll(reader); err != nil || len(remaining) != 0 {
		t.Fatalf("remaining SSE bytes = %q, err = %v; want clean end", remaining, err)
	}
}

func TestFactoryResponseEventsBySessionID_UnknownSessionNeverFallsBackToDefault(t *testing.T) {
	defaultStore := responseeventstore.NewSessionResponseEventStore("session-default")
	publishResponseProgress(t, defaultStore, "default-retained")
	srv := newTestServer(&testutil.MockFactory{SessionFactories: map[string]*testutil.MockFactory{
		"~default": {ResponseEventStore: defaultStore},
	}})

	request := httptest.NewRequest(http.MethodGet, "/factory-sessions/session-missing/response-events", nil)
	recorder := httptest.NewRecorder()
	srv.Handler().ServeHTTP(recorder, request)
	assertJSONError(t, recorder, http.StatusNotFound, "RESPONSE_EVENT_SESSION_NOT_FOUND", "factory response-event session not found")
	if defaultStore.SubscriberCount() != 0 {
		t.Fatalf("default response-event subscribers = %d, want 0 after unknown-session request", defaultStore.SubscriberCount())
	}
}

type responseEventTestClock struct {
	now time.Time
}

func (c *responseEventTestClock) Now() time.Time {
	return c.now
}

func TestFactoryResponseEventsBySessionID_CompletedWithinRetentionDrainsAndCloses(t *testing.T) {
	start := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	clock := &responseEventTestClock{now: start}
	store := responseeventstore.NewSessionResponseEventStoreWithClock("session-beta", clock)
	wantFirst := publishResponseProgress(t, store, "completed-first")
	wantSecond := publishResponseProgress(t, store, "completed-second")
	store.Complete()
	clock.now = start.Add(responseeventstore.CompletedStreamRetentionWindow - time.Second)

	srv := newTestServer(&testutil.MockFactory{SessionFactories: map[string]*testutil.MockFactory{
		"session-beta": {ResponseEventStore: store},
	}})
	request := httptest.NewRequest(http.MethodGet, "/factory-sessions/session-beta/response-events", nil)
	recorder := httptest.NewRecorder()
	srv.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("completed response-event status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	reader := bufio.NewReader(recorder.Body)
	first := readSSEFactoryResponseEvent(t, reader)
	second := readSSEFactoryResponseEvent(t, reader)
	if first.EventID != wantFirst.EventID || second.EventID != wantSecond.EventID {
		t.Fatalf("completed drain events = [%q %q], want [%q %q]", first.EventID, second.EventID, wantFirst.EventID, wantSecond.EventID)
	}
	if remaining, err := io.ReadAll(reader); err != nil || len(remaining) != 0 {
		t.Fatalf("completed stream remainder = %q, err = %v; want clean end", remaining, err)
	}
}

func TestFactoryResponseEventsBySessionID_ExpiredCompletedStreamReturnsTypedGone(t *testing.T) {
	start := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	clock := &responseEventTestClock{now: start}
	store := responseeventstore.NewSessionResponseEventStoreWithClock("session-beta", clock)
	publishResponseProgress(t, store, "completed")
	store.Complete()
	clock.now = start.Add(responseeventstore.CompletedStreamRetentionWindow)

	srv := newTestServer(&testutil.MockFactory{SessionFactories: map[string]*testutil.MockFactory{
		"session-beta": {ResponseEventStore: store},
	}})
	request := httptest.NewRequest(http.MethodGet, "/factory-sessions/session-beta/response-events", nil)
	recorder := httptest.NewRecorder()
	srv.Handler().ServeHTTP(recorder, request)

	assertJSONError(t, recorder, http.StatusGone, "RESPONSE_EVENT_STREAM_EXPIRED", "factory response-event stream expired")
	if got := recorder.Header().Get("Content-Type"); strings.Contains(got, "text/event-stream") {
		t.Fatalf("expired stream Content-Type = %q, want typed JSON before SSE headers", got)
	}
}

func TestFactoryResponseEventsBySessionID_SubscriptionFailureReturnsTypedInternalError(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{SessionFactories: map[string]*testutil.MockFactory{
		"session-beta": {ResponseEventSubscribeErr: errors.New("subscription storage failed")},
	}})
	request := httptest.NewRequest(http.MethodGet, "/factory-sessions/session-beta/response-events", nil)
	recorder := httptest.NewRecorder()
	srv.Handler().ServeHTTP(recorder, request)

	assertJSONError(t, recorder, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to subscribe to factory response events")
}

func publishResponseProgress(t *testing.T, store *responseeventstore.SessionResponseEventStore, label string) responseevents.FactoryResponseEvent {
	t.Helper()
	payload, err := json.Marshal(responseevents.ProgressPayload{Label: label})
	if err != nil {
		t.Fatalf("marshal response progress payload: %v", err)
	}
	event, err := store.Publish(responseevents.FactoryResponseEvent{
		FactorySessionID: store.FactorySessionID(),
		Kind:             responseevents.KindProgress,
		Phase:            responseevents.PhaseUpdated,
		Provenance: responseevents.Provenance{
			Provider:        "test-provider",
			NativeEventType: "progress",
			Delivery:        responseevents.DeliveryNativeStream,
			Representation:  responseevents.RepresentationNotification,
			Fidelity:        responseevents.FidelityLossless,
		},
		RunID:   "run-test",
		Payload: payload,
	})
	if err != nil {
		t.Fatalf("publish response progress: %v", err)
	}
	return event
}

func readSSEFactoryResponseEvent(t *testing.T, reader *bufio.Reader) responseevents.FactoryResponseEvent {
	t.Helper()
	var idLine, dataLine string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read response-event SSE line: %v", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		switch {
		case strings.HasPrefix(line, "id: "):
			if idLine != "" {
				t.Fatalf("SSE message has multiple id lines")
			}
			idLine = strings.TrimPrefix(line, "id: ")
		case strings.HasPrefix(line, "data: "):
			if dataLine != "" {
				t.Fatalf("SSE message has multiple data lines")
			}
			dataLine = strings.TrimPrefix(line, "data: ")
		default:
			t.Fatalf("unexpected response-event SSE line %q", line)
		}
	}
	if idLine == "" || dataLine == "" {
		t.Fatalf("SSE message id=%q data=%q, want exactly one of each", idLine, dataLine)
	}
	var event responseevents.FactoryResponseEvent
	if err := json.Unmarshal([]byte(dataLine), &event); err != nil {
		t.Fatalf("decode response-event SSE data: %v", err)
	}
	if idLine != fmt.Sprint(event.Sequence) {
		t.Fatalf("SSE id = %q, want event sequence %d", idLine, event.Sequence)
	}
	if err := responseevents.ValidateEvent(event); err != nil {
		t.Fatalf("SSE response event is schema-invalid: %v", err)
	}
	return event
}

func decodeSSEStreamGap(t *testing.T, event responseevents.FactoryResponseEvent) responseevents.StreamGapPayload {
	t.Helper()
	if event.Kind != responseevents.KindStreamGap {
		t.Fatalf("first response event kind = %q, want STREAM_GAP", event.Kind)
	}
	var payload responseevents.StreamGapPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatalf("decode STREAM_GAP payload: %v", err)
	}
	return payload
}

func TestSessionScopedAPI_ReadsAndMutationsTargetOnlyRequestedSession(t *testing.T) {
	now := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	defaultFactoryID := "root-runtime"
	betaFactoryID := "beta-runtime"
	defaultSession := newSessionScopedMockFactory(t, now, &defaultFactoryID, apisurface.DefaultCurrentFactoryName, "tok-default-1", "default-work-1", "factory-event/work-request/default-history")
	betaSession := newSessionScopedMockFactory(t, now, &betaFactoryID, "beta", "tok-beta-1", "beta-work-1", "factory-event/work-request/beta-history")
	srv := newTestServer(&testutil.MockFactory{
		CurrentFactory: &factoryapi.Factory{Name: apisurface.DefaultCurrentFactoryName, Id: &defaultFactoryID},
		SessionFactories: map[string]*testutil.MockFactory{
			"~default":     defaultSession,
			"session-beta": betaSession,
		},
	})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	assertScopedSessionSubmit(t, server.URL, betaSession, defaultSession)
	assertScopedSessionList(t, server.URL, betaSession, defaultSession)
	assertScopedSessionWorkRead(t, server.URL)
	assertScopedSessionStatus(t, server.URL)
	assertScopedCurrentFactory(t, server.URL, "beta")
	assertScopedSessionEvents(t, server.URL, "factory-event/work-request/beta-history")
}

func newSessionScopedMockFactory(
	t *testing.T,
	now time.Time,
	factoryID *string,
	factoryName string,
	tokenID string,
	workID string,
	historyEventID string,
) *testutil.MockFactory {
	return &testutil.MockFactory{
		Marking: &petri.MarkingSnapshot{
			Tokens: map[string]*factorytoken.Token{
				tokenID: listWorkToken(tokenID, workID, "task:init", "task", now),
			},
		},
		Net: sessionScopedStateNet(),
		FactoryEventStream: &interfaces.FactoryEventStream{
			StreamGenerationID: "stream-gen-" + factoryName,
			History:            []interfaces.FactoryEvent{testutil.FactoryEvent(t, factoryapi.FactoryEvent{Id: historyEventID, Type: factoryapi.FactoryEventTypeWorkRequest})},
			Events:             make(chan interfaces.FactoryEvent),
		},
		CurrentFactory: &factoryapi.Factory{Name: factoryName, Id: factoryID},
	}
}

func sessionScopedStateNet() *state.Net {
	return &state.Net{
		Places: map[string]*petri.Place{
			"task:init": {ID: "task:init", TypeID: "task", State: "init"},
			"task:done": {ID: "task:done", TypeID: "task", State: "done"},
		},
		WorkTypes: map[string]*state.WorkType{
			"task": {
				ID: "task",
				States: []state.StateDefinition{
					{Value: "init", Category: state.StateCategoryInitial},
					{Value: "done", Category: state.StateCategoryTerminal},
				},
			},
		},
	}
}

func assertScopedSessionSubmit(t *testing.T, serverURL string, betaSession *testutil.MockFactory, defaultSession *testutil.MockFactory) {
	t.Helper()

	response := requireHTTPSuccess(t, http.MethodPost, serverURL+"/factory-sessions/session-beta/work", bytes.NewBufferString(`{"name":"scoped-submit","workTypeName":"task","traceId":"trace-scoped-submit","payload":{"title":"scoped"}}`), "application/json", http.StatusCreated)
	defer response.Body.Close()

	var submitBody factoryapi.SubmitWorkResponse
	if err := json.NewDecoder(response.Body).Decode(&submitBody); err != nil {
		t.Fatalf("decode scoped submit response: %v", err)
	}
	assertSubmitWorkResponseIdentifiers(t, submitBody, submitWorkResponseExpectation{
		traceID:      "trace-scoped-submit",
		name:         "scoped-submit",
		workTypeName: "task",
		sessionID:    "session-beta",
		workIDSuffix: "-scoped-submit",
	})

	if len(betaSession.WorkRequests) != 1 {
		t.Fatalf("beta submitted work requests = %d, want 1", len(betaSession.WorkRequests))
	}
	if len(defaultSession.WorkRequests) != 0 {
		t.Fatalf("default submitted work requests = %d, want 0", len(defaultSession.WorkRequests))
	}
}

func assertScopedSessionList(t *testing.T, serverURL string, betaSession *testutil.MockFactory, defaultSession *testutil.MockFactory) {
	t.Helper()

	response := requireHTTPSuccess(t, http.MethodGet, serverURL+"/factory-sessions/session-beta/work", nil, "", http.StatusOK)
	defer response.Body.Close()

	var listBody factoryapi.ListWorkResponse
	if err := json.NewDecoder(response.Body).Decode(&listBody); err != nil {
		t.Fatalf("decode scoped list response: %v", err)
	}
	if len(listBody.Results) != 1 || stringValue(listBody.Results[0].WorkId) != "beta-work-1" {
		t.Fatalf("scoped list results = %#v, want beta-work-1", listBody.Results)
	}
	if betaSession.EngineStateSnapshotCalls == 0 {
		t.Fatal("expected scoped GET /work to read the targeted session snapshot")
	}
	if defaultSession.EngineStateSnapshotCalls != 0 {
		t.Fatalf("default session snapshot calls = %d, want 0 after scoped list", defaultSession.EngineStateSnapshotCalls)
	}
}

func assertScopedSessionWorkRead(t *testing.T, serverURL string) {
	t.Helper()

	response := requireHTTPSuccess(t, http.MethodGet, serverURL+"/factory-sessions/session-beta/work/tok-beta-1", nil, "", http.StatusOK)
	defer response.Body.Close()
}

func assertScopedSessionStatus(t *testing.T, serverURL string) {
	t.Helper()

	response := requireHTTPSuccess(t, http.MethodGet, serverURL+"/factory-sessions/session-beta/status", nil, "", http.StatusOK)
	defer response.Body.Close()
}

func assertScopedCurrentFactory(t *testing.T, serverURL string, wantName string) {
	t.Helper()

	response := requireHTTPSuccess(t, http.MethodGet, serverURL+"/factory-sessions/session-beta/factory", nil, "", http.StatusOK)
	defer response.Body.Close()

	var currentBody factoryapi.Factory
	if err := json.NewDecoder(response.Body).Decode(&currentBody); err != nil {
		t.Fatalf("decode scoped current factory response: %v", err)
	}
	if currentBody.Name != wantName {
		t.Fatalf("scoped current factory name = %q, want %s", currentBody.Name, wantName)
	}
}

func assertScopedSessionEvents(t *testing.T, serverURL string, wantEventID string) {
	t.Helper()

	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, serverURL+"/factory-sessions/session-beta/events", nil)
	if err != nil {
		t.Fatalf("new scoped /events request: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("GET /factory-sessions/session-beta/events: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("GET /factory-sessions/session-beta/events status = %d, want 200: %s", response.StatusCode, string(body))
	}
	if got := response.Header.Get(sessionEventStreamGenerationHeader); got != "stream-gen-beta" {
		t.Fatalf("%s = %q, want stream-gen-beta", sessionEventStreamGenerationHeader, got)
	}

	streamed := readSSEFactoryEvent(t, bufio.NewReader(response.Body))
	if streamed.Id != wantEventID {
		t.Fatalf("scoped streamed event id = %q, want %s", streamed.Id, wantEventID)
	}
}

func requireHTTPSuccess(
	t *testing.T,
	method string,
	url string,
	body io.Reader,
	contentType string,
	wantStatus int,
) *http.Response {
	t.Helper()

	var (
		response *http.Response
		err      error
	)
	switch method {
	case http.MethodGet:
		response, err = http.Get(url)
	case http.MethodPost:
		response, err = http.Post(url, contentType, body)
	default:
		request, requestErr := http.NewRequestWithContext(context.Background(), method, url, body)
		if requestErr != nil {
			t.Fatalf("%s %s request: %v", method, url, requestErr)
		}
		if contentType != "" {
			request.Header.Set("Content-Type", contentType)
		}
		response, err = http.DefaultClient.Do(request)
	}
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	if response.StatusCode != wantStatus {
		bodyBytes, _ := io.ReadAll(response.Body)
		response.Body.Close()
		t.Fatalf("%s %s status = %d, want %d: %s", method, url, response.StatusCode, wantStatus, string(bodyBytes))
	}
	return response
}

func TestSessionScopedAPI_UnknownSessionReturnsNotFound(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{
		SessionFactories: map[string]*testutil.MockFactory{
			"~default": {Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*factorytoken.Token)}},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/factory-sessions/missing-session/status", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	assertJSONError(t, rec, http.StatusNotFound, "NOT_FOUND", "factory session not found")
}

func TestFactorySessionsAPI_ListFactorySessions(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{
		FactorySessions: factoryapi.ListFactorySessionsResponse{
			Sessions: []factoryapi.FactorySessionSummary{
				{
					FactoryDir: "/workspace/root",
					FolderPath: "/workspace/root",
					Id:         "~default",
					IsDefault:  true,
					Project:    "root",
					Target: factoryapi.FactorySessionTargetRef{
						Kind: factoryapi.FactorySessionTargetRefKindDefault,
					},
				},
				{
					FactoryDir: "/workspace/root/beta",
					FolderPath: "/workspace/root",
					Id:         "session-beta",
					IsDefault:  false,
					Project:    "beta",
					Target: factoryapi.FactorySessionTargetRef{
						Kind: factoryapi.FactorySessionTargetRefKindNamed,
						Name: stringPointerForAPITest("beta"),
					},
				},
			},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/factory-sessions", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /factory-sessions status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var response factoryapi.ListFactorySessionsResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode list factory sessions response: %v", err)
	}
	if len(response.Sessions) != 2 {
		t.Fatalf("factory sessions = %#v, want default and beta sessions", response.Sessions)
	}
	ids := map[string]bool{}
	for _, session := range response.Sessions {
		ids[session.Id] = true
	}
	if !ids["~default"] || !ids["session-beta"] {
		t.Fatalf("factory session ids = %#v, want ~default and session-beta", ids)
	}
}

func TestFactorySessionsAPI_OpenFactorySession(t *testing.T) {
	mf := &testutil.MockFactory{
		OpenFactorySessionResult: factoryapi.OpenFactorySessionResponse{
			Session: &factoryapi.FactorySessionSummary{
				FactoryDir: "/workspace/fleet/beta",
				FolderPath: "/workspace/fleet",
				Id:         "session-beta",
				IsDefault:  false,
				Project:    "beta",
				Target: factoryapi.FactorySessionTargetRef{
					Kind: factoryapi.FactorySessionTargetRefKindNamed,
					Name: stringPointerForAPITest("beta"),
				},
			},
		},
	}
	srv := newTestServer(mf)

	req := httptest.NewRequest(http.MethodPost, "/factory-sessions", bytes.NewBufferString(`{"folderPath":"/workspace/fleet","target":{"kind":"named","name":"beta"}}`))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /factory-sessions status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if len(mf.OpenedFactorySessions) != 1 {
		t.Fatalf("opened factory sessions = %d, want 1", len(mf.OpenedFactorySessions))
	}
	if mf.OpenedFactorySessions[0].FolderPath != "/workspace/fleet" {
		t.Fatalf("opened session folder = %q, want /workspace/fleet", mf.OpenedFactorySessions[0].FolderPath)
	}
	if mf.OpenedFactorySessions[0].Target == nil ||
		mf.OpenedFactorySessions[0].Target.Kind != factoryapi.FactorySessionTargetRefKindNamed ||
		mf.OpenedFactorySessions[0].Target.Name == nil ||
		*mf.OpenedFactorySessions[0].Target.Name != "beta" {
		t.Fatalf("opened session target = %#v, want named beta", mf.OpenedFactorySessions[0].Target)
	}
	var response factoryapi.OpenFactorySessionResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode open factory session response: %v", err)
	}
	if response.Session == nil || response.Session.Id != "session-beta" {
		t.Fatalf("open factory session response = %#v, want session-beta", response)
	}
}

func TestFactorySessionsAPI_OpenFactorySession_ValidationTargets(t *testing.T) {
	mf := &testutil.MockFactory{
		OpenFactorySessionErr: factorysessions.NewValidationError(
			factorysessions.ValidationReasonMissing,
			"folderPath",
			errors.New("folder validation failed"),
		),
	}
	srv := newTestServer(mf)

	req := httptest.NewRequest(http.MethodPost, "/factory-sessions", bytes.NewBufferString(`{"folderPath":"/workspace/missing","validateOnly":true}`))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("POST /factory-sessions validation status = %d, want 400: %s", rec.Code, rec.Body.String())
	}

	var response factoryapi.ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode open factory session error response: %v", err)
	}
	if response.Code != factoryapi.ErrorResponseCodeBADREQUEST {
		t.Fatalf("open factory session error code = %q, want BAD_REQUEST", response.Code)
	}
	if response.Targets == nil || len(*response.Targets) != 1 {
		t.Fatalf("open factory session error targets = %#v, want one target", response.Targets)
	}
	target := (*response.Targets)[0]
	if target.Code != "factory.session.field.missing" ||
		target.Subject.Type != factoryapi.FactoryValidationSubjectTypeFactory ||
		target.Subject.Id != "folderPath" ||
		target.Subject.Location != factoryapi.FactoryValidationSubjectLocationReference {
		t.Fatalf("open factory session error target = %#v, want structured folder validation target", target)
	}
}

func TestFactorySessionsAPI_OpenFactorySession_ConfigLoadFailureTargets(t *testing.T) {
	mf := &testutil.MockFactory{
		OpenFactorySessionErr: apiTestSessionValidationError{
			message: "factory configuration could not be loaded from the selected folder",
			code:    "FACTORY_SESSION_CONFIG_LOAD_FAILED",
			targets: []factoryapi.FactoryValidationTarget{
				apisurface.FactoryValidationTargetToAPI(factoryvalidation.FactorySessionTargetTarget(
					"config_load_failed",
					"default",
					`Factory target "default" at "/workspace/fleet" could not be loaded: unexpected end of JSON input`,
				)),
			},
		},
	}
	srv := newTestServer(mf)

	req := httptest.NewRequest(http.MethodPost, "/factory-sessions", bytes.NewBufferString(`{"folderPath":"/workspace/fleet","validateOnly":true}`))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("POST /factory-sessions config-load failure status = %d, want 400: %s", rec.Code, rec.Body.String())
	}

	var response factoryapi.ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode open factory session config-load error response: %v", err)
	}
	if response.Code != "FACTORY_SESSION_CONFIG_LOAD_FAILED" {
		t.Fatalf("open factory session config-load error code = %q, want FACTORY_SESSION_CONFIG_LOAD_FAILED", response.Code)
	}
	if response.Message != "factory configuration could not be loaded from the selected folder" {
		t.Fatalf("open factory session config-load error message = %q, want safe summary", response.Message)
	}
	if response.Targets == nil || len(*response.Targets) != 1 {
		t.Fatalf("open factory session config-load error targets = %#v, want one target", response.Targets)
	}
	target := (*response.Targets)[0]
	if target.Code != "factory.session.target.config_load_failed" ||
		target.Subject.Type != factoryapi.FactoryValidationSubjectTypeFactory ||
		target.Subject.Id != "default" ||
		target.Subject.Location != factoryapi.FactoryValidationSubjectLocationReference {
		t.Fatalf("open factory session config-load error target = %#v, want structured config-load target", target)
	}
}

func TestFactorySessionsAPI_CloseFactorySession(t *testing.T) {
	mf := &testutil.MockFactory{}
	srv := newTestServer(mf)

	req := httptest.NewRequest(http.MethodDelete, "/factory-sessions/session-beta", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE /factory-sessions/session-beta status = %d, want 204: %s", rec.Code, rec.Body.String())
	}
	if len(mf.ClosedFactorySessions) != 1 || mf.ClosedFactorySessions[0] != "session-beta" {
		t.Fatalf("closed factory sessions = %#v, want [session-beta]", mf.ClosedFactorySessions)
	}
}

type apiTestSessionValidationError struct {
	message string
	code    string
	targets []factoryapi.FactoryValidationTarget
}

func (e apiTestSessionValidationError) Error() string {
	return e.message
}

func (e apiTestSessionValidationError) ErrorTargets() []factoryapi.FactoryValidationTarget {
	return e.targets
}

func (e apiTestSessionValidationError) ErrorCode() string {
	if e.code == "" {
		return "BAD_REQUEST"
	}
	return e.code
}

func TestFactorySessionsAPI_CloseFactorySession_NotFound(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{
		CloseFactorySessionErr: apisurface.ErrFactorySessionNotFound,
	})

	req := httptest.NewRequest(http.MethodDelete, "/factory-sessions/missing-session", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	assertJSONError(t, rec, http.StatusNotFound, "NOT_FOUND", "factory session not found")
}

func TestGetCurrentFactoryBySessionId_ReturnsSessionDefinitionAndVersion(t *testing.T) {
	defaultVersion := factoryapi.HybridLogicalTimestamp{Physical: time.Unix(0, 1).UTC(), Logical: 1}
	sessionVersion := factoryapi.HybridLogicalTimestamp{Physical: time.Unix(0, 2).UTC(), Logical: 2}
	srv := newTestServer(&testutil.MockFactory{
		SessionFactories: map[string]*testutil.MockFactory{
			"~default": {
				CurrentFactory: &factoryapi.Factory{Name: "alpha"},
				FactoryVersion: defaultVersion,
			},
			"session-2": {
				CurrentFactory: &factoryapi.Factory{Name: "beta"},
				FactoryVersion: sessionVersion,
			},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/factory-sessions/session-2/factory", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET factory status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var response factoryapi.Factory
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode factory response: %v", err)
	}
	if response.Name != "beta" || response.Version == nil || *response.Version != sessionVersion {
		t.Fatalf("factory response = %#v, want beta/%#v", response, sessionVersion)
	}
}

func TestSaveCurrentFactoryBySessionId_SubmitsToTargetedSessionOnly(t *testing.T) {
	defaultVersion := factoryapi.HybridLogicalTimestamp{Physical: time.Unix(0, 1).UTC(), Logical: 1}
	sessionVersion := factoryapi.HybridLogicalTimestamp{Physical: time.Unix(0, 2).UTC(), Logical: 2}
	defaultFactory := &testutil.MockFactory{
		CurrentFactory: &factoryapi.Factory{Name: "alpha"},
		FactoryVersion: defaultVersion,
	}
	sessionFactory := &testutil.MockFactory{
		CurrentFactory: &factoryapi.Factory{Name: "beta"},
		FactoryVersion: sessionVersion,
	}
	srv := newTestServer(&testutil.MockFactory{
		SessionFactories: map[string]*testutil.MockFactory{
			"~default":  defaultFactory,
			"session-2": sessionFactory,
		},
	})

	body := saveFactoryForSessionRequestBody(`{"name":"beta","version":{"physical":"1970-01-01T00:00:00.000000002Z","logical":2},"workTypes":[],"workstations":[],"workers":[]}`)
	req := httptest.NewRequest(http.MethodPut, "/factory-sessions/session-2/factory", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("PUT factory status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if len(defaultFactory.SavedCurrentFactories) != 0 {
		t.Fatalf("default session save count = %d, want 0", len(defaultFactory.SavedCurrentFactories))
	}
	if len(sessionFactory.SavedCurrentFactories) != 1 {
		t.Fatalf("session save count = %d, want 1", len(sessionFactory.SavedCurrentFactories))
	}
	saved := sessionFactory.SavedCurrentFactories[0]
	if saved.Name != "beta" {
		t.Fatalf("saved factory = %#v, want beta definition", saved)
	}
}

func TestCurrentFactoryBySessionId_UnknownSessionReturnsNotFound(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{
		SessionFactories: map[string]*testutil.MockFactory{
			"~default": {},
		},
	})

	getReq := httptest.NewRequest(http.MethodGet, "/factory-sessions/missing-session/factory", nil)
	getRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(getRec, getReq)
	assertJSONError(t, getRec, http.StatusNotFound, "NOT_FOUND", "factory session not found")

	putReq := httptest.NewRequest(http.MethodPut, "/factory-sessions/missing-session/factory", bytes.NewBufferString(saveFactoryForSessionRequestBody(`{"name":"beta"}`)))
	putReq.Header.Set("Content-Type", "application/json")
	putRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(putRec, putReq)
	assertJSONError(t, putRec, http.StatusNotFound, "NOT_FOUND", "factory session not found")
}

func TestFactorySessionsAPI_InvokeFactorySession(t *testing.T) {
	const namedGoalParityText = "Plan the sprint from CLI and API parity coverage"

	tests := []struct {
		name           string
		body           string
		result         apisurface.FactoryInvocationResult
		wantSubmitText string
	}{
		{
			name: "default text input",
			body: `{"sourceKind":"text","content":[{"type":"text","text":"invoke this"}]}`,
			result: apisurface.FactoryInvocationResult{
				RequestID:     "invoke-1",
				TraceID:       "trace-invoke-1",
				Status:        "COMPLETED",
				PrimaryResult: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "primary output"}},
			},
		},
		{
			name: "named goal parity text",
			body: `{"sourceKind":"text","content":[{"type":"text","text":"` + namedGoalParityText + `"}]}`,
			result: apisurface.FactoryInvocationResult{
				RequestID: "request-goal-parity-success",
				TraceID:   "trace-goal-parity-success",
				Status:    "COMPLETED",
				PrimaryResult: []work.WorkContentPart{{
					Type: work.WorkContentPartTypeText,
					Text: "goal parity completed",
				}},
			},
			wantSubmitText: namedGoalParityText,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mock := &testutil.MockFactory{
				SessionFactories: map[string]*testutil.MockFactory{
					"~default": {},
				},
				InvokeFactoryResult: tc.result,
			}
			assertFactorySessionInvocation(t, mock, tc.body, tc.result, tc.wantSubmitText)
		})
	}
}

func TestFactorySessionsAPI_InvokeFactorySession_DecodesStructuredArgs(t *testing.T) {
	mock := &testutil.MockFactory{
		SessionFactories: map[string]*testutil.MockFactory{
			"~default": {},
		},
		InvokeFactoryResult: apisurface.FactoryInvocationResult{
			RequestID: "invoke-structured-1",
			TraceID:   "trace-structured-1",
			Status:    "COMPLETED",
			PrimaryResult: []work.WorkContentPart{{
				Type: work.WorkContentPartTypeText,
				Text: "ok",
			}},
		},
	}

	srv := newTestServer(mock)
	req := httptest.NewRequest(http.MethodPost, "/factory-sessions/~default/invocations", bytes.NewBufferString(`{"args":{"input":"hello","tag":["alpha","beta"]}}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /factory-sessions/~default/invocations status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if len(mock.InvokedFactorySessions) != 1 {
		t.Fatalf("invoked factory sessions = %d, want 1", len(mock.InvokedFactorySessions))
	}
	if mock.InvokedFactorySessions[0].Args == nil {
		t.Fatal("invocation args = nil, want decoded args map")
	}
	if got := (*mock.InvokedFactorySessions[0].Args)["input"]; got != "hello" {
		t.Fatalf("args[input] = %#v, want hello", got)
	}
}

func TestFactorySessionsAPI_InvokeFactorySession_InputConflictReturnsStableBadRequest(t *testing.T) {
	const namedGoalParityText = "Plan the sprint from CLI and API parity coverage"
	conflictMessage := "invocation input sources conflict: positional_text, stdin_text"

	srv := newTestServer(&testutil.MockFactory{
		SessionFactories: map[string]*testutil.MockFactory{
			"~default": {},
		},
		InvokeFactoryErr: &invocations.InputError{
			Code:    invocations.InputErrorCodeSourceConflict,
			Message: conflictMessage,
		},
	})

	body := `{"sourceKind":"text","content":[{"type":"text","text":"` + namedGoalParityText + `"}]}`
	req := httptest.NewRequest(http.MethodPost, "/factory-sessions/~default/invocations", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	assertJSONError(t, rec, http.StatusBadRequest, string(invocations.InputErrorCodeSourceConflict), conflictMessage)
}

func TestParseCodexSessionSummary_ExtractsDiagnosticDetails(t *testing.T) {
	session := strings.Join([]string{
		`{"timestamp":"2026-05-18T10:00:00Z","type":"turn_context"}`,
		`{"timestamp":"2026-05-18T10:00:01Z","type":"response_item","payload":{"type":"reasoning","summary":["checked input"],"encrypted_content":"sealed"}}`,
		`{"timestamp":"2026-05-18T10:00:02Z","type":"response_item","payload":{"type":"function_call","call_id":"call-1","name":"exec_command","arguments":{"cmd":"go test ./pkg/api"}}}`,
		`{"timestamp":"2026-05-18T10:00:03Z","type":"response_item","payload":{"type":"function_call_output","call_id":"call-1","output":"ok"}}`,
		`{"timestamp":"2026-05-18T10:00:04Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":100,"cached_input_tokens":40,"output_tokens":25,"reasoning_output_tokens":5,"total_tokens":130}}}}`,
		`{"timestamp":"2026-05-18T10:00:05Z","type":"turn_context"}`,
		`{"timestamp":"2026-05-18T10:00:06Z","type":"response_item","payload":{"type":"custom_tool_call","call_id":"call-2","name":"apply_patch","input":"patch text","status":"in_progress"}}`,
		`{"timestamp":"2026-05-18T10:00:07Z","type":"event_msg","payload":{"type":"new_future_event"}}`,
		`{"timestamp":"2026-05-18T10:00:08Z","type":"unexpected_top_level"}`,
		`{bad json`,
	}, "\n")

	summary, err := parseCodexSessionSummary(strings.NewReader(session))
	if err != nil {
		t.Fatalf("parse codex session summary: %v", err)
	}
	assertCodexSessionSummaryCoreCounts(t, summary)
	assertCodexSessionSummaryFunctionCalls(t, summary)
	assertCodexSessionSummaryReasoning(t, summary)
	parsed, err := parseCodexSessionDetails(strings.NewReader(session))
	if err != nil {
		t.Fatalf("parse codex session details: %v", err)
	}
	assertParsedCodexSessionSummaryTranscript(t, parsed)
	assertCodexSessionSummaryTokenUsage(t, summary)
	assertCodexSessionSummaryUnknowns(t, summary)
}

func TestParseCodexSessionDetails_EmitsMixedTranscriptChronologically(t *testing.T) {
	session := strings.Join([]string{
		`{"timestamp":"2026-05-18T10:00:00Z","type":"turn_context"}`,
		`{"timestamp":"2026-05-18T10:00:01Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Inspect the failing run."}]}}`,
		`{"timestamp":"2026-05-18T10:00:02Z","type":"response_item","payload":{"type":"reasoning","summary":["Checking tool output"],"encrypted_content":"sealed"}}`,
		`{"timestamp":"2026-05-18T10:00:03Z","type":"response_item","payload":{"type":"function_call","call_id":"call-1","name":"exec_command","arguments":{"cmd":"go test ./pkg/api"}}}`,
		`{"timestamp":"2026-05-18T10:00:04Z","type":"response_item","payload":{"type":"function_call_output","call_id":"call-1","output":"ok","status":"completed"}}`,
		`{"timestamp":"2026-05-18T10:00:05Z","type":"event_msg","payload":{"type":"agent_message","message":"The package tests passed."}}`,
		`{"timestamp":"2026-05-18T10:00:06Z","type":"event_msg","payload":{"type":"task_started","message":"Applying follow-up patch"}}`,
		`{"timestamp":"2026-05-18T10:00:07Z","type":"event_msg","payload":{"type":"agent_reasoning","text":"Need one more validation step."}}`,
		`{"timestamp":"2026-05-18T10:00:08Z","type":"event_msg","payload":{"type":"new_future_event"}}`,
		`{"timestamp":"2026-05-18T10:00:09Z","type":"unexpected_top_level"}`,
		`{bad json`,
	}, "\n")

	parsed, err := parseCodexSessionDetails(strings.NewReader(session))
	if err != nil {
		t.Fatalf("parse codex session details: %v", err)
	}

	assertMixedCodexSessionSummary(t, parsed.Summary)
	assertMixedCodexSessionTranscript(t, parsed)
}

func assertCodexSessionSummaryCoreCounts(t *testing.T, summary factoryapi.ProviderSessionParseSummary) {
	t.Helper()

	if summary.LineCount != 10 || summary.EventCount != 9 || summary.MalformedLineCount != 1 || summary.UnknownEventCount != 2 || len(summary.Turns) != 2 || len(summary.FunctionCalls) != 2 {
		t.Fatalf("summary = %#v, want parsed counts and two turns/calls", summary)
	}
}

func assertCodexSessionSummaryFunctionCalls(t *testing.T, summary factoryapi.ProviderSessionParseSummary) {
	t.Helper()

	firstCall := summary.FunctionCalls[0]
	if firstCall.Order != 1 || stringValue(firstCall.Name) != "exec_command" || stringValue(firstCall.Arguments) != `{"cmd":"go test ./pkg/api"}` || stringValue(firstCall.Output) != "ok" || stringValue(firstCall.Status) != "completed" {
		t.Fatalf("first function call = %#v, want completed exec_command call", firstCall)
	}
	secondCall := summary.FunctionCalls[1]
	if secondCall.Order != 2 || stringValue(secondCall.Name) != "apply_patch" || stringValue(secondCall.Status) != "in_progress" || stringValue(secondCall.Output) != "" {
		t.Fatalf("second function call = %#v, want in-progress custom tool call", secondCall)
	}
}

func assertCodexSessionSummaryReasoning(t *testing.T, summary factoryapi.ProviderSessionParseSummary) {
	t.Helper()

	if len(summary.Reasoning) != 1 || stringValue(summary.Reasoning[0].Summary) != `["checked input"]` || summary.Reasoning[0].Encrypted == nil || !*summary.Reasoning[0].Encrypted || stringValue(summary.Reasoning[0].EncryptedContent) != "sealed" {
		t.Fatalf("reasoning = %#v, want summary, encrypted marker, and encrypted content", summary.Reasoning)
	}
}

func assertParsedCodexSessionSummaryTranscript(t *testing.T, parsed parsedCodexSessionDetails) {
	t.Helper()

	if len(parsed.Transcript) != 4 {
		t.Fatalf("transcript = %#v, want four ordered transcript entries", parsed.Transcript)
	}
	if parsed.Transcript[0].Type != factoryapi.Reasoning || stringValue(parsed.Transcript[0].SourceType) != "reasoning" || intValue(parsed.Transcript[0].LineNumber) != 2 {
		t.Fatalf("first transcript entry = %#v, want reasoning line 2", parsed.Transcript[0])
	}
	if parsed.Transcript[1].Type != factoryapi.ToolCall || stringValue(parsed.Transcript[1].Name) != "exec_command" || stringValue(parsed.Transcript[1].Arguments) != `{"cmd":"go test ./pkg/api"}` {
		t.Fatalf("second transcript entry = %#v, want exec_command tool call", parsed.Transcript[1])
	}
	if parsed.Transcript[2].Type != factoryapi.ToolOutput || stringValue(parsed.Transcript[2].Output) != "ok" || stringValue(parsed.Transcript[2].Status) != "completed" {
		t.Fatalf("third transcript entry = %#v, want completed tool output", parsed.Transcript[2])
	}
	if parsed.Transcript[3].Type != factoryapi.ToolCall || stringValue(parsed.Transcript[3].Name) != "apply_patch" || stringValue(parsed.Transcript[3].Status) != "in_progress" {
		t.Fatalf("fourth transcript entry = %#v, want in-progress apply_patch tool call", parsed.Transcript[3])
	}
	if parsed.Transcript[3].Order != 4 {
		t.Fatalf("final transcript entry order = %d, want 4", parsed.Transcript[3].Order)
	}
}

func assertCodexSessionSummaryTokenUsage(t *testing.T, summary factoryapi.ProviderSessionParseSummary) {
	t.Helper()

	if summary.TokenUsage == nil || intValue(summary.TokenUsage.InputTokens) != 100 || intValue(summary.TokenUsage.CachedInputTokens) != 40 || intValue(summary.TokenUsage.OutputTokens) != 25 || intValue(summary.TokenUsage.ReasoningOutputTokens) != 5 || intValue(summary.TokenUsage.TotalTokens) != 130 {
		t.Fatalf("token usage = %#v, want total consumed token fields", summary.TokenUsage)
	}
}

func assertCodexSessionSummaryUnknowns(t *testing.T, summary factoryapi.ProviderSessionParseSummary) {
	t.Helper()

	if len(summary.UnknownEvents) != 2 || summary.UnknownEvents[0].LineNumber != 8 || stringValue(summary.UnknownEvents[0].Type) != "event_msg" || stringValue(summary.UnknownEvents[0].PayloadType) != "new_future_event" || summary.UnknownEvents[1].LineNumber != 9 || stringValue(summary.UnknownEvents[1].Type) != "unexpected_top_level" {
		t.Fatalf("unknown events = %#v, want compact line-level unknown records", summary.UnknownEvents)
	}
	if len(summary.ParseErrors) != 1 || summary.ParseErrors[0].LineNumber != 10 {
		t.Fatalf("parse errors = %#v, want malformed line retained", summary.ParseErrors)
	}
}

func assertMixedCodexSessionSummary(t *testing.T, summary factoryapi.ProviderSessionParseSummary) {
	t.Helper()

	if summary.LineCount != 11 || summary.EventCount != 10 || summary.MalformedLineCount != 1 || summary.UnknownEventCount != 2 {
		t.Fatalf("summary = %#v, want mixed-session diagnostic counts", summary)
	}
	if len(summary.Turns) != 1 || summary.Turns[0].FunctionCallCount != 1 || summary.Turns[0].ReasoningCount != 2 {
		t.Fatalf("turn summary = %#v, want one turn with function and reasoning counts", summary.Turns)
	}
	if len(summary.UnknownEvents) != 2 || summary.UnknownEvents[0].LineNumber != 9 || summary.UnknownEvents[1].LineNumber != 10 {
		t.Fatalf("unknown events = %#v, want unknown event_msg and top-level event retained", summary.UnknownEvents)
	}
	if len(summary.ParseErrors) != 1 || summary.ParseErrors[0].LineNumber != 11 {
		t.Fatalf("parse errors = %#v, want malformed line 11 retained", summary.ParseErrors)
	}
}

func assertMixedCodexSessionTranscriptLength(t *testing.T, parsed parsedCodexSessionDetails) {
	t.Helper()

	if len(parsed.Transcript) != 7 {
		t.Fatalf("transcript = %#v, want seven ordered transcript entries", parsed.Transcript)
	}
}

func assertMixedCodexSessionTranscriptEntry(t *testing.T, parsed parsedCodexSessionDetails, index int, wantType factoryapi.ProviderSessionTranscriptEntryType, wantLine int, wantText string) {
	t.Helper()

	entry := parsed.Transcript[index]
	if entry.Order != index+1 || entry.Type != wantType || intValue(entry.LineNumber) != wantLine || stringValue(entry.Text) != wantText {
		t.Fatalf("transcript[%d] = %#v, want order=%d type=%q line=%d text=%q", index, entry, index+1, wantType, wantLine, wantText)
	}
	if intValue(entry.TurnIndex) != 1 {
		t.Fatalf("transcript[%d] turn index = %#v, want 1", index, entry.TurnIndex)
	}
}

func assertMixedCodexSessionTranscript(t *testing.T, parsed parsedCodexSessionDetails) {
	t.Helper()

	assertMixedCodexSessionTranscriptLength(t, parsed)
	assertMixedCodexSessionTranscriptUserMessage(t, parsed)
	assertMixedCodexSessionTranscriptReasoning(t, parsed)
	assertMixedCodexSessionTranscriptToolEvents(t, parsed)
	assertMixedCodexSessionTranscriptAssistantAndSystemEvents(t, parsed)
}

func assertMixedCodexSessionTranscriptUserMessage(t *testing.T, parsed parsedCodexSessionDetails) {
	t.Helper()

	assertMixedCodexSessionTranscriptEntry(t, parsed, 0, factoryapi.UserMessage, 2, "Inspect the failing run.")
	if parsed.Transcript[0].SourceType == nil || *parsed.Transcript[0].SourceType != "message" {
		t.Fatalf("first transcript source type = %#v, want message", parsed.Transcript[0].SourceType)
	}
}

func assertMixedCodexSessionTranscriptReasoning(t *testing.T, parsed parsedCodexSessionDetails) {
	t.Helper()

	if parsed.Transcript[1].Order != 2 || parsed.Transcript[1].Type != factoryapi.Reasoning || intValue(parsed.Transcript[1].LineNumber) != 3 || stringValue(parsed.Transcript[1].Summary) != `["Checking tool output"]` || parsed.Transcript[1].Encrypted == nil || !*parsed.Transcript[1].Encrypted || stringValue(parsed.Transcript[1].EncryptedContent) != "sealed" {
		t.Fatalf("transcript[1] = %#v, want encrypted reasoning summary and content on line 3", parsed.Transcript[1])
	}

	assertMixedCodexSessionTranscriptEntry(t, parsed, 6, factoryapi.Reasoning, 8, "Need one more validation step.")
	if parsed.Transcript[6].SourceType == nil || *parsed.Transcript[6].SourceType != "agent_reasoning" {
		t.Fatalf("final reasoning transcript source type = %#v, want agent_reasoning", parsed.Transcript[6].SourceType)
	}
}

func assertMixedCodexSessionTranscriptToolEvents(t *testing.T, parsed parsedCodexSessionDetails) {
	t.Helper()

	if parsed.Transcript[2].Order != 3 || parsed.Transcript[2].Type != factoryapi.ToolCall || intValue(parsed.Transcript[2].LineNumber) != 4 || stringValue(parsed.Transcript[2].Name) != "exec_command" {
		t.Fatalf("transcript[2] = %#v, want tool call on line 4", parsed.Transcript[2])
	}
	if parsed.Transcript[3].Order != 4 || parsed.Transcript[3].Type != factoryapi.ToolOutput || intValue(parsed.Transcript[3].LineNumber) != 5 || stringValue(parsed.Transcript[3].Output) != "ok" || stringValue(parsed.Transcript[3].Status) != "completed" {
		t.Fatalf("transcript[3] = %#v, want tool output on line 5", parsed.Transcript[3])
	}
}

func assertMixedCodexSessionTranscriptAssistantAndSystemEvents(t *testing.T, parsed parsedCodexSessionDetails) {
	t.Helper()

	assertMixedCodexSessionTranscriptEntry(t, parsed, 4, factoryapi.AssistantMessage, 6, "The package tests passed.")
	if parsed.Transcript[4].SourceType == nil || *parsed.Transcript[4].SourceType != "agent_message" {
		t.Fatalf("assistant transcript source type = %#v, want agent_message", parsed.Transcript[4].SourceType)
	}

	assertMixedCodexSessionTranscriptEntry(t, parsed, 5, factoryapi.SystemEvent, 7, "Applying follow-up patch")
	if parsed.Transcript[5].SourceType == nil || *parsed.Transcript[5].SourceType != "task_started" {
		t.Fatalf("system-event transcript source type = %#v, want task_started", parsed.Transcript[5].SourceType)
	}
}

func TestParseCodexSessionSummary_AcceptsLargeJSONLRecords(t *testing.T) {
	session := strings.Join([]string{
		`{"timestamp":"2026-05-18T10:00:00Z","type":"turn_context"}`,
		`{"timestamp":"2026-05-18T10:00:01Z","type":"response_item","payload":{"type":"reasoning","content":"` + strings.Repeat("x", 128*1024) + `"}}`,
	}, "\n")

	summary, err := parseCodexSessionSummary(strings.NewReader(session))
	if err != nil {
		t.Fatalf("parse codex session summary: %v", err)
	}
	if summary.LineCount != 2 || summary.EventCount != 2 || len(summary.Reasoning) != 1 {
		t.Fatalf("summary = %#v, want large response item parsed successfully", summary)
	}
}

func TestGetProviderSessionDetails_NotFoundIsDistinguishable(t *testing.T) {
	srv := newTestServerWithCodexRoot(t.TempDir())
	req := httptest.NewRequest("GET", "/provider-sessions/detail?provider=codex&kind=session_id&id=missing-session", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assertJSONError(t, rec, http.StatusNotFound, "NOT_FOUND", "provider session not found")
}

func TestGetProviderSessionDetails_CursorNotFoundIsDistinguishable(t *testing.T) {
	srv := newTestServerWithCursorRoot(t.TempDir())
	req := httptest.NewRequest("GET", "/provider-sessions/detail?provider=cursor&kind=session_id&id=missing-session", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assertJSONError(t, rec, http.StatusNotFound, "NOT_FOUND", "provider session not found")
}

func TestGetProviderSessionDetails_LegacyAgentCursorNotFoundIsDistinguishable(t *testing.T) {
	srv := newTestServerWithCursorRoot(t.TempDir())
	req := httptest.NewRequest("GET", "/provider-sessions/detail?provider=agent&kind=session_id&id=missing-session", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assertJSONError(t, rec, http.StatusNotFound, "NOT_FOUND", "provider session not found")
}

func TestGetProviderSessionDetails_CursorNotFoundLogsDiagnostic(t *testing.T) {
	root := t.TempDir()
	core, logs := observer.New(zap.InfoLevel)
	srv := NewServerWithOptions(&testutil.MockFactory{
		Marking: &petri.MarkingSnapshot{
			Tokens: make(map[string]*factorytoken.Token),
		},
	}, 8080, zap.New(core), ServerOptions{CursorSessionsRoot: root})

	req := httptest.NewRequest("GET", "/provider-sessions/detail?provider=cursor&kind=session_id&id=missing-session", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	assertJSONError(t, rec, http.StatusNotFound, "NOT_FOUND", "provider session not found")

	entries := logs.FilterMessage("cursor provider session lookup not found").AllUntimed()
	if len(entries) != 1 {
		t.Fatalf("cursor not-found diagnostic count = %d, want 1", len(entries))
	}
	fields := entries[0].ContextMap()
	if fields["provider"] != "cursor" {
		t.Fatalf("provider field = %#v, want cursor", fields["provider"])
	}
	if fields["lookup_kind"] != "session_id" {
		t.Fatalf("lookup_kind field = %#v, want session_id", fields["lookup_kind"])
	}
	if fields["requested_id"] != "missing-session" {
		t.Fatalf("requested_id field = %#v, want missing-session", fields["requested_id"])
	}
	if fields["searched_root"] != root {
		t.Fatalf("searched_root field = %#v, want %q", fields["searched_root"], root)
	}
	if fields["root_configured"] != true {
		t.Fatalf("root_configured field = %#v, want true", fields["root_configured"])
	}
}

func TestGetProviderSessionDetails_CursorNotFoundWithUnavailableRoot(t *testing.T) {
	srv := newTestServerWithUnavailableCursorRoot(t)
	req := httptest.NewRequest("GET", "/provider-sessions/detail?provider=cursor&kind=session_id&id=missing-session", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assertJSONError(t, rec, http.StatusNotFound, "NOT_FOUND", "provider session not found")
}

func TestGetProviderSessionDetails_CursorNotFoundWithMissingRootDirectory(t *testing.T) {
	missingRoot := filepath.Join(t.TempDir(), "does-not-exist")
	srv := newTestServerWithCursorRoot(missingRoot)
	req := httptest.NewRequest("GET", "/provider-sessions/detail?provider=cursor&kind=session_id&id=missing-session", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assertJSONError(t, rec, http.StatusNotFound, "NOT_FOUND", "provider session not found")
}

func TestGetProviderSessionDetails_CursorNotFoundLogsDiagnosticWhenRootUnconfigured(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	missingRoot := filepath.Join(t.TempDir(), "cursor-root-unavailable")
	srv := NewServerWithOptions(&testutil.MockFactory{
		Marking: &petri.MarkingSnapshot{
			Tokens: make(map[string]*factorytoken.Token),
		},
	}, 8080, zap.New(core), ServerOptions{CursorSessionsRoot: missingRoot})

	req := httptest.NewRequest("GET", "/provider-sessions/detail?provider=cursor&kind=session_id&id=missing-session", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	assertJSONError(t, rec, http.StatusNotFound, "NOT_FOUND", "provider session not found")

	entries := logs.FilterMessage("cursor provider session lookup not found").AllUntimed()
	if len(entries) != 1 {
		t.Fatalf("cursor not-found diagnostic count = %d, want 1", len(entries))
	}
	fields := entries[0].ContextMap()
	if fields["provider"] != "cursor" {
		t.Fatalf("provider field = %#v, want cursor", fields["provider"])
	}
	if fields["lookup_kind"] != "session_id" {
		t.Fatalf("lookup_kind field = %#v, want session_id", fields["lookup_kind"])
	}
	if fields["requested_id"] != "missing-session" {
		t.Fatalf("requested_id field = %#v, want missing-session", fields["requested_id"])
	}
	if fields["root_configured"] != false {
		t.Fatalf("root_configured field = %#v, want false", fields["root_configured"])
	}
	if _, ok := fields["searched_root"]; ok {
		t.Fatalf("searched_root field = %#v, want omitted when root unconfigured", fields["searched_root"])
	}
}

func TestGetProviderSessionDetails_IgnoresUnsupportedRolloutFileNames(t *testing.T) {
	root := t.TempDir()
	writeNamedProviderSessionFixture(t, root, "rollout-backup-sess_123.jsonl", `{"type":"session_meta","id":"sess_123"}`)
	writeNamedProviderSessionFixture(t, root, "rollout-2026-05-20T17-35-24-backup-sess_123.jsonl", `{"type":"session_meta","id":"sess_123"}`)

	srv := newTestServerWithCodexRoot(root)
	req := httptest.NewRequest("GET", "/provider-sessions/detail?provider=codex&kind=session_id&id=sess_123", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assertJSONError(t, rec, http.StatusNotFound, "NOT_FOUND", "provider session not found")
}

func TestGetProviderSessionDetails_RejectsPathLikeAndMalformedIdentifiers(t *testing.T) {
	for _, target := range []string{
		"/provider-sessions/detail?provider=codex&kind=session_id&id=../secret",
		"/provider-sessions/detail?provider=codex&kind=session_id&id=/tmp/rollout-session.jsonl",
		"/provider-sessions/detail?provider=codex&kind=session_id&id=session.with.dot",
	} {
		t.Run(target, func(t *testing.T) {
			srv := newTestServerWithCodexRoot(t.TempDir())
			req := httptest.NewRequest("GET", target, nil)
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, req)
			assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "provider session must be a codex session_id identifier without path separators")
		})
	}
}

func TestGetProviderSessionDetails_CursorRejectsPathLikeAndMalformedIdentifiers(t *testing.T) {
	for _, target := range []string{
		"/provider-sessions/detail?provider=cursor&kind=session_id&id=../secret",
		"/provider-sessions/detail?provider=cursor&kind=session_id&id=/tmp/store.db",
		"/provider-sessions/detail?provider=cursor&kind=session_id&id=session.with.dot",
	} {
		t.Run(target, func(t *testing.T) {
			srv := newTestServerWithCursorRoot(t.TempDir())
			req := httptest.NewRequest("GET", target, nil)
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, req)
			assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "provider session must be a cursor session_id identifier without path separators")
		})
	}
}

func TestGetProviderSessionDetails_LegacyAgentCursorRejectsPathLikeAndMalformedIdentifiers(t *testing.T) {
	for _, target := range []string{
		"/provider-sessions/detail?provider=agent&kind=session_id&id=../secret",
		"/provider-sessions/detail?provider=agent&kind=session_id&id=/tmp/store.db",
		"/provider-sessions/detail?provider=agent&kind=session_id&id=session.with.dot",
	} {
		t.Run(target, func(t *testing.T) {
			srv := newTestServerWithCursorRoot(t.TempDir())
			req := httptest.NewRequest("GET", target, nil)
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, req)
			assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "provider session must be a cursor session_id identifier without path separators")
		})
	}
}

func TestGetProviderSessionDetails_RejectsUnsupportedProviderOrKindByContract(t *testing.T) {
	for _, target := range []string{
		"/provider-sessions/detail?provider=openai&kind=session_id&id=sess-123",
		"/provider-sessions/detail?provider=codex&kind=path&id=sess-123",
		"/provider-sessions/detail?provider=cursor&kind=path&id=sess-123",
	} {
		t.Run(target, func(t *testing.T) {
			srv := newTestServerWithCodexRoot(t.TempDir())
			req := httptest.NewRequest("GET", target, nil)
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, req)
			assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "invalid request parameter")
		})
	}
}

func TestGetProviderSessionDetails_RejectsSessionSymlinkOutsideConfiguredRoot(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	outsideSessionPath := filepath.Join(outside, "rollout-sess-outside.jsonl")
	if err := os.WriteFile(outsideSessionPath, []byte(`{"type":"session_meta"}`), 0o600); err != nil {
		t.Fatalf("write outside session fixture: %v", err)
	}
	sessionDir := filepath.Join(root, "2026", "05", "18")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("create session dir: %v", err)
	}
	if err := os.Symlink(outsideSessionPath, filepath.Join(sessionDir, "rollout-sess-outside.jsonl")); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlink capability unavailable: %v", err)
		}
		t.Fatalf("create provider session symlink: %v", err)
	}

	srv := newTestServerWithCodexRoot(root)
	req := httptest.NewRequest("GET", "/provider-sessions/detail?provider=codex&kind=session_id&id=sess-outside", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "provider session must be a codex session_id identifier without path separators")
}

func TestGetProviderSessionDetails_RejectsSessionSymlinkOutsideConfiguredRootEvenWhenValidMatchExists(t *testing.T) {
	root := t.TempDir()
	writeProviderSessionFixture(t, root, "sess-shared", `{"type":"session_meta","id":"sess-shared"}`)
	outside := t.TempDir()
	outsideSessionPath := filepath.Join(outside, "rollout-2026-05-20T17-35-24-sess-shared.jsonl")
	if err := os.WriteFile(outsideSessionPath, []byte(`{"type":"session_meta"}`), 0o600); err != nil {
		t.Fatalf("write outside session fixture: %v", err)
	}
	sessionDir := filepath.Join(root, "2026", "05", "18")
	if err := os.Symlink(outsideSessionPath, filepath.Join(sessionDir, "rollout-2026-05-20T17-35-24-sess-shared.jsonl")); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlink capability unavailable: %v", err)
		}
		t.Fatalf("create provider session symlink: %v", err)
	}

	srv := newTestServerWithCodexRoot(root)
	req := httptest.NewRequest("GET", "/provider-sessions/detail?provider=codex&kind=session_id&id=sess-shared", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "provider session must be a codex session_id identifier without path separators")
}

func TestLoadProviderSessionDetails_LoadsExactRolloutFromConfiguredRoot(t *testing.T) {
	root := t.TempDir()
	writeProviderSessionFixture(t, root, "sess_123", strings.Join([]string{
		`{"type":"session_meta","id":"sess_123"}`,
		`{"type":"response_item","item":{"type":"reasoning"}}`,
	}, "\n"))

	resp, err := loadProviderSessionDetails(root, "sess_123")
	if err != nil {
		t.Fatalf("loadProviderSessionDetails: %v", err)
	}
	if string(resp.ProviderSession.Provider) != "codex" || string(resp.ProviderSession.Kind) != "session_id" || resp.ProviderSession.Id != "sess_123" {
		t.Fatalf("provider session = %#v, want codex session_id sess_123", resp.ProviderSession)
	}
	if resp.Source.RelativePath != "2026/05/18/rollout-sess_123.jsonl" || resp.Source.SizeBytes == 0 {
		t.Fatalf("source = %#v, want rooted rollout metadata", resp.Source)
	}
	if resp.Parse.EventCount != 2 {
		t.Fatalf("parse summary = %#v, want two parsed events", resp.Parse)
	}
}

func TestLoadProviderSessionDetails_LoadsTimestampPrefixedRolloutFromConfiguredRoot(t *testing.T) {
	root := t.TempDir()
	sessionID := "019e44f4-580e-7f32-981e-1e54ec6907d6"
	writeNamedProviderSessionFixture(t, root, "rollout-2026-05-20T17-35-24-"+sessionID+".jsonl", strings.Join([]string{
		`{"type":"session_meta","id":"` + sessionID + `"}`,
		`{"type":"response_item","payload":{"type":"reasoning"}}`,
	}, "\n"))

	resp, err := loadProviderSessionDetails(root, sessionID)
	if err != nil {
		t.Fatalf("loadProviderSessionDetails: %v", err)
	}
	wantRelativePath := "2026/05/18/rollout-2026-05-20T17-35-24-" + sessionID + ".jsonl"
	if resp.Source.RelativePath != wantRelativePath || resp.ProviderSession.Id != sessionID {
		t.Fatalf("provider session detail = %#v, want timestamp-prefixed rollout at %s", resp, wantRelativePath)
	}
}

func TestLoadProviderSessionDetails_PrefersExactRolloutWhenBothLayoutsExist(t *testing.T) {
	root := t.TempDir()
	writeProviderSessionFixture(t, root, "sess_123", `{"type":"session_meta","id":"sess_123"}`)
	writeNamedProviderSessionFixture(t, root, "rollout-2026-05-20T17-35-24-sess_123.jsonl", `{"type":"session_meta","id":"sess_123"}`)

	resp, err := loadProviderSessionDetails(root, "sess_123")
	if err != nil {
		t.Fatalf("loadProviderSessionDetails: %v", err)
	}
	if resp.Source.RelativePath != "2026/05/18/rollout-sess_123.jsonl" {
		t.Fatalf("relative path = %q, want exact rollout basename", resp.Source.RelativePath)
	}
}

func TestLoadProviderSessionDetails_NotFoundIsDistinguishable(t *testing.T) {
	_, err := loadProviderSessionDetails(t.TempDir(), "missing-session")
	if !errors.Is(err, errProviderSessionNotFound) {
		t.Fatalf("err = %v, want errProviderSessionNotFound", err)
	}
}

func TestLoadProviderSessionDetails_RejectsPathLikeIdentifiers(t *testing.T) {
	for _, id := range []string{"../secret", "/tmp/rollout-session.jsonl", "session.with.dot"} {
		t.Run(id, func(t *testing.T) {
			_, err := loadProviderSessionDetails(t.TempDir(), id)
			if !errors.Is(err, errInvalidProviderSessionIdentifier) {
				t.Fatalf("err = %v, want errInvalidProviderSessionIdentifier", err)
			}
		})
	}
}

func TestResolveCodexSessionFile_RejectsAmbiguousTimestampPrefixedMatches(t *testing.T) {
	root := t.TempDir()
	writeNamedProviderSessionFixture(t, root, "rollout-2026-05-20T17-35-24-sess_123.jsonl", `{"type":"session_meta","id":"sess_123"}`)
	sessionDir := filepath.Join(root, "2026", "05", "19")
	writeNamedProviderSessionFixtureAt(t, sessionDir, "rollout-2026-05-20T17-45-24-sess_123.jsonl", `{"type":"session_meta","id":"sess_123"}`)

	_, err := resolveCodexSessionFile(root, "sess_123")
	if !errors.Is(err, errAmbiguousProviderSessionFile) {
		t.Fatalf("err = %v, want errAmbiguousProviderSessionFile", err)
	}
}

func TestMatchesCodexSessionBaseName_AcceptsSupportedLayoutsOnly(t *testing.T) {
	exactName := "rollout-sess_123.jsonl"
	tests := []struct {
		baseName string
		want     bool
	}{
		{baseName: exactName, want: true},
		{baseName: "rollout-2026-05-20T17-35-24-sess_123.jsonl", want: true},
		{baseName: "rollout-backup-sess_123.jsonl", want: false},
		{baseName: "rollout-2026-05-20T17-35-24-backup-sess_123.jsonl", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.baseName, func(t *testing.T) {
			if got := matchesCodexSessionBaseName(tc.baseName, "sess_123", exactName); got != tc.want {
				t.Fatalf("matchesCodexSessionBaseName(%q) = %v, want %v", tc.baseName, got, tc.want)
			}
		})
	}
}

func writeNamedProviderSessionFixtureAt(t *testing.T, dir, fileName, contents string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create provider session fixture directory: %v", err)
	}
	path := filepath.Join(dir, fileName)
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write named provider session fixture: %v", err)
	}
}

func TestGetProviderSessionDetails_EventRefRoundTripLoadsCursorAndCodex(t *testing.T) {
	codexRoot := t.TempDir()
	writeProviderSessionFixture(t, codexRoot, "sess_123", strings.Join([]string{
		`{"type":"session_meta","id":"sess_123"}`,
		`{"type":"response_item","item":{"type":"reasoning"}}`,
	}, "\n"))

	cursorRoot, cursorSessionID := writeCursorProviderSessionUUIDFixture(t)
	srv := newTestServerWithProviderSessionRoots(codexRoot, cursorRoot)

	codexEventRef := factoryapi.LoadableProviderSessionRef{
		Provider: factoryapi.Codex,
		Kind:     factoryapi.LoadableProviderSessionKindSessionID,
		Id:       "sess_123",
	}
	assertProviderSessionDetailLoadsFromEventRef(t, srv, codexEventRef, factoryapi.Codex)

	cursorEventRef := factoryapi.LoadableProviderSessionRef{
		Provider: factoryapi.Cursor,
		Kind:     factoryapi.LoadableProviderSessionKindSessionID,
		Id:       cursorSessionID,
	}
	assertProviderSessionDetailLoadsFromEventRef(t, srv, cursorEventRef, factoryapi.Cursor)

	legacyAgentEventRef := factoryapi.LoadableProviderSessionRef{
		Provider: factoryapi.LoadableProviderSessionProvider("agent"),
		Kind:     factoryapi.LoadableProviderSessionKindSessionID,
		Id:       cursorSessionID,
	}
	assertProviderSessionDetailLoadsFromEventRef(t, srv, legacyAgentEventRef, factoryapi.Cursor)

	canonicalizedLegacyRef := loadableProviderSessionRefFromEventMetadata(workerexecution.ProviderSessionMetadata{
		Provider: "agent",
		Kind:     "session_id",
		ID:       cursorSessionID,
	})
	if string(canonicalizedLegacyRef.Provider) != string(factoryapi.Cursor) {
		t.Fatalf("canonicalized legacy ref provider = %q, want cursor", canonicalizedLegacyRef.Provider)
	}
	assertProviderSessionDetailLoadsFromEventRef(t, srv, canonicalizedLegacyRef, factoryapi.Cursor)
}

func TestGetProviderSessionDetails_RegressionLoadsCodexAndCursorFromConfiguredRoots(t *testing.T) {
	codexRoot := t.TempDir()
	writeProviderSessionFixture(t, codexRoot, "sess_123", strings.Join([]string{
		`{"type":"session_meta","id":"sess_123"}`,
		`{"type":"response_item","item":{"type":"reasoning"}}`,
	}, "\n"))

	cursorRoot, cursorSessionID := writeCursorProviderSessionUUIDFixture(t)

	srv := newTestServerWithProviderSessionRoots(codexRoot, cursorRoot)

	codexReq := httptest.NewRequest("GET", "/provider-sessions/detail?provider=codex&kind=session_id&id=sess_123", nil)
	codexRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(codexRec, codexReq)
	if codexRec.Code != http.StatusOK {
		t.Fatalf("codex status = %d, want 200: %s", codexRec.Code, codexRec.Body.String())
	}
	codexResp := decodeJSONResponse[factoryapi.ProviderSessionDetailResponse](t, codexRec)
	assertProviderSessionResponseIdentity(t, codexResp)

	cursorReq := httptest.NewRequest("GET", "/provider-sessions/detail?provider=cursor&kind=session_id&id="+cursorSessionID, nil)
	cursorRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(cursorRec, cursorReq)
	if cursorRec.Code != http.StatusOK {
		t.Fatalf("cursor status = %d, want 200: %s", cursorRec.Code, cursorRec.Body.String())
	}
	cursorResp := decodeJSONResponse[factoryapi.ProviderSessionDetailResponse](t, cursorRec)
	if string(cursorResp.ProviderSession.Provider) != "cursor" || cursorResp.ProviderSession.Id != cursorSessionID {
		t.Fatalf("cursor provider session = %#v, want cursor session_id %s", cursorResp.ProviderSession, cursorSessionID)
	}
}

func TestGetProviderSessionDetails_LoadsCodexSessionFromConfiguredRoot(t *testing.T) {
	root := t.TempDir()
	writeProviderSessionFixture(t, root, "sess_123", strings.Join([]string{
		`{"type":"session_meta","id":"sess_123"}`,
		`{"type":"response_item","item":{"type":"reasoning"}}`,
		`{"unexpected":true}`,
		`not-json`,
		``,
	}, "\n"))

	srv := newTestServerWithCodexRoot(root)
	req := httptest.NewRequest("GET", "/provider-sessions/detail?provider=codex&kind=session_id&id=sess_123", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	resp := decodeJSONResponse[factoryapi.ProviderSessionDetailResponse](t, rec)
	assertProviderSessionResponseIdentity(t, resp)
	assertProviderSessionParseCounts(t, resp.Parse)
	assertProviderSessionTranscriptSummary(t, resp)
	assertProviderSessionParseDiagnostics(t, resp.Parse)
}

func TestGetProviderSessionDetails_LoadsCursorSessionFromConfiguredRoot(t *testing.T) {
	root, sessionID := writeCursorProviderSessionFixture(t)

	srv := newTestServerWithCursorRoot(root)
	req := httptest.NewRequest("GET", "/provider-sessions/detail?provider=cursor&kind=session_id&id="+sessionID, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	resp := decodeJSONResponse[factoryapi.ProviderSessionDetailResponse](t, rec)
	if string(resp.ProviderSession.Provider) != "cursor" || string(resp.ProviderSession.Kind) != "session_id" || resp.ProviderSession.Id != sessionID {
		t.Fatalf("provider session = %#v, want cursor session_id %s", resp.ProviderSession, sessionID)
	}
	if resp.Source.RelativePath != "workspace-hash/"+sessionID+"/store.db" || resp.Source.SizeBytes == 0 {
		t.Fatalf("source = %#v, want rooted cursor store.db metadata", resp.Source)
	}
	if resp.Parse.EventCount != 1 || resp.Parse.LineCount < 1 {
		t.Fatalf("parse summary = %#v, want one readable cursor event", resp.Parse)
	}
	if len(resp.Transcript) != 1 || stringValue(resp.Transcript[0].Text) != "Hello from API fixture" {
		t.Fatalf("transcript = %#v, want one readable cursor transcript entry", resp.Transcript)
	}
	if resp.Parse.TokenUsage == nil || intValue(resp.Parse.TokenUsage.InputTokens) != 100 || intValue(resp.Parse.TokenUsage.TotalTokens) != 175 {
		t.Fatalf("token usage = %#v, want aggregated cursor usage metadata", resp.Parse.TokenUsage)
	}
}

func TestGetProviderSessionDetails_LoadsCursorUUIDSessionFromConfiguredRoot(t *testing.T) {
	root, sessionID := writeCursorProviderSessionUUIDFixture(t)

	srv := newTestServerWithCursorRoot(root)
	req := httptest.NewRequest("GET", "/provider-sessions/detail?provider=cursor&kind=session_id&id="+sessionID, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	resp := decodeJSONResponse[factoryapi.ProviderSessionDetailResponse](t, rec)
	if string(resp.ProviderSession.Provider) != "cursor" || string(resp.ProviderSession.Kind) != "session_id" || resp.ProviderSession.Id != sessionID {
		t.Fatalf("provider session = %#v, want cursor session_id %s", resp.ProviderSession, sessionID)
	}
	wantRelativePath := customerCursorWorkspaceHash + "/" + sessionID + "/store.db"
	if resp.Source.RelativePath != wantRelativePath || resp.Source.SizeBytes == 0 {
		t.Fatalf("source = %#v, want rooted cursor store.db metadata at %s", resp.Source, wantRelativePath)
	}
	if resp.Parse.EventCount != 1 || len(resp.Transcript) != 1 || stringValue(resp.Transcript[0].Text) != "Hello from API fixture" {
		t.Fatalf("response = %#v, want readable cursor transcript for UUID session_id", resp)
	}
}

func TestGetProviderSessionDetails_LoadsLegacyAgentCursorSessionFromConfiguredRoot(t *testing.T) {
	root, sessionID := writeCursorProviderSessionFixture(t)

	srv := newTestServerWithCursorRoot(root)
	req := httptest.NewRequest("GET", "/provider-sessions/detail?provider=agent&kind=session_id&id="+sessionID, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	resp := decodeJSONResponse[factoryapi.ProviderSessionDetailResponse](t, rec)
	if string(resp.ProviderSession.Provider) != "cursor" || string(resp.ProviderSession.Kind) != "session_id" || resp.ProviderSession.Id != sessionID {
		t.Fatalf("provider session = %#v, want canonical cursor session_id %s", resp.ProviderSession, sessionID)
	}
}

func assertProviderSessionResponseIdentity(t *testing.T, resp factoryapi.ProviderSessionDetailResponse) {
	t.Helper()

	if string(resp.ProviderSession.Provider) != "codex" || string(resp.ProviderSession.Kind) != "session_id" || resp.ProviderSession.Id != "sess_123" {
		t.Fatalf("provider session = %#v, want codex session_id sess_123", resp.ProviderSession)
	}
	if resp.Source.RelativePath != "2026/05/18/rollout-sess_123.jsonl" || resp.Source.SizeBytes == 0 {
		t.Fatalf("source = %#v, want rooted rollout path with metadata", resp.Source)
	}
}

func assertProviderSessionParseCounts(t *testing.T, parse factoryapi.ProviderSessionParseSummary) {
	t.Helper()

	if parse.LineCount != 4 || parse.EventCount != 3 || parse.MalformedLineCount != 1 || parse.UnknownEventCount != 1 {
		t.Fatalf("parse summary = %#v, want line/event/malformed/unknown counts", parse)
	}
}

func assertProviderSessionTranscriptSummary(t *testing.T, resp factoryapi.ProviderSessionDetailResponse) {
	t.Helper()

	if len(resp.Transcript) != 1 || resp.Transcript[0].Type != factoryapi.Reasoning || resp.Transcript[0].Order != 1 {
		t.Fatalf("transcript = %#v, want one reasoning transcript entry", resp.Transcript)
	}
	if len(resp.Parse.Turns) != 1 || resp.Parse.Turns[0].ReasoningCount != 1 || len(resp.Parse.Reasoning) != 1 || resp.Parse.Reasoning[0].SourceType != "reasoning" {
		t.Fatalf("parse detail = %#v, want reasoning turn summary", resp.Parse)
	}
}

func assertProviderSessionParseDiagnostics(t *testing.T, parse factoryapi.ProviderSessionParseSummary) {
	t.Helper()

	if len(parse.ParseErrors) != 1 || parse.ParseErrors[0].LineNumber != 4 || len(parse.UnknownEvents) != 1 || parse.UnknownEvents[0].LineNumber != 3 {
		t.Fatalf("parse diagnostics = %#v, want malformed line 4 and unknown line 3", parse)
	}
}
