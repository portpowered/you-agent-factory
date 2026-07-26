package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

func TestListPackagedFactoriesReturnsPublishedCatalog(t *testing.T) {
	srv := NewServer(nil, nil, nil, zap.NewNop())
	recorder := httptest.NewRecorder()
	srv.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/packaged-factories", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var response factoryapi.PackagedFactoryCatalogResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Factories) == 0 {
		t.Fatal("catalog response contained no factories")
	}
	for _, factory := range response.Factories {
		if factory.Name == "" || factory.Project == "" || factory.Slug == "" || len(factory.Json) == 0 || factory.Yaml == "" {
			t.Fatalf("catalog entry is incomplete: %#v", factory)
		}
	}
}
