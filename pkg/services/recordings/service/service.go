package service

import (
	"fmt"
	"time"

	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/recordings/replay"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerprovider "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
)

// NewRuntimeRecorder constructs the replay artifact and streaming recorder for
// one runtime.
func NewRuntimeRecorder(
	storage platformreplay.Storage,
	flushInterval time.Duration,
	loaded factorydefinitions.LoadedFactorySource,
	now func() time.Time,
	recordPath string,
	captureLoadedFactorySnapshot factorydefinitions.LoadedFactorySnapshotCapturer,
) (recordings.RuntimeRecorder, error) {
	if recordPath == "" {
		return nil, nil
	}
	if now == nil {
		return nil, fmt.Errorf("recording clock is required")
	}
	if captureLoadedFactorySnapshot == nil {
		return nil, fmt.Errorf("loaded Factory snapshot capturer is required")
	}
	recordedAt := now().UTC()
	sourceDirectory := ""
	if loaded != nil {
		sourceDirectory = loaded.FactoryDir()
	}
	snapshot, err := captureLoadedFactorySnapshot(
		loaded,
		sourceDirectory,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("build replay artifact config: %w", err)
	}
	artifact, err := replay.NewEventLogArtifact(
		recordedAt,
		snapshot,
		&factorydefinitions.ReplayWallClockMetadata{StartedAt: recordedAt},
		factorydefinitions.ReplayDiagnostics{},
	)
	if err != nil {
		return nil, err
	}
	recorder, err := replay.NewRecorder(storage, recordPath, artifact, flushInterval)
	if err != nil {
		return nil, fmt.Errorf("create replay recorder: %w", err)
	}
	return recorder, nil
}

func NewReplayClock(artifact *factorydefinitions.ReplayArtifact) recordings.Clock {
	if artifact == nil {
		return nil
	}
	return replay.NewArtifactClock(artifact)
}

func NewReplayExecution(
	artifact *factorydefinitions.ReplayArtifact,
	decodeFactorySnapshot factorydefinitions.FactorySnapshotJSONDecoder,
	decodeRuntimeConfig factorydefinitions.ReplayRuntimeConfigDecoder,
) (
	workerprovider.Provider,
	workers.CommandRunner,
	[]recordings.ReplayHook,
	recordings.CompletionDeliveryPlanner,
	error,
) {
	if artifact == nil {
		return nil, nil, nil, nil, nil
	}
	sideEffects, err := replay.NewSideEffects(
		decodeFactorySnapshot,
		decodeRuntimeConfig,
		artifact,
	)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("build replay side effects: %w", err)
	}
	submissionHook, err := replay.NewSubmissionHook(
		decodeFactorySnapshot,
		decodeRuntimeConfig,
		artifact,
	)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("build replay submission hook: %w", err)
	}
	workStateChangeHook, err := replay.NewWorkStateChangeHook(
		decodeFactorySnapshot,
		decodeRuntimeConfig,
		artifact,
	)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("build replay work state change hook: %w", err)
	}
	deliveryPlan, err := replay.NewCompletionDeliveryPlan(
		decodeFactorySnapshot,
		decodeRuntimeConfig,
		artifact,
	)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("build replay completion delivery plan: %w", err)
	}
	return sideEffects,
		sideEffects,
		[]recordings.ReplayHook{submissionHook, workStateChangeHook},
		deliveryPlan,
		nil
}
