package factory

import (
	"sort"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// PublicWorkTokens is the deduplicated public work token set from marking and
// active dispatch consumed inputs.
type PublicWorkTokens struct {
	Tokens []*workers.Token
	// MarkingTokens retains detached facts for resources and system tokens too;
	// peer projections filter those product-specific categories after crossing
	// the Runtime boundary.
	MarkingTokens    []workers.Token
	InFlightOnlyByID map[string]struct{}
}

// CollectPublicWorkTokens returns public work tokens from marking plus active
// dispatch ConsumedTokens. Tokens with the same non-empty Color.WorkID dedupe
// with marking precedence. InFlightOnlyByID lists token IDs present only on an
// active dispatch and not in the marking snapshot.
func CollectPublicWorkTokens(
	markingTokens map[string]*RuntimeToken,
	dispatches map[string]*interfaces.DispatchEntry,
) PublicWorkTokens {
	result := PublicWorkTokens{
		Tokens:           make([]*workers.Token, 0),
		MarkingTokens:    make([]workers.Token, 0, len(markingTokens)),
		InFlightOnlyByID: make(map[string]struct{}),
	}

	seenWorkIDs := make(map[string]struct{})

	markingIDs := make([]string, 0, len(markingTokens))
	for id := range markingTokens {
		markingIDs = append(markingIDs, id)
	}
	sort.Strings(markingIDs)
	for _, id := range markingIDs {
		runtimeToken := markingTokens[id]
		if runtimeToken == nil {
			continue
		}
		token := factorytoken.ToWorker(*runtimeToken)
		result.MarkingTokens = append(result.MarkingTokens, token)
		if !IsPublicWorkToken(runtimeToken) {
			continue
		}
		result.Tokens = append(result.Tokens, &token)
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
			runtimeToken := factorytoken.FromWorker(entry.ConsumedTokens[i])
			if !IsPublicWorkToken(&runtimeToken) {
				continue
			}
			token := factorytoken.ToWorker(runtimeToken)
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
func IsPublicWorkToken(token *RuntimeToken) bool {
	return token != nil &&
		token.Color.DataType != RuntimeTokenDataTypeResource &&
		token.Color.WorkTypeID != interfaces.SystemTimeWorkTypeID
}
