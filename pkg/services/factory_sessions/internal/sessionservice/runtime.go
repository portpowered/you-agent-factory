// Package service owns Factory Session lifecycle, routing, and application
// operations. Wire constructs it; initializer activates its runtime lifecycle.
package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	sessionruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimeports"
	"github.com/portpowered/infinite-you/pkg/services/models"

	"go.uber.org/zap"
)

// factoryRuntimeBundle is the compatibility record retained by the binding
// edge while Runtime exposes the private instance-host operations.
type factoryRuntimeBundle = runtimeports.RuntimeInstance

type liveRuntimeHandle = runtimeports.RuntimeHandle

type RuntimeSidecars interface {
	Preseed(context.Context, runtimeports.RuntimeInstance) error
	Start(context.Context, runtimeports.RuntimeHandle) error
	Stop(runtimeports.RuntimeHandle)
}

func runtimeModeOrDefault(mode interfaces.RuntimeMode) interfaces.RuntimeMode {
	if mode == "" {
		return interfaces.RuntimeModeBatch
	}
	return mode
}

func (a *Assembly) scopedPreseedRuntimeInputs(fs *SessionRuntime, ctx context.Context, instance runtimeports.RuntimeInstance) error {
	if fs == nil || fs.runtimeSidecars == nil {
		return fmt.Errorf("runtime sidecar service is required")
	}
	return fs.runtimeSidecars.Preseed(ctx, instance)
}

// SessionRuntime is the scoped lifecycle handle used by the process and
// Definition compatibility edges. Reusable operations and collaborators stay
// on its fixed owner; this handle holds only selected facts and acquired state.
type SessionRuntime struct {
	runtimeID        string
	generationID     string
	owner            *Assembly
	releaseMu        sync.Mutex
	releaseDone      bool
	runtimeMu        sync.RWMutex
	runtimeState     runtimebinding.State
	openingSession   *livesession.LiveSession
	runtimeBuild     runtimeports.RuntimeReplacementBuilder
	modelsScope      models.RuntimeScopeRef
	runtimeLifecycle runtimeports.RuntimeLifecycle
	runtimeSidecars  RuntimeSidecars
	factoryRootDir   string
	// startupBundle holds the built default runtime before Run registers ~default.
	startupSessionID string
	dir              string
	executionBaseDir string
	runtimeMode      interfaces.RuntimeMode
	backendScopeID   string
	workFile         string
	startupWorkOnce  sync.Once
	startupWorkErr   error
	workflowID       string
	logger           *zap.Logger
	callerContext    context.Context
	startTime        time.Time
	clock            factory.Clock
}

// ActivateNamedFactory builds a replacement runtime from a persisted named
// factory directory and swaps it in only after the current runtime is idle.
func (a *Assembly) scopedActivateNamedFactory(fs *SessionRuntime, ctx context.Context, name string) error {
	if fs == nil {
		return fmt.Errorf("factory service is required")
	}
	return a.factoryDefinitions.ActivateNamedFactory(ctx, name)
}

func (a *Assembly) scopeBuildReplacementFactoryRuntime(fs *SessionRuntime,
	ctx context.Context,
	folderPath string,
	factoryDir string,
	sessionID string,
) (factoryRuntimeBundle, error) {
	if fs == nil || fs.runtimeBuild == nil {
		return nil, fmt.Errorf("factory service is required")
	}
	bundle, err := fs.runtimeBuild.BuildReplacement(
		ctx,
		folderPath,
		factoryDir,
		sessionID,
		runtimebinding.ReplacementExecutionBaseDir(
			a.state, folderPath, factoryDir, sessionID, fs.executionBaseDir,
		),
	)
	if err != nil {
		return nil, err
	}
	if err := fs.bindModelsRuntimeScope(bundle); err != nil {
		return nil, errors.Join(err, bundle.CloseArtifacts())
	}
	fs.bindRuntimeReadMetrics(bundle)
	return bundle, nil
}

func (a *Assembly) scopeBindModelsRuntimeScope(fs *SessionRuntime, bundle runtimeports.RuntimeInstance) error {
	if fs == nil || fs.modelsScope.IsZero() {
		return nil
	}
	if bundle == nil {
		return fmt.Errorf("replacement Factory Runtime is unavailable")
	}
	binder, ok := bundle.(interface {
		BindModelsRuntimeScope(models.RuntimeScopeRef) error
	})
	if !ok {
		return fmt.Errorf("replacement Factory Runtime does not support Models runtime scope binding")
	}
	if err := binder.BindModelsRuntimeScope(fs.modelsScope); err != nil {
		return fmt.Errorf("bind Models runtime scope to replacement Factory Runtime: %w", err)
	}
	return nil
}

func (a *Assembly) scopedStartDefaultRuntime(fs *SessionRuntime,
	ctx context.Context,
	runCtx context.Context,
) (liveRuntimeHandle, error) {
	if fs == nil {
		return nil, fmt.Errorf("factory session service is required")
	}
	runtimeBundle := fs.currentRuntimeBundle()
	sessionID := strings.TrimSpace(fs.startupSessionID)
	if sessionID == "" {
		sessionID = factorysessions.DefaultSessionID
	}
	target := sessionruntime.DefaultTarget(runtimeBundle.Directory(), runtimeBundle.FolderDirectory(), fs.factoryRootDir)
	if session := a.state.Resolve(sessionID); session != nil {
		placement := session.Placement()
		target.Ref = placement.Target
		target.FactoryDir = session.FactoryDir
		target.FolderPath = placement.FolderPath
		target.Project = placement.Project
	}
	return runtimebinding.StartInitial(
		ctx,
		runCtx,
		a.state,
		&fs.runtimeState,
		sessionID,
		fs.factoryRootDir,
		runtimeBundle,
		target,
		fs.runtimeMode,
		fs.runtimeLifecycle,
		fs.StopLiveRuntime,
		a.releaseWorkAdmissionProjection,
	)
}

func (a *Assembly) scopeRequireIdleRuntime(fs *SessionRuntime, ctx context.Context) error {
	sessionID := fs.runSessionID()
	if session := a.state.Resolve(sessionID); session != nil && runtimebinding.HandleFromSession(session) != nil {
		return fs.requireIdleRuntimeForSession(ctx, sessionID)
	}

	runtime := fs.currentRuntimeService()
	if runtime == nil {
		return fmt.Errorf("factory runtime is not available")
	}
	observationResult, err := runtime.Observe(ctx, factory.ObserveRequest{
		Scope: factory.ObservationScopeFull,
	})
	if err != nil {
		return fmt.Errorf("read current runtime status: %w", err)
	}
	return factory.RequireIdleRuntimeFromObservation(observationResult.Observation)
}

func (a *Assembly) scopeCurrentRuntimeBundle(fs *SessionRuntime) factoryRuntimeBundle {
	if fs == nil {
		return nil
	}
	return runtimebinding.CurrentBundle(a.state, &fs.runtimeState)
}

// CurrentRuntimeBundle returns the active Factory Runtime bundle for
// initializer-owned startup diagnostics.
func (a *Assembly) scopedCurrentRuntimeBundle(fs *SessionRuntime) runtimeports.RuntimeInstance {
	return fs.currentRuntimeBundle()
}

func (a *Assembly) scopedStartLiveRuntimeSidecars(fs *SessionRuntime, ctx context.Context, handle liveRuntimeHandle) error {
	if fs == nil || fs.runtimeSidecars == nil {
		return fmt.Errorf("runtime sidecar service is required")
	}
	return fs.runtimeSidecars.Start(ctx, handle)
}

func (a *Assembly) scopedStopLiveRuntimeSidecars(fs *SessionRuntime, handle liveRuntimeHandle) {
	if fs != nil && fs.runtimeSidecars != nil {
		fs.runtimeSidecars.Stop(handle)
	}
}

func (a *Assembly) scopedStopLiveRuntime(fs *SessionRuntime, handle liveRuntimeHandle) error {
	if handle == nil {
		return nil
	}
	if fs == nil {
		return fmt.Errorf("factory session service is required")
	}
	if fs.runtimeLifecycle == nil {
		return fmt.Errorf("factory runtime lifecycle service is required")
	}
	fs.runtimeMu.RLock()
	caller := fs.callerContext
	fs.runtimeMu.RUnlock()
	if caller != nil && errors.Is(caller.Err(), context.Canceled) {
		if producer, ok := handle.RuntimeInstance().(interface{ RecordProducerError(error) }); ok {
			producer.RecordProducerError(context.Canceled)
		}
	}
	return fs.runtimeLifecycle.Stop(handle)
}

func (a *Assembly) scopedShutdownOtherLiveSessions(fs *SessionRuntime, except liveRuntimeHandle) error {
	if fs == nil {
		return nil
	}
	var sessionIDs []string
	if a.state != nil && a.state.Registry() != nil {
		sessionIDs = a.state.Registry().IDs()
	}
	err := runtimebinding.ShutdownOtherLiveSessions(a.state, except, fs.StopLiveRuntime)
	for _, sessionID := range sessionIDs {
		if a.state.Resolve(sessionID) == nil {
			a.releaseWorkAdmissionProjection(sessionID)
		}
	}
	return err
}

func (fs *SessionRuntime) PreseedRuntimeInputs(ctx context.Context, instance runtimeports.RuntimeInstance) error {
	var a *Assembly
	if fs != nil {
		a = fs.owner
	}
	return a.scopedPreseedRuntimeInputs(fs, ctx, instance)
}

func (fs *SessionRuntime) ActivateNamedFactory(ctx context.Context, name string) error {
	var a *Assembly
	if fs != nil {
		a = fs.owner
	}
	return a.scopedActivateNamedFactory(fs, ctx, name)
}

func (fs *SessionRuntime) buildReplacementFactoryRuntime(
	ctx context.Context,
	folderPath string,
	factoryDir string,
	sessionID string,
) (factoryRuntimeBundle, error) {
	var a *Assembly
	if fs != nil {
		a = fs.owner
	}
	return a.scopeBuildReplacementFactoryRuntime(fs, ctx, folderPath, factoryDir, sessionID)
}

func (fs *SessionRuntime) bindModelsRuntimeScope(bundle runtimeports.RuntimeInstance) error {
	var a *Assembly
	if fs != nil {
		a = fs.owner
	}
	return a.scopeBindModelsRuntimeScope(fs, bundle)
}

func (fs *SessionRuntime) StartDefaultRuntime(
	ctx context.Context,
	runCtx context.Context,
) (liveRuntimeHandle, error) {
	var a *Assembly
	if fs != nil {
		a = fs.owner
	}
	return a.scopedStartDefaultRuntime(fs, ctx, runCtx)
}

func (fs *SessionRuntime) requireIdleRuntime(ctx context.Context) error {
	var a *Assembly
	if fs != nil {
		a = fs.owner
	}
	return a.scopeRequireIdleRuntime(fs, ctx)
}

func (fs *SessionRuntime) currentRuntimeBundle() factoryRuntimeBundle {
	var a *Assembly
	if fs != nil {
		a = fs.owner
	}
	return a.scopeCurrentRuntimeBundle(fs)
}

func (fs *SessionRuntime) CurrentRuntimeBundle() roles.RuntimeObservations {
	var a *Assembly
	if fs != nil {
		a = fs.owner
	}
	return a.scopedCurrentRuntimeBundle(fs)
}

func (fs *SessionRuntime) StartLiveRuntimeSidecars(ctx context.Context, handle liveRuntimeHandle) error {
	var a *Assembly
	if fs != nil {
		a = fs.owner
	}
	return a.scopedStartLiveRuntimeSidecars(fs, ctx, handle)
}

func (fs *SessionRuntime) StopLiveRuntimeSidecars(handle liveRuntimeHandle) {
	var a *Assembly
	if fs != nil {
		a = fs.owner
	}
	a.scopedStopLiveRuntimeSidecars(fs, handle)
}

func (fs *SessionRuntime) StopLiveRuntime(handle liveRuntimeHandle) error {
	var a *Assembly
	if fs != nil {
		a = fs.owner
	}
	return a.scopedStopLiveRuntime(fs, handle)
}

func (fs *SessionRuntime) ShutdownOtherLiveSessions(except liveRuntimeHandle) error {
	var a *Assembly
	if fs != nil {
		a = fs.owner
	}
	return a.scopedShutdownOtherLiveSessions(fs, except)
}
