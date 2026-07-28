package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestAdapter_ListProvidersHTTPInvokesFakeRootAndEncodesSuccess(t *testing.T) {
	t.Parallel()

	var invoked bool
	codex := providers.Descriptor{
		ID:           providers.IDCodex,
		DisplayName:  "Codex",
		Availability: providers.AvailabilitySelectable,
		Readiness:    providers.ReadinessReady,
	}
	fake := &rootFake{
		listProviders: func(context.Context, providers.ListProvidersRequest) (providers.ListProvidersResult, error) {
			invoked = true
			return providers.ListProvidersResult{Providers: []providers.Descriptor{codex}}, nil
		},
	}
	adapter := NewAdapter(fake)
	recorder := httptest.NewRecorder()

	adapter.ListProvidersHTTP(recorder, httptest.NewRequest(http.MethodGet, "/providers", nil))

	if !invoked {
		t.Fatal("ListProvidersHTTP did not invoke the injected Providers root")
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	var response ListProvidersResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Providers) != 1 || response.Providers[0].ID != "codex" {
		t.Fatalf("response = %#v, want encoded codex descriptor", response)
	}
}

func TestAdapter_GetProviderHTTPInvokesFakeRootWithDecodedID(t *testing.T) {
	t.Parallel()

	var invokedID providers.ID
	codex := providers.Descriptor{
		ID:          providers.IDCodex,
		DisplayName: "Codex",
	}
	fake := &rootFake{
		getProvider: func(_ context.Context, request providers.GetProviderRequest) (providers.GetProviderResult, error) {
			invokedID = request.ID
			return providers.GetProviderResult{Provider: codex}, nil
		},
	}
	adapter := NewAdapter(fake)
	recorder := httptest.NewRecorder()

	adapter.GetProviderHTTP(recorder, httptest.NewRequest(http.MethodGet, "/providers/codex", nil), "codex")

	if invokedID != providers.IDCodex {
		t.Fatalf("GetProviderHTTP invoked root with id = %q, want codex", invokedID)
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	var response GetProviderResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Provider.ID != "codex" {
		t.Fatalf("response = %#v, want encoded codex descriptor", response)
	}
}

func TestAdapter_GetProviderHTTPRejectsInvalidIDBeforeFakeRoot(t *testing.T) {
	t.Parallel()

	fake := &rootFake{
		getProvider: func(context.Context, providers.GetProviderRequest) (providers.GetProviderResult, error) {
			t.Fatal("fake root must not be invoked for invalid provider id")
			return providers.GetProviderResult{}, nil
		},
	}
	adapter := NewAdapter(fake)
	recorder := httptest.NewRecorder()

	adapter.GetProviderHTTP(recorder, httptest.NewRequest(http.MethodGet, "/providers/", nil), "   ")

	assertCatalogHTTPError(t, recorder, http.StatusBadRequest, "BAD_REQUEST", "invalid provider id")
}

func TestAdapter_GetProviderHTTPMapsUnknownProviderFromFakeRoot(t *testing.T) {
	t.Parallel()

	fake := &rootFake{
		getProvider: func(context.Context, providers.GetProviderRequest) (providers.GetProviderResult, error) {
			return providers.GetProviderResult{}, providers.ErrUnknownProvider
		},
	}
	adapter := NewAdapter(fake)
	recorder := httptest.NewRecorder()

	adapter.GetProviderHTTP(recorder, httptest.NewRequest(http.MethodGet, "/providers/missing", nil), "missing")

	assertCatalogHTTPError(t, recorder, http.StatusNotFound, "NOT_FOUND", "provider not found")
}

func TestAdapter_GetProviderHTTPMapsUnavailableProviderFromFakeRoot(t *testing.T) {
	t.Parallel()

	fake := &rootFake{
		getProvider: func(context.Context, providers.GetProviderRequest) (providers.GetProviderResult, error) {
			return providers.GetProviderResult{}, providers.ErrProviderUnavailable
		},
	}
	adapter := NewAdapter(fake)
	recorder := httptest.NewRecorder()

	adapter.GetProviderHTTP(recorder, httptest.NewRequest(http.MethodGet, "/providers/cursor", nil), "cursor")

	assertCatalogHTTPError(
		t,
		recorder,
		http.StatusNotFound,
		"PROVIDER_UNAVAILABLE",
		providers.ErrProviderUnavailable.Error(),
	)
}

func TestWriteCatalogOrInternalError_SanitizesUnmappedFailures(t *testing.T) {
	t.Parallel()

	adapter := NewAdapter(&rootFake{})
	recorder := httptest.NewRecorder()
	err := errors.New("pkg/services/providers/internal/catalog: boom")

	WriteCatalogOrInternalErrorForTest(adapter, recorder, err, catalogGetFailedMessage)

	body := recorder.Body.String()
	if recorder.Code != http.StatusInternalServerError ||
		!strings.Contains(body, `"code":"INTERNAL_ERROR"`) ||
		!strings.Contains(body, `"family":"INTERNAL_SERVER_ERROR"`) ||
		!strings.Contains(body, `"message":"failed to get provider"`) ||
		strings.Contains(body, "pkg/services/providers") ||
		strings.Contains(body, "boom") {
		t.Fatalf("response = %d %s, want sanitized internal error", recorder.Code, body)
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
	if strings.Contains(response.Message, "pkg/services/providers/internal") {
		t.Fatalf("message leaks internal package path: %q", response.Message)
	}
}
