package runtimeopening

import (
	"fmt"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func runtimeOpeningRequestFromActivation(
	request factoryruntime.RuntimeActivationRequest,
) (*factorysessions.RuntimeOpeningRequest, error) {
	definitionDirectory := strings.TrimSpace(request.Inputs.Definition.Directory)
	if definitionDirectory == "" {
		definitionDirectory = request.Snapshot.FactoryDir
	}
	executionBaseDir := strings.TrimSpace(request.Inputs.Definition.ExecutionBaseDir)
	if executionBaseDir == "" {
		executionBaseDir = request.Snapshot.RuntimeBaseDir
	}
	if definitionDirectory == "" {
		return nil, fmt.Errorf("runtime activation inputs: Factory Definition directory is required")
	}

	return &factorysessions.RuntimeOpeningRequest{
		FactoryDefinition: factorydefinitionsRuntimeOpeningRequest(
			definitionDirectory,
			request.Inputs.Definition.SourcePath,
			executionBaseDir,
		),
		FactoryRuntime: request.Runtime,
		FactorySession: factorysessions.SessionRuntimeOpeningRequest{
			FactorySessionID:  request.FactorySessionID,
			PersistencePolicy: factorysessions.PersistencePolicy(request.Inputs.Session.PersistencePolicy),
			BackendScopeID:    request.Inputs.Session.BackendScopeID,
			SystemConfigHome:  request.Inputs.Session.SystemConfigHome,
			SystemConfigPath:  request.Inputs.Session.SystemConfigPath,
			WorkFile:          request.Inputs.Session.WorkFile,
			Host: factorysessions.RuntimeHostRequest{
				Directory:   request.Inputs.Session.Host.Directory,
				RuntimeMode: request.Inputs.Session.Host.RuntimeMode,
				WorkFile:    request.Inputs.Session.Host.WorkFile,
				MockWorkers: request.Inputs.Session.Host.MockWorkers,
				Host:        request.Inputs.Session.Host.Host,
				Port:        request.Inputs.Session.Host.Port,
				AutoPort:    request.Inputs.Session.Host.AutoPort,
			},
		},
		Workers: workers.RuntimeOpeningRequest{
			RunnerID:                          request.Inputs.Workers.RunnerID,
			Worktree:                          request.Inputs.Workers.Worktree,
			WorkerReasoningEffort:             request.Inputs.Workers.WorkerReasoningEffort,
			MockWorkers:                       activationMockWorkers(request.Inputs.Workers.MockWorkers),
			InvocationSkipPermissionsOverride: request.Inputs.Workers.InvocationSkipPermissionsOverride,
			SkipBuiltInPrerequisiteValidation: request.Inputs.Workers.SkipBuiltInPrerequisiteValidation,
		},
		Recordings:          recordingsRuntimeOpeningRequest(request),
		ModelCacheDirectory: request.Inputs.ModelCacheDirectory,
		OperatorDefaults: operatorsettings.ResolvedDefaults{
			WorkerModelProvider: request.Inputs.OperatorDefaults.WorkerModelProvider,
			WorkerModel:         request.Inputs.OperatorDefaults.WorkerModel,
			ConfigPath:          request.Inputs.OperatorDefaults.ConfigPath,
		},
	}, nil
}

func factorydefinitionsRuntimeOpeningRequest(directory, sourcePath, executionBaseDir string) factorydefinitions.RuntimeOpeningRequest {
	return factorydefinitions.RuntimeOpeningRequest{
		Directory:        directory,
		SourcePath:       sourcePath,
		ExecutionBaseDir: executionBaseDir,
	}
}

func recordingsRuntimeOpeningRequest(request factoryruntime.RuntimeActivationRequest) recordings.RuntimeOpeningRequest {
	return recordings.RuntimeOpeningRequest{
		RecordPath:    request.Inputs.Recordings.RecordPath,
		ReplayPath:    request.Inputs.Recordings.ReplayPath,
		WorkflowID:    request.Inputs.Recordings.WorkflowID,
		FlushInterval: request.Inputs.Recordings.FlushInterval,
	}
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
		config.MockWorkers[index] = converted
	}
	return config
}
