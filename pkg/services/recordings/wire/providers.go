package wire

import (
	"context"
	"fmt"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingsinternal "github.com/portpowered/infinite-you/pkg/services/recordings/internal"
	artifactsimpl "github.com/portpowered/infinite-you/pkg/services/recordings/internal/artifacts"
	replayimpl "github.com/portpowered/infinite-you/pkg/services/recordings/internal/replay"
	historicalquery "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/historical_query"
	historicalquerywire "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/historical_query/wire"
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

// NewReplayInputLoader constructs the path-based, pre-ledger
// recordings.ReplayInputLoader implementation selected by
// process-graph composition, composing the existing portable-recording
// decoder/validator with the existing legacy replay artifact loader behind
// the one Recordings-owned replay-input capability so callers no longer
// combine a raw file reader, the aliased portable-recording decoder/
// validator, and the legacy loader themselves.
//
// This capability contains only LoadReplayInput because it is constructed and
// injected before a Factory Session ledger exists. Ledger-scoped artifact
// behavior remains on RecordingReplayArtifacts, whose implementation has the
// required ledger and publication dependencies.
func NewReplayInputLoader(
	readFile recordings.RecordingReadFile,
	loadLegacy recordings.ReplayArtifactLoader,
	logger logging.Logger,
	openFiles ...recordings.RecordingOpenFile,
) recordings.ReplayInputLoader {
	var openFile recordings.RecordingOpenFile
	if len(openFiles) > 0 {
		openFile = openFiles[0]
	}
	loadLegacyMetadata := func(path string) (recordings.ReplayInputMetadata, error) {
		return replayimpl.LoadMetadata(openFile, path)
	}
	return recordingsinternal.NewReplayInputLoader(
		readFile, openFile, loadLegacy, loadLegacyMetadata, logger,
	)
}

// NewRuntimeRoot constructs the singular process-scoped Recordings authority.
// Runtime ledgers, projection use, replay collaborators, and recording
// lifecycle state are acquired through the returned root's RuntimeOpening
// capability rather than through opening-local constructors.
func NewRuntimeRoot(
	targets recordings.LiveRecordingTargetPlanner,
	writeFile func(string, []byte) error,
	makeDirectories recordings.RecordingMakeDirectories,
	createTemporaryFile recordings.RecordingCreateTemporaryFile,
	removePath recordings.RecordingRemovePath,
	renamePath recordings.RecordingRenamePath,
	readFile recordings.RecordingReadFile,
	captureSnapshot factorydefinitions.LoadedFactorySnapshotCapturer,
	decodeSnapshot factorydefinitions.FactorySnapshotJSONDecoder,
	decodeRuntimeConfig factorydefinitions.ReplayRuntimeConfigDecoder,
	replayInputs recordings.ReplayInputLoader,
	logger logging.Logger,
	clocks ...recordings.RecordingClock,
) (recordings.Service, error) {
	return newRuntimeRoot(
		targets,
		writeFile,
		nil,
		makeDirectories,
		createTemporaryFile,
		removePath,
		renamePath,
		readFile,
		captureSnapshot,
		decodeSnapshot,
		decodeRuntimeConfig,
		replayInputs,
		logger,
		clocks...,
	)
}

// NewRuntimeRootWithAppend constructs the process-scoped Recordings authority
// with the separate append effect required by new v2 JSONL recordings.
func NewRuntimeRootWithAppend(
	targets recordings.LiveRecordingTargetPlanner,
	writeFile func(string, []byte) error,
	appendFile func(string, []byte) error,
	makeDirectories recordings.RecordingMakeDirectories,
	createTemporaryFile recordings.RecordingCreateTemporaryFile,
	removePath recordings.RecordingRemovePath,
	renamePath recordings.RecordingRenamePath,
	readFile recordings.RecordingReadFile,
	captureSnapshot factorydefinitions.LoadedFactorySnapshotCapturer,
	decodeSnapshot factorydefinitions.FactorySnapshotJSONDecoder,
	decodeRuntimeConfig factorydefinitions.ReplayRuntimeConfigDecoder,
	replayInputs recordings.ReplayInputLoader,
	logger logging.Logger,
	clocks ...recordings.RecordingClock,
) (recordings.Service, error) {
	if appendFile == nil {
		return NewRuntimeRoot(
			targets,
			writeFile,
			makeDirectories,
			createTemporaryFile,
			removePath,
			renamePath,
			readFile,
			captureSnapshot,
			decodeSnapshot,
			decodeRuntimeConfig,
			replayInputs,
			logger,
			clocks...,
		)
	}
	return newRuntimeRoot(
		targets,
		writeFile,
		appendFile,
		makeDirectories,
		createTemporaryFile,
		removePath,
		renamePath,
		readFile,
		captureSnapshot,
		decodeSnapshot,
		decodeRuntimeConfig,
		replayInputs,
		logger,
		clocks...,
	)
}

func newRuntimeRoot(
	targets recordings.LiveRecordingTargetPlanner,
	writeFile func(string, []byte) error,
	appendFile func(string, []byte) error,
	makeDirectories recordings.RecordingMakeDirectories,
	createTemporaryFile recordings.RecordingCreateTemporaryFile,
	removePath recordings.RecordingRemovePath,
	renamePath recordings.RecordingRenamePath,
	readFile recordings.RecordingReadFile,
	captureSnapshot factorydefinitions.LoadedFactorySnapshotCapturer,
	decodeSnapshot factorydefinitions.FactorySnapshotJSONDecoder,
	decodeRuntimeConfig factorydefinitions.ReplayRuntimeConfigDecoder,
	replayInputs recordings.ReplayInputLoader,
	logger logging.Logger,
	clocks ...recordings.RecordingClock,
) (recordings.Service, error) {
	publication, err := recordingsinternal.NewPortableArtifactPublication(
		makeDirectories,
		createTemporaryFile,
		removePath,
		renamePath,
		readFile,
	)
	if err != nil {
		return nil, fmt.Errorf("construct Recordings publication: %w", err)
	}
	historicalQuery := historicalquerywire.NewService(
		readFile,
		recordingsinternal.NewProjectionService(),
	)
	root := recordingsinternal.NewRuntimeRootWithHistoricalQueryAndAppender(
		targets,
		writeFile,
		appendFile,
		readFile,
		publication,
		captureSnapshot,
		decodeSnapshot,
		decodeRuntimeConfig,
		replayInputs,
		logger,
		historicalQuery,
		clocks...,
	)
	if root == nil {
		return nil, fmt.Errorf("construct Recordings: runtime root rejected its dependencies")
	}
	return root, nil
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
	recordingID string,
	recordPath string,
	captureLoadedFactorySnapshot factorydefinitions.LoadedFactorySnapshotCapturer,
) (recordings.RuntimeRecorder, error) {
	return recordingsinternal.NewLifecycleRuntimeRecorder(
		flushInterval,
		loaded,
		now,
		recordingID,
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
	providers.Service,
	platformprocess.CommandRunner,
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
	makeDirectories recordings.RecordingMakeDirectories,
	createTemporaryFile recordings.RecordingCreateTemporaryFile,
	removePath recordings.RecordingRemovePath,
	renamePath recordings.RecordingRenamePath,
	readFile recordings.RecordingReadFile,
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
	return NewServiceWithProjectionAndEffects(
		ledger,
		projection,
		targets,
		writeFile,
		makeDirectories,
		createTemporaryFile,
		removePath,
		renamePath,
		readFile,
		clocks...,
	)
}

// NewServiceWithProjectionAndEffects constructs the Recordings root with the
// exact portable-artifact filesystem effects selected by the application
// graph. This owner wire adapts those effects into the private artifact
// publication capability and selects no host defaults.
func NewServiceWithProjectionAndEffects(
	ledger recordings.Ledger,
	projection recordings.ProjectionService,
	targets recordings.LiveRecordingTargetPlanner,
	writeFile func(string, []byte) error,
	makeDirectories recordings.RecordingMakeDirectories,
	createTemporaryFile recordings.RecordingCreateTemporaryFile,
	removePath recordings.RecordingRemovePath,
	renamePath recordings.RecordingRenamePath,
	readFile recordings.RecordingReadFile,
	clocks ...recordings.RecordingClock,
) (recordings.Service, error) {
	if ledger == nil {
		return nil, fmt.Errorf("construct Recordings: ledger is required")
	}
	if projection == nil {
		return nil, fmt.Errorf("construct Recordings: projection is required")
	}
	publication, err := recordingsinternal.NewPortableArtifactPublication(
		makeDirectories,
		createTemporaryFile,
		removePath,
		renamePath,
		readFile,
	)
	if err != nil {
		return nil, fmt.Errorf("construct Recordings publication: %w", err)
	}
	historicalQuery := historicalquerywire.NewService(readFile, projection)
	return newServiceWithProjection(
		ledger,
		projection,
		targets,
		writeFile,
		publication,
		historicalQuery,
		false,
		clocks...,
	)
}

type portableArtifactPublication interface {
	Publish(context.Context, string, []byte) error
	Read(context.Context, string) ([]byte, error)
}

func newServiceWithProjection(
	ledger recordings.Ledger,
	projection recordings.ProjectionService,
	targets recordings.LiveRecordingTargetPlanner,
	writeFile func(string, []byte) error,
	publication portableArtifactPublication,
	historicalQuery historicalquery.Service,
	requireWriter bool,
	clocks ...recordings.RecordingClock,
) (recordings.Service, error) {
	if ledger == nil {
		return nil, fmt.Errorf("construct Recordings: ledger is required")
	}
	if projection == nil {
		return nil, fmt.Errorf("construct Recordings: projection is required")
	}
	if requireWriter && writeFile == nil {
		return nil, fmt.Errorf("construct Recordings: snapshot write function is required")
	}
	var writer recordings.RecordingSnapshotWriter
	var tickers recordings.RecordingFlushTickerFactory
	if writeFile != nil {
		writer = recordingsinternal.NewReplayRecordingSnapshotWriter(writeFile)
		tickers = recordingsinternal.NewRecordingFlushTickerFactory()
	}
	service := recordingsinternal.NewServiceWithLifecycleEffectsAndHistoricalQuery(
		ledger,
		projection,
		targets,
		writer,
		tickers,
		publication,
		historicalQuery,
		clocks...,
	)
	if service == nil {
		return nil, fmt.Errorf("construct Recordings: implementation rejected its dependencies")
	}
	return service, nil
}
