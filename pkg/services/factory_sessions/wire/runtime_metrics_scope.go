package wire

import (
	"context"
	"fmt"
	"strings"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

// NewRuntimeMetricsScopeResolver constructs the Factory Sessions-owned
// retained-scope operation used by the base metrics and Costs adapters.
func NewRuntimeMetricsScopeResolver(
	reader factorysessions.RuntimeMetricsScopeSessionReader,
) factorysessions.RuntimeMetricsScopeResolver {
	if reader == nil {
		return nil
	}
	return runtimeMetricsScopeResolver{reader: reader}
}

type runtimeMetricsScopeResolver struct {
	reader factorysessions.RuntimeMetricsScopeSessionReader
}

func (resolver runtimeMetricsScopeResolver) ResolveRuntimeMetricsScope(
	ctx context.Context,
	requestedID string,
) (factorysessions.RuntimeMetricsScope, error) {
	requestedID = strings.TrimSpace(requestedID)
	if requestedID == "" {
		return factorysessions.RuntimeMetricsScope{}, fmt.Errorf(
			"%w: Factory Session ID is required",
			factorysessions.ErrRuntimeNotAvailable,
		)
	}
	if resolver.reader == nil {
		return factorysessions.RuntimeMetricsScope{}, fmt.Errorf(
			"%w: Factory Sessions scope reader is required",
			factorysessions.ErrRuntimeNotAvailable,
		)
	}
	projection, err := resolver.reader.GetFactorySession(ctx, requestedID)
	if err != nil {
		return factorysessions.RuntimeMetricsScope{}, err
	}
	identity := projection.Runtime.StreamIdentity
	if identity == nil || strings.TrimSpace(identity.FactorySessionID) == "" {
		return factorysessions.RuntimeMetricsScope{}, fmt.Errorf(
			"%w: Factory Session %q has no retained metrics scope",
			factorysessions.ErrRuntimeNotAvailable,
			requestedID,
		)
	}
	retainedIDs := make([]string, 0, len(projection.Runtime.RetainedMetricsSessionIDs)+1)
	for _, retainedID := range projection.Runtime.RetainedMetricsSessionIDs {
		retainedID = strings.TrimSpace(retainedID)
		if retainedID == "" {
			continue
		}
		alreadyRetained := false
		for _, existingID := range retainedIDs {
			if existingID == retainedID {
				alreadyRetained = true
				break
			}
		}
		if !alreadyRetained {
			retainedIDs = append(retainedIDs, retainedID)
		}
	}
	if len(retainedIDs) == 0 {
		retainedIDs = append(retainedIDs, strings.TrimSpace(identity.FactorySessionID))
	}
	return factorysessions.RuntimeMetricsScope{
		RequestedFactorySessionID: requestedID,
		RetainedFactorySessionIDs: retainedIDs,
	}, nil
}

var _ factorysessions.RuntimeMetricsScopeResolver = runtimeMetricsScopeResolver{}
