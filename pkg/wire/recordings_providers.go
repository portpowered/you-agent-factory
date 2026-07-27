package wire

import (
	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	factorysessionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingsservice "github.com/portpowered/infinite-you/pkg/services/recordings/service"
)

func provideRecordingsFactory(
	targets recordings.LiveRecordingTargetPlanner,
	storage platformreplay.Storage,
) factorysessionwire.RecordingsFactory {
	writer := recordingsservice.NewRecordingSnapshotWriter(storage.WriteFile)
	tickers := recordingsservice.NewRecordingFlushTickerFactory()
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
		)
	}
}
