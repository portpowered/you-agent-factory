package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func submitWorkRequestWithFactory(activeFactory factory.Factory) workRequestSubmitter {
	if activeFactory == nil {
		return nil
	}
	return func(ctx context.Context, request interfaces.WorkRequest) error {
		_, err := activeFactory.SubmitWorkRequest(ctx, request)
		return err
	}
}

func (fs *FactoryService) currentRuntimeSubmitter() workRequestSubmitter {
	return submitWorkRequestWithFactory(fs.currentFactory())
}

func (fs *FactoryService) preseedCurrentRuntimeInputs(ctx context.Context) error {
	runtimeBundle := fs.currentRuntimeBundle()
	if runtimeBundle == nil || runtimeBundle.listener == nil {
		return nil
	}
	if err := runtimeBundle.listener.PreseedInputs(ctx); err != nil {
		return fmt.Errorf("preseed inputs: %w", err)
	}
	return nil
}

func (fs *FactoryService) swapActiveRuntime(runtimeBundle *replacementFactoryRuntime) {
	if runtimeBundle == nil {
		fs.clearActiveRuntime()
		return
	}
	fs.runtimeMu.Lock()
	defer fs.runtimeMu.Unlock()
	fs.eventHistory = runtimeBundle.eventHistory
	fs.factory = runtimeBundle.factory
	fs.listener = runtimeBundle.listener
	fs.net = runtimeBundle.net
	fs.runtimeCfg = runtimeBundle.runtimeCfg
	fs.modelResources = runtimeBundle.modelResources
	fs.modelAssets = runtimeBundle.modelAssets
	fs.localModels = runtimeBundle.localModels
	fs.cfg.Dir = runtimeBundle.dir
}

func (fs *FactoryService) clearActiveRuntime() {
	fs.runtimeMu.Lock()
	defer fs.runtimeMu.Unlock()
	fs.eventHistory = nil
	fs.factory = nil
	fs.listener = nil
	fs.net = nil
	fs.runtimeCfg = nil
	fs.modelResources = nil
	fs.modelAssets = nil
	fs.localModels = nil
	if fs.cfg != nil && strings.TrimSpace(fs.factoryRootDir) != "" {
		fs.cfg.Dir = fs.factoryRootDir
	}
}

func (fs *FactoryService) currentRunState() *serviceRunState {
	fs.runMu.RLock()
	defer fs.runMu.RUnlock()
	return fs.runState
}

func (fs *FactoryService) currentLiveRuntime() *liveRuntimeHandle {
	fs.runMu.RLock()
	defer fs.runMu.RUnlock()
	if fs.runState == nil {
		return nil
	}
	return fs.runState.runtime
}

func (fs *FactoryService) setRunState(ctx context.Context, sessionID string, runtime *liveRuntimeHandle) {
	fs.runMu.Lock()
	defer fs.runMu.Unlock()
	if ctx == nil {
		fs.runState = nil
		return
	}
	fs.runState = &serviceRunState{
		ctx:       ctx,
		sessionID: sessionID,
		runtime:   runtime,
	}
}

func (fs *FactoryService) clearRunState() {
	fs.runMu.Lock()
	defer fs.runMu.Unlock()
	fs.runState = nil
}

func (h *liveRuntimeHandle) completed() bool {
	if h == nil {
		return true
	}
	select {
	case <-h.runDone:
		return true
	default:
		return false
	}
}

func (h *liveRuntimeHandle) result() error {
	if h == nil {
		return nil
	}
	h.runErrMu.RLock()
	defer h.runErrMu.RUnlock()
	return h.runErr
}

func (h *liveRuntimeHandle) setRunResult(err error) {
	h.runErrMu.Lock()
	h.runErr = err
	h.runErrMu.Unlock()
	close(h.runDone)
}

func (h *liveRuntimeHandle) wait() error {
	if h == nil {
		return nil
	}
	<-h.runDone
	return h.result()
}
