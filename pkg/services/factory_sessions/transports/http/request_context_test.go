package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionshttp "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/http"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

func TestHandlerFromRoot_GetFactorySessionCanceledDuringRootCallCompletesWithoutHang(t *testing.T) {
	t.Parallel()

	entered := make(chan struct{})
	root := &httpSessionsRootFake{
		getSession: func(ctx context.Context, _ string) (factorysessions.SessionProjection, error) {
			close(entered)
			<-ctx.Done()
			return factorysessions.SessionProjection{}, ctx.Err()
		},
	}
	handler := factorysessionshttp.NewReadHandler(root, nil, factorysessionshttp.ReadProjectionSessionListReader{Reader: root}, testRequestPreparation{}, zap.NewNop())

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	req := httptest.NewRequest(http.MethodGet, "/factory-sessions/session-alpha", nil).WithContext(ctx)
	recorder := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		handler.GetFactorySession(recorder, req, "session-alpha")
		close(done)
	}()

	select {
	case <-entered:
	case <-time.After(time.Minute):
		t.Fatal("GetFactorySession did not enter the selected root")
	}
	cancel()

	select {
	case <-done:
	case <-time.After(time.Minute):
		t.Fatal("GetFactorySession hung after request cancellation")
	}
	if body := recorder.Body.String(); body != "" {
		t.Fatalf("response body = %q, want empty cancel-oriented outcome", body)
	}
}

func TestHandlerFromRoot_GetFactorySessionCanceledBeforeRootCallCompletesWithoutBody(t *testing.T) {
	t.Parallel()

	root := &httpSessionsRootFake{}
	handler := factorysessionshttp.NewReadHandler(root, nil, factorysessionshttp.ReadProjectionSessionListReader{Reader: root}, testRequestPreparation{}, zap.NewNop())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	recorder := httptest.NewRecorder()
	handler.GetFactorySession(recorder, httptest.NewRequest(http.MethodGet, "/factory-sessions/session-alpha", nil).WithContext(ctx), "session-alpha")

	if body := recorder.Body.String(); body != "" {
		t.Fatalf("response body = %q, want empty cancel-oriented outcome", body)
	}
}

func TestHandlerFromRoot_ListFactorySessionsDeadlineExceededReturnsGatewayTimeout(t *testing.T) {
	t.Parallel()

	ctx := context.WithValue(t.Context(), contextTestKey{}, "selected-list")
	root := &httpSessionsRootFake{
		listSessions: func(received context.Context, request factorysessions.ListSessionsRequest) (factorysessions.ListSessionsResult, error) {
			if received != ctx {
				t.Fatal("list dropped the selected request context")
			}
			return factorysessions.ListSessionsResult{}, context.DeadlineExceeded
		},
	}
	handler := factorysessionshttp.NewReadHandler(root, nil, factorysessionshttp.ReadProjectionSessionListReader{Reader: root}, testRequestPreparation{}, zap.NewNop())

	recorder := httptest.NewRecorder()
	handler.ListFactorySessions(recorder, httptest.NewRequest(http.MethodGet, "/factory-sessions", nil).WithContext(ctx), factoryapi.ListFactorySessionsParams{})
	if recorder.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want 504 Gateway Timeout", recorder.Code)
	}
	assertErrorResponse(t, recorder.Body.Bytes(), factoryapi.ErrorFamilyInternalServerError, factoryapi.ErrorResponseCodeINTERNALERROR, "factory session request timed out")
}

func TestHandlerFromRoot_CloseFactorySessionDeadlineExceededReturnsGatewayTimeout(t *testing.T) {
	t.Parallel()

	ctx := context.WithValue(t.Context(), contextTestKey{}, "selected-close")
	root := &httpSessionsRootFake{
		onDelete: func(received context.Context, _ string) error {
			if received != ctx {
				t.Fatal("close dropped the selected request context")
			}
			return context.DeadlineExceeded
		},
	}
	handler := factorysessionshttp.NewLifecycleHandler(root, root, root, testRequestPreparation{}, zap.NewNop())

	recorder := httptest.NewRecorder()
	handler.CloseFactorySession(recorder, httptest.NewRequest(http.MethodDelete, "/factory-sessions/session-alpha", nil).WithContext(ctx), "session-alpha")
	if recorder.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want 504 Gateway Timeout", recorder.Code)
	}
	assertErrorResponse(t, recorder.Body.Bytes(), factoryapi.ErrorFamilyInternalServerError, factoryapi.ErrorResponseCodeINTERNALERROR, "factory session request timed out")
}

type contextTestKey struct{}

func TestSessionsRequestContextErrorResponseForTest(t *testing.T) {
	t.Parallel()

	if status, response, ok := factorysessionshttp.SessionsRequestContextErrorResponseForTest(context.Canceled); !ok || status != 0 || response != nil {
		t.Fatalf("canceled = (%d, %#v, %v), want (0, nil, true)", status, response, ok)
	}

	status, response, ok := factorysessionshttp.SessionsRequestContextErrorResponseForTest(context.DeadlineExceeded)
	if !ok || status != http.StatusGatewayTimeout {
		t.Fatalf("deadline status = %d, ok = %v, want 504 true", status, ok)
	}
	body, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	if !strings.Contains(string(body), "factory session request timed out") {
		t.Fatalf("response = %s, want timeout message", body)
	}
}
