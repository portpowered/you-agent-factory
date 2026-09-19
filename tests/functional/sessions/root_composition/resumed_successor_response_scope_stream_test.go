//go:build factoryartifact

package root_composition_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func readOpenResumedResponseEvents(
	t testing.TB,
	baseURL, sessionID string,
) []factoryapi.FactoryResponseEvent {
	t.Helper()
	stream := support.OpenFactoryResponseEventStreamAt(
		t,
		support.SessionResponseEventsURL(baseURL, sessionID),
	)
	defer stream.Close()
	retainedCount := stream.RetainedFrameCount
	if !stream.HasRetainedFrameCount {
		t.Fatal("resumed response-event stream omitted its retained-prefix count")
	}
	events := make([]factoryapi.FactoryResponseEvent, 0, retainedCount)
	for index := 0; index < retainedCount; index++ {
		events = append(events, stream.NextFrame(resumedResponseScopeStreamTimeout).Event)
	}
	return events
}

func readClosedResumedResponseEvents(t testing.TB, baseURL, sessionID string) []factoryapi.FactoryResponseEvent {
	t.Helper()
	stream := support.OpenFactoryResponseEventStreamAt(
		t,
		support.SessionResponseEventsURL(baseURL, sessionID),
	)
	defer stream.Close()
	var events []factoryapi.FactoryResponseEvent
	for len(events) < 4096 {
		result := stream.TryNextFrameResult(resumedResponseScopeStreamTimeout)
		switch result.Outcome {
		case support.FactoryResponseEventStreamOutcomeFrame:
			events = append(events, result.Frame.Event)
		case support.FactoryResponseEventStreamOutcomeEOF:
			return events
		case support.FactoryResponseEventStreamOutcomeTimeout:
			t.Fatalf("terminal resumed response-event stream did not close after %d frames", len(events))
		default:
			t.Fatalf("terminal resumed response-event stream failed: %s", result.Diagnostic())
		}
	}
	t.Fatal("terminal resumed response-event stream exceeded 4096 frames")
	return nil
}

func resumedResponseScopeCursor(event factoryapi.FactoryEvent) support.FactoryEventReadCursor {
	sequence := support.ReconnectSequenceForFactoryEvent(event)
	return support.FactoryEventReadCursor{AfterEventID: event.Id, AfterSequence: &sequence}
}

func lastResumedResponseSequence(events []factoryapi.FactoryResponseEvent) int64 {
	var last int64
	for _, event := range events {
		if event.Sequence > last {
			last = event.Sequence
		}
	}
	return last
}

func resumedResponseScopeWork(workID, name, payloadMarker string) factoryapi.Work {
	return resumedResponseScopeWorkOfType(resumedResponseScopeWorkType, workID, name, payloadMarker)
}

func resumedResponseScopeWorkOfType(workType, workID, name, payloadMarker string) factoryapi.Work {
	item := factoryapi.Work{
		Name:         name,
		WorkTypeName: &workType,
		Payload:      map[string]any{"marker": payloadMarker},
	}
	if workID != "" {
		item.WorkId = &workID
	}
	return item
}

func resumedResponseScopeGeneratedWorkID(ledger resumedResponseScopeLedger, workType string) string {
	return fmt.Sprintf("work-%s-%d", workType, ledger.historicalWorkMaxima[workType]+1)
}

func assertReservedWorkReadable(t testing.TB, baseURL, sessionID string, reserved factoryapi.Work) {
	t.Helper()
	got := support.GetJSON[factoryapi.Work](
		t,
		support.SessionWorkURL(baseURL, sessionID, "/work/"+url.PathEscape(resumedResponseScopeReservedWorkID)),
	)
	if support.StringPointerValue(got.WorkId) != resumedResponseScopeReservedWorkID {
		t.Fatalf("reserved historical Work ID = %q, want %q", support.StringPointerValue(got.WorkId), resumedResponseScopeReservedWorkID)
	}
	if support.StringPointerValue(got.WorkTypeName) != support.StringPointerValue(reserved.WorkTypeName) {
		t.Fatalf("reserved historical Work type = %q, want %q", support.StringPointerValue(got.WorkTypeName), support.StringPointerValue(reserved.WorkTypeName))
	}
}

func putResumedWorkRequest(t testing.TB, baseURL, sessionID string, request factoryapi.WorkRequest) (int, []byte) {
	t.Helper()
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal resumed Work request: %v", err)
	}
	endpoint := support.SessionWorkURL(
		baseURL,
		sessionID,
		"/work-requests/"+url.PathEscape(request.RequestId),
	)
	httpRequest, err := http.NewRequestWithContext(t.Context(), http.MethodPut, endpoint, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build resumed Work request: %v", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(httpRequest)
	if err != nil {
		t.Fatalf("PUT resumed Work request: %v", err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read resumed Work request response: %v", err)
	}
	return response.StatusCode, responseBody
}

func resumedInlineJSON(t testing.TB, request factoryapi.WorkRequest) string {
	t.Helper()
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal CLI resumed Work request: %v", err)
	}
	return string(encoded)
}

func assertWorkRequestConflict(t testing.TB, status int, body []byte, boundary string) {
	t.Helper()
	if status != http.StatusConflict {
		t.Fatalf("%s Work conflict status = %d, want 409: %s", boundary, status, strings.TrimSpace(string(body)))
	}
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("decode %s Work conflict: %v", boundary, err)
	}
	if response.Code != factoryapi.ErrorResponseCodeCONFLICT || response.Family != factoryapi.ErrorFamilyConflict {
		t.Fatalf("%s Work conflict = %#v, want CONFLICT/CONFLICT", boundary, response)
	}
	if strings.Contains(string(body), resumedResponseScopeSecretMarker) {
		t.Fatalf("%s Work conflict echoed untrusted payload marker", boundary)
	}
}

func executeResumedResponseScopeCLI(
	t testing.TB,
	server *support.FunctionalAPIServer,
	args []string,
) (string, string, error) {
	t.Helper()
	home := t.TempDir()
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	inputs.Input.WorkingDirectory = home
	inputs.Input.Stdin = strings.NewReader("")
	stdinIsTTY := true
	inputs.Input.StdinIsTTY = &stdinIsTTY
	err := server.Execute(t, inputs.Input)
	return inputs.Stdout(), inputs.Stderr(), err
}

func readResumedFutureEventsUntilWorkTerminal(
	t testing.TB,
	stream *support.FactoryEventStream,
	workID string,
) []factoryapi.FactoryEvent {
	t.Helper()
	var events []factoryapi.FactoryEvent
	for len(events) < 4096 {
		event, ok := stream.TryNextEvent(resumedResponseScopeStreamTimeout)
		if !ok {
			t.Fatalf("timed out waiting for factory event stream payload within %s after %d events", resumedResponseScopeStreamTimeout, len(events))
		}
		events = append(events, event)
		if event.Type == factoryapi.FactoryEventTypeWorkStateChange {
			if resumedWorkStateChangeIsTerminal(t, event, workID) {
				return events
			}
			continue
		}
		if resumedDispatchResponseAccepted(t, event, workID) {
			return events
		}
	}
	t.Fatalf("read %d resumed Factory Events without generated Work %q terminal state", len(events), workID)
	return nil
}

func resumedWorkStateChangeIsTerminal(t testing.TB, event factoryapi.FactoryEvent, workID string) bool {
	t.Helper()
	payload, err := event.Payload.AsWorkStateChangeEventPayload()
	if err != nil {
		t.Fatalf("decode generated Work state event %q: %v", event.Id, err)
	}
	return payload.WorkId == workID && (payload.ToState == "complete" || payload.ToState == "failed")
}

func resumedDispatchResponseAccepted(t testing.TB, event factoryapi.FactoryEvent, workID string) bool {
	t.Helper()
	if event.Type != factoryapi.FactoryEventTypeDispatchResponse || !resumedEventContainsWork(event, workID) {
		return false
	}
	payload, err := event.Payload.AsDispatchResponseEventPayload()
	if err != nil {
		t.Fatalf("decode generated terminal dispatch response %q: %v", event.Id, err)
	}
	return payload.TransitionId == "validate" && payload.Outcome == factoryapi.WorkOutcomeAccepted
}

func assertGeneratedWorkCanonicalProgress(t testing.TB, events []factoryapi.FactoryEvent, workID string) string {
	t.Helper()
	workRequestSeen, workProgressSeen, dispatchID := false, false, ""
	for _, event := range events {
		if !resumedEventContainsWork(event, workID) {
			continue
		}
		switch event.Type {
		case factoryapi.FactoryEventTypeWorkRequest:
			workRequestSeen = true
		case factoryapi.FactoryEventTypeWorkStateChange:
			payload, err := event.Payload.AsWorkStateChangeEventPayload()
			if err != nil {
				t.Fatalf("decode generated Work progress event %q: %v", event.Id, err)
			}
			workProgressSeen = workProgressSeen || payload.WorkId == workID && payload.FromState != payload.ToState
		case factoryapi.FactoryEventTypeDispatchRequest,
			factoryapi.FactoryEventTypeDispatchWorkerSessionAssociation,
			factoryapi.FactoryEventTypeScriptRequest,
			factoryapi.FactoryEventTypeScriptResponse,
			factoryapi.FactoryEventTypeAgentRunResponse,
			factoryapi.FactoryEventTypeDispatchResponse:
			workProgressSeen = workProgressSeen || event.Type != factoryapi.FactoryEventTypeDispatchResponse
			if event.Context.DispatchId != nil && *event.Context.DispatchId != "" {
				dispatchID = *event.Context.DispatchId
			}
		}
	}
	if !workRequestSeen || !workProgressSeen || dispatchID == "" {
		t.Fatalf("generated Work %q progress request=%t progress=%t dispatch=%q", workID, workRequestSeen, workProgressSeen, dispatchID)
	}
	return dispatchID
}

func resumedEventContainsWork(event factoryapi.FactoryEvent, workID string) bool {
	if event.Context.WorkIds == nil {
		return false
	}
	for _, candidate := range *event.Context.WorkIds {
		if candidate == workID {
			return true
		}
	}
	return false
}

func readResumedResponseEventsUntilDispatchTerminal(
	t testing.TB,
	stream *support.FactoryResponseEventStream,
	dispatchID string,
) []factoryapi.FactoryResponseEvent {
	t.Helper()
	var events []factoryapi.FactoryResponseEvent
	for len(events) < 4096 {
		result := stream.TryNextFrameResult(resumedResponseScopeStreamTimeout)
		if result.Outcome != support.FactoryResponseEventStreamOutcomeFrame {
			t.Fatalf("read generated Response Events: %s", result.Diagnostic())
		}
		event := result.Frame.Event
		if support.StringPointerValue(event.DispatchId) != dispatchID {
			continue
		}
		events = append(events, event)
		if resumedResponseEventIsTerminal(event) {
			return events
		}
	}
	t.Fatalf("read 4096 Response Events without terminal dispatch %q", dispatchID)
	return nil
}

func resumedResponseEventIsTerminal(event factoryapi.FactoryResponseEvent) bool {
	if event.Kind == factoryapi.FactoryResponseEventKindRun {
		return event.Phase == factoryapi.FactoryResponseEventPhaseCompleted ||
			event.Phase == factoryapi.FactoryResponseEventPhaseFailed ||
			event.Phase == factoryapi.FactoryResponseEventPhaseCanceled
	}
	return event.Kind == factoryapi.FactoryResponseEventKindError &&
		(event.Phase == factoryapi.FactoryResponseEventPhaseFailed ||
			event.Phase == factoryapi.FactoryResponseEventPhaseCanceled)
}

func assertGeneratedWorkResponseProgress(
	t testing.TB,
	events []factoryapi.FactoryResponseEvent,
	dispatchID, sessionID string,
) {
	t.Helper()
	if len(events) < 2 {
		t.Fatalf("generated Response Events for dispatch %q = %d, want progress and terminal", dispatchID, len(events))
	}
	progress, terminal := false, false
	for _, event := range events {
		if event.FactorySessionId != sessionID || support.StringPointerValue(event.DispatchId) != dispatchID {
			t.Fatalf("generated Response Event correlation = %#v, want session %q dispatch %q", event, sessionID, dispatchID)
		}
		if resumedResponseEventIsTerminal(event) {
			terminal = true
		} else {
			progress = true
		}
	}
	if !progress || !terminal {
		t.Fatalf("generated Response Events progress=%t terminal=%t events=%#v", progress, terminal, events)
	}
}

func assertGeneratedWorkTerminal(t testing.TB, item factoryapi.Work, workID string) {
	t.Helper()
	if support.StringPointerValue(item.WorkId) != workID || item.State == nil {
		t.Fatalf("generated Work = %#v, want terminal Work %q", item, workID)
	}
	if item.State.Type != factoryapi.WorkStateTypeTERMINAL && item.State.Type != factoryapi.WorkStateTypeFAILED {
		t.Fatalf("generated Work %q state = %#v, want terminal or failed", workID, item.State)
	}
}

func assertGeneratedSuffixesAboveHistory(
	t testing.TB,
	baseURL, sessionID string,
	ledger resumedResponseScopeLedger,
) {
	t.Helper()
	works := readAllResumedSessionWork(t, baseURL, sessionID)
	newWorkCount := 0
	for _, item := range works {
		workID := support.StringPointerValue(item.WorkId)
		match := resumedResponseScopeWorkIDPattern.FindStringSubmatch(workID)
		if len(match) != 3 {
			continue
		}
		value, err := strconv.Atoi(match[2])
		if err != nil {
			t.Fatalf("parse public generated Work suffix %q: %v", workID, err)
		}
		if _, historical := ledger.historicalWorkIDs[workID]; historical {
			continue
		}
		newWorkCount++
		if value <= ledger.historicalWorkMaxima[match[1]] {
			t.Fatalf("generated Work %q suffix = %d, historical %s maximum = %d", workID, value, match[1], ledger.historicalWorkMaxima[match[1]])
		}
	}
	if newWorkCount == 0 {
		t.Fatal("public Work board has no generated higher-suffix Work")
	}
}

func assertResumedResponseScopeNoWarnings(
	t testing.TB,
	canonicalEvents []factoryapi.FactoryEvent,
	responseEvents []factoryapi.FactoryResponseEvent,
) {
	t.Helper()
	for _, value := range []any{canonicalEvents, responseEvents} {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("marshal resumed warning scan: %v", err)
		}
		text := string(encoded)
		if strings.Contains(text, "CANONICAL_EVENT_PUBLISH_FAILED") || strings.Contains(strings.ToLower(text), "publication-complete") {
			t.Fatalf("resumed successor emitted a publication warning")
		}
	}
}

func postResumedLifecycleControl(t testing.TB, baseURL, sessionID, operation string) (int, []byte) {
	t.Helper()
	body := bytes.NewReader([]byte(`{}`))
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) + "/" + operation
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, body)
	if err != nil {
		t.Fatalf("build resumed lifecycle control: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST resumed lifecycle control: %v", err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read resumed lifecycle control: %v", err)
	}
	return response.StatusCode, responseBody
}

func assertTerminalResumedSession(t testing.TB, session factoryapi.FactorySession) {
	t.Helper()
	if session.Runtime.LifecycleControlStatus == nil {
		t.Fatalf("terminal resumed session has no lifecycle status: %#v", session)
	}
	switch *session.Runtime.LifecycleControlStatus {
	case factoryapi.FactorySessionDurableLifecycleStatusSucceeded,
		factoryapi.FactorySessionDurableLifecycleStatusFailed,
		factoryapi.FactorySessionDurableLifecycleStatusCanceled,
		factoryapi.FactorySessionDurableLifecycleStatusInterrupted,
		factoryapi.FactorySessionDurableLifecycleStatusTimedOut,
		factoryapi.FactorySessionDurableLifecycleStatusTerminated:
	default:
		t.Fatalf("resumed successor lifecycle status = %q, want terminal", *session.Runtime.LifecycleControlStatus)
	}
}

func assertResumedResponseEventOrder(
	t testing.TB,
	events []factoryapi.FactoryResponseEvent,
	sessionID, dispatchID string,
) {
	t.Helper()
	if len(events) == 0 {
		t.Fatal("terminal response-event replay is empty")
	}
	seen := make(map[string]struct{}, len(events))
	var previousSequence int64
	matchingDispatch := false
	for index, event := range events {
		if event.EventId == "" || event.FactorySessionId != sessionID {
			t.Fatalf("terminal Response Event %d = %#v, want stable session/event identity", index, event)
		}
		if _, duplicate := seen[event.EventId]; duplicate {
			t.Fatalf("terminal Response Events duplicate ID %q", event.EventId)
		}
		seen[event.EventId] = struct{}{}
		if index > 0 && event.Sequence <= previousSequence {
			t.Fatalf("terminal Response Event sequence %d after %d is not strictly increasing", event.Sequence, previousSequence)
		}
		previousSequence = event.Sequence
		matchingDispatch = matchingDispatch || support.StringPointerValue(event.DispatchId) == dispatchID
	}
	if !matchingDispatch {
		t.Fatalf("terminal Response Event replay omitted dispatch %q", dispatchID)
	}
}

func assertTerminalLifecycleConflict(t testing.TB, status int, body []byte, boundary string) {
	t.Helper()
	if status != http.StatusConflict {
		t.Fatalf("%s terminal lifecycle status = %d, want 409: %s", boundary, status, strings.TrimSpace(string(body)))
	}
	var response factoryapi.FactorySessionLifecycleControlResponse
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("decode %s terminal lifecycle conflict: %v", boundary, err)
	}
	if response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeTerminalSession {
		t.Fatalf("%s terminal lifecycle response = %#v, want TERMINAL_SESSION", boundary, response)
	}
}

func assertRecordedSuccessorExists(t testing.TB, successorPath string) {
	t.Helper()
	stat, err := os.Stat(successorPath)
	if err != nil {
		t.Fatalf("successor recording was not created: %v", err)
	}
	if stat.Size() == 0 {
		t.Fatal("successor recording is empty")
	}
}
