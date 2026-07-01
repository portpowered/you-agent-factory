package dataplane

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/factorysessions"
)

// LiveOpenHost starts live Factory Session runtimes through the composition-root
// runtime host seam without owning discovery or open policy.
type LiveOpenHost interface {
	OpenLiveSessionForTarget(ctx context.Context, target factorysessions.Target) (sessionID string, err error)
}
