package factory

import (
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerprovider "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
)

// WorkersRuntimeExecutorsFactory is the Factory Runtime construction port for
// obtaining configured Worker dispatch executors. Wire supplies the Workers
// implementation.
type WorkersRuntimeExecutorsFactory func(
	workers.RuntimeService,
	factorydefinitions.RuntimeConfigLookup,
	*factorydefinitions.FactoryConfig,
	string,
	*workers.Context,
	logging.Logger,
	bool,
	*bool,
	workerprovider.Provider,
	workers.ProgressPublisher,
	workers.ScriptEventRecorder,
	workers.InferenceEventRecorder,
	workers.ModelEventRecorder,
	workers.AgentRunEventRecorder,
	func() time.Time,
) (map[string]workers.WorkerExecutor, error)

// WorkersMockCommandRunnerFactory is the Factory Runtime construction port for
// applying the Workers-owned mock command policy selected by Wire.
type WorkersMockCommandRunnerFactory func(
	*workers.MockWorkersConfig,
	factorydefinitions.RuntimeDefinitionLookup,
	workers.CommandRunner,
) workers.CommandRunner
