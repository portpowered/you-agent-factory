package internal

import (
	"fmt"
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"

	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	replayimpl "github.com/portpowered/infinite-you/pkg/services/recordings/internal/replay"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func NewReplayClock(artifact *recordings.ReplayArtifact) recordings.Clock {
	if artifact == nil {
		return nil
	}
	return replayimpl.NewArtifactClock(artifact)
}

func NewReplayExecution(
	artifact *recordings.ReplayArtifact,
	decodeFactorySnapshot factorydefinitionswire.FactorySnapshotJSONDecoder,
	decodeRuntimeConfig factorydefinitionswire.ReplayRuntimeConfigDecoder,
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
	sideEffects, err := replayimpl.NewSideEffects(
		decodeFactorySnapshot,
		decodeRuntimeConfig,
		artifact,
	)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("build replay side effects: %w", err)
	}
	submissionHook, err := replayimpl.NewSubmissionHook(
		decodeFactorySnapshot,
		decodeRuntimeConfig,
		artifact,
	)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("build replay submission hook: %w", err)
	}
	workStateChangeHook, err := replayimpl.NewWorkStateChangeHook(
		decodeFactorySnapshot,
		decodeRuntimeConfig,
		artifact,
	)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("build replay work state change hook: %w", err)
	}
	deliveryPlan, err := replayimpl.NewCompletionDeliveryPlan(
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
