package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

func TestAdapter_ListModelsInvokesFakeRootAndEncodesSuccess(t *testing.T) {
	t.Parallel()

	var invoked bool
	root := &rootFake{
		list: func(context.Context) (models.List, error) {
			invoked = true
			return models.List{
				Results: []models.Summary{{
					Name:   "voice",
					Status: models.StatusReady,
				}},
			}, nil
		},
	}
	handler := NewHandlerFromRoot(RootBinding{Models: root}, zap.NewNop())
	recorder := httptest.NewRecorder()

	handler.ListModels(recorder, httptest.NewRequest(http.MethodGet, "/models", nil))

	if !invoked {
		t.Fatal("ListModels did not invoke the injected Models root")
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	var response factoryapi.ListModelsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Results) != 1 || response.Results[0].Name != "voice" {
		t.Fatalf("response = %#v, want encoded voice model summary", response)
	}
}

func TestAdapter_GetModelInvokesFakeRootWithDecodedName(t *testing.T) {
	t.Parallel()

	var invokedName string
	root := &rootFake{
		get: func(_ context.Context, name string) (models.Detail, error) {
			invokedName = name
			return models.Detail{
				Summary: models.Summary{Name: name, Status: models.StatusReady},
			}, nil
		},
	}
	handler := NewHandlerFromRoot(RootBinding{Models: root}, zap.NewNop())
	recorder := httptest.NewRecorder()

	handler.GetModel(recorder, httptest.NewRequest(http.MethodGet, "/models/voice", nil), "voice")

	if invokedName != "voice" {
		t.Fatalf("GetModel invoked root with name = %q, want voice", invokedName)
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	var response factoryapi.ModelDetail
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Name != "voice" {
		t.Fatalf("response = %#v, want encoded voice detail", response)
	}
}

func TestAdapter_GetModelRejectsEmptyNameBeforeFakeRoot(t *testing.T) {
	t.Parallel()

	root := &rootFake{
		get: func(context.Context, string) (models.Detail, error) {
			t.Fatal("fake root must not be invoked for empty model name")
			return models.Detail{}, nil
		},
	}
	handler := NewHandlerFromRoot(RootBinding{Models: root}, zap.NewNop())
	recorder := httptest.NewRecorder()

	handler.GetModel(recorder, httptest.NewRequest(http.MethodGet, "/models/", nil), "   ")

	assertCatalogHTTPError(t, recorder, http.StatusNotFound, "NOT_FOUND", "model not found")
}

func TestAdapter_GetModelMapsNotFoundFromFakeRoot(t *testing.T) {
	t.Parallel()

	root := &rootFake{
		get: func(context.Context, string) (models.Detail, error) {
			return models.Detail{}, models.ErrNotFound
		},
	}
	handler := NewHandlerFromRoot(RootBinding{Models: root}, zap.NewNop())
	recorder := httptest.NewRecorder()

	handler.GetModel(recorder, httptest.NewRequest(http.MethodGet, "/models/missing", nil), "missing")

	assertCatalogHTTPError(t, recorder, http.StatusNotFound, "NOT_FOUND", "model not found")
}

func TestAdapter_ListModelsMapsUnavailableCatalogFromFakeRoot(t *testing.T) {
	t.Parallel()

	root := &rootFake{
		list: func(context.Context) (models.List, error) {
			return models.List{}, models.ErrUnavailable
		},
	}
	handler := NewHandlerFromRoot(RootBinding{Models: root}, zap.NewNop())
	recorder := httptest.NewRecorder()

	handler.ListModels(recorder, httptest.NewRequest(http.MethodGet, "/models", nil))

	assertCatalogHTTPError(
		t,
		recorder,
		http.StatusNotFound,
		"MODEL_NOT_AVAILABLE",
		models.ErrUnavailable.Error(),
	)
}

func TestAdapter_GetModelMapsUnavailableCatalogFromFakeRoot(t *testing.T) {
	t.Parallel()

	root := &rootFake{
		get: func(context.Context, string) (models.Detail, error) {
			return models.Detail{}, models.ErrUnavailable
		},
	}
	handler := NewHandlerFromRoot(RootBinding{Models: root}, zap.NewNop())
	recorder := httptest.NewRecorder()

	handler.GetModel(recorder, httptest.NewRequest(http.MethodGet, "/models/voice", nil), "voice")

	assertCatalogHTTPError(
		t,
		recorder,
		http.StatusNotFound,
		"MODEL_NOT_AVAILABLE",
		models.ErrUnavailable.Error(),
	)
}

func TestAdapter_ScopedCatalogListGetDecodeIntoRootRequests(t *testing.T) {
	t.Parallel()

	scope, err := (models.RuntimeScopeRef{}).Parse("factory-session:http-scope")
	if err != nil {
		t.Fatalf("parse Models scope: %v", err)
	}
	var listRequest models.ListModelsRequest
	var getRequest models.GetModelRequest
	root := &rootFake{
		listCatalog: func(_ context.Context, request models.ListModelsRequest) (models.ListModelsResult, error) {
			listRequest = request
			return models.ListModelsResult{
				Models: []models.Summary{{Name: "voice"}},
			}, nil
		},
		getCatalog: func(_ context.Context, request models.GetModelRequest) (models.GetModelResult, error) {
			getRequest = request
			return models.GetModelResult{
				Model: models.Detail{Summary: models.Summary{Name: request.Name}},
			}, nil
		},
	}
	handler := NewHandlerFromRoot(RootBinding{Models: root, Scope: scope}, zap.NewNop())

	listRecorder := httptest.NewRecorder()
	handler.ListModels(listRecorder, httptest.NewRequest(http.MethodGet, "/models", nil))
	if listRecorder.Code != http.StatusOK || !strings.Contains(listRecorder.Body.String(), `"name":"voice"`) {
		t.Fatalf("list response = %d %s, want scoped voice list", listRecorder.Code, listRecorder.Body.String())
	}
	if listRequest.Scope != scope {
		t.Fatalf("ListCatalog request scope = %q, want %q", listRequest.Scope, scope)
	}

	getRecorder := httptest.NewRecorder()
	handler.GetModel(getRecorder, httptest.NewRequest(http.MethodGet, "/models/voice", nil), "voice")
	if getRecorder.Code != http.StatusOK || !strings.Contains(getRecorder.Body.String(), `"name":"voice"`) {
		t.Fatalf("get response = %d %s, want scoped voice detail", getRecorder.Code, getRecorder.Body.String())
	}
	if getRequest.Scope != scope || getRequest.Name != "voice" {
		t.Fatalf("GetCatalogModel request = %#v, want scoped voice", getRequest)
	}
}

func TestCatalogRootErrorResponse_DoesNotLeakInternalPaths(t *testing.T) {
	t.Parallel()

	internalErr := errors.New("pkg/services/models/internal/catalog: cache dir /tmp/models unavailable")
	status, response, ok := CatalogRootErrorResponse(internalErr)
	if ok {
		t.Fatalf("CatalogRootErrorResponse(%v) = handled, want unmapped internal failure", internalErr)
	}
	if status != 0 || response.Message != "" {
		t.Fatalf("CatalogRootErrorResponse(%v) = %d %#v, want unmapped", internalErr, status, response)
	}
}

func assertCatalogHTTPError(
	t *testing.T,
	recorder *httptest.ResponseRecorder,
	wantStatus int,
	wantCode string,
	wantMessage string,
) {
	t.Helper()

	if recorder.Code != wantStatus {
		t.Fatalf("status = %d, want %d body = %s", recorder.Code, wantStatus, recorder.Body.String())
	}
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if string(response.Code) != wantCode {
		t.Fatalf("code = %q, want %q", response.Code, wantCode)
	}
	if response.Message != wantMessage {
		t.Fatalf("message = %q, want %q", response.Message, wantMessage)
	}
	if strings.Contains(response.Message, "pkg/services/models/internal") {
		t.Fatalf("message leaks internal package path: %q", response.Message)
	}
}
