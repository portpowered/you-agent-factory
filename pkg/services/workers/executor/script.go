package executor

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	workerconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
	runnerwire "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/wire"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/process"
)

// ScriptExecutor adapts the common Runner result onto the workstation result
// boundary. Production construction sets runner through the private immutable
// registry; the remaining configuration fields support package-local legacy
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

	runnerMu sync.Mutex
	runner   workers.Runner
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
	commandRunner := workerprocess.CommandRunnerWithLogging(
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

// NewScriptExecutorWithDependencies preserves the existing package-local
// constructor while routing execution through the private Runner registry.
func NewScriptExecutorWithDependencies(
	definition *workerconfig.FactoryWorkerConfig,
	commandRunner CommandRunner,
	logger logging.Logger,
	factoryDirectory string,
	record workers.ScriptEventRecorder,
	now func() time.Time,
	factoryDocs workers.FactoryDocsLoader,
) (*ScriptExecutor, error) {
	if definition == nil {
		return nil, errors.New("construct script worker: definition is required")
	}
	if commandRunner == nil {
		return nil, errors.New("construct script worker: command runner is required")
	}
	if factoryDocs == nil {
		return nil, errors.New("construct script worker: Factory docs loader is required")
	}
	if now == nil {
		executor := NewScriptExecutorWithRunner(
			definition,
			commandRunner,
			logger,
			factoryDirectory,
			record,
			now,
		)
		executor.FactoryDocs = factoryDocs
		return executor, nil
	}
	return newScriptExecutor(
		definition,
		commandRunner,
		logger,
		factoryDirectory,
		nil,
		record,
		now,
		factoryDocs,
	)
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
	if commandRunner == nil {
		return nil, errors.New("construct script worker: command runner is required")
	}
	if factoryDocs == nil {
		return nil, errors.New("construct script worker: Factory docs loader is required")
	}
	if now == nil {
		return nil, errors.New("construct script worker: clock is required")
	}
	binding, err := resolveScriptRunner(
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
		runner:        binding.Runner,
	}, nil
}

func resolveScriptRunner(
	definition *workerconfig.FactoryWorkerConfig,
	commandRunner CommandRunner,
	factoryDirectory string,
	publish workers.ProgressPublisher,
	record workers.ScriptEventRecorder,
	now func() time.Time,
	factoryDocs workers.FactoryDocsLoader,
) (runners.Binding, error) {
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
			CommandRunner: ensureStreamingCommandRunner(commandRunner),
			FactoryDocs:   factoryDocs,
			Now:           now,
			Publish:       publish,
			Record:        record,
		},
	)
	if err != nil {
		return runners.Binding{}, fmt.Errorf("construct script worker: %w", err)
	}
	binding, err := registry.Resolve(runners.ResolutionRequest{
		Identity: runners.ScriptIdentity,
	})
	if err != nil {
		return runners.Binding{}, fmt.Errorf("resolve script worker: %w", err)
	}
	return binding, nil
}

// NewScriptExecutor creates a compatibility adapter. Production construction
// uses ScriptFactory.New so incomplete dependencies fail before dispatch.
func NewScriptExecutor(
	definition *workerconfig.FactoryWorkerConfig,
	logger logging.Logger,
	factoryDirectory string,
	record workers.ScriptEventRecorder,
	now func() time.Time,
) *ScriptExecutor {
	if definition == nil {
		return &ScriptExecutor{Logger: logger, Now: now}
	}
	return &ScriptExecutor{
		Command:    definition.Command,
		Args:       append([]string(nil), definition.Args...),
		FactoryDir: strings.TrimSpace(factoryDirectory),
		Logger:     logger,
		recorder:   record,
		Now:        now,
	}
}

// NewScriptExecutorWithRunner creates a compatibility adapter with an explicit
// command edge. It still resolves the Script Runner through the private registry
// when Execute first observes the complete legacy configuration.
func NewScriptExecutorWithRunner(
	definition *workerconfig.FactoryWorkerConfig,
	runner CommandRunner,
	logger logging.Logger,
	factoryDirectory string,
	record workers.ScriptEventRecorder,
	now func() time.Time,
) *ScriptExecutor {
	executor := NewScriptExecutor(definition, logger, factoryDirectory, record, now)
	executor.CommandRunner = runner
	return executor
}

// Execute invokes only the registry-resolved common Runner and translates its
// canonical result into the existing workstation result shape.
func (se *ScriptExecutor) Execute(
	ctx context.Context,
	request workers.WorkstationExecutionRequest,
) (workers.WorkResult, error) {
	if se == nil || se.Now == nil {
		return workers.WorkResult{}, errors.New("script executor clock is required")
	}
	runner, err := se.resolvedRunner()
	if err != nil {
		return workers.WorkResult{}, err
	}
	result, executionErr := runner.Execute(ctx, scriptRunnerRequest(request))
	return scriptWorkResult(request.Dispatch.DispatchID, request.Dispatch.TransitionID, result, executionErr), nil
}

func (se *ScriptExecutor) resolvedRunner() (workers.Runner, error) {
	se.runnerMu.Lock()
	defer se.runnerMu.Unlock()
	if se.runner != nil {
		return se.runner, nil
	}
	if se.CommandRunner == nil {
		return nil, errors.New("construct script worker: command runner is required")
	}
	factoryDocs := se.FactoryDocs
	if factoryDocs == nil {
		factoryDocs = func(string) (map[string]string, error) { return nil, nil }
	}
	binding, err := resolveScriptRunner(
		&workerconfig.FactoryWorkerConfig{
			Command: se.Command,
			Args:    append([]string(nil), se.Args...),
		},
		se.CommandRunner,
		se.FactoryDir,
		se.Publish,
		se.recorder,
		se.Now,
		factoryDocs,
	)
	if err != nil {
		return nil, err
	}
	se.runner = binding.Runner
	return se.runner, nil
}

type streamingCommandRunner interface {
	RunStreaming(
		context.Context,
		CommandRequest,
		workerprocess.OutputChunkObserver,
	) (CommandResult, error)
}

type completeOutputStreamingRunner struct {
	CommandRunner
}

func ensureStreamingCommandRunner(runner CommandRunner) CommandRunner {
	if _, ok := runner.(streamingCommandRunner); ok {
		return runner
	}
	return completeOutputStreamingRunner{CommandRunner: runner}
}

func (r completeOutputStreamingRunner) RunStreaming(
	ctx context.Context,
	request CommandRequest,
	observer workerprocess.OutputChunkObserver,
) (CommandResult, error) {
	result, err := r.Run(ctx, request)
	if observer != nil && len(result.Stdout) > 0 {
		observer(workerprocess.OutputStreamStdout, append([]byte(nil), result.Stdout...))
	}
	if observer != nil && len(result.Stderr) > 0 {
		observer(workerprocess.OutputStreamStderr, append([]byte(nil), result.Stderr...))
	}
	return result, err
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
		// retain their explicit retryable metadata.
		if providerErr.Type == workers.WorkFailureTypeTimeout {
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

func executionWorkDir(request workers.WorkstationExecutionRequest) string {
	if request.WorkingDirectory != "" {
		return request.WorkingDirectory
	}
	return request.Worktree
}

var _ WorkstationRequestExecutor = (*ScriptExecutor)(nil)
