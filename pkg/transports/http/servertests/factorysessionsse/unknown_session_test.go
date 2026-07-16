package factorysessionsse

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const (
	factorySessionSSEUnknownSessionID       = "b08-sse-unknown-session"
	factorySessionSSEDefaultFallbackEventID = "b08-sse-default-session/only-event"
)

func TestFactorySessionSSEUnknownSession_ReturnsTypedNotFoundWithinBoundedTimeout(t *testing.T) {
	fixture := NewFactorySessionSSEFixture(t)
	server := httptest.NewServer(newAPITestServer(fixture.RootMockFactory()).Handler())
	defer server.Close()

	harness := NewFactorySessionSSEHarness(t, 2*time.Second)
	checkpoint := FactorySessionSSECheckpoint{
		AfterEventID:  factorySessionSSEUnknownCursorEventID,
		AfterSequence: factorySessionSSESessionSequencePointer(7),
	}
	stream, err := harness.TryOpenFromCheckpoint(
		context.Background(),
		server.URL,
		factorySessionSSEUnknownSessionID,
		checkpoint,
	)
	if stream != nil {
		stream.Close()
		t.Fatal("unknown session unexpectedly opened an SSE stream")
	}
	var openErr *FactorySessionSSEOpenError
	if !errors.As(err, &openErr) {
		t.Fatalf("error = %T %v, want FactorySessionSSEOpenError", err, err)
	}
	if openErr.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", openErr.StatusCode)
	}
	if openErr.SessionID != factorySessionSSEUnknownSessionID || openErr.Checkpoint.AfterEventID != checkpoint.AfterEventID {
		t.Fatalf("open error context = %#v, want unknown session and supplied checkpoint", openErr)
	}
	if openErr.Response.Code != factoryapi.ErrorResponseCodeNOTFOUND {
		t.Fatalf("code = %q, want NOT_FOUND", openErr.Response.Code)
	}
	if !strings.Contains(openErr.Response.Message, "factory session not found") {
		t.Fatalf("message = %q, want factory session not found guidance", openErr.Response.Message)
	}
}

func TestFactorySessionSSEUnknownSession_NeverFallsBackToDefaultOrOtherSession(t *testing.T) {
	fixture := NewFactorySessionSSEFixture(t)
	defaultSessionID := "~default"
	root := fixture.RootMockFactory()
	root.SessionFactories[defaultSessionID] = &testutil.MockFactory{
		FactoryEventStream: &interfaces.FactoryEventStream{
			StreamGenerationID:  "b08-sse-default-stream-gen",
			BackendScopeID:      "b08-sse-default-backend-scope",
			LogicalSessionKeyID: "b08-sse-default-logical-key",
			FactorySessionID:    defaultSessionID,
			History: []interfaces.FactoryEvent{
				testutil.FactoryEvent(t, testAPIFactoryEvent(
					t,
					factoryapi.FactoryEventTypeRunRequest,
					factorySessionSSEDefaultFallbackEventID,
					factoryapi.FactoryEventContext{
						Tick:            0,
						Sequence:        0,
						SessionSequence: factorySessionSSESessionSequencePointer(0),
						EventTime:       factorySessionSSEFixtureEventTime,
						SessionId:       &defaultSessionID,
					},
					factoryapi.RunRequestEventPayload{
						RecordedAt: factorySessionSSEFixtureEventTime,
						Factory:    factoryapi.Factory{Name: "default-session-factory"},
					},
				)),
			},
			Events: make(chan interfaces.FactoryEvent, 1),
		},
	}

	server := httptest.NewServer(newAPITestServer(root).Handler())
	defer server.Close()

	harness := NewFactorySessionSSEHarness(t, 2*time.Second)
	resp := harness.GetSessionEvents(server.URL, factorySessionSSEUnknownSessionID, "", "")
	defer func() { _ = resp.Body.Close() }()

	body := readFactorySessionSSEUnknownSessionErrorResponseBody(t, resp)
	assertFactorySessionSSEUnknownSessionErrorPayload(t, body)
	assertFactorySessionSSEBodyDoesNotReplayRetainedHistory(t, body, fixture.Retained)
	if strings.Contains(string(body), factorySessionSSEDefaultFallbackEventID) {
		t.Fatalf("unknown-session response replayed default session event id %q", factorySessionSSEDefaultFallbackEventID)
	}
}

func TestFactorySessionSSEUnknownSession_JSONProbeClassifiesUnknownSessionDistinctFromCursorStale(t *testing.T) {
	fixture := NewFactorySessionSSEFixture(t)
	server := httptest.NewServer(newAPITestServer(fixture.RootMockFactory()).Handler())
	defer server.Close()

	harness := NewFactorySessionSSEHarness(t, 2*time.Second)
	recovery, resp := harness.ProbeRecovery(
		server.URL,
		factorySessionSSEUnknownSessionID,
		"after_event_id="+factorySessionSSEUnknownCursorEventID,
	)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("recovery probe status = %d, want 200: %s", resp.StatusCode, readBody(t, resp))
	}
	if recovery.FactorySessionId != factorySessionSSEUnknownSessionID {
		t.Fatalf("factorySessionId = %q, want %q", recovery.FactorySessionId, factorySessionSSEUnknownSessionID)
	}
	if recovery.Outcome != factoryapi.FactorySessionEventStreamRecoveryOutcomeUNKNOWNSESSION {
		t.Fatalf("outcome = %q, want UNKNOWN_SESSION", recovery.Outcome)
	}
	if recovery.Retry.OmitAfterEventId || recovery.Retry.OmitAfterSequence {
		t.Fatalf("retry = %#v, want both omit flags false for unknown session", recovery.Retry)
	}
	if recovery.Outcome == factoryapi.FactorySessionEventStreamRecoveryOutcomeCURSORSTALE {
		t.Fatal("unknown session probe must not classify as CURSOR_STALE")
	}
}

func TestFactorySessionSSEBoundedTimeoutReportsSelectorCheckpointAndLastFrame(t *testing.T) {
	fixture := NewFactorySessionSSEFixture(t)
	server := httptest.NewServer(newAPITestServer(fixture.RootMockFactory()).Handler())
	defer server.Close()

	harness := NewFactorySessionSSEHarness(t, 2*time.Second)
	checkpoint := FactorySessionSSECheckpoint{AfterEventID: fixture.Retained[0].Id}
	stream := harness.OpenFromCheckpoint(server.URL, fixture.SessionID, checkpoint)
	defer stream.Close()
	lastEvent := stream.ReadEvents(len(fixture.Retained) - 1)[1]

	bound := 30 * time.Millisecond
	_, err := stream.TryReadNextEvent(bound)
	var readErr *FactorySessionSSEReadError
	if !errors.As(err, &readErr) {
		t.Fatalf("error = %T %v, want FactorySessionSSEReadError", err, err)
	}
	if readErr.Outcome != FactorySessionSSEReadOutcomeWaitingTimeout {
		t.Fatalf("outcome = %q, want WAITING_TIMEOUT", readErr.Outcome)
	}
	if readErr.SessionID != fixture.SessionID || readErr.Checkpoint.AfterEventID != checkpoint.AfterEventID {
		t.Fatalf("read error context = %#v, want fixture session and supplied checkpoint", readErr)
	}
	if readErr.ElapsedBound != bound {
		t.Fatalf("elapsed bound = %s, want %s", readErr.ElapsedBound, bound)
	}
	if readErr.LastValidFrame == nil || readErr.LastValidFrame.FactoryEvent == nil {
		t.Fatalf("last valid frame = %#v, want decoded Factory Event", readErr.LastValidFrame)
	}
	if readErr.LastValidFrame.FactoryEvent.Id != lastEvent.Id {
		t.Fatalf("last valid event id = %q, want %q", readErr.LastValidFrame.FactoryEvent.Id, lastEvent.Id)
	}
	for _, want := range []string{fixture.SessionID, checkpoint.AfterEventID, lastEvent.Id, bound.String()} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want diagnostic context %q", err, want)
		}
	}
}

func TestFactorySessionSSEBoundedTimeoutExplicitlyReportsNoValidFrame(t *testing.T) {
	reader, writer := io.Pipe()
	defer func() { _ = reader.Close() }()
	defer func() { _ = writer.Close() }()
	stream := &FactorySessionSSEStream{
		t:         t,
		timeout:   2 * time.Second,
		reader:    bufio.NewReader(reader),
		ctx:       context.Background(),
		sessionID: factorySessionSSEFixtureSessionID,
	}

	_, err := stream.TryReadNextEvent(30 * time.Millisecond)
	var readErr *FactorySessionSSEReadError
	if !errors.As(err, &readErr) {
		t.Fatalf("error = %T %v, want FactorySessionSSEReadError", err, err)
	}
	if readErr.LastValidFrame != nil {
		t.Fatalf("last valid frame = %#v, want none", readErr.LastValidFrame)
	}
	if !strings.Contains(err.Error(), "no valid frame observed") {
		t.Fatalf("error = %q, want explicit no-valid-frame diagnostic", err)
	}
}

func TestFactorySessionSSECallerCancellationIsPromptAndTerminal(t *testing.T) {
	for attempt := 0; attempt < 10; attempt++ {
		t.Run(fmt.Sprintf("attempt-%02d", attempt), func(t *testing.T) {
			fixture := NewFactorySessionSSEFixture(t)
			server := httptest.NewServer(newAPITestServer(fixture.RootMockFactory()).Handler())
			defer server.Close()

			harness := NewFactorySessionSSEHarness(t, 2*time.Second)
			ctx, cancel := context.WithCancel(context.Background())
			stream, err := harness.TryOpenFromCheckpoint(
				ctx,
				server.URL,
				fixture.SessionID,
				FactorySessionSSECheckpoint{},
			)
			if err != nil {
				t.Fatalf("open session stream: %v", err)
			}
			defer stream.Close()
			lastEvent := stream.ReadEvents(len(fixture.Retained))[len(fixture.Retained)-1]

			cancelTimer := time.AfterFunc(25*time.Millisecond, cancel)
			started := time.Now()
			_, err = stream.TryReadNextEvent(2 * time.Second)
			cancelTimer.Stop()
			if elapsed := time.Since(started); elapsed >= 500*time.Millisecond {
				t.Fatalf("canceled read elapsed = %s, want prompt termination", elapsed)
			}
			assertFactorySessionSSECanceledRead(t, err, fixture.SessionID, lastEvent.Id)

			fixture.PublishLive(fixture.LiveDispatchEvent(t))
			_, err = stream.TryReadNextEvent(100 * time.Millisecond)
			assertFactorySessionSSECanceledRead(t, err, fixture.SessionID, lastEvent.Id)
		})
	}
}

func assertFactorySessionSSECanceledRead(t *testing.T, err error, sessionID, lastEventID string) {
	t.Helper()

	var readErr *FactorySessionSSEReadError
	if !errors.As(err, &readErr) {
		t.Fatalf("error = %T %v, want FactorySessionSSEReadError", err, err)
	}
	if readErr.Outcome != FactorySessionSSEReadOutcomeCallerCanceled {
		t.Fatalf("outcome = %q, want CALLER_CANCELED", readErr.Outcome)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if readErr.SessionID != sessionID {
		t.Fatalf("session id = %q, want %q", readErr.SessionID, sessionID)
	}
	if readErr.LastValidFrame == nil || readErr.LastValidFrame.FactoryEvent == nil {
		t.Fatalf("last valid frame = %#v, want decoded Factory Event", readErr.LastValidFrame)
	}
	if readErr.LastValidFrame.FactoryEvent.Id != lastEventID {
		t.Fatalf("last valid event id = %q, want %q", readErr.LastValidFrame.FactoryEvent.Id, lastEventID)
	}
}

func readFactorySessionSSEUnknownSessionErrorResponseBody(t *testing.T, resp *http.Response) []byte {
	t.Helper()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", resp.StatusCode, readBody(t, resp))
	}
	if got := resp.Header.Get("Content-Type"); strings.Contains(got, "text/event-stream") {
		t.Fatalf("Content-Type = %q, want typed error response instead of SSE stream", got)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read error response body: %v", err)
	}
	return body
}

func assertFactorySessionSSEUnknownSessionErrorPayload(t *testing.T, body []byte) {
	t.Helper()

	var errResp factoryapi.ErrorResponse
	if err := json.Unmarshal(body, &errResp); err != nil {
		t.Fatalf("decode error response: %v: %s", err, string(body))
	}
	if errResp.Code != factoryapi.ErrorResponseCodeNOTFOUND {
		t.Fatalf("code = %q, want NOT_FOUND", errResp.Code)
	}
	if !strings.Contains(errResp.Message, "factory session not found") {
		t.Fatalf("message = %q, want factory session not found guidance", errResp.Message)
	}
}
