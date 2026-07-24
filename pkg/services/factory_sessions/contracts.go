package factorysessions

import (
	"context"
	"io"
	"io/fs"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/models"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

// InvocationMetric records one emitted runtime counter together with its
// low-cardinality dimensions.
type InvocationMetric struct {
	Name   string
	Labels map[string]string
}

// Effect-port contracts owned by the Sessions root. Nested internal/contracts
// aliases these symbols for private implementation packages.
type ExecutionOpeningFileSystem interface {
	Getwd() (string, error)
	Stat(string) (fs.FileInfo, error)
}

type DirectoryInspection interface {
	Stat(string) (fs.FileInfo, error)
	ReadDir(string) ([]fs.DirEntry, error)
}

type CursorPersistenceFileSystem interface {
	MkdirAll(string, fs.FileMode) error
	ReadFile(string) ([]byte, error)
	Remove(string) error
	Rename(string, string) error
}

type CursorPersistenceTemporaryFile interface {
	io.Writer
	Name() string
	Chmod(fs.FileMode) error
	Sync() error
	Close() error
}

type CursorPersistenceCreateTemporaryFile func(string, string) (CursorPersistenceTemporaryFile, error)

type RuntimePersistenceFileSystem interface {
	MkdirAll(string, fs.FileMode) error
	ReadFile(string) ([]byte, error)
	WriteFile(string, []byte, fs.FileMode) error
}

type InvocationMetricsRecorder interface {
	RecordInvocationMetric(InvocationMetric)
}

// InvocationTarget contains the detached configuration selected for one
// invocation. Operations remain consumer-owned interfaces.
type InvocationTarget struct {
	FactoryDir                       string
	FactorySourcePath                string
	RunnerID                         string
	OperatorDefaults                 operatorsettings.ResolvedDefaults
	ExecutionBaseDir                 string
	HomeDir                          string
	Logger                           *zap.Logger
	Verbose                          bool
	RecordPath                       string
	ReplayPath                       string
	RuntimeLogDir                    string
	RuntimeLogConfig                 factoryruntime.RuntimeLogStorageConfig
	RuntimeMetricsDir                string
	RuntimeMetricsConfig             factoryruntime.RuntimeMetricsStorageConfig
	ModelCacheDir                    string
	WorkflowID                       string
	MockWorkersConfig                *workers.MockWorkersConfig
	SkipPermissionsOverride          *bool
	SkipRunnerPrerequisiteValidation bool
	MetricsRecorder                  interface {
		RecordInvocationMetric(InvocationMetric)
	}
}

// FactoryInvocationOutcome is the detached result of one Factory invocation.
type FactoryInvocationOutcome struct {
	Result interfaces.FactoryInvocationResult
}

// FactoryEventConsumer receives ordered canonical events during one invocation.
type FactoryEventConsumer func([]interfaces.FactoryEvent)

// ApplicationOpeningPorts contains invocation-local observation edges.
type ApplicationOpeningPorts struct {
	InvocationMetricsRecorder interface {
		RecordInvocationMetric(InvocationMetric)
	}
	RuntimeHostObserver RuntimeHostObserver
}

// ApplicationOpeningRequest binds a runtime request to invocation-local ports.
type ApplicationOpeningRequest struct {
	Runtime *RuntimeOpeningRequest
	Ports   ApplicationOpeningPorts
}

// RuntimeHTTPServices is the detached set of opened runtime services consumed
// by the HTTP transport binding.
type RuntimeHTTPServices struct {
	FactoryRuntime     factoryruntime.Service
	FactoryDefinitions interfaces.Service
	WorkflowPreview    factoryruntime.WorkflowPreviewOperation
	FactorySessions    Service
	SessionInvocation  interface {
		InvokeFactorySession(context.Context, string, InvocationRequest) (interfaces.FactoryInvocationResult, error)
	}
	SessionExecution ExecutionService
	Work             work.Service
	Models           models.Service
	Workers          workers.Service
	ProviderSessions providersessions.Service
	WorkerPrompts    workers.PromptTemplates
	Logger           *zap.Logger
}

// FactoryScaffoldInitializer initializes a newly selected Factory directory.
type FactoryScaffoldInitializer func(string) error

// EditableFactoryValidator validates a detached Factory definition before
// persistence.
type EditableFactoryValidator func(
	context.Context,
	*interfaces.FactorySnapshot,
	interfaces.WorkstationLoader,
) error

// ReconnectCursorValidator validates an acknowledged cursor against a
// detached canonical event snapshot. Recordings supplies the implementation.
type ReconnectCursorValidator func(
	[]interfaces.FactoryEvent,
	interfaces.FactoryEventReconnectCursor,
	interfaces.FactoryEventReconnectScope,
) error
