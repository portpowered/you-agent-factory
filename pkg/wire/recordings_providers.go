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
) factorysessionwire.RecordingsFactory {
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
