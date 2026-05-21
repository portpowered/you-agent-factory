package scheduler

import (
	"time"

	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/testutil/runtimefixtures"
)

var baseTokenTime = time.Date(2026, 4, 17, 9, 0, 0, 0, time.UTC)

func newPriorityAwareScheduler(maxDispatches int) *WorkInQueueScheduler {
	return NewWorkInQueueScheduler(maxDispatches, WithRuntimeConfig(schedulerWorkstationPriorityRuntimeConfig()))
}

func firingDecisionIDs(decisions []interfaces.FiringDecision) []string {
	ids := make([]string, len(decisions))
	for i := range decisions {
		ids[i] = decisions[i].TransitionID
	}
	return ids
}

func schedulerStatePriorityNet() *state.Net {
	return &state.Net{
		Places: map[string]*petri.Place{
			"task:init":   {ID: "task:init", TypeID: "task", State: "init"},
			"task:review": {ID: "task:review", TypeID: "task", State: "review"},
		},
		WorkTypes: map[string]*state.WorkType{
			"task": {
				ID: "task",
				States: []state.StateDefinition{
					{Value: "init", Category: state.StateCategoryInitial},
					{Value: "review", Category: state.StateCategoryProcessing},
				},
			},
		},
	}
}

func schedulerWorkstationPriorityNet() *state.Net {
	net := schedulerStatePriorityNet()
	net.Places[interfaces.SystemTimePendingPlaceID] = &petri.Place{ID: interfaces.SystemTimePendingPlaceID, TypeID: interfaces.SystemTimeWorkTypeID, State: interfaces.SystemTimePendingState}
	net.WorkTypes[interfaces.SystemTimeWorkTypeID] = &state.WorkType{
		ID: interfaces.SystemTimeWorkTypeID,
		States: []state.StateDefinition{
			{Value: interfaces.SystemTimePendingState, Category: state.StateCategoryProcessing},
		},
	}
	net.Transitions = map[string]*petri.Transition{
		"tr-a-cron":                             {ID: "tr-a-cron", Name: "tr-a-cron", WorkerType: "agent"},
		"tr-a-cron-processing":                  {ID: "tr-a-cron-processing", Name: "tr-a-cron-processing", WorkerType: "agent"},
		"tr-b-repeater":                         {ID: "tr-b-repeater", Name: "tr-b-repeater", WorkerType: "agent"},
		"tr-b-cron-initial":                     {ID: "tr-b-cron-initial", Name: "tr-b-cron-initial", WorkerType: "agent"},
		"tr-b-standard":                         {ID: "tr-b-standard", Name: "tr-b-standard", WorkerType: "agent"},
		"tr-c-repeater":                         {ID: "tr-c-repeater", Name: "tr-c-repeater", WorkerType: "agent"},
		"tr-c-repeater-initial":                 {ID: "tr-c-repeater-initial", Name: "tr-c-repeater-initial", WorkerType: "agent"},
		"tr-z-standard":                         {ID: "tr-z-standard", Name: "tr-z-standard", WorkerType: "agent"},
		"tr-a-logical":                          {ID: "tr-a-logical"},
		"tr-z-standard-processing":              {ID: "tr-z-standard-processing", Name: "tr-z-standard-processing", WorkerType: "agent"},
		interfaces.SystemTimeExpiryTransitionID: {ID: interfaces.SystemTimeExpiryTransitionID, Type: petri.TransitionExhaustion},
	}
	return net
}

func schedulerWorkstationPriorityRuntimeConfig() runtimefixtures.RuntimeWorkstationLookupFixture {
	return runtimefixtures.RuntimeWorkstationLookupFixture{
		Workstations: map[string]*interfaces.FactoryWorkstationConfig{
			"tr-a-cron":                {Name: "tr-a-cron", Kind: interfaces.WorkstationKindCron},
			"tr-a-cron-processing":     {Name: "tr-a-cron-processing", Kind: interfaces.WorkstationKindCron},
			"tr-b-cron-initial":        {Name: "tr-b-cron-initial", Kind: interfaces.WorkstationKindCron},
			"tr-b-repeater":            {Name: "tr-b-repeater", Kind: interfaces.WorkstationKindRepeater},
			"tr-b-standard":            {Name: "tr-b-standard", Kind: interfaces.WorkstationKindStandard},
			"tr-c-repeater":            {Name: "tr-c-repeater", Kind: interfaces.WorkstationKindRepeater},
			"tr-c-repeater-initial":    {Name: "tr-c-repeater-initial", Kind: interfaces.WorkstationKindRepeater},
			"tr-z-standard":            {Name: "tr-z-standard", Kind: interfaces.WorkstationKindStandard},
			"tr-z-standard-processing": {Name: "tr-z-standard-processing", Kind: interfaces.WorkstationKindStandard},
		},
	}
}

func priorityEnabledTransition(transitionID, placeID, tokenID string, enteredAt time.Time) interfaces.EnabledTransition {
	return interfaces.EnabledTransition{
		TransitionID: transitionID,
		WorkerType:   "agent",
		Bindings: map[string][]interfaces.Token{
			"input": {{
				ID:        tokenID,
				PlaceID:   placeID,
				EnteredAt: enteredAt,
				Color:     interfaces.TokenColor{WorkID: "work-" + tokenID, TraceID: "trace-" + tokenID, WorkTypeID: "task"},
			}},
		},
	}
}
