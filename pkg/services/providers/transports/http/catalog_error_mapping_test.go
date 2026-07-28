package http_test

import (
	"errors"
	"net/http"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	providershttp "github.com/portpowered/infinite-you/pkg/services/providers/transports/http"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestCatalogRootErrorResponse_MapsInvalidIDFailures(t *testing.T) {
	t.Parallel()

	status, response, ok := providershttp.CatalogRootErrorResponseForTest(providers.ErrInvalidID)
	if !ok {
		t.Fatal("CatalogRootErrorResponse(ErrInvalidID) = not handled, want typed bad request")
	}
	if status != http.StatusBadRequest ||
		response.Family != factoryapi.ErrorFamilyBadRequest ||
		response.Code != factoryapi.ErrorResponseCodeBADREQUEST ||
		response.Message != "invalid provider id" {
		t.Fatalf("CatalogRootErrorResponse(ErrInvalidID) = %d %#v, want bad request", status, response)
	}
}

func TestCatalogRootErrorResponse_MapsUnknownProviderFailures(t *testing.T) {
	t.Parallel()

	status, response, ok := providershttp.CatalogRootErrorResponseForTest(providers.ErrUnknownProvider)
	if !ok {
		t.Fatal("CatalogRootErrorResponse(ErrUnknownProvider) = not handled, want typed not found")
	}
	if status != http.StatusNotFound ||
		response.Family != factoryapi.ErrorFamilyNotFound ||
		response.Code != factoryapi.ErrorResponseCodeNOTFOUND ||
		response.Message != "provider not found" {
		t.Fatalf("CatalogRootErrorResponse(ErrUnknownProvider) = %d %#v, want not found", status, response)
	}
}

func TestCatalogRootErrorResponse_MapsUnavailableProviderFailures(t *testing.T) {
	t.Parallel()

	status, response, ok := providershttp.CatalogRootErrorResponseForTest(providers.ErrProviderUnavailable)
	if !ok {
		t.Fatal("CatalogRootErrorResponse(ErrProviderUnavailable) = not handled, want typed unavailable")
	}
	if status != http.StatusNotFound ||
		response.Family != factoryapi.ErrorFamilyNotFound ||
		response.Code != factoryapi.ErrorResponseCode("PROVIDER_UNAVAILABLE") ||
		response.Message != providers.ErrProviderUnavailable.Error() {
		t.Fatalf("CatalogRootErrorResponse(ErrProviderUnavailable) = %d %#v, want unavailable", status, response)
	}
}

func TestCatalogRootErrorResponse_ReturnsFalseForUnmappedFailures(t *testing.T) {
	t.Parallel()

	err := errors.New("pkg/services/providers/internal/catalog: boom")
	if _, _, ok := providershttp.CatalogRootErrorResponseForTest(err); ok {
		t.Fatalf("unmapped failure %#v must not be handled", err)
	}
}
