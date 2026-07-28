package http

import (
	"errors"
	"net/http"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestCatalogRootErrorResponse_MapsNotFoundFailures(t *testing.T) {
	t.Parallel()

	status, response, ok := CatalogRootErrorResponse(models.ErrNotFound)
	if !ok {
		t.Fatal("CatalogRootErrorResponse(ErrNotFound) = not handled, want typed not found")
	}
	if status != http.StatusNotFound ||
		response.Family != factoryapi.ErrorFamilyNotFound ||
		response.Code != factoryapi.ErrorResponseCodeNOTFOUND ||
		response.Message != catalogNotFoundMessage {
		t.Fatalf("CatalogRootErrorResponse(ErrNotFound) = %d %#v, want not found", status, response)
	}
}

func TestCatalogRootErrorResponse_MapsUnavailableCatalogFailures(t *testing.T) {
	t.Parallel()

	status, response, ok := CatalogRootErrorResponse(models.ErrUnavailable)
	if !ok {
		t.Fatal("CatalogRootErrorResponse(ErrUnavailable) = not handled, want typed unavailable")
	}
	if status != http.StatusNotFound ||
		response.Family != factoryapi.ErrorFamilyNotFound ||
		response.Code != factoryapi.ErrorResponseCode(catalogErrorCodeModelNotAvailable) ||
		response.Message != models.ErrUnavailable.Error() {
		t.Fatalf("CatalogRootErrorResponse(ErrUnavailable) = %d %#v, want unavailable catalog", status, response)
	}
}

func TestCatalogRootErrorResponse_LeavesUnmappedInternalFailures(t *testing.T) {
	t.Parallel()

	err := errors.New("pkg/services/models/internal/catalog: boom")
	if _, _, ok := CatalogRootErrorResponse(err); ok {
		t.Fatalf("CatalogRootErrorResponse(%v) = handled, want unmapped internal failure", err)
	}
}
