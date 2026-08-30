package support

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factorysessionshttp "github.com/portpowered/infinite-you/pkg/services/recordings/transports/http"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestTerminalFactoryEventObservationAnchorsRetainedCursorAndReturnsPostCursorRunResponse(t *testing.T) {
	const sessionID = "terminal-observation-session"
	requestReady := make(chan struct{})
	release := make(chan struct{})
	server := newTerminalObservationTestServer(t, 2, func(w http.ResponseWriter, r *http.Request) {
		close(requestReady)
		<-release

		writeTerminalObservationSSE(t, w, terminalObservationEventJSON(
			"retained-run-response", sessionID, 10, 101, "RUN_RESPONSE", "FAILED",
		))
		writeTerminalObservationSSE(t, w, terminalObservationEventJSON(
			"retained-telemetry", sessionID, 11, 102, "WORK_STATE_CHANGE", "",
		))
		writeTerminalObservationSSE(t, w, terminalObservationEventJSON(
			"live-telemetry", sessionID, 12, 103, "DISPATCH_RESPONSE", "",
		))
		writeTerminalObservationSSE(t, w, terminalObservationEventJSON(
			"other-session-terminal", "other-session", 13, 1, "RUN_RESPONSE", "FAILED",
		))
		writeTerminalObservationSSE(t, w, terminalObservationEventJSON(
			"live-run-response", sessionID, 14, 104, "RUN_RESPONSE", "COMPLETED",
		))
	})

	observation := OpenSessionTerminalFactoryEventObservation(t, server.URL, sessionID)
	select {
	case <-requestReady:
	case <-time.After(time.Second):
		t.Fatal("terminal event observation request was not established")
	}
	close(release)

	event := observation.Wait(2 * time.Second)
	if event.Id != "live-run-response" || event.Type != factoryapi.FactoryEventTypeRunResponse {
		t.Fatalf("terminal event = %#v, want post-cursor live RUN_RESPONSE", event)
	}
	if event.Context.SessionId == nil || *event.Context.SessionId != sessionID {
		t.Fatalf("terminal event session ID = %#v, want %q", event.Context.SessionId, sessionID)
	}
	payload, err := event.Payload.AsRunResponseEventPayload()
	if err != nil {
		t.Fatalf("decode terminal RUN_RESPONSE payload: %v", err)
	}
	if payload.State == nil || *payload.State != factoryapi.FactoryStateCompleted {
		t.Fatalf("terminal RUN_RESPONSE state = %#v, want COMPLETED", payload.State)
	}

	observation.mu.Lock()
	cursor := observation.cursor
	terminalSignals := observation.terminalSignals
	observation.mu.Unlock()
	if cursor.AfterEventID != "retained-telemetry" || cursor.AfterSequence == nil || *cursor.AfterSequence != 102 {
		t.Fatalf("retained cursor = %#v, want event retained-telemetry/session sequence 102", cursor)
	}
	if terminalSignals != 1 {
		t.Fatalf("terminal signal count = %d, want one capacity-one signal", terminalSignals)
	}

	second, err := observation.wait(time.Second)
	if err != nil {
		t.Fatalf("second terminal wait: %v", err)
	}
	if second.Id != event.Id {
		t.Fatalf("second terminal event ID = %q, want %q", second.Id, event.Id)
	}
}

func TestTerminalEventObservationSupportsConcurrentSessionStreamsAndDuplicateTerminalEvents(t *testing.T) {
	const (
		firstSession  = "terminal-observation-first"
		secondSession = "terminal-observation-second"
	)
	release := make(chan struct{})
	server := newTerminalObservationTestServer(t, 0, func(w http.ResponseWriter, r *http.Request) {
		sessionID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/factory-sessions/"), "/events")
		<-release
		otherSession := secondSession
		if sessionID == secondSession {
			otherSession = firstSession
		}
		writeTerminalObservationSSE(t, w, terminalObservationEventJSON(
			"wrong-session-terminal-"+sessionID, otherSession, 1, 1, "RUN_RESPONSE", "FAILED",
		))
		writeTerminalObservationSSE(t, w, terminalObservationEventJSON(
			"terminal-"+sessionID, sessionID, 2, 2, "RUN_RESPONSE", "COMPLETED",
		))
	})

	first, err := openTerminalFactoryEventObservation(server.URL, firstSession)
	if err != nil {
		t.Fatalf("open first observation: %v", err)
	}
	defer first.Close()
	second, err := openTerminalFactoryEventObservation(server.URL, secondSession)
	if err != nil {
		t.Fatalf("open second observation: %v", err)
	}
	defer second.Close()
	close(release)

	var waitGroup sync.WaitGroup
	waitGroup.Add(2)
	results := make(chan struct {
		name  string
		event factoryapi.FactoryEvent
		err   error
	}, 2)
	go func() {
		defer waitGroup.Done()
		event, err := first.wait(2 * time.Second)
		results <- struct {
			name  string
			event factoryapi.FactoryEvent
			err   error
		}{"first", event, err}
	}()
	go func() {
		defer waitGroup.Done()
		event, err := second.wait(2 * time.Second)
		results <- struct {
			name  string
			event factoryapi.FactoryEvent
			err   error
		}{"second", event, err}
	}()
	waitGroup.Wait()
	close(results)
	for result := range results {
		if result.err != nil {
			t.Fatalf("%s terminal wait: %v", result.name, result.err)
		}
		wantSession := firstSession
		if result.name == "second" {
			wantSession = secondSession
		}
		if result.event.Id != "terminal-"+wantSession || result.event.Context.SessionId == nil || *result.event.Context.SessionId != wantSession {
			t.Fatalf("%s terminal event = %#v, want session-scoped terminal event", result.name, result.event)
		}
	}

	duplicate := decodeTerminalObservationEvent(t, terminalObservationEventJSON(
		"duplicate", firstSession, 3, 3, "RUN_RESPONSE", "FAILED",
	))
	if first.accept(duplicate) {
		t.Fatal("duplicate RUN_RESPONSE was accepted after terminal completion")
	}
	first.mu.Lock()
	terminalSignals := first.terminalSignals
	first.mu.Unlock()
	if terminalSignals != 1 {
		t.Fatalf("duplicate terminal signal count = %d, want one", terminalSignals)
	}
}

func TestTerminalFactoryEventObservationTimeoutAndCloseReleaseStream(t *testing.T) {
	requestStarted := make(chan struct{})
	requestEnded := make(chan struct{})
	server := newTerminalObservationTestServer(t, 0, func(_ http.ResponseWriter, r *http.Request) {
		close(requestStarted)
		<-r.Context().Done()
		close(requestEnded)
	})

	observation, err := openTerminalFactoryEventObservation(server.URL, "timeout-session")
	if err != nil {
		t.Fatalf("open timeout observation: %v", err)
	}
	if _, err := observation.wait(25 * time.Millisecond); err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("timeout error = %v, want bounded wait timeout", err)
	}
	select {
	case <-requestStarted:
	case <-time.After(time.Second):
		t.Fatal("timeout observation request was not established")
	}
	observation.Close()
	select {
	case <-requestEnded:
	case <-time.After(time.Second):
		t.Fatal("closing terminal observation did not cancel the SSE request")
	}
	select {
	case <-observation.done:
	default:
		t.Fatal("closing terminal observation left its reader running")
	}
}

func TestTerminalEventObservationSpineUsesRootProcessAndCanonicalRunResponse(t *testing.T) {
	factoryDir := ScaffoldSingleStepFactory(t, "terminal-event-observation-root-spine")
	WriteAgentConfig(t, factoryDir, "processor", BuildModelWorkerConfig(modelprovider.ProviderCodex, "test-model"))

	providerRelease := make(chan struct{})
	shutdownRelease := make(chan struct{})
	var releaseShutdownOnce sync.Once
	releaseShutdown := func() {
		releaseShutdownOnce.Do(func() { close(shutdownRelease) })
	}
	defer releaseShutdown()
	apiServer := NewProcessAPIServer()
	apiServer.HoldShutdownUntilSignaled(shutdownRelease)
	process := BuildProcess(t, serviceedges.Edges{
		APIServerStarter:      apiServer.Start,
		ProviderCommandRunner: NewGatedSuccessCommandRunner("completed COMPLETE", providerRelease),
	})
	CleanupProcess(t, process)
	inputs := FakeInputs(t.Context(), []string{
		"you", "run",
		"--dir", factoryDir,
		"--continuously",
		"--with-server",
		"--server", "http://127.0.0.1:1",
		"--no-record",
		"--quiet",
	})
	inputs.Input.WorkingDirectory = factoryDir
	home := t.TempDir()
	inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	command := StartProcessCommand(t, process, inputs.Input)
	baseURL := apiServer.WaitForURL(t)
	session := GetDefaultSession(t, baseURL)
	if session.Id == "" {
		t.Fatal("root process returned an empty Factory Session ID")
	}
	WaitForStatus(t, baseURL, functionalServerReadyTimeout, func(status factoryapi.StatusResponse) bool {
		return status.RuntimeStatus != ""
	})

	observation := OpenSessionTerminalFactoryEventObservation(t, baseURL, session.Id)
	eventStream := OpenFactoryEventStreamAt(t, SessionEventsURL(baseURL, session.Id))
	dispatchComplete := make(chan struct {
		event factoryapi.FactoryEvent
		err   error
	}, 1)
	go func() {
		event, err := waitForAcceptedDispatchResponse(eventStream, 15*time.Second)
		dispatchComplete <- struct {
			event factoryapi.FactoryEvent
			err   error
		}{event, err}
	}()
	workName := "canonical terminal event observation"
	submitted := SubmitDefaultSessionWork(t, baseURL, factoryapi.SubmitWorkRequest{
		Name:         &workName,
		WorkTypeName: "task",
		Payload:      json.RawMessage(`{"title":"canonical terminal event observation"}`),
	})
	if !submitted.Accepted {
		t.Fatalf("root public Work submission = %#v, want accepted", submitted)
	}
	close(providerRelease)
	select {
	case result := <-dispatchComplete:
		if result.err != nil {
			t.Fatalf("observe accepted dispatch through Factory Events: %v", result.err)
		}
		if result.event.Context.SessionId != nil &&
			*result.event.Context.SessionId != session.Id &&
			*result.event.Context.SessionId != factorysessions.DefaultSessionID {
			t.Fatalf("dispatch event session ID = %q, want nil, %q, or %q", *result.event.Context.SessionId, session.Id, factorysessions.DefaultSessionID)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("timed out waiting for accepted dispatch event")
	}
	command.cancel()
	event := observation.Wait(15 * time.Second)
	if event.Type != factoryapi.FactoryEventTypeRunResponse {
		t.Fatalf("root terminal event type = %q, want RUN_RESPONSE", event.Type)
	}
	if event.Context.SessionId != nil && *event.Context.SessionId != session.Id {
		t.Fatalf("root terminal event session ID = %#v, want nil or %q", event.Context.SessionId, session.Id)
	}
	payload, err := event.Payload.AsRunResponseEventPayload()
	if err != nil {
		t.Fatalf("decode root terminal RUN_RESPONSE: %v", err)
	}
	if payload.State == nil || *payload.State != factoryapi.FactoryStateCompleted {
		t.Fatalf("root terminal RUN_RESPONSE state = %#v, want COMPLETED", payload.State)
	}

	completedSession := GetDefaultSession(t, baseURL)
	if completedSession.Runtime.Status != factoryapi.FactorySessionStatusFINISHED {
		t.Fatalf("root completed session status = %q, want FINISHED", completedSession.Runtime.Status)
	}
	work := ListDefaultSessionWork(t, baseURL)
	if len(work.Results) != 1 || work.Results[0].State == nil || work.Results[0].State.Name != "complete" {
		t.Fatalf("root completed work = %#v, want one complete Work item", work.Results)
	}

	releaseShutdown()
	select {
	case <-command.Done():
	case <-time.After(15 * time.Second):
		t.Fatal("root Process.Execute did not finish after API shutdown release")
	}
	if err := command.Err(); err != nil {
		t.Fatalf("root Process.Execute error = %v", err)
	}
}

func waitForAcceptedDispatchResponse(stream *FactoryEventStream, timeout time.Duration) (factoryapi.FactoryEvent, error) {
	if stream == nil {
		return factoryapi.FactoryEvent{}, fmt.Errorf("Factory Event stream is nil")
	}
	if timeout <= 0 {
		return factoryapi.FactoryEvent{}, fmt.Errorf("dispatch event timeout must be positive")
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	var seen []string
	for {
		select {
		case event := <-stream.events:
			seen = append(seen, terminalObservationEventSummary(event))
			if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
				continue
			}
			payload, err := event.Payload.AsDispatchResponseEventPayload()
			if err != nil {
				return factoryapi.FactoryEvent{}, fmt.Errorf("decode dispatch response event: %w", err)
			}
			if payload.Outcome == factoryapi.WorkOutcomeAccepted {
				return event, nil
			}
		case err := <-stream.errs:
			return factoryapi.FactoryEvent{}, err
		case <-stream.done:
			return factoryapi.FactoryEvent{}, fmt.Errorf("Factory Event stream closed before accepted dispatch response")
		case <-timer.C:
			return factoryapi.FactoryEvent{}, fmt.Errorf("timed out waiting for accepted dispatch response; observed events=%v", seen)
		}
	}
}

func terminalObservationEventSummary(event factoryapi.FactoryEvent) string {
	if event.Type == factoryapi.FactoryEventTypeDispatchResponse {
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err == nil {
			errorText := ""
			if payload.Error != nil {
				errorText = *payload.Error
			}
			return fmt.Sprintf("%s(outcome=%s,error=%q)", event.Type, payload.Outcome, errorText)
		}
	}
	return string(event.Type)
}

func TestTerminalFactoryEventObservationRejectsInvalidStreamResponses(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		contentType string
		retained    string
		want        string
	}{
		{name: "http reject", status: http.StatusServiceUnavailable, contentType: "text/plain", retained: "0", want: "status = 503"},
		{name: "wrong content type", status: http.StatusOK, contentType: "application/json", retained: "0", want: "content type"},
		{name: "missing retained count", status: http.StatusOK, contentType: "text/event-stream", want: "omitted"},
		{name: "invalid retained count", status: http.StatusOK, contentType: "text/event-stream", retained: "many", want: "non-negative integer"},
		{name: "negative retained count", status: http.StatusOK, contentType: "text/event-stream", retained: "-1", want: "non-negative integer"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", test.contentType)
				if test.retained != "" {
					w.Header().Set(factorysessionshttp.SessionEventStreamRetainedCountHeader, test.retained)
				}
				w.WriteHeader(test.status)
				_, _ = io.WriteString(w, "invalid stream response")
			}))
			defer server.Close()

			_, err := openTerminalFactoryEventObservation(server.URL, "invalid-response-session")
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("open error = %v, want substring %q", err, test.want)
			}
		})
	}

	if _, err := openTerminalFactoryEventObservation("", "session"); err == nil || !strings.Contains(err.Error(), "base URL is empty") {
		t.Fatalf("empty base URL error = %v, want validation error", err)
	}
	if _, err := openTerminalFactoryEventObservation("http://127.0.0.1:1", ""); err == nil || !strings.Contains(err.Error(), "session ID is empty") {
		t.Fatalf("empty session ID error = %v, want validation error", err)
	}
}

func TestTerminalFactoryEventObservationFailsSafelyForMalformedAndPrematureStreams(t *testing.T) {
	tests := []struct {
		name          string
		retainedCount int
		writeResponse func(http.ResponseWriter)
		wantError     string
	}{
		{
			name:          "malformed JSON",
			writeResponse: func(w http.ResponseWriter) { _, _ = io.WriteString(w, "data: {not-json}\n\n") },
			wantError:     "decode Factory Event SSE payload",
		},
		{
			name:          "named SSE event",
			writeResponse: func(w http.ResponseWriter) { _, _ = io.WriteString(w, "event: RUN_RESPONSE\n\n") },
			wantError:     "named event line",
		},
		{
			name:          "premature retained history close",
			retainedCount: 1,
			wantError:     "retained cursor boundary",
		},
		{
			name:      "premature live close",
			wantError: "closed before RUN_RESPONSE",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := newTerminalObservationTestServer(t, test.retainedCount, func(w http.ResponseWriter, _ *http.Request) {
				if test.writeResponse != nil {
					test.writeResponse(w)
				}
			})
			observation, err := openTerminalFactoryEventObservation(server.URL, "malformed-session")
			if err != nil {
				t.Fatalf("open observation: %v", err)
			}
			defer observation.Close()
			_, err = observation.wait(time.Second)
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("observation error = %v, want substring %q", err, test.wantError)
			}
		})
	}
}

func newTerminalObservationTestServer(
	t *testing.T,
	retainedCount int,
	handler func(http.ResponseWriter, *http.Request),
) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set(
			factorysessionshttp.SessionEventStreamRetainedCountHeader,
			strconv.Itoa(retainedCount),
		)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		} else {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		handler(w, r)
	}))
}

func writeTerminalObservationSSE(t *testing.T, w http.ResponseWriter, payload []byte) {
	t.Helper()
	if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
		return
	}
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}

func terminalObservationEventJSON(
	id string,
	sessionID string,
	sequence int,
	sessionSequence int,
	eventType string,
	state string,
) []byte {
	payload := map[string]any{}
	if state != "" {
		payload["state"] = state
	}
	value, _ := json.Marshal(map[string]any{
		"context": map[string]any{
			"eventTime":       "2026-08-30T12:00:00Z",
			"sequence":        sequence,
			"sessionId":       sessionID,
			"sessionSequence": sessionSequence,
			"tick":            sequence,
		},
		"id":            id,
		"payload":       payload,
		"schemaVersion": "agent-factory.event.v1",
		"type":          eventType,
	})
	return value
}

func decodeTerminalObservationEvent(t *testing.T, raw []byte) factoryapi.FactoryEvent {
	t.Helper()
	var event factoryapi.FactoryEvent
	if err := json.Unmarshal(raw, &event); err != nil {
		t.Fatalf("decode test Factory Event: %v", err)
	}
	return event
}
