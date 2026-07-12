package factory

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

func TestReplaceCurrent_WritesHumanReadableConfirmation(t *testing.T) {
	srv := currentFactorySaveServer(t, factoryapi.Factory{Name: "beta"})
	defer srv.Close()

	var out strings.Builder
	if err := ReplaceCurrent(ReplaceCurrentConfig{Server: serverBase(t, srv), Output: &out}); err != nil {
		t.Fatalf("ReplaceCurrent: %v", err)
	}

	want := "Replaced current factory beta\nSession: ~default\n"
	if got := out.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestReplaceCurrent_UsesSessionScopedPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/factory-sessions/session-beta/factory" {
			t.Fatalf("path = %q, want /factory-sessions/session-beta/factory", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet, http.MethodPut:
			if err := json.NewEncoder(w).Encode(factoryapi.Factory{Name: "beta"}); err != nil {
				t.Fatalf("encode response: %v", err)
			}
		default:
			t.Fatalf("method = %s", r.Method)
		}
	}))
	defer srv.Close()

	var out strings.Builder
	if err := ReplaceCurrent(ReplaceCurrentConfig{
		Server:    serverBase(t, srv),
		SessionID: "session-beta",
		Output:    &out,
	}); err != nil {
		t.Fatalf("ReplaceCurrent: %v", err)
	}
	if got := out.String(); !strings.Contains(got, "Session: session-beta\n") {
		t.Fatalf("output = %q, want session-beta label", got)
	}
}
