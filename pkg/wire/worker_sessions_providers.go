package wire

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	runtime "runtime"
	"strings"

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

// provideWorkerSessionsFactory constructs the one canonical per-session
// Worker Sessions (W4 Runtime dispatch cutover) construction path over the
// already-composed events.Service and logging.Logger singletons. Factory
// Runtime consumes only the returned factoryruntime.WorkerSessionsFactory
// function value, never worker_sessions/wire directly, so composition of the
// peer worker_sessions service stays inside pkg/wire.
func provideWorkerSessionsFactory(
	eventsService events.Service,
	providerSessions providersessions.Service,
	logger logging.Logger,
	recorder recordings.WorkerSessionRecordingService,
) factoryruntime.WorkerSessionsFactory {
	return provideWorkerSessionsFactoryWithRecorder(eventsService, providerSessions, logger, recorder)
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
		root, err := workerRecordingRoot(edges)
		if err != nil {
			return nil, err
		}
		writer, err = recordingswire.NewWorkerRecordingFileWriter(
			platformreplay.NewLocal(runtime.GOOS),
			root,
		)
		if err != nil {
			return nil, err
		}
	}
	return writer, nil
}

const (
	workerRecordingHomeDirectory  = ".you-agent-factory"
	workerRecordingStoreDirectory = "worker-sessions"
)

// workerRecordingRoot places durable Worker history under the operator home.
// An explicitly supplied resolver remains the isolated scenario seam used by
// functional callers and embedding applications.
func workerRecordingRoot(edges serviceedges.Edges) (string, error) {
	if edges.WorkerSessionResolveHomeDirectory == nil {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve Worker Session recording home directory: %w", err)
		}
		return filepath.Join(home, workerRecordingHomeDirectory, workerRecordingStoreDirectory), nil
	}

	home, err := edges.WorkerSessionResolveHomeDirectory()
	if err != nil {
		return "", fmt.Errorf("resolve Worker Session recording home directory: %w", err)
	}
	home = strings.TrimSpace(home)
	if home == "" {
		return "", fmt.Errorf("resolve Worker Session recording home directory: empty path")
	}
	return filepath.Join(home, workerRecordingHomeDirectory, workerRecordingStoreDirectory), nil
}

func provideWorkerSessionsFactoryWithRecorder(
	eventsService events.Service,
	providerSessions providersessions.Service,
	logger logging.Logger,
	recorder recordings.WorkerSessionRecordingService,
) factoryruntime.WorkerSessionsFactory {
	return func(execution workers.Service, clock platformclock.Source) (workersessions.Service, error) {
		return workersessionswire.NewService(execution, eventsService, logger, clock, providerSessions, recorder)
	}
}
