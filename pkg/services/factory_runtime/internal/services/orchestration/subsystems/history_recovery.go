package subsystems

import (
	"sort"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workers "github.com/portpowered/infinite-you/pkg/services/workers"
)

type historyRecoveryFact struct {
	at         time.Time
	tick       int
	completion *interfaces.FactoryWorldDispatchCompletion
	move       *interfaces.FactoryWorldWorkStateChangeRecord
	start      bool
}

// RecoverWorkHistories rebuilds executable guard counters from canonical
// dispatches, capturing inputs at dispatch start just as the live engine does.
// It neither changes the ledger nor counts interrupted attempts as visits.
func RecoverWorkHistories(world *interfaces.FactoryWorldState, net *state.Net) map[string]factorytoken.History {
	histories := make(map[string]factorytoken.History)
	if world == nil {
		return histories
	}
	var facts []historyRecoveryFact
	for i := range world.CompletedDispatches {
		c := &world.CompletedDispatches[i]
		facts = append(facts, historyRecoveryFact{at: c.StartedAt, tick: c.StartedTick, completion: c, start: true}, historyRecoveryFact{at: c.CompletedAt, tick: c.CompletedTick, completion: c})
	}
	for _, moves := range world.WorkStateChangesByWorkID {
		for i := range moves {
			m := &moves[i]
			facts = append(facts, historyRecoveryFact{at: m.EventTime, tick: m.Tick, move: m})
		}
	}
	completeTimes := true
	for _, fact := range facts {
		completeTimes = completeTimes && !fact.at.IsZero()
	}
	sort.SliceStable(facts, func(i, j int) bool {
		if completeTimes && !facts[i].at.Equal(facts[j].at) {
			return facts[i].at.Before(facts[j].at)
		}
		return facts[i].tick < facts[j].tick
	})
	inputs := make(map[string][]factorytoken.Token)
	for _, fact := range facts {
		if fact.move != nil {
			m := fact.move
			if (m.Source == work.WorkStateChangeSourceAPI || m.Source == work.WorkStateChangeSourceCLI) && state.CategoryForState(net.WorkTypes, m.WorkTypeName, m.FromState) == state.StateCategoryFailed {
				h := histories[m.WorkID]
				factorytoken.ClearGuardBlockingFields(&h)
				histories[m.WorkID] = h
			}
			continue
		}
		c := fact.completion
		if fact.start {
			inputs[c.DispatchID] = recoveryConsumedTokens(c, histories)
			continue
		}
		consumed := inputs[c.DispatchID]
		delete(inputs, c.DispatchID)
		recoverCompletionHistory(c, consumed, histories, net)
	}
	return histories
}

func recoveryConsumedTokens(c *interfaces.FactoryWorldDispatchCompletion, histories map[string]factorytoken.History) []factorytoken.Token {
	items := make(map[string]work.FactoryWorkItem)
	for _, item := range c.InputWorkItems {
		items[item.ID] = item
	}
	var tokens []factorytoken.Token
	for _, input := range c.ConsumedInputs {
		id := input.TokenID
		if input.WorkItem != nil {
			id = input.WorkItem.ID
			items[id] = *input.WorkItem
		}
		if id == "" || input.Resource != nil {
			continue
		}
		item := items[id]
		tokens = append(tokens, factorytoken.Token{PlaceID: input.PlaceID, Color: factorytoken.Color{WorkID: id, ParentID: item.ParentID, DataType: factorytoken.DataTypeWork}, History: factorytoken.CloneHistory(histories[id])})
	}
	if len(tokens) == 0 {
		for _, id := range c.WorkItemIDs {
			item := items[id]
			tokens = append(tokens, factorytoken.Token{PlaceID: state.PlaceID(item.WorkTypeID, item.State), Color: factorytoken.Color{WorkID: id, ParentID: item.ParentID, DataType: factorytoken.DataTypeWork}, History: factorytoken.CloneHistory(histories[id])})
		}
	}
	return tokens
}

func recoverCompletionHistory(c *interfaces.FactoryWorldDispatchCompletion, consumed []factorytoken.Token, histories map[string]factorytoken.History, net *state.Net) {
	result := workers.WorkResult{TransitionID: c.TransitionID, Outcome: workers.WorkOutcome(c.Result.Outcome), FailureMetadata: c.Result.FailureMetadata}
	history := buildHistory(consumed, &result, candidateWorkID(net, c.TransitionID, consumed))
	for _, output := range c.OutputWorkItems {
		h := factorytoken.CloneHistory(history)
		if result.Outcome == workers.OutcomeCanceled {
			h = histories[output.ID]
		}
		message := c.Result.Error
		failed := result.Outcome == workers.OutcomeFailed
		if result.Outcome == workers.OutcomeRejected && state.CategoryForState(net.WorkTypes, output.WorkTypeID, output.State) == state.StateCategoryFailed {
			failed = true
			message = c.Result.Feedback
		}
		if failed {
			h.LastError = message
			h.FailureLog = append(h.FailureLog, factorytoken.Failure{TransitionID: c.TransitionID, Timestamp: c.CompletedAt, Error: message})
		}
		histories[output.ID] = h
	}
}
