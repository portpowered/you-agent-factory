package omni_artifact_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestFactorySessionOmniArtifactRedactsUnsafeFailureAcrossBoundaries(t *testing.T) {
	t.Parallel()

	unsafeValues := []string{
		`C:\unsafe\omni\secret.txt`,
		"127.0.0.1:49123",
		"omni-secret-value",
		"raw-protocol-response",
	}
	fixture := newFactoryFixture(t, "unused after redaction")
	fixture.protocol.SetError(errors.New(fmt.Sprintf(
		"protocol failed path=%s address=%s secret=%s raw=%s",
		unsafeValues[0], unsafeValues[1], unsafeValues[2], unsafeValues[3],
	)))
	fixture.startLive(t, "redact unsafe protocol failure")
	fixture.submit(t, "redact unsafe protocol failure")
	support.WaitForSessionTerminalStatus(t, fixture.baseURL, fixture.sessionID, omniFactoryFunctionalTimeout)

	cliStdout := fixture.inputs.Stdout()
	cliStderr := fixture.inputs.Stderr()
	workBody := readPublicHTTPBody(t, support.SessionWorkURL(fixture.baseURL, fixture.sessionID, "/work"))
	eventsBody := readPublicEventsHTTPBody(t, support.SessionEventsURL(fixture.baseURL, fixture.sessionID))
	fixture.stopLive(t)
	recordingBody, err := os.ReadFile(fixture.recordingPath)
	if err != nil {
		t.Fatalf("read redaction recording: %v", err)
	}
	liveEvents := fixture.recording(t)

	observations := map[string]string{
		"CLI stdout":              cliStdout,
		"CLI stderr":              cliStderr,
		"public Work HTTP":        workBody,
		"public events HTTP":      eventsBody,
		"recording artifact":      string(recordingBody),
		"decoded live event JSON": mustMarshalEvents(t, liveEvents),
	}
	for label, observed := range observations {
		assertRedaction(t, label, observed, unsafeValues)
	}

	replayed := fixture.replay(t)
	assertRedaction(t, "replay Work HTTP", replayed.rawWork, unsafeValues)
	assertRedaction(t, "replay events HTTP", replayed.rawEvents, unsafeValues)
}

func readPublicHTTPBody(t *testing.T, endpoint string) string {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatalf("build GET %s: %v", endpoint, err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("GET %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read GET %s: %v", endpoint, err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		t.Fatalf("GET %s status = %d: %s", endpoint, response.StatusCode, strings.TrimSpace(string(body)))
	}
	return string(body)
}

func readPublicEventsHTTPBody(t *testing.T, endpoint string) string {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatalf("build GET %s: %v", endpoint, err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("GET %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("GET %s status = %d: %s", endpoint, response.StatusCode, strings.TrimSpace(string(body)))
	}
	retained, err := strconv.Atoi(strings.TrimSpace(response.Header.Get("X-Factory-Session-Retained-Event-Count")))
	if err != nil {
		t.Fatalf("GET %s retained event count = %q: %v", endpoint, response.Header.Get("X-Factory-Session-Retained-Event-Count"), err)
	}
	scanner := bufio.NewScanner(response.Body)
	lines := make([]string, 0, retained)
	for len(lines) < retained && scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data:") {
			lines = append(lines, line)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("read GET %s: %v", endpoint, err)
	}
	if len(lines) != retained {
		t.Fatalf("GET %s returned %d retained events, want %d", endpoint, len(lines), retained)
	}
	return strings.Join(lines, "\n")
}

func mustMarshalEvents(t *testing.T, events interface{}) string {
	t.Helper()
	data, err := json.Marshal(events)
	if err != nil {
		t.Fatalf("marshal observed events: %v", err)
	}
	return string(data)
}

func assertRedaction(t *testing.T, label, observed string, unsafeValues []string) {
	t.Helper()
	for _, unsafeValue := range unsafeValues {
		if strings.Contains(observed, unsafeValue) {
			t.Fatalf("%s leaked unsafe value %q: %s", label, unsafeValue, observed)
		}
	}
}
