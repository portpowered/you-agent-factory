package http_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionshttp "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/http"
	"go.uber.org/zap"
)

type explicitHostedSessionRoot struct {
	*httpSessionsRootFake
	live []factorysessions.SessionView
}

func (root *explicitHostedSessionRoot) Get(context.Context, factorysessions.SessionGetRequest) (factorysessions.SessionGetResult, error) {
	return factorysessions.SessionGetResult{}, factorysessions.ErrSessionNotFound
}

func (root *explicitHostedSessionRoot) List(_ context.Context, request factorysessions.SessionListRequest) (factorysessions.SessionListResult, error) {
	if request.Mode != factorysessions.SessionOperationModeLive {
		return factorysessions.SessionListResult{}, factorysessions.ErrSessionNotFound
	}
	return factorysessions.SessionListResult{Mode: request.Mode, Sessions: root.live}, nil
}

func TestDurableStartUsesSoleExplicitHostedSessionProjectRoot(t *testing.T) {
	t.Parallel()
	root := &explicitHostedSessionRoot{
		httpSessionsRootFake: &httpSessionsRootFake{onStart: func(_ context.Context, request factorysessions.SessionStartRequest) (factorysessions.SessionStartResult, error) {
			if request.FolderPath != "/selected-factory" {
				t.Fatalf("durable project root = %q, want selected live Factory", request.FolderPath)
			}
			return factorysessions.SessionStartResult{Async: &factorysessions.AsyncStartResult{SessionID: "durable-1", Status: "RUNNING"}}, nil
		}},
		live: []factorysessions.SessionView{{SessionID: "explicit-1", FactoryDir: "/selected-factory"}},
	}
	handler := factorysessionshttp.NewHandlerFromRoot(factorysessionshttp.RootBinding{Sessions: root}, zap.NewNop())
	recorder := httptest.NewRecorder()
	handler.StartDurableFactorySessionAsync(recorder, httptest.NewRequest(http.MethodPost, "/factory-sessions/async", bytes.NewBufferString(`{"requestId":"durable-1","source":{"kind":"FACTORY_ID","factoryId":"factory-alpha"}}`)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
}

func TestDurableStartRejectsAmbiguousExplicitHostedSessions(t *testing.T) {
	t.Parallel()
	root := &explicitHostedSessionRoot{
		httpSessionsRootFake: &httpSessionsRootFake{onStart: func(context.Context, factorysessions.SessionStartRequest) (factorysessions.SessionStartResult, error) {
			t.Fatal("ambiguous project root must not start durable execution")
			return factorysessions.SessionStartResult{}, nil
		}},
		live: []factorysessions.SessionView{{SessionID: "explicit-1", FactoryDir: "/first"}, {SessionID: "explicit-2", FactoryDir: "/second"}},
	}
	handler := factorysessionshttp.NewHandlerFromRoot(factorysessionshttp.RootBinding{Sessions: root}, zap.NewNop())
	recorder := httptest.NewRecorder()
	handler.StartDurableFactorySessionAsync(recorder, httptest.NewRequest(http.MethodPost, "/factory-sessions/async", bytes.NewBufferString(`{"requestId":"durable-1","source":{"kind":"FACTORY_ID","factoryId":"factory-alpha"}}`)))
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "current Factory Session is ambiguous") {
		t.Fatalf("status = %d, body = %s, want actionable ambiguous selection", recorder.Code, recorder.Body.String())
	}
}
