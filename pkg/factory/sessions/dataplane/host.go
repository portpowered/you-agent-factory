package dataplane

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factory/sessions"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
)

// LiveOpenHost starts live Factory Session runtimes through the composition-root
// runtime host seam without owning discovery or open policy.
type LiveOpenHost interface {
	OpenLiveSessionForTarget(ctx context.Context, target factorysessions.Target) (sessionID string, err error)
}

// LiveLifecycleHost supervises live session pause, resume, and close through the
// composition-root runtime host seam.
type LiveLifecycleHost interface {
	SessionFactory(sessionID string) (factory.Factory, error)
	StopLiveSession(sessionID string) error
	ObserveLiveLifecycleControl(
		sessionID string,
		operation factorysessionexecution.LifecycleControlKind,
		control factorysessionexecution.ControlRequest,
		outcome factorysessionexecution.LifecycleControlOutcome,
		status factorysessionexecution.LifecycleStatus,
		err error,
	)
}
