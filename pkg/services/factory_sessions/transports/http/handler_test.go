package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

func TestHandlerGetFactorySessionMapsUnavailableDependency(t *testing.T) {
	handler := NewHandler(Dependencies{}, zap.NewNop())
	recorder := httptest.NewRecorder()

	handler.GetFactorySession(recorder, httptest.NewRequest(http.MethodGet, "/factory-sessions/session-alpha", nil), "session-alpha")

	if recorder.Code != http.StatusInternalServerError || !strings.Contains(recorder.Body.String(), "session-scoped API is unavailable") {
		t.Fatalf("response = %d %s, want unavailable dependency error", recorder.Code, recorder.Body.String())
	}
}
