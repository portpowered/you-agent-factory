// Wire-facing construction consumes narrow service dependencies.
package service

import (
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/fileeffects"
	identity "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/identity"
	sessionruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtime"
	"go.uber.org/zap"
)

// NewSessionRuntime assembles the Factory Session runtime from flat service
// collaborators injected by the composition root.
func NewSessionRuntime(
	factoryRootDir string,
	clock factory.Clock,
	baseLogger *zap.Logger,
	logger *zap.Logger,
	runtimeBuild factory.ReplacementBuilder,
	startupBundle factory.HostedInstance,
	runtimeLifecycle factory.Lifecycle,
	runtimeSidecars RuntimeSidecars,
	durableExecution factorysessions.ExecutionService,
	dir string,
	executionBaseDir string,
	runtimeMode interfaces.RuntimeMode,
	backendScopeID string,
	workFile string,
	workflowID string,
	workstationLoader interfaces.WorkstationLoader,
	loadFactory interfaces.LoadedFactoryLoader,
	factoryScaffoldInitializer factorysessions.FactoryScaffoldInitializer,
	editableFactoryValidator factorysessions.EditableFactoryValidator,
	reconnectCursorValidator factorysessions.ReconnectCursorValidator,
	worldStateProjector factory.WorldStateProjector,
	invocationMetricsRecorder factorysessions.InvocationMetricsRecorder,
	newJavaScriptCheckpointStore factory.JavaScriptCheckpointStoreFactory,
	sessionResultProjection factory.SessionResultProjectionOperation,
	sessionState *sessionruntime.Service,
	sessionIDs factorysessions.SessionIDGenerator,
	resolveHome factorysessions.HomeDirectoryResolver,
	directoryInspection factorysessions.DirectoryInspection,
	namedPaths interfaces.NamedPathResolver,
	initialWorkFiles fileeffects.InitialWorkReader,
	identityService identity.Service,
) *SessionRuntime {
	if sessionState == nil || clock == nil || sessionIDs == nil || resolveHome == nil || directoryInspection == nil || namedPaths == nil || initialWorkFiles == nil || sessionResultProjection == nil || identityService == nil {
		return nil
	}
	host := &SessionRuntime{
		factoryRootDir: factoryRootDir, sessionState: sessionState,
		dir: dir, executionBaseDir: executionBaseDir,
		runtimeMode: runtimeMode, backendScopeID: backendScopeID,
		workFile: workFile, workflowID: workflowID,
		workstationLoader:          workstationLoader,
		loadFactory:                loadFactory,
		factoryScaffoldInitializer: factoryScaffoldInitializer,
		editableFactoryValidator:   editableFactoryValidator,
		reconnectCursorValidator:   reconnectCursorValidator,
		worldStateProjector:        worldStateProjector,
		invocationMetricsRecorder:  invocationMetricsRecorder,
		baseLogger:                 baseLogger, logger: logger, clock: clock,
		runtimeBuild: runtimeBuild, runtimeLifecycle: runtimeLifecycle, runtimeSidecars: runtimeSidecars,
		durableExecution:             durableExecution,
		newJavaScriptCheckpointStore: newJavaScriptCheckpointStore,
		sessionResultProjection:      sessionResultProjection,
		directoryInspection:          directoryInspection,
		sessionIDs:                   sessionIDs,
		resolveHome:                  resolveHome,
		namedPaths:                   namedPaths,
		initialWorkFiles:             initialWorkFiles,
		identity:                     identityService,
	}
	host.runtimeState.SetStartup(startupBundle)
	return host
}
