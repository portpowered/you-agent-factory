package wire

import (
	"fmt"

	processcontract "github.com/portpowered/infinite-you/pkg/initializer/process"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
	factorysessionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
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
	logger logging.Logger,
) recordings.ReplayInputLoader {
	return recordingswire.NewReplayInputLoader(recordings.RecordingReadFile(replayFiles), loadReplay, logger)
}

// provideRecordingsProjectionCapability carries the already-composed
// Recordings projection across the neutral initializer boundary.
func provideRecordingsProjectionCapability(
	service recordings.Service,
) processcontract.RecordingsProjectionCapability {
	var projection recordings.ProjectionService
	if opening, ok := service.(recordings.RuntimeOpening); ok {
		projection = opening.Projection()
	}
	return recordingsProjectionCapability{projection: projection}
}

type recordingsProjectionCapability struct {
	projection recordings.ProjectionService
}

func (capability recordingsProjectionCapability) RecordingsProjection() any {
	return recordingsProjection{projection: capability.projection}
}

type recordingsProjection struct {
	projection recordings.ProjectionService
}

func (projection recordingsProjection) ReconstructFactoryWorldState(
	events []recordings.FactoryEvent,
	selectedTick int,
) (recordings.FactoryWorldState, error) {
	return projection.projection.ReconstructFactoryWorldState(events, selectedTick)
}

func (projection recordingsProjection) ProjectWorkstationRequests(
	world recordings.FactoryWorldState,
) recordings.WorkstationFactoryWorldWorkstationRequestProjectionSlice {
	return projection.projection.ProjectWorkstationRequests(world)
}

// provideOperatorSettingsCapability carries the already-composed Operator
// Settings root across the neutral initializer boundary for public bindings.
func provideOperatorSettingsCapability(
	service operatorsettings.Service,
) processcontract.OperatorSettingsCapability {
	return operatorSettingsCapability{service: service}
}

type operatorSettingsCapability struct {
	service operatorsettings.Service
}

func (capability operatorSettingsCapability) OperatorSettings() any {
	return capability.service
}
