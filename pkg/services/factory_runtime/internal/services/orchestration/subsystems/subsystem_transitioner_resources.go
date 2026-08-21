package subsystems

import (
	"fmt"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
)

// hasFanoutGroup checks if a transition has a fanout group configured.
func (t *TransitionerSubsystem) hasFanoutGroup(transitionID string) bool {
	if t.netDefinition.FanoutGroups == nil {
		return false
	}
	_, ok := t.netDefinition.FanoutGroups[transitionID]
	return ok
}

// releaseResourceTokens returns consumed resource tokens back to their original resource places.
func (t *TransitionerSubsystem) releaseResourceTokens(consumedTokens []factorytoken.Token, alreadyCovered map[string]int, transitionID string, now time.Time) []interfaces.MarkingMutation {
	var mutations []interfaces.MarkingMutation
	for i := range consumedTokens {
		consumed := consumedTokens[i]
		if consumed.Color.DataType != factorytoken.DataTypeResource {
			continue
		}
		if alreadyCovered[consumed.PlaceID] > 0 {
			alreadyCovered[consumed.PlaceID]--
			continue
		}
		resourceToken := t.transformer.ReleasedResourceToken(consumed, consumed.PlaceID, now)
		mutations = append(mutations, interfaces.MarkingMutation{
			Type:     interfaces.MutationCreate,
			ToPlace:  consumed.PlaceID,
			NewToken: resourceToken,
			Reason:   fmt.Sprintf("release resource %s for transition %s", consumed.PlaceID, transitionID),
		})
	}
	return mutations
}

func tokenColorsFromTokens(tokens []factorytoken.Token) []factorytoken.Color {
	colors := make([]factorytoken.Color, len(tokens))
	for i, token := range tokens {
		colors[i] = token.Color
	}
	return colors
}
