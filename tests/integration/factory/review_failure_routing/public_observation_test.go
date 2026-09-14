package review_failure_routing

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const maxRoutingEventLineBytes = 4 * 1024 * 1024

func waitForRoutingSession(t *testing.T, child *routingChild) string {
	t.Helper()
	deadline := time.NewTimer(childReadyTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(childPollInterval)
	defer ticker.Stop()
	for {
		response, err := httpClient.Get(child.baseURL + "/factory-sessions?scope=live")
		if err == nil {
			var listed factoryapi.ListFactorySessionsResponse
			decodeErr := json.NewDecoder(response.Body).Decode(&listed)
			_ = response.Body.Close()
			if decodeErr == nil {
				for _, session := range listed.Sessions {
					if session.IsDefault && strings.TrimSpace(session.Id) != "" {
						child.sessionID = session.Id
						return session.Id
					}
				}
			}
		}
		if exited, waitErr := child.exited(); exited {
			t.Fatalf("Factory child exited before its live session was visible: %v\n%s", waitErr, child.evidence())
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("timed out waiting for default live Factory Session\n%s", child.evidence())
		}
	}
}

func submitRoutingBatch(t *testing.T, child *routingChild, requestID string, works []factoryapi.Work) {
	t.Helper()
	request := factoryapi.WorkRequest{
		RequestId: requestID,
		Type:      factoryapi.WorkRequestTypeFactoryRequestBatch,
		Works:     &works,
	}
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal routing Work request %q: %v", requestID, err)
	}
	endpoint := child.baseURL + "/factory-sessions/" + url.PathEscape(child.sessionID) + "/work-requests/" + url.PathEscape(requestID)
	requestHTTP, err := http.NewRequest(http.MethodPut, endpoint, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build routing Work request %q: %v", requestID, err)
	}
	requestHTTP.Header.Set("Content-Type", "application/json")
	response, err := httpClient.Do(requestHTTP)
	if err != nil {
		t.Fatalf("submit routing Work request %q: %v\n%s", requestID, err, child.evidence())
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		responseBody, _ := io.ReadAll(response.Body)
		t.Fatalf("submit routing Work request %q status=%d body=%s", requestID, response.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	var result factoryapi.UpsertWorkRequestResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("decode routing Work request %q response: %v", requestID, err)
	}
	if result.RequestId != requestID || len(result.Works) != len(works) {
		t.Fatalf("routing Work request %q result=%#v, want %d admitted works", requestID, result, len(works))
	}
}

func readRoutingWorks(baseURL, sessionID string) ([]factoryapi.Work, error) {
	endpoint := baseURL + "/factory-sessions/" + url.PathEscape(sessionID) + "/work?includeSuperseded=true"
	response, err := httpClient.Get(endpoint)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		return nil, fmt.Errorf("GET Factory Work status=%d body=%s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	var listed factoryapi.ListWorkResponse
	if err := json.NewDecoder(response.Body).Decode(&listed); err != nil {
		return nil, err
	}
	return listed.Results, nil
}

func readRoutingEvents(baseURL, sessionID string) ([]factoryEvent, error) {
	endpoint := baseURL + "/factory-sessions/" + url.PathEscape(sessionID) + "/events"
	response, err := httpClient.Get(endpoint)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		return nil, fmt.Errorf("GET Factory Events status=%d body=%s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	retainedCount, err := strconv.Atoi(strings.TrimSpace(response.Header.Get(factorysessions.SessionEventStreamRetainedCountHeader)))
	if err != nil {
		return nil, fmt.Errorf("parse retained Factory Event count: %w", err)
	}
	events := make([]factoryEvent, 0, retainedCount)
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 64*1024), maxRoutingEventLineBytes)
	for len(events) < retainedCount && scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var event factoryEvent
		if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &event); err != nil {
			return nil, fmt.Errorf("decode Factory Event: %w", err)
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("read Factory Events: %w", err)
	}
	if len(events) != retainedCount {
		return nil, fmt.Errorf("Factory Event stream ended after %d of %d retained events", len(events), retainedCount)
	}
	return events, nil
}

type factoryEvent = factoryapi.FactoryEvent

type routingDispatch struct {
	ID          string
	Transition  string
	InputIDs    []string
	TraceID     string
	Response    *factoryapi.DispatchResponseEventPayload
	RequestSeen bool
}

func routingDispatches(t testing.TB, events []factoryEvent) []routingDispatch {
	t.Helper()
	byID := make(map[string]int)
	dispatches := make([]routingDispatch, 0)
	for _, event := range events {
		if event.Context.DispatchId == nil || strings.TrimSpace(*event.Context.DispatchId) == "" {
			continue
		}
		id := *event.Context.DispatchId
		switch event.Type {
		case factoryapi.FactoryEventTypeDispatchRequest:
			payload, err := event.Payload.AsDispatchRequestEventPayload()
			if err != nil {
				t.Fatalf("decode DISPATCH_REQUEST %q: %v", id, err)
			}
			inputIDs := make([]string, 0, len(payload.Inputs))
			for _, input := range payload.Inputs {
				inputIDs = append(inputIDs, input.WorkId)
			}
			traceID := ""
			if event.Context.CurrentChainingTraceId != nil {
				traceID = *event.Context.CurrentChainingTraceId
			}
			byID[id] = len(dispatches)
			dispatches = append(dispatches, routingDispatch{
				ID: id, Transition: payload.TransitionId, InputIDs: inputIDs,
				TraceID: traceID, RequestSeen: true,
			})
		case factoryapi.FactoryEventTypeDispatchResponse:
			payload, err := event.Payload.AsDispatchResponseEventPayload()
			if err != nil {
				t.Fatalf("decode DISPATCH_RESPONSE %q: %v", id, err)
			}
			if index, ok := byID[id]; ok {
				dispatches[index].Response = &payload
				continue
			}
			byID[id] = len(dispatches)
			dispatches = append(dispatches, routingDispatch{ID: id, Transition: payload.TransitionId, Response: &payload})
		}
	}
	return dispatches
}

func routingDispatchHasInputs(dispatch routingDispatch, want ...string) bool {
	if len(dispatch.InputIDs) != len(want) {
		return false
	}
	counts := make(map[string]int, len(want))
	for _, id := range want {
		counts[id]++
	}
	for _, id := range dispatch.InputIDs {
		if counts[id] == 0 {
			return false
		}
		counts[id]--
	}
	for _, count := range counts {
		if count != 0 {
			return false
		}
	}
	return true
}

func routingDispatchHasInput(dispatch routingDispatch, want string) bool {
	for _, id := range dispatch.InputIDs {
		if id == want {
			return true
		}
	}
	return false
}

func routingDiagnostic(response *factoryapi.DispatchResponseEventPayload) string {
	if response == nil {
		return ""
	}
	if response.Feedback != nil {
		return *response.Feedback
	}
	if response.FailureDetail != nil {
		return response.FailureDetail.Message
	}
	if response.Error != nil {
		return *response.Error
	}
	return ""
}

func routingWorkByID(works []factoryapi.Work) map[string]factoryapi.Work {
	byID := make(map[string]factoryapi.Work, len(works))
	for _, work := range works {
		if work.WorkId != nil && *work.WorkId != "" {
			byID[*work.WorkId] = work
		}
	}
	return byID
}

func routingState(work factoryapi.Work) string {
	if work.State == nil {
		return ""
	}
	return work.State.Name
}

type routingWorkSnapshot struct {
	Name       string
	State      string
	TraceID    string
	CurrentID  string
	Payload    string
	RequestID  string
	StateClass string
}

func snapshotRoutingWork(work factoryapi.Work) routingWorkSnapshot {
	snapshot := routingWorkSnapshot{Name: work.Name, State: routingState(work), Payload: payloadString(work.Payload)}
	if work.TraceId != nil {
		snapshot.TraceID = *work.TraceId
	}
	if work.CurrentChainingTraceId != nil {
		snapshot.CurrentID = *work.CurrentChainingTraceId
	}
	if work.RequestId != nil {
		snapshot.RequestID = *work.RequestId
	}
	if work.State != nil {
		snapshot.StateClass = string(work.State.Type)
	}
	return snapshot
}

func payloadString(value interface{}) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(data)
}
