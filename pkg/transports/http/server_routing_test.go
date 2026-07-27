package http

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUnknownRouteReturnsStructuredNotFound(t *testing.T) {
	srv := newFactoryDefinitionTestServer(nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/functional-routing-unknown-route", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assertJSONError(t, rec, http.StatusNotFound, "NOT_FOUND", "route not found")
}
