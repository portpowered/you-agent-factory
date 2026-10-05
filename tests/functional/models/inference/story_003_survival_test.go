package inference_test

import (
	"net/http"
	"testing"
)

// assertStory003ServerSurvivesFailedPull proves a failed pull leaves the
// in-process server serving later requests.
func assertStory003ServerSurvivesFailedPull(t *testing.T, serverURL string) {
	t.Helper()
	for _, path := range []string{"/status", "/models"} {
		response, err := http.Get(serverURL + path)
		if err != nil {
			t.Fatalf("GET %s after failed pull: %v", path, err)
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("GET %s after failed pull status = %d, want 200 (server must survive a failed pull)", path, response.StatusCode)
		}
	}
}
