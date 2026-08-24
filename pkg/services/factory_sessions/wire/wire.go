// Package wire is the Factory Sessions service composition boundary.
//
// Wire performs construction only, returns the singular factorysessions.Service
// root interface, and starts no lifecycle components. Parent-private identity
// and response-stream owner wiring stays inside the owner service assembly path;
// peers depend on Service rather than owner internals or construction ports.
package wire

import (
	"fmt"

	events "github.com/portpowered/infinite-you/pkg/services/events"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/fileeffects"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livechange"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/requestpreparation"
	factorysessionroot "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/service"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	durableexecutionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution/wire"
	identitywire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/identity/wire"
	responsestreamwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/response_stream/wire"
	sessionprojection "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionprojection"
	factorysessioncontracts "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire/contracts"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
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

// NewLiveChangeCoordinator constructs the one process-scoped admission
// coordinator shared by live and durable Factory Session execution.
func NewLiveChangeCoordinator() factorysessioncontracts.LiveChangeCoordinator {
	return livechange.NewCoordinator()
}

// NewService constructs an inert Factory Sessions root from construction and
// process-edge ports. It composes the accepted root through parent-private
// identity and response-stream owner construction without publishing owner types
// on the returned peer surface. Missing required construction ports fail with a
// deterministic construction error and a nil service.
func NewService(
	newJavaScriptCheckpointStore factoryruntime.JavaScriptCheckpointStoreFactory,
	sessionResultProjection factoryruntime.SessionResultProjectionOperation,
	interpolation factorydefinitions.InvocationInterpolationService,
	invocationWorkTypes factorydefinitions.InvocationWorkTypeService,
	ttsObservability factorydefinitions.TTSObservabilityService,
	eventIDs factorysessions.ResponseEventIDGenerator,
	responseEventRetentionLimits *factorysessions.ResponseEventRetentionLimits,
	sessionIDs factorysessions.SessionIDGenerator,
	resolveHome factorysessions.HomeDirectoryResolver,
	directoryInspection DirectoryInspection,
	namedPaths factorydefinitions.NamedPathResolver,
	invocationInputFiles fileeffects.InvocationInputReader,
	initialWorkFiles fileeffects.InitialWorkReader,
	resolveSymlinks factorysessions.LogicalTargetResolveSymlinks,
	eventsService events.Service,
	clock factoryruntime.Clock,
	liveChangeCoordinator factorysessioncontracts.LiveChangeCoordinator,
	recordedSessionInventory recordings.RecordedSessionInventory,
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
	if clock == nil {
		return nil, fmt.Errorf("construct Factory Sessions: clock is required")
	}
	if liveChangeCoordinator == nil {
		return nil, fmt.Errorf("construct Factory Sessions: live-change coordinator is required")
	}
	identityService, err := identitywire.NewService(resolveSymlinks, resolveHome, directoryInspection)
	if err != nil {
		return nil, err
	}
	responseStreams, err := responsestreamwire.NewService(eventIDs, responseEventRetentionLimits, eventsService)
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
		clock,
		liveChangeCoordinator,
		recordedSessionInventory,
	)
	if err != nil {
		return nil, err
	}
	if service == nil {
		return nil, fmt.Errorf("construct Factory Sessions: implementation rejected its dependencies")
	}
	return service, nil
}

// NewDetachedOperations binds the build-first Sessions operation view to the
// already-composed root. It performs no child construction or lifecycle work.

type detachedOperationsProvider interface {
	DetachedOperations() factorysessions.DetachedService
}

func NewDetachedOperations(owner factorysessions.Service) (factorysessions.DetachedService, error) {
	if provider, ok := owner.(detachedOperationsProvider); ok {
		if operations := provider.DetachedOperations(); operations != nil {
			return operations, nil
		}
	}
	return (&factorysessions.DetachedOperations{}).Bind(owner)
}

// TODO(btrc-p4-sessions-lifecycle-003): remove after application Wire callers
// use the root-owned detached capability.
// NewDurableExecution constructs the configured durable execution capability
// without exposing its implementation package to application Wire.
func NewDurableExecution(
	projectRoot string,
	persistencePolicy factorysessions.PersistencePolicy,
	stores RuntimePersistenceStoreFactory,
	childExecutorMode string,
	clock factoryruntime.Clock,
	syncWaits factorysessionexecution.SyncWaitScheduler,
	checkpointSummaries factoryruntime.JavaScriptCheckpointSummaries,
	workflows factoryruntime.JavaScriptWorkflows,
	orchestration factoryruntime.OrchestrationJavaScriptExecution,
	workerPresetIDs map[string]struct{},
	workerSettings factoryruntime.JavaScriptWorkerSettings,
	recordingWriter recordings.PortableRecordingWriter,
	generateSessionID factorysessions.SessionIDGenerator,
	generateResponseEventID factorysessions.ResponseEventIDGenerator,
	responseEventRetentionLimits *factorysessions.ResponseEventRetentionLimits,
	eventsService events.Service,
	liveChangeCoordinator factorysessioncontracts.LiveChangeCoordinator,
) (durableexecution.Service, error) {
	responseStreams, err := responsestreamwire.NewService(generateResponseEventID, responseEventRetentionLimits, eventsService)
	if err != nil {
		return nil, err
	}
	return durableexecutionwire.NewDurable(
		projectRoot, persistencePolicy, stores, childExecutorMode, clock, syncWaits,
		checkpointSummaries, workflows, orchestration, workflows,
		workerPresetIDs, workerSettings,
		recordingWriter, generateSessionID, generateResponseEventID, responseStreams,
		liveChangeCoordinator,
	)
}

// TODO(btrc-p4-sessions-lifecycle-003): remove after application Wire callers
// use the root-owned detached capability.
// NewStandaloneExecution constructs the configured standalone execution
// capability without exposing its implementation package to application Wire.
func NewStandaloneExecution(
	provider factorysessions.ExecutionProvider,
	projectRoot string,
	stores RuntimePersistenceStoreFactory,
	fixtureCatalogPath string,
	childExecutorMode string,
	execution factorysessionexecution.WorkerExecution,
	clock factoryruntime.Clock,
	syncWaits factorysessionexecution.SyncWaitScheduler,
	checkpointSummaries factoryruntime.JavaScriptCheckpointSummaries,
	workflows factoryruntime.JavaScriptWorkflows,
	orchestration factoryruntime.OrchestrationJavaScriptExecution,
	recordingWriter recordings.PortableRecordingWriter,
	generateSessionID factorysessions.SessionIDGenerator,
	fixtureFiles fileeffects.ContractFixtureReader,
	liveChangeCoordinator factorysessioncontracts.LiveChangeCoordinator,
) (durableexecution.Service, error) {
	return durableexecutionwire.NewStandalone(
		provider, projectRoot, stores, fixtureCatalogPath, childExecutorMode,
		execution,
		clock, syncWaits, checkpointSummaries, workflows, orchestration, workflows,
		recordingWriter, generateSessionID, fixtureFiles, liveChangeCoordinator,
	)
}
