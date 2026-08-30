package support

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// WaitForSessionWorkTerminalFromFactoryEvents waits for canonical event
// activity and checks the public Work projection only after an event wake. It
// is the continuous-runtime handoff used before requesting shutdown: the
// terminal RUN_RESPONSE is still observed separately by
// TerminalFactoryEventObservation.
func WaitForSessionWorkTerminalFromFactoryEvents(
	t testing.TB,
	baseURL string,
	sessionID string,
	timeout time.Duration,
) {
	_ = waitForSessionWorkProjectionFromFactoryEvents(
		t,
		baseURL,
		sessionID,
		timeout,
		nil,
		func(item factoryapi.Work) bool {
			return item.State != nil && isTerminalWorkState(item.State.Type)
		},
		"terminal Work",
	)
}

// WaitForSessionWorkIDsAtStateFromFactoryEvents waits for selected Work items
// to reach the requested public state. It reads the projection only after a
// canonical Factory Event wake, so callers do not need a status polling loop.
func WaitForSessionWorkIDsAtStateFromFactoryEvents(
	t testing.TB,
	baseURL string,
	sessionID string,
	workIDs []string,
	stateName string,
	timeout time.Duration,
) []factoryapi.Work {
	return waitForSessionWorkProjectionFromFactoryEvents(
		t,
		baseURL,
		sessionID,
		timeout,
		workIDs,
		func(item factoryapi.Work) bool {
			return item.State != nil && item.State.Name == stateName
		},
		"Work state "+stateName,
	)
}

func terminalWorkEventWake(eventType factoryapi.FactoryEventType) bool {
	switch eventType {
	case factoryapi.FactoryEventTypeWorkRequest,
		factoryapi.FactoryEventTypeWorkStateChange,
		factoryapi.FactoryEventTypeDispatchResponse:
		return true
	default:
		return false
	}
}

func waitForSessionWorkProjectionFromFactoryEvents(
	t testing.TB,
	baseURL, sessionID string,
	timeout time.Duration,
	workIDs []string,
	matches func(factoryapi.Work) bool,
	description string,
) []factoryapi.Work {
	t.Helper()
	if timeout <= 0 {
		t.Fatalf("%s event timeout must be positive", description)
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	stream := OpenFactoryEventStreamAt(t, SessionEventsURL(baseURL, sessionID))
	defer stream.Close()

	if matched, ok := sessionWorkProjectionMatches(t, baseURL, sessionID, workIDs, matches); ok {
		return matched
	}
	for {
		event, err := stream.nextEventContext(ctx)
		if err != nil {
			t.Fatalf(
				"wait for %s through canonical Factory Events for session %q: %v",
				description,
				sessionID,
				err,
			)
		}
		if !terminalWorkEventWake(event.Type) {
			continue
		}
		if matched, ok := sessionWorkProjectionMatches(t, baseURL, sessionID, workIDs, matches); ok {
			return matched
		}
	}
}

func sessionWorkProjectionMatches(
	t testing.TB,
	baseURL, sessionID string,
	workIDs []string,
	matches func(factoryapi.Work) bool,
) ([]factoryapi.Work, bool) {
	t.Helper()
	endpoint := strings.TrimSuffix(baseURL, "/") +
		"/factory-sessions/" + url.PathEscape(sessionID) + "/work"
	listed := GetJSON[factoryapi.ListWorkResponse](t, endpoint)
	wanted := make(map[string]struct{}, len(workIDs))
	for _, workID := range workIDs {
		wanted[workID] = struct{}{}
	}
	if len(wanted) == 0 {
		if len(listed.Results) == 0 {
			return nil, false
		}
		for _, item := range listed.Results {
			if !matches(item) {
				return nil, false
			}
		}
		return listed.Results, true
	}

	matched := make(map[string]factoryapi.Work, len(wanted))
	for _, item := range listed.Results {
		workID := StringPointerValue(item.WorkId)
		if _, ok := wanted[workID]; ok && matches(item) {
			matched[workID] = item
		}
	}
	if len(matched) != len(wanted) {
		return nil, false
	}
	ordered := make([]factoryapi.Work, 0, len(workIDs))
	for _, workID := range workIDs {
		ordered = append(ordered, matched[workID])
	}
	return ordered, true
}

func isTerminalWorkState(stateType factoryapi.WorkStateType) bool {
	return stateType == factoryapi.WorkStateTypeTERMINAL || stateType == factoryapi.WorkStateTypeFAILED
}
