package support

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestObserveSessionTerminalStatusTimeoutReportsCorrelationAndLastObservation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/events") {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set(factorysessions.SessionEventStreamRetainedCountHeader, "0")
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			<-r.Context().Done()
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(factoryapi.StatusResponse{
			Categories:    factoryapi.StatusCategories{Processing: 2},
			RuntimeStatus: "ACTIVE",
			TotalTokens:   2,
		})
	}))
	defer server.Close()

	_, err := observeSessionTerminalStatus(server.URL, "session-timeout-context", 25*time.Millisecond)
	if err == nil {
		t.Fatal("observeSessionTerminalStatus() error = nil")
	}
	message := err.Error()
	for _, want := range []string{
		"session-timeout-context",
		"/factory-sessions/session-timeout-context/status",
		`RuntimeStatus:"ACTIVE"`,
		"Processing:2",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("timeout error = %q, want %q", message, want)
		}
	}
}
