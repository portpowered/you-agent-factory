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
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/fileeffects"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	sessionruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimeports"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	identity "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/identity"
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

func (fs *SessionRuntime) PreseedRuntimeInputs(ctx context.Context, instance runtimeports.RuntimeInstance) error {
	if fs == nil || fs.runtimeSidecars == nil {
		return fmt.Errorf("runtime sidecar service is required")
	}
	return fs.runtimeSidecars.Preseed(ctx, instance)
}

// SessionRuntime owns live-session routing, runtime replacement, and the
// callbacks required by independently provided application services.
//
// Extracted domains are composed explicitly: Factory Sessions owns the live
// session registry, Models owns managed runtime wiring, Workers owns hosted
// pollers, and Automations owns cron and poller supervision.
type SessionRuntime struct {
	runtimeMu        sync.RWMutex
	runtimeState     runtimebinding.State
	sessionState     *sessionruntime.Service
	sessionGateway   sessionGateway
	runtimeBuild     runtimeports.RuntimeReplacementBuilder
	modelsScope      models.RuntimeScopeRef
	runtimeLifecycle runtimeports.RuntimeLifecycle
	runtimeSidecars  RuntimeSidecars
	factoryRootDir   string
	// startupBundle holds the built default runtime before Run registers ~default.
	startupSessionID               string
	dir                            string
	executionBaseDir               string
	runtimeMode                    interfaces.RuntimeMode
	backendScopeID                 string
	workFile                       string
	workflowID                     string
	workstationLoader              interfaces.WorkstationLoader
	loadFactory                    interfaces.LoadedFactoryLoader
	factoryScaffoldInitializer     factorysessions.FactoryScaffoldInitializer
	editableFactoryValidator       factorysessions.EditableFactoryValidator
	reconnectCursorValidator       factorysessions.ReconnectCursorValidator
	worldStateProjector            factory.WorldStateProjector
	invocationMetricsRecorder      roles.InvocationMetricsRecorder
	baseLogger                     *zap.Logger
	logger                         *zap.Logger
	startTime                      time.Time
	clock                          factory.Clock
	definitions                    interfaces.Service
	durableExecution               durableexecution.Service
	newJavaScriptCheckpointStore   factory.JavaScriptCheckpointStoreFactory
	sessionResultProjection        factory.SessionResultProjectionOperation
	directoryInspection            roles.DirectoryInspection
	sessionIDs                     factorysessions.SessionIDGenerator
	resolveHome                    factorysessions.HomeDirectoryResolver
	namedPaths                     interfaces.NamedPathResolver
	initialWorkFiles               fileeffects.InitialWorkReader
	identity                       identity.Service
	releaseWorkAdmissionProjection func(string)
	retireWorkAdmissionProjection  func(string, *factorysessions.LiveRuntime, factory.RuntimeRecord)
}

// ActivateNamedFactory builds a replacement runtime from a persisted named
// factory directory and swaps it in only after the current runtime is idle.
func (fs *SessionRuntime) ActivateNamedFactory(ctx context.Context, name string) error {
	if fs == nil {
		return fmt.Errorf("factory service is required")
	}
	svc := fs.requireDefinitions()
	if svc == nil {
		return fmt.Errorf("factory definition service is required")
	}
	return svc.ActivateNamedFactory(ctx, name)
}

func (fs *SessionRuntime) buildReplacementFactoryRuntime(
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
			fs.sessionState, folderPath, factoryDir, sessionID, fs.executionBaseDir,
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

func (fs *SessionRuntime) bindModelsRuntimeScope(bundle runtimeports.RuntimeInstance) error {
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

func (fs *SessionRuntime) StartDefaultRuntime(
	ctx context.Context,
	runCtx context.Context,
	serviceMode bool,
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
	if session := fs.sessionState.Resolve(sessionID); session != nil {
		target.Ref = session.Target
		target.FactoryDir = session.FactoryDir
		target.FolderPath = session.FolderPath
		target.Project = session.Project
	}
	return runtimebinding.StartInitial(
		ctx,
		runCtx,
		fs.sessionState,
		&fs.runtimeState,
		sessionID,
		fs.factoryRootDir,
		runtimeBundle,
		target,
		serviceMode,
		fs.runtimeMode,
		fs.runtimeLifecycle,
		fs.StartLiveRuntimeSidecars,
		fs.StopLiveRuntime,
		fs.releaseWorkAdmissionProjection,
	)
}

func (fs *SessionRuntime) requireIdleRuntime(ctx context.Context) error {
	sessionID := fs.runSessionID()
	if session := fs.sessionState.Resolve(sessionID); session != nil && runtimebinding.HandleFromSession(session) != nil {
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

func (fs *SessionRuntime) currentRuntimeBundle() factoryRuntimeBundle {
	if fs == nil {
		return nil
	}
	return runtimebinding.CurrentBundle(fs.sessionState, &fs.runtimeState)
}

// CurrentRuntimeBundle returns the active Factory Runtime bundle for
// initializer-owned startup diagnostics.
func (fs *SessionRuntime) CurrentRuntimeBundle() runtimeports.RuntimeInstance {
	return fs.currentRuntimeBundle()
}

func (fs *SessionRuntime) StartLiveRuntimeSidecars(ctx context.Context, handle liveRuntimeHandle) error {
	if fs == nil || fs.runtimeSidecars == nil {
		return fmt.Errorf("runtime sidecar service is required")
	}
	return fs.runtimeSidecars.Start(ctx, handle)
}

func (fs *SessionRuntime) StopLiveRuntimeSidecars(handle liveRuntimeHandle) {
	if fs != nil && fs.runtimeSidecars != nil {
		fs.runtimeSidecars.Stop(handle)
		return
	}
	if fs != nil && fs.runtimeLifecycle != nil {
		fs.runtimeLifecycle.StopSidecars(handle)
		return
	}
}

func (fs *SessionRuntime) StopLiveRuntime(handle liveRuntimeHandle) error {
	if handle == nil {
		return nil
	}
	if fs == nil {
		return fmt.Errorf("factory session service is required")
	}
	if fs.runtimeLifecycle == nil {
		return fmt.Errorf("factory runtime lifecycle service is required")
	}
	return fs.runtimeLifecycle.Stop(handle)
}

func (fs *SessionRuntime) ShutdownOtherLiveSessions(except liveRuntimeHandle) error {
	if fs == nil {
		return nil
	}
	var sessionIDs []string
	if fs.sessionState != nil && fs.sessionState.Registry() != nil {
		sessionIDs = fs.sessionState.Registry().IDs()
	}
	err := runtimebinding.ShutdownOtherLiveSessions(fs.sessionState, except, fs.StopLiveRuntime)
	if fs.releaseWorkAdmissionProjection != nil {
		for _, sessionID := range sessionIDs {
			if fs.sessionState.Resolve(sessionID) == nil {
				fs.releaseWorkAdmissionProjection(sessionID)
			}
		}
	}
	return err
}
