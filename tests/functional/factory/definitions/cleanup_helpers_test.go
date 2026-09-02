package definitions

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// closeDefinitionsFactorySession closes customer-created sessions and verifies
// their public identity is no longer readable.
func closeDefinitionsFactorySession(t testing.TB, baseURL, sessionID string) bool {
	t.Helper()
	support.CloseFactorySessionAt(t, baseURL, sessionID)
	if deleted := definitionsFactorySessionDeleted(baseURL, sessionID); !deleted {
		t.Errorf("Factory Definitions session %q remains after cleanup", sessionID)
		return false
	}
	return true
}

func definitionsFactorySessionDeleted(baseURL, sessionID string) bool {
	if strings.TrimSpace(baseURL) == "" || strings.TrimSpace(sessionID) == "" {
		return false
	}
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID)
	client := http.Client{Timeout: time.Second}
	response, err := client.Get(endpoint)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	return response.StatusCode == http.StatusNotFound
}

func definitionsListenerClosed(done <-chan struct{}) bool {
	if done == nil {
		return false
	}
	select {
	case <-done:
		return true
	default:
		return false
	}
}
