package apiserver_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const factorySessionSSEUnknownCursorEventID = "b08-sse-fixture/unknown-retained-event"

func TestFactorySessionSSEInvalidCursor_UnknownAfterEventIDReturnsTypedErrorNotFullHistory(t *testing.T) {
	fixture := NewFactorySessionSSEFixture(t)
	server := httptest.NewServer(newAPITestServer(fixture.RootMockFactory()).Handler())
	defer server.Close()

	harness := NewFactorySessionSSEHarness(t, 2*time.Second)
	query := "after_event_id=" + url.QueryEscape(factorySessionSSEUnknownCursorEventID)
	resp := harness.GetSessionEvents(server.URL, fixture.SessionID, query, "")
	defer resp.Body.Close()

	body := readFactorySessionSSEErrorResponseBody(t, resp)
	assertFactorySessionSSEInvalidCursorErrorPayload(t, body)
	assertFactorySessionSSEBodyDoesNotReplayRetainedHistory(t, body, fixture.Retained)
}

func TestFactorySessionSSEInvalidCursor_UnknownAfterSequenceReturnsTypedErrorNotFullHistory(t *testing.T) {
	fixture := NewFactorySessionSSEFixture(t)
	server := httptest.NewServer(newAPITestServer(fixture.RootMockFactory()).Handler())
	defer server.Close()

	harness := NewFactorySessionSSEHarness(t, 2*time.Second)
	resp := harness.GetSessionEvents(server.URL, fixture.SessionID, "after_sequence=999", "")
	defer resp.Body.Close()

	body := readFactorySessionSSEErrorResponseBody(t, resp)
	assertFactorySessionSSEInvalidCursorErrorPayload(t, body)
	assertFactorySessionSSEBodyDoesNotReplayRetainedHistory(t, body, fixture.Retained)
}

func TestFactorySessionSSEInvalidCursor_JSONProbeClassifiesStaleCursorWithOmitGuidance(t *testing.T) {
	fixture := NewFactorySessionSSEFixture(t)
	server := httptest.NewServer(newAPITestServer(fixture.RootMockFactory()).Handler())
	defer server.Close()

	harness := NewFactorySessionSSEHarness(t, 2*time.Second)
	query := "after_event_id=" + url.QueryEscape(factorySessionSSEUnknownCursorEventID)
	recovery, resp := harness.ProbeRecovery(server.URL, fixture.SessionID, query)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("recovery probe status = %d, want 200: %s", resp.StatusCode, readBody(t, resp))
	}
	if recovery.FactorySessionId != fixture.SessionID {
		t.Fatalf("factorySessionId = %q, want %q", recovery.FactorySessionId, fixture.SessionID)
	}
	if recovery.Outcome != factoryapi.FactorySessionEventStreamRecoveryOutcomeCURSORSTALE {
		t.Fatalf("outcome = %q, want CURSOR_STALE", recovery.Outcome)
	}
	if !recovery.Retry.OmitAfterEventId || !recovery.Retry.OmitAfterSequence {
		t.Fatalf("retry = %#v, want both omit flags true for stale cursor recovery", recovery.Retry)
	}
}

func TestFactorySessionSSEInvalidCursor_JSONProbeValidCursorReturnsStreamReady(t *testing.T) {
	fixture := NewFactorySessionSSEFixture(t)
	server := httptest.NewServer(newAPITestServer(fixture.RootMockFactory()).Handler())
	defer server.Close()

	harness := NewFactorySessionSSEHarness(t, 2*time.Second)
	query := "after_event_id=" + url.QueryEscape(fixture.Retained[1].Id)
	recovery, resp := harness.ProbeRecovery(server.URL, fixture.SessionID, query)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("recovery probe status = %d, want 200: %s", resp.StatusCode, readBody(t, resp))
	}
	if recovery.FactorySessionId != fixture.SessionID {
		t.Fatalf("factorySessionId = %q, want %q", recovery.FactorySessionId, fixture.SessionID)
	}
	if recovery.Outcome != factoryapi.FactorySessionEventStreamRecoveryOutcomeSTREAMREADY {
		t.Fatalf("outcome = %q, want STREAM_READY", recovery.Outcome)
	}
	if recovery.Retry.OmitAfterEventId || recovery.Retry.OmitAfterSequence {
		t.Fatalf("retry = %#v, want both omit flags false for reusable cursor", recovery.Retry)
	}
}

func readFactorySessionSSEErrorResponseBody(t *testing.T, resp *http.Response) []byte {
	t.Helper()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", resp.StatusCode, readBody(t, resp))
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

func assertFactorySessionSSEInvalidCursorErrorPayload(t *testing.T, body []byte) {
	t.Helper()

	var errResp factoryapi.ErrorResponse
	if err := json.Unmarshal(body, &errResp); err != nil {
		t.Fatalf("decode error response: %v: %s", err, string(body))
	}
	if errResp.Code != factoryapi.ErrorResponseCodeBADREQUEST {
		t.Fatalf("code = %q, want BAD_REQUEST", errResp.Code)
	}
	if !strings.Contains(errResp.Message, "invalid event reconnect cursor") {
		t.Fatalf("message = %q, want invalid event reconnect cursor guidance", errResp.Message)
	}
}

func assertFactorySessionSSEBodyDoesNotReplayRetainedHistory(
	t *testing.T,
	body []byte,
	retained []factoryapi.FactoryEvent,
) {
	t.Helper()

	bodyText := string(body)
	for _, event := range retained {
		if strings.Contains(bodyText, event.Id) {
			t.Fatalf("invalid cursor response replayed retained event id %q", event.Id)
		}
	}
	if strings.Contains(bodyText, "data: ") {
		t.Fatal("invalid cursor response contained SSE data frames")
	}
}
