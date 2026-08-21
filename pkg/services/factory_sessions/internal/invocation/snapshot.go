package invocation

import (
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"sort"
	"strings"
)

// ClassifyMissingPrimaryResultFromSnapshot deterministically identifies the
// blocked or human-gated Work item responsible for a missing invocation result.
func ClassifyMissingPrimaryResultFromSnapshot(sessionID string, snapshot *interfaces.EngineStateSnapshot[factory.PetriMarkingSnapshot, *factory.RuntimeNet], input SessionInvocationWaitInput) *work.PrimaryResultError {
	if snapshot == nil || strings.TrimSpace(input.RequestID) == "" {
		return nil
	}
	materialized := factory.CollectPublicWorkTokens(snapshot.Marking.Tokens, nil)
	tokens := materialized.Tokens
	sort.Slice(tokens, func(i, j int) bool {
		leftID, rightID, leftState, rightState := "", "", "", ""
		if tokens[i] != nil {
			leftID = tokens[i].Color.WorkID
			leftState = tokens[i].State
		}
		if tokens[j] != nil {
			rightID = tokens[j].Color.WorkID
			rightState = tokens[j].State
		}
		if leftID == rightID {
			return leftState < rightState
		}
		return leftID < rightID
	})
	for _, wantState := range []string{"blocked", "needs-human"} {
		for _, token := range tokens {
			if token == nil {
				continue
			}
			if strings.TrimSpace(token.Color.RequestID) != strings.TrimSpace(input.RequestID) || strings.TrimSpace(token.State) != wantState {
				continue
			}
			return work.ClassifyMissingPrimaryResultWorkItem(input.RequestID, input.InvocationReturn, work.FactoryWorkItem{
				ID: token.Color.WorkID, WorkTypeID: token.Color.WorkTypeID,
				DisplayName: token.Color.Name, State: token.State,
			}, sessionID)
		}
	}
	return nil
}
