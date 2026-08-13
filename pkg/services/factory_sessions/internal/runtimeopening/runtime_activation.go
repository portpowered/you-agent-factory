package runtimeopening

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// activatedRuntimeService is the private handoff stored by the Runtime root.
// The embedded service is the completed Factory Sessions runtime; the product
// roles remain private to this opener and are exposed only through the same
// activation result that published the service.
type activatedRuntimeService struct {
	factoryruntime.Service
	factoryruntime.APIFactory
	products runtimeProducts
}

func (s *activatedRuntimeService) runtimeProducts() runtimeProducts {
	if s == nil {
		return runtimeProducts{}
	}
	return s.products
}

func (f *Factory) activateRuntime(
	ctx context.Context,
	request factoryruntime.RuntimeActivationRequest,
) (*factoryruntime.RuntimeActivation, error) {
	openingRequest, err := runtimeOpeningRequestFromActivation(request)
	if err != nil {
		return nil, err
	}
	products, err := f.openRuntime(ctx, openingRequest, f.baseLogger)
	if err != nil {
		return nil, err
	}
	service := products.application.HTTP.FactoryRuntime
	if service == nil {
		return nil, fmt.Errorf("activate Factory Runtime: opened runtime service is required")
	}
	legacyObservation, ok := service.(factoryruntime.APIFactory)
	if !ok {
		return nil, fmt.Errorf("activate Factory Runtime: opened runtime legacy observation is required")
	}
	return &factoryruntime.RuntimeActivation{
		Service:        &activatedRuntimeService{Service: service, APIFactory: legacyObservation, products: products},
		HostedInstance: products.startup,
		Replacement:    products.replacement,
		BuildSpec:      products.buildSpec,
		Lifecycle:      products.lifecycle,
		Sidecars:       products.sidecars,
		Close: func(closeCtx context.Context) error {
			if products.application.Resources.Close == nil {
				return nil
			}
			return products.application.Resources.Close()
		},
	}, nil
}

func (f *Factory) openActivatedRuntime(
	ctx context.Context,
	request *factorysessions.RuntimeOpeningRequest,
) (runtimeProducts, error) {
	if f == nil || f.runtimeRoot == nil {
		return runtimeProducts{}, fmt.Errorf("open Factory Runtime: Runtime root is required")
	}
	activationRequest, err := f.activationRequest(request)
	if err != nil {
		return runtimeProducts{}, err
	}
	result, err := f.runtimeRoot.Activate(ctx, activationRequest)
	if err != nil {
		return runtimeProducts{}, err
	}
	handoff, ok := result.Runtime.Service.(*activatedRuntimeService)
	if !ok || handoff == nil {
		return runtimeProducts{}, fmt.Errorf("open Factory Runtime: activation handoff is unavailable")
	}
	products := handoff.runtimeProducts()
	closeRuntime := f.activationCloser(result.RuntimeID)
	products.application.HTTP.FactoryRuntime = f.runtimeRoot
	products.application.Resources.Close = closeRuntime
	products.invocation.CloseArtifacts = closeRuntime
	products.execution.Resources.Close = closeRuntime
	return products, nil
}

func (f *Factory) activationCloser(runtimeID string) func() error {
	var mu sync.Mutex
	closed := false
	return func() error {
		mu.Lock()
		if closed {
			mu.Unlock()
			return nil
		}
		mu.Unlock()
		_, err := f.runtimeRoot.Deactivate(
			context.Background(),
			factoryruntime.RuntimeDeactivationRequest{RuntimeID: runtimeID},
		)
		mu.Lock()
		if err == nil {
			closed = true
		}
		mu.Unlock()
		return err
	}
}

func (f *Factory) activationRequest(
	request *factorysessions.RuntimeOpeningRequest,
) (factoryruntime.RuntimeActivationRequest, error) {
	if request == nil {
		return factoryruntime.RuntimeActivationRequest{}, fmt.Errorf("open Factory Runtime: runtime opening request is required")
	}
	opening := *request
	runtimeID := strings.TrimSpace(opening.FactoryRuntime.RuntimeInstanceID)
	if runtimeID == "" {
		if f.generateRuntimeInstanceID == nil {
			return factoryruntime.RuntimeActivationRequest{}, fmt.Errorf("open Factory Runtime: runtime instance ID generator is required")
		}
		runtimeID = strings.TrimSpace(f.generateRuntimeInstanceID())
		if runtimeID == "" {
			return factoryruntime.RuntimeActivationRequest{}, fmt.Errorf("open Factory Runtime: runtime instance ID generator returned an empty identity")
		}
		opening.FactoryRuntime.RuntimeInstanceID = runtimeID
	}
	factoryDir := strings.TrimSpace(opening.FactoryDefinition.Directory)
	if factoryDir == "" {
		factoryDir = strings.TrimSpace(opening.FactoryDefinition.SourcePath)
	}
	if factoryDir == "" {
		return factoryruntime.RuntimeActivationRequest{}, fmt.Errorf("open Factory Runtime: Factory Definition directory is required")
	}
	runtimeBaseDir := strings.TrimSpace(opening.FactoryDefinition.ExecutionBaseDir)
	if runtimeBaseDir == "" {
		runtimeBaseDir = factoryDir
	}
	name := filepath.Base(filepath.Clean(factoryDir))
	if name == "." || name == string(filepath.Separator) || name == "" {
		name = "runtime"
	}
	snapshot := factorydefinitions.RuntimeSnapshot{
		FactoryDir:        factoryDir,
		RuntimeBaseDir:    runtimeBaseDir,
		Invocation:        factorydefinitions.RuntimeSnapshotInvocationContext{FactorySessionID: factorysessions.DefaultSessionID, WorkflowID: opening.Recordings.WorkflowID},
		DefinitionVersion: &factorydefinitions.FactoryVersion{Logical: 1},
		EffectiveFactory:  factorydefinitions.FactoryConfig{Name: name},
	}
	f.populateActivationSnapshot(&snapshot, factoryDir, runtimeBaseDir)
	return factoryruntime.RuntimeActivationRequest{
		RuntimeID:        runtimeID,
		FactorySessionID: factorysessions.DefaultSessionID,
		Snapshot:         snapshot,
		Runtime:          opening.FactoryRuntime,
		Inputs:           runtimeActivationInputs(opening),
	}, nil
}

// populateActivationSnapshot resolves the detached value used for root
// validation and publication. Runtime opening still performs its canonical
// load during activation; this pre-resolution gives the root the same
// effective Factory identity and definition version without retaining the
// mutable loaded source.
func (f *Factory) populateActivationSnapshot(
	snapshot *factorydefinitions.RuntimeSnapshot,
	factoryDir string,
	runtimeBaseDir string,
) {
	if f == nil || snapshot == nil || f.loadFactory == nil {
		return
	}
	loaded, err := f.loadFactory(factoryDir, nil)
	if err != nil || loaded == nil {
		return
	}
	loaded.SetRuntimeBaseDir(runtimeBaseDir)
	config := loaded.FactoryConfig()
	if config == nil {
		return
	}
	cloned, err := factorydefinitions.CloneFactoryConfig(config)
	if err != nil || cloned == nil {
		return
	}
	snapshot.FactoryDir = firstNonEmpty(loaded.FactoryDir(), snapshot.FactoryDir)
	snapshot.RuntimeBaseDir = firstNonEmpty(loaded.RuntimeBaseDir(), snapshot.RuntimeBaseDir)
	snapshot.EffectiveFactory = *cloned
	snapshot.Workers = make([]factorydefinitions.FactoryWorkerConfig, len(cloned.Workers))
	for index, worker := range cloned.Workers {
		snapshot.Workers[index] = factorydefinitions.CloneWorkerConfig(worker)
	}
	snapshot.Workstations = make([]factorydefinitions.FactoryWorkstationConfig, len(cloned.Workstations))
	for index, workstation := range cloned.Workstations {
		snapshot.Workstations[index] = factorydefinitions.CloneWorkstationConfig(workstation)
	}
	if config.Version != nil {
		version := *config.Version
		snapshot.DefinitionVersion = &version
	}
}

func runtimeActivationInputs(request factorysessions.RuntimeOpeningRequest) factoryruntime.RuntimeActivationInputs {
	return factoryruntime.RuntimeActivationInputs{
		Definition: factoryruntime.RuntimeActivationDefinitionInputs{
			Directory:        request.FactoryDefinition.Directory,
			SourcePath:       request.FactoryDefinition.SourcePath,
			ExecutionBaseDir: request.FactoryDefinition.ExecutionBaseDir,
		},
		Session: factoryruntime.RuntimeActivationSessionInputs{
			PersistencePolicy: string(request.FactorySession.PersistencePolicy),
			BackendScopeID:    request.FactorySession.BackendScopeID,
			SystemConfigHome:  request.FactorySession.SystemConfigHome,
			SystemConfigPath:  request.FactorySession.SystemConfigPath,
			WorkFile:          request.FactorySession.WorkFile,
			Host: factoryruntime.RuntimeActivationHostInputs{
				Directory:   request.FactorySession.Host.Directory,
				RuntimeMode: request.FactorySession.Host.RuntimeMode,
				WorkFile:    request.FactorySession.Host.WorkFile,
				MockWorkers: request.FactorySession.Host.MockWorkers,
				Host:        request.FactorySession.Host.Host,
				Port:        request.FactorySession.Host.Port,
				AutoPort:    request.FactorySession.Host.AutoPort,
			},
		},
		Workers: factoryruntime.RuntimeActivationWorkerInputs{
			RunnerID:                          request.Workers.RunnerID,
			Worktree:                          request.Workers.Worktree,
			WorkerReasoningEffort:             request.Workers.WorkerReasoningEffort,
			MockWorkers:                       runtimeActivationMockWorkers(request.Workers.MockWorkers),
			InvocationSkipPermissionsOverride: request.Workers.InvocationSkipPermissionsOverride,
			SkipBuiltInPrerequisiteValidation: request.Workers.SkipBuiltInPrerequisiteValidation,
		},
		Recordings: factoryruntime.RuntimeActivationRecordingInputs{
			RecordPath:    request.Recordings.RecordPath,
			ReplayPath:    request.Recordings.ReplayPath,
			WorkflowID:    request.Recordings.WorkflowID,
			FlushInterval: request.Recordings.FlushInterval,
		},
		ModelCacheDirectory: request.ModelCacheDirectory,
		OperatorDefaults: factoryruntime.RuntimeActivationOperatorDefaults{
			WorkerModelProvider: request.OperatorDefaults.WorkerModelProvider,
			WorkerModel:         request.OperatorDefaults.WorkerModel,
			ConfigPath:          request.OperatorDefaults.ConfigPath,
		},
	}
}

func runtimeActivationMockWorkers(input *workers.MockWorkersConfig) *factoryruntime.RuntimeActivationMockWorkersConfig {
	if input == nil {
		return nil
	}
	output := &factoryruntime.RuntimeActivationMockWorkersConfig{
		UnmatchedDispatchPolicy: string(input.UnmatchedDispatchPolicy),
		MockWorkers:             make([]factoryruntime.RuntimeActivationMockWorker, len(input.MockWorkers)),
	}
	for index, worker := range input.MockWorkers {
		converted := factoryruntime.RuntimeActivationMockWorker{
			ID:              worker.ID,
			WorkerName:      worker.WorkerName,
			WorkstationName: worker.WorkstationName,
			RunType:         string(worker.RunType),
			WorkInputs:      make([]factoryruntime.RuntimeActivationMockWorkInput, len(worker.WorkInputs)),
		}
		for inputIndex, workInput := range worker.WorkInputs {
			converted.WorkInputs[inputIndex] = factoryruntime.RuntimeActivationMockWorkInput{
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
			converted.ScriptConfig = &factoryruntime.RuntimeActivationMockScript{
				Command:          worker.ScriptConfig.Command,
				Args:             append([]string(nil), worker.ScriptConfig.Args...),
				Env:              cloneStringMap(worker.ScriptConfig.Env),
				WorkingDirectory: worker.ScriptConfig.WorkingDirectory,
				Stdin:            worker.ScriptConfig.Stdin,
				Timeout:          worker.ScriptConfig.Timeout,
			}
		}
		if worker.RejectConfig != nil {
			converted.RejectConfig = &factoryruntime.RuntimeActivationMockReject{Stdout: worker.RejectConfig.Stdout, Stderr: worker.RejectConfig.Stderr}
			if worker.RejectConfig.ExitCode != nil {
				value := *worker.RejectConfig.ExitCode
				converted.RejectConfig.ExitCode = &value
			}
		}
		output.MockWorkers[index] = converted
	}
	return output
}

func cloneStringMap(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}
