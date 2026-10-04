package wire

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	runtime "runtime"

	processcontract "github.com/portpowered/infinite-you/pkg/initializer/process"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	events "github.com/portpowered/infinite-you/pkg/services/events"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
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
	if writer == nil {
		projectRoot, err := provideFactorySessionsWorkingDirectory(edges).Getwd()
		if err != nil {
			return nil, fmt.Errorf("resolve Worker recording project root: %w", err)
		}
		if projectRoot == "" || !filepath.IsAbs(projectRoot) {
			return nil, fmt.Errorf("resolve Worker recording project root: expected a non-empty absolute directory")
		}
		writer, err = recordingswire.NewWorkerRecordingFileWriter(
			platformreplay.NewLocal(runtime.GOOS),
			filepath.Join(projectRoot, ".you-agent-factory", "worker-recordings"),
		)
		if err != nil {
			return nil, err
		}
	}
	return writer, nil
}

// provideWorkerSessionsService constructs the canonical process supervisor.
func provideWorkerSessionsService(
	execution workers.Service,
	eventsService events.Service,
	providerSessions providersessions.Service,
	logger logging.Logger,
	clock factoryruntime.Clock,
	scheduler platformclock.TimerSource,
	recorder recordings.WorkerSessionRecordingService,
) (workersessions.Service, error) {
	return workersessionswire.NewService(execution, eventsService, logger, clock, scheduler, providerSessions, recorder)
}

func provideWorkerAttemptOpener(service workersessions.Service) (factoryruntime.WorkerAttemptOpener, error) {
	opener, ok := service.(factoryruntime.WorkerAttemptOpener)
	if !ok {
		return nil, fmt.Errorf("Worker Sessions runtime attempt capability is required")
	}
	return opener, nil
}
