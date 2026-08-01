package factory

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

// LoadedConfig is the Factory Runtime view of an effective loaded definition.
// Config loaders satisfy it structurally; the runtime service root does not
// expose their concrete implementation.
type LoadedConfig interface {
	factorydefinitions.RuntimeDefinitionLookup
	FactoryDir() string
	RuntimeBaseDir() string
	FactoryConfig() *factorydefinitions.FactoryConfig
}

// LoadedFactoryLoader supplies one mutable effective Factory Definition to the
// runtime construction boundary.
type LoadedFactoryLoader = factorydefinitions.LoadedFactoryLoader

// SessionBuildSpec is the immutable input contract for constructing one
// session-owned runtime bundle.
type SessionBuildSpec struct {
	Dir                   string
	FolderPath            string
	SessionID             string
	ExecutionBaseDir      string
	LoadedFactoryCfg      LoadedConfig
	BaseLogger            *zap.Logger
	RuntimeInstanceID     string
	Clock                 Clock
	RecordPath            string
	WorkflowID            string
	ProviderOverride      workers.Runner
	ProviderCommandRunner workers.CommandRunner
	CommandRunnerOverride workers.CommandRunner
	SubmissionHooks       []SubmissionHook
	CompletionPlanner     CompletionDeliveryPlanner
	PetriMutationRecorder PetriMutationRecorder
}
