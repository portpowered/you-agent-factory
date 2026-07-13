package materialize

import (
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
)

// PublicWorkTokens is the deduplicated public work token set from marking and
// active dispatch consumed inputs.
type PublicWorkTokens struct {
	Tokens           []*interfaces.Token
	InFlightOnlyByID map[string]struct{}
}

// CollectPublicWorkTokens returns public work tokens from marking plus active
// dispatch ConsumedTokens. Tokens with the same non-empty Color.WorkID dedupe
// with marking precedence. InFlightOnlyByID lists token IDs present only on an
// active dispatch and not in the marking snapshot.
func CollectPublicWorkTokens(
	marking *petri.MarkingSnapshot,
	dispatches map[string]*interfaces.DispatchEntry,
) PublicWorkTokens {
	result := PublicWorkTokens{
		Tokens:           make([]*interfaces.Token, 0),
		InFlightOnlyByID: make(map[string]struct{}),
	}

	markingTokens := markingTokensMap(marking)
	seenWorkIDs := make(map[string]struct{})

	for _, token := range markingTokens {
		if !IsPublicWorkToken(token) {
			continue
		}
		result.Tokens = append(result.Tokens, token)
		if workID := token.Color.WorkID; workID != "" {
			seenWorkIDs[workID] = struct{}{}
		}
	}

	seenDispatchTokenIDs := make(map[string]struct{})
	for _, entry := range dispatches {
		if entry == nil {
			continue
		}
		for i := range entry.ConsumedTokens {
			token := entry.ConsumedTokens[i]
			if !IsPublicWorkToken(&token) {
				continue
			}
			if token.ID != "" {
				if _, seen := seenDispatchTokenIDs[token.ID]; seen {
					continue
				}
				seenDispatchTokenIDs[token.ID] = struct{}{}
			}
			if workID := token.Color.WorkID; workID != "" {
				if _, seen := seenWorkIDs[workID]; seen {
					continue
				}
				seenWorkIDs[workID] = struct{}{}
			}
			tokenCopy := token
			result.Tokens = append(result.Tokens, &tokenCopy)
			if tokenCopy.ID != "" {
				result.InFlightOnlyByID[tokenCopy.ID] = struct{}{}
			}
		}
	}

	return result
}

// IsPublicWorkToken reports whether a token should appear in work list/show
// materialization. Resource and system-time tokens are excluded.
func IsPublicWorkToken(token *interfaces.Token) bool {
	return token != nil &&
		token.Color.DataType != interfaces.DataTypeResource &&
		!interfaces.IsSystemTimeToken(token)
}

func markingTokensMap(marking *petri.MarkingSnapshot) map[string]*interfaces.Token {
	if marking == nil || marking.Tokens == nil {
		return nil
	}
	return marking.Tokens
}
