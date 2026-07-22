package support_test

import (
	"strings"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestDefaultSessionEventsURL_UsesCanonicalSessionScopedRoute(t *testing.T) {
	t.Parallel()

	got := support.DefaultSessionEventsURL("http://127.0.0.1:7437")
	want := "http://127.0.0.1:7437/factory-sessions/" + factorysessions.DefaultSessionID + "/events"
	if got != want {
		t.Fatalf("DefaultSessionEventsURL = %q, want %q", got, want)
	}
	if strings.HasSuffix(got, "/events") && !strings.Contains(got, "/factory-sessions/") {
		t.Fatalf("default session event stream must not use process-global /events, got %q", got)
	}
}
