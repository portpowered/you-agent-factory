package dataplane

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

// LiveOpenHost starts live Factory Session runtimes through the composition-root
// runtime host seam without owning discovery or open policy.
type LiveOpenHost interface {
	OpenLiveSessionForTarget(ctx context.Context, target factorysessions.Target) (sessionID string, err error)
}

// LiveLifecycleHost supervises live session pause, resume, and close through the
// composition-root runtime host seam.
type LiveLifecycleHost interface {
	SessionFactory(sessionID string) (factory.Service, error)
	StopLiveSession(sessionID string) error
	ObserveLiveLifecycleControl(
		sessionID string,
		operation factorysessions.LifecycleControlKind,
		control factorysessions.ControlRequest,
		outcome factorysessions.LifecycleControlOutcome,
		status factorysessions.LifecycleStatus,
		err error,
	)
}
