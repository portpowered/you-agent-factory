package runtimebuild

import (
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factory/replay"
	"github.com/portpowered/infinite-you/pkg/workers"
	workerapplication "github.com/portpowered/infinite-you/pkg/workers/application"
	"go.uber.org/zap"
)

// SessionSpecInput is the shared constructor input for deriving one immutable
// session build spec from service-level runtime policy plus session-owned state.
type SessionSpecInput struct {
	Dir                                    string
	FolderPath                             string
	SessionID                              string
	ExecutionBaseDir                       string
	LoadedFactoryCfg                       *factoryconfig.LoadedFactoryConfig
	RuntimeInstanceID                      string
	SideEffects                            *replay.SideEffects
	AdditionalFactoryOpts                  []factory.FactoryOption
	PreserveCompatibilityDefaultRecordPath bool
}

// SessionBuildSpec is the immutable input contract for constructing one
// session-owned runtime bundle.
type SessionBuildSpec struct {
	Dir                   string
	FolderPath            string
	SessionID             string
	ExecutionBaseDir      string
	LoadedFactoryCfg      *factoryconfig.LoadedFactoryConfig
	BaseLogger            *zap.Logger
	RuntimeInstanceID     string
	Clock                 factory.Clock
	RecordPath            string
	WorkflowID            string
	ProviderOverride      workers.Provider
	ProviderCommandRunner workers.CommandRunner
	CommandRunnerOverride workers.CommandRunner
	WorkerApplication     workerapplication.Components
	AdditionalFactoryOpts []factory.FactoryOption
}
