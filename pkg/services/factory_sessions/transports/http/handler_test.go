package http

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"go.uber.org/zap"
)

type liveSessionAPIFake struct {
	apisurface.LiveSessionAPI
	get func(context.Context, string) (factoryapi.FactorySession, error)
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

type factoryStatusRuntimeRole struct {
	factoryruntime.Service
	observation  factoryruntime.Observation
	observations int
}

type factoryStatusProjectorRole struct {
	status      factoryruntime.FactoryStatus
	projections int
}

type factoryStatusSessionRole struct {
	observations int
	sessionIDs   []string
}

func (role *factoryStatusRuntimeRole) Observe(_ context.Context, _ factoryruntime.ObserveRequest) (factoryruntime.ObserveResult, error) {
	role.observations++
	return factoryruntime.ObserveResult{Observation: role.observation}, nil
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
	scopedObservation := factoryruntime.Observation{
		Status: factoryruntime.ObservationStatusActive,
		Progress: factoryruntime.ObservationProgress{
			TotalWorkCount: 99,
			WorkCategories: factoryruntime.ObservationWorkCategories{Processing: 7, Terminal: 92},
		},
		Health: factoryruntime.ObservationHealth{FactoryState: "OBSERVATION_SOURCE"},
	}
	runtime := &factoryStatusRuntimeRole{observation: scopedObservation}
	sessions := &factoryStatusSessionRole{}
	projector := &factoryStatusProjectorRole{status: factoryruntime.FactoryStatus{
		FactoryState:  "CURRENT",
		RuntimeStatus: "ACTIVE",
		TotalTokens:   2,
		Categories:    factoryruntime.FactoryStatusCategories{Processing: 1},
	}}
	api := NewFactoryStatusAPI(sessions, projector)

	assertFactoryStatusRequest(t, api, "", runtime, sessions, projector, 1)
	assertFactoryStatusRequest(t, api, "session-beta", runtime, sessions, projector, 2)
	if len(sessions.sessionIDs) != 2 || sessions.sessionIDs[0] != factorysessions.DefaultSessionID || sessions.sessionIDs[1] != "session-beta" {
		t.Fatalf("session IDs = %#v, want [~default session-beta]", sessions.sessionIDs)
	}
}

func assertFactoryStatusRequest(
	t *testing.T,
	api apisurface.FactoryStatusAPI,
	sessionID string,
	runtime *factoryStatusRuntimeRole,
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
	if runtime.observations != 0 {
		t.Fatalf("bound runtime observations = %d, want 0", runtime.observations)
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
