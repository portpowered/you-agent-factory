package service

import (
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/runtimepersist"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	responsestreamservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/response_stream"
	factorysessioncontracts "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire/contracts"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
)

// NewDurable constructs the runtime-backed durable execution capability while
// keeping persistence selection and concrete implementation choice private.
func NewDurable(
	projectRoot string,
	persistencePolicy factorysessions.PersistencePolicy,
	stores roles.RuntimePersistenceStoreFactory,
	childExecutorMode string,
	clock factoryruntime.Clock,
	syncWaits factorysessionexecution.SyncWaitScheduler,
	checkpointSummaries factoryruntime.JavaScriptCheckpointSummaries,
	workflows factoryruntime.JavaScriptWorkflows,
	orchestration factoryruntime.OrchestrationJavaScriptExecution,
	childValues factoryruntime.JavaScriptChildValues,
	workerPresetIDs map[string]struct{},
	workerSettings factoryruntime.JavaScriptWorkerSettings,
	recordingWriter recordings.PortableRecordingWriter,
	generateSessionID factorysessions.SessionIDGenerator,
	generateResponseEventID factorysessions.ResponseEventIDGenerator,
	responseStreams responsestreamservice.Service,
	liveChangeCoordinator factorysessioncontracts.LiveChangeCoordinator,
) (durableexecution.Service, error) {
	persistence, err := factorysessionexecution.PersistenceChoiceForPolicy(
		persistencePolicy,
		projectRoot,
		adaptRuntimePersistenceStoreFactory(stores),
	)
	if err != nil {
		return nil, err
	}
	// A runtime-backed live session invokes its children as Workers through its
	// own Factory Runtime, so it takes no direct provider edge of its own. The
	// mode still arrives from composition: a session with no provider behind it
	// runs fake children, exactly as before.
	return factorysessionexecution.NewProcessDurableExecutionService(
		projectRoot,
		childExecutorMode,
		nil,
		persistence,
		clock,
		syncWaits,
		checkpointSummaries,
		workflows,
		orchestration,
		childValues,
		workerPresetIDs,
		workerSettings,
		recordingWriter,
		generateSessionID,
		generateResponseEventID,
		responseStreams,
		liveChangeCoordinator,
		adaptRuntimePersistenceStoreFactory(stores),
		nil, nil, nil, nil, nil,
	)
}

func adaptRuntimePersistenceStoreFactory(
	stores roles.RuntimePersistenceStoreFactory,
) func(string) (runtimepersist.Store, error) {
	if stores == nil {
		return nil
	}
	return func(projectRoot string) (runtimepersist.Store, error) {
		return stores(projectRoot)
	}
}
