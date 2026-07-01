package service

import (
	"time"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/localmodels"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/modelhost"
	"github.com/portpowered/infinite-you/pkg/workers"
	"go.uber.org/zap"
)

// RuntimeFileLoggingPolicy controls whether bundle construction creates a runtime file sink.
type RuntimeFileLoggingPolicy string

const (
	RuntimeFileLoggingPolicyEnabled  RuntimeFileLoggingPolicy = "enabled"
	RuntimeFileLoggingPolicyDisabled RuntimeFileLoggingPolicy = "disabled"
)

// Config carries host-level settings required to build one runnable runtime bundle.
type Config struct {
	RunnerID                                string
	RuntimeMode                             interfaces.RuntimeMode
	Verbose                                 bool
	RuntimeInstanceID                       string
	RuntimeLogDir                           string
	RuntimeLogConfig                        logging.RuntimeLogConfig
	RuntimeFileLoggingPolicy                RuntimeFileLoggingPolicy
	RuntimeMetricsDir                       string
	RuntimeMetricsConfig                    logging.RuntimeMetricsConfig
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
	LocalModelHooks                         localmodels.Hooks
	ExtraOptions                            []factory.FactoryOption
	InvocationMetricsRecorder               InvocationMetricsRecorder
	Logger                                  *zap.Logger
}

// InvocationMetricsRecorder receives invocation boundary counter emissions.
type InvocationMetricsRecorder interface {
	RecordInvocationMetric(metric InvocationMetric)
}

// InvocationMetric is one invocation boundary counter emission.
type InvocationMetric struct {
	Name   string
	Labels map[string]string
}

// ModelHostDiagnostics configures model-host logging and metrics during bundle build.
func ModelHostDiagnostics(cfg Config) modelhost.Diagnostics {
	diagnostics := modelhost.Diagnostics{}
	if cfg.Logger != nil {
		diagnostics.Logger = newZapModelHostLogger(cfg.Logger.Named("modelhost"))
	}
	if cfg.InvocationMetricsRecorder != nil {
		diagnostics.Metrics = newModelHostMetricsRecorder(cfg.InvocationMetricsRecorder)
	}
	return diagnostics
}
