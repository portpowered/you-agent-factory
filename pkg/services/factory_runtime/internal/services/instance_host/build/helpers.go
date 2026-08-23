package runtimebuild

import (
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

// MockCommandRunnerFactory decorates a command runner with the
// composition-selected mock-worker implementation.
type MockCommandRunnerFactory func(
	*workers.MockWorkersConfig,
	interfaces.RuntimeDefinitionLookup,
	platformprocess.CommandRunner,
) platformprocess.CommandRunner

// NewSessionLogger annotates runtime logs with session and directory metadata.
func NewSessionLogger(base *zap.Logger, sessionID string, folderPath string, factoryDir string) *zap.Logger {
	return factory.NewSessionLogger(base, sessionID, folderPath, factoryDir)
}

// WarnPortableBundledReplacementReport logs portable bundled file replacement targets.
func WarnPortableBundledReplacementReport(
	logger *zap.Logger,
	message string,
	replacements []interfaces.PortableBundledFileReplacement,
) {
	factory.WarnPortableBundledReplacementReport(logger, message, replacements)
}

func providerOverrideForMode(provider providers.Service, replayProvider providers.Service) providers.Service {
	if provider != nil || replayProvider == nil {
		return provider
	}
	return replayProvider
}

func commandRunnerOverrideForMode(
	mockWorkersConfig *workers.MockWorkersConfig,
	scriptCommandRunner platformprocess.CommandRunner,
	runtimeCfg interfaces.RuntimeConfigLookup,
	replayCommandRunner platformprocess.CommandRunner,
	newMockCommandRunner MockCommandRunnerFactory,
) platformprocess.CommandRunner {
	next := scriptCommandRunner
	if replayCommandRunner != nil && scriptCommandRunner == nil {
		next = replayCommandRunner
	}
	if mockWorkersConfig == nil || newMockCommandRunner == nil {
		return next
	}
	return newMockCommandRunner(mockWorkersConfig, runtimeCfg, next)
}

func providerCommandRunnerForMode(
	mockWorkersConfig *workers.MockWorkersConfig,
	providerCommandRunner platformprocess.CommandRunner,
	runtimeCfg interfaces.RuntimeConfigLookup,
	newMockCommandRunner MockCommandRunnerFactory,
) platformprocess.CommandRunner {
	if mockWorkersConfig == nil || newMockCommandRunner == nil {
		return nil
	}
	return newMockCommandRunner(mockWorkersConfig, runtimeCfg, providerCommandRunner)
}
