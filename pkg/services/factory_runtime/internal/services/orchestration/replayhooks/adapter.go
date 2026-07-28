package replayhooks

import (
	"context"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// Adapt translates Recordings-owned replay actions at the Factory Runtime
// boundary. Recordings never observes the concrete Petri snapshot.
func Adapt(hooks []recordings.ReplayHook) []factoryruntime.SubmissionHook {
	if len(hooks) == 0 {
		return nil
	}
	adapted := make([]factoryruntime.SubmissionHook, 0, len(hooks))
	for _, hook := range hooks {
		if hook != nil {
			adapted = append(adapted, AdaptOne(hook))
		}
	}
	return adapted
}

// AdaptOne adapts one Recordings replay hook to the runtime hook contract.
func AdaptOne(hook recordings.ReplayHook) factoryruntime.SubmissionHook {
	if hook == nil {
		return nil
	}
	return replayHookAdapter{hook: hook}
}

type replayHookAdapter struct {
	hook recordings.ReplayHook
}

func (a replayHookAdapter) Name() string {
	return a.hook.Name()
}

func (a replayHookAdapter) Priority() int {
	return a.hook.Priority()
}

func (a replayHookAdapter) OnTick(
	ctx context.Context,
	input interfaces.SubmissionHookContext[interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]],
) (interfaces.SubmissionHookResult, error) {
	result, err := a.hook.OnTick(ctx, recordings.ReplaySnapshot{
		Tick:          input.Snapshot.TickCount,
		TokenByWorkID: replayTokensByWorkID(input.Snapshot.Marking.Tokens),
	})
	if err != nil {
		return interfaces.SubmissionHookResult{}, err
	}
	return interfaces.SubmissionHookResult{
		GeneratedBatches: result.GeneratedBatches,
		MarkingMutations: result.MarkingMutations,
		KeepAlive:        result.KeepAlive,
	}, nil
}

func replayTokensByWorkID(tokens map[string]*workers.Token) map[string]recordings.ReplayWorkToken {
	if len(tokens) == 0 {
		return nil
	}
	byWorkID := make(map[string]recordings.ReplayWorkToken)
	for tokenID, token := range tokens {
		if token == nil || token.Color.WorkID == "" || token.Color.DataType == workers.DataTypeResource {
			continue
		}
		byWorkID[token.Color.WorkID] = recordings.ReplayWorkToken{
			TokenID: tokenID,
			PlaceID: token.PlaceID,
		}
	}
	return byWorkID
}
