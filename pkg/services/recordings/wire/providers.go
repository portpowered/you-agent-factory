package wire

import (
	"fmt"
	"time"

	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingsinternal "github.com/portpowered/infinite-you/pkg/services/recordings/internal"
	artifactsimpl "github.com/portpowered/infinite-you/pkg/services/recordings/internal/artifacts"
	replayimpl "github.com/portpowered/infinite-you/pkg/services/recordings/internal/replay"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// NewPortableRecordingWriter constructs the portable recording writer selected
// by process-graph composition without importing the transitional artifacts/
// shim.
func NewPortableRecordingWriter(
	makeDirectories recordings.RecordingMakeDirectories,
	createTemporaryFile recordings.RecordingCreateTemporaryFile,
	removePath recordings.RecordingRemovePath,
	renamePath recordings.RecordingRenamePath,
) (recordings.PortableRecordingWriter, error) {
	return artifactsimpl.NewAtomicWriter(makeDirectories, createTemporaryFile, removePath, renamePath)
}

// NewReplayArtifactLoader constructs the replay artifact loader selected by
// process-graph composition without importing the transitional replay/ shim.
func NewReplayArtifactLoader(
	storage platformreplay.Storage,
	decodeFactorySnapshot factorydefinitions.FactorySnapshotJSONDecoder,
) recordings.ReplayArtifactLoader {
	return func(path string) (*recordings.ReplayArtifact, error) {
		return replayimpl.Load(storage, path, decodeFactorySnapshot)
	}
}

// NewProjectionService constructs the Recordings projection capability for
// process-graph composition.
func NewProjectionService() recordings.ProjectionService {
	return recordingsinternal.NewProjectionService()
}

// NewRuntimeLedger constructs a runtime event ledger for process-graph
// composition.
func NewRuntimeLedger(
	topology recordings.InitialStructureSource,
	now func() time.Time,
	streamGenerationID string,
	definitions factorydefinitions.RuntimeDefinitionLookup,
) recordings.RuntimeEventLedger {
	return recordingsinternal.NewRuntimeLedger(topology, now, streamGenerationID, definitions)
}

// NewLifecycleRuntimeRecorder constructs the runtime recorder used while
// opening Factory Session runtime state.
func NewLifecycleRuntimeRecorder(
	flushInterval time.Duration,
	loaded factorydefinitions.LoadedFactorySource,
	now func() time.Time,
	recordPath string,
	captureLoadedFactorySnapshot factorydefinitions.LoadedFactorySnapshotCapturer,
) (recordings.RuntimeRecorder, error) {
	return recordingsinternal.NewLifecycleRuntimeRecorder(
		flushInterval,
		loaded,
		now,
		recordPath,
		captureLoadedFactorySnapshot,
	)
}

// NewReplayClock constructs the replay clock selected from a replay artifact.
func NewReplayClock(artifact *recordings.ReplayArtifact) recordings.Clock {
	return recordingsinternal.NewReplayClock(artifact)
}

// NewReplayExecution constructs replay execution collaborators from a replay
// artifact and the canonical definition decoders selected by the process graph.
func NewReplayExecution(
	artifact *recordings.ReplayArtifact,
	decodeFactorySnapshot factorydefinitions.FactorySnapshotJSONDecoder,
	decodeRuntimeConfig factorydefinitions.ReplayRuntimeConfigDecoder,
) (
	workers.Runner,
	workers.CommandRunner,
	[]recordings.ReplayHook,
	recordings.CompletionDeliveryPlanner,
	error,
) {
	return recordingsinternal.NewReplayExecution(
		artifact,
		decodeFactorySnapshot,
		decodeRuntimeConfig,
	)
}

// NewServiceWithProjection constructs the Recordings root from a caller-owned
// ledger and projection. Process composition uses this when runtime opening
// shares one projection instance across reconnect validation and root assembly.
func NewServiceWithProjection(
	ledger recordings.Ledger,
	projection recordings.ProjectionService,
	targets recordings.LiveRecordingTargetPlanner,
	writeFile func(string, []byte) error,
	clocks ...recordings.RecordingClock,
) (recordings.Service, error) {
	if ledger == nil {
		return nil, fmt.Errorf("construct Recordings: ledger is required")
	}
	if projection == nil {
		return nil, fmt.Errorf("construct Recordings: projection is required")
	}
	if writeFile == nil {
		return nil, fmt.Errorf("construct Recordings: snapshot write function is required")
	}
	writer := recordingsinternal.NewReplayRecordingSnapshotWriter(writeFile)
	tickers := recordingsinternal.NewRecordingFlushTickerFactory()
	publication, err := recordingsinternal.NewPortableArtifactPublication()
	if err != nil {
		return nil, err
	}
	service := recordingsinternal.NewServiceWithLifecycleEffects(
		ledger,
		projection,
		targets,
		writer,
		tickers,
		publication,
		clocks...,
	)
	if service == nil {
		return nil, fmt.Errorf("construct Recordings: implementation rejected its dependencies")
	}
	return service, nil
}
