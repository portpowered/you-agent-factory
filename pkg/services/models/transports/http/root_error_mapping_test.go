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
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

func TestRootErrorResponse_MapsRepresentativeTypedFailures(t *testing.T) {
	t.Parallel()

	for _, tt := range rootErrorMappingCases() {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertRootErrorMapping(t, tt)
		})
	}
}

type rootErrorMappingCase struct {
	name       string
	operation  modelsHTTPOperation
	err        error
	wantStatus int
	wantCode   string
	wantFamily factoryapi.ErrorFamily
}

func rootErrorMappingCases() []rootErrorMappingCase {
	return []rootErrorMappingCase{
		{
			name: "catalog not found", operation: modelsHTTPOperationCatalog, err: models.ErrNotFound,
			wantStatus: http.StatusNotFound, wantCode: "NOT_FOUND", wantFamily: factoryapi.ErrorFamilyNotFound,
		},
		{
			name: "catalog unavailable", operation: modelsHTTPOperationCatalog, err: models.ErrUnavailable,
			wantStatus: http.StatusNotFound, wantCode: "MODEL_NOT_AVAILABLE", wantFamily: factoryapi.ErrorFamilyNotFound,
		},
		{
			name: "pull not found", operation: modelsHTTPOperationPull, err: models.ErrNotFound,
			wantStatus: http.StatusNotFound, wantCode: "NOT_FOUND", wantFamily: factoryapi.ErrorFamilyNotFound,
		},
		{
			name: "pull unsupported", operation: modelsHTTPOperationPull, err: models.ErrPullUnsupported,
			wantStatus: http.StatusBadRequest, wantCode: "BAD_REQUEST", wantFamily: factoryapi.ErrorFamilyBadRequest,
		},
		{
			name: "pull not available", operation: modelsHTTPOperationPull, err: models.ErrNotAvailable,
			wantStatus: http.StatusNotFound, wantCode: "MODEL_NOT_AVAILABLE", wantFamily: factoryapi.ErrorFamilyNotFound,
		},
		{
			name: "remove cache not found", operation: modelsHTTPOperationRemove, err: models.ErrModelCacheNotFound,
			wantStatus: http.StatusNotFound, wantCode: "MODEL_CACHE_NOT_FOUND", wantFamily: factoryapi.ErrorFamilyNotFound,
		},
		{
			name: "remove cache in use", operation: modelsHTTPOperationRemove, err: models.ErrModelCacheInUse,
			wantStatus: http.StatusConflict, wantCode: "MODEL_CACHE_IN_USE", wantFamily: factoryapi.ErrorFamilyConflict,
		},
		{
			name: "remove unsafe cache", operation: modelsHTTPOperationRemove, err: models.ErrModelCacheUnsafe,
			wantStatus: http.StatusBadRequest, wantCode: "BAD_REQUEST", wantFamily: factoryapi.ErrorFamilyBadRequest,
		},
		{
			name: "invoke missing runtime", operation: modelsHTTPOperationInvoke, err: models.ErrMissing,
			wantStatus: http.StatusNotFound, wantCode: "MODEL_NOT_AVAILABLE", wantFamily: factoryapi.ErrorFamilyNotFound,
		},
		{
			name: "invoke loading runtime", operation: modelsHTTPOperationInvoke, err: models.ErrLoading,
			wantStatus: http.StatusConflict, wantCode: "MODEL_RUNTIME_LOADING", wantFamily: factoryapi.ErrorFamilyConflict,
		},
		{
			name: "invoke failed runtime", operation: modelsHTTPOperationInvoke, err: models.ErrFailed,
			wantStatus: http.StatusServiceUnavailable, wantCode: "MODEL_RUNTIME_FAILED", wantFamily: factoryapi.ErrorFamilyInternalServerError,
		},
		{
			name: "invoke unsupported runtime", operation: modelsHTTPOperationInvoke, err: models.ErrUnsupported,
			wantStatus: http.StatusBadRequest, wantCode: "MODEL_RUNTIME_UNSUPPORTED", wantFamily: factoryapi.ErrorFamilyBadRequest,
		},
		{
			name: "invoke unsupported operation", operation: modelsHTTPOperationInvoke, err: models.ErrUnsupportedOperation,
			wantStatus: http.StatusBadRequest, wantCode: "BAD_REQUEST", wantFamily: factoryapi.ErrorFamilyBadRequest,
		},
		{
			name: "invoke unsupported response mode", operation: modelsHTTPOperationInvoke, err: models.ErrUnsupportedResponseMode,
			wantStatus: http.StatusBadRequest, wantCode: "BAD_REQUEST", wantFamily: factoryapi.ErrorFamilyBadRequest,
		},
	}
}

func assertRootErrorMapping(t *testing.T, tt rootErrorMappingCase) {
	t.Helper()

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
}

func TestRootErrorResponse_MapsInferenceFailureForInvoke(t *testing.T) {
	t.Parallel()

	failure := &models.InferenceFailure{
		Class:   models.InferenceFailureClassTimeout,
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

// TestRootErrorResponse_MapsEveryInferenceFailureClassForInvoke pins the public
// HTTP outcome of every classified inference failure the invoke operation can
// surface. inference_failure_mapping.go routes five classes to five distinct
// status/code pairs, so a table over the complete class set is what makes a
// later change to where the failure type lives provably behavior preserving.
func TestRootErrorResponse_MapsEveryInferenceFailureClassForInvoke(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name       string
		class      models.InferenceFailureClass
		wantStatus int
		wantCode   string
		wantFamily factoryapi.ErrorFamily
	}{
		{
			name: "missing model", class: models.InferenceFailureClassMissingModel,
			wantStatus: http.StatusNotFound, wantCode: "MODEL_NOT_AVAILABLE",
			wantFamily: factoryapi.ErrorFamilyNotFound,
		},
		{
			name: "loading model", class: models.InferenceFailureClassLoadingModel,
			wantStatus: http.StatusConflict, wantCode: "MODEL_RUNTIME_LOADING",
			wantFamily: factoryapi.ErrorFamilyConflict,
		},
		{
			name: "unsupported operation", class: models.InferenceFailureClassUnsupportedOperation,
			wantStatus: http.StatusBadRequest, wantCode: "BAD_REQUEST",
			wantFamily: factoryapi.ErrorFamilyBadRequest,
		},
		{
			name: "timeout", class: models.InferenceFailureClassTimeout,
			wantStatus: http.StatusGatewayTimeout, wantCode: "MODEL_INFERENCE_TIMEOUT",
			wantFamily: factoryapi.ErrorFamilyInternalServerError,
		},
		{
			name: "runtime failure", class: models.InferenceFailureClassRuntimeFailure,
			wantStatus: http.StatusInternalServerError, wantCode: "MODEL_INFERENCE_RUNTIME_FAILURE",
			wantFamily: factoryapi.ErrorFamilyInternalServerError,
		},
		{
			name: "unrecognized class falls back to runtime failure", class: models.InferenceFailureClass("not_a_known_class"),
			wantStatus: http.StatusInternalServerError, wantCode: "MODEL_INFERENCE_RUNTIME_FAILURE",
			wantFamily: factoryapi.ErrorFamilyInternalServerError,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			failure := &models.InferenceFailure{
				Class:      tt.class,
				Message:    "inference failed for " + string(tt.class),
				ModelName:  "voice",
				WorkerName: "narrator",
				Operation:  "TTS",
			}
			status, response, ok := RootErrorResponse(failure, modelsHTTPOperationInvoke)
			if !ok {
				t.Fatalf("RootErrorResponse(class %q) = not handled, want typed invoke failure", tt.class)
			}
			if status != tt.wantStatus ||
				string(response.Code) != tt.wantCode ||
				response.Family != tt.wantFamily ||
				response.Message != failure.Error() {
				t.Fatalf("RootErrorResponse(class %q) = %d %#v, want %d code=%s family=%s message=%q",
					tt.class, status, response, tt.wantStatus, tt.wantCode, tt.wantFamily, failure.Error())
			}
		})
	}
}

// TestRootErrorResponse_WrappedInferenceFailureStaysClassified pins that the
// invoke mapping unwraps a classified failure carried inside another error,
// which is how the runtime surfaces it through the Worker path.
func TestRootErrorResponse_WrappedInferenceFailureStaysClassified(t *testing.T) {
	t.Parallel()

	failure := &models.InferenceFailure{
		Class:   models.InferenceFailureClassMissingModel,
		Message: "model voice is not installed",
	}
	wrapped := fmt.Errorf("invoke model: %w", failure)

	status, response, ok := RootErrorResponse(wrapped, modelsHTTPOperationInvoke)
	if !ok {
		t.Fatal("RootErrorResponse(wrapped InferenceFailure) = not handled, want typed invoke failure")
	}
	if status != http.StatusNotFound ||
		string(response.Code) != "MODEL_NOT_AVAILABLE" ||
		response.Message != failure.Error() {
		t.Fatalf("RootErrorResponse(wrapped InferenceFailure) = %d %#v, want 404 MODEL_NOT_AVAILABLE with the classified message", status, response)
	}
}

// TestRootErrorResponse_IgnoresInferenceFailureOutsideInvoke pins that the
// classified failure is an invoke-only mapping: the catalog and pull
// operations must not adopt it.
func TestRootErrorResponse_IgnoresInferenceFailureOutsideInvoke(t *testing.T) {
	t.Parallel()

	failure := &models.InferenceFailure{
		Class:   models.InferenceFailureClassTimeout,
		Message: "model inference timed out",
	}
	for _, operation := range []modelsHTTPOperation{modelsHTTPOperationCatalog, modelsHTTPOperationPull} {
		if _, _, ok := RootErrorResponse(failure, operation); ok {
			t.Fatalf("RootErrorResponse(InferenceFailure, %v) = handled, want invoke-only mapping", operation)
		}
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
			handler := NewHandlerFromRoot(testRootBinding(root), zap.NewNop())
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
	handler := NewHandlerFromRoot(testRootBinding(root), zap.NewNop())
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
