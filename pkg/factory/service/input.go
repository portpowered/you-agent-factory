package service

import (
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
	factoryevents "github.com/portpowered/infinite-you/pkg/factory/events"
	"github.com/portpowered/infinite-you/pkg/workers"
	workerprovider "github.com/portpowered/infinite-you/pkg/workers/provider"
	"go.uber.org/zap"
)

const defaultSessionID = "~default"

// BuildInput is the immutable input for constructing one hosted runtime bundle.
type BuildInput struct {
	Dir                           string
	FolderPath                    string
	SessionID                     string
	Config                        Config
	LoadedFactoryCfg              *factoryconfig.LoadedFactoryConfig
	BaseLogger                    *zap.Logger
	RuntimeInstanceID             string
	Clock                         factory.Clock
	RecordPath                    string
	WorkflowID                    string
	ProviderOverride              workers.Provider
	ProviderCommandRunner         workers.CommandRunner
	CommandRunnerOverride         workers.CommandRunner
	AdditionalFactoryOpts         []factory.FactoryOption
	LoadWorkerOpts                func(*factoryevents.FactoryEventHistory) ([]factory.FactoryOption, error)
	PrefetchedLocalModels         LocalModelDomain
	InferenceProgressPublisher    workerprovider.InferenceProgressPublisher
	InferenceProgressPublisherSet bool
	DispatchCompleted             func(string)
}
