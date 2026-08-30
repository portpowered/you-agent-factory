package support

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// These function-valued test-harness entrypoints are imported by external
// functional-test packages, not by production roots. Keeping the support
// implementation as function values preserves runtime behavior while making
// the production-only deadcode inventory's ownership boundary explicit.

// WaitForSessionWorkTerminalFromFactoryEvents waits for canonical event
// activity and checks the public Work projection only after an event wake. It
// is the continuous-runtime handoff used before requesting shutdown: the
// terminal RUN_RESPONSE is still observed separately by
// TerminalFactoryEventObservation.
var WaitForSessionWorkTerminalFromFactoryEvents = func(
	t testing.TB,
	baseURL string,
	sessionID string,
	timeout time.Duration,
) {
	waitForSessionWorkTerminalFromFactoryEvents(t, baseURL, sessionID, 0, timeout)
}

// WaitForSessionWorkCountTerminalFromFactoryEvents waits for an explicit
// admission set to become terminal. Continuous watcher/repeater scenarios use
// this form because a terminal projection of the Work currently listed is not
// an admission boundary: a later WORK_REQUEST can add another Work item.
var WaitForSessionWorkCountTerminalFromFactoryEvents = func(
	t testing.TB,
	baseURL string,
	sessionID string,
	expectedWorkCount int,
	timeout time.Duration,
) {
	waitForSessionWorkTerminalFromFactoryEvents(t, baseURL, sessionID, expectedWorkCount, timeout)
}

var waitForSessionWorkTerminalFromFactoryEvents = func(
	t testing.TB,
	baseURL string,
	sessionID string,
	expectedWorkCount int,
	timeout time.Duration,
) {
	if expectedWorkCount < 0 {
		t.Fatalf("expected Work admission count must not be negative")
	}
	_ = waitForSessionWorkProjectionFromFactoryEvents(
		t,
		baseURL,
		sessionID,
		timeout,
		nil,
		expectedWorkCount,
		func(item factoryapi.Work) bool {
			return item.State != nil && isTerminalWorkState(item.State.Type)
		},
		"terminal Work",
	)
}

// WaitForSessionWorkIDsAtStateFromFactoryEvents waits for selected Work items
// to reach the requested public state. It reads the projection only after a
// canonical Factory Event wake, so callers do not need a status polling loop.
var WaitForSessionWorkIDsAtStateFromFactoryEvents = func(
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
		0,
		func(item factoryapi.Work) bool {
			return item.State != nil && item.State.Name == stateName
		},
		"Work state "+stateName,
	)
}

var terminalWorkEventWake = func(eventType factoryapi.FactoryEventType) bool {
	switch eventType {
	case factoryapi.FactoryEventTypeWorkRequest,
		factoryapi.FactoryEventTypeWorkStateChange,
		factoryapi.FactoryEventTypeDispatchResponse:
		return true
	default:
		return false
	}
}

var waitForSessionWorkProjectionFromFactoryEvents = func(
	t testing.TB,
	baseURL, sessionID string,
	timeout time.Duration,
	workIDs []string,
	expectedWorkCount int,
	matches func(factoryapi.Work) bool,
	description string,
) []factoryapi.Work {
	t.Helper()
	if timeout <= 0 {
		t.Fatalf("%s event timeout must be positive", description)
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	endpoint := strings.TrimSuffix(baseURL, "/") +
		"/factory-sessions/" + url.PathEscape(sessionID) + "/events"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatalf("build Factory Event request: %v", err)
	}
	request.Header.Set("Accept", "text/event-stream")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("GET Factory Event stream: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 8*1024))
		_ = response.Body.Close()
		t.Fatalf("GET Factory Event stream status = %d body = %q", response.StatusCode, strings.TrimSpace(string(body)))
	}
	if !strings.Contains(response.Header.Get("Content-Type"), "text/event-stream") {
		_ = response.Body.Close()
		t.Fatalf("GET Factory Event stream content type = %q", response.Header.Get("Content-Type"))
	}
	defer response.Body.Close()
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	var dataLines []string
	nextEvent := func() factoryapi.FactoryEvent {
		for scanner.Scan() {
			line := scanner.Text()
			switch {
			case line == "":
				if len(dataLines) == 0 {
					continue
				}
				var event factoryapi.FactoryEvent
				if err := json.Unmarshal([]byte(strings.Join(dataLines, "\n")), &event); err != nil {
					t.Fatalf("decode Factory Event SSE payload: %v", err)
				}
				dataLines = nil
				return event
			case strings.HasPrefix(line, ":"):
				continue
			case strings.HasPrefix(line, "data:"):
				dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			case strings.HasPrefix(line, "event:"):
				t.Fatalf("Factory Event SSE emitted named event line %q", line)
			case strings.HasPrefix(line, "id:") || strings.HasPrefix(line, "retry:"):
				continue
			default:
				t.Fatalf("Factory Event SSE emitted unsupported line %q", line)
			}
		}
		if err := scanner.Err(); err != nil {
			t.Fatalf("read Factory Event SSE: %v", err)
		}
		t.Fatal("Factory Event SSE closed before the expected event")
		return factoryapi.FactoryEvent{}
	}
	readProjection := func() []factoryapi.Work {
		workEndpoint := strings.TrimSuffix(baseURL, "/") +
			"/factory-sessions/" + url.PathEscape(sessionID) + "/work"
		workRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, workEndpoint, nil)
		if err != nil {
			t.Fatalf("build Work projection request: %v", err)
		}
		workResponse, err := http.DefaultClient.Do(workRequest)
		if err != nil {
			t.Fatalf("GET Work projection: %v", err)
		}
		defer workResponse.Body.Close()
		if workResponse.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(io.LimitReader(workResponse.Body, 8*1024))
			t.Fatalf("GET Work projection status = %d body = %q", workResponse.StatusCode, strings.TrimSpace(string(body)))
		}
		var listed factoryapi.ListWorkResponse
		if err := json.NewDecoder(workResponse.Body).Decode(&listed); err != nil {
			t.Fatalf("decode Work projection: %v", err)
		}
		return listed.Results
	}

	if matched, ok := sessionWorkProjectionMatches(t, readProjection, workIDs, expectedWorkCount, matches); ok {
		return matched
	}
	for {
		event := nextEvent()
		if !terminalWorkEventWake(event.Type) {
			continue
		}
		if matched, ok := sessionWorkProjectionMatches(t, readProjection, workIDs, expectedWorkCount, matches); ok {
			return matched
		}
	}
}

var sessionWorkProjectionMatches = func(
	t testing.TB,
	readProjection func() []factoryapi.Work,
	workIDs []string,
	expectedWorkCount int,
	matches func(factoryapi.Work) bool,
) ([]factoryapi.Work, bool) {
	t.Helper()
	listed := readProjection()
	wanted := make(map[string]struct{}, len(workIDs))
	for _, workID := range workIDs {
		wanted[workID] = struct{}{}
	}
	if len(wanted) == 0 {
		if len(listed) == 0 {
			return nil, false
		}
		if expectedWorkCount > 0 && len(listed) != expectedWorkCount {
			return nil, false
		}
		for _, item := range listed {
			if !matches(item) {
				return nil, false
			}
		}
		return listed, true
	}

	matched := make(map[string]factoryapi.Work, len(wanted))
	for _, item := range listed {
		workID := ""
		if item.WorkId != nil {
			workID = *item.WorkId
		}
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

var isTerminalWorkState = func(stateType factoryapi.WorkStateType) bool {
	return stateType == factoryapi.WorkStateTypeTERMINAL || stateType == factoryapi.WorkStateTypeFAILED
}
