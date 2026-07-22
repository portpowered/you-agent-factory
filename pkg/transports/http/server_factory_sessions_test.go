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
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/factorysessionfixtures"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	modelshttp "github.com/portpowered/infinite-you/pkg/services/models/transports/http"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func newLiveSessionTestServer(sessions apisurface.LiveSessionAPI) *Server {
	return newFactorySessionRolesTestServer(sessions, nil, nil, nil)
}

type httpRequestPreparationFake struct {
	factorysessions.RequestPreparation
}

func (httpRequestPreparationFake) PrepareListSessions(request factorysessions.ListSessionsRequest) (factorysessions.ListSessionsRequest, error) {
	if request.Scope == "" {
		request.Scope = factorysessions.SessionListScopeLive
	}
	return request, nil
}

type strictInvocationAPIFake struct {
	invoke func(context.Context, string, factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error)
}

type strictFactoryStatusAPIFake struct {
	project func(context.Context, string) (factoryruntime.FactoryStatus, error)
}

func (fake strictFactoryStatusAPIFake) ProjectFactoryStatus(ctx context.Context, sessionID string) (factoryruntime.FactoryStatus, error) {
	if fake.project == nil {
		panic("unexpected FactoryStatusAPI.ProjectFactoryStatus call")
	}
	return fake.project(ctx, sessionID)
}

func (fake strictInvocationAPIFake) InvokeFactorySession(ctx context.Context, sessionID string, request factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
	if fake.invoke == nil {
		panic("unexpected InvocationAPI.InvokeFactorySession call")
	}
	return fake.invoke(ctx, sessionID, request)
}

func newFactorySessionRolesTestServer(
	sessions apisurface.LiveSessionAPI,
	workAPI apisurface.WorkAPI,
	definitions apisurface.FactorySaveAPI,
	invocation apisurface.InvocationAPI,
	statusRoles ...apisurface.FactoryStatusAPI,
) *Server {
	var status apisurface.FactoryStatusAPI
	if len(statusRoles) > 0 {
		status = statusRoles[0]
	}
	workRead, _ := workAPI.(apisurface.WorkReadAPI)
	var liveLister factorysessions.LiveSessionListReader
	if sessions != nil {
		liveLister = httpLiveSessionListReader{sessions: sessions}
	}
	return newServerFromRoles(
		nil, status, sessions, workAPI, workRead, invocation, &modelshttp.Handler{},
		definitions, httpFactoryValidator{}, nil,
		nil, nil, nil, nil, nil, liveLister, nil, nil,
		newContentStagingFake(),
		&workRequestPreparationFake{prepare: func(_ context.Context, input work.WorkRequestPreparation) (work.WorkRequest, error) {
			return input.Request, nil
		}},
		httpRequestPreparationFake{}, zap.NewNop(),
	)
}

type httpLiveSessionListReader struct {
	sessions apisurface.LiveSessionAPI
}

func (reader httpLiveSessionListReader) ListScopedLiveSessions(ctx context.Context) ([]factorysessions.ScopedLiveSessionSummary, error) {
	response, err := reader.sessions.ListFactorySessions(ctx)
	if err != nil {
		return nil, err
	}
	rows := make([]factorysessions.ScopedLiveSessionSummary, 0, len(response.Sessions))
	for _, session := range response.Sessions {
		target := factorysessions.TargetRef{Kind: factorysessions.TargetKind(session.Target.Kind)}
		if session.Target.Name != nil {
			target.Name = strings.TrimSpace(*session.Target.Name)
		}
		rows = append(rows, factorysessions.ScopedLiveSessionSummary{
			ID: session.Id, FactoryDir: session.FactoryDir, FolderPath: session.FolderPath,
			Project: session.Project, IsDefault: session.IsDefault, Target: target,
		})
	}
	return rows, nil
}

func TestFactoryResponseEventsBySessionID_RetainedThenLiveUsesExactSessionAndFlushesEachMessage(t *testing.T) {
	subscription := factorysessionfixtures.NewFactoryResponseEventSubscription(2)
	wantRetained := responseProgressEvent(t, "session-beta", 1, "beta-retained")
	subscription.Batches <- responseEventRecords(t, wantRetained)
	var subscribedSession string
	srv := newLiveSessionTestServer(strictLiveSessionAPIFake{
		subscribe: func(
			_ context.Context,
			request factorysessions.ResponseEventSubscriptionRequest,
		) (apisurface.FactoryResponseEventSubscription, error) {
			subscribedSession = request.SessionID
			return subscription, nil
		},
	})
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
	if subscribedSession != "session-beta" {
		t.Fatalf("subscribed session = %q, want session-beta", subscribedSession)
	}
	wantLive := responseProgressEvent(t, "session-beta", 2, "beta-live")
	subscription.Batches <- responseEventRecords(t, wantLive)
	live := readSSEFactoryResponseEvent(t, reader)
	if live.EventID != wantLive.EventID || live.Sequence <= retained.Sequence {
		t.Fatalf("live response event = %#v, want event %q after sequence %d", live, wantLive.EventID, retained.Sequence)
	}

	cancel()
	select {
	case <-subscription.Detached:
	case <-time.After(time.Second):
		t.Fatal("response-event subscription was not detached after disconnect")
	}
}

func TestFactoryResponseEventsBySessionID_DisconnectDoesNotStopPublicationOrCatchUp(t *testing.T) {
	firstSubscription := factorysessionfixtures.NewFactoryResponseEventSubscription(1)
	retained := responseProgressEvent(t, "session-beta", 1, "before-disconnect")
	firstSubscription.Batches <- responseEventRecords(t, retained)
	secondSubscription := factorysessionfixtures.NewFactoryResponseEventSubscription(1)
	afterDisconnect := responseProgressEvent(t, "session-beta", 2, "after-disconnect")
	secondSubscription.Batches <- responseEventRecords(t, afterDisconnect)
	close(secondSubscription.Batches)
	var subscriptionCount int
	srv := newLiveSessionTestServer(strictLiveSessionAPIFake{
		subscribe: func(
			_ context.Context,
			request factorysessions.ResponseEventSubscriptionRequest,
		) (apisurface.FactoryResponseEventSubscription, error) {
			subscriptionCount++
			if request.SessionID != "session-beta" {
				t.Fatalf("subscribed session = %q, want session-beta", request.SessionID)
			}
			if subscriptionCount == 1 {
				return firstSubscription, nil
			}
			if request.AfterSequence != retained.Sequence {
				t.Fatalf("reconnect cursor = %d, want %d", request.AfterSequence, retained.Sequence)
			}
			return secondSubscription, nil
		},
	})
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
	select {
	case <-firstSubscription.Detached:
	case <-time.After(time.Second):
		t.Fatal("first response-event subscription was not detached")
	}
	catchUpRequest, err := http.NewRequest(
		http.MethodGet,
		httpServer.URL+"/factory-sessions/session-beta/response-events?after_sequence=1",
		nil,
	)
	if err != nil {
		t.Fatalf("new catch-up request: %v", err)
	}
	catchUp, err := http.DefaultClient.Do(catchUpRequest)
	if err != nil {
		t.Fatalf("open catch-up stream: %v", err)
	}
	defer catchUp.Body.Close()
	gotAfterDisconnect := readSSEFactoryResponseEvent(t, bufio.NewReader(catchUp.Body))
	if gotAfterDisconnect.EventID != afterDisconnect.EventID {
		t.Fatalf("event after reconnect = %#v, want %q", gotAfterDisconnect, afterDisconnect.EventID)
	}
}

func TestFactoryResponseEventsBySessionID_StaleCursorGetsGapBeforeRetainedAndLiveEvents(t *testing.T) {
	subscription := factorysessionfixtures.NewFactoryResponseEventSubscription(2)
	gap := responseStreamGapEvent(t, "session-beta", 2, 2, 3)
	retainedFirst := responseProgressEvent(t, "session-beta", 3, "retained-3")
	retainedSecond := responseProgressEvent(t, "session-beta", 4, "retained-4")
	subscription.Batches <- responseEventRecords(t, gap, retainedFirst, retainedSecond)
	srv := newLiveSessionTestServer(strictLiveSessionAPIFake{
		subscribe: func(
			_ context.Context,
			request factorysessions.ResponseEventSubscriptionRequest,
		) (apisurface.FactoryResponseEventSubscription, error) {
			if request.SessionID != "session-beta" || request.AfterSequence != 1 {
				t.Fatalf("subscription = (%q, %d), want (session-beta, 1)", request.SessionID, request.AfterSequence)
			}
			if len(request.Kinds) != 1 || request.Kinds[0] != factorysessions.ResponseEventKindProgress {
				t.Fatalf("kind selection = %#v, want [PROGRESS]", request.Kinds)
			}
			return subscription, nil
		},
	})
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
	gotGap := readSSEFactoryResponseEvent(t, reader)
	gapPayload := decodeSSEStreamGap(t, gotGap)
	if gapPayload.FromSequence != 2 || gapPayload.ToSequence != 2 || gapPayload.FirstAvailableSequence != 3 {
		t.Fatalf("STREAM_GAP payload = %#v, want unavailable 2..2 and first available 3", gapPayload)
	}
	retained := []factorysessions.FactoryResponseEvent{
		readSSEFactoryResponseEvent(t, reader),
		readSSEFactoryResponseEvent(t, reader),
	}
	if retained[0].EventID != retainedFirst.EventID || retained[1].EventID != retainedSecond.EventID {
		t.Fatalf("retained events = [%q %q], want [%q %q]", retained[0].EventID, retained[1].EventID, retainedFirst.EventID, retainedSecond.EventID)
	}
	liveWant := responseProgressEvent(t, "session-beta", 5, "live-5")
	subscription.Batches <- responseEventRecords(t, liveWant)
	live := readSSEFactoryResponseEvent(t, reader)
	if live.EventID != liveWant.EventID || live.Sequence != 5 {
		t.Fatalf("live event = %#v, want sequence 5 event %q", live, liveWant.EventID)
	}
}

func TestFactoryResponseEventsBySessionID_CursorInsideRetainedSuffixDoesNotGetGap(t *testing.T) {
	retainedFirst := responseProgressEvent(t, "session-beta", 3, "retained-3")
	retainedSecond := responseProgressEvent(t, "session-beta", 4, "retained-4")
	subscription := factorysessionfixtures.NewFactoryResponseEventSubscription(1)
	subscription.Batches <- responseEventRecords(t, retainedSecond)
	close(subscription.Batches)
	srv := newLiveSessionTestServer(strictLiveSessionAPIFake{
		subscribe: func(
			_ context.Context,
			request factorysessions.ResponseEventSubscriptionRequest,
		) (apisurface.FactoryResponseEventSubscription, error) {
			if request.AfterSequence != retainedFirst.Sequence {
				t.Fatalf("after sequence = %d, want %d", request.AfterSequence, retainedFirst.Sequence)
			}
			return subscription, nil
		},
	})

	request := httptest.NewRequest(http.MethodGet, "/factory-sessions/session-beta/response-events?after_sequence=3", nil)
	recorder := httptest.NewRecorder()
	srv.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("current response-event status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	got := readSSEFactoryResponseEvent(t, bufio.NewReader(recorder.Body))
	if got.Kind == factorysessions.ResponseEventKindStreamGap || got.EventID != retainedSecond.EventID {
		t.Fatalf("current cursor event = %#v, want retained event %q without gap after %q", got, retainedSecond.EventID, retainedFirst.EventID)
	}
}

func TestFactoryResponseEventsBySessionID_KnownCursorEmitsOnlyNewerEvents(t *testing.T) {
	first := responseProgressEvent(t, "session-beta", 1, "first")
	second := responseProgressEvent(t, "session-beta", 2, "second")
	third := responseProgressEvent(t, "session-beta", 3, "third")
	subscription := factorysessionfixtures.NewFactoryResponseEventSubscription(1)
	subscription.Batches <- responseEventRecords(t, second, third)
	close(subscription.Batches)
	srv := newLiveSessionTestServer(strictLiveSessionAPIFake{
		subscribe: func(
			_ context.Context,
			request factorysessions.ResponseEventSubscriptionRequest,
		) (apisurface.FactoryResponseEventSubscription, error) {
			if request.AfterSequence != first.Sequence {
				t.Fatalf("after sequence = %d, want %d", request.AfterSequence, first.Sequence)
			}
			return subscription, nil
		},
	})

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
	var subscribedSession string
	srv := newLiveSessionTestServer(strictLiveSessionAPIFake{
		subscribe: func(
			_ context.Context,
			request factorysessions.ResponseEventSubscriptionRequest,
		) (apisurface.FactoryResponseEventSubscription, error) {
			subscribedSession = request.SessionID
			return nil, apisurface.ErrFactorySessionNotFound
		},
	})

	request := httptest.NewRequest(http.MethodGet, "/factory-sessions/session-missing/response-events", nil)
	recorder := httptest.NewRecorder()
	srv.Handler().ServeHTTP(recorder, request)
	assertJSONError(t, recorder, http.StatusNotFound, "RESPONSE_EVENT_SESSION_NOT_FOUND", "factory response-event session not found")
	if subscribedSession != "session-missing" {
		t.Fatalf("subscribed session = %q, want session-missing without default fallback", subscribedSession)
	}
}

func TestFactoryResponseEventsBySessionID_CompletedWithinRetentionDrainsAndCloses(t *testing.T) {
	wantFirst := responseProgressEvent(t, "session-beta", 1, "completed-first")
	wantSecond := responseProgressEvent(t, "session-beta", 2, "completed-second")
	subscription := factorysessionfixtures.NewFactoryResponseEventSubscription(1)
	subscription.Batches <- responseEventRecords(t, wantFirst, wantSecond)
	close(subscription.Batches)
	srv := newLiveSessionTestServer(strictLiveSessionAPIFake{
		subscribe: func(
			context.Context,
			factorysessions.ResponseEventSubscriptionRequest,
		) (apisurface.FactoryResponseEventSubscription, error) {
			return subscription, nil
		},
	})
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
	srv := newLiveSessionTestServer(strictLiveSessionAPIFake{
		subscribe: func(
			context.Context,
			factorysessions.ResponseEventSubscriptionRequest,
		) (apisurface.FactoryResponseEventSubscription, error) {
			return nil, apisurface.ErrFactoryResponseEventStreamExpired
		},
	})
	request := httptest.NewRequest(http.MethodGet, "/factory-sessions/session-beta/response-events", nil)
	recorder := httptest.NewRecorder()
	srv.Handler().ServeHTTP(recorder, request)

	assertJSONError(t, recorder, http.StatusGone, "RESPONSE_EVENT_STREAM_EXPIRED", "factory response-event stream expired")
	if got := recorder.Header().Get("Content-Type"); strings.Contains(got, "text/event-stream") {
		t.Fatalf("expired stream Content-Type = %q, want typed JSON before SSE headers", got)
	}
}

func TestFactoryResponseEventsBySessionID_SubscriptionFailureReturnsTypedInternalError(t *testing.T) {
	srv := newLiveSessionTestServer(strictLiveSessionAPIFake{
		subscribe: func(
			context.Context,
			factorysessions.ResponseEventSubscriptionRequest,
		) (apisurface.FactoryResponseEventSubscription, error) {
			return nil, errors.New("subscription storage failed")
		},
	})
	request := httptest.NewRequest(http.MethodGet, "/factory-sessions/session-beta/response-events", nil)
	recorder := httptest.NewRecorder()
	srv.Handler().ServeHTTP(recorder, request)

	assertJSONError(t, recorder, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to subscribe to factory response events")
}

func responseProgressEvent(
	t *testing.T,
	sessionID string,
	sequence int64,
	label string,
) factorysessions.FactoryResponseEvent {
	t.Helper()
	payload, err := json.Marshal(factorysessions.ResponseEventProgress{Label: label})
	if err != nil {
		t.Fatalf("marshal response progress payload: %v", err)
	}
	return factorysessions.FactoryResponseEvent{
		SchemaVersion:    factorysessions.ResponseEventSchemaVersionV1,
		Sequence:         sequence,
		EventID:          fmt.Sprintf("response-event/%d", sequence),
		FactorySessionID: sessionID,
		Kind:             factorysessions.ResponseEventKindProgress,
		Phase:            factorysessions.ResponseEventPhaseUpdated,
		Provenance: factorysessions.ResponseEventProvenance{
			Provider:        "test-provider",
			NativeEventType: "progress",
			Delivery:        factorysessions.ResponseEventDeliveryNativeStream,
			Representation:  factorysessions.ResponseEventRepresentationNotification,
			Fidelity:        factorysessions.ResponseEventFidelityLossless,
		},
		RunID:      "run-test",
		RecordedAt: time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC),
		Payload:    payload,
	}
}

func responseStreamGapEvent(
	t *testing.T,
	sessionID string,
	fromSequence int64,
	toSequence int64,
	firstAvailableSequence int64,
) factorysessions.FactoryResponseEvent {
	t.Helper()
	payload, err := json.Marshal(factorysessions.ResponseEventStreamGap{
		FromSequence:           fromSequence,
		ToSequence:             toSequence,
		FirstAvailableSequence: firstAvailableSequence,
		Reason:                 "retention_limit",
	})
	if err != nil {
		t.Fatalf("marshal response stream gap: %v", err)
	}
	return factorysessions.FactoryResponseEvent{
		SchemaVersion:    factorysessions.ResponseEventSchemaVersionV1,
		EventID:          fmt.Sprintf("response-event/gap/%d/%d", fromSequence, toSequence),
		FactorySessionID: sessionID,
		Kind:             factorysessions.ResponseEventKindStreamGap,
		Phase:            factorysessions.ResponseEventPhaseUpdated,
		Provenance: factorysessions.ResponseEventProvenance{
			Provider:        "you-agent-factory",
			NativeEventType: "response.retention_gap",
			Delivery:        factorysessions.ResponseEventDeliverySynthesized,
			Representation:  factorysessions.ResponseEventRepresentationNotification,
			Fidelity:        factorysessions.ResponseEventFidelityLossy,
		},
		RecordedAt: time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC),
		Payload:    payload,
	}
}

func responseEventRecords(
	t *testing.T,
	events ...factorysessions.FactoryResponseEvent,
) []apisurface.FactoryResponseEventRecord {
	t.Helper()
	records := make([]apisurface.FactoryResponseEventRecord, 0, len(events))
	for _, event := range events {
		data, err := json.Marshal(event)
		if err != nil {
			t.Fatalf("marshal response event: %v", err)
		}
		records = append(records, apisurface.FactoryResponseEventRecord{
			Sequence: event.Sequence,
			Kind:     string(event.Kind),
			Data:     data,
		})
	}
	return records
}

func readSSEFactoryResponseEvent(t *testing.T, reader *bufio.Reader) factorysessions.FactoryResponseEvent {
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
	var event factorysessions.FactoryResponseEvent
	if err := json.Unmarshal([]byte(dataLine), &event); err != nil {
		t.Fatalf("decode response-event SSE data: %v", err)
	}
	if idLine != fmt.Sprint(event.Sequence) {
		t.Fatalf("SSE id = %q, want event sequence %d", idLine, event.Sequence)
	}
	return event
}

func decodeSSEStreamGap(t *testing.T, event factorysessions.FactoryResponseEvent) factorysessions.ResponseEventStreamGap {
	t.Helper()
	if event.Kind != factorysessions.ResponseEventKindStreamGap {
		t.Fatalf("first response event kind = %q, want STREAM_GAP", event.Kind)
	}
	var payload factorysessions.ResponseEventStreamGap
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatalf("decode STREAM_GAP payload: %v", err)
	}
	return payload
}

type sessionScopedHTTPObservation struct {
	eventStream    *interfaces.FactoryEventStream
	currentFactory factoryapi.Factory
	readItem       work.ReadModel
	WorkRequests   []work.WorkRequest
	ListWorkCalls  int
	GetWorkCalls   int
}

func newSessionScopedHTTPObservation(
	t *testing.T,
	_ time.Time,
	factoryID *string,
	factoryName string,
	tokenID string,
	workID string,
	historyEventID string,
) *sessionScopedHTTPObservation {
	t.Helper()
	return &sessionScopedHTTPObservation{
		readItem: work.ReadModel{CursorID: tokenID, WorkID: workID, Name: workID, WorkTypeName: "task", State: &work.State{Name: "init", Type: work.StateTypeInitial}},
		eventStream: &interfaces.FactoryEventStream{
			StreamGenerationID: "stream-gen-" + factoryName,
			History: []interfaces.FactoryEvent{canonicalFactoryEventForHTTPTest(t, factoryapi.FactoryEvent{
				Id: historyEventID, Type: factoryapi.FactoryEventTypeWorkRequest,
			})},
			Events: make(chan interfaces.FactoryEvent),
		},
		currentFactory: factoryapi.Factory{Name: factoryapi.FactoryName(factoryName), Id: factoryID},
	}
}

func newSessionScopedRolesTestServer(sessions map[string]*sessionScopedHTTPObservation) *Server {
	lookup := func(sessionID string) (*sessionScopedHTTPObservation, error) {
		session, ok := sessions[sessionID]
		if !ok {
			return nil, apisurface.ErrFactorySessionNotFound
		}
		return session, nil
	}
	workRole := strictWorkAPIFake{
		submit: func(_ context.Context, sessionID string, request work.WorkRequest) (work.WorkRequestSubmitResult, error) {
			session, err := lookup(sessionID)
			if err != nil {
				return work.WorkRequestSubmitResult{}, err
			}
			session.WorkRequests = append(session.WorkRequests, request)
			return acceptProgrammedHTTPWorkRequest(request), nil
		},
		list: func(_ context.Context, sessionID string, _ work.ListOptions) (work.ListResult, error) {
			session, err := lookup(sessionID)
			if err != nil {
				return work.ListResult{}, err
			}
			session.ListWorkCalls++
			return work.ListResult{Results: []work.ReadModel{session.readItem}, MaxResults: work.DefaultListMaxResults}, nil
		},
		getWork: func(_ context.Context, sessionID, id string) (work.ReadModel, error) {
			session, err := lookup(sessionID)
			if err != nil {
				return work.ReadModel{}, err
			}
			session.GetWorkCalls++
			if id != session.readItem.CursorID && id != session.readItem.WorkID {
				return work.ReadModel{}, work.ErrWorkNotFound
			}
			return session.readItem, nil
		},
		subscribe: func(_ context.Context, sessionID string, _ *interfaces.FactoryEventReconnectCursor) (*interfaces.FactoryEventStream, error) {
			session, err := lookup(sessionID)
			if err != nil {
				return nil, err
			}
			return session.eventStream, nil
		},
	}
	liveRole := strictLiveSessionAPIFake{get: func(_ context.Context, sessionID string) (factoryapi.FactorySession, error) {
		if _, err := lookup(sessionID); err != nil {
			return factoryapi.FactorySession{}, err
		}
		return factoryapi.FactorySession{Id: sessionID}, nil
	}}
	definitions := strictFactorySaveAPIFake{getCurrent: func(_ context.Context, sessionID string) (factoryapi.Factory, error) {
		session, err := lookup(sessionID)
		if err != nil {
			return factoryapi.Factory{}, err
		}
		return session.currentFactory, nil
	}}
	status := strictFactoryStatusAPIFake{project: func(_ context.Context, sessionID string) (factoryruntime.FactoryStatus, error) {
		_, err := lookup(sessionID)
		if err != nil {
			return factoryruntime.FactoryStatus{}, err
		}
		return factoryruntime.FactoryStatus{TotalTokens: 1}, nil
	}}
	return newFactorySessionRolesTestServer(liveRole, workRole, definitions, nil, status)
}

func TestSessionScopedAPI_ReadsAndMutationsTargetOnlyRequestedSession(t *testing.T) {
	now := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	defaultFactoryID := "root-runtime"
	betaFactoryID := "beta-runtime"
	defaultSession := newSessionScopedHTTPObservation(t, now, &defaultFactoryID, apisurface.DefaultCurrentFactoryName, "tok-default-1", "default-work-1", "factory-event/work-request/default-history")
	betaSession := newSessionScopedHTTPObservation(t, now, &betaFactoryID, "beta", "tok-beta-1", "beta-work-1", "factory-event/work-request/beta-history")
	srv := newSessionScopedRolesTestServer(map[string]*sessionScopedHTTPObservation{
		factorysessions.DefaultSessionID: defaultSession,
		"session-beta":                   betaSession,
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

func assertScopedSessionSubmit(t *testing.T, serverURL string, betaSession *sessionScopedHTTPObservation, defaultSession *sessionScopedHTTPObservation) {
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

func assertScopedSessionList(t *testing.T, serverURL string, betaSession *sessionScopedHTTPObservation, defaultSession *sessionScopedHTTPObservation) {
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
	if betaSession.ListWorkCalls == 0 {
		t.Fatal("expected scoped GET /work to call the targeted Work read role")
	}
	if defaultSession.ListWorkCalls != 0 {
		t.Fatalf("default session ListWork calls = %d, want 0 after scoped list", defaultSession.ListWorkCalls)
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
	srv := newFactorySessionRolesTestServer(nil, nil, nil, nil, strictFactoryStatusAPIFake{
		project: func(context.Context, string) (factoryruntime.FactoryStatus, error) {
			return factoryruntime.FactoryStatus{}, apisurface.ErrFactorySessionNotFound
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/factory-sessions/missing-session/status", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	assertJSONError(t, rec, http.StatusNotFound, "NOT_FOUND", "factory session not found")
}

func TestFactorySessionsAPI_ListFactorySessions(t *testing.T) {
	srv := newLiveSessionTestServer(strictLiveSessionAPIFake{list: func(context.Context) (factoryapi.ListFactorySessionsResponse, error) {
		return factoryapi.ListFactorySessionsResponse{
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
		}, nil
	}})

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
	var opened []factoryapi.OpenFactorySessionRequest
	srv := newLiveSessionTestServer(strictLiveSessionAPIFake{open: func(_ context.Context, request factoryapi.OpenFactorySessionRequest) (factoryapi.OpenFactorySessionResponse, error) {
		opened = append(opened, request)
		return factoryapi.OpenFactorySessionResponse{
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
		}, nil
	}})

	req := httptest.NewRequest(http.MethodPost, "/factory-sessions", bytes.NewBufferString(`{"folderPath":"/workspace/fleet","target":{"kind":"named","name":"beta"}}`))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /factory-sessions status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if len(opened) != 1 {
		t.Fatalf("opened factory sessions = %d, want 1", len(opened))
	}
	if opened[0].FolderPath != "/workspace/fleet" {
		t.Fatalf("opened session folder = %q, want /workspace/fleet", opened[0].FolderPath)
	}
	if opened[0].Target == nil ||
		opened[0].Target.Kind != factoryapi.FactorySessionTargetRefKindNamed ||
		opened[0].Target.Name == nil ||
		*opened[0].Target.Name != "beta" {
		t.Fatalf("opened session target = %#v, want named beta", opened[0].Target)
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
	srv := newLiveSessionTestServer(strictLiveSessionAPIFake{open: func(context.Context, factoryapi.OpenFactorySessionRequest) (factoryapi.OpenFactorySessionResponse, error) {
		return factoryapi.OpenFactorySessionResponse{}, apiTestSessionValidationError{
			message: "folder validation failed",
			targets: []factoryapi.FactoryValidationTarget{{
				Code:     "factory.session.field.missing",
				Severity: factoryapi.FactoryValidationSeverityError,
				Message:  "folder validation failed",
				Subject: factoryapi.FactoryValidationSubject{
					Type:     factoryapi.FactoryValidationSubjectTypeFactory,
					Id:       "folderPath",
					Location: factoryapi.FactoryValidationSubjectLocationReference,
				},
			}},
		}
	}})

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
	srv := newLiveSessionTestServer(strictLiveSessionAPIFake{open: func(context.Context, factoryapi.OpenFactorySessionRequest) (factoryapi.OpenFactorySessionResponse, error) {
		return factoryapi.OpenFactorySessionResponse{}, apiTestSessionValidationError{
			message: "factory configuration could not be loaded from the selected folder",
			code:    "FACTORY_SESSION_CONFIG_LOAD_FAILED",
			targets: []factoryapi.FactoryValidationTarget{
				apisurface.FactoryValidationTargetToAPI(interfaces.FactorySessionTargetValidationTarget(
					"config_load_failed",
					"default",
					`Factory target "default" at "/workspace/fleet" could not be loaded: unexpected end of JSON input`,
				)),
			},
		}
	}})

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
	var closed []string
	srv := newLiveSessionTestServer(strictLiveSessionAPIFake{close: func(_ context.Context, sessionID string) error {
		closed = append(closed, sessionID)
		return nil
	}})

	req := httptest.NewRequest(http.MethodDelete, "/factory-sessions/session-beta", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE /factory-sessions/session-beta status = %d, want 204: %s", rec.Code, rec.Body.String())
	}
	if len(closed) != 1 || closed[0] != "session-beta" {
		t.Fatalf("closed factory sessions = %#v, want [session-beta]", closed)
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
	srv := newLiveSessionTestServer(strictLiveSessionAPIFake{close: func(context.Context, string) error {
		return apisurface.ErrFactorySessionNotFound
	}})

	req := httptest.NewRequest(http.MethodDelete, "/factory-sessions/missing-session", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	assertJSONError(t, rec, http.StatusNotFound, "NOT_FOUND", "factory session not found")
}

func TestGetCurrentFactoryBySessionId_ReturnsSessionDefinitionAndVersion(t *testing.T) {
	sessionVersion := factoryapi.HybridLogicalTimestamp{Physical: time.Unix(0, 2).UTC(), Logical: 2}
	srv := newFactorySessionRolesTestServer(nil, nil, strictFactorySaveAPIFake{getCurrent: func(_ context.Context, sessionID string) (factoryapi.Factory, error) {
		if sessionID != "session-2" {
			return factoryapi.Factory{}, apisurface.ErrFactorySessionNotFound
		}
		return factoryapi.Factory{Name: "beta", Version: &sessionVersion}, nil
	}}, nil)

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
	sessionVersion := factoryapi.HybridLogicalTimestamp{Physical: time.Unix(0, 2).UTC(), Logical: 2}
	savedBySession := make(map[string][]factoryapi.Factory)
	srv := newFactorySessionRolesTestServer(nil, nil, strictFactorySaveAPIFake{save: func(_ context.Context, sessionID string, _ factoryapi.FactorySaveMode, request factoryapi.Factory) (factoryapi.Factory, error) {
		if sessionID != "session-2" && sessionID != factorysessions.DefaultSessionID {
			return factoryapi.Factory{}, apisurface.ErrFactorySessionNotFound
		}
		savedBySession[sessionID] = append(savedBySession[sessionID], request)
		request.Version = &sessionVersion
		return request, nil
	}}, nil)

	body := saveFactoryForSessionRequestBody(`{"name":"beta","version":{"physical":"1970-01-01T00:00:00.000000002Z","logical":2},"workTypes":[],"workstations":[],"workers":[]}`)
	req := httptest.NewRequest(http.MethodPut, "/factory-sessions/session-2/factory", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("PUT factory status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if len(savedBySession[factorysessions.DefaultSessionID]) != 0 {
		t.Fatalf("default session save count = %d, want 0", len(savedBySession[factorysessions.DefaultSessionID]))
	}
	if len(savedBySession["session-2"]) != 1 {
		t.Fatalf("session save count = %d, want 1", len(savedBySession["session-2"]))
	}
	saved := savedBySession["session-2"][0]
	if saved.Name != "beta" {
		t.Fatalf("saved factory = %#v, want beta definition", saved)
	}
}

func TestCurrentFactoryBySessionId_UnknownSessionReturnsNotFound(t *testing.T) {
	srv := newFactorySessionRolesTestServer(nil, nil, strictFactorySaveAPIFake{
		getCurrent: func(context.Context, string) (factoryapi.Factory, error) {
			return factoryapi.Factory{}, apisurface.ErrFactorySessionNotFound
		},
		save: func(context.Context, string, factoryapi.FactorySaveMode, factoryapi.Factory) (factoryapi.Factory, error) {
			return factoryapi.Factory{}, apisurface.ErrFactorySessionNotFound
		},
	}, nil)

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
			assertStrictFactorySessionInvocation(t, tc.body, tc.result, tc.wantSubmitText)
		})
	}
}

func assertStrictFactorySessionInvocation(
	t *testing.T,
	body string,
	wantResult apisurface.FactoryInvocationResult,
	wantSubmitText string,
) {
	t.Helper()
	var invoked []factoryapi.InvocationRequest
	role := strictInvocationAPIFake{invoke: func(_ context.Context, sessionID string, request factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
		if sessionID != factorysessions.DefaultSessionID {
			return apisurface.FactoryInvocationResult{}, apisurface.ErrFactorySessionNotFound
		}
		invoked = append(invoked, request)
		return wantResult, nil
	}}
	srv := newFactorySessionRolesTestServer(nil, nil, nil, role)
	req := httptest.NewRequest(http.MethodPost, "/factory-sessions/~default/invocations", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /factory-sessions/~default/invocations status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	response := decodeJSONResponse[factoryapi.InvocationResponse](t, rec)
	if response.RequestId != wantResult.RequestID || response.TraceId != wantResult.TraceID || response.Status != factoryapi.InvocationTerminalStatus(wantResult.Status) {
		t.Fatalf("invocation response = %#v, want completed invocation identifiers", response)
	}
	assertGeneratedWorkContentParts(t, response.PrimaryResult, wantResult.PrimaryResult)
	if wantSubmitText != "" {
		if len(invoked) != 1 {
			t.Fatalf("invoked factory sessions = %d, want 1", len(invoked))
		}
		if got := extractInvocationRequestText(t, &invoked[0]); got != wantSubmitText {
			t.Fatalf("invocation text = %q, want %q", got, wantSubmitText)
		}
	}
}

func TestFactorySessionsAPI_InvokeFactorySession_DecodesStructuredArgs(t *testing.T) {
	var invoked []factoryapi.InvocationRequest
	role := strictInvocationAPIFake{invoke: func(_ context.Context, _ string, request factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
		invoked = append(invoked, request)
		return apisurface.FactoryInvocationResult{
			RequestID: "invoke-structured-1",
			TraceID:   "trace-structured-1",
			Status:    "COMPLETED",
			PrimaryResult: []work.WorkContentPart{{
				Type: work.WorkContentPartTypeText,
				Text: "ok",
			}},
		}, nil
	}}

	srv := newFactorySessionRolesTestServer(nil, nil, nil, role)
	req := httptest.NewRequest(http.MethodPost, "/factory-sessions/~default/invocations", bytes.NewBufferString(`{"args":{"input":"hello","tag":["alpha","beta"]}}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /factory-sessions/~default/invocations status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if len(invoked) != 1 {
		t.Fatalf("invoked factory sessions = %d, want 1", len(invoked))
	}
	if invoked[0].Args == nil {
		t.Fatal("invocation args = nil, want decoded args map")
	}
	if got := (*invoked[0].Args)["input"]; got != "hello" {
		t.Fatalf("args[input] = %#v, want hello", got)
	}
}

func TestFactorySessionsAPI_InvokeFactorySession_InputConflictReturnsStableBadRequest(t *testing.T) {
	const namedGoalParityText = "Plan the sprint from CLI and API parity coverage"
	conflictMessage := "invocation input sources conflict: positional_text, stdin_text"

	srv := newFactorySessionRolesTestServer(nil, nil, nil, strictInvocationAPIFake{invoke: func(context.Context, string, factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
		return apisurface.FactoryInvocationResult{}, &work.InputError{
			Code:    work.InputErrorCodeSourceConflict,
			Message: conflictMessage,
		}
	}})

	body := `{"sourceKind":"text","content":[{"type":"text","text":"` + namedGoalParityText + `"}]}`
	req := httptest.NewRequest(http.MethodPost, "/factory-sessions/~default/invocations", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	assertJSONError(t, rec, http.StatusBadRequest, string(work.InputErrorCodeSourceConflict), conflictMessage)
}

func TestGetProviderSessionDetails_NotFoundIsDistinguishable(t *testing.T) {
	srv := newTestServerWithProviderSessionCalls(t, providerSessionFailure("codex", "session_id", "missing-session", providersessions.ErrSessionNotFound))
	req := httptest.NewRequest("GET", "/provider-sessions/detail?provider=codex&kind=session_id&id=missing-session", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assertJSONError(t, rec, http.StatusNotFound, "NOT_FOUND", "provider session not found")
}

func TestGetProviderSessionDetails_CursorNotFoundIsDistinguishable(t *testing.T) {
	srv := newTestServerWithProviderSessionCalls(t, providerSessionFailure("cursor", "session_id", "missing-session", providersessions.ErrSessionNotFound))
	req := httptest.NewRequest("GET", "/provider-sessions/detail?provider=cursor&kind=session_id&id=missing-session", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assertJSONError(t, rec, http.StatusNotFound, "NOT_FOUND", "provider session not found")
}

func TestGetProviderSessionDetails_LegacyAgentCursorNotFoundIsDistinguishable(t *testing.T) {
	srv := newTestServerWithProviderSessionCalls(t, providerSessionFailure("agent", "session_id", "missing-session", providersessions.ErrSessionNotFound))
	req := httptest.NewRequest("GET", "/provider-sessions/detail?provider=agent&kind=session_id&id=missing-session", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assertJSONError(t, rec, http.StatusNotFound, "NOT_FOUND", "provider session not found")
}

func TestGetProviderSessionDetails_CursorNotFoundLogsDiagnostic(t *testing.T) {
	root := t.TempDir()
	core, logs := observer.New(zap.InfoLevel)
	srv := newTestServerWithProviderSessionCallsAndLogger(t, zap.New(core), providerSessionFailure(
		"cursor", "session_id", "missing-session",
		providerSessionLookupFailure(providersessions.ProviderCursor, root, providersessions.ErrSessionNotFound),
	))

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
	srv := newTestServerWithProviderSessionCalls(t, providerSessionFailure("cursor", "session_id", "missing-session", providersessions.ErrSessionNotFound))
	req := httptest.NewRequest("GET", "/provider-sessions/detail?provider=cursor&kind=session_id&id=missing-session", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assertJSONError(t, rec, http.StatusNotFound, "NOT_FOUND", "provider session not found")
}

func TestGetProviderSessionDetails_CursorNotFoundWithMissingRootDirectory(t *testing.T) {
	srv := newTestServerWithProviderSessionCalls(t, providerSessionFailure("cursor", "session_id", "missing-session", providersessions.ErrSessionNotFound))
	req := httptest.NewRequest("GET", "/provider-sessions/detail?provider=cursor&kind=session_id&id=missing-session", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assertJSONError(t, rec, http.StatusNotFound, "NOT_FOUND", "provider session not found")
}

func TestGetProviderSessionDetails_CursorNotFoundLogsDiagnosticWhenRootUnconfigured(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	srv := newTestServerWithProviderSessionCallsAndLogger(t, zap.New(core), providerSessionFailure(
		"cursor", "session_id", "missing-session",
		providerSessionLookupFailure(providersessions.ProviderCursor, "", providersessions.ErrSessionNotFound),
	))

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

func TestGetProviderSessionDetails_RejectsPathLikeAndMalformedIdentifiers(t *testing.T) {
	for _, test := range []struct{ target, id string }{
		{"/provider-sessions/detail?provider=codex&kind=session_id&id=../secret", "../secret"},
		{"/provider-sessions/detail?provider=codex&kind=session_id&id=/tmp/rollout-session.jsonl", "/tmp/rollout-session.jsonl"},
		{"/provider-sessions/detail?provider=codex&kind=session_id&id=session.with.dot", "session.with.dot"},
	} {
		t.Run(test.target, func(t *testing.T) {
			srv := newTestServerWithProviderSessionCalls(t, providerSessionFailure("codex", "session_id", test.id, providersessions.ErrInvalidIdentifier))
			req := httptest.NewRequest("GET", test.target, nil)
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, req)
			assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "provider session must be a codex session_id identifier without path separators")
		})
	}
}

func TestGetProviderSessionDetails_CursorRejectsPathLikeAndMalformedIdentifiers(t *testing.T) {
	for _, test := range []struct{ target, id string }{
		{"/provider-sessions/detail?provider=cursor&kind=session_id&id=../secret", "../secret"},
		{"/provider-sessions/detail?provider=cursor&kind=session_id&id=/tmp/store.db", "/tmp/store.db"},
		{"/provider-sessions/detail?provider=cursor&kind=session_id&id=session.with.dot", "session.with.dot"},
	} {
		t.Run(test.target, func(t *testing.T) {
			srv := newTestServerWithProviderSessionCalls(t, providerSessionFailure(
				"cursor", "session_id", test.id,
				providerSessionLookupFailure(providersessions.ProviderCursor, "", providersessions.ErrInvalidIdentifier),
			))
			req := httptest.NewRequest("GET", test.target, nil)
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, req)
			assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "provider session must be a cursor session_id identifier without path separators")
		})
	}
}

func TestGetProviderSessionDetails_LegacyAgentCursorRejectsPathLikeAndMalformedIdentifiers(t *testing.T) {
	for _, test := range []struct{ target, id string }{
		{"/provider-sessions/detail?provider=agent&kind=session_id&id=../secret", "../secret"},
		{"/provider-sessions/detail?provider=agent&kind=session_id&id=/tmp/store.db", "/tmp/store.db"},
		{"/provider-sessions/detail?provider=agent&kind=session_id&id=session.with.dot", "session.with.dot"},
	} {
		t.Run(test.target, func(t *testing.T) {
			srv := newTestServerWithProviderSessionCalls(t, providerSessionFailure(
				"agent", "session_id", test.id,
				providerSessionLookupFailure(providersessions.ProviderCursor, "", providersessions.ErrInvalidIdentifier),
			))
			req := httptest.NewRequest("GET", test.target, nil)
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, req)
			assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "provider session must be a cursor session_id identifier without path separators")
		})
	}
}

func TestGetProviderSessionDetails_RejectsUnsupportedProviderOrKindByContract(t *testing.T) {
	for _, test := range []struct {
		target, provider, kind string
		err                    error
	}{
		{"/provider-sessions/detail?provider=openai&kind=session_id&id=sess-123", "openai", "session_id", providersessions.ErrUnsupportedProvider},
		{"/provider-sessions/detail?provider=codex&kind=path&id=sess-123", "codex", "path", providersessions.ErrUnsupportedKind},
		{"/provider-sessions/detail?provider=cursor&kind=path&id=sess-123", "cursor", "path", providersessions.ErrUnsupportedKind},
	} {
		t.Run(test.target, func(t *testing.T) {
			srv := newTestServerWithProviderSessionCalls(t, providerSessionFailure(test.provider, test.kind, "sess-123", test.err))
			req := httptest.NewRequest("GET", test.target, nil)
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, req)
			assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "invalid request parameter")
		})
	}
}

func TestGetProviderSessionDetails_EventRefRoundTripLoadsCursorAndCodex(t *testing.T) {
	cursorSessionID := customerCursorProviderSessionID
	codexDetail := codexProviderSessionDetail("sess_123", "2026/05/18/rollout-sess_123.jsonl", 3)
	cursorDetail := cursorProviderSessionDetail(cursorSessionID, customerCursorWorkspaceHash+"/"+cursorSessionID+"/store.db")
	srv := newTestServerWithProviderSessionCalls(t,
		providerSessionSuccess("codex", "sess_123", codexDetail),
		providerSessionSuccess("cursor", cursorSessionID, cursorDetail),
		providerSessionSuccess("agent", cursorSessionID, cursorDetail),
		providerSessionSuccess("cursor", cursorSessionID, cursorDetail),
	)

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

	canonicalizedLegacyRef := factoryapi.LoadableProviderSessionRef{
		Provider: factoryapi.Cursor,
		Kind:     factoryapi.LoadableProviderSessionKindSessionID,
		Id:       cursorSessionID,
	}
	if string(canonicalizedLegacyRef.Provider) != string(factoryapi.Cursor) {
		t.Fatalf("canonicalized legacy ref provider = %q, want cursor", canonicalizedLegacyRef.Provider)
	}
	assertProviderSessionDetailLoadsFromEventRef(t, srv, canonicalizedLegacyRef, factoryapi.Cursor)
}

func TestGetProviderSessionDetails_RegressionLoadsCodexAndCursorFromConfiguredRoots(t *testing.T) {
	cursorSessionID := customerCursorProviderSessionID
	srv := newTestServerWithProviderSessionCalls(t,
		providerSessionSuccess("codex", "sess_123", codexProviderSessionDetail("sess_123", "2026/05/18/rollout-sess_123.jsonl", 3)),
		providerSessionSuccess("cursor", cursorSessionID, cursorProviderSessionDetail(cursorSessionID, customerCursorWorkspaceHash+"/"+cursorSessionID+"/store.db")),
	)

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
	srv := newTestServerWithProviderSessionCalls(t, providerSessionSuccess(
		"codex", "sess_123", codexProviderSessionDetail("sess_123", "2026/05/18/rollout-sess_123.jsonl", 3),
	))
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
	sessionID := "cursor-api-readable"
	srv := newTestServerWithProviderSessionCalls(t, providerSessionSuccess(
		"cursor", sessionID, cursorProviderSessionDetail(sessionID, "workspace-hash/"+sessionID+"/store.db"),
	))
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
	sessionID := customerCursorProviderSessionID
	srv := newTestServerWithProviderSessionCalls(t, providerSessionSuccess(
		"cursor", sessionID, cursorProviderSessionDetail(sessionID, customerCursorWorkspaceHash+"/"+sessionID+"/store.db"),
	))
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
	sessionID := "cursor-api-readable"
	srv := newTestServerWithProviderSessionCalls(t, providerSessionSuccess(
		"agent", sessionID, cursorProviderSessionDetail(sessionID, "workspace-hash/"+sessionID+"/store.db"),
	))
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
