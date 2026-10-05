package internal

import (
	"context"
	"fmt"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

type initialAssembly interface {
	Assemble(
		ctx context.Context,
		defaultWorkerModelProvider string,
		defaultWorkerModel string,
		applyOperatorDefaults bool,
		recordPath string,
		workflowID string,
		defaultSessionID string,
		metricsSessionID string,
		mockWorkersConfig *workers.MockWorkersConfig,
		runtimeMode factorydefinitions.RuntimeMode,
		runtimeScheduler factoryruntime.Scheduler,
		inlineDispatch bool,
		runtimeLogDir string,
		runtimeLogConfig factoryruntime.RuntimeLogStorageConfig,
		runtimeFileLoggingPolicy factoryruntime.RuntimeFileLoggingPolicy,
		runtimeMetricsPolicy factoryruntime.RuntimeMetricsPolicy,
		runtimeMetricsDir string,
		runtimeMetricsConfig factoryruntime.RuntimeMetricsStorageConfig,
		recordFlushInterval time.Duration,
		backendScopeID string,
		factoryRunnerID string,
		verbose bool,
		skipBuiltInPrerequisiteValidation bool,
		invocationSkipPermissionsOverride *bool,
		clock factoryruntime.Clock,
		baseLogger *zap.Logger,
		publishRuntimeStreams bool,
		observations factoryruntime.SessionObservations,
		dir string,
		factoryRootDir string,
		executionBaseDir string,
		loadedFactory factorydefinitions.MutableLoadedFactorySource,
		runtimeInstanceID string,
		replayArtifact *factorydefinitions.ReplayArtifact,
		resumeInput *recordings.LoadResumeInputResult,
		restoredWorldState *factorydefinitions.FactoryWorldState,
		restoredEventHistory []factorydefinitions.FactoryEvent,
		serviceMode bool,
	) (*factoryruntime.RuntimeInitialOpening, error)
}

// InitialActivation owns fixed initial-opening behavior. It never stores an
// opening's observations or mutable definition on the reusable owner.
type InitialActivation struct {
	assembly    initialAssembly
	clock       factoryruntime.Clock
	logger      *zap.Logger
	materialize func(*factorydefinitions.RuntimeSnapshot, string) (factorydefinitions.MutableLoadedFactorySource, error)
}

func NewInitialActivation(assembly initialAssembly, clock factoryruntime.Clock, logger *zap.Logger,
	materialize func(*factorydefinitions.RuntimeSnapshot, string) (factorydefinitions.MutableLoadedFactorySource, error),
) *InitialActivation {
	return &InitialActivation{assembly: assembly, clock: clock, logger: logger, materialize: materialize}
}

func (a *InitialActivation) Open(ctx context.Context, request factoryruntime.RuntimeActivationRequest,
	observations factoryruntime.SessionObservations,
) (*factoryruntime.RuntimeInitialOpening, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	loaded, err := a.materialize(&request.Snapshot, request.Inputs.Definition.ExecutionBaseDir)
	if err != nil {
		return nil, err
	}
	if effort := request.Inputs.Workers.WorkerReasoningEffort; effort != "" {
		if err := loaded.MutateWorkers(func(worker *factorydefinitions.FactoryWorkerConfig) error {
			worker.ReasoningEffort = effort
			return nil
		}); err != nil {
			return nil, fmt.Errorf("apply worker reasoning effort override: %w", err)
		}
	}
	metricsID := request.Inputs.Session.CanonicalSessionID
	if metricsID == "" {
		metricsID = request.FactorySessionID
	}
	// Sessions normalizes Definition.Directory to the opening workspace root.
	// Snapshot.FactoryDir identifies the loaded definition, which may be nested.
	return a.assembly.Assemble(ctx,
		request.Inputs.OperatorDefaults.WorkerModelProvider, request.Inputs.OperatorDefaults.WorkerModel, true,
		request.Inputs.Recordings.RecordPath, request.Inputs.Recordings.WorkflowID,
		request.FactorySessionID, metricsID, activationMockWorkers(request.Inputs.Workers.MockWorkers),
		request.Runtime.Mode, nil, false,
		request.Runtime.LogDirectory, request.Runtime.LogConfig, request.Runtime.FileLoggingPolicy,
		request.Runtime.MetricsPolicy, request.Runtime.MetricsDirectory, request.Runtime.MetricsConfig,
		request.Inputs.Recordings.FlushInterval, request.Inputs.Session.BackendScopeID,
		request.Inputs.Workers.RunnerID, request.Runtime.Verbose,
		request.Inputs.Workers.SkipBuiltInPrerequisiteValidation, request.Inputs.Workers.InvocationSkipPermissionsOverride,
		a.clock, a.logger, true, observations,
		request.Inputs.Definition.Directory, request.Inputs.Definition.Directory, request.Inputs.Definition.ExecutionBaseDir,
		loaded, request.RuntimeID, nil, nil, nil, nil, request.Runtime.Mode == factorydefinitions.RuntimeModeService)
}

func cloneInt64Pointer(value *int64) *int64 {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}
func activationMockWorkers(input *factoryruntime.RuntimeActivationMockWorkersConfig) *workers.MockWorkersConfig {
	if input == nil {
		return nil
	}
	config := &workers.MockWorkersConfig{
		UnmatchedDispatchPolicy: workers.MockWorkerUnmatchedDispatchPolicy(input.UnmatchedDispatchPolicy),
		MockWorkers:             make([]workers.MockWorkerConfig, len(input.MockWorkers)),
	}
	for index, worker := range input.MockWorkers {
		converted := workers.MockWorkerConfig{
			ID:              worker.ID,
			WorkerName:      worker.WorkerName,
			WorkstationName: worker.WorkstationName,
			RunType:         workers.MockWorkerRunType(worker.RunType),
			WorkInputs:      make([]workers.MockWorkInputSelector, len(worker.WorkInputs)),
		}
		for inputIndex, workInput := range worker.WorkInputs {
			converted.WorkInputs[inputIndex] = workers.MockWorkInputSelector{
				WorkID:      workInput.WorkID,
				WorkType:    workInput.WorkType,
				State:       workInput.State,
				InputName:   workInput.InputName,
				TraceID:     workInput.TraceID,
				Channel:     workInput.Channel,
				PayloadHash: workInput.PayloadHash,
			}
		}
		if worker.ScriptConfig != nil {
			script := &workers.MockWorkerScriptConfig{
				Command:          worker.ScriptConfig.Command,
				Args:             append([]string(nil), worker.ScriptConfig.Args...),
				Env:              make(map[string]string, len(worker.ScriptConfig.Env)),
				WorkingDirectory: worker.ScriptConfig.WorkingDirectory,
				Stdin:            worker.ScriptConfig.Stdin,
				Timeout:          worker.ScriptConfig.Timeout,
			}
			for key, value := range worker.ScriptConfig.Env {
				script.Env[key] = value
			}
			converted.ScriptConfig = script
		}
		if worker.RejectConfig != nil {
			reject := &workers.MockWorkerRejectConfig{
				Stdout: worker.RejectConfig.Stdout,
				Stderr: worker.RejectConfig.Stderr,
			}
			if worker.RejectConfig.ExitCode != nil {
				value := *worker.RejectConfig.ExitCode
				reject.ExitCode = &value
			}
			converted.RejectConfig = reject
		}
		if worker.GateConfig != nil {
			converted.GateConfig = &workers.MockWorkerGateConfig{
				ArrivedFile: worker.GateConfig.ArrivedFile,
				ReleaseFile: worker.GateConfig.ReleaseFile,
				Timeout:     worker.GateConfig.Timeout,
			}
		}
		if worker.Usage != nil {
			converted.Usage = &workers.MockWorkerUsageConfig{
				Provider:              worker.Usage.Provider,
				Model:                 worker.Usage.Model,
				InputTokens:           cloneInt64Pointer(worker.Usage.InputTokens),
				OutputTokens:          cloneInt64Pointer(worker.Usage.OutputTokens),
				CachedInputTokens:     cloneInt64Pointer(worker.Usage.CachedInputTokens),
				ReasoningOutputTokens: cloneInt64Pointer(worker.Usage.ReasoningOutputTokens),
			}
		}
		config.MockWorkers[index] = converted
	}
	return config
}
