package wire

import (
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingscli "github.com/portpowered/infinite-you/pkg/services/recordings/transports/cli"
	recordingswire "github.com/portpowered/infinite-you/pkg/services/recordings/wire"
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

// provideRecordingLifecycleFactory narrows the Recordings root constructed by
// provideRecordingsFactory down to the RecordingLifecycle capability that the
// Factory Session runtime-opening path binds to the runtime recorder. Wire
// performs this narrowing once, at composition time, so runtime-opening
// receives the narrow capability explicitly instead of discovering it from
// the broad Service at call time.
func provideRecordingLifecycleFactory(
	edges serviceedges.Edges,
	targets recordings.LiveRecordingTargetPlanner,
	storage platformreplay.Storage,
) factorysessionwire.RecordingLifecycleFactory {
	serviceFactory := provideRecordingsFactory(edges, targets, storage)
	return func(
		ledger recordings.Ledger,
		projection recordings.ProjectionService,
	) recordings.RecordingLifecycle {
		service := serviceFactory(ledger, projection)
		lifecycle, ok := service.(recordings.RecordingLifecycle)
		if !ok {
			panic("construct Recordings: service does not expose the recording lifecycle capability")
		}
		return lifecycle
	}
}

// provideRecordingReplayArtifactsFactory narrows the Recordings root
// constructed by provideRecordingsFactory down to the narrow
// RecordingReplayArtifacts capability, so callers that only need to load
// finalized replay facts and read, export, decode, or validate existing
// portable artifacts receive that capability explicitly at Wire composition
// time instead of depending on the broad Service.
func provideRecordingReplayArtifactsFactory(
	edges serviceedges.Edges,
	targets recordings.LiveRecordingTargetPlanner,
	storage platformreplay.Storage,
) func(recordings.Ledger, recordings.ProjectionService) recordings.RecordingReplayArtifacts {
	serviceFactory := provideRecordingsFactory(edges, targets, storage)
	return func(
		ledger recordings.Ledger,
		projection recordings.ProjectionService,
	) recordings.RecordingReplayArtifacts {
		service := serviceFactory(ledger, projection)
		replayArtifacts, ok := service.(recordings.RecordingReplayArtifacts)
		if !ok {
			panic("construct Recordings: service does not expose the replay/artifact capability")
		}
		return replayArtifacts
	}
}
