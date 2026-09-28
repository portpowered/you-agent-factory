package service

import (
	"context"
	"errors"
	"sync"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
)

// sessionActivation follows the canonical live-session record. Each cleanup
// phase is retried only until it succeeds, including after a canceled caller.
type sessionActivation struct {
	mu               sync.Mutex
	lifecycle        roles.LifecycleRuntime
	stopWorker       factorysessions.RuntimeStop
	cancel           context.CancelFunc
	closeArtifacts   func() error
	workerStopped    bool
	lifecycleStopped bool
	artifactsClosed  bool
}

func (a *sessionActivation) Close(ctx context.Context) error {
	if a == nil {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	var result error
	if a.stopWorker != nil && !a.workerStopped {
		if err := a.stopWorker(ctx); err != nil {
			result = errors.Join(result, err)
		} else {
			a.workerStopped = true
		}
	}
	if a.lifecycle != nil && !a.lifecycleStopped {
		if err := a.lifecycle.StopLifecycle(ctx); err != nil {
			result = errors.Join(result, err)
		} else {
			a.lifecycleStopped = true
		}
	}
	if a.cancel != nil {
		a.cancel()
	}
	if a.closeArtifacts != nil && !a.artifactsClosed {
		if err := a.closeArtifacts(); err != nil {
			result = errors.Join(result, err)
		} else {
			a.artifactsClosed = true
		}
	}
	return result
}
