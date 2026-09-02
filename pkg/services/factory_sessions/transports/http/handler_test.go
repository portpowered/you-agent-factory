package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"go.uber.org/zap"
)

type liveSessionAPIFake struct {
	apisurface.LiveSessionAPI
	get func(context.Context, string) (factoryapi.FactorySession, error)
}

type invocationAPIFake struct {
	err error
}

func (fake invocationAPIFake) InvokeFactorySession(
	context.Context,
	string,
	factoryapi.InvocationRequest,
) (apisurface.FactoryInvocationResult, error) {
	return apisurface.FactoryInvocationResult{}, fake.err
}

func (fake liveSessionAPIFake) GetFactorySession(ctx context.Context, id string) (factoryapi.FactorySession, error) {
	if fake.get == nil {
		panic("unexpected GetFactorySession call")
	}
	return fake.get(ctx, id)
}

func TestHandlerGetFactorySessionOwnsServiceInvocationAndEncoding(t *testing.T) {
	handler := NewHandler(Dependencies{Sessions: liveSessionAPIFake{
		get: func(_ context.Context, id string) (factoryapi.FactorySession, error) {
			return factoryapi.FactorySession{
				Id: id, FactoryDir: "/workspace/alpha", FolderPath: "/workspace",
				Project: "alpha", Target: factoryapi.FactorySessionTargetRef{Kind: factoryapi.FactorySessionTargetRefKindNamed},
				Runtime: factoryapi.FactorySessionRuntime{},
			}, nil
		},
	}}, zap.NewNop())
	recorder := httptest.NewRecorder()

	handler.GetFactorySession(recorder, httptest.NewRequest(http.MethodGet, "/factory-sessions/session-alpha", nil), "session-alpha")

	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"id":"session-alpha"`) {
		t.Fatalf("response = %d %s, want encoded session list", recorder.Code, recorder.Body.String())
	}
}

func TestHandlerOpenFactorySessionRejectsInvalidPayloadBeforeServiceInvocation(t *testing.T) {
	handler := NewHandler(Dependencies{Sessions: liveSessionAPIFake{}}, zap.NewNop())
	recorder := httptest.NewRecorder()

	handler.OpenFactorySession(
		recorder,
		httptest.NewRequest(http.MethodPost, "/factory-sessions", strings.NewReader(`{"target":{"kind":"default"},"unknown":true}`)),
	)

	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), `"code":"BAD_REQUEST"`) {
		t.Fatalf("response = %d %s, want typed bad request", recorder.Code, recorder.Body.String())
	}
}

func TestHandlerInvokeFactorySessionPreservesWrappedPayloadLimitDiagnostic(t *testing.T) {
	payloadSize := &work.PayloadSizeError{
		WorkName: "invocation", PayloadBytes: 65537, PayloadLimitBytes: 65536,
	}
	handler := NewHandler(Dependencies{
		Invocation: invocationAPIFake{err: fmt.Errorf("prepare invocation: %w", payloadSize)},
	}, zap.NewNop())
	recorder := httptest.NewRecorder()

	handler.InvokeFactorySessionBySessionId(
		recorder,
		httptest.NewRequest(http.MethodPost, "/factory-sessions/session-alpha/invocations", strings.NewReader(`{}`)),
		"session-alpha",
	)

	if recorder.Code != http.StatusBadRequest ||
		!strings.Contains(recorder.Body.String(), "payloadBytes=65537 payloadLimitBytes=65536") {
		t.Fatalf("response = %d %s, want preserved Work payload limit", recorder.Code, recorder.Body.String())
	}
}

func TestRequestAcceptsJSONContentType(t *testing.T) {
	tests := []struct {
		header string
		want   bool
	}{
		{header: "", want: true},
		{header: "application/json", want: true},
		{header: "application/problem+json", want: true},
		{header: "application/json; charset=utf-8", want: true},
		{header: "text/plain", want: false},
		{header: "application/xml", want: false},
		{header: "not-a-media-type", want: false},
	}
	for _, tc := range tests {
		if got := requestAcceptsJSONContentType(tc.header); got != tc.want {
			t.Fatalf("requestAcceptsJSONContentType(%q) = %v, want %v", tc.header, got, tc.want)
		}
	}
}

func TestHandlerOpenFactorySessionRejectsUnsupportedMediaTypeBeforeServiceInvocation(t *testing.T) {
	handler := NewHandler(Dependencies{Sessions: liveSessionAPIFake{}}, zap.NewNop())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/factory-sessions", strings.NewReader(`{"target":{"kind":"default"}}`))
	request.Header.Set("Content-Type", "text/plain")

	handler.OpenFactorySession(recorder, request)

	if recorder.Code != http.StatusUnsupportedMediaType ||
		!strings.Contains(recorder.Body.String(), `"code":"UNSUPPORTED_MEDIA_TYPE"`) {
		t.Fatalf("response = %d %s, want unsupported media type", recorder.Code, recorder.Body.String())
	}
}

func TestHandlerGetFactorySessionMapsUnavailableDependency(t *testing.T) {
	handler := NewHandler(Dependencies{}, zap.NewNop())
	recorder := httptest.NewRecorder()

	handler.GetFactorySession(recorder, httptest.NewRequest(http.MethodGet, "/factory-sessions/session-alpha", nil), "session-alpha")

	if recorder.Code != http.StatusInternalServerError || !strings.Contains(recorder.Body.String(), "session-scoped API is unavailable") {
		t.Fatalf("response = %d %s, want unavailable dependency error", recorder.Code, recorder.Body.String())
	}
}

type factoryStatusProjectorRole struct {
	status      factoryruntime.FactoryStatus
	projections int
}

type factoryStatusSessionRole struct {
	observations int
	sessionIDs   []string
}

func (role *factoryStatusProjectorRole) ProjectFactoryStatusFromObservation(factoryruntime.Observation) factoryruntime.FactoryStatus {
	role.projections++
	return role.status
}

func (role *factoryStatusSessionRole) ObserveForSession(
	_ context.Context,
	sessionID string,
	_ factoryruntime.ObserveRequest,
) (factoryruntime.ObserveResult, error) {
	role.observations++
	role.sessionIDs = append(role.sessionIDs, sessionID)
	if sessionID == "missing" {
		return factoryruntime.ObserveResult{}, factorysessions.ErrSessionNotFound
	}
	return factoryruntime.ObserveResult{
		Observation: factoryruntime.Observation{
			Health: factoryruntime.ObservationHealth{FactoryState: "SESSION_" + sessionID},
		},
	}, nil
}

func TestFactoryStatusAPIUsesBoundSessionRuntimeForCurrentAndSessionRoutes(t *testing.T) {
	sessions := &factoryStatusSessionRole{}
	projector := &factoryStatusProjectorRole{status: factoryruntime.FactoryStatus{
		FactoryState:  "CURRENT",
		RuntimeStatus: "ACTIVE",
		TotalTokens:   2,
		Categories:    factoryruntime.FactoryStatusCategories{Processing: 1},
	}}
	api := NewFactoryStatusAPI(sessions, projector)

	assertFactoryStatusRequest(t, api, "", sessions, projector, 1)
	assertFactoryStatusRequest(t, api, "session-beta", sessions, projector, 2)
	if len(sessions.sessionIDs) != 2 || sessions.sessionIDs[0] != factorysessions.DefaultSessionID || sessions.sessionIDs[1] != "session-beta" {
		t.Fatalf("session IDs = %#v, want [~default session-beta]", sessions.sessionIDs)
	}
}

func assertFactoryStatusRequest(
	t *testing.T,
	api apisurface.FactoryStatusAPI,
	sessionID string,
	sessions *factoryStatusSessionRole,
	projector *factoryStatusProjectorRole,
	expectedCalls int,
) {
	t.Helper()
	got, err := api.ProjectFactoryStatus(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("status error = %v", err)
	}
	if got.FactoryState != "CURRENT" || got.RuntimeStatus != "ACTIVE" || got.TotalTokens != 2 || got.Categories.Processing != 1 {
		t.Fatalf("status = %#v, want injected projector result", got)
	}
	if sessions.observations != expectedCalls || projector.projections != expectedCalls {
		t.Fatalf("observations = %d, projections = %d, want %d", sessions.observations, projector.projections, expectedCalls)
	}
}

func TestFactoryStatusAPIReturnsMissingSessionError(t *testing.T) {
	t.Parallel()

	api := NewFactoryStatusAPI(
		&factoryStatusSessionRole{},
		&factoryStatusProjectorRole{},
	)
	_, err := api.ProjectFactoryStatus(context.Background(), "missing")
	if err == nil || !errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("missing session error = %v, want ErrSessionNotFound", err)
	}
}

func TestMergeScopedSessionList_HistorySelectsRecordedProjectionOnly(t *testing.T) {
	t.Parallel()

	live := &recordedListingLiveReader{err: errors.New("live reader must not be called")}
	durable := &recordedListingDurableReader{result: factorysessions.ListSessionsResult{
		LiveSessions: []factorysessions.LiveSessionSummary{{ID: "live-ignored"}},
		DurableSessions: []factorysessions.DurableSessionListSummary{{
			SessionID: "durable-ignored",
		}},
		RecordedSessions: []factorysessions.RecordedSessionListSummary{
			{SessionID: "session-b", Source: factorysessions.RecordedSessionListSourceHistory, ArtifactReference: "2026/08/24/session-b.jsonl", Format: factorysessions.RecordedSessionListFormatV2JSONL},
			{SessionID: "session-a", Source: factorysessions.RecordedSessionListSourceHistory, ArtifactReference: "2026/08/23/session-a.json", Format: factorysessions.RecordedSessionListFormatV1JSON},
		},
	}}

	result, err := mergeScopedSessionList(context.Background(), factorysessions.ListSessionsRequest{
		Scope: factorysessions.SessionListScopeHistory,
	}, live, durable)
	if err != nil {
		t.Fatalf("merge history: %v", err)
	}
	if live.calls != 0 {
		t.Fatalf("live calls = %d, want zero", live.calls)
	}
	if durable.request.Scope != factorysessions.SessionListScopeHistory || durable.request.ExcludeRecordedHistory {
		t.Fatalf("recorded inventory request = %#v, want history without history exclusion", durable.request)
	}
	if len(result.LiveSessions) != 0 || len(result.DurableSessions) != 0 || len(result.RecordedSessions) != 2 {
		t.Fatalf("history result = %#v, want recorded rows only", result)
	}
	if result.RecordedSessions[0].SessionID != "session-a" || result.RecordedSessions[1].SessionID != "session-b" {
		t.Fatalf("recorded ordering = %#v, want canonical session ordering", result.RecordedSessions)
	}
}

func TestMergeScopedSessionList_LiveExcludesRecordedHistoryAtSourceBoundary(t *testing.T) {
	t.Parallel()

	live := &recordedListingLiveReader{rows: []factorysessions.ScopedLiveSessionSummary{{ID: "live-session"}}}
	durable := &recordedListingDurableReader{result: factorysessions.ListSessionsResult{
		RecordedSessions: []factorysessions.RecordedSessionListSummary{{SessionID: "recorded-session"}},
	}}

	result, err := mergeScopedSessionList(context.Background(), factorysessions.ListSessionsRequest{
		Scope: factorysessions.SessionListScopeLive,
	}, live, durable)
	if err != nil {
		t.Fatalf("merge live: %v", err)
	}
	if durable.request.Scope != factorysessions.SessionListScopeAll || !durable.request.ExcludeRecordedHistory {
		t.Fatalf("live inventory request = %#v, want all with recorded history excluded", durable.request)
	}
	if len(result.LiveSessions) != 1 || result.LiveSessions[0].ID != "live-session" || len(result.RecordedSessions) != 0 {
		t.Fatalf("live result = %#v, want live only", result)
	}
}

func TestMergeScopedSessionList_PersistedExcludesRecordedHistoryAtSourceBoundary(t *testing.T) {
	t.Parallel()

	durable := &recordedListingDurableReader{result: factorysessions.ListSessionsResult{
		DurableSessions: []factorysessions.DurableSessionListSummary{{
			SessionID: "durable-session", Status: factorysessions.LifecycleStatusSucceeded,
		}},
		RecordedSessions: []factorysessions.RecordedSessionListSummary{{
			SessionID: "recorded-session",
		}},
	}}

	result, err := mergeScopedSessionList(context.Background(), factorysessions.ListSessionsRequest{
		Scope: factorysessions.SessionListScopePersisted,
	}, nil, durable)
	if err != nil {
		t.Fatalf("merge persisted: %v", err)
	}
	if durable.request.Scope != factorysessions.SessionListScopeAll || !durable.request.ExcludeRecordedHistory {
		t.Fatalf("persisted inventory request = %#v, want all with recorded history excluded", durable.request)
	}
	if len(result.DurableSessions) != 1 || len(result.RecordedSessions) != 0 {
		t.Fatalf("persisted result = %#v, want durable rows without history", result)
	}
}

type recordedListingLiveReader struct {
	rows  []factorysessions.ScopedLiveSessionSummary
	err   error
	calls int
}

func (reader *recordedListingLiveReader) ListScopedLiveSessions(context.Context) ([]factorysessions.ScopedLiveSessionSummary, error) {
	reader.calls++
	return reader.rows, reader.err
}

type recordedListingDurableReader struct {
	result  factorysessions.ListSessionsResult
	request factorysessions.ListSessionsRequest
	calls   int
}

func (reader *recordedListingDurableReader) ListSessions(_ context.Context, request factorysessions.ListSessionsRequest) (factorysessions.ListSessionsResult, error) {
	reader.calls++
	reader.request = request
	return reader.result, nil
}
