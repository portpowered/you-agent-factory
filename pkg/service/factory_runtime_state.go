package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
	factory_context "github.com/portpowered/infinite-you/pkg/factory/context"
	"github.com/portpowered/infinite-you/pkg/factory/requests"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/replay"
	"go.uber.org/zap"
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

// SubmitWorkRequest submits a canonical work request batch to the factory.
func (fs *FactoryService) SubmitWorkRequest(ctx context.Context, request interfaces.WorkRequest) (interfaces.WorkRequestSubmitResult, error) {
	fs.activationMu.RLock()
	defer fs.activationMu.RUnlock()

	activeFactory := fs.currentFactory()
	if activeFactory == nil {
		return interfaces.WorkRequestSubmitResult{}, fmt.Errorf("factory service runtime is not available")
	}
	return activeFactory.SubmitWorkRequest(ctx, request)
}

// SubscribeFactoryEvents returns canonical factory event history followed by
// live events from the current service-owned runtime.
func (fs *FactoryService) SubscribeFactoryEvents(ctx context.Context) (*interfaces.FactoryEventStream, error) {
	activeFactory := fs.currentFactory()
	if activeFactory == nil {
		return nil, fmt.Errorf("factory service runtime is not available")
	}
	stream, err := activeFactory.SubscribeFactoryEvents(ctx)
	if err != nil {
		return nil, fmt.Errorf("subscribe factory events: %w", err)
	}
	return stream, nil
}

// WaitToComplete returns a channel that is closed when all tokens reach
// terminal or failed places and no dispatches are in flight. Delegates to
// the underlying factory's termination signal.
func (fs *FactoryService) WaitToComplete() <-chan struct{} {
	activeFactory := fs.currentFactory()
	if activeFactory == nil {
		ch := make(chan struct{})
		close(ch)
		return ch
	}
	return activeFactory.WaitToComplete()
}

// GetEngineStateSnapshot returns the factory boundary's aggregate
// observability snapshot.
func (fs *FactoryService) GetEngineStateSnapshot(ctx context.Context) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
	activeFactory := fs.currentFactory()
	if activeFactory == nil {
		return nil, fmt.Errorf("factory service runtime is not available")
	}
	snap, err := activeFactory.GetEngineStateSnapshot(ctx)
	if err != nil {
		return nil, fmt.Errorf("get engine state snapshot: %w", err)
	}
	return snap, nil
}

// Pause pauses the current runtime instance.
func (fs *FactoryService) Pause(ctx context.Context) error {
	activeFactory := fs.currentFactory()
	if activeFactory == nil {
		return fmt.Errorf("factory service runtime is not available")
	}
	if err := activeFactory.Pause(ctx); err != nil {
		return fmt.Errorf("pause factory: %w", err)
	}
	return nil
}

// GetFactoryEvents returns the canonical factory event history.
func (fs *FactoryService) GetFactoryEvents(ctx context.Context) ([]factoryapi.FactoryEvent, error) {
	activeFactory := fs.currentFactory()
	if activeFactory == nil {
		return nil, fmt.Errorf("factory service runtime is not available")
	}
	events, err := activeFactory.GetFactoryEvents(ctx)
	if err != nil {
		return nil, fmt.Errorf("get factory events: %w", err)
	}
	return events, nil
}

func (fs *FactoryService) submitWorkFile(ctx context.Context) error {
	data, err := os.ReadFile(fs.cfg.WorkFile)
	if err != nil {
		return fmt.Errorf("read work file %s: %w", fs.cfg.WorkFile, err)
	}
	workRequest, err := requests.ParseCanonicalWorkRequestJSON(data)
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

func (fs *FactoryService) currentFactory() factory.Factory {
	if fs == nil {
		return nil
	}
	if compatibilitySession := fs.compatibilitySession(); compatibilitySession != nil && compatibilitySession.handle != nil && compatibilitySession.handle.runtime != nil {
		return compatibilitySession.handle.runtime.factory
	}
	fs.runtimeMu.RLock()
	defer fs.runtimeMu.RUnlock()
	return fs.factory
}

func (fs *FactoryService) currentRuntimeConfig() *factoryconfig.LoadedFactoryConfig {
	if fs == nil {
		return nil
	}
	if compatibilitySession := fs.compatibilitySession(); compatibilitySession != nil && compatibilitySession.handle != nil && compatibilitySession.handle.runtime != nil {
		return compatibilitySession.handle.runtime.runtimeCfg
	}
	fs.runtimeMu.RLock()
	defer fs.runtimeMu.RUnlock()
	return fs.runtimeCfg
}

func (fs *FactoryService) compatibilitySession() *liveFactorySession {
	if fs == nil {
		return nil
	}
	return fs.defaultSession()
}

func (fs *FactoryService) workflowID() string {
	if fs == nil || fs.cfg == nil {
		return ""
	}
	fs.runtimeMu.RLock()
	defer fs.runtimeMu.RUnlock()
	return fs.cfg.WorkflowID
}

func validateReplayModeConfig(cfg *FactoryServiceConfig) error {
	if cfg == nil {
		return fmt.Errorf("factory service config is required")
	}
	if cfg.RecordPath != "" && cfg.ReplayPath != "" {
		return fmt.Errorf("--record and --replay cannot be used together")
	}
	return nil
}

func loadFactoryConfigForMode(cfg *FactoryServiceConfig) (*factoryconfig.LoadedFactoryConfig, *interfaces.ReplayArtifact, error) {
	if cfg.ReplayPath == "" {
		loaded, err := factoryconfig.LoadRuntimeConfig(cfg.Dir, cfg.WorkstationLoader)
		if loaded != nil {
			loaded.SetRuntimeBaseDir(cfg.ExecutionBaseDir)
		}
		return loaded, nil, err
	}
	artifact, err := replay.Load(cfg.ReplayPath)
	if err != nil {
		return nil, nil, fmt.Errorf("load replay artifact: %w", err)
	}
	runtimeCfg, err := replay.RuntimeConfigFromGeneratedFactory(artifact.Factory)
	if err != nil {
		return nil, nil, fmt.Errorf("load embedded replay config: %w", err)
	}
	loaded, err := factoryconfig.NewLoadedFactoryConfig(runtimeCfg.FactoryDir(), runtimeCfg.Factory, runtimeCfg)
	if err != nil {
		return nil, nil, fmt.Errorf("build embedded replay config: %w", err)
	}
	loaded.SetRuntimeBaseDir(cfg.ExecutionBaseDir)
	return loaded, artifact, nil
}

func warnReplayMetadataMismatches(cfg *FactoryServiceConfig, artifact *interfaces.ReplayArtifact, logger *zap.Logger) {
	if artifact == nil || cfg == nil || cfg.Dir == "" {
		return
	}
	current, err := factoryconfig.LoadRuntimeConfig(cfg.Dir, cfg.WorkstationLoader)
	if err != nil {
		return
	}
	currentFactory, err := replay.GeneratedFactoryFromRuntimeConfig(
		current.FactoryDir(),
		current.FactoryConfig(),
		current,
		replay.WithGeneratedFactorySourceDirectory(current.FactoryDir()),
		replay.WithGeneratedFactoryWorkflowID(cfg.WorkflowID),
	)
	if err != nil {
		return
	}
	for _, warning := range replay.FactoryMetadataWarnings(artifact.Factory, currentFactory) {
		logger.Warn("replay artifact metadata differs from current checkout",
			zap.String("category", replay.DivergenceCategoryConfigMismatch),
			zap.String("metadata_key", warning.Key),
			zap.String("artifact", warning.Artifact),
			zap.String("current", warning.Current),
		)
	}
}

func warnPortableBundledReplacementReport(
	logger *zap.Logger,
	message string,
	replacements []factoryconfig.PortableBundledFileReplacement,
) {
	if logger == nil || len(replacements) == 0 {
		return
	}
	targets := make([]string, 0, len(replacements))
	for _, replacement := range replacements {
		targets = append(targets, replacement.TargetPath)
	}
	logger.Warn(message, zap.Strings("target_paths", targets))
}

func runtimeWorkflowContext(cfg *interfaces.FactoryConfig) *factory_context.FactoryContext {
	projectID := factory_context.DefaultProjectID
	if cfg != nil && cfg.Project != "" {
		projectID = factory_context.ResolveProjectID(cfg.Project, nil, nil)
	}
	return &factory_context.FactoryContext{
		ProjectID: projectID,
		EnvVars:   make(map[string]string),
	}
}

func newRecordingArtifact(
	cfg *FactoryServiceConfig,
	factoryDir string,
	factoryCfg *interfaces.FactoryConfig,
	runtimeCfg interfaces.RuntimeDefinitionLookup,
	clock factory.Clock,
) (*interfaces.ReplayArtifact, error) {
	if cfg.RecordPath == "" {
		return nil, nil
	}
	now := factory.EnsureClock(clock).Now().UTC()
	generatedFactory, err := replay.GeneratedFactoryFromRuntimeConfig(
		factoryDir,
		factoryCfg,
		runtimeCfg,
		replay.WithGeneratedFactorySourceDirectory(factoryDir),
		replay.WithGeneratedFactoryWorkflowID(cfg.WorkflowID),
	)
	if err != nil {
		return nil, fmt.Errorf("build replay artifact config: %w", err)
	}
	return replay.NewEventLogArtifactFromFactory(now, generatedFactory, &interfaces.ReplayWallClockMetadata{
		StartedAt: now,
	}, interfaces.ReplayDiagnostics{})
}

func (fs *FactoryService) finalizeRuntimeArtifacts(runtimeBundle *replacementFactoryRuntime) error {
	if runtimeBundle == nil {
		return nil
	}
	var errs []error
	if runtimeBundle.recording != nil {
		runtimeBundle.recording.Finish(factory.EnsureClock(fs.clock).Now().UTC())
		if err := runtimeBundle.recording.Flush(); err != nil {
			errs = append(errs, err)
		}
		if err := runtimeBundle.recording.Err(); err != nil {
			errs = append(errs, err)
		}
	}
	if runtimeBundle.logSink != nil {
		if err := runtimeBundle.logSink.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func newSessionLogger(base *zap.Logger, sessionID string, folderPath string, factoryDir string) *zap.Logger {
	if base == nil {
		base = zap.NewNop()
	}
	return base.With(
		zap.String("session_id", sessionID),
		zap.String("folder_path", folderPath),
		zap.String("factory_dir", factoryDir),
	)
}

func sessionScopedRecordPath(basePath string, sessionID string) string {
	if strings.TrimSpace(basePath) == "" {
		return basePath
	}
	if strings.Contains(basePath, "__factory_session_id__") {
		return strings.ReplaceAll(basePath, "__factory_session_id__", sessionID)
	}
	if sessionID == defaultFactorySessionID {
		return basePath
	}
	ext := filepath.Ext(basePath)
	base := strings.TrimSuffix(basePath, ext)
	return base + "." + sessionID + ext
}

func (r *replacementFactoryRuntime) runtimeLogger() *zap.Logger {
	if r == nil || r.logger == nil {
		return zap.NewNop()
	}
	return r.logger
}

func runtimeModeOrDefault(mode interfaces.RuntimeMode) interfaces.RuntimeMode {
	if mode == "" {
		return interfaces.RuntimeModeBatch
	}
	return mode
}

func (fs *FactoryService) waitForServiceModeStartupWorkReadability(ctx context.Context, serviceMode bool) error {
	if !serviceMode || fs.cfg.WorkFile == "" || fs.cfg.APIServerReady == nil || fs.cfg.Port <= 0 || fs.cfg.APIServerStarter == nil {
		return nil
	}
	apiServerExit := fs.apiServerExit
	select {
	case <-fs.cfg.APIServerReady:
	case err := <-apiServerExit:
		return startupReadinessError(err)
	case <-ctx.Done():
		return ctx.Err()
	}

	timer := time.NewTimer(serviceModeStartupWorkReadabilityDelay)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case err := <-apiServerExit:
		return startupReadinessError(err)
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (fs *FactoryService) failServiceModeStartup(currentRuntime *liveRuntimeHandle, startupErr error) error {
	fs.clearRunState()
	fs.unregisterLiveSession(defaultFactorySessionID)
	if currentRuntime == nil {
		return startupErr
	}
	if stopErr := fs.stopLiveRuntime(currentRuntime); stopErr != nil && !errors.Is(stopErr, context.Canceled) {
		return errors.Join(startupErr, stopErr)
	}
	return startupErr
}

func startupReadinessError(err error) error {
	if err == nil {
		return fmt.Errorf("wait for service-mode startup work readiness: API server stopped before signaling readiness")
	}
	return fmt.Errorf("wait for service-mode startup work readiness: %w", err)
}
