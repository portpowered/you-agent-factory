package wire

import (
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	factorysessionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingsservice "github.com/portpowered/infinite-you/pkg/services/recordings/service"
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
	writer := recordingsservice.NewReplayRecordingSnapshotWriter(storage.WriteFile)
	tickers := recordingsservice.NewRecordingFlushTickerFactory()
	publication, err := recordingsservice.NewPortableArtifactPublication()
	if err != nil {
		panic(err)
	}
	return func(
		ledger recordings.Ledger,
		projection recordings.ProjectionService,
	) recordings.Service {
		return recordingsservice.NewServiceWithLifecycleEffects(
			ledger,
			projection,
			targets,
			writer,
			tickers,
			publication,
			platformclock.Real{},
		)
	}
}
