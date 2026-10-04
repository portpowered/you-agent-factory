package wire

import (
	"fmt"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingscli "github.com/portpowered/infinite-you/pkg/services/recordings/transports/cli"
	recordingswire "github.com/portpowered/infinite-you/pkg/services/recordings/wire"
)

func provideRecordingsCLIAdapter() recordingscli.Adapter {
	return recordingswire.NewCLIAdapter()
}

func provideRecordingsRoot(
	edges serviceedges.Edges,
	ledger recordings.Ledger,
	projection recordings.ProjectionService,
	lifecycle recordingswire.RecordingLifecycleOwner,
	artifacts recordingswire.ArtifactsExportOwner,
	replay recordingswire.ReplayOwner,
	canonical recordingswire.CanonicalLedgerOwner,
	historical recordingswire.HistoricalQueryOwner,
	clock recordings.RecordingClock,
	logger logging.Logger,
	router *recordingswire.RuntimeLedgerRouter,
	captureSnapshot factorydefinitions.LoadedFactorySnapshotCapturer,
	decodeSnapshot factorydefinitions.FactorySnapshotJSONDecoder,
	decodeRuntimeConfig factorydefinitions.ReplayRuntimeConfigDecoder,
	replayInputs recordings.ReplayInputLoader,
) recordings.Service {
	service := recordingswire.NewService(ledger, projection, lifecycle, artifacts, replay, canonical, historical, clock, logger, router, captureSnapshot, decodeSnapshot, decodeRuntimeConfig, replayInputs)
	if edges.RecordingsRootObserver != nil {
		edges.RecordingsRootObserver(service)
	}
	if edges.RecordingsWorkSnapshotReaderObserver != nil {
		edges.RecordingsWorkSnapshotReaderObserver(recordingswire.NewWorkSnapshotReader(service))
	}
	return service
}

func provideRecordingClock(clock factoryruntime.Clock) (recordings.RecordingClock, error) {
	if isNilModelEdgeDependency(clock) {
		return nil, fmt.Errorf("construct Recordings: clock is required")
	}
	return clock, nil
}

func provideRecordingSnapshotWriter(
	edges serviceedges.Edges,
	storage platformreplay.Storage,
	readFile recordings.RecordingReadFile,
) recordings.RecordingSnapshotWriter {
	writeFile := storage.WriteFile
	if edges.RecordingWriteFile != nil {
		writeFile = edges.RecordingWriteFile
	}
	var appendFile func(string, []byte) error
	if edges.RecordingAppendFile != nil {
		appendFile = edges.RecordingAppendFile
	} else if appender, ok := storage.(platformreplay.Appender); ok {
		appendFile = appender.AppendFile
	}
	return recordingswire.NewReplayRecordingSnapshotWriter(writeFile, appendFile, readFile)
}

func provideRecordingPublication(edges serviceedges.Edges) (recordingswire.PortableArtifactPublication, error) {
	makeDirectories, createTemporaryFile, removePath, renamePath, readFile := provideRecordingFilesystemEffects(edges)
	return recordingswire.NewPortableArtifactPublication(makeDirectories, createTemporaryFile, removePath, renamePath, readFile)
}

func provideRecordingReadFile(edges serviceedges.Edges) recordings.RecordingReadFile {
	_, _, _, _, readFile := provideRecordingFilesystemEffects(edges)
	return readFile
}

func provideRecordingsRuntimeScopeService(
	service recordings.Service,
) (recordings.RuntimeScopeService, error) {
	runtime, ok := service.(recordings.RuntimeScopeService)
	if !ok || runtime == nil {
		return nil, fmt.Errorf("compose Recordings runtime scope: service does not implement RuntimeScopeService")
	}
	return runtime, nil
}

// provideFactorySessionReplayInputs composes the Recordings-owned, path-based
// ReplayInputLoader capability from the existing legacy replay
// artifact loader and replay recording file reader, so the Factory Sessions
// runtime-opening replay-input lane receives one already-constructed
// capability instead of combining those two raw effects itself. This operation
// is intentionally distinct from the ledger-backed RecordingReplayArtifacts
// capability: it is composed before a Factory Session ledger exists and its
// complete contract is the single LoadReplayInput operation.
func provideFactorySessionReplayInputs(
	loadReplay recordings.ReplayArtifactLoader,
	replayFiles factorysessionwire.ReplayRecordingReader,
	openFile recordings.RecordingOpenFile,
	logger logging.Logger,
) recordings.ReplayInputLoader {
	return recordingswire.NewReplayInputLoader(
		recordings.RecordingReadFile(replayFiles), loadReplay, logger, openFile,
	)
}

func provideRecordingOpenFile(edges serviceedges.Edges) recordings.RecordingOpenFile {
	if edges.RecordingOpenFile != nil {
		return edges.RecordingOpenFile
	}
	return platformfilesystem.Local{}.Open
}

func provideRecordedSessionInventory(
	edges serviceedges.Edges,
	replayInputs recordings.ReplayInputLoader,
	logger logging.Logger,
) recordings.RecordedSessionInventory {
	readDir := edges.RecordingReadDirectory
	if readDir == nil {
		readDir = platformfilesystem.Local{}.ReadDir
	}
	return recordingswire.NewRecordedSessionInventory(readDir, replayInputs, logger)
}
