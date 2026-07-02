package runtimehost

import (
	"time"

	factory_context "github.com/portpowered/infinite-you/pkg/factory/context"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	factoryservice "github.com/portpowered/infinite-you/pkg/factory/service"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/workers"
	"go.uber.org/zap"
)

// ModelEventRecorder records model execution events into the factory event stream.
type ModelEventRecorder = modelEventRecorder

// InferenceProgressPublisherFactory builds per-session inference progress publishers.
type InferenceProgressPublisherFactory = inferenceProgressPublisherFactory

// DispatchCompletionObserverFactory builds per-session dispatch completion observers.
type DispatchCompletionObserverFactory = dispatchCompletionObserverFactory

// LoadFactoryConfigForMode loads runtime config and optional replay metadata for service build wiring.
func LoadFactoryConfigForMode(cfg *Config) (*factoryconfig.LoadedFactoryConfig, *interfaces.ReplayArtifact, error) {
	return loadFactoryConfigForMode(cfg)
}

// WarnReplayMetadataMismatches logs replay metadata mismatches during service build wiring.
func WarnReplayMetadataMismatches(cfg *Config, artifact *interfaces.ReplayArtifact, logger *zap.Logger) {
	warnReplayMetadataMismatches(cfg, artifact, logger)
}

// RuntimeWorkflowContext builds workflow context for runtime bundle construction.
func RuntimeWorkflowContext(cfg *interfaces.FactoryConfig, sessionID string) *factory_context.FactoryContext {
	return runtimeWorkflowContext(cfg, sessionID)
}

// Model execution trace hooks used by pkg/service runtime bundle construction.
var (
	MarkModelExecutionResourceWaitStarted   = markModelExecutionResourceWaitStarted
	MarkModelExecutionResourceWaitFinished  = markModelExecutionResourceWaitFinished
	MarkModelExecutionLoadRequested         = markModelExecutionLoadRequested
	MarkModelExecutionLoadFinished          = markModelExecutionLoadFinished
	MarkModelExecutionLoadReused            = markModelExecutionLoadReused
)

// NewRecordingModelRunner wraps a runner with model execution event recording.
func NewRecordingModelRunner(
	inner workers.Runner,
	factoryCfg *interfaces.FactoryConfig,
	workerDef *interfaces.WorkerConfig,
	recorder ModelEventRecorder,
	now func() time.Time,
) workers.Runner {
	return newRecordingModelRunner(inner, factoryCfg, workerDef, recorder, now)
}

// LiveSessionBundle returns the runtime bundle attached to a live session handle.
func LiveSessionBundle(session *factorysessions.LiveSession) *factoryservice.Bundle {
	return liveSessionBundle(session)
}

// CoordinatorPolicyRunnerID returns the configured factory runner override.
func CoordinatorPolicyRunnerID(policy CoordinatorPolicy) string {
	return policy.runnerID
}

// CoordinatorPolicyVerbose reports whether verbose runtime diagnostics are enabled.
func CoordinatorPolicyVerbose(policy CoordinatorPolicy) bool {
	return policy.verbose
}

// CoordinatorPolicyProviderOverride returns the configured provider override.
func CoordinatorPolicyProviderOverride(policy CoordinatorPolicy) workers.Provider {
	return policy.providerOverride
}

// CoordinatorPolicyProviderCommandRunnerOverride returns the configured provider command runner override.
func CoordinatorPolicyProviderCommandRunnerOverride(policy CoordinatorPolicy) workers.CommandRunner {
	return policy.providerCommandRunnerOverride
}

// CoordinatorPolicyCommandRunnerOverride returns the configured script command runner override.
func CoordinatorPolicyCommandRunnerOverride(policy CoordinatorPolicy) workers.CommandRunner {
	return policy.commandRunnerOverride
}
