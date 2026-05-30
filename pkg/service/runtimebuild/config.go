package runtimebuild

import (
	"time"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/localmodels"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/workers"
)

// Config carries service-level settings required to build runnable runtimes.
type Config struct {
	ExecutionBaseDir                        string
	RunnerID                                string
	RuntimeMode                             interfaces.RuntimeMode
	Verbose                                 bool
	RuntimeInstanceID                       string
	RuntimeLogDir                           string
	RuntimeLogConfig                        logging.RuntimeLogConfig
	RecordPath                              string
	WorkflowID                              string
	MockWorkersConfig                       *factoryconfig.MockWorkersConfig
	RecordFlushInterval                     time.Duration
	ModelCacheDir                           string
	SkipBuiltInRunnerPrerequisiteValidation bool
	WorkstationLoader                       factoryconfig.WorkstationLoader
	ProviderOverride                        workers.Provider
	ProviderCommandRunnerOverride           workers.CommandRunner
	CommandRunnerOverride                   workers.CommandRunner
	LocalModelRuntimeOverride               localmodels.Runtime
	ExtraOptions                            []factory.FactoryOption
}
