package factory

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

func writeFactoryConfigFile(t *testing.T, dir, stem string, payload []byte) string {
	t.Helper()

	path := filepath.Join(dir, stem+".json")
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
	return path
}

func saveTestNamedFactoryPayload(t *testing.T, project string) []byte {
	t.Helper()
	return listTestNamedFactoryPayload(t, project)
}

func ioDiscard(t *testing.T) *strings.Builder {
	t.Helper()
	return &strings.Builder{}
}

func currentFactorySaveServer(t *testing.T, current factoryapi.Factory) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/factory-sessions/~default/factory" {
			t.Fatalf("path = %q, want /factory-sessions/~default/factory", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet, http.MethodPut:
			if err := json.NewEncoder(w).Encode(current); err != nil {
				t.Fatalf("encode response: %v", err)
			}
		default:
			t.Fatalf("method = %s", r.Method)
		}
	}))
}
