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
	tokens := make([]*factory.RuntimeToken, 0, len(snapshot.Marking.Tokens))
	for _, token := range snapshot.Marking.Tokens {
		tokens = append(tokens, token)
	}
	sort.Slice(tokens, func(i, j int) bool {
		leftID, rightID := "", ""
		if tokens[i] != nil {
			leftID = tokens[i].Color.WorkID
		}
		if tokens[j] != nil {
			rightID = tokens[j].Color.WorkID
		}
		if leftID == rightID {
			return tokenPlaceID(tokens[i]) < tokenPlaceID(tokens[j])
		}
		return leftID < rightID
	})
	for _, wantState := range []string{"blocked", "needs-human"} {
		for _, token := range tokens {
			if token == nil || token.Color.DataType == factory.RuntimeTokenDataTypeResource {
				continue
			}
			if strings.TrimSpace(token.Color.RequestID) != strings.TrimSpace(input.RequestID) || tokenStateName(token.PlaceID) != wantState {
				continue
			}
			return work.ClassifyMissingPrimaryResultWorkItem(input.RequestID, input.InvocationReturn, work.FactoryWorkItem{
				ID: token.Color.WorkID, WorkTypeID: token.Color.WorkTypeID,
				DisplayName: token.Color.Name, State: tokenStateName(token.PlaceID),
			}, sessionID)
		}
	}
	return nil
}

func tokenStateName(placeID string) string {
	trimmed := strings.TrimSpace(placeID)
	if index := strings.LastIndexByte(trimmed, ':'); index >= 0 {
		return trimmed[index+1:]
	}
	return trimmed
}

func tokenPlaceID(token *factory.RuntimeToken) string {
	if token == nil {
		return ""
	}
	return token.PlaceID
}
