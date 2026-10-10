package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// Keep the production journal and its sync-confirmed reads. The controlled
// external store boundary loses only the selected phase acknowledgement.
type interruptPhaseAckStore struct {
	recordings.WorkerRecordingStore
	inputAcknowledgements sync.Map
	requesterPreparations sync.Map
}

func (store *interruptPhaseAckStore) PersistWorkerRecord(ctx context.Context, record recordings.WorkerRecordingRecord) error {
	if strings.HasPrefix(record.WorkerSessionID, "t7-degraded-") && record.Record.ID.Position >= 3 {
		return errors.New("private-t7-capture-failure")
	}
	if record.WorkerSessionID == "revival-incomplete-capture" && record.Record.ID.Position >= 3 {
		return errors.New("private-revival-capture-failure")
	}
	if strings.HasPrefix(record.WorkerSessionID, "interrupt-admission-failure-") {
		return errors.New("private-successor-opening-detail")
	}
	err := store.WorkerRecordingStore.PersistWorkerRecord(ctx, record)
	if err == nil && record.WorkerSessionID == "continuation-opening-ack-lost-successor" {
		return errors.New("private-continuation-opening-acknowledgement-detail")
	}
	return err
}

func (store *interruptPhaseAckStore) PersistWorkerControlInput(ctx context.Context, key recordings.WorkerControlOperationKey, input json.RawMessage) (string, error) {
	if strings.HasPrefix(key.RequestID, "continue/continuation-input-write-failure") {
		return "", errors.New("private-continuation-sync-detail")
	}
	if strings.Contains(key.RequestID, "interrupt-input-write-failure") {
		return "", errors.New("private-input-write-detail")
	}
	ref, err := store.WorkerRecordingStore.PersistWorkerControlInput(ctx, key, input)
	if err == nil && strings.Contains(key.RequestID, "interrupt-ack-input-unsynced") {
		return ref, errors.New("private-input-sync-unconfirmed")
	}
	if err == nil && strings.Contains(key.RequestID, "interrupt-ack-input-missing-ref") {
		return "", errors.New("private-input-reference-unavailable")
	}
	if err == nil && strings.Contains(key.RequestID, "interrupt-ack-input") {
		if _, lost := store.inputAcknowledgements.LoadOrStore(key, true); !lost {
			return ref, errors.New("private-interrupt-input-acknowledgement-detail")
		}
	}
	if err == nil && strings.HasPrefix(key.RequestID, "continue/continuation-input-ack-lost") {
		return "", errors.New("private-continuation-acknowledgement-detail")
	}
	return ref, err
}

// F7-C10: the actual Recordings store syncs the tuple, but the storage edge
// loses the response. Public continuation still admits exactly one successor.
func TestContinuationInputLostAcknowledgementAdmitsOneSuccessor(t *testing.T) {
	t.Parallel()
	fixture := ensureInvokeContinuePackageFixture(t)
	scenario := fixture.scenario(t, "continuation-input-ack-lost")
	t.Cleanup(func() { scenario.close(t) })
	path := filepath.Join(scenario.workingDirectory, "execution.json")
	writeInvokeContinueExecutionSpec(t, path, invokeContinueExecutionSpec{
		requestID: "continuation-input-ack-lost-start", workerSessionID: "continuation-input-ack-lost-source", dispatchID: "continuation-input-ack-lost-attempt",
		factorySessionID: scenario.session.id, workingDirectory: scenario.workingDirectory, userMessage: "initial input",
	})
	invoke := support.FakeInputs(t.Context(), []string{"you", "--json", "worker-sessions", "invoke", "--execution", path})
	invoke.Input.Env, invoke.Input.WorkingDirectory = scenario.environment(), scenario.workingDirectory
	if err := fixture.process.Execute(invoke.Input); err != nil {
		t.Fatalf("source invocation: %v %s %s", err, invoke.Stdout(), invoke.Stderr())
	}
	for range 2 {
		request := support.FakeInputs(t.Context(), []string{"you", "--json", "worker-sessions", "continue", "continuation-input-ack-lost-source",
			"--request-id", "continuation-input-ack-lost-request", "--successor-worker-session-id", "continuation-input-ack-lost-successor", "--user-message", "follow up"})
		request.Input.Env, request.Input.WorkingDirectory = scenario.environment(), scenario.workingDirectory
		if err := fixture.process.Execute(request.Input); err != nil {
			t.Fatalf("committed input acknowledgement: %v %s %s", err, request.Stdout(), request.Stderr())
		}
		var result directWorkerSessionCLIResult
		decodeDirectWorkerSessionResult(t, request.Stdout(), &result)
		if !result.Accepted || result.State != "COMPLETED" || result.SuccessorWorkerSessionID != "continuation-input-ack-lost-successor" ||
			!strings.Contains(result.Output, "continued COMPLETE") || scenario.providerRunner.CallCount() != 2 {
			t.Fatalf("lost acknowledgement repeated admission or lost output: %#v calls=%d", result, scenario.providerRunner.CallCount())
		}
		if strings.Contains(request.Stdout()+request.Stderr(), "private-continuation") {
			t.Fatal("storage diagnostic leaked")
		}
	}
}

// The direct source completes normally; only its continuation-input sync is
// refused at the external storage edge. Repeated CLI requests cannot admit a
// provider or leak the storage diagnostic.
func TestContinuationInputSyncFailurePreventsSuccessorAdmission(t *testing.T) {
	t.Parallel()
	fixture := ensureInvokeContinuePackageFixture(t)
	scenario := fixture.scenario(t, "continuation-input-write-failure")
	t.Cleanup(func() { scenario.close(t) })
	executionPath := filepath.Join(scenario.workingDirectory, "execution.json")
	writeInvokeContinueExecutionSpec(t, executionPath, invokeContinueExecutionSpec{
		requestID: "continuation-input-write-failure-start", workerSessionID: "continuation-input-write-failure-source", dispatchID: "continuation-input-write-failure-attempt",
		factorySessionID: scenario.session.id, workingDirectory: scenario.workingDirectory, userMessage: "initial input",
	})
	invoke := support.FakeInputs(t.Context(), []string{"you", "--json", "worker-sessions", "invoke", "--execution", executionPath})
	invoke.Input.Env, invoke.Input.WorkingDirectory = scenario.environment(), scenario.workingDirectory
	if err := fixture.process.Execute(invoke.Input); err != nil {
		t.Fatalf("source invocation: %v\n%s\n%s", err, invoke.Stdout(), invoke.Stderr())
	}
	for range 2 {
		inputs := support.FakeInputs(t.Context(), []string{"you", "--json", "worker-sessions", "continue", "continuation-input-write-failure-source",
			"--request-id", "continuation-input-write-failure-request", "--successor-worker-session-id", "continuation-input-write-failure-successor", "--user-message", "follow up", "--async"})
		inputs.Input.Env, inputs.Input.WorkingDirectory = scenario.environment(), scenario.workingDirectory
		if err := fixture.process.Execute(inputs.Input); err == nil {
			t.Fatal("unsynced continuation admitted successor")
		}
		assertDirectWorkerSessionCLIError(t, inputs, "WORKER_SESSION_CONTINUATION_ADMISSION_FAILED")
		if strings.Contains(inputs.Stdout()+inputs.Stderr(), "private-continuation-sync-detail") {
			t.Fatal("private persistence error leaked")
		}
	}
	if calls := scenario.providerRunner.CallCount(); calls != 1 {
		t.Fatalf("provider calls = %d, want source only", calls)
	}
}

func (store *interruptPhaseAckStore) ReadWorkerControlInput(ctx context.Context, key recordings.WorkerControlOperationKey, ref string) (json.RawMessage, error) {
	if strings.Contains(key.RequestID, "interrupt-input-corrupt") {
		return json.RawMessage(`{"replacementMessage":"private-corrupt-input-detail"}`), nil
	}
	if strings.Contains(key.RequestID, "interrupt-input-read-failure") {
		return nil, errors.New("private-input-read-detail")
	}
	return store.WorkerRecordingStore.ReadWorkerControlInput(ctx, key, ref)
}

func (store *interruptPhaseAckStore) BeginWorkerControlOperation(ctx context.Context, record recordings.WorkerControlOperationRecord) (recordings.WorkerControlOperationRecord, bool, error) {
	if record.Operation.Action == "interrupt" && strings.Contains(record.Operation.RequestID, "interrupt-intent-failure") {
		return recordings.WorkerControlOperationRecord{}, false, errors.New("private-intent-detail")
	}
	accepted, created, err := store.WorkerRecordingStore.BeginWorkerControlOperation(ctx, record)
	if err == nil && strings.Contains(record.Operation.RequestID, "interrupt-ack-intent") {
		return recordings.WorkerControlOperationRecord{}, false, errors.New("private-intent-acknowledgement-detail")
	}
	return accepted, created, err
}

func (store *interruptPhaseAckStore) LoadWorkerControlOperation(ctx context.Context, key recordings.WorkerControlOperationKey) (recordings.WorkerControlOperationRecord, error) {
	record, err := store.WorkerRecordingStore.LoadWorkerControlOperation(ctx, key)
	if err == nil && strings.Contains(key.RequestID, "interrupt-ack-intent-disputed") {
		record.Operation.InputDigest = strings.Repeat("0", 64)
	}
	return record, err
}

func (store *interruptPhaseAckStore) AdvanceWorkerControlOperation(ctx context.Context, record recordings.WorkerControlOperationRecord, expected uint64) (recordings.WorkerControlOperationRecord, error) {
	accepted, err := store.WorkerRecordingStore.AdvanceWorkerControlOperation(ctx, record, expected)
	if err != nil {
		return accepted, err
	}
	if strings.Contains(record.Operation.RequestID, "interrupt-phase-conflict") && record.Operation.Phase == "SOURCE_STOPPED" {
		return recordings.WorkerControlOperationRecord{}, recordings.ErrWorkerControlConflict
	}
	for name, phase := range map[string]string{
		"interrupt-ack-source": "SOURCE_STOPPED", "interrupt-ack-admission": "SUCCESSOR_ADMITTED", "interrupt-ack-completion": "COMPLETED",
	} {
		if strings.Contains(record.Operation.RequestID, name) && record.Operation.Phase == phase {
			return recordings.WorkerControlOperationRecord{}, errors.New("private-acknowledgement-detail")
		}
	}
	return accepted, nil
}

func TestInterruptUncertainAcknowledgementKeepsPublicOutcome(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"interrupt-ack-input", "interrupt-ack-intent", "interrupt-ack-source", "interrupt-ack-admission", "interrupt-ack-completion"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
			defer cancel()
			scenario := newS8InterruptScenario(t, ctx, name)
			scenario.ids.interruptRequest = name + "-" + scenario.ids.interruptRequest
			defer scenario.runner.releaseAll()
			ids := scenario.ids
			invokeS8RemoteWorker(t, ctx, scenario.manager, scenario.env, scenario.repositoryA.path, scenario.serverURL, s8RemoteWorkerInvocation{
				requestID: ids.requestA, workerSessionID: ids.workerA, dispatchID: ids.dispatchA,
				factorySessionID: scenario.session.id, repository: scenario.repositoryA.path, workID: ids.workA, message: s8MessageA,
			})
			scenario.runner.waitStarted(t, scenario.repositoryA.path, s8InterruptCallAInitial, scenario.fixture.router.requests)
			first := postS8Interrupt(t, ctx, scenario.serverURL, ids.workerA, ids.interruptRequest, ids.successor, s8ReplacementMessage)
			assertS8APIInterruptAdmission(t, first, ids)
			scenario.runner.waitCanceled(t, scenario.repositoryA.path, s8InterruptCallAInitial)
			scenario.runner.waitStarted(t, scenario.repositoryA.path, s8InterruptCallASuccessor, scenario.fixture.router.requests)
			scenario.runner.assertOrder(t, "start:"+s8InterruptCallAInitial, "cancel:"+s8InterruptCallAInitial, "start:"+s8InterruptCallASuccessor)
			cli := interruptS8RemoteWorker(t, ctx, scenario.manager, scenario.env, scenario.repositoryA.path, scenario.serverURL, ids.workerA, ids.interruptRequest, ids.successor)
			replayed := postS8Interrupt(t, ctx, scenario.serverURL, ids.workerA, ids.interruptRequest, ids.successor, s8ReplacementMessage)
			if !reflect.DeepEqual(first, replayed) || !reflect.DeepEqual(s8InterruptResultFromAPI(first), cli) || scenario.runner.CallCount() != 2 || scenario.runner.cancellationCount(s8InterruptCallAInitial) != 1 {
				t.Fatalf("uncertain acknowledgement changed public outcome: first=%#v replay=%#v CLI=%#v", first, replayed, cli)
			}
			scenario.runner.release(t, scenario.repositoryA.path, s8InterruptCallASuccessor)
			_ = replayS8RemoteWorker(t, ctx, scenario.manager, scenario.env, scenario.repositoryA.path, scenario.serverURL, ids.successor)
			assertS8WorkNotAdvanced(t, scenario.fixture, scenario.session.id, ids.workA)
			scenario.close(t)
		})
	}
}

func (store *interruptPhaseAckStore) SaveWorkerRestartRecipe(ctx context.Context, target recordings.WorkerControlTarget, request workers.WorkstationDispatchRequest, metadata ...json.RawMessage) error {
	if target.WorkerSessionID == "restart-recipe-write-failure" || target.WorkerSessionID == "restart-recipe-unsafe" ||
		target.WorkerSessionID == "continuation-unadmitted-recipe-successor" || strings.HasPrefix(target.WorkerSessionID, "requester-recipe-failed-") {
		return errors.New("private-recipe-sync-detail")
	}
	err := store.WorkerRecordingStore.SaveWorkerRestartRecipe(ctx, target, request, metadata...)
	if err != nil {
		return err
	}
	if value, exists := store.requesterPreparations.Load(filepath.Base(request.Execution.WorkingDirectory)); exists {
		gate := value.(*requesterPreparationGate)
		gate.once.Do(func() {
			gate.workerID, gate.sessionID = target.WorkerSessionID, target.FactorySessionID
			close(gate.prepared)
		})
		select {
		case <-gate.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (store *interruptPhaseAckStore) ReadWorkerRestartRecipe(ctx context.Context, target recordings.WorkerControlTarget) (workers.WorkstationDispatchRequest, error) {
	if strings.HasPrefix(target.WorkerSessionID, "interrupt-recipe-read-failure-") {
		return workers.WorkstationDispatchRequest{}, errors.New("private-recipe-read-detail")
	}
	execution, err := store.WorkerRecordingStore.ReadWorkerRestartRecipe(ctx, target)
	if strings.HasPrefix(target.WorkerSessionID, "interrupt-recipe-unsafe-read-") {
		execution.Execution.EnvVars = map[string]string{"API_KEY": "private-recipe-read-detail"}
	}
	return execution, err
}

func (store *interruptPhaseAckStore) LookupPreparedWorkerContinuationSource(ctx context.Context, target recordings.WorkerControlTarget) (recordings.WorkerContinuationSource, error) {
	if target.WorkerSessionID == "revival-missing-recipe" {
		return recordings.WorkerContinuationSource{}, errors.New("private-revival-missing-recipe")
	}
	source, err := store.WorkerRecordingStore.LookupPreparedWorkerContinuationSource(ctx, target)
	if target.WorkerSessionID == "revival-stale-recipe" {
		source.Execution.Execution.Dispatch.DispatchID = "stale-physical-attempt"
	}
	return source, err
}

// Preserve the real store's bounded read and activation capabilities while
// keeping this decorator's controlled write/activity faults.
func (store *interruptPhaseAckStore) LookupWorkerSessionSummary(ctx context.Context, id string) (recordings.WorkerCapturedSummary, error) {
	return store.WorkerRecordingStore.(recordings.WorkerCapturedSummaryReader).LookupWorkerSessionSummary(ctx, id)
}

func (store *interruptPhaseAckStore) RecoverWorkerOwners(ctx context.Context) error {
	return store.WorkerRecordingStore.(interface{ RecoverWorkerOwners(context.Context) error }).RecoverWorkerOwners(ctx)
}

func (store *interruptPhaseAckStore) ValidateWorkerContinuationSource(ctx context.Context, target recordings.WorkerControlTarget) (recordings.WorkerContinuationSource, error) {
	source, err := store.LookupPreparedWorkerContinuationSource(ctx, target)
	if err != nil || source.Execution.Execution.Dispatch.DispatchID != target.ExpectedAttemptID {
		return source, err
	}
	return store.WorkerRecordingStore.ValidateWorkerContinuationSource(ctx, target)
}
