package service

import (
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/recordings/replay"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

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
	workers.Provider,
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
