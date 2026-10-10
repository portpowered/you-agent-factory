package customer_lifecycles_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type packagedGoalInvocationObservation struct {
	sessionID string
	listed    factoryapi.ListWorkResponse
	events    []factoryapi.FactoryEvent
}

func observePackagedGoalInvocation(
	t testing.TB,
	baseURL string,
	sessionID string,
) packagedGoalInvocationObservation {
	t.Helper()
	workEndpoint := strings.TrimSuffix(baseURL, "/") +
		"/factory-sessions/" + url.PathEscape(sessionID) + "/work"
	return packagedGoalInvocationObservation{
		sessionID: sessionID,
		listed:    support.GetJSON[factoryapi.ListWorkResponse](t, workEndpoint),
		events:    support.GetFactoryEventsForSessionAt(t, baseURL, sessionID),
	}
}

func assertPackagedGoalInvocationWorkAndEvents(
	t testing.TB,
	response factoryapi.InvocationResponse,
	observation packagedGoalInvocationObservation,
	wantState string,
) factoryapi.Work {
	t.Helper()
	if strings.TrimSpace(response.RequestId) == "" || strings.TrimSpace(response.TraceId) == "" {
		t.Fatalf("invocation identity = request %q trace %q, want both non-empty", response.RequestId, response.TraceId)
	}
	if response.SessionId != nil && strings.TrimSpace(*response.SessionId) != "" &&
		*response.SessionId != observation.sessionID {
		t.Fatalf("invocation sessionId = %q, want observed session %q", *response.SessionId, observation.sessionID)
	}

	var matches []factoryapi.Work
	for _, candidate := range observation.listed.Results {
		if candidate.WorkTypeName == nil || *candidate.WorkTypeName != "goal" {
			continue
		}
		if packagedGoalWorkCorrelatesWithCLIInvocation(candidate, response) {
			matches = append(matches, candidate)
		}
	}
	if len(matches) != 1 {
		t.Fatalf(
			"observed goal Work matches for request %q trace %q = %d, want one; listed=%#v",
			response.RequestId, response.TraceId, len(matches), observation.listed.Results,
		)
	}
	work := matches[0]
	if work.WorkId == nil || strings.TrimSpace(*work.WorkId) == "" {
		t.Fatalf("correlated goal Work ID = %#v, want non-empty", work.WorkId)
	}
	if work.State == nil {
		t.Fatalf("correlated goal Work %q state is nil", *work.WorkId)
	}
	if wantState != "" && work.State.Name != wantState {
		t.Fatalf("correlated goal Work %q state = %q, want %q", *work.WorkId, work.State.Name, wantState)
	}
	if work.State.Type != factoryapi.WorkStateTypeTERMINAL && work.State.Type != factoryapi.WorkStateTypeFAILED {
		t.Fatalf("correlated goal Work %q state type = %q, want terminal or failed", *work.WorkId, work.State.Type)
	}

	assertPackagedGoalEventOrder(t, observation.events, observation.sessionID, response, *work.WorkId, work.State.Name)
	return work
}

func assertPackagedGoalNoAdmission(
	t testing.TB,
	observation packagedGoalInvocationObservation,
) {
	t.Helper()
	if len(observation.listed.Results) != 0 {
		t.Fatalf("rejected invocation listed Work = %#v, want no admitted Work", observation.listed.Results)
	}
	for _, event := range observation.events {
		if event.Type == factoryapi.FactoryEventTypeWorkRequest {
			t.Fatalf("rejected invocation emitted WORK_REQUEST event %#v, want absent admission", event)
		}
	}
}

func assertPackagedGoalEventOrder(
	t testing.TB,
	events []factoryapi.FactoryEvent,
	sessionID string,
	response factoryapi.InvocationResponse,
	workID string,
	wantTerminalState string,
) {
	t.Helper()
	ordered := collectPackagedGoalSessionEvents(t, events, sessionID)
	previousSequence := -1
	admissionSequence := -1
	terminalSequence := -1
	dispatchSequence := -1
	dispatchID := ""
	for index, event := range ordered {
		if (event.Type == factoryapi.FactoryEventTypeWorkRequest || event.Type == factoryapi.FactoryEventTypeWorkStateChange) &&
			(event.Context.SessionId == nil || *event.Context.SessionId != sessionID) {
			t.Fatalf("correlated Factory Event[%d] type=%q session ID = %#v, want %q", index, event.Type, event.Context.SessionId, sessionID)
		}
		if event.Context.SessionSequence == nil {
			t.Fatalf("Factory Event[%d] sessionSequence is nil", index)
		}
		sequence := *event.Context.SessionSequence
		if sequence <= previousSequence {
			t.Fatalf("Factory Event[%d] sessionSequence = %d after %d, want strictly increasing order", index, sequence, previousSequence)
		}
		previousSequence = sequence

		switch event.Type {
		case factoryapi.FactoryEventTypeWorkRequest:
			payload, err := event.Payload.AsWorkRequestEventPayload()
			if err != nil {
				t.Fatalf("decode WORK_REQUEST event %q: %v", event.Id, err)
			}
			if event.Context.RequestId == nil || *event.Context.RequestId != response.RequestId {
				continue
			}
			for _, requested := range support.FactoryWorksValue(payload.Works) {
				if requested.WorkId != nil && *requested.WorkId == workID {
					if admissionSequence >= 0 {
						t.Fatalf("multiple correlated WORK_REQUEST events for Work %q", workID)
					}
					admissionSequence = sequence
				}
			}
		case factoryapi.FactoryEventTypeWorkStateChange:
			payload, err := event.Payload.AsWorkStateChangeEventPayload()
			if err != nil {
				t.Fatalf("decode WORK_STATE_CHANGE event %q: %v", event.Id, err)
			}
			if payload.WorkId == workID && payload.ToState == wantTerminalState {
				if terminalSequence >= 0 {
					t.Fatalf("multiple correlated terminal WORK_STATE_CHANGE events for Work %q", workID)
				}
				terminalSequence = sequence
				if payload.Source != factoryapi.WorkStateChangeSourceDispatch || payload.FromState == payload.ToState ||
					payload.FromPlaceId == payload.ToPlaceId || payload.TriggerWorkId != nil || !crossEventContainsWorkID(event, workID) {
					t.Fatalf("terminal relocation lost dispatch source or Work identity: %#v", payload)
				}
			}
		case factoryapi.FactoryEventTypeDispatchResponse:
			payload, err := event.Payload.AsDispatchResponseEventPayload()
			if err != nil {
				t.Fatalf("decode DISPATCH_RESPONSE event %q: %v", event.Id, err)
			}
			if crossEventContainsWorkID(event, workID) &&
				(payload.Outcome == factoryapi.WorkOutcomeAccepted || payload.Outcome == factoryapi.WorkOutcomeFailed) {
				if dispatchSequence >= 0 {
					t.Fatalf("multiple correlated terminal dispatch results for Work %q", workID)
				}
				if event.Context.DispatchId == nil || *event.Context.DispatchId == "" ||
					payload.OutputWork == nil || len(*payload.OutputWork) != 1 {
					t.Fatalf("terminal dispatch lost identity/output: %#v", event)
				}
				output := (*payload.OutputWork)[0]
				if output.WorkId == nil || *output.WorkId != workID || output.State == nil || output.State.Name != wantTerminalState {
					t.Fatalf("terminal dispatch output=%#v, want Work %q state %q", output, workID, wantTerminalState)
				}
				dispatchID = *event.Context.DispatchId
				dispatchSequence = sequence
			}
		}
	}
	if admissionSequence < 0 {
		t.Fatalf("missing correlated WORK_REQUEST admission for request %q Work %q", response.RequestId, workID)
	}
	if terminalSequence < 0 {
		t.Fatalf("missing correlated terminal Work event for Work %q state %q; events=%v", workID, wantTerminalState, summarizeCrossFactoryEvents(ordered))
	}
	if admissionSequence >= terminalSequence {
		t.Fatalf("Work %q admission sequence=%d terminal sequence=%d, want admission first", workID, admissionSequence, terminalSequence)
	}
	if dispatchID == "" || dispatchSequence <= admissionSequence || dispatchSequence >= terminalSequence {
		t.Fatalf("Work %q admission=%d dispatch=%d relocation=%d, want separate ordered facts", workID, admissionSequence, dispatchSequence, terminalSequence)
	}
}

func collectPackagedGoalSessionEvents(
	t testing.TB,
	events []factoryapi.FactoryEvent,
	sessionID string,
) []factoryapi.FactoryEvent {
	t.Helper()
	if len(events) == 0 {
		t.Fatalf("Factory Events for session %q are empty, want admission and terminal witnesses", sessionID)
	}

	seenIDs := make(map[string]struct{}, len(events))
	ordered := make([]factoryapi.FactoryEvent, 0, len(events))
	for index, event := range events {
		if strings.TrimSpace(event.Id) == "" {
			t.Fatalf("Factory Event[%d] ID is empty", index)
		}
		if _, exists := seenIDs[event.Id]; exists {
			t.Fatalf("Factory Event ID %q is duplicated", event.Id)
		}
		seenIDs[event.Id] = struct{}{}
		if event.Context.SessionId != nil && *event.Context.SessionId != sessionID {
			t.Fatalf("Factory Event[%d] type=%q session ID = %q, want %q", index, event.Type, *event.Context.SessionId, sessionID)
		}
		if event.Context.SessionId != nil {
			ordered = append(ordered, event)
		}
	}
	if len(ordered) == 0 {
		t.Fatalf("Factory Events for session %q have no session-scoped records", sessionID)
	}
	return ordered
}

func crossEventContainsWorkID(event factoryapi.FactoryEvent, workID string) bool {
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

func summarizeCrossFactoryEvents(events []factoryapi.FactoryEvent) []string {
	summaries := make([]string, 0, len(events))
	for _, event := range events {
		sequence := "-"
		if event.Context.SessionSequence != nil {
			sequence = fmt.Sprint(*event.Context.SessionSequence)
		}
		workIDs := "-"
		if event.Context.WorkIds != nil {
			workIDs = strings.Join(*event.Context.WorkIds, ",")
		}
		requestID := "-"
		if event.Context.RequestId != nil {
			requestID = *event.Context.RequestId
		}
		summaries = append(summaries, fmt.Sprintf("%s/%s seq=%s request=%s work=%s", event.Type, event.Id, sequence, requestID, workIDs))
	}
	return summaries
}

func removeCrossOwnedPath(t testing.TB, label, path string) {
	t.Helper()
	if strings.TrimSpace(path) == "" {
		return
	}
	if err := os.RemoveAll(path); err != nil {
		t.Errorf("remove %s %q: %v", label, path, err)
		return
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("%s %q remains after cleanup; stat error: %v", label, path, err)
		return
	}

}

func crossHomeFromEnvironment(environment []string) string {
	for _, entry := range environment {
		name, value, ok := strings.Cut(entry, "=")
		if ok && strings.EqualFold(name, "HOME") {
			return value
		}
	}
	return ""
}

// assertCrossListenerClosed observes the local-real HTTP edge after its
// owning Process.Execute has joined. The short bounded retry covers the
// transport's asynchronous close without turning the behavior test into an
// unbounded polling loop.
func assertCrossListenerClosed(t testing.TB, baseURL string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	client := &http.Client{Timeout: 250 * time.Millisecond}
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(baseURL, "/")+"/status", nil)
		if err == nil {
			response, requestErr := client.Do(request)
			if requestErr != nil {
				cancel()
				return
			}
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
		}
		cancel()
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("local-real listener %s remained reachable after owning process cleanup", baseURL)
}

func assertPackagedGoalFactorySessionAbsent(t testing.TB, baseURL, sessionID string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatalf("build deleted Factory Session probe: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("GET deleted Factory Session %q: %v", sessionID, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("GET deleted Factory Session %q status = %d, want 404: %s", sessionID, response.StatusCode, strings.TrimSpace(string(body)))
	}
}
