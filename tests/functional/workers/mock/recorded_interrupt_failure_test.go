package mock

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// M3/M4/M5 share an immutable host and storage fault route. Each direct source
// owns its gate, work identity and control tuple; neither cell can launch a
// provider. The storage edge delegates all unaffected writes to production.
func TestRecordedInterruptPreservesMockExecutionPolicyFailures(t *testing.T) {
	t.Parallel()
	dir := support.ScaffoldFactory(t, map[string]any{"workers": []map[string]string{{"name": "worker"}}})
	support.WriteAgentConfig(t, dir, "worker", "---\ntype: MODEL_WORKER\nmodelProvider: codex\nmodel: mock-model\n---\n")
	deny := &interruptDenyNative{}
	store := &mockInterruptFaultStore{}
	gates := map[string]*support.MockWorkerGate{
		"input-failure":   support.NewMockWorkerGate(t),
		"opening-failure": support.NewMockWorkerGate(t),
		"completion-ack":  support.NewMockWorkerGate(t),
	}
	config := &workers.MockWorkersConfig{UnmatchedDispatchPolicy: workers.MockWorkerUnmatchedDispatchPolicyPassthrough}
	for name, gate := range gates {
		config.MockWorkers = append(config.MockWorkers, workers.MockWorkerConfig{
			ID: name, WorkerName: name, RunType: workers.MockWorkerRunTypeAccept, GateConfig: gate.Config(30 * time.Second),
		})
	}
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir, WaitForServiceModeRuntime: true,
		Env: sharedWorkersMockEnvironment(t, writeSharedWorkersMockOperatorHome(t)), MockWorkersConfig: config,
		Edges: serviceedges.Edges{
			ProviderCommandRunner: deny, ScriptCommandRunner: deny, WorkerRecordingWriter: store,
			WorkerRecordingStoreObserver: func(real recordings.WorkerRecordingStore) { store.WorkerRecordingStore = real },
		},
	})
	support.WaitForRuntimeIdle(t, server.URL(), 10*time.Second)
	for name, gate := range gates {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			t.Cleanup(gate.Release)
			checkMockInterruptFailure(t, server.URL(), dir, name, gate)
		})
	}
	t.Cleanup(func() {
		if got := deny.attempts.Load(); got != 0 {
			t.Errorf("native launch attempts = %d, want zero", got)
		}
	})
}

func checkMockInterruptFailure(t *testing.T, baseURL, dir, name string, gate *support.MockWorkerGate) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	sourceID, successorID := name+"-source", name+"-successor"
	execution := map[string]any{
		"workstationName": workers.ProviderInvocationRoute, "workerType": name, "runnerId": "codex",
		"executorProvider": "codex", "modelProvider": "codex", "model": "mock-model",
		"workingDirectory": dir, "workingDirectoryAuthored": true, "userMessage": "source message",
		"dispatch": map[string]any{"dispatchId": name + "-dispatch", "workstationName": workers.ProviderInvocationRoute,
			"workerType": name, "execution": map[string]any{"workIds": []string{name}}},
	}
	interruptPost(t, ctx, baseURL+"/worker-sessions", map[string]any{
		"requestId": name + "-start", "workerSessionId": sourceID, "execution": execution,
	}, http.StatusAccepted)
	gate.WaitForArrival(t, 10*time.Second)
	if err := os.Remove(gate.Config(30 * time.Second).ArrivedFile); err != nil {
		t.Fatal(err)
	}
	request := map[string]any{"requestId": name + "-interrupt", "successorWorkerSessionId": successorID,
		"replacementMessage": "replacement message", "resumeMode": "recorded"}
	if name == "completion-ack" {
		checkMockInterruptLostAcknowledgement(t, ctx, baseURL, sourceID, successorID, request, gate)
		return
	}
	status, phase, sourceState := http.StatusInternalServerError, "VALIDATION", "RUNNING"
	if name == "opening-failure" {
		status, phase, sourceState = http.StatusServiceUnavailable, "SUCCESSOR_ADMISSION", "CANCELED"
	}
	endpoint := baseURL + "/worker-sessions/" + sourceID + "/interrupt"
	var first factoryapi.WorkerSessionInterruptError
	for attempt := range 2 {
		body := interruptPost(t, ctx, endpoint, request, status)
		var result factoryapi.WorkerSessionInterruptError
		if err := json.Unmarshal(body, &result); err != nil {
			t.Fatal(err)
		}
		assertMockInterruptFailure(t, name, phase, body, result)
		if attempt == 0 {
			first = result
		} else if !reflect.DeepEqual(first, result) {
			t.Fatalf("retry changed failure: %+v / %+v", first, result)
		}
	}
	assertMockInterruptFailedAdmissionState(t, ctx, baseURL, name, sourceID, successorID, sourceState)
}

func assertMockInterruptFailedAdmissionState(t *testing.T, ctx context.Context, baseURL, name, sourceID, successorID, sourceState string) {
	t.Helper()
	source := support.GetJSON[factoryapi.WorkerSessionObservation](t, baseURL+"/worker-sessions/"+sourceID)
	if string(source.State) != sourceState || source.SuccessorWorkerSessionId != nil {
		t.Fatalf("source after failure = %+v", source)
	}
	listed := support.GetJSON[factoryapi.ListWorkerSessionsResponse](t, baseURL+"/worker-sessions?scope=direct")
	for _, observation := range listed.Sessions {
		if observation.WorkerSessionId == successorID && (name == "input-failure" || observation.State != "FAILED") {
			t.Fatalf("failed admission exposed executing successor: %+v", observation)
		}
	}
	if name == "input-failure" {
		interruptPost(t, ctx, baseURL+"/worker-sessions/"+sourceID+"/terminate", map[string]any{}, http.StatusOK)
		stopped := support.GetJSON[factoryapi.WorkerSessionObservation](t, baseURL+"/worker-sessions/"+sourceID)
		if stopped.State != "TERMINATED" {
			t.Fatalf("source no longer controllable after persistence refusal: %+v", stopped)
		}
	}
}

func assertMockInterruptFailure(t *testing.T, name, phase string, body []byte, result factoryapi.WorkerSessionInterruptError) {
	t.Helper()
	if string(result.Phase) != phase || strings.Contains(string(body), "private-mock-storage") {
		t.Fatalf("failure phase/redaction/admission = %+v", result)
	}
	if name == "input-failure" && (result.Code != "INTERNAL_ERROR" || result.Successor != nil) {
		t.Fatalf("pre-stop refusal = %+v", result)
	}
	if name == "opening-failure" && (result.Code != "WORKER_SESSION_INTERRUPT_SUCCESSOR_ADMISSION_FAILED" ||
		result.Source == nil || result.Source.State != "CANCELED" || result.Successor == nil || result.Successor.State != "FAILED") {
		t.Fatalf("stopped source/failed reserved successor = %s", body)
	}
}

func checkMockInterruptLostAcknowledgement(t *testing.T, ctx context.Context, baseURL, sourceID, successorID string, request any, gate *support.MockWorkerGate) {
	t.Helper()
	endpoint := baseURL + "/worker-sessions/" + sourceID + "/interrupt"
	var first factoryapi.WorkerSessionInterruptResponse
	for attempt := range 2 {
		body := interruptPost(t, ctx, endpoint, request, http.StatusAccepted)
		var result factoryapi.WorkerSessionInterruptResponse
		if err := json.Unmarshal(body, &result); err != nil {
			t.Fatal(err)
		}
		if !result.Accepted || result.Source.State != "CANCELED" || result.SuccessorWorkerSessionId != successorID || strings.Contains(string(body), "private-mock-storage") {
			t.Fatalf("lost acknowledgement outcome = %+v", result)
		}
		if attempt == 0 {
			first = result
		} else if !reflect.DeepEqual(first, result) {
			t.Fatalf("lost acknowledgement repeated admission: %+v / %+v", first, result)
		}
	}
	gate.WaitForArrival(t, 10*time.Second)
	gate.Release()
	// The public observation is the completion signal; the bounded poll crosses
	// the HTTP contract because this gate cannot observe capture finalization.
	final, err := support.WaitForObservation(10*time.Second, func() (factoryapi.WorkerSessionObservation, error) {
		return support.GetJSON[factoryapi.WorkerSessionObservation](t, baseURL+"/worker-sessions/"+successorID), nil
	}, func(observation factoryapi.WorkerSessionObservation) bool { return observation.State == "COMPLETED" })
	if err != nil || final.PredecessorWorkerSessionId == nil || *final.PredecessorWorkerSessionId != sourceID {
		t.Fatalf("successor completion after lost acknowledgement = %+v, %v", final, err)
	}
	var replay factoryapi.WorkerSessionInterruptResponse
	if err := json.Unmarshal(interruptPost(t, ctx, endpoint, request, http.StatusAccepted), &replay); err != nil || !reflect.DeepEqual(first, replay) {
		t.Fatalf("terminal replay changed admission: %+v, %v", replay, err)
	}
}

type mockInterruptFaultStore struct {
	recordings.WorkerRecordingStore
}

func (store *mockInterruptFaultStore) PersistWorkerRecord(ctx context.Context, record recordings.WorkerRecordingRecord) error {
	if record.WorkerSessionID == "opening-failure-successor" {
		return errors.New("private-mock-storage-opening")
	}
	return store.WorkerRecordingStore.PersistWorkerRecord(ctx, record)
}

func (store *mockInterruptFaultStore) PersistWorkerControlInput(ctx context.Context, key recordings.WorkerControlOperationKey, input json.RawMessage) (string, error) {
	if key.RequestID == "input-failure-interrupt" {
		return "", errors.New("private-mock-storage-input")
	}
	return store.WorkerRecordingStore.PersistWorkerControlInput(ctx, key, input)
}

func (store *mockInterruptFaultStore) AdvanceWorkerControlOperation(ctx context.Context, record recordings.WorkerControlOperationRecord, expected uint64) (recordings.WorkerControlOperationRecord, error) {
	accepted, err := store.WorkerRecordingStore.AdvanceWorkerControlOperation(ctx, record, expected)
	if err == nil && record.Operation.RequestID == "completion-ack-interrupt" && record.Operation.Phase == "COMPLETED" {
		return recordings.WorkerControlOperationRecord{}, errors.New("private-mock-storage-completion-ack")
	}
	return accepted, err
}
