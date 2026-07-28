package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionshttp "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/http"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

func TestHandlerFromRoot_ListFactorySessionsDecodesScopeBeforeRootInvocation(t *testing.T) {
	t.Parallel()

	persistedScope := factoryapi.FactorySessionListScopePersisted
	root := &httpSessionsRootFake{
		listSessions: func(_ context.Context, request factorysessions.ListSessionsRequest) (factorysessions.ListSessionsResult, error) {
			if request.Scope != factorysessions.SessionListScopeAll {
				t.Fatalf("durable inventory scope = %q, want all", request.Scope)
			}
			return factorysessions.ListSessionsResult{
				Scope: factorysessions.SessionListScopePersisted,
				DurableSessions: []factorysessions.DurableSessionListSummary{{
					SessionID: "dur-sess-alpha", Status: factorysessions.LifecycleStatusSucceeded,
				}},
			}, nil
		},
	}
	handler := factorysessionshttp.NewHandlerFromRoot(factorysessionshttp.RootBinding{Sessions: root}, zap.NewNop())
	recorder := httptest.NewRecorder()

	handler.ListFactorySessions(recorder, httptest.NewRequest(http.MethodGet, "/factory-sessions?scope=persisted", nil), factoryapi.ListFactorySessionsParams{Scope: &persistedScope})

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	var response factoryapi.ListFactorySessionsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Scope == nil || *response.Scope != factoryapi.FactorySessionListScopePersisted {
		t.Fatalf("scope = %#v, want persisted", response.Scope)
	}
	if len(response.Sessions) != 0 {
		t.Fatalf("live sessions = %#v, want none for persisted scope", response.Sessions)
	}
	if response.DurableSessions == nil || len(*response.DurableSessions) != 1 || (*response.DurableSessions)[0].SessionId != "dur-sess-alpha" {
		t.Fatalf("durable sessions = %#v, want one persisted row", response.DurableSessions)
	}
}

func TestHandlerFromRoot_ListFactorySessionsInvalidScopeReturnsBadRequestWithoutRootCall(t *testing.T) {
	t.Parallel()

	unsupportedScope := factoryapi.FactorySessionListScope("workspace")
	root := &httpSessionsRootFake{
		listSessions: func(context.Context, factorysessions.ListSessionsRequest) (factorysessions.ListSessionsResult, error) {
			t.Fatal("fake root must not be invoked for invalid list scope")
			return factorysessions.ListSessionsResult{}, nil
		},
		listReads: func(context.Context) ([]factorysessions.ReadProjection, error) {
			t.Fatal("fake root must not be invoked for invalid list scope")
			return nil, nil
		},
	}
	handler := factorysessionshttp.NewHandlerFromRoot(factorysessionshttp.RootBinding{Sessions: root}, zap.NewNop())
	recorder := httptest.NewRecorder()

	handler.ListFactorySessions(recorder, httptest.NewRequest(http.MethodGet, "/factory-sessions?scope=workspace", nil), factoryapi.ListFactorySessionsParams{Scope: &unsupportedScope})

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
	var errResp factoryapi.ErrorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if errResp.Code != factoryapi.ErrorResponseCodeBADREQUEST || errResp.Family != factoryapi.ErrorFamilyBadRequest {
		t.Fatalf("error = %#v, want BAD_REQUEST family/code", errResp)
	}
	if !strings.Contains(errResp.Message, "scope must be live, persisted, or all") {
		t.Fatalf("message = %q, want scope validation text", errResp.Message)
	}
}

func TestHandlerFromRoot_GetFactorySessionEncodesRootProjectionToAPI(t *testing.T) {
	t.Parallel()

	root := &httpSessionsRootFake{
		sessions: map[string]factorysessions.SessionProjection{
			"session-alpha": {
				Context: factorysessions.ProjectionContext{
					FactorySessionID: "session-alpha",
					Session: &factorysessions.ScopedLiveSessionSummary{
						ID: "session-alpha", FactoryDir: "/workspace/alpha", FolderPath: "/workspace",
						Project: "alpha", IsDefault: true,
						Target: factorysessions.TargetRef{Kind: factorysessions.TargetKindNamed, Name: "alpha"},
					},
				},
			},
		},
	}
	handler := factorysessionshttp.NewHandlerFromRoot(factorysessionshttp.RootBinding{Sessions: root}, zap.NewNop())
	recorder := httptest.NewRecorder()

	handler.GetFactorySession(recorder, httptest.NewRequest(http.MethodGet, "/factory-sessions/session-alpha", nil), "session-alpha")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	var response factoryapi.FactorySession
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Id != "session-alpha" || response.Project != "alpha" || response.FactoryDir != "/workspace/alpha" {
		t.Fatalf("response = %#v, want encoded live session fields", response)
	}
	if !response.IsDefault {
		t.Fatalf("isDefault = %v, want true", response.IsDefault)
	}
}

func TestHandlerFromRoot_GetFactorySessionNotFoundReturnsTypedErrorResponse(t *testing.T) {
	t.Parallel()

	root := &httpSessionsRootFake{}
	handler := factorysessionshttp.NewHandlerFromRoot(factorysessionshttp.RootBinding{Sessions: root}, zap.NewNop())
	recorder := httptest.NewRecorder()

	handler.GetFactorySession(recorder, httptest.NewRequest(http.MethodGet, "/factory-sessions/missing-session", nil), "missing-session")

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", recorder.Code)
	}
	var errResp factoryapi.ErrorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if errResp.Code != factoryapi.ErrorResponseCodeNOTFOUND || errResp.Family != factoryapi.ErrorFamilyNotFound {
		t.Fatalf("error = %#v, want NOT_FOUND family/code", errResp)
	}
	if errResp.Message != "factory session not found" {
		t.Fatalf("message = %q, want stable not-found text", errResp.Message)
	}
}
