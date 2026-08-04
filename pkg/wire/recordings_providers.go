package wire

import (
	"fmt"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingscli "github.com/portpowered/infinite-you/pkg/services/recordings/transports/cli"
	recordingswire "github.com/portpowered/infinite-you/pkg/services/recordings/wire"
	"go.uber.org/zap"
)

func provideRecordingsCLIAdapter() recordingscli.Adapter {
	return recordingswire.NewCLIAdapter()
}

func provideRecordingsFactory(
	edges serviceedges.Edges,
	targets recordings.LiveRecordingTargetPlanner,
	storage platformreplay.Storage,
) func(recordings.Ledger, recordings.ProjectionService) recordings.Service {
	return func(
		ledger recordings.Ledger,
		projection recordings.ProjectionService,
	) recordings.Service {
		makeDirectories, createTemporaryFile, removePath, renamePath, readFile := provideRecordingFilesystemEffects(edges)
		service, err := recordingswire.NewServiceWithProjectionAndEffects(
			ledger,
			projection,
			targets,
			storage.WriteFile,
			makeDirectories,
			createTemporaryFile,
			removePath,
			renamePath,
			readFile,
			platformclock.Real{},
		)
		if err != nil {
			panic(err)
		}
		return service
	}
}

// provideRecordingReplayArtifactsFactory composes the single phase-aware
// Recordings replay/artifact capability consumed by Factory Sessions. Its
// runtime view first loads a caller-selected portable or legacy replay input,
// then binds the same narrow capability to the runtime's canonical ledger and
// projection once those request-scoped values exist. No caller assembles raw
// file effects, validates a portable document, or discovers a broad Service.
func provideRecordingReplayArtifactsFactory(
	edges serviceedges.Edges,
	targets recordings.LiveRecordingTargetPlanner,
	storage platformreplay.Storage,
	loadReplay recordings.ReplayArtifactLoader,
	replayFiles factorysessionwire.ReplayRecordingReader,
	logger *zap.Logger,
) factorysessionwire.RecordingReplayArtifactsFactory {
	serviceFactory := provideRecordingsFactory(edges, targets, storage)
	runtimeBuilder := func(
		ledger recordings.Ledger,
		projection recordings.ProjectionService,
	) (recordings.RecordingReplayArtifacts, recordings.RecordingLifecycle, error) {
		service := serviceFactory(ledger, projection)
		replayArtifacts, ok := service.(recordings.RecordingReplayArtifacts)
		if !ok {
			return nil, nil, fmt.Errorf("construct Recordings: service does not expose the replay/artifact capability")
		}
		lifecycle, ok := service.(recordings.RecordingLifecycle)
		if !ok {
			return nil, nil, fmt.Errorf("construct Recordings: service does not expose the recording lifecycle capability")
		}
		return replayArtifacts, lifecycle, nil
	}
	return recordingswire.NewRecordingReplayArtifactsFactory(
		recordings.RecordingReadFile(replayFiles),
		loadReplay,
		logger,
		runtimeBuilder,
	)
}
