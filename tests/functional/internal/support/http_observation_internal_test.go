package support

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestWaitForStatusAtTimeoutReportsLastObservation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(factoryapi.StatusResponse{
			Categories:    factoryapi.StatusCategories{Processing: 2},
			RuntimeStatus: "ACTIVE",
			TotalTokens:   2,
		})
	}))
	defer server.Close()

	endpoint := server.URL + "/factory-sessions/session-timeout-context/status"
	_, err := waitForStatusAt(endpoint, 25*time.Millisecond, terminalSessionStatusIsComplete)
	if err == nil {
		t.Fatal("waitForStatusAt() error = nil")
	}
	message := err.Error()
	for _, want := range []string{
		`RuntimeStatus:"ACTIVE"`,
		"Processing:2",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("timeout error = %q, want %q", message, want)
		}
	}
}
