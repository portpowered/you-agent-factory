package sessioncontrols_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	pauseResumeProcessTaskWorkstation = "process-task"
	pauseResumeDrainWaitTimeout       = 30 * time.Second
	pauseResumeBusyLoopWorkflowName   = "busy-loop"
	pauseResumeDurableStatusTimeout   = 15 * time.Second
)

func pauseResumeControlsFactoryDirWithBusyLoop(t *testing.T) string {
	t.Helper()

	dir := support.ScaffoldFactory(t, pauseResumeControlsFactoryConfig())
	workflowDir := filepath.Join(dir, ".claude", "workflows")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	fixturePath := support.AgentFactoryPath(
		t,
		filepath.Join("tests", "fixtures", "javascript_runtime", pauseResumeBusyLoopWorkflowName+".workflow.js"),
	)
	raw, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read workflow fixture %s: %v", fixturePath, err)
	}
	if err := os.WriteFile(
		filepath.Join(workflowDir, pauseResumeBusyLoopWorkflowName+".js"),
		raw,
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

func waitForDurableFactorySessionTerminal(
	t *testing.T,
	baseURL string,
	sessionID string,
	timeout time.Duration,
) factoryapi.FactorySessionDurableLifecycleStatus {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		session := readDurableFactorySession(t, baseURL, sessionID)
		if session.Status == factoryapi.FactorySessionDurableLifecycleStatusTerminated ||
			session.Status == factoryapi.FactorySessionDurableLifecycleStatusCanceled {
			return session.Status
		}
		time.Sleep(15 * time.Millisecond)
	}
	session := readDurableFactorySession(t, baseURL, sessionID)
	t.Fatalf(
		"durable session %s status = %q, want TERMINATED or CANCELED within %s",
		sessionID,
		session.Status,
		timeout,
	)
	return ""
}

func waitForDurableFactorySessionStatus(
	t *testing.T,
	baseURL string,
	sessionID string,
	want factoryapi.FactorySessionDurableLifecycleStatus,
	timeout time.Duration,
) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		session := readDurableFactorySession(t, baseURL, sessionID)
		if session.Status == want {
			return
		}
		time.Sleep(15 * time.Millisecond)
	}
	session := readDurableFactorySession(t, baseURL, sessionID)
	t.Fatalf(
		"durable session %s status = %q, want %q within %s",
		sessionID,
		session.Status,
		want,
		timeout,
	)
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
	want factoryapi.FactorySessionDurableLifecycleStatus,
) {
	t.Helper()

	session := support.GetDefaultSession(t, baseURL)
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
		baseURL+"/factory-sessions/"+sessionID+"/"+pathSegment,
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
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + sessionID + "/" + pathSegment
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

func pauseResumeControlsSlowMockWorkersConfig() *workers.MockWorkersConfig {
	return &workers.MockWorkersConfig{
		UnmatchedDispatchPolicy: workers.MockWorkerUnmatchedDispatchPolicyPassthrough,
		MockWorkers: []workers.MockWorkerConfig{
			{
				WorkerName:      "mock-worker",
				WorkstationName: pauseResumeProcessTaskWorkstation,
				RunType:         workers.MockWorkerRunTypeScript,
				ScriptConfig: &workers.MockWorkerScriptConfig{
					Command: "/bin/sh",
					Args: []string{
						"-c",
						"sleep 2 && echo mock worker accepted",
					},
				},
			},
		},
	}
}

func waitForSessionWorkIDsAtCustomerState(
	t *testing.T,
	baseURL string,
	workIDs []string,
	location string,
	timeout time.Duration,
) {
	t.Helper()

	want := make(map[string]bool, len(workIDs))
	for _, workID := range workIDs {
		want[workID] = true
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		listed := support.ListDefaultSessionWork(t, baseURL)
		found := 0
		for _, workID := range workIDs {
			if support.HasWorkAtCustomerState(listed, workID, location) {
				found++
			}
		}
		if found == len(want) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	listed := support.ListDefaultSessionWork(t, baseURL)
	t.Fatalf(
		"timed out waiting for work IDs %v at %s; listed=%#v",
		workIDs,
		location,
		listed.Results,
	)
}

func assertBufferedWorkDrainedInSubmissionOrder(
	t *testing.T,
	server *support.FunctionalAPIServer,
	firstWorkID string,
	secondWorkID string,
) {
	t.Helper()

	waitForSessionWorkIDsAtCustomerState(
		t,
		server.URL(),
		[]string{firstWorkID, secondWorkID},
		support.WorkCustomerLocation("task", "complete"),
		pauseResumeDrainWaitTimeout,
	)

	dispatches := support.ObserveDispatchEvents(t, server.GetFactoryEvents(t))
	firstDispatch, okFirst := dispatchObservationForWorkAtWorkstation(
		dispatches,
		firstWorkID,
		pauseResumeProcessTaskWorkstation,
	)
	secondDispatch, okSecond := dispatchObservationForWorkAtWorkstation(
		dispatches,
		secondWorkID,
		pauseResumeProcessTaskWorkstation,
	)
	if !okFirst || !okSecond {
		t.Fatalf(
			"dispatch history missing %q dispatches for buffered work %q and %q: %#v",
			pauseResumeProcessTaskWorkstation,
			firstWorkID,
			secondWorkID,
			dispatches,
		)
	}
	if !firstDispatch.StartedAt.Before(secondDispatch.StartedAt) {
		t.Fatalf(
			"dispatch start order = first@%s second@%s for works %q then %q; want first buffered work to start before second",
			firstDispatch.StartedAt.UTC(),
			secondDispatch.StartedAt.UTC(),
			firstWorkID,
			secondWorkID,
		)
	}
}

func dispatchObservationForWorkAtWorkstation(
	dispatches []support.DispatchEventObservation,
	workID string,
	workstation string,
) (support.DispatchEventObservation, bool) {
	for _, dispatch := range dispatches {
		if dispatch.Request.TransitionId != workstation {
			continue
		}
		if support.DispatchObservationIncludesWork(dispatch, workID) {
			return dispatch, true
		}
	}
	return support.DispatchEventObservation{}, false
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
