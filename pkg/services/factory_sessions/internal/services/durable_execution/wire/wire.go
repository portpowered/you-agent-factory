// Package wire constructs the owner-private durable execution capability.
package wire

import (
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/runtimepersist"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	responsestreamservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/response_stream"
	factorysessioncontracts "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire/contracts"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"go.uber.org/zap"
)

// NewRuntimeBacked constructs one inert runtime-backed durable owner. Store
// selection happens only when an explicit opening acquires its resources.
func NewRuntimeBacked(
	stores roles.RuntimePersistenceStoreFactory,
	clock factoryruntime.Clock,
	syncWaits factorysessionexecution.SyncWaitScheduler,
	checkpointSummaries factoryruntime.JavaScriptCheckpointSummaries,
	workflows factoryruntime.JavaScriptWorkflows,
	orchestration factoryruntime.OrchestrationJavaScriptExecution,
	childValues factoryruntime.JavaScriptChildValues,
	recordingWriter recordings.PortableRecordingWriter,
	generateSessionID factorysessions.SessionIDGenerator,
	generateResponseEventID factorysessions.ResponseEventIDGenerator,
	responseStreams responsestreamservice.Service,
	liveChangeCoordinator factorysessioncontracts.LiveChangeCoordinator,
	workerExecution factorysessionexecution.WorkerExecution,
	logger *zap.Logger,
) (*factorysessionexecution.JavaScriptRuntimeService, error) {
	persistence := factorysessionexecution.NewScopePersistence(func(root string) (runtimepersist.Store, error) { return stores(root) })
	return factorysessionexecution.NewProcessDurableRuntime(
		"", factorysessions.ChildExecutorModeFake, persistence, clock, syncWaits,
		checkpointSummaries, workflows, orchestration, childValues,
		nil, factoryruntime.JavaScriptWorkerSettings{}, recordingWriter,
		generateSessionID, generateResponseEventID, responseStreams, liveChangeCoordinator,
		nil, nil, nil, workerExecution, nil, logger,
	), nil
}
