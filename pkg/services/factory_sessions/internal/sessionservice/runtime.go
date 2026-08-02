// Package service owns Factory Session lifecycle, routing, and application
// operations. Wire constructs it; initializer activates its runtime lifecycle.
package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/fileeffects"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	sessionruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	identity "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/identity"

	"go.uber.org/zap"
)

// factoryRuntimeBundle is the public capability view owned by Factory Runtime.
type factoryRuntimeBundle = factory.HostedInstance

// liveRuntimeHandle is the public lifecycle handle owned by Factory Runtime.
type liveRuntimeHandle = factory.HostedHandle

type RuntimeSidecars interface {
	Preseed(context.Context, factory.HostedInstance) error
	Start(context.Context, factory.HostedHandle) error
	Stop(factory.HostedHandle)
}

func (fs *SessionRuntime) PreseedRuntimeInputs(ctx context.Context, instance factory.HostedInstance) error {
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
	runtimeBuild     factory.ReplacementBuilder
	runtimeLifecycle factory.Lifecycle
	runtimeSidecars  RuntimeSidecars
	factoryRootDir   string
	// startupBundle holds the built default runtime before Run registers ~default.
	dir                          string
	executionBaseDir             string
	runtimeMode                  interfaces.RuntimeMode
	backendScopeID               string
	workFile                     string
	workflowID                   string
	workstationLoader            interfaces.WorkstationLoader
	loadFactory                  interfaces.LoadedFactoryLoader
	factoryScaffoldInitializer   factorysessions.FactoryScaffoldInitializer
	editableFactoryValidator     factorysessions.EditableFactoryValidator
	reconnectCursorValidator     factorysessions.ReconnectCursorValidator
	worldStateProjector          factory.WorldStateProjector
	invocationMetricsRecorder    roles.InvocationMetricsRecorder
	baseLogger                   *zap.Logger
	logger                       *zap.Logger
	startTime                    time.Time
	clock                        factory.Clock
	definitions                  interfaces.Service
	durableExecution             durableexecution.Service
	newJavaScriptCheckpointStore factory.JavaScriptCheckpointStoreFactory
	sessionResultProjection      factory.SessionResultProjectionOperation
	directoryInspection          roles.DirectoryInspection
	sessionIDs                   factorysessions.SessionIDGenerator
	resolveHome                  factorysessions.HomeDirectoryResolver
	namedPaths                   interfaces.NamedPathResolver
	initialWorkFiles             fileeffects.InitialWorkReader
	identity                     identity.Service
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
	return bundle, nil
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
	return runtimebinding.StartDefault(
		ctx,
		runCtx,
		fs.sessionState,
		&fs.runtimeState,
		fs.factoryRootDir,
		runtimeBundle,
		sessionruntime.DefaultTarget(runtimeBundle.Directory(), runtimeBundle.FolderDirectory(), fs.factoryRootDir),
		serviceMode,
		fs.runtimeMode,
		fs.runtimeLifecycle,
		fs.StartLiveRuntimeSidecars,
		fs.StopLiveRuntime,
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
func (fs *SessionRuntime) CurrentRuntimeBundle() factory.HostedInstance {
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
	return runtimebinding.ShutdownOtherLiveSessions(fs.sessionState, except, fs.StopLiveRuntime)
}

func (fs *SessionRuntime) waitForActiveRuntime(ctx context.Context) error {
	for {
		handle := fs.runtimeState.ActiveHandle()
		if handle == nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(25 * time.Millisecond):
				continue
			}
		}
		select {
		case <-ctx.Done():
			_ = handle.Wait()
		case <-handle.RunDoneCh():
		}
		if fs.runtimeState.ActiveHandle() != handle {
			continue
		}
		if factory.RuntimeModeOrDefault(fs.runtimeMode) == interfaces.RuntimeModeService &&
			fs.sessionState.Registry() != nil && fs.sessionState.Registry().Count() == 0 {
			continue
		}
		return handle.Result()
	}
}
