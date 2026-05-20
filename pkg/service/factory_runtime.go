package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/portpowered/infinite-you/pkg/cli/dashboardrender"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factory/projections"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"

	"go.uber.org/zap"
)

// Run starts the file watcher, dashboard, API server, and factory engine.
// It blocks until ctx is cancelled or the factory reaches a terminal state.
func (fs *FactoryService) Run(ctx context.Context) error {
	runCtx, cancelRunSidecars := context.WithCancel(ctx)
	var sidecars sync.WaitGroup
	serviceMode := runtimeModeOrDefault(fs.cfg.RuntimeMode) == interfaces.RuntimeModeService

	defer fs.finishRun(cancelRunSidecars, &sidecars)
	if err := fs.startRecording(runCtx); err != nil {
		return err
	}
	fs.startServiceSidecars(runCtx, &sidecars, serviceMode)
	if err := fs.prepareRuntimeInputs(ctx, serviceMode); err != nil {
		return err
	}

	currentRuntime, err := fs.startCurrentRuntime(ctx, runCtx, serviceMode)
	if err != nil {
		return err
	}
	err = fs.waitForActiveRuntime(ctx)
	if stopErr := fs.stopLiveRuntime(fs.currentLiveRuntime()); stopErr != nil && stopErr != context.Canceled && err == nil {
		err = stopErr
	}
	if writeErr := fs.finishRecording(); writeErr != nil {
		return writeErr
	}
	fs.renderFinalDashboard(ctx)

	if err != nil && err != context.Canceled {
		return fmt.Errorf("factory run: %w", err)
	}
	_ = currentRuntime
	return nil
}

func (fs *FactoryService) finishRun(cancelRunSidecars context.CancelFunc, sidecars *sync.WaitGroup) {
	if fs.logSink != nil {
		defer func() {
			if err := fs.logSink.Close(); err != nil {
				fs.logger.Warn("runtime log close failed", zap.Error(err))
			}
		}()
	}
	cancelRunSidecars()
	fs.clearRunState()
	sidecars.Wait()
}

func (fs *FactoryService) startRecording(runCtx context.Context) error {
	if fs.recording == nil {
		return nil
	}
	fs.recording.Start(runCtx)
	return fs.recording.Flush()
}

func (fs *FactoryService) startServiceSidecars(runCtx context.Context, sidecars *sync.WaitGroup, serviceMode bool) {
	if !serviceMode {
		sidecars.Add(1)
		go func() {
			defer sidecars.Done()
			if err := fs.listener.Watch(runCtx); err != nil && err != context.Canceled {
				fs.logger.Error("file watcher error", zap.Error(err))
			}
		}()
	}
	if fs.cfg.APIServerStarter != nil && fs.cfg.Port > 0 {
		sidecars.Add(1)
		go func() {
			defer sidecars.Done()
			if err := fs.cfg.APIServerStarter(runCtx, fs, fs.cfg.Port, fs.logger); err != nil {
				fs.logger.Error("API server error", zap.Error(err))
			}
		}()
	}
	fs.startTime = fs.clock.Now()
	if fs.cfg.SimpleDashboardRenderer != nil {
		sidecars.Add(1)
		go func() {
			defer sidecars.Done()
			fs.dashboardLoop(runCtx)
		}()
	}
}

func (fs *FactoryService) prepareRuntimeInputs(ctx context.Context, serviceMode bool) error {
	if !serviceMode {
		if err := fs.preseedCurrentRuntimeInputs(ctx); err != nil {
			return err
		}
	}
	if fs.cfg.WorkFile != "" {
		if err := fs.submitWorkFile(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (fs *FactoryService) startCurrentRuntime(ctx context.Context, runCtx context.Context, serviceMode bool) (*liveRuntimeHandle, error) {
	currentRuntime := fs.startLiveRuntime(runCtx, fs.currentRuntimeBundle())
	fs.setRunState(runCtx, currentRuntime)
	if err := fs.waitForLiveRuntimeStart(ctx, currentRuntime); err != nil {
		fs.clearRunState()
		_ = fs.stopLiveRuntime(currentRuntime)
		return nil, fmt.Errorf("start runtime: %w", err)
	}
	if serviceMode {
		if err := fs.startLiveRuntimeSidecars(runCtx, currentRuntime); err != nil {
			fs.clearRunState()
			_ = fs.stopLiveRuntime(currentRuntime)
			return nil, err
		}
	}
	fs.logFactoryStart()
	return currentRuntime, nil
}

func (fs *FactoryService) logFactoryStart() {
	runtimeLogConfig := fs.logSink.Config()
	fs.logger.Info("factory started",
		zap.String("dir", fs.cfg.Dir),
		zap.String("runtime_log_path", fs.logSink.Path()),
		zap.String("runtime_log_appender", logging.RuntimeLogAppenderZapRollingFile),
		zap.Int("runtime_log_max_size_mb", runtimeLogConfig.MaxSize),
		zap.Int("runtime_log_max_backups", runtimeLogConfig.MaxBackups),
		zap.Int("runtime_log_max_age_days", runtimeLogConfig.MaxAge),
		zap.Bool("runtime_log_compress", runtimeLogConfig.Compress),
		zap.String("runtime_env_log_channel", logging.RuntimeEnvLogChannelRecord),
		zap.String("runtime_success_command_output", logging.RuntimeSuccessCommandOutputPolicy),
		zap.String("runtime_failure_command_output", logging.RuntimeFailureCommandOutputPolicy),
		zap.String("runtime_verbose_command_output", logging.RuntimeVerboseCommandOutputPolicy),
		zap.String("record_command_diagnostics", logging.RuntimeRecordCommandDiagnosticsMode),
		zap.String("runtime_mode", string(runtimeModeOrDefault(fs.cfg.RuntimeMode))),
		zap.Bool("mock-workers", fs.cfg.MockWorkersConfig != nil),
		zap.Int("port", fs.cfg.Port),
	)
}

func (fs *FactoryService) finishRecording() error {
	if fs.recording != nil {
		fs.recording.Finish(fs.clock.Now().UTC())
	}
	if writeErr := fs.writeRecording(); writeErr != nil {
		return writeErr
	}
	if fs.recording != nil {
		if recordErr := fs.recording.Err(); recordErr != nil {
			return recordErr
		}
	}
	return nil
}

func (fs *FactoryService) renderFinalDashboard(ctx context.Context) {
	if fs.cfg.SimpleDashboardRenderer != nil {
		fs.renderDashboard(ctx)
	}
}

func (fs *FactoryService) startLiveRuntime(ctx context.Context, runtimeBundle *replacementFactoryRuntime) *liveRuntimeHandle {
	if runtimeBundle == nil {
		return nil
	}
	runCtx, runCancel := context.WithCancel(ctx)
	handle := &liveRuntimeHandle{
		runtime:   runtimeBundle,
		runCancel: runCancel,
		runDone:   make(chan struct{}),
	}
	go func() {
		handle.setRunResult(runtimeBundle.factory.Run(runCtx))
	}()
	return handle
}

func (fs *FactoryService) startLiveRuntimeSidecars(ctx context.Context, handle *liveRuntimeHandle) error {
	if handle == nil || handle.runtime == nil {
		return fmt.Errorf("runtime handle is required")
	}

	handle.sidecarMu.Lock()
	defer handle.sidecarMu.Unlock()
	if handle.sidecarCancel != nil {
		return nil
	}

	sidecarCtx, sidecarCancel := context.WithCancel(ctx)
	handle.sidecarCancel = sidecarCancel
	if handle.runtime.listener != nil {
		handle.sidecars.Add(1)
		go func() {
			defer handle.sidecars.Done()
			if err := handle.runtime.listener.Watch(sidecarCtx); err != nil && err != context.Canceled {
				fs.logger.Error("file watcher error", zap.Error(err))
			}
		}()
	}

	fs.startCronWatchersForRuntime(
		sidecarCtx,
		&handle.sidecars,
		handle.runtime.runtimeCfg.FactoryDir(),
		handle.runtime.runtimeCfg.FactoryConfig(),
		handle.runtime.runtimeCfg,
		submitWorkRequestWithFactory(handle.runtime.factory),
	)
	if handle.runtime.listener != nil {
		if err := handle.runtime.listener.PreseedInputs(sidecarCtx); err != nil {
			sidecarCancel()
			handle.sidecars.Wait()
			handle.sidecarCancel = nil
			return fmt.Errorf("preseed inputs: %w", err)
		}
	}
	return nil
}

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

func (fs *FactoryService) stopLiveRuntimeSidecars(handle *liveRuntimeHandle) {
	if handle == nil {
		return
	}
	handle.sidecarMu.Lock()
	cancel := handle.sidecarCancel
	handle.sidecarCancel = nil
	handle.sidecarMu.Unlock()
	if cancel == nil {
		return
	}
	cancel()
	handle.sidecars.Wait()
}

func (fs *FactoryService) restoreLiveRuntimeSidecars(runState *serviceRunState) {
	if runState == nil || runState.ctx == nil || runState.runtime == nil {
		return
	}
	if err := fs.startLiveRuntimeSidecars(runState.ctx, runState.runtime); err != nil {
		fs.logger.Error("restore prior runtime sidecars failed", zap.Error(err))
	}
}

func (fs *FactoryService) stopLiveRuntime(handle *liveRuntimeHandle) error {
	if handle == nil {
		return nil
	}
	fs.stopLiveRuntimeSidecars(handle)
	if handle.runCancel != nil {
		handle.runCancel()
	}
	return handle.wait()
}

func (fs *FactoryService) waitForLiveRuntimeStart(ctx context.Context, handle *liveRuntimeHandle) error {
	if handle == nil || handle.runtime == nil {
		return fmt.Errorf("runtime handle is required")
	}

	startCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-startCtx.Done():
			if errors.Is(startCtx.Err(), context.Canceled) {
				_ = handle.wait()
				return handle.result()
			}
			if handle.completed() {
				return handle.result()
			}
			return startCtx.Err()
		case <-handle.runDone:
			return handle.result()
		case <-ticker.C:
			snap, err := handle.runtime.factory.GetEngineStateSnapshot(context.Background())
			if err != nil {
				continue
			}
			if snap.FactoryState == string(interfaces.FactoryStateRunning) {
				return nil
			}
		}
	}
}

func (fs *FactoryService) waitForActiveRuntime(ctx context.Context) error {
	for {
		handle := fs.currentLiveRuntime()
		if handle == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			_ = handle.wait()
		case <-handle.runDone:
		}
		if fs.currentLiveRuntime() != handle {
			continue
		}
		return handle.result()
	}
}

func (fs *FactoryService) swapActiveRuntime(runtimeBundle *replacementFactoryRuntime) {
	fs.runtimeMu.Lock()
	defer fs.runtimeMu.Unlock()
	fs.eventHistory = runtimeBundle.eventHistory
	fs.factory = runtimeBundle.factory
	fs.listener = runtimeBundle.listener
	fs.net = runtimeBundle.net
	fs.runtimeCfg = runtimeBundle.runtimeCfg
	fs.cfg.Dir = runtimeBundle.dir
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

func (fs *FactoryService) setRunState(ctx context.Context, runtime *liveRuntimeHandle) {
	fs.runMu.Lock()
	defer fs.runMu.Unlock()
	if ctx == nil || runtime == nil {
		fs.runState = nil
		return
	}
	fs.runState = &serviceRunState{
		ctx:     ctx,
		runtime: runtime,
	}
}

func (fs *FactoryService) clearRunState() {
	fs.setRunState(nil, nil)
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

func (fs *FactoryService) submitWorkFile(ctx context.Context) error {
	data, err := os.ReadFile(fs.cfg.WorkFile)
	if err != nil {
		return fmt.Errorf("read work file %s: %w", fs.cfg.WorkFile, err)
	}
	workRequest, err := factory.ParseCanonicalWorkRequestJSON(data)
	if err != nil {
		return fmt.Errorf("parse work file %s: %w", fs.cfg.WorkFile, err)
	}
	activeFactory := fs.currentFactory()
	if activeFactory == nil {
		return fmt.Errorf("factory service runtime is not available")
	}
	if _, err := activeFactory.SubmitWorkRequest(ctx, workRequest); err != nil {
		return fmt.Errorf("submit initial work: %w", err)
	}
	fs.logger.Info("submitted initial work", zap.String("file", fs.cfg.WorkFile))
	return nil
}

func (fs *FactoryService) dashboardLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			fs.renderDashboard(ctx)
		}
	}
}

func (fs *FactoryService) renderDashboard(ctx context.Context) {
	now := factory.EnsureClock(fs.clock).Now()
	input, err := fs.buildSimpleDashboardRenderInput(ctx, now)
	if err != nil {
		if fs.logger != nil {
			fs.logger.Error("simple dashboard render failed", zap.Error(err))
		}
		return
	}
	fs.cfg.SimpleDashboardRenderer(input)
}

func (fs *FactoryService) buildSimpleDashboardRenderInput(ctx context.Context, now time.Time) (SimpleDashboardRenderInput, error) {
	es, err := fs.GetEngineStateSnapshot(ctx)
	if err != nil {
		return SimpleDashboardRenderInput{}, err
	}
	renderData, err := fs.simpleDashboardRenderData(ctx, es.TickCount, es.ActiveThrottlePauses)
	if err != nil {
		return SimpleDashboardRenderInput{}, err
	}
	return SimpleDashboardRenderInput{
		EngineState: *es,
		RenderData:  renderData,
		Now:         now,
	}, nil
}

func (fs *FactoryService) simpleDashboardRenderData(
	ctx context.Context,
	selectedTick int,
	activeThrottlePauses []interfaces.ActiveThrottlePause,
) (dashboardrender.SimpleDashboardRenderData, error) {
	events, err := fs.GetFactoryEvents(ctx)
	if err != nil {
		return dashboardrender.SimpleDashboardRenderData{}, err
	}
	worldState, err := projections.ReconstructFactoryWorldState(events, selectedTick)
	if err != nil {
		return dashboardrender.SimpleDashboardRenderData{}, err
	}
	renderData := dashboardrender.SimpleDashboardRenderDataFromWorldState(worldState)
	renderData.ActiveThrottlePauses = projections.ProjectActiveThrottlePauses(worldState.Topology, activeThrottlePauses)
	return renderData, nil
}
