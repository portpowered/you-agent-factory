package wire

import (
	"context"
	"fmt"
	"strings"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	executeservice "github.com/portpowered/infinite-you/pkg/services/workers/internal/service"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/process"
	runnerswire "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/executor/agentrun"
)

// statelessProviderContract supplies the native Providers operations that are
// not relevant to these Workers execution assertions. Keeping the test seam
// execute-shaped means the core Workers tests do not need the legacy
// inference-shaped adapter used by later caller-family migrations.
type statelessProviderContract struct{}

func (statelessProviderContract) SupportsContinuation(context.Context, providers.SessionRef) (bool, error) {
	return false, nil
}

func (statelessProviderContract) ListProviders(
	context.Context,
	providers.ListProvidersRequest,
) (providers.ListProvidersResult, error) {
	return providers.ListProvidersResult{Providers: []providers.Descriptor{{ID: providers.IDCodex}}}, nil
}

func (statelessProviderContract) GetProvider(
	_ context.Context,
	request providers.GetProviderRequest,
) (providers.GetProviderResult, error) {
	if err := request.Validate(); err != nil {
		return providers.GetProviderResult{}, err
	}
	if request.ID != providers.IDCodex {
		return providers.GetProviderResult{}, providers.ErrUnknownProvider
	}
	return providers.GetProviderResult{Provider: providers.Descriptor{ID: providers.IDCodex}}, nil
}

func (statelessProviderContract) ResolveIdentity(
	_ context.Context,
	request providers.ResolveIdentityRequest,
) (providers.ResolveIdentityResult, error) {
	identity := strings.ToLower(strings.TrimSpace(request.Identity))
	if identity == "openai" {
		identity = string(providers.IDCodex)
	}
	if identity != string(providers.IDCodex) {
		return providers.ResolveIdentityResult{}, providers.ErrUnknownProvider
	}
	return providers.ResolveIdentityResult{ID: providers.IDCodex}, nil
}

func (statelessProviderContract) ResolveSelection(
	ctx context.Context,
	request providers.ResolveSelectionRequest,
) (providers.ResolveSelectionResult, error) {
	identity := request.Workstation
	if identity == "" {
		identity = request.Factory
	}
	if identity == "" {
		identity = request.ModelProvider
	}
	resolved, err := (statelessProviderContract{}).ResolveIdentity(
		ctx,
		providers.ResolveIdentityRequest{Identity: identity},
	)
	if err != nil {
		return providers.ResolveSelectionResult{}, err
	}
	return providers.ResolveSelectionResult{Provider: resolved.ID}, nil
}

func (statelessProviderContract) ControlAttempt(
	_ context.Context,
	request providers.ControlAttemptRequest,
) (providers.ControlAttemptResult, error) {
	if err := request.Validate(); err != nil {
		return providers.ControlAttemptResult{}, err
	}
	return providers.ControlAttemptResult{
		Provider:  request.Provider,
		AttemptID: request.AttemptID,
		Action:    request.Action,
		Outcome:   providers.ControlOutcomeUnsupported,
	}, nil
}

func (statelessProviderContract) Continue(
	_ context.Context,
	request providers.ContinueRequest,
) (providers.ContinueResult, error) {
	if err := request.Validate(); err != nil {
		return providers.ContinueResult{}, err
	}
	return providers.ContinueResult{
		Reference: request.Reference,
		Outcome:   providers.ContinuationOutcomeUnsupported,
	}, nil
}

func (statelessProviderContract) ContinueReference(
	_ context.Context,
	request providers.ContinueReferenceRequest,
) (providers.ContinueReferenceResult, error) {
	reference, err := request.Reference.ToSessionRef()
	if err != nil {
		return providers.ContinueReferenceResult{}, err
	}
	return providers.ContinueReferenceResult{
		Reference: reference.ContinuationRef(),
		Outcome:   providers.ContinuationOutcomeUnsupported,
	}, nil
}

// These test-only inputs retain the preexisting legacy composition witnesses.
type AgentDependencies struct {
	Providers         providers.Service
	Publish           workers.ProgressPublisher
	DecisionEnvelopes factorydefinitions.DecisionEnvelopeService
}
type ScriptDependencies struct {
	CommandRunner platformprocess.CommandRunner
	FactoryDocs   workers.FactoryDocsLoader
	Now           func() time.Time
	Publish       workers.ProgressPublisher
	Record        workers.ScriptEventRecorder
}
type InferenceDependencies struct {
	Models interface {
		InvokeModel(context.Context, models.InvokeModelRequest) (models.InvokeModelResult, error)
	}
	Delegate            runners.Strategy
	ContentMaterializer work.ContentMaterializer
	MediaFiles          platformfilesystem.ReadOpener
}
type MockDependencies struct {
	Next platformprocess.CommandRunner
}

func newLegacyStatelessService(
	agentDependencies AgentDependencies,
	scriptConfig ScriptConfig,
	scriptDependencies ScriptDependencies,
	inferenceConfig InferenceConfig,
	inferenceDependencies InferenceDependencies,
	observe workers.ObservationSink,
	logger logging.Logger,
	clock func() time.Time,
	worktree workers.FactoryWorktreePreparer,
	worktreeRelease func(context.Context, workers.FactoryWorktreePreparation) error,
	temporaryFiles workers.TemporaryFileSystem,
	agentToolFiles workers.AgentToolFileSystem,
	providerOverrides ...providers.Service,
) (workers.Service, error) {
	privateScriptDependencies := privateScriptDependencies(scriptDependencies, logger, clock, nil)
	agentRunner, scriptRunner, inferenceRunner, err := productionStrategies(
		agentDependencies, scriptConfig, privateScriptDependencies, inferenceConfig, inferenceDependencies,
	)
	if err != nil {
		return nil, fmt.Errorf("construct Workers: %w", err)
	}
	runnerRegistry, err := runnerswire.NewProductionRegistry(agentRunner, scriptRunner, inferenceRunner)
	if err != nil {
		return nil, fmt.Errorf("construct Workers: %w", err)
	}
	var providerOverride providers.Service
	if len(providerOverrides) > 0 {
		providerOverride = providerOverrides[0]
	}
	executeService, err := executeservice.NewWithProviderOverride(
		runnerRegistry,
		agentDependencies.Providers,
		observe,
		logger,
		clock,
		worktree,
		worktreeRelease,
		temporaryFiles,
		providerOverride,
		agentrun.NewLibraryHarnessAdapter(agentToolFiles),
		agentDependencies.DecisionEnvelopes,
		scriptDependencies.FactoryDocs,
	)
	if err != nil {
		return nil, fmt.Errorf("construct Workers: %w", err)
	}
	return NewService(executeService)
}

func newLegacyMockStatelessService(
	agentDependencies AgentDependencies,
	scriptConfig ScriptConfig,
	scriptDependencies ScriptDependencies,
	inferenceConfig InferenceConfig,
	inferenceDependencies InferenceDependencies,
	mockWorkers *workers.MockWorkersConfig,
	mockDependencies MockDependencies,
	observe workers.ObservationSink,
	logger logging.Logger,
	clock func() time.Time,
	worktree workers.FactoryWorktreePreparer,
	worktreeRelease func(context.Context, workers.FactoryWorktreePreparation) error,
	temporaryFiles workers.TemporaryFileSystem,
	agentToolFiles workers.AgentToolFileSystem,
	providerOverrides ...providers.Service,
) (workers.Service, error) {
	if mockWorkers == nil {
		return nil, fmt.Errorf("construct mock Workers: mock workers config is required")
	}
	privateScriptDependencies := privateScriptDependencies(scriptDependencies, logger, clock, agentToolFiles)
	agentRunner, scriptRunner, inferenceRunner, err := productionStrategies(
		agentDependencies, scriptConfig, privateScriptDependencies, inferenceConfig, inferenceDependencies,
	)
	if err != nil {
		return nil, fmt.Errorf("construct mock Workers: %w", err)
	}
	mockRunner, err := runnerswire.NewMockRunner(
		runnerswire.MockRunnerConfig{WorkersConfig: mockWorkers},
		workerprocess.AdaptPlatformCommandRunner(mockDependencies.Next), agentToolFiles,
	)
	if err != nil {
		return nil, fmt.Errorf("construct mock Workers: %w", invalidRunnerConstruction(runners.MockIdentity, err))
	}
	runnerRegistry, err := runnerswire.NewMockProductionRegistry(agentRunner, scriptRunner, inferenceRunner, mockRunner)
	if err != nil {
		return nil, fmt.Errorf("construct mock Workers: %w", err)
	}
	var providerOverride providers.Service
	if len(providerOverrides) > 0 {
		providerOverride = providerOverrides[0]
	}
	executeService, err := executeservice.NewWithProviderOverride(
		runnerRegistry,
		agentDependencies.Providers,
		observe,
		logger,
		clock,
		worktree,
		worktreeRelease,
		temporaryFiles,
		providerOverride,
		agentrun.NewLibraryHarnessAdapter(agentToolFiles),
		agentDependencies.DecisionEnvelopes,
		scriptDependencies.FactoryDocs,
	)
	if err != nil {
		return nil, fmt.Errorf("construct mock Workers: %w", err)
	}
	return NewService(executeService)
}

func privateScriptDependencies(
	dependencies ScriptDependencies,
	logger logging.Logger,
	clock func() time.Time,
	gateFiles workers.AgentToolFileSystem,
) legacyScriptDependencies {
	commandRunner := newContextualMockWorkerCommandRunner(
		workerprocess.AdaptPlatformCommandRunner(dependencies.CommandRunner),
		gateFiles,
	)
	if commandRunner != nil && clock != nil {
		commandRunner = workerprocess.CommandRunnerWithLogging(
			commandRunner,
			logger,
			workerprocess.ClockFunc(clock),
		)
	}
	return legacyScriptDependencies{
		CommandRunner: commandRunner,
		FactoryDocs:   dependencies.FactoryDocs,
		Now:           dependencies.Now,
		Publish:       dependencies.Publish,
		Record:        dependencies.Record,
	}
}

// Legacy composition fixtures retain their existing behavioral assertions;
// canonical production composition is generated from focused providers.
func productionStrategies(
	agentDependencies AgentDependencies,
	scriptConfig ScriptConfig,
	scriptDependencies legacyScriptDependencies,
	inferenceConfig InferenceConfig,
	inferenceDependencies InferenceDependencies,
) (runners.Strategy, runners.Strategy, runners.Strategy, error) {
	agent, err := agentImplementation(agentDependencies)
	if err != nil {
		return nil, nil, nil, invalidRunnerConstruction(runners.AgentIdentity, err)
	}
	script, err := legacyScriptImplementation(scriptConfig, scriptDependencies)
	if err != nil {
		return nil, nil, nil, invalidRunnerConstruction(runners.ScriptIdentity, err)
	}
	delegate := inferenceDependencies.Delegate
	if delegate == nil {
		delegate = agent
	}
	inferenceDependencies.Delegate = delegate
	inference, err := inferenceImplementation(inferenceConfig, inferenceDependencies)
	if err != nil {
		return nil, nil, nil, invalidRunnerConstruction(runners.InferenceIdentity, err)
	}
	return agent, script, inference, nil
}
func invalidRunnerConstruction(identity string, err error) error {
	return fmt.Errorf(
		"%w: %s runner construction failed: %w",
		workers.ErrInvalidRunnerRegistration,
		identity,
		err,
	)
}

func legacyScriptImplementation(
	config ScriptConfig,
	dependencies legacyScriptDependencies,
) (runners.Strategy, error) {
	if strings.TrimSpace(config.Command) == "" && !config.RequestSelected {
		return nil, workers.NewProviderError(workers.WorkFailureTypeMisconfigured, "script command is required", nil)
	}
	commandRunner, ok := dependencies.CommandRunner.(workerprocess.StreamingCommandRunner)
	if !ok {
		return nil, workers.NewProviderError(workers.WorkFailureTypeMisconfigured, "script command runner must support streaming", nil)
	}
	return runnerswire.NewScriptRunner(
		runnerswire.ScriptRunnerConfig{
			Command:          config.Command,
			Args:             append([]string(nil), config.Args...),
			Stdin:            config.Stdin,
			FactoryDirectory: config.FactoryDirectory,
			RequestSelected:  config.RequestSelected,
		},
		commandRunner,
		dependencies.FactoryDocs,
		dependencies.Now,
		dependencies.Publish,
		dependencies.Record,
	), nil
}
func inferenceImplementation(
	config InferenceConfig,
	dependencies InferenceDependencies,
) (runners.Strategy, error) {
	return runnerswire.NewInferenceRunner(
		runnerswire.InferenceRunnerConfig{
			Worker: snapshotInferenceWorker(config.Worker),
			Resources: append(
				[]models.LocalResource(nil),
				config.Resources...,
			),
			Scope: config.Scope,
		},
		dependencies.Models, dependencies.Delegate, dependencies.ContentMaterializer, dependencies.MediaFiles,
	)
}

func agentImplementation(
	dependencies AgentDependencies,
) (runners.Strategy, error) {
	return runnerswire.NewAgentRunner(
		dependencies.Providers,
		dependencies.Publish,
		dependencies.DecisionEnvelopes,
	)
}

func snapshotInferenceWorker(worker models.LocalWorker) models.LocalWorker {
	worker.Resources = append([]models.LocalResource(nil), worker.Resources...)
	return worker
}

type legacyScriptDependencies struct {
	CommandRunner workerprocess.CommandRunner
	FactoryDocs   workers.FactoryDocsLoader
	Now           func() time.Time
	Publish       workers.ProgressPublisher
	Record        workers.ScriptEventRecorder
}
