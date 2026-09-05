package inference_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/internal/functionalevidence"
)

// TestWSRFT010WorkerSessionIDHTTPHistory proves the canonical Factory Session
// Worker-ID reads through the public server. The first process supplies live
// registry reads and a provider-neutral Worker execution; a fresh replay
// process then supplies the historical read without a provider or live Events
// topic. The same test also checks scope isolation and typed identity errors.
//
// WSR-FT-010: live, restarted historical, and no-Provider-Session-reference
// Worker-ID observation/transcript/event reads.
func TestWSRFT010WorkerSessionIDHTTPHistory(t *testing.T) {
	t.Parallel()
	dir := support.ScaffoldSingleStepFactory(t, "wsr-ft-010-worker-id-history")
	home := t.TempDir()
	artifactPath := filepath.Join(t.TempDir(), "wsr-ft-010-worker-id-history.replay.json")
	sessionID := uuid.NewString()
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Args:                      []string{"--session", sessionID, "--record", artifactPath},
		Edges:                     serviceedges.Edges{WorkerSessionResolveHomeDirectory: func() (string, error) { return home, nil }},
		ProviderOverride:          support.MockInferenceProvider("provider-neutral completion"),
	})

	workName := "wsr-ft-010-work"
	submitted := support.SubmitSessionWorkAt(t, server.URL(), sessionID, factoryapi.SubmitWorkRequest{
		Name:         &workName,
		WorkTypeName: "task",
		Payload:      json.RawMessage(`{"title":"WSR-FT-010 Worker-ID history"}`),
	})
	if submitted.WorkId == nil || strings.TrimSpace(*submitted.WorkId) == "" {
		t.Fatalf("submit response = %#v, want a Work ID", submitted)
	}
	workID := *submitted.WorkId
	support.WaitForSessionTerminalStatus(t, server.URL(), sessionID, 30*time.Second)

	liveList := support.ListSessionWorkerSessions(t, server.URL(), sessionID, workID)
	if len(liveList.Sessions) != 1 {
		t.Fatalf("live Worker Session list = %#v, want one observation", liveList)
	}
	workerID := liveList.Sessions[0].WorkerSessionId
	if strings.TrimSpace(workerID) == "" {
		t.Fatalf("live Worker Session observation = %#v, want Worker Session ID", liveList.Sessions[0])
	}

	live := getWSRFT010Observation(t, server.URL(), sessionID, workerID)
	assertWSRFT010Observation(t, live, sessionID, workerID, workID)
	if live.RecordingHealth == nil || *live.RecordingHealth != factoryapi.WorkerSessionObservationRecordingHealthComplete {
		t.Fatalf("live Worker-ID recording health = %#v, want COMPLETE", live.RecordingHealth)
	}
	liveEvents := getWSRFT010Events(t, server.URL(), sessionID, workerID)
	assertWSRFT010Events(t, liveEvents, workerID, workID)
	assertWSRFT010DurableTranscript(t, server.URL(), sessionID, workerID, workID)
	assertWSRFT010IdentityErrors(t, server.URL(), sessionID)

	server.Stop(t)
	replayServer := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                t.TempDir(),
		WaitForServiceModeRuntime: true,
		Args:                      []string{"--session", sessionID, "--replay", artifactPath, "--no-record"},
		Edges:                     serviceedges.Edges{WorkerSessionResolveHomeDirectory: func() (string, error) { return home, nil }},
	})
	historical := getWSRFT010Observation(t, replayServer.URL(), sessionID, workerID)
	assertWSRFT010Observation(t, historical, sessionID, workerID, workID)
	if historical.ProviderSessionAvailable || historical.ProviderSession != nil {
		t.Fatalf("historical provider association = %#v/%t, want no Provider Session reference", historical.ProviderSession, historical.ProviderSessionAvailable)
	}
	historicalEvents := getWSRFT010Events(t, replayServer.URL(), sessionID, workerID)
	assertWSRFT010Events(t, historicalEvents, workerID, workID)
	assertWSRFT010DurableTranscript(t, replayServer.URL(), sessionID, workerID, workID)

	status, response := getWSRFT010Error(t, workerObservationURL(replayServer.URL(), "other-session", workerID))
	if status != http.StatusNotFound || response.Code != factoryapi.ErrorResponseCodeNOTFOUND {
		t.Fatalf("cross-session Worker-ID read = %d/%#v, want 404/NOT_FOUND", status, response)
	}
	status, response = getWSRFT010Error(t, workerObservationURL(replayServer.URL(), "other-session", workerID)+"/transcript")
	if status != http.StatusNotFound || response.Code != factoryapi.ErrorResponseCodeNOTFOUND {
		t.Fatalf("cross-session Worker-ID transcript read = %d/%#v, want 404/NOT_FOUND", status, response)
	}
	functionalevidence.Covers(t,
		"rest/getWorkerSessionObservationByFactorySessionAndWorkerSessionId",
		"rest/getWorkerSessionObservationByWorkerSessionId",
		"rest/readWorkerSessionTranscriptByFactorySessionAndWorkerSessionId",
		"rest/readWorkerSessionTranscriptByWorkerSessionId",
	)
}

func getWSRFT010Events(t *testing.T, baseURL, sessionID, workerID string) []factoryapi.WorkerSessionEvent {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	endpoint := workerObservationURL(baseURL, sessionID, workerID) + "/events?replayOnly=true"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatalf("build Worker-ID event request: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("GET Worker-ID events: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("GET Worker-ID events status = %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	var events []factoryapi.WorkerSessionEvent
	scanner := bufio.NewScanner(response.Body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var event factoryapi.WorkerSessionEvent
		if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &event); err != nil {
			t.Fatalf("decode Worker-ID event: %v", err)
		}
		events = append(events, event)
		if event.ReplaySummary != nil && event.ReplaySummary.Complete {
			return events
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("read Worker-ID events: %v", err)
	}
	t.Fatalf("Worker-ID event stream ended without complete replay summary: %#v", events)
	return nil
}

func getWSRFT010Observation(
	t *testing.T,
	baseURL, sessionID, workerID string,
) factoryapi.WorkerSessionObservation {
	t.Helper()
	response, err := http.Get(workerObservationURL(baseURL, sessionID, workerID))
	if err != nil {
		t.Fatalf("GET Worker Session observation: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("GET Worker Session observation status = %d, want 200: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	var observation factoryapi.WorkerSessionObservation
	if err := json.NewDecoder(response.Body).Decode(&observation); err != nil {
		t.Fatalf("decode Worker Session observation: %v", err)
	}
	return observation
}

func assertWSRFT010Observation(
	t *testing.T,
	observation factoryapi.WorkerSessionObservation,
	sessionID, workerID, workID string,
) {
	t.Helper()
	if observation.WorkerSessionId != workerID || observation.State != factoryapi.WorkerSessionObservationStateCompleted {
		t.Fatalf("Worker-ID observation = %#v, want completed %q", observation, workerID)
	}
	if observation.FactorySessionId == nil || *observation.FactorySessionId != sessionID {
		t.Fatalf("Worker-ID Factory Session scope = %#v, want %q", observation.FactorySessionId, sessionID)
	}
	if observation.AttemptId == "" || !containsWSRFT010(observation.WorkIds, workID) {
		t.Fatalf("Worker-ID correlation = attempt %q/Work %#v, want Work %q", observation.AttemptId, observation.WorkIds, workID)
	}
	if observation.ProviderSessionAvailable || observation.ProviderSession != nil {
		t.Fatalf("Worker-ID provider association = %#v/%t, want provider-neutral observation", observation.ProviderSession, observation.ProviderSessionAvailable)
	}
	if observation.Transcript != factoryapi.WorkerSessionObservationTranscriptUNAVAILABLE {
		t.Fatalf("Worker-ID transcript availability = %q, want UNAVAILABLE without provider detail", observation.Transcript)
	}
}

func assertWSRFT010Events(
	t *testing.T,
	events []factoryapi.WorkerSessionEvent,
	workerID, workID string,
) {
	t.Helper()
	if len(events) < 2 {
		t.Fatalf("Worker-ID event history = %#v, want records and replay summary", events)
	}
	for _, event := range events {
		if event.WorkerSessionId != workerID || !containsWSRFT010(event.WorkIds, workID) {
			t.Fatalf("Worker-ID event correlation = %#v, want Worker %q/Work %q", event, workerID, workID)
		}
	}
	last := events[len(events)-1]
	if last.ReplaySummary == nil || !last.ReplaySummary.Complete {
		t.Fatalf("Worker-ID event history final frame = %#v, want complete replay summary", last)
	}
	for _, event := range events[:len(events)-1] {
		if event.ProviderSession != nil {
			t.Fatalf("provider-neutral Worker-ID event = %#v, want no Provider Session reference", event.ProviderSession)
		}
	}
}

func assertWSRFT010DurableTranscript(t *testing.T, baseURL, sessionID, workerID, workID string) {
	t.Helper()
	response, err := http.Get(workerObservationURL(baseURL, sessionID, workerID) + "/transcript")
	if err != nil {
		t.Fatalf("GET durable Worker-ID transcript: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("durable Worker-ID transcript status = %d, want 200: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	var transcript factoryapi.WorkerSessionTranscriptResponse
	if err := json.NewDecoder(response.Body).Decode(&transcript); err != nil {
		t.Fatalf("decode durable Worker-ID transcript: %v", err)
	}
	if transcript.WorkerSessionId != workerID || transcript.ProviderSession != nil {
		t.Fatalf("durable Worker-ID transcript identity = %#v, want Worker %q and no Provider Session", transcript, workerID)
	}
	if transcript.RecordingHealth != factoryapi.WorkerSessionTranscriptRecordingHealthComplete {
		t.Fatalf("durable Worker-ID transcript health = %q, want COMPLETE", transcript.RecordingHealth)
	}
	if !containsWSRFT010(transcript.WorkIds, workID) || len(transcript.Entries) == 0 {
		t.Fatalf("durable Worker-ID transcript correlation = Work %#v/entries=%d, want Work %q and entries", transcript.WorkIds, len(transcript.Entries), workID)
	}
	for index := 1; index < len(transcript.Entries); index++ {
		if transcript.Entries[index].Order <= transcript.Entries[index-1].Order {
			t.Fatalf("durable Worker-ID transcript entries out of order: %#v", transcript.Entries)
		}
	}
}

func assertWSRFT010IdentityErrors(t *testing.T, baseURL, sessionID string) {
	t.Helper()
	status, response := getWSRFT010Error(t, workerObservationURL(baseURL, sessionID, " "))
	if status != http.StatusBadRequest || response.Code != factoryapi.ErrorResponseCodeBADREQUEST {
		t.Fatalf("malformed Worker-ID read = %d/%#v, want 400/BAD_REQUEST", status, response)
	}
	status, response = getWSRFT010Error(t, workerObservationURL(baseURL, sessionID, "unknown-worker-session"))
	if status != http.StatusNotFound || response.Code != factoryapi.ErrorResponseCodeNOTFOUND {
		t.Fatalf("unknown Worker-ID read = %d/%#v, want 404/NOT_FOUND", status, response)
	}
}

func getWSRFT010Error(t *testing.T, endpoint string) (int, factoryapi.ErrorResponse) {
	t.Helper()
	response, err := http.Get(endpoint)
	if err != nil {
		t.Fatalf("GET %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	var payload factoryapi.ErrorResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode %s error response: %v", endpoint, err)
	}
	return response.StatusCode, payload
}

func workerObservationURL(baseURL, sessionID, workerID string) string {
	return fmt.Sprintf(
		"%s/factory-sessions/%s/worker-sessions/%s",
		strings.TrimSuffix(baseURL, "/"),
		url.PathEscape(sessionID),
		url.PathEscape(workerID),
	)
}

func containsWSRFT010(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
