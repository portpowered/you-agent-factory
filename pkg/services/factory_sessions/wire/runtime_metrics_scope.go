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
	reader interface {
		GetFactorySession(context.Context, string) (factorysessions.SessionProjection, error)
	},
) factorysessions.RuntimeMetricsScopeResolver {
	if reader == nil {
		return nil
	}
	return runtimeMetricsScopeResolver{reader: reader}
}

type runtimeMetricsScopeResolver struct {
	reader interface {
		GetFactorySession(context.Context, string) (factorysessions.SessionProjection, error)
	}
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
	// Keep the public selector alongside the canonical lineage. Older replay
	// artifacts can legitimately retain the selector (notably ~default) in
	// their runtime metric envelope even when the live session now exposes a
	// generated canonical identity. The resolved session is authoritative, so
	// this compatibility identity cannot broaden an unknown-session query.
	if !containsRuntimeMetricsSessionID(retainedIDs, requestedID) {
		retainedIDs = append(retainedIDs, requestedID)
	}
	return factorysessions.RuntimeMetricsScope{
		RequestedFactorySessionID: requestedID,
		RetainedFactorySessionIDs: retainedIDs,
	}, nil
}

func containsRuntimeMetricsSessionID(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

var _ factorysessions.RuntimeMetricsScopeResolver = runtimeMetricsScopeResolver{}
