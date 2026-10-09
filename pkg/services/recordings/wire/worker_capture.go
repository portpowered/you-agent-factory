package wire

import (
	"context"
	"reflect"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workerrecordingwire "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/worker_capture/wire"
)

// NewWorkerOwnerRecoveryOperation binds the existing writer's boot capability.
// External writers retain their explicit ownership/recovery policy.
func NewWorkerOwnerRecoveryOperation(writer recordings.WorkerRecordingWriter, attribution recordings.WorkerWorkAttributionReader) recordings.WorkerOwnerRecoveryOperation {
	return func(ctx context.Context) error {
		if recovery, ok := writer.(interface{ RecoverWorkerOwners(context.Context) error }); ok {
			if err := recovery.RecoverWorkerOwners(ctx); err != nil {
				return err
			}
		}
		if preparation, ok := attribution.(interface{ PrepareWorkerWorkAttribution(context.Context) error }); ok {
			return preparation.PrepareWorkerWorkAttribution(ctx)
		}
		return ctx.Err()
	}
}

// NewWorkerSessionRecorder constructs the Recordings-owned capture capability
// over the process Events stream and an explicit durable writer. It performs
// no subscription until a Worker Session asks to start recording.
func NewWorkerSessionRecorder(
	eventService events.Service,
	writer recordings.WorkerRecordingWriter,
	logger logging.Logger,
) (recordings.WorkerSessionRecordingService, error) {
	return workerrecordingwire.New(eventService, writer, logger)
}

// NewWorkerRecordingFileWriter constructs the default durable Worker sidecar
// writer from the policy-free replay storage effect selected by Wire.
func NewWorkerRecordingFileWriter(
	storage platformreplay.Storage,
	appender platformreplay.Appender,
	directory platformreplay.DirectoryScanner,
	clock recordings.WorkerCaptureClock,
	root string,
	ownerEpoch string,
) (recordings.WorkerRecordingStore, error) {
	return workerrecordingwire.NewFileWriter(storage, appender, directory, clock, root, ownerEpoch)
}

// NewWorkerControlOperationStore selects the control capability of the same
// recording writer. It performs no IO and never supplies an alternate ledger.
func NewWorkerControlOperationStore(writer recordings.WorkerRecordingWriter) (recordings.WorkerControlOperationStore, error) {
	store, ok := writer.(recordings.WorkerControlOperationStore)
	if !ok || store == nil {
		return nil, recordings.ErrMissingWorkerControlOperationStore
	}
	value := reflect.ValueOf(store)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		if value.IsNil() {
			return nil, recordings.ErrMissingWorkerControlOperationStore
		}
	}
	return store, nil
}

// NewWorkerRestartInputStore binds the existing profile-owned artifact writer.
// Construction is inert and a missing capability fails before admission.
func NewWorkerRestartInputStore(writer recordings.WorkerRecordingWriter) (recordings.WorkerRestartInputStore, error) {
	store, ok := writer.(recordings.WorkerRestartInputStore)
	if !ok || store == nil || (reflect.ValueOf(store).Kind() == reflect.Pointer && reflect.ValueOf(store).IsNil()) {
		return nil, recordings.ErrMissingWorkerRestartInputStore
	}
	return store, nil
}
