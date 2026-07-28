package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

func TestRootErrorResponse_MapsRepresentativeTypedFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		operation  modelsHTTPOperation
		err        error
		wantStatus int
		wantCode   string
		wantFamily factoryapi.ErrorFamily
	}{
		{
			name:       "catalog not found",
			operation:  modelsHTTPOperationCatalog,
			err:        models.ErrNotFound,
			wantStatus: http.StatusNotFound,
			wantCode:   "NOT_FOUND",
			wantFamily: factoryapi.ErrorFamilyNotFound,
		},
		{
			name:       "catalog unavailable",
			operation:  modelsHTTPOperationCatalog,
			err:        models.ErrUnavailable,
			wantStatus: http.StatusNotFound,
			wantCode:   "MODEL_NOT_AVAILABLE",
			wantFamily: factoryapi.ErrorFamilyNotFound,
		},
		{
			name:       "pull not found",
			operation:  modelsHTTPOperationPull,
			err:        models.ErrNotFound,
			wantStatus: http.StatusNotFound,
			wantCode:   "NOT_FOUND",
			wantFamily: factoryapi.ErrorFamilyNotFound,
		},
		{
			name:       "pull unsupported",
			operation:  modelsHTTPOperationPull,
			err:        models.ErrPullUnsupported,
			wantStatus: http.StatusBadRequest,
			wantCode:   "BAD_REQUEST",
			wantFamily: factoryapi.ErrorFamilyBadRequest,
		},
		{
			name:       "pull not available",
			operation:  modelsHTTPOperationPull,
			err:        models.ErrNotAvailable,
			wantStatus: http.StatusNotFound,
			wantCode:   "MODEL_NOT_AVAILABLE",
			wantFamily: factoryapi.ErrorFamilyNotFound,
		},
		{
			name:       "invoke missing runtime",
			operation:  modelsHTTPOperationInvoke,
			err:        models.ErrMissing,
			wantStatus: http.StatusNotFound,
			wantCode:   "MODEL_NOT_AVAILABLE",
			wantFamily: factoryapi.ErrorFamilyNotFound,
		},
		{
			name:       "invoke loading runtime",
			operation:  modelsHTTPOperationInvoke,
			err:        models.ErrLoading,
			wantStatus: http.StatusConflict,
			wantCode:   "MODEL_RUNTIME_LOADING",
			wantFamily: factoryapi.ErrorFamilyConflict,
		},
		{
			name:       "invoke failed runtime",
			operation:  modelsHTTPOperationInvoke,
			err:        models.ErrFailed,
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   "MODEL_RUNTIME_FAILED",
			wantFamily: factoryapi.ErrorFamilyInternalServerError,
		},
		{
			name:       "invoke unsupported runtime",
			operation:  modelsHTTPOperationInvoke,
			err:        models.ErrUnsupported,
			wantStatus: http.StatusBadRequest,
			wantCode:   "MODEL_RUNTIME_UNSUPPORTED",
			wantFamily: factoryapi.ErrorFamilyBadRequest,
		},
		{
			name:       "invoke unsupported operation",
			operation:  modelsHTTPOperationInvoke,
			err:        models.ErrUnsupportedOperation,
			wantStatus: http.StatusBadRequest,
			wantCode:   "BAD_REQUEST",
			wantFamily: factoryapi.ErrorFamilyBadRequest,
		},
		{
			name:       "invoke unsupported response mode",
			operation:  modelsHTTPOperationInvoke,
			err:        models.ErrUnsupportedResponseMode,
			wantStatus: http.StatusBadRequest,
			wantCode:   "BAD_REQUEST",
			wantFamily: factoryapi.ErrorFamilyBadRequest,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			status, response, ok := RootErrorResponse(tt.err, tt.operation)
			if !ok {
				t.Fatalf("RootErrorResponse(%v, %v) = not handled, want typed outcome", tt.err, tt.operation)
			}
			if status != tt.wantStatus ||
				response.Family != tt.wantFamily ||
				string(response.Code) != tt.wantCode {
				t.Fatalf("RootErrorResponse(%v, %v) = %d %#v, want %d family=%s code=%s",
					tt.err, tt.operation, status, response, tt.wantStatus, tt.wantFamily, tt.wantCode)
			}
			if strings.TrimSpace(response.Message) == "" {
				t.Fatalf("RootErrorResponse(%v, %v) message = %q, want non-empty public message", tt.err, tt.operation, response.Message)
			}
		})
	}
}

func TestRootErrorResponse_MapsInferenceFailureForInvoke(t *testing.T) {
	t.Parallel()

	failure := &workers.InferenceFailure{
		Class:   workers.InferenceFailureClassTimeout,
		Message: "model inference timed out",
	}
	status, response, ok := RootErrorResponse(failure, modelsHTTPOperationInvoke)
	if !ok {
		t.Fatal("RootErrorResponse(InferenceFailure) = not handled, want typed invoke failure")
	}
	if status != http.StatusGatewayTimeout ||
		response.Family != factoryapi.ErrorFamilyInternalServerError ||
		response.Code != factoryapi.ErrorResponseCode("MODEL_INFERENCE_TIMEOUT") ||
		response.Message != failure.Error() {
		t.Fatalf("RootErrorResponse(InferenceFailure) = %d %#v, want timeout inference failure", status, response)
	}
}

func TestRootErrorResponse_IgnoresCrossOperationTypedFailures(t *testing.T) {
	t.Parallel()

	if _, _, ok := RootErrorResponse(models.ErrUnavailable, modelsHTTPOperationPull); ok {
		t.Fatal("RootErrorResponse(ErrUnavailable, pull) = handled, want catalog-only mapping")
	}
	if _, _, ok := RootErrorResponse(models.ErrPullUnsupported, modelsHTTPOperationInvoke); ok {
		t.Fatal("RootErrorResponse(ErrPullUnsupported, invoke) = handled, want pull-only mapping")
	}
}

func TestRootErrorResponse_LeavesUnmappedInternalFailures(t *testing.T) {
	t.Parallel()

	internalErr := errors.New("pkg/services/models/internal/cache: /tmp/models unavailable")
	for _, operation := range []modelsHTTPOperation{
		modelsHTTPOperationCatalog,
		modelsHTTPOperationPull,
		modelsHTTPOperationInvoke,
	} {
		if _, _, ok := RootErrorResponse(internalErr, operation); ok {
			t.Fatalf("RootErrorResponse(%v, %v) = handled, want unmapped internal failure", internalErr, operation)
		}
	}
}

func TestHandler_PullModelMapsTypedRootFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		rootErr     error
		wantStatus  int
		wantCode    string
		wantMessage string
	}{
		{
			name:        "not found",
			rootErr:     models.ErrNotFound,
			wantStatus:  http.StatusNotFound,
			wantCode:    "NOT_FOUND",
			wantMessage: catalogNotFoundMessage,
		},
		{
			name:        "pull unsupported",
			rootErr:     fmt.Errorf("%w: model is cloud-only", models.ErrPullUnsupported),
			wantStatus:  http.StatusBadRequest,
			wantCode:    "BAD_REQUEST",
			wantMessage: "model pull is not supported: model is cloud-only",
		},
		{
			name:        "not available",
			rootErr:     models.ErrNotAvailable,
			wantStatus:  http.StatusNotFound,
			wantCode:    "MODEL_NOT_AVAILABLE",
			wantMessage: models.ErrNotAvailable.Error(),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			root := &rootFake{
				pull: func(context.Context, string) (models.PullResult, error) {
					return models.PullResult{}, tt.rootErr
				},
			}
			handler := NewHandlerFromRoot(RootBinding{Models: root}, zap.NewNop())
			recorder := httptest.NewRecorder()

			handler.PullModel(recorder, httptest.NewRequest(http.MethodPost, "/models/voice/pull", nil), "voice")

			assertCatalogHTTPError(t, recorder, tt.wantStatus, tt.wantCode, tt.wantMessage)
		})
	}
}

func TestHandler_PullModelUnmappedFailureDoesNotLeakInternalPaths(t *testing.T) {
	t.Parallel()

	root := &rootFake{
		pull: func(context.Context, string) (models.PullResult, error) {
			return models.PullResult{}, errors.New("pkg/services/models/internal/cache: /tmp/models unavailable")
		},
	}
	handler := NewHandlerFromRoot(RootBinding{Models: root}, zap.NewNop())
	recorder := httptest.NewRecorder()

	handler.PullModel(recorder, httptest.NewRequest(http.MethodPost, "/models/voice/pull", nil), "voice")

	assertCatalogHTTPError(t, recorder, http.StatusInternalServerError, "INTERNAL_ERROR", pullFailedMessage)
}

func TestHandler_InvokeModelMapsTypedRootFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		rootErr     error
		wantStatus  int
		wantCode    string
		wantMessage string
	}{
		{
			name:        "loading runtime",
			rootErr:     models.ErrLoading,
			wantStatus:  http.StatusConflict,
			wantCode:    "MODEL_RUNTIME_LOADING",
			wantMessage: models.ErrLoading.Error(),
		},
		{
			name:        "failed runtime",
			rootErr:     models.ErrFailed,
			wantStatus:  http.StatusServiceUnavailable,
			wantCode:    "MODEL_RUNTIME_FAILED",
			wantMessage: models.ErrFailed.Error(),
		},
		{
			name:        "unsupported runtime",
			rootErr:     models.ErrUnsupported,
			wantStatus:  http.StatusBadRequest,
			wantCode:    "MODEL_RUNTIME_UNSUPPORTED",
			wantMessage: models.ErrUnsupported.Error(),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			invoker := invokeInvokerFake{
				invoke: func(context.Context, string, models.Request) (models.Result, error) {
					return models.Result{}, tt.rootErr
				},
			}
			handler := NewHandlerFromRoot(RootBinding{
				Models:  &rootFake{},
				Invoker: invoker,
			}, zap.NewNop())
			recorder := httptest.NewRecorder()
			body := `{"operation":"TTS","content":[{"type":"TEXT","text":"hello"}]}`

			handler.InvokeModel(
				recorder,
				httptest.NewRequest(http.MethodPost, "/models/voice/invocations", strings.NewReader(body)),
				"voice",
			)

			assertCatalogHTTPError(t, recorder, tt.wantStatus, tt.wantCode, tt.wantMessage)
		})
	}
}

func TestHandler_InvokeModelUnmappedFailureDoesNotLeakInternalPaths(t *testing.T) {
	t.Parallel()

	invoker := invokeInvokerFake{
		invoke: func(context.Context, string, models.Request) (models.Result, error) {
			return models.Result{}, errors.New("pkg/services/models/internal/runtime: stack trace at invoke.go:42")
		},
	}
	handler := NewHandlerFromRoot(RootBinding{
		Models:  &rootFake{},
		Invoker: invoker,
	}, zap.NewNop())
	recorder := httptest.NewRecorder()
	body := `{"operation":"TTS","content":[{"type":"TEXT","text":"hello"}]}`

	handler.InvokeModel(
		recorder,
		httptest.NewRequest(http.MethodPost, "/models/voice/invocations", strings.NewReader(body)),
		"voice",
	)

	assertCatalogHTTPError(t, recorder, http.StatusInternalServerError, "INTERNAL_ERROR", invokeFailedMessage)
}
