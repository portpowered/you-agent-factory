package wire

import (
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
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
) (recordings.Root, error) {
	makeDirectories, createTemporaryFile, removePath, renamePath, readFile := provideRecordingFilesystemEffects(edges)
	return recordingswire.NewRuntimeRoot(
		targets,
		storage.WriteFile,
		makeDirectories,
		createTemporaryFile,
		removePath,
		renamePath,
		readFile,
		captureSnapshot,
		factorydefinitionswire.FactorySnapshotJSONDecoder(),
		factorydefinitionswire.ReplayRuntimeConfigDecoder(),
		replayInputs,
		platformclock.Real{},
	)
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
	logger logging.Logger,
) recordings.ReplayInputLoader {
	return recordingswire.NewReplayInputLoader(recordings.RecordingReadFile(replayFiles), loadReplay, logger)
}
