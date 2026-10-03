package wire

import (
	"context"
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
	artifactsexport "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/artifacts_export"
	artifactsexportwire "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/artifacts_export/wire"
	canonicalledger "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/canonical_ledger"
	canonicalledgerwire "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/canonical_ledger/wire"
	historicalquery "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/historical_query"
	historicalquerywire "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/historical_query/wire"
	recordinglifecycle "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/recording_lifecycle"
	recordinglifecyclewire "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/recording_lifecycle/wire"
	recordingsreplay "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/replay"
	replaywire "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/replay/wire"
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

type RecordingLifecycleOwner = recordinglifecycle.Service
type ArtifactsExportOwner = artifactsexport.Service
type ReplayOwner = recordingsreplay.Service
type CanonicalLedgerOwner = canonicalledger.Service
type HistoricalQueryOwner = historicalquery.Service
type RuntimeLedgerRouter = recordingsinternal.RuntimeLedgerRouter

type PortableArtifactPublication interface {
	Publish(context.Context, string, []byte) error
	Read(context.Context, string) ([]byte, error)
}

func NewRuntimeLedgerRouter(clock recordings.RecordingClock) *RuntimeLedgerRouter {
	return recordingsinternal.NewRuntimeLedgerRouter(clock)
}

func RuntimeLedger(router *RuntimeLedgerRouter) recordings.Ledger { return router }

func NewRecordingLifecycleOwner(
	targets recordings.LiveRecordingTargetPlanner,
	writer recordings.RecordingSnapshotWriter,
	tickers recordings.RecordingFlushTickerFactory,
	clock recordings.RecordingClock,
) RecordingLifecycleOwner {
	return recordinglifecyclewire.NewService(targets, writer, tickers, clock)
}

func NewCanonicalLedgerOwner(ledger recordings.Ledger) CanonicalLedgerOwner {
	return canonicalledgerwire.NewService(ledger)
}

func NewArtifactsExportOwner(lifecycle RecordingLifecycleOwner, publication PortableArtifactPublication) ArtifactsExportOwner {
	return artifactsexportwire.NewService(lifecycle, publication)
}

func NewReplayOwner(
	lifecycle RecordingLifecycleOwner,
	projection recordings.ProjectionService,
	readFile recordings.RecordingReadFile,
	decodeFactorySnapshot factorydefinitions.FactorySnapshotJSONDecoder,
) ReplayOwner {
	return replaywire.NewService(lifecycle, projection, readFile, decodeFactorySnapshot)
}

func NewHistoricalQueryOwner(readFile recordings.RecordingReadFile, projection recordings.ProjectionService) HistoricalQueryOwner {
	return historicalquerywire.NewService(readFile, projection)
}

func NewPortableArtifactPublication(
	makeDirectories recordings.RecordingMakeDirectories,
	createTemporaryFile recordings.RecordingCreateTemporaryFile,
	removePath recordings.RecordingRemovePath,
	renamePath recordings.RecordingRenamePath,
	readFile recordings.RecordingReadFile,
) (PortableArtifactPublication, error) {
	return recordingsinternal.NewPortableArtifactPublication(makeDirectories, createTemporaryFile, removePath, renamePath, readFile)
}

func NewReplayRecordingSnapshotWriter(
	writeFile func(string, []byte) error,
	appendFile func(string, []byte) error,
	readFile recordings.RecordingReadFile,
) recordings.RecordingSnapshotWriter {
	return recordingsinternal.NewReplayRecordingSnapshotWriterWithReader(writeFile, appendFile, readFile)
}

func NewRecordingFlushTickerFactory() recordings.RecordingFlushTickerFactory {
	return recordingsinternal.NewRecordingFlushTickerFactory()
}
