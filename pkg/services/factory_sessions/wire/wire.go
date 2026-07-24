// Package wire is the Factory Sessions service composition boundary.
// Application Wire uses these providers without importing the service's
// concrete implementation package.
package wire

import (
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/fileeffects"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/requestpreparation"
	factorysessionroot "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/service"
	durableexecutionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution/wire"
	identity "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/identity"
	identitywire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/identity/wire"
	responsestreamwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/response_stream/wire"
	sessionprojection "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionprojection"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// NewRequestPreparation constructs the private request-normalization
// implementation for injection into Factory Sessions-owned transports.
func NewRequestPreparation() RequestPreparation {
	return requestpreparation.New()
}

// NewWorkStopSummaryProjector constructs the owner-private stopped-state
// projection policy for injection into Work-owned read adapters.
func NewWorkStopSummaryProjector() factorysessions.WorkStopSummaryProjector {
	return func(request factorysessions.WorkStopSummaryRequest) *factorysessions.StopSummary {
		return sessionprojection.ProjectWorkStopSummary(
			request.SessionID,
			request.Snapshot,
			request.Token,
			request.SessionStopSummary,
		)
	}
}

// NewService constructs the inert, process-scoped Factory Sessions service.
// Runtime-specific values are supplied later through Service.ForRuntime.
func NewService(
	newJavaScriptCheckpointStore factoryruntime.JavaScriptCheckpointStoreFactory,
	sessionResultProjection factoryruntime.SessionResultProjectionOperation,
	interpolation factorydefinitions.InvocationInterpolationService,
	invocationWorkTypes factorydefinitions.InvocationWorkTypeService,
	ttsObservability factorydefinitions.TTSObservabilityService,
	eventIDs factorysessions.ResponseEventIDGenerator,
	sessionIDs factorysessions.SessionIDGenerator,
	resolveHome factorysessions.HomeDirectoryResolver,
	directoryInspection DirectoryInspection,
	namedPaths factorydefinitions.NamedPathResolver,
	invocationInputFiles fileeffects.InvocationInputReader,
	initialWorkFiles fileeffects.InitialWorkReader,
	resolveSymlinks factorysessions.LogicalTargetResolveSymlinks,
) (factorysessions.Service, error) {
	if sessionResultProjection == nil {
		return nil, fmt.Errorf("construct Factory Sessions: session result projection is required")
	}
	if eventIDs == nil {
		return nil, fmt.Errorf("construct Factory Sessions: response event ID generator is required")
	}
	if sessionIDs == nil {
		return nil, fmt.Errorf("construct Factory Sessions: session ID generator is required")
	}
	if resolveHome == nil {
		return nil, fmt.Errorf("construct Factory Sessions: home directory resolver is required")
	}
	if directoryInspection == nil {
		return nil, fmt.Errorf("construct Factory Sessions: directory inspection is required")
	}
	if namedPaths == nil {
		return nil, fmt.Errorf("construct Factory Sessions: named path resolver is required")
	}
	if invocationInputFiles == nil {
		return nil, fmt.Errorf("construct Factory Sessions: invocation input reader is required")
	}
	if initialWorkFiles == nil {
		return nil, fmt.Errorf("construct Factory Sessions: initial Work reader is required")
	}
	identityService, err := identitywire.NewService(identity.Dependencies{
		ResolveSymlinks: resolveSymlinks,
		ResolveHome:     resolveHome,
		Directories:     directoryInspection,
	})
	if err != nil {
		return nil, err
	}
	responseStreams, err := responsestreamwire.NewService(eventIDs)
	if err != nil {
		return nil, err
	}

	service, err := factorysessionroot.NewRoot(
		newJavaScriptCheckpointStore,
		sessionResultProjection,
		interpolation,
		invocationWorkTypes,
		ttsObservability,
		eventIDs,
		sessionIDs,
		resolveHome,
		directoryInspection,
		namedPaths,
		invocationInputFiles,
		initialWorkFiles,
		identityService,
		responseStreams,
	)
	if err != nil {
		return nil, err
	}
	if service == nil {
		return nil, fmt.Errorf("construct Factory Sessions: implementation rejected its dependencies")
	}
	return service, nil
}

// NewDurableExecution constructs the configured durable execution capability
// without exposing its implementation package to application Wire.
func NewDurableExecution(
	projectRoot string,
	persistencePolicy factorysessions.PersistencePolicy,
	stores RuntimePersistenceStoreFactory,
	executor workers.InvocationExecutor,
	clock factoryruntime.Clock,
	syncWaits factorysessionexecution.SyncWaitScheduler,
	checkpointSummaries factoryruntime.JavaScriptCheckpointSummaries,
	workflows factoryruntime.JavaScriptWorkflows,
	workerPresetIDs map[string]struct{},
	workerSettings factoryruntime.JavaScriptWorkerSettings,
	recordingWriter recordings.PortableRecordingWriter,
	generateSessionID factorysessions.SessionIDGenerator,
) (factorysessions.ExecutionService, error) {
	return durableexecutionwire.NewDurable(
		projectRoot, persistencePolicy, stores, executor, clock, syncWaits,
		checkpointSummaries, workflows, workerPresetIDs, workerSettings,
		recordingWriter, generateSessionID,
	)
}

// NewStandaloneExecution constructs the configured standalone execution
// capability without exposing its implementation package to application Wire.
func NewStandaloneExecution(
	provider factorysessions.ExecutionProvider,
	projectRoot string,
	stores RuntimePersistenceStoreFactory,
	fixtureCatalogPath string,
	childExecutorMode string,
	executor workers.InvocationExecutor,
	clock factoryruntime.Clock,
	syncWaits factorysessionexecution.SyncWaitScheduler,
	checkpointSummaries factoryruntime.JavaScriptCheckpointSummaries,
	workflows factoryruntime.JavaScriptWorkflows,
	recordingWriter recordings.PortableRecordingWriter,
	generateSessionID factorysessions.SessionIDGenerator,
	fixtureFiles fileeffects.ContractFixtureReader,
) (factorysessions.ExecutionService, error) {
	return durableexecutionwire.NewStandalone(
		provider, projectRoot, stores, fixtureCatalogPath, childExecutorMode,
		executor, clock, syncWaits, checkpointSummaries, workflows,
		recordingWriter, generateSessionID, fixtureFiles,
	)
}
