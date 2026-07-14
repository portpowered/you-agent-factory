package apiserver_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
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
	resp := harness.GetSessionEvents(server.URL, factorySessionSSEUnknownSessionID, "", "")
	defer resp.Body.Close()

	body := readFactorySessionSSEUnknownSessionErrorResponseBody(t, resp)
	assertFactorySessionSSEUnknownSessionErrorPayload(t, body)
	assertFactorySessionSSEBodyDoesNotReplayRetainedHistory(t, body, fixture.Retained)
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
			History: []factoryapi.FactoryEvent{
				testAPIFactoryEvent(
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
				),
			},
			Events: make(chan factoryapi.FactoryEvent, 1),
		},
	}

	server := httptest.NewServer(newAPITestServer(root).Handler())
	defer server.Close()

	harness := NewFactorySessionSSEHarness(t, 2*time.Second)
	resp := harness.GetSessionEvents(server.URL, factorySessionSSEUnknownSessionID, "", "")
	defer resp.Body.Close()

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
	defer resp.Body.Close()

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
