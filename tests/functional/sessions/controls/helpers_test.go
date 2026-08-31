package sessioncontrols_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	pauseResumeProcessTaskWorkstation = "process-task"
	pauseResumeDrainWaitTimeout       = 30 * time.Second
	pauseResumeBusyLoopWorkflowName   = "busy-loop"
	pauseResumeBusyLoopWorkflowSource = `var spin = 0;
while (true) {
  spin += 1;
}
`
	pauseResumeDurableStatusTimeout = 15 * time.Second

	interruptedInspectWorkTypeName      = "goal"
	interruptedInspectReviewWorkstation = "review-goal"
)

func openPauseResumeLifecycleEventStream(
	t *testing.T,
	baseURL string,
	sessionID string,
) *support.FactoryEventStream {
	t.Helper()

	retained := support.GetFactoryEventsForSessionAt(t, baseURL, sessionID)
	if len(retained) == 0 {
		t.Fatal("canonical Factory Event history is empty before pause/resume controls")
	}

	// Open without a cursor so the retained prefix flushes the SSE handshake.
	// Then consume through the last event in the committed snapshot. Events
	// published between that snapshot and the subscription remain queued after
	// this point, so the lifecycle assertion has a stable observation boundary.
	lastRetainedEventID := retained[len(retained)-1].Id
	stream := support.OpenFactoryEventStreamAt(
		t,
		support.SessionEventsURL(baseURL, sessionID),
	)
	deadline := time.Now().Add(pauseResumeDurableStatusTimeout)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			t.Fatalf(
				"timed out establishing Factory Event observation point after retained event %q",
				lastRetainedEventID,
			)
		}
		event, ok := stream.TryNextEvent(remaining)
		if !ok {
			t.Fatalf(
				"Factory Event stream closed before observation point at retained event %q",
				lastRetainedEventID,
			)
		}
		if event.Id == lastRetainedEventID {
			return stream
		}
	}
}

func controlsSessionEndpoint(baseURL, sessionID, path string) string {
	return strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) + path
}

func getControlsSession(
	t testing.TB,
	baseURL string,
	sessionID string,
) factoryapi.FactorySession {
	t.Helper()
	response := support.GetJSON[factoryapi.FactorySessionGetResponse](
		t,
		controlsSessionEndpoint(baseURL, sessionID, ""),
	)
	session, err := response.AsFactorySession()
	if err != nil {
		t.Fatalf("decode Factory Session %q: %v", sessionID, err)
	}
	return session
}

func listControlsSessionWork(
	t testing.TB,
	baseURL string,
	sessionID string,
) factoryapi.ListWorkResponse {
	t.Helper()
	return support.GetJSON[factoryapi.ListWorkResponse](
		t,
		controlsSessionEndpoint(baseURL, sessionID, "/work"),
	)
}

func getControlsSessionWorkByID(
	t testing.TB,
	baseURL string,
	sessionID string,
	workID string,
) factoryapi.Work {
	t.Helper()
	if strings.TrimSpace(workID) == "" {
		t.Fatal("workID is empty")
	}
	return support.GetJSON[factoryapi.Work](
		t,
		controlsSessionEndpoint(baseURL, sessionID, "/work/"+url.PathEscape(workID)),
	)
}

func submitControlsSessionWork(
	t testing.TB,
	baseURL string,
	sessionID string,
	request factoryapi.SubmitWorkRequest,
) factoryapi.SubmitWorkResponse {
	t.Helper()
	return support.SubmitSessionWorkAt(t, baseURL, sessionID, request)
}

func pauseResumeControlsFactoryDirWithBusyLoop(t *testing.T) string {
	t.Helper()

	dir := support.ScaffoldFactory(t, pauseResumeControlsFactoryConfig())
	workflowDir := filepath.Join(dir, ".claude", "workflows")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(workflowDir, pauseResumeBusyLoopWorkflowName+".js"),
		[]byte(pauseResumeBusyLoopWorkflowSource),
		0o600,
	); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
	return dir
}

func pauseResumeControlsFactoryConfig() map[string]any {
	return map[string]any{
		"name": "sessions-controls-pause-resume",
		"workTypes": []map[string]any{
			{
				"name": "task",
				"states": []map[string]any{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []map[string]string{
			{"name": "mock-worker"},
		},
		"workstations": []map[string]any{
			{
				"name":      "process-task",
				"worker":    "mock-worker",
				"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
				"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
			},
		},
	}
}

func startBusyLoopDurableSession(t *testing.T, baseURL string, requestID string) string {
	t.Helper()

	workflowName := pauseResumeBusyLoopWorkflowName
	started := postSessionControlJSON[factoryapi.FactorySessionExecutionResponse](
		t,
		strings.TrimSuffix(baseURL, "/")+"/factory-sessions/async",
		factoryapi.FactorySessionExecutionRequest{
			RequestId: requestID,
			Source: factoryapi.FactorySessionExecutionSource{
				Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowName,
				WorkflowName: &workflowName,
			},
		},
		"start busy-loop durable Factory Session",
	)
	if started.SessionId == "" {
		t.Fatalf("async durable session id is empty: %#v", started)
	}
	return started.SessionId
}

func uniqueControlsDurableRequestID(prefix string) string {
	return prefix + "-" + uuid.NewString()
}

func readDurableFactorySession(
	t *testing.T,
	baseURL string,
	sessionID string,
) factoryapi.FactorySessionDurableReadModel {
	t.Helper()

	response := support.GetJSON[factoryapi.FactorySessionGetResponse](
		t,
		strings.TrimSuffix(baseURL, "/")+"/factory-sessions/"+sessionID,
	)
	session, err := response.AsFactorySessionDurableReadModel()
	if err != nil {
		t.Fatalf("decode durable session %s: %v", sessionID, err)
	}
	if session.SessionId != sessionID {
		t.Fatalf("durable session id = %q, want %q", session.SessionId, sessionID)
	}
	return session
}

func isDurableFactorySessionTerminal(
	status factoryapi.FactorySessionDurableLifecycleStatus,
) bool {
	return status == factoryapi.FactorySessionDurableLifecycleStatusTerminated ||
		status == factoryapi.FactorySessionDurableLifecycleStatusCanceled
}

func assertDurableFactorySessionRemainsTerminal(
	t *testing.T,
	baseURL string,
	sessionID string,
	context string,
) {
	t.Helper()

	session := readDurableFactorySession(t, baseURL, sessionID)
	if !isDurableFactorySessionTerminal(session.Status) {
		t.Fatalf(
			"%s: session %s status = %q, want terminal TERMINATED or CANCELED",
			context,
			sessionID,
			session.Status,
		)
	}
}

func openDurableSessionEventStream(
	t *testing.T,
	baseURL string,
	sessionID string,
) *support.FactoryEventStream {
	t.Helper()
	return support.OpenFactoryEventStreamAt(t, support.SessionEventsURL(baseURL, sessionID))
}

func waitForDurableSessionLifecycleStatus(
	t *testing.T,
	baseURL string,
	stream *support.FactoryEventStream,
	sessionID string,
	want factoryapi.FactorySessionDurableLifecycleStatus,
	timeout time.Duration,
) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			t.Fatalf("timed out waiting for durable Factory Session %s status %q", sessionID, want)
		}
		event, ok := stream.TryNextEvent(remaining)
		if !ok {
			if durableSessionLifecycleStatusObservedInRetainedEvents(t, baseURL, sessionID, want) {
				return
			}
			t.Fatalf(
				"Factory Event stream closed before durable Factory Session %s reached %q; retained=%v",
				sessionID,
				want,
				support.GetFactoryEventsForSessionAt(t, baseURL, sessionID),
			)
		}
		if durableSessionLifecycleStatusObserved(t, event, sessionID, want) {
			return
		}
	}
}

func durableSessionLifecycleStatusObservedInRetainedEvents(
	t testing.TB,
	baseURL string,
	sessionID string,
	want factoryapi.FactorySessionDurableLifecycleStatus,
) bool {
	t.Helper()
	for _, event := range support.GetFactoryEventsForSessionAt(t, baseURL, sessionID) {
		if durableSessionLifecycleStatusObserved(t, event, sessionID, want) {
			return true
		}
	}
	return false
}

func durableSessionLifecycleStatusObserved(
	t testing.TB,
	event factoryapi.FactoryEvent,
	sessionID string,
	want factoryapi.FactorySessionDurableLifecycleStatus,
) bool {
	t.Helper()
	if event.Context.SessionId != nil && *event.Context.SessionId != sessionID {
		return false
	}
	switch event.Type {
	case factoryapi.FactoryEventTypeSessionStarted:
		return want == factoryapi.FactorySessionDurableLifecycleStatusRunning
	case factoryapi.FactoryEventTypeSessionLifecycleControl:
		payload, err := event.Payload.AsSessionLifecycleControlEventPayload()
		if err != nil {
			t.Fatalf("decode durable lifecycle-control event %q: %v", event.Id, err)
		}
		return payload.Outcome == factoryapi.FactorySessionLifecycleControlOutcomeAccepted &&
			durableLifecycleStatusMatches(payload.NewStatus, want)
	case factoryapi.FactoryEventTypeSessionCompleted:
		payload, err := event.Payload.AsSessionCompletedEventPayload()
		if err != nil {
			t.Fatalf("decode durable SESSION_COMPLETED event %q: %v", event.Id, err)
		}
		return durableLifecycleStatusMatches(payload.FinalStatus, want)
	default:
		return false
	}
}

func durableLifecycleStatusMatches(
	got factoryapi.FactorySessionDurableLifecycleStatus,
	want factoryapi.FactorySessionDurableLifecycleStatus,
) bool {
	if got == want {
		return true
	}
	// The terminate control may be finalized as CANCELED by the runtime after
	// the accepted TERMINATED control response. The public characterization
	// accepts either terminal outcome, so the canonical completion witness must
	// preserve that existing contract too.
	return want == factoryapi.FactorySessionDurableLifecycleStatusTerminated &&
		got == factoryapi.FactorySessionDurableLifecycleStatusCanceled
}

func assertAcceptedSessionLifecycleControl(
	t *testing.T,
	response factoryapi.FactorySessionLifecycleControlResponse,
	sessionID string,
	operation factoryapi.FactorySessionLifecycleControlKind,
	wantStatus factoryapi.FactorySessionDurableLifecycleStatus,
) {
	t.Helper()

	if response.Operation != operation {
		t.Fatalf("lifecycle control operation = %q, want %q", response.Operation, operation)
	}
	if response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("lifecycle control outcome = %q, want ACCEPTED; response=%#v", response.Outcome, response)
	}
	if response.SessionId != sessionID {
		t.Fatalf("lifecycle control sessionId = %q, want %q", response.SessionId, sessionID)
	}
	if response.Status != wantStatus {
		t.Fatalf("lifecycle control status = %q, want %q", response.Status, wantStatus)
	}
}

func assertLiveSessionLifecycleControlStatus(
	t *testing.T,
	baseURL string,
	sessionID string,
	want factoryapi.FactorySessionDurableLifecycleStatus,
) {
	t.Helper()

	session := getControlsSession(t, baseURL, sessionID)
	if session.Runtime.LifecycleControlStatus == nil {
		t.Fatalf("live session %s lifecycleControlStatus = nil, want %q", session.Id, want)
	}
	if *session.Runtime.LifecycleControlStatus != want {
		t.Fatalf(
			"live session %s lifecycleControlStatus = %q, want %q",
			session.Id,
			*session.Runtime.LifecycleControlStatus,
			want,
		)
	}
}

func postSessionLifecycleControl(
	t *testing.T,
	baseURL string,
	sessionID string,
	operation factoryapi.FactorySessionLifecycleControlKind,
) factoryapi.FactorySessionLifecycleControlResponse {
	t.Helper()

	pathSegment := lifecycleControlPathSegment(operation)
	return postSessionControlJSON[factoryapi.FactorySessionLifecycleControlResponse](
		t,
		controlsSessionEndpoint(baseURL, sessionID, "/"+pathSegment),
		factoryapi.FactorySessionLifecycleControlRequest{},
		"apply Factory Session lifecycle control "+string(operation),
	)
}

func postSessionLifecycleControlExpectConflict(
	t *testing.T,
	baseURL string,
	sessionID string,
	operation factoryapi.FactorySessionLifecycleControlKind,
) factoryapi.FactorySessionLifecycleControlResponse {
	t.Helper()

	pathSegment := lifecycleControlPathSegment(operation)
	endpoint := controlsSessionEndpoint(baseURL, sessionID, "/"+pathSegment)
	body, err := json.Marshal(factoryapi.FactorySessionLifecycleControlRequest{})
	if err != nil {
		t.Fatalf("marshal lifecycle control request: %v", err)
	}
	response, err := http.Post(endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read POST %s response: %v", endpoint, err)
	}
	if response.StatusCode != http.StatusConflict {
		t.Fatalf(
			"POST %s status = %d, want 409 conflict: %s",
			endpoint,
			response.StatusCode,
			payload,
		)
	}
	var decoded factoryapi.FactorySessionLifecycleControlResponse
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("decode POST %s response: %v\n%s", endpoint, err, payload)
	}
	return decoded
}

func isRejectedLifecycleControlOutcome(
	outcome factoryapi.FactorySessionLifecycleControlOutcome,
) bool {
	switch outcome {
	case factoryapi.FactorySessionLifecycleControlOutcomeConflict,
		factoryapi.FactorySessionLifecycleControlOutcomeInvalidState,
		factoryapi.FactorySessionLifecycleControlOutcomeTerminalSession:
		return true
	default:
		return false
	}
}

func assertRejectedSessionLifecycleControl(
	t *testing.T,
	response factoryapi.FactorySessionLifecycleControlResponse,
	sessionID string,
	operation factoryapi.FactorySessionLifecycleControlKind,
	wantStatus factoryapi.FactorySessionDurableLifecycleStatus,
) {
	t.Helper()

	if response.Operation != operation {
		t.Fatalf("lifecycle control operation = %q, want %q", response.Operation, operation)
	}
	if !isRejectedLifecycleControlOutcome(response.Outcome) {
		t.Fatalf(
			"lifecycle control outcome = %q, want CONFLICT, INVALID_STATE, or TERMINAL_SESSION; response=%#v",
			response.Outcome,
			response,
		)
	}
	if response.SessionId != sessionID {
		t.Fatalf("lifecycle control sessionId = %q, want %q", response.SessionId, sessionID)
	}
	if response.Status != wantStatus {
		t.Fatalf("lifecycle control status = %q, want %q unchanged", response.Status, wantStatus)
	}
}

func lifecycleControlPathSegment(operation factoryapi.FactorySessionLifecycleControlKind) string {
	switch operation {
	case factoryapi.FactorySessionLifecycleControlKindPause:
		return "pause"
	case factoryapi.FactorySessionLifecycleControlKindResume:
		return "resume"
	case factoryapi.FactorySessionLifecycleControlKindCancel:
		return "cancel"
	case factoryapi.FactorySessionLifecycleControlKindTerminate:
		return "terminate"
	default:
		return string(operation)
	}
}

func postSessionControlJSON[T any](t *testing.T, endpoint string, request any, failurePrefix string) T {
	t.Helper()

	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("%s: marshal request: %v", failurePrefix, err)
	}
	response, err := http.Post(endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("%s: POST %s: %v", failurePrefix, endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("%s: POST %s status = %d, want success: %s", failurePrefix, endpoint, response.StatusCode, payload)
	}
	var decoded T
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("%s: decode %s response: %v", failurePrefix, endpoint, err)
	}
	return decoded
}

func stringPointer(value string) *string {
	return &value
}

func stringPointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func waitForSessionWorkIDsAtCustomerState(
	t *testing.T,
	baseURL string,
	sessionID string,
	workIDs []string,
	location string,
	timeout time.Duration,
) {
	t.Helper()

	want := make(map[string]bool, len(workIDs))
	for _, workID := range workIDs {
		want[workID] = true
	}
	stream := support.OpenFactoryEventStreamAt(
		t,
		support.SessionEventsURL(baseURL, sessionID),
	)
	deadline := time.Now().Add(timeout)
	observed := make(map[string]bool, len(want))
	for len(observed) < len(want) {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			t.Fatalf(
				"timed out waiting for work IDs %v at %s; observed=%v",
				workIDs,
				location,
				observed,
			)
		}
		event, ok := stream.TryNextEvent(remaining)
		if !ok {
			if workStateObservedInRetainedEvents(t, baseURL, sessionID, want, location, observed) {
				return
			}
			t.Fatalf(
				"Factory Event stream closed before work IDs %v reached %s; observed=%v; retained=%v",
				workIDs,
				location,
				observed,
				support.GetFactoryEventsForSessionAt(t, baseURL, sessionID),
			)
		}
		observeWorkStateEvent(t, event, want, location, observed)
	}
}

func observeWorkStateEvent(
	t testing.TB,
	event factoryapi.FactoryEvent,
	want map[string]bool,
	location string,
	observed map[string]bool,
) {
	t.Helper()
	// Petri transitions publish WORK_STATE_CHANGE directly. Classifier
	// workstations publish their selected destination on DISPATCH_RESPONSE, so
	// both canonical forms are valid synchronization evidence for this witness.
	switch event.Type {
	case factoryapi.FactoryEventTypeWorkStateChange:
		payload, err := event.Payload.AsWorkStateChangeEventPayload()
		if err != nil {
			t.Fatalf("decode WORK_STATE_CHANGE event %q: %v", event.Id, err)
		}
		if _, ok := want[payload.WorkId]; ok &&
			support.WorkCustomerLocation(payload.WorkTypeName, payload.ToState) == location {
			observed[payload.WorkId] = true
		}
	case factoryapi.FactoryEventTypeDispatchResponse:
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			t.Fatalf("decode DISPATCH_RESPONSE event %q: %v", event.Id, err)
		}
		if payload.OutputWork == nil {
			return
		}
		for _, output := range *payload.OutputWork {
			if output.WorkId == nil || output.State == nil || output.WorkTypeName == nil {
				continue
			}
			if _, ok := want[*output.WorkId]; ok &&
				support.WorkCustomerLocation(*output.WorkTypeName, output.State.Name) == location {
				observed[*output.WorkId] = true
			}
		}
	}
}

func workStateObservedInRetainedEvents(
	t testing.TB,
	baseURL string,
	sessionID string,
	want map[string]bool,
	location string,
	observed map[string]bool,
) bool {
	t.Helper()
	for _, event := range support.GetFactoryEventsForSessionAt(t, baseURL, sessionID) {
		observeWorkStateEvent(t, event, want, location, observed)
	}
	return len(observed) == len(want)
}

func assertBufferedWorkDrainedInSubmissionOrder(
	t *testing.T,
	baseURL string,
	sessionID string,
	firstWorkID string,
	secondWorkID string,
) {
	t.Helper()

	stream := support.OpenFactoryEventStreamAt(
		t,
		support.SessionEventsURL(baseURL, sessionID),
	)
	deadline := time.Now().Add(pauseResumeDrainWaitTimeout)
	var events []factoryapi.FactoryEvent
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			t.Fatalf(
				"timed out waiting for dispatches for buffered work %q and %q",
				firstWorkID,
				secondWorkID,
			)
		}
		event, ok := stream.TryNextEvent(remaining)
		if !ok {
			t.Fatalf(
				"Factory Event stream closed before dispatches for buffered work %q and %q completed",
				firstWorkID,
				secondWorkID,
			)
		}
		events = append(events, event)
		dispatches := support.ObserveDispatchEvents(t, events)
		firstIndex, okFirst := completedDispatchObservationIndex(
			dispatches,
			firstWorkID,
			pauseResumeProcessTaskWorkstation,
		)
		secondIndex, okSecond := completedDispatchObservationIndex(
			dispatches,
			secondWorkID,
			pauseResumeProcessTaskWorkstation,
		)
		if !okFirst || !okSecond {
			continue
		}
		if firstIndex >= secondIndex {
			firstDispatch := dispatches[firstIndex]
			secondDispatch := dispatches[secondIndex]
			t.Fatalf(
				"dispatch order = first@%d (%s) second@%d (%s) for works %q then %q; want first buffered work dispatched before second",
				firstIndex,
				firstDispatch.StartedAt.UTC(),
				secondIndex,
				secondDispatch.StartedAt.UTC(),
				firstWorkID,
				secondWorkID,
			)
		}
		return
	}
}

func completedDispatchObservationIndex(
	dispatches []support.DispatchEventObservation,
	workID string,
	workstation string,
) (int, bool) {
	index, ok := dispatchObservationIndexForWorkAtWorkstation(dispatches, workID, workstation)
	if !ok || dispatches[index].Response == nil {
		return -1, false
	}
	return index, true
}

func dispatchObservationIndexForWorkAtWorkstation(
	dispatches []support.DispatchEventObservation,
	workID string,
	workstation string,
) (int, bool) {
	for index, dispatch := range dispatches {
		if dispatch.Request.TransitionId != workstation {
			continue
		}
		if support.DispatchObservationIncludesWork(dispatch, workID) {
			return index, true
		}
	}
	return -1, false
}

func waitForPauseResumeLifecycleControlEvents(
	t *testing.T,
	stream *support.FactoryEventStream,
	timeout time.Duration,
) {
	t.Helper()

	wantOperations := []factoryapi.FactorySessionLifecycleControlKind{
		factoryapi.FactorySessionLifecycleControlKindPause,
		factoryapi.FactorySessionLifecycleControlKindResume,
	}
	var observed []factoryapi.FactoryEvent
	deadline := time.Now().Add(timeout)
	for len(observed) < len(wantOperations) {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			failPauseResumeLifecycleControlWait(t, "deadline expired", observed)
		}

		// The stream is the synchronization primitive. The deadline bounds only
		// missing durable evidence; it does not poll Work projection state or add
		// a sleep-based success path.
		event, ok := stream.TryNextEvent(remaining)
		if !ok {
			failPauseResumeLifecycleControlWait(t, "event stream closed or deadline expired", observed)
		}
		if event.Type != factoryapi.FactoryEventTypeSessionLifecycleControl {
			continue
		}

		observed = append(observed, event)
		payload, err := event.Payload.AsSessionLifecycleControlEventPayload()
		if err != nil {
			failPauseResumeLifecycleControlWait(
				t,
				fmt.Sprintf("malformed SESSION_LIFECYCLE_CONTROL payload: %v", err),
				observed,
			)
		}
		wantOperation := wantOperations[len(observed)-1]
		if payload.Operation != wantOperation ||
			payload.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
			failPauseResumeLifecycleControlWait(
				t,
				fmt.Sprintf(
					"observed operation=%q outcome=%q; want accepted %q",
					payload.Operation,
					payload.Outcome,
					wantOperation,
				),
				observed,
			)
		}
	}

	assertPauseResumeLifecycleControlEvents(t, observed)
}

func failPauseResumeLifecycleControlWait(
	t *testing.T,
	reason string,
	observed []factoryapi.FactoryEvent,
) {
	t.Helper()

	history := make([]string, 0, len(observed))
	for _, event := range observed {
		payload, err := event.Payload.AsSessionLifecycleControlEventPayload()
		if err != nil {
			history = append(history, fmt.Sprintf(
				"id=%q sequence=%d malformed_payload=%v",
				event.Id,
				event.Context.Sequence,
				err,
			))
			continue
		}
		history = append(history, fmt.Sprintf(
			"id=%q sequence=%d operation=%q outcome=%q occurredAt=%s",
			event.Id,
			event.Context.Sequence,
			payload.Operation,
			payload.Outcome,
			payload.OccurredAt.UTC().Format(time.RFC3339Nano),
		))
	}
	t.Fatalf(
		"pause/resume durable lifecycle event wait failed: %s; expected accepted operations in order [PAUSE, RESUME]; observed lifecycle-event history: %v",
		reason,
		history,
	)
}

func filterSessionLifecycleControlEvents(events []factoryapi.FactoryEvent) []factoryapi.FactoryEvent {
	var lifecycleControls []factoryapi.FactoryEvent
	for _, event := range events {
		if event.Type == factoryapi.FactoryEventTypeSessionLifecycleControl {
			lifecycleControls = append(lifecycleControls, event)
		}
	}
	return lifecycleControls
}

func assertPauseResumeLifecycleControlEvents(
	t *testing.T,
	events []factoryapi.FactoryEvent,
) {
	t.Helper()

	lifecycleControls := filterSessionLifecycleControlEvents(events)
	if len(lifecycleControls) < 2 {
		t.Fatalf(
			"SESSION_LIFECYCLE_CONTROL events = %d, want at least pause and resume",
			len(lifecycleControls),
		)
	}

	pauseEvent := lifecycleControls[len(lifecycleControls)-2]
	resumeEvent := lifecycleControls[len(lifecycleControls)-1]
	if pauseEvent.Type != factoryapi.FactoryEventTypeSessionLifecycleControl {
		t.Fatalf("pause event type = %q, want SESSION_LIFECYCLE_CONTROL", pauseEvent.Type)
	}
	if resumeEvent.Type != factoryapi.FactoryEventTypeSessionLifecycleControl {
		t.Fatalf("resume event type = %q, want SESSION_LIFECYCLE_CONTROL", resumeEvent.Type)
	}

	pausePayload, err := pauseEvent.Payload.AsSessionLifecycleControlEventPayload()
	if err != nil {
		t.Fatalf("decode pause lifecycle-control payload: %v", err)
	}
	if pausePayload.Operation != factoryapi.FactorySessionLifecycleControlKindPause {
		t.Fatalf("pause payload operation = %q, want PAUSE", pausePayload.Operation)
	}
	if pausePayload.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("pause payload outcome = %q, want ACCEPTED", pausePayload.Outcome)
	}

	resumePayload, err := resumeEvent.Payload.AsSessionLifecycleControlEventPayload()
	if err != nil {
		t.Fatalf("decode resume lifecycle-control payload: %v", err)
	}
	if resumePayload.Operation != factoryapi.FactorySessionLifecycleControlKindResume {
		t.Fatalf("resume payload operation = %q, want RESUME", resumePayload.Operation)
	}
	if resumePayload.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("resume payload outcome = %q, want ACCEPTED", resumePayload.Outcome)
	}

	if !pausePayload.OccurredAt.Before(resumePayload.OccurredAt) &&
		!pausePayload.OccurredAt.Equal(resumePayload.OccurredAt) {
		t.Fatalf(
			"lifecycle-control event order = pause@%s resume@%s, want pause before resume",
			pausePayload.OccurredAt.UTC(),
			resumePayload.OccurredAt.UTC(),
		)
	}
}

func scaffoldInterruptedInspectFactory(t *testing.T) string {
	t.Helper()

	dir := support.ScaffoldFactory(t, interruptedInspectFactoryConfig())
	support.WriteAgentConfig(t, dir, "mock-worker", `---
type: SCRIPT_WORKER
command: /bin/echo
args:
  - interrupted
---
Classify goal work.
`)
	support.WriteWorkstationConfig(t, dir, interruptedInspectReviewWorkstation, `---
type: CLASSIFIER_WORKSTATION
---
Review goal work for interrupted routing.
`)
	return dir
}

func interruptedInspectFactoryConfig() map[string]any {
	return map[string]any{
		"name": "sessions-controls-interrupted-inspect",
		"workTypes": []map[string]any{
			{
				"name": interruptedInspectWorkTypeName,
				"states": []map[string]any{
					{"name": "init", "type": "INITIAL"},
					{"name": "interrupted", "type": "FAILED"},
					{"name": "complete", "type": "TERMINAL"},
				},
			},
		},
		"workers": []map[string]string{
			{"name": "mock-worker"},
		},
		"workstations": []map[string]any{
			{
				"name":   interruptedInspectReviewWorkstation,
				"type":   "CLASSIFIER_WORKSTATION",
				"worker": "mock-worker",
				"inputs": []map[string]string{{"workType": interruptedInspectWorkTypeName, "state": "init"}},
				"classificationRoutes": []map[string]any{
					{
						"label": "accepted",
						"outputs": []map[string]string{
							{"workType": interruptedInspectWorkTypeName, "state": "complete"},
						},
					},
					{
						"label": "interrupted",
						"outputs": []map[string]string{
							{"workType": interruptedInspectWorkTypeName, "state": "interrupted"},
						},
					},
				},
			},
		},
	}
}

func assertInterruptedStopSummary(
	t *testing.T,
	summary *factoryapi.FactoryStopSummary,
	context string,
) {
	t.Helper()

	if summary == nil {
		t.Fatalf("%s stopSummary = nil, want INTERRUPTED dispatch context", context)
	}
	if summary.StopKind != factoryapi.FactoryStopKind("INTERRUPTED") {
		t.Fatalf("%s stopKind = %q, want INTERRUPTED", context, summary.StopKind)
	}
	if summary.LatestDispatch == nil ||
		summary.LatestDispatch.Status != factoryapi.FactoryDispatchStatusINTERRUPTED {
		t.Fatalf(
			"%s latestDispatch = %#v, want INTERRUPTED dispatch context",
			context,
			summary.LatestDispatch,
		)
	}
	if summary.LatestResultSummary == nil ||
		strings.TrimSpace(*summary.LatestResultSummary) == "" {
		t.Fatalf(
			"%s latestResultSummary = %#v, want interrupted stop explanation",
			context,
			summary.LatestResultSummary,
		)
	}
}
