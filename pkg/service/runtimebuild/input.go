package runtimebuild

import (
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/workers"
	"go.uber.org/zap"
)

// BuildInput is the canonical input for constructing one runnable runtime bundle.
type BuildInput struct {
	Dir                   string
	FolderPath            string
	SessionID             string
	LoadedFactoryCfg      *factoryconfig.LoadedFactoryConfig
	BaseLogger            *zap.Logger
	RuntimeInstanceID     string
	Clock                 factory.Clock
	RecordPath            string
	WorkflowID            string
	ProviderOverride      workers.Provider
	ProviderCommandRunner workers.CommandRunner
	CommandRunnerOverride workers.CommandRunner
	AdditionalFactoryOpts []factory.FactoryOption
}
