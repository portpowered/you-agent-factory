package wire

import (
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	factorysessionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingswire "github.com/portpowered/infinite-you/pkg/services/recordings/wire"
	recordingscli "github.com/portpowered/infinite-you/pkg/services/recordings/transports/cli"
)

func provideRecordingsCLIAdapter() recordingscli.Adapter {
	return recordingswire.NewCLIAdapter()
}

func provideRecordingsFactory(
	targets recordings.LiveRecordingTargetPlanner,
	storage platformreplay.Storage,
) factorysessionwire.RecordingsFactory {
	return func(
		ledger recordings.Ledger,
		projection recordings.ProjectionService,
	) recordings.Service {
		service, err := recordingswire.NewServiceWithProjection(
			ledger,
			projection,
			targets,
			storage.WriteFile,
			platformclock.Real{},
		)
		if err != nil {
			panic(err)
		}
		return service
	}
}
