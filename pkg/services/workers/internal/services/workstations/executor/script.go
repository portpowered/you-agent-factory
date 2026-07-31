package executor

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	workerconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/process"
	runnerwire "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/wire"
)

// ScriptExecutor adapts the private runners.Service.Execute result onto the
// workstation result boundary. Production construction holds the immutable
// script registry; remaining configuration fields support package-local legacy
// tests while that compatibility surface is retired.
type ScriptExecutor struct {
	Command       string
	Args          []string
	FactoryDir    string
	CommandRunner CommandRunner
	Logger        logging.Logger
	recorder      workers.ScriptEventRecorder
	Now           func() time.Time
	FactoryDocs   workers.FactoryDocsLoader
	Publish       workers.ProgressPublisher

	registry runners.Service
}

// ScriptEventRecorder receives worker-owned script-boundary facts.
type ScriptEventRecorder = workers.ScriptEventRecorder

// ScriptFactory owns the exact inert effects used to construct configured
// registry-backed Script Runners.
type ScriptFactory struct {
	commandRunner CommandRunner
	commandClock  workerprocess.Clock
	factoryDocs   workers.FactoryDocsLoader
}

// NewScriptFactory validates the command edge selected by process composition.
func NewScriptFactory(
	runner CommandRunner,
	commandClock workerprocess.Clock,
	factoryDocs workers.FactoryDocsLoader,
) (*ScriptFactory, error) {
	if runner == nil {
		return nil, errors.New("construct script worker factory: command runner is required")
	}
	if commandClock == nil {
		return nil, errors.New("construct script worker factory: command clock is required")
	}
	if factoryDocs == nil {
		return nil, errors.New("construct script worker factory: Factory docs loader is required")
	}
	return &ScriptFactory{
		commandRunner: runner,
		commandClock:  commandClock,
		factoryDocs:   factoryDocs,
	}, nil
}

// New constructs one configured Script Runner and resolves it through the
// private immutable registry before exposing the workstation adapter.
func (f *ScriptFactory) New(
	definition *workerconfig.FactoryWorkerConfig,
	logger logging.Logger,
	factoryDirectory string,
	publish workers.ProgressPublisher,
	record workers.ScriptEventRecorder,
	now func() time.Time,
) (*ScriptExecutor, error) {
	if f == nil {
		return nil, errors.New("construct script worker: factory is required")
	}
	commandRunner := workers.CommandRunnerWithLogging(
		f.commandRunner,
		logger,
		f.commandClock,
	)
	return newScriptExecutor(
		definition,
		commandRunner,
		logger,
		factoryDirectory,
		publish,
		record,
		now,
		f.factoryDocs,
	)
}

// WithCommandRunner returns a validated copy with a per-runtime wrapper edge.
func (f *ScriptFactory) WithCommandRunner(runner CommandRunner) (*ScriptFactory, error) {
	if f == nil {
		return nil, errors.New("construct script worker factory: base factory is required")
	}
	if runner == nil {
		return f, nil
	}
	return NewScriptFactory(runner, f.commandClock, f.factoryDocs)
}

func newScriptExecutor(
	definition *workerconfig.FactoryWorkerConfig,
	commandRunner CommandRunner,
	logger logging.Logger,
	factoryDirectory string,
	publish workers.ProgressPublisher,
	record workers.ScriptEventRecorder,
	now func() time.Time,
	factoryDocs workers.FactoryDocsLoader,
) (*ScriptExecutor, error) {
	if definition == nil {
		return nil, errors.New("construct script worker: definition is required")
	}
	registry, err := resolveScriptRegistry(
		definition,
		commandRunner,
		factoryDirectory,
		publish,
		record,
		now,
		factoryDocs,
	)
	if err != nil {
		return nil, err
	}
	return &ScriptExecutor{
		Command:       definition.Command,
		Args:          append([]string(nil), definition.Args...),
		FactoryDir:    strings.TrimSpace(factoryDirectory),
		CommandRunner: commandRunner,
		Logger:        logger,
		recorder:      record,
		Now:           now,
		FactoryDocs:   factoryDocs,
		Publish:       publish,
		registry:      registry,
	}, nil
}

func resolveScriptRegistry(
	definition *workerconfig.FactoryWorkerConfig,
	commandRunner CommandRunner,
	factoryDirectory string,
	publish workers.ProgressPublisher,
	record workers.ScriptEventRecorder,
	now func() time.Time,
	factoryDocs workers.FactoryDocsLoader,
) (runners.Service, error) {
	if publish == nil {
		publish = func(workers.ProgressFragment) {}
	}
	if record == nil {
		record = func(workers.ScriptEvent) {}
	}
	registry, err := runnerwire.NewScriptRegistry(
		runners.ScriptConfig{
			Command:          definition.Command,
			Args:             append([]string(nil), definition.Args...),
			FactoryDirectory: strings.TrimSpace(factoryDirectory),
		},
		runners.ScriptDependencies{
			CommandRunner: commandRunner,
			FactoryDocs:   factoryDocs,
			Now:           now,
			Publish:       publish,
			Record:        record,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("construct script worker: %w", err)
	}
	if _, err := registry.Resolve(runners.ResolutionRequest{
		Identity: runners.ScriptIdentity,
	}); err != nil {
		return nil, fmt.Errorf("resolve script worker: %w", err)
	}
	return registry, nil
}

// Execute invokes the private runners.Service.Execute boundary and translates
// its canonical result into the existing workstation result shape.
func (se *ScriptExecutor) Execute(
	ctx context.Context,
	request workers.WorkstationExecutionRequest,
) (workers.WorkResult, error) {
	if se == nil || se.registry == nil {
		return workers.WorkResult{}, errors.New("script executor runner registry is required")
	}
	result, executionErr := se.registry.Execute(ctx, runners.ExecuteRequest{
		Identity: runners.ScriptIdentity,
		Attempt:  scriptRunnerRequest(request),
	})
	return scriptWorkResult(request.Dispatch.DispatchID, request.Dispatch.TransitionID, result, executionErr), nil
}

// ExecuteWithWorker runs a dispatch with its invocation-interpolated Script
// Worker definition. Script command arguments are invocation data just like
// model/provider fields and must not remain frozen at runtime construction.
func (se *ScriptExecutor) ExecuteWithWorker(
	ctx context.Context,
	request workers.WorkstationExecutionRequest,
	definition *workerconfig.FactoryWorkerConfig,
) (workers.WorkResult, error) {
	if se == nil {
		return workers.WorkResult{}, errors.New("script executor is required")
	}
	if definition != nil && definition.Command == se.Command && slices.Equal(definition.Args, se.Args) {
		return se.Execute(ctx, request)
	}
	interpolated, err := newScriptExecutor(
		definition,
		se.CommandRunner,
		se.Logger,
		se.FactoryDir,
		se.Publish,
		se.recorder,
		se.Now,
		se.FactoryDocs,
	)
	if err != nil {
		return workers.WorkResult{}, err
	}
	return interpolated.Execute(ctx, request)
}

func scriptRunnerRequest(request workers.WorkstationExecutionRequest) workers.RunnerExecutionRequest {
	return workers.RunnerExecutionRequest{
		Dispatch:           request.Dispatch,
		WorkerType:         request.WorkerType,
		WorkstationType:    request.WorkstationType,
		RunnerID:           runners.ScriptIdentity,
		ProjectID:          request.ProjectID,
		InputTokens:        request.InputTokens,
		ModelOperation:     request.ModelOperation,
		ModelBindings:      request.ModelBindings,
		SystemPrompt:       request.SystemPrompt,
		UserMessage:        request.UserMessage,
		OutputSchema:       request.OutputSchema,
		EnvVars:            request.EnvVars,
		ProcessEnvironment: request.ProcessEnvironment,
		Worktree:           request.Worktree,
		WorkingDirectory:   request.WorkingDirectory,
		SessionID:          request.FactorySessionID,
	}
}

func scriptWorkResult(
	dispatchID string,
	transitionID string,
	execution workers.RunnerExecutionResult,
	executionErr error,
) workers.WorkResult {
	result := workers.WorkResult{
		DispatchID:   dispatchID,
		TransitionID: transitionID,
		Outcome:      workers.OutcomeAccepted,
		Output:       execution.Content,
		Diagnostics:  workers.CloneWorkDiagnostics(execution.Diagnostics),
	}
	if result.Diagnostics != nil && result.Diagnostics.Command != nil {
		result.Metrics.Duration = result.Diagnostics.Command.Duration
	}
	if executionErr == nil {
		return result
	}
	result.Outcome = workers.OutcomeFailed
	result.Error = scriptExecutionErrorMessage(executionErr)
	var providerErr *workers.ProviderError
	if errors.As(executionErr, &providerErr) {
		// Preserve the established workstation retry boundary: ordinary script
		// exit and process failures are terminal Work results, while timeouts
		// retain their explicit retryable metadata. Missing-executable metadata
		// is terminal and is surfaced for public dispatch failure details.
		if providerErr.Type == workers.WorkFailureTypeTimeout ||
			providerErr.Type == workers.WorkFailureTypeMissingExecutable {
			result.FailureMetadata = &workers.WorkFailureMetadata{
				Family: providerErr.Family,
				Type:   providerErr.Type,
			}
		}
		if providerErr.Diagnostics != nil {
			result.Diagnostics = workers.CloneWorkDiagnostics(providerErr.Diagnostics)
		}
	}
	return result
}

func scriptExecutionErrorMessage(err error) string {
	var providerErr *workers.ProviderError
	if errors.As(err, &providerErr) && strings.TrimSpace(providerErr.Message) != "" {
		if providerErr.Type == workers.WorkFailureTypeInternalServerError &&
			providerErr.Cause != nil {
			return "execution cancelled: " + providerErr.Cause.Error()
		}
		return providerErr.Message
	}
	return "execution cancelled: " + err.Error()
}

var _ WorkstationRequestExecutor = (*ScriptExecutor)(nil)
