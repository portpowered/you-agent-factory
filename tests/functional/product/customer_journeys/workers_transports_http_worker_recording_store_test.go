package customer_journeys_test

import (
	"context"
	"encoding/json"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// These capture-only fixtures explicitly refuse durable controls. Their
// snapshot store supplies no sync acknowledgement or execution authority.
type unavailableWorkerControlStore struct{}

func (unavailableWorkerControlStore) ReadWorkerContinuationInput(context.Context, recordings.WorkerControlOperationKey) (json.RawMessage, error) {
	return nil, recordings.ErrWorkerRecordingPersistence
}

func (unavailableWorkerControlStore) ValidateWorkerRestartRecipe(context.Context, string, workers.WorkstationDispatchRequest) error {
	return recordings.ErrMissingWorkerRestartInputStore
}

func (unavailableWorkerControlStore) SaveWorkerRestartRecipe(context.Context, recordings.WorkerControlTarget, workers.WorkstationDispatchRequest) error {
	return recordings.ErrInvalidRecordingRedactionRequest
}

func (unavailableWorkerControlStore) ReadWorkerRestartRecipe(context.Context, recordings.WorkerControlTarget) (workers.WorkstationDispatchRequest, error) {
	return workers.WorkstationDispatchRequest{}, recordings.ErrWorkerRecordingPersistence
}

func (unavailableWorkerControlStore) ReadWorkerContinuationSource(context.Context, recordings.WorkerControlTarget) (recordings.WorkerContinuationSource, error) {
	return recordings.WorkerContinuationSource{}, recordings.ErrWorkerRecordingPersistence
}

func (unavailableWorkerControlStore) BeginWorkerControlOperation(context.Context, recordings.WorkerControlOperationRecord) (recordings.WorkerControlOperationRecord, bool, error) {
	return recordings.WorkerControlOperationRecord{}, false, recordings.ErrWorkerRecordingPersistence
}
func (unavailableWorkerControlStore) AdvanceWorkerControlOperation(context.Context, recordings.WorkerControlOperationRecord, uint64) (recordings.WorkerControlOperationRecord, error) {
	return recordings.WorkerControlOperationRecord{}, recordings.ErrWorkerRecordingPersistence
}
func (unavailableWorkerControlStore) LoadWorkerControlOperation(context.Context, recordings.WorkerControlOperationKey) (recordings.WorkerControlOperationRecord, error) {
	return recordings.WorkerControlOperationRecord{}, recordings.ErrWorkerRecordingPersistence
}
func (unavailableWorkerControlStore) ListWorkerControlOperations(context.Context, recordings.WorkerControlTarget) ([]recordings.WorkerControlOperationRecord, error) {
	return nil, recordings.ErrWorkerRecordingPersistence
}
func (unavailableWorkerControlStore) PersistWorkerControlInput(context.Context, recordings.WorkerControlOperationKey, json.RawMessage) (string, error) {
	return "", recordings.ErrWorkerRecordingPersistence
}
func (unavailableWorkerControlStore) ReadWorkerControlInput(context.Context, recordings.WorkerControlOperationKey, string) (json.RawMessage, error) {
	return nil, recordings.ErrWorkerRecordingPersistence
}
