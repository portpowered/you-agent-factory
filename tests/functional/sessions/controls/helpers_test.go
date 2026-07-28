package sessioncontrols_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	pauseResumeProcessTaskWorkstation = "process-task"
	pauseResumeDrainWaitTimeout       = 30 * time.Second
)

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
