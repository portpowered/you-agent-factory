package wire

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"path/filepath"
	runtime "runtime"

	"github.com/google/uuid"

	processcontract "github.com/portpowered/infinite-you/pkg/initializer/process"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	events "github.com/portpowered/infinite-you/pkg/services/events"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingswire "github.com/portpowered/infinite-you/pkg/services/recordings/wire"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	workersessionswire "github.com/portpowered/infinite-you/pkg/services/worker_sessions/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func provideWorkerRecordingReader(
	writer recordings.WorkerRecordingWriter,
) (processcontract.WorkerRecordingReader, error) {
	reader, ok := writer.(recordings.WorkerRecordingReader)
	if !ok || reader == nil {
		return nil, fmt.Errorf("compose Worker recording reader: %w", recordings.ErrMissingWorkerRecordingReader)
	}
	return workerRecordingReaderCapability{reader: reader}, nil
}

type workerRecordingReaderCapability struct {
	reader recordings.WorkerRecordingReader
}

func (capability workerRecordingReaderCapability) LoadWorkerRecording(
	ctx context.Context,
	recordingID string,
) (json.RawMessage, error) {
	snapshot, err := capability.reader.LoadWorkerRecording(ctx, recordingID)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return nil, fmt.Errorf("encode Worker recording snapshot: %w", err)
	}
	return append(json.RawMessage(nil), payload...), nil
}

func provideWorkerSessionRecorder(
	eventsService events.Service,
	writer recordings.WorkerRecordingWriter,
	logger logging.Logger,
) (recordings.WorkerSessionRecordingService, error) {
	return recordingswire.NewWorkerSessionRecorder(eventsService, writer, logger)
}

func provideWorkerRecordingWriter(
	edges serviceedges.Edges,
) (recordings.WorkerRecordingWriter, error) {
	writer := edges.WorkerRecordingWriter
	if writer == nil || edges.WorkerRecordingStoreObserver != nil {
		projectRoot, err := provideFactorySessionsWorkingDirectory(edges).Getwd()
		if err != nil {
			return nil, fmt.Errorf("resolve Worker recording project root: %w", err)
		}
		if projectRoot == "" || !filepath.IsAbs(projectRoot) {
			return nil, fmt.Errorf("resolve Worker recording project root: expected a non-empty absolute directory")
		}
		storage := platformreplay.NewLocal(runtime.GOOS)
		store, err := recordingswire.NewWorkerRecordingFileWriter(
			workerCaptureStorage{Local: storage, readFile: edges.RecordingReadFile}, storage, storage, platformclock.Ensure(edges.Clock),
			filepath.Join(projectRoot, ".you-agent-factory", "worker-recordings"),
			uuid.NewString(),
		)
		if err != nil {
			return nil, err
		}
		if edges.WorkerRecordingStoreObserver != nil {
			edges.WorkerRecordingStoreObserver(store)
		}
		if writer == nil {
			writer = store
		}
	}
	return writer, nil
}

type workerCaptureStorage struct {
	platformreplay.Local
	readFile recordings.RecordingReadFile
}

func (storage workerCaptureStorage) ReadFile(path string) ([]byte, error) {
	if storage.readFile != nil {
		return storage.readFile(path)
	}
	return storage.Local.ReadFile(path)
}

// provideWorkerSessionsService constructs the canonical process supervisor.
func provideWorkerHistorySnapshotBudget() *workersessionswire.HistorySnapshotBudget {
	return &workersessionswire.HistorySnapshotBudget{Entropy: rand.Reader}
}

func provideWorkerSessionsService(
	execution workers.Service,
	eventsService events.Service,
	providerSessions providersessions.Service,
	logger logging.Logger,
	clock factoryruntime.Clock,
	scheduler platformclock.TimerSource,
	recorder recordings.WorkerSessionRecordingService,
	writer recordings.WorkerRecordingWriter,
	operations recordings.WorkerControlOperationStore,
	restart recordings.WorkerRestartInputStore,
	snapshots *workersessionswire.HistorySnapshotBudget,
	providerService providers.Service,
) (workersessions.Service, error) {
	// Legacy injected writers still support execution without captured reads.
	reader, _ := writer.(recordings.WorkerCapturedActivityReader)
	return workersessionswire.NewService(execution, eventsService, logger, clock, scheduler, providerSessions, recorder, reader, operations, restart, snapshots, providerService)
}

func provideWorkerAttemptOpener(service workersessions.Service) (factoryruntime.WorkerAttemptOpener, error) {
	opener, ok := service.(factoryruntime.WorkerAttemptOpener)
	if !ok {
		return nil, fmt.Errorf("worker sessions runtime attempt capability is required")
	}
	return opener, nil
}
