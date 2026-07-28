// Package service is a transitional compile shim that re-exports the composed
// Recordings root from pkg/services/recordings/internal. Peers should
// construct through recordings/wire; baseline deletion of this path is owned
// by DEL-REC.
package service

import (
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingsinternal "github.com/portpowered/infinite-you/pkg/services/recordings/internal"
	artifactsexport "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/artifacts_export"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func NewReplayClock(artifact *recordings.ReplayArtifact) recordings.Clock {
	return recordingsinternal.NewReplayClock(artifact)
}

func NewReplayExecution(
	artifact *recordings.ReplayArtifact,
	decodeFactorySnapshot factorydefinitions.FactorySnapshotJSONDecoder,
	decodeRuntimeConfig factorydefinitions.ReplayRuntimeConfigDecoder,
) (
	workers.Provider,
	workers.CommandRunner,
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

func NewProjectionService() recordings.ProjectionService {
	return recordingsinternal.NewProjectionService()
}

func NewService(
	ledger recordings.Ledger,
	projection recordings.ProjectionService,
	targets ...recordings.LiveRecordingTargetPlanner,
) recordings.Service {
	return recordingsinternal.NewService(ledger, projection, targets...)
}

func NewServiceWithLifecycleEffects(
	ledger recordings.Ledger,
	projection recordings.ProjectionService,
	targetPlanner recordings.LiveRecordingTargetPlanner,
	writer recordings.RecordingSnapshotWriter,
	tickers recordings.RecordingFlushTickerFactory,
	publication artifactsexport.PortableArtifactPublication,
	clocks ...recordings.RecordingClock,
) recordings.Service {
	return recordingsinternal.NewServiceWithLifecycleEffects(
		ledger,
		projection,
		targetPlanner,
		writer,
		tickers,
		publication,
		clocks...,
	)
}

func NewRuntimeLedger(
	topology recordings.InitialStructureSource,
	now func() time.Time,
	streamGenerationID string,
	definitions factorydefinitions.RuntimeDefinitionLookup,
) recordings.RuntimeEventLedger {
	return recordingsinternal.NewRuntimeLedger(topology, now, streamGenerationID, definitions)
}

func NewRecordingSnapshotWriter(
	write func(string, []byte) error,
) recordings.RecordingSnapshotWriter {
	return recordingsinternal.NewRecordingSnapshotWriter(write)
}

func NewReplayRecordingSnapshotWriter(
	write func(string, []byte) error,
) recordings.RecordingSnapshotWriter {
	return recordingsinternal.NewReplayRecordingSnapshotWriter(write)
}

func NewRecordingFlushTickerFactory() recordings.RecordingFlushTickerFactory {
	return recordingsinternal.NewRecordingFlushTickerFactory()
}

func NewPortableArtifactPublication() (artifactsexport.PortableArtifactPublication, error) {
	return recordingsinternal.NewPortableArtifactPublication()
}

func NewLifecycleRuntimeRecorder(
	flushInterval time.Duration,
	loaded factorydefinitions.LoadedFactorySource,
	now func() time.Time,
	recordPath string,
	captureLoadedFactorySnapshot factorydefinitions.LoadedFactorySnapshotCapturer,
) (recordings.RuntimeRecorder, error) {
	return recordingsinternal.NewLifecycleRuntimeRecorder(
		flushInterval,
		loaded,
		now,
		recordPath,
		captureLoadedFactorySnapshot,
	)
}
