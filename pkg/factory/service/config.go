package service

import (
	"strings"
	"time"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	modelhost "github.com/portpowered/infinite-you/pkg/models/host"
	localmodels "github.com/portpowered/infinite-you/pkg/models/local"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformmetrics "github.com/portpowered/infinite-you/pkg/platform/metrics"
	"github.com/portpowered/infinite-you/pkg/workers"
	"go.uber.org/zap"
)

// RuntimeFileLoggingPolicy controls whether bundle construction creates a runtime file sink.
type RuntimeFileLoggingPolicy string

const (
	RuntimeFileLoggingPolicyEnabled  RuntimeFileLoggingPolicy = "enabled"
	RuntimeFileLoggingPolicyDisabled RuntimeFileLoggingPolicy = "disabled"
)

// RuntimeMetricsPolicy controls whether bundle construction creates a runtime metrics sink.
type RuntimeMetricsPolicy string

const (
	RuntimeMetricsPolicyEnabled  RuntimeMetricsPolicy = "enabled"
	RuntimeMetricsPolicyDisabled RuntimeMetricsPolicy = "disabled"
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
	RuntimeMetricsPolicy                    RuntimeMetricsPolicy
	RuntimeMetricsDir                       string
	RuntimeMetricsConfig                    platformmetrics.RuntimeMetricsConfig
	RecordPath                              string
	WorkflowID                              string
	MockWorkersConfig                       *factoryconfig.MockWorkersConfig
	RecordFlushInterval                     time.Duration
	ModelCacheDir                           string
	SkipBuiltInRunnerPrerequisiteValidation bool
	WorkstationLoader                       factoryconfig.WorkstationLoader
	ProviderOverride                        workers.Provider
	LocalModelRuntimeOverride               localmodels.Runtime
	LocalModelHooks                         localmodels.Hooks
	ModelAssetsOverride                     localmodels.AssetPuller
	ModelHostOverride                       modelhost.Host
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

type zapModelHostLogger struct {
	logger *zap.Logger
}

func newZapModelHostLogger(logger *zap.Logger) modelhost.Logger {
	if logger == nil {
		return nil
	}
	return zapModelHostLogger{logger: logger}
}

func (l zapModelHostLogger) Info(msg string, fields map[string]string) {
	l.logger.Info(msg, modelHostZapFields(fields)...)
}

func (l zapModelHostLogger) Warn(msg string, fields map[string]string) {
	l.logger.Warn(msg, modelHostZapFields(fields)...)
}

func modelHostZapFields(fields map[string]string) []zap.Field {
	if len(fields) == 0 {
		return nil
	}
	out := make([]zap.Field, 0, len(fields))
	for key, value := range fields {
		out = append(out, zap.String(key, value))
	}
	return out
}

type invocationMetricsRecorderAdapter struct {
	recorder InvocationMetricsRecorder
}

func newModelHostMetricsRecorder(recorder InvocationMetricsRecorder) modelhost.MetricsRecorder {
	if recorder == nil {
		return nil
	}
	return invocationMetricsRecorderAdapter{recorder: recorder}
}

func (a invocationMetricsRecorderAdapter) RecordMetric(name string, labels map[string]string) {
	if a.recorder == nil || strings.TrimSpace(name) == "" {
		return
	}
	a.recorder.RecordInvocationMetric(InvocationMetric{
		Name:   name,
		Labels: cloneMetricLabels(labels),
	})
}

func cloneMetricLabels(labels map[string]string) map[string]string {
	if len(labels) == 0 {
		return nil
	}
	out := make(map[string]string, len(labels))
	for key, value := range labels {
		out[key] = value
	}
	return out
}

// HostConfigInput carries service-level runtime host settings for bundle construction.
type HostConfigInput struct {
	RunnerID                                string
	RuntimeMode                             interfaces.RuntimeMode
	Verbose                                 bool
	RuntimeInstanceID                       string
	RuntimeLogDir                           string
	RuntimeLogConfig                        logging.RuntimeLogConfig
	RuntimeFileLoggingPolicy                RuntimeFileLoggingPolicy
	RuntimeMetricsPolicy                    RuntimeMetricsPolicy
	RuntimeMetricsDir                       string
	RuntimeMetricsConfig                    platformmetrics.RuntimeMetricsConfig
	RecordPath                              string
	WorkflowID                              string
	MockWorkersConfig                       *factoryconfig.MockWorkersConfig
	RecordFlushInterval                     time.Duration
	ModelCacheDir                           string
	SkipBuiltInRunnerPrerequisiteValidation bool
	WorkstationLoader                       factoryconfig.WorkstationLoader
	ProviderOverride                        workers.Provider
	LocalModelRuntimeOverride               localmodels.Runtime
	LocalModelHooks                         localmodels.Hooks
	ModelAssetsOverride                     localmodels.AssetPuller
	ModelHostOverride                       modelhost.Host
	ExtraOptions                            []factory.FactoryOption
	InvocationMetricsRecorder               InvocationMetricsRecorder
	Logger                                  *zap.Logger
}

// ConfigFromHostInput maps caller-provided host settings into bundle-build config.
func ConfigFromHostInput(input HostConfigInput) Config {
	return Config{
		RunnerID:                                input.RunnerID,
		RuntimeMode:                             input.RuntimeMode,
		Verbose:                                 input.Verbose,
		RuntimeInstanceID:                       input.RuntimeInstanceID,
		RuntimeLogDir:                           input.RuntimeLogDir,
		RuntimeLogConfig:                        input.RuntimeLogConfig,
		RuntimeFileLoggingPolicy:                input.RuntimeFileLoggingPolicy,
		RuntimeMetricsPolicy:                    input.RuntimeMetricsPolicy,
		RuntimeMetricsDir:                       input.RuntimeMetricsDir,
		RuntimeMetricsConfig:                    input.RuntimeMetricsConfig,
		RecordPath:                              input.RecordPath,
		WorkflowID:                              input.WorkflowID,
		MockWorkersConfig:                       input.MockWorkersConfig,
		RecordFlushInterval:                     input.RecordFlushInterval,
		ModelCacheDir:                           input.ModelCacheDir,
		SkipBuiltInRunnerPrerequisiteValidation: input.SkipBuiltInRunnerPrerequisiteValidation,
		WorkstationLoader:                       input.WorkstationLoader,
		ProviderOverride:                        input.ProviderOverride,
		LocalModelRuntimeOverride:               input.LocalModelRuntimeOverride,
		LocalModelHooks:                         input.LocalModelHooks,
		ModelAssetsOverride:                     input.ModelAssetsOverride,
		ModelHostOverride:                       input.ModelHostOverride,
		ExtraOptions:                            input.ExtraOptions,
		InvocationMetricsRecorder:               input.InvocationMetricsRecorder,
		Logger:                                  input.Logger,
	}
}
