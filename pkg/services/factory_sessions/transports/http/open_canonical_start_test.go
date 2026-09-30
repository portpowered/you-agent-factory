package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionshttp "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/http"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

var errOpenCanonicalBoom = errors.New("open rejected")

func TestOpenFactorySession_UsesCanonicalStartAndMapsLiveResult(t *testing.T) {
	t.Parallel()

	var got factorysessions.SessionStartRequest
	root := &httpSessionsRootFake{
		onStart: func(_ context.Context, request factorysessions.SessionStartRequest) (factorysessions.SessionStartResult, error) {
			got = request
			return factorysessions.SessionStartResult{
				SessionID: "session-live", Mode: factorysessions.SessionOperationModeLive,
				Live: &factorysessions.SessionOpenResult{
					SessionID: "session-live",
					Session: &factorysessions.SessionView{
						SessionID: "session-live", FactoryDir: "/workspace/factory",
						FolderPath: "/workspace", Project: "demo",
						Target: factorysessions.TargetRef{Kind: factorysessions.TargetKindNamed, Name: "beta"},
					},
					Targets:               []factorysessions.Target{{Ref: factorysessions.TargetRef{Kind: factorysessions.TargetKindNamed, Name: "beta"}}},
					InitializedNewFactory: true,
					FolderPath:            " /workspace ",
				},
			}, nil
		},
	}
	handler := factorysessionshttp.NewHandlerFromRoot(factorysessionshttp.RootBinding{Sessions: root}, zap.NewNop())

	body := `{"folderPath":"/workspace","target":{"kind":"named","name":"beta"},"validateOnly":true,"initNewFactory":true}`
	request := httptest.NewRequest(http.MethodPost, "/factory-sessions", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.OpenFactorySession(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 body=%s", recorder.Code, recorder.Body.String())
	}
	if got.Mode != factorysessions.SessionOperationModeLive || got.FolderPath != "/workspace" || !got.ValidateOnly || !got.InitNewFactory {
		t.Fatalf("start request = %#v, want live mapped open fields", got)
	}
	if got.Target == nil || got.Target.Kind != factorysessions.TargetKindNamed || got.Target.Name != "beta" {
		t.Fatalf("start target = %#v, want named beta", got.Target)
	}
	var response factoryapi.OpenFactorySessionResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	assertCanonicalLiveOpenResponse(t, response)
}

func assertCanonicalLiveOpenResponse(t *testing.T, response factoryapi.OpenFactorySessionResponse) {
	t.Helper()
	if response.Session == nil || response.Session.Id != "session-live" || response.Session.Project != "demo" {
		t.Fatalf("session = %#v, want mapped session-live", response.Session)
	}
	if response.InitsNewFactory == nil || !*response.InitsNewFactory {
		t.Fatalf("initsNewFactory = %#v, want true", response.InitsNewFactory)
	}
	if response.Targets == nil || len(*response.Targets) != 1 {
		t.Fatalf("targets = %#v, want 1", response.Targets)
	}
}

func TestOpenFactorySession_CanonicalStartErrorMapsToBadRequest(t *testing.T) {
	t.Parallel()

	root := &httpSessionsRootFake{
		onStart: func(context.Context, factorysessions.SessionStartRequest) (factorysessions.SessionStartResult, error) {
			return factorysessions.SessionStartResult{}, errOpenCanonicalBoom
		},
	}
	handler := factorysessionshttp.NewHandlerFromRoot(factorysessionshttp.RootBinding{Sessions: root}, zap.NewNop())
	body := `{"folderPath":"/workspace"}`
	request := httptest.NewRequest(http.MethodPost, "/factory-sessions", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.OpenFactorySession(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestOpenFactorySession_RequiresCanonicalSessionsRoot(t *testing.T) {
	t.Parallel()
	handler := factorysessionshttp.NewHandler(factorysessionshttp.Dependencies{}, zap.NewNop())
	request := httptest.NewRequest(http.MethodPost, "/factory-sessions", strings.NewReader(`{"folderPath":"/workspace"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.OpenFactorySession(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestOpenFactorySession_SessionNotFoundReturnsCanonicalFailure(t *testing.T) {
	t.Parallel()

	root := &httpSessionsRootFake{
		onStart: func(context.Context, factorysessions.SessionStartRequest) (factorysessions.SessionStartResult, error) {
			return factorysessions.SessionStartResult{}, factorysessions.ErrSessionNotFound
		},
	}
	handler := factorysessionshttp.NewHandlerFromRoot(factorysessionshttp.RootBinding{Sessions: root}, zap.NewNop())
	body := `{"folderPath":"/workspace"}`
	request := httptest.NewRequest(http.MethodPost, "/factory-sessions", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.OpenFactorySession(recorder, request)

	if recorder.Code == http.StatusOK {
		t.Fatalf("status = 200, want canonical failure without second open body=%s", recorder.Body.String())
	}
}
