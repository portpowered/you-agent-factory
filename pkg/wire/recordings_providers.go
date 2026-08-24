package wire

import (
	"fmt"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
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
	targets recordings.LiveRecordingTargetPlanner,
	storage platformreplay.Storage,
	captureSnapshot factorydefinitions.LoadedFactorySnapshotCapturer,
	replayInputs recordings.ReplayInputLoader,
	logger logging.Logger,
) (recordings.Service, error) {
	makeDirectories, createTemporaryFile, removePath, renamePath, readFile := provideRecordingFilesystemEffects(edges)
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
	return recordingswire.NewRuntimeRootWithAppend(
		targets,
		writeFile,
		appendFile,
		makeDirectories,
		createTemporaryFile,
		removePath,
		renamePath,
		readFile,
		captureSnapshot,
		factorydefinitionswire.FactorySnapshotJSONDecoder(),
		factorydefinitionswire.ReplayRuntimeConfigDecoder(),
		replayInputs,
		logger,
		platformclock.Real{},
	)
}

func provideRecordingsRuntimeOpening(
	service recordings.Service,
) (recordings.RuntimeOpening, error) {
	runtime, ok := service.(recordings.RuntimeOpening)
	if !ok || runtime == nil {
		return nil, fmt.Errorf("compose Recordings runtime opening: service does not implement RuntimeOpening")
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
