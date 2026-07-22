//go:build functionallong

package support

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// RecordReplayFixture creates a deterministic recorded artifact for functional
// replay scenarios using only scripted test collaborators.
func RecordReplayFixture(t *testing.T) string {
	t.Helper()

	dir := testutil.CopyFixtureDir(t, LegacyFixtureDir(t, "service_simple"))
	artifactPath := filepath.Join(t.TempDir(), "service-simple.replay.json")
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "task",
		WorkID:     "replay-event-stream-work",
		TraceID:    "replay-event-stream-trace",
		Payload:    []byte(`{"title":"replay event stream"}`),
	})

	server := StartFunctionalAPIServer(t, FunctionalAPIServerConfig{
		FactoryDir:                dir,
		Args:                      []string{"--record", artifactPath},
		WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{
			ProviderOverride: testutil.NewMockProvider(
				workerexecution.InferenceResponse{Content: "Step one done. COMPLETE"},
				workerexecution.InferenceResponse{Content: "Step two done. COMPLETE"},
			),
		},
	})
	response := submitReplayFixtureWorkRequest(t, server.URL())
	if response.RequestId != "replay-event-stream-request" || len(response.Works) != 1 {
		t.Fatalf("public replay-fixture work-request response = %#v, want one accepted work", response)
	}
	waitForReplayFixtureRecording(t, artifactPath)
	server.Stop(t)
	return artifactPath
}

func submitReplayFixtureWorkRequest(t *testing.T, baseURL string) factoryapi.UpsertWorkRequestResponse {
	t.Helper()

	body := []byte(`{"requestId":"replay-event-stream-request","type":"FACTORY_REQUEST_BATCH","works":[{"name":"replay-event-stream-work","workId":"replay-event-stream-work","workTypeName":"task","traceId":"replay-event-stream-trace","payload":"{\"title\":\"replay event stream\"}"}]}`)
	req, err := http.NewRequest(http.MethodPut, DefaultSessionWorkURL(baseURL, "/work-requests/replay-event-stream-request"), bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build public replay-fixture work request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("submit public replay-fixture work request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("submit public replay-fixture work request status = %d, want %d", resp.StatusCode, http.StatusCreated)
	}
	var result factoryapi.UpsertWorkRequestResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode public replay-fixture work request response: %v", err)
	}
	return result
}

func waitForReplayFixtureRecording(t *testing.T, artifactPath string) {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(artifactPath)
		if err == nil {
			var artifact struct {
				Events []factoryapi.FactoryEvent `json:"events"`
			}
			if json.Unmarshal(data, &artifact) == nil && replayFixtureEventCount(artifact.Events, factoryapi.FactoryEventTypeDispatchResponse) > 0 {
				return
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for completed public recording at %s", artifactPath)
}

func replayFixtureEventCount(events []factoryapi.FactoryEvent, kind factoryapi.FactoryEventType) int {
	count := 0
	for _, event := range events {
		if event.Type == kind {
			count++
		}
	}
	return count
}
