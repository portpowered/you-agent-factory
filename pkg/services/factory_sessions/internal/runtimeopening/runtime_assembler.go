package runtimeopening

import (
	"context"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/automations"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimeports"
	"github.com/portpowered/infinite-you/pkg/services/providers"
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
		string,
		factorydefinitions.WorkstationLoader,
		factoryruntime.LoadedFactoryLoader,
		providers.Service,
		platformprocess.CommandRunner,
		platformprocess.CommandRunner,
		*workers.MockWorkersConfig,
		factorydefinitions.RuntimeMode,
		factoryruntime.Scheduler,
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
		factoryruntime.WorkersMockCommandRunnerFactory,
		func(string) workers.ProgressPublisher,
		func(string) func(string),
		factoryruntime.PetriMutationRecorder,
		factoryruntime.WorldStateProjector,
		recordings.RuntimeOpening,
		factorydefinitions.InitialFactorySnapshotFactory,
		string,
		string,
		string,
		factorydefinitions.MutableLoadedFactorySource,
		string,
		*factorydefinitions.ReplayArtifact,
		*recordings.LoadResumeInputResult,
		*factorydefinitions.FactoryWorldState,
		[]factorydefinitions.FactoryEvent,
		automations.Service,
		bool,
	) (
		runtimeports.RuntimeReplacementBuilder,
		runtimeports.RuntimeInstance,
		factoryruntime.SessionBuildSpec,
		runtimeports.RuntimeLifecycle,
		runtimeports.RuntimeSidecarService,
		error,
	)
}
