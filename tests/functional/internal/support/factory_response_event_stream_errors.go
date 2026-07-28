package support

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// FactoryResponseEventStreamHTTPError is a typed non-200 response from the
// public Response Event SSE endpoint before stream headers are committed.
type FactoryResponseEventStreamHTTPError struct {
	StatusCode int
	Response   factoryapi.ErrorResponse
	Body       string
}

// GetFactoryResponseEventStreamHTTPErrorAt requests one Response Event stream
// and returns the typed HTTP error payload when the open is rejected.
func GetFactoryResponseEventStreamHTTPErrorAt(
	t testing.TB,
	endpoint string,
) FactoryResponseEventStreamHTTPError {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), functionalServerReadyTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatalf("build factory response event stream error probe request: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("GET factory response event stream error probe: %v", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read factory response event stream error probe response: %v", err)
	}
	bodyText := strings.TrimSpace(string(body))
	if strings.Contains(response.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf(
			"response-event error probe Content-Type = %q, want typed JSON before SSE headers",
			response.Header.Get("Content-Type"),
		)
	}
	var errResp factoryapi.ErrorResponse
	if err := json.Unmarshal(body, &errResp); err != nil {
		t.Fatalf("decode response-event stream error probe: %v: %s", err, bodyText)
	}
	return FactoryResponseEventStreamHTTPError{
		StatusCode: response.StatusCode,
		Response:   errResp,
		Body:       bodyText,
	}
}
