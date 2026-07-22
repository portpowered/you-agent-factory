package factorysessionsse

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
	server := httptest.NewServer(newAPITestServer(fixture.WorkAPI()).Handler())
	defer server.Close()

	harness := newFactorySessionSSEHarness(t, 2*time.Second)
	query := "after_event_id=" + url.QueryEscape(factorySessionSSEUnknownCursorEventID)
	resp := harness.GetSessionEvents(server.URL, fixture.SessionID, query, "")
	defer closeFactorySessionSSEResponse(t, resp)

	body := readFactorySessionSSEErrorResponseBody(t, resp)
	assertFactorySessionSSEInvalidCursorErrorPayload(t, body)
	assertFactorySessionSSEBodyDoesNotReplayRetainedHistory(t, body, fixture.Retained)
}

func TestFactorySessionSSEInvalidCursor_UnknownAfterSequenceReturnsTypedErrorNotFullHistory(t *testing.T) {
	fixture := NewFactorySessionSSEFixture(t)
	server := httptest.NewServer(newAPITestServer(fixture.WorkAPI()).Handler())
	defer server.Close()

	harness := newFactorySessionSSEHarness(t, 2*time.Second)
	resp := harness.GetSessionEvents(server.URL, fixture.SessionID, "after_sequence=999", "")
	defer closeFactorySessionSSEResponse(t, resp)

	body := readFactorySessionSSEErrorResponseBody(t, resp)
	assertFactorySessionSSEInvalidCursorErrorPayload(t, body)
	assertFactorySessionSSEBodyDoesNotReplayRetainedHistory(t, body, fixture.Retained)
}

func TestFactorySessionSSEInvalidCursor_JSONProbeClassifiesStaleCursorWithOmitGuidance(t *testing.T) {
	fixture := NewFactorySessionSSEFixture(t)
	server := httptest.NewServer(newAPITestServer(fixture.WorkAPI()).Handler())
	defer server.Close()

	harness := newFactorySessionSSEHarness(t, 2*time.Second)
	query := "after_event_id=" + url.QueryEscape(factorySessionSSEUnknownCursorEventID)
	recovery, resp := harness.ProbeRecovery(server.URL, fixture.SessionID, query)
	defer closeFactorySessionSSEResponse(t, resp)

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
	server := httptest.NewServer(newAPITestServer(fixture.WorkAPI()).Handler())
	defer server.Close()

	harness := newFactorySessionSSEHarness(t, 2*time.Second)
	query := "after_event_id=" + url.QueryEscape(fixture.Retained[1].Id)
	recovery, resp := harness.ProbeRecovery(server.URL, fixture.SessionID, query)
	defer closeFactorySessionSSEResponse(t, resp)

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

func TestFactorySessionSSEInvalidCursor_StreamGenerationChangeInvalidatesAndOmitsPriorCheckpoint(t *testing.T) {
	fixture := NewFactorySessionSSEFixture(t)
	root := fixture.WorkAPI()
	server := httptest.NewServer(newAPITestServer(root).Handler())
	defer server.Close()

	harness := newFactorySessionSSEHarness(t, 2*time.Second)
	priorStream := harness.Open(server.URL, fixture.SessionID, "")
	priorStream.ReadEvents(2)
	priorIdentity := priorStream.Identity
	priorStream.Close()
	if priorIdentity != (FactorySessionSSEStreamIdentity{
		BackendScopeID:      factorySessionSSEFixtureBackendScopeID,
		LogicalSessionKeyID: factorySessionSSEFixtureLogicalSessionKey,
		FactorySessionID:    fixture.SessionID,
		StreamGenerationID:  factorySessionSSEFixtureStreamGenerationID,
	}) {
		t.Fatalf("prior identity = %#v, want complete original generation identity", priorIdentity)
	}

	checkpointSequence := 1
	priorCheckpoint := FactorySessionSSECheckpoint{
		AfterEventID:  fixture.Retained[1].Id,
		AfterSequence: &checkpointSequence,
	}
	currentRetained := fixture.ReplaceStreamGeneration(t, root)

	recovery, resp := harness.ProbeRecoveryFromCheckpoint(server.URL, fixture.SessionID, priorCheckpoint)
	defer closeFactorySessionSSEResponse(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("recovery probe status = %d, want 200: %s", resp.StatusCode, readBody(t, resp))
	}
	if recovery.Outcome != factoryapi.FactorySessionEventStreamRecoveryOutcomeCURSORSTALE {
		t.Fatalf("outcome = %q, want CURSOR_STALE", recovery.Outcome)
	}
	if !recovery.Retry.OmitAfterEventId || !recovery.Retry.OmitAfterSequence {
		t.Fatalf("retry = %#v, want both prior cursor forms omitted", recovery.Retry)
	}

	retryCheckpoint := priorCheckpoint.ApplyRecovery(recovery)
	if retryCheckpoint.AfterEventID != "" || retryCheckpoint.AfterSequence != nil {
		t.Fatalf("retry checkpoint = %#v, want both stale cursors omitted", retryCheckpoint)
	}
	currentStream := harness.OpenFromCheckpoint(server.URL, fixture.SessionID, retryCheckpoint)
	defer currentStream.Close()
	currentEvents := currentStream.ReadEvents(len(currentRetained))
	if currentEvents[0].Id != currentRetained[0].Id {
		t.Fatalf("first recovered event id = %q, want current retained boundary %q", currentEvents[0].Id, currentRetained[0].Id)
	}

	currentIdentity := currentStream.Identity
	if currentIdentity.BackendScopeID != priorIdentity.BackendScopeID ||
		currentIdentity.LogicalSessionKeyID != priorIdentity.LogicalSessionKeyID ||
		currentIdentity.FactorySessionID != priorIdentity.FactorySessionID {
		t.Fatalf("current identity = %#v, want backend/logical/Factory Session identity retained from %#v", currentIdentity, priorIdentity)
	}
	if currentIdentity.StreamGenerationID == priorIdentity.StreamGenerationID {
		t.Fatalf("current stream generation = %q, want change from prior identity %#v", currentIdentity.StreamGenerationID, priorIdentity)
	}
	if currentIdentity.StreamGenerationID != factorySessionSSEFixtureNextGenerationID {
		t.Fatalf("current stream generation = %q, want %q", currentIdentity.StreamGenerationID, factorySessionSSEFixtureNextGenerationID)
	}
}

func closeFactorySessionSSEResponse(t *testing.T, resp *http.Response) {
	t.Helper()
	if err := resp.Body.Close(); err != nil {
		t.Errorf("close session events response body: %v", err)
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
