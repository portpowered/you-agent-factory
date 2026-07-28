package runtimeopening

import (
	"context"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/automations"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

// FactoryRuntimeAssembler is the session-owned runtime assembly operation
// constructed once by Wire. Assemble receives only invocation/session values;
// its product-policy dependencies are already bound.
type FactoryRuntimeAssembler interface {
	Assemble(
		context.Context,
		string,
		string,
		bool,
		string,
		string,
		string,
		factorydefinitions.WorkstationLoader,
		factoryruntime.LoadedFactoryLoader,
		workers.Provider,
		workers.CommandRunner,
		workers.CommandRunner,
		*workers.MockWorkersConfig,
		factorydefinitions.RuntimeMode,
		factoryruntime.Scheduler,
		map[string]workers.WorkerExecutor,
		func(string, workers.WorkerExecutor) workers.WorkerExecutor,
		bool,
		recordings.SubmissionRecorder,
		recordings.DispatchRecorder,
		string,
		factoryruntime.RuntimeLogStorageConfig,
		factoryruntime.RuntimeFileLoggingPolicy,
		factoryruntime.RuntimeMetricsPolicy,
		string,
		factoryruntime.RuntimeMetricsStorageConfig,
		time.Duration,
		string,
		string,
		bool,
		bool,
		*bool,
		factoryruntime.Clock,
		*zap.Logger,
		workers.RuntimeService,
		factoryruntime.WorkersRuntimeExecutorsFactory,
		factoryruntime.WorkersMockCommandRunnerFactory,
		func(string) workers.ProgressPublisher,
		func(string) func(string),
		factoryruntime.PetriMutationRecorder,
		factoryruntime.WorldStateProjector,
		factoryruntime.RuntimeLedgerFactory,
		recordings.RuntimeRecorderFactory,
		factorydefinitions.InitialFactorySnapshotFactory,
		string,
		string,
		string,
		factorydefinitions.MutableLoadedFactorySource,
		string,
		*factorydefinitions.ReplayArtifact,
		recordings.ReplayExecutionFactory,
		automations.Service,
		bool,
	) (
		factoryruntime.ReplacementBuilder,
		factoryruntime.HostedInstance,
		factoryruntime.SessionBuildSpec,
		factoryruntime.Lifecycle,
		factoryruntime.Sidecars,
		error,
	)
}
