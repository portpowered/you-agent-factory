package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

// Keep the production journal and its sync-confirmed reads. The controlled
// external store boundary loses only the selected phase acknowledgement.
type interruptPhaseAckStore struct {
	recordings.WorkerRecordingStore
}

func (store *interruptPhaseAckStore) PersistWorkerRecord(ctx context.Context, record recordings.WorkerRecordingRecord) error {
	if strings.HasPrefix(record.WorkerSessionID, "interrupt-admission-failure-") {
		return errors.New("private-successor-opening-detail")
	}
	return store.WorkerRecordingStore.PersistWorkerRecord(ctx, record)
}

func (store *interruptPhaseAckStore) PersistWorkerControlInput(ctx context.Context, key recordings.WorkerControlOperationKey, input json.RawMessage) (string, error) {
	if strings.Contains(key.RequestID, "interrupt-input-write-failure") {
		return "", errors.New("private-input-write-detail")
	}
	return store.WorkerRecordingStore.PersistWorkerControlInput(ctx, key, input)
}

func (store *interruptPhaseAckStore) ReadWorkerControlInput(ctx context.Context, key recordings.WorkerControlOperationKey, ref string) (json.RawMessage, error) {
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
	for _, name := range []string{"interrupt-ack-intent", "interrupt-ack-source", "interrupt-ack-admission", "interrupt-ack-completion"} {
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
