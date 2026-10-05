package customer_journeys_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// The read/recovery boundary accepts the documented snapshot representation.
// Seed a valid Direct tool record without a provider association; provider
// decoders have their own size policies and are not the subject of this proof.
func assertOversizedCapturedPayload(t *testing.T, config support.FunctionalAPIServerConfig, current factoryapi.WorkerSessionLogPage) {
	t.Helper()
	page := current
	page.Events = append([]factoryapi.WorkerSessionEvent(nil), current.Events...)
	position := replaceCapturedToolPayload(t, &page)
	exact, err := json.Marshal(page.Events[position].Event.Payload)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	writeLegacyCapturedFixture(t, root, page)
	config.Edges.FactorySessionsWorkingDirectory = capturedRecordingDirectory(root)
	server := support.StartFunctionalAPIServer(t, config)
	bounded := assertLegacyLogsCLIHTTPParity(t, server, page.WorkerSessionId)
	ref := assertCapturedPayloadMetadata(t, bounded.Events[position].Event, exact)
	assertCapturedPayloadRoundTrip(t, server, page.WorkerSessionId, ref, exact)
	for _, query := range []string{
		"artifactRef=" + url.QueryEscape("sibling/2"),
		"artifactRef=" + url.QueryEscape(ref) + "&limit=1",
		"artifactRef=" + url.QueryEscape(ref) + "&nextToken=invalid",
		"artifactRef=" + url.QueryEscape(page.WorkerSessionId+"/../2"),
	} {
		response, err := http.Get(server.URL() + "/worker-sessions/" + url.PathEscape(page.WorkerSessionId) + "/logs?" + query)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusBadRequest {
			t.Fatalf("invalid artifact selector status=%d", response.StatusCode)
		}
	}
	server.Close(t)
	restarted := support.StartFunctionalAPIServer(t, config)
	assertCapturedPayloadRoundTrip(t, restarted, page.WorkerSessionId, ref, exact)
}

func replaceCapturedToolPayload(t *testing.T, page *factoryapi.WorkerSessionLogPage) int {
	t.Helper()
	for index, frame := range page.Events {
		if frame.Event.Payload["kind"] != "MESSAGE" {
			continue
		}
		// A new map preserves the original witness's payload and event identity.
		payload := make(map[string]interface{}, len(frame.Event.Payload))
		for key, value := range frame.Event.Payload {
			payload[key] = value
		}
		payload["kind"] = "TOOL"
		payload["phase"] = "DELTA"
		payload["payload"] = map[string]any{"toolCallId": "large-tool", "outputDelta": strings.Repeat("x", 3<<20)}
		page.Events[index].Event.Payload = payload
		return index
	}
	t.Fatal("Direct fixture has no captured message to replace with tool output")
	return 0
}

func assertCapturedPayloadMetadata(t *testing.T, event factoryapi.WorkerSessionEventRecord, exact []byte) string {
	t.Helper()
	returned, err := json.Marshal(event.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if event.Truncated == nil || !*event.Truncated || event.ArtifactRef == nil || event.OriginalBytes == nil || *event.OriginalBytes != int64(len(exact)) || event.ReturnedBytes == nil || *event.ReturnedBytes != int64(len(returned)) || len(returned) > 1<<20 {
		t.Fatal("oversized event lost its cap or exact size/reference metadata")
	}
	frame, err := json.Marshal(event)
	if err != nil || len(frame) > 1<<20 {
		t.Fatal("returned oversized event exceeds 1 MiB")
	}
	return *event.ArtifactRef
}

func assertCapturedPayloadRoundTrip(t *testing.T, server *support.FunctionalAPIServer, id, ref string, exact []byte) {
	t.Helper()
	endpoint := server.URL() + "/worker-sessions/" + url.PathEscape(id) + "/logs?artifactRef=" + url.QueryEscape(ref)
	response, err := http.Get(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil || response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "application/octet-stream" || !bytes.Equal(data, exact) {
		t.Fatalf("HTTP full payload differs: status=%d bytes=%d error=%v", response.StatusCode, len(data), err)
	}
	inputs := support.FakeInputs(t.Context(), []string{"you", "worker-sessions", "read", "--worker-session-id", id, "--view", "logs", "--artifact-ref", ref, "--server", server.URL(), "--output", "json"})
	if err := server.Execute(t, inputs.Input); err != nil {
		t.Fatalf("CLI full payload: %v %s", err, inputs.Stderr())
	}
	if !bytes.Equal([]byte(inputs.Stdout()), exact) {
		t.Fatal("CLI full payload changed captured bytes or added an envelope")
	}
}
