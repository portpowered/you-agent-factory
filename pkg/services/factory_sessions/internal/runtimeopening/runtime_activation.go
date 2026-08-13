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
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/logicaltarget"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
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
	products, err := f.openRuntimeWithSnapshot(ctx, openingRequest, f.baseLogger, &request.Snapshot)
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
	return f.openActivatedRuntimeWithReplayInput(ctx, request, nil)
}

func (f *Factory) openActivatedRuntimeWithReplayInput(
	ctx context.Context,
	request *factorysessions.RuntimeOpeningRequest,
	preloadedReplayInput *recordings.LoadReplayInputResult,
) (runtimeProducts, error) {
	if f == nil || f.runtimeRoot == nil {
		return runtimeProducts{}, fmt.Errorf("open Factory Runtime: Runtime root is required")
	}
	activationRequest, err := f.activationRequestWithReplayInput(ctx, request, preloadedReplayInput)
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
	ctx context.Context,
	request *factorysessions.RuntimeOpeningRequest,
) (factoryruntime.RuntimeActivationRequest, error) {
	return f.activationRequestWithReplayInput(ctx, request, nil)
}

func (f *Factory) activationRequestWithReplayInput(
	ctx context.Context,
	request *factorysessions.RuntimeOpeningRequest,
	preloadedReplayInput *recordings.LoadReplayInputResult,
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
	var (
		factoryDir       string
		sourcePath       string
		runtimeBaseDir   string
		snapshot         factorydefinitions.RuntimeSnapshot
		snapshotResolved bool
		err              error
	)
	sessionID := sessionIDForOpening(opening)
	if replaySnapshot, ok, err := f.resolveLegacyReplaySnapshot(ctx, opening, preloadedReplayInput); err != nil {
		return factoryruntime.RuntimeActivationRequest{}, err
	} else if ok {
		snapshot = replaySnapshot
		factoryDir = strings.TrimSpace(snapshot.FactoryDir)
		runtimeBaseDir = strings.TrimSpace(opening.FactoryDefinition.ExecutionBaseDir)
		if runtimeBaseDir == "" {
			runtimeBaseDir = strings.TrimSpace(snapshot.RuntimeBaseDir)
		}
		snapshotResolved = true
	}
	if !snapshotResolved {
		factoryDir, sourcePath, err = f.resolveActivationDefinitionSource(opening)
		if err != nil {
			return factoryruntime.RuntimeActivationRequest{}, err
		}
		if factoryDir == "" && sourcePath == "" {
			return factoryruntime.RuntimeActivationRequest{}, fmt.Errorf("open Factory Runtime: Factory Definition directory is required")
		}
		runtimeBaseDir = strings.TrimSpace(opening.FactoryDefinition.ExecutionBaseDir)
		if runtimeBaseDir == "" {
			runtimeBaseDir = factoryDir
		}
		if runtimeBaseDir == "" {
			runtimeBaseDir = sourcePath
		}
	}
	name := filepath.Base(filepath.Clean(factoryDir))
	if name == "." || name == string(filepath.Separator) || name == "" {
		name = "runtime"
	}
	if !snapshotResolved {
		if f.factoryDefinitions == nil {
			return factoryruntime.RuntimeActivationRequest{}, runtimeSnapshotResolverUnavailable()
		}
		resolved, resolveErr := f.factoryDefinitions.ResolveRuntimeSnapshot(ctx, factorydefinitions.ResolveRuntimeSnapshotRequest{
			// SourcePath is the concrete authored file for direct layouts. Do not
			// send the retained directory as FactoryDir as the Definitions root
			// rejects two distinct source identities.
			FactoryDir:       "",
			SourcePath:       sourcePath,
			ExecutionBaseDir: runtimeBaseDir,
			Invocation: factorydefinitions.RuntimeSnapshotInvocationContext{
				FactorySessionID: sessionID,
				WorkflowID:       opening.Recordings.WorkflowID,
			},
		})
		if resolveErr != nil {
			return factoryruntime.RuntimeActivationRequest{}, resolveErr
		}
		snapshot, err = factorydefinitions.CloneRuntimeSnapshot(resolved.Snapshot)
		if err != nil {
			return factoryruntime.RuntimeActivationRequest{}, fmt.Errorf("open Factory Runtime: detach resolved Factory Definition snapshot: %w", err)
		}
	}
	if strings.TrimSpace(snapshot.FactoryDir) == "" {
		snapshot.FactoryDir = factoryDir
		if strings.TrimSpace(snapshot.FactoryDir) == "" && strings.TrimSpace(sourcePath) != "" {
			snapshot.FactoryDir = filepath.Dir(sourcePath)
		}
	}
	if strings.TrimSpace(snapshot.RuntimeBaseDir) == "" {
		snapshot.RuntimeBaseDir = runtimeBaseDir
	}
	if snapshot.EffectiveFactory.Name == "" {
		snapshot.EffectiveFactory.Name = name
	}
	snapshot.Invocation.FactorySessionID = sessionID
	if workflowID := strings.TrimSpace(opening.Recordings.WorkflowID); workflowID != "" {
		snapshot.Invocation.WorkflowID = workflowID
	}
	if snapshot.DefinitionVersion == nil {
		// Older authored Factory files predate persisted version metadata. Keep
		// their established opening behavior while making the version explicit
		// in the activation request.
		snapshot.DefinitionVersion = &factorydefinitions.FactoryVersion{Logical: 1}
	}
	inputs := runtimeActivationInputs(opening)
	// Runtime root activation must receive the same resolved source identity
	// that Definitions used. In particular, named paths and directory-backed
	// authored files cannot be rediscovered from the caller's shorthand after
	// the snapshot has crossed the boundary.
	if factoryDir != "" {
		inputs.Definition.Directory = factoryDir
	}
	if sourcePath != "" {
		inputs.Definition.SourcePath = sourcePath
	}
	return factoryruntime.RuntimeActivationRequest{
		RuntimeID:        runtimeID,
		FactorySessionID: sessionID,
		Snapshot:         snapshot,
		Runtime:          opening.FactoryRuntime,
		Inputs:           inputs,
	}, nil
}

func sessionIDForOpening(opening factorysessions.RuntimeOpeningRequest) string {
	if sessionID := strings.TrimSpace(opening.FactorySession.FactorySessionID); sessionID != "" {
		return sessionID
	}
	return factorysessions.DefaultSessionID
}

func (f *Factory) resolveLegacyReplaySnapshot(
	ctx context.Context,
	opening factorysessions.RuntimeOpeningRequest,
	preloadedReplayInput *recordings.LoadReplayInputResult,
) (factorydefinitions.RuntimeSnapshot, bool, error) {
	if strings.TrimSpace(opening.Recordings.ReplayPath) == "" ||
		(preloadedReplayInput == nil && f.replayInputs == nil) {
		return factorydefinitions.RuntimeSnapshot{}, false, nil
	}
	var input recordings.LoadReplayInputResult
	if preloadedReplayInput != nil {
		input = *preloadedReplayInput
	} else {
		loaded, err := f.replayInputs.LoadReplayInput(
			recordings.LoadReplayInputRequest{Path: opening.Recordings.ReplayPath},
		)
		if err != nil {
			return factorydefinitions.RuntimeSnapshot{}, false, fmt.Errorf("open Factory Runtime: load replay input for activation: %w", err)
		}
		input = loaded
	}
	if input.Portable != nil || input.Legacy == nil || input.Legacy.Factory == nil {
		return factorydefinitions.RuntimeSnapshot{}, false, nil
	}
	if f.factoryDefinitions == nil {
		return factorydefinitions.RuntimeSnapshot{}, false, runtimeSnapshotResolverUnavailable()
	}
	if f.decodeReplayConfig == nil {
		return factorydefinitions.RuntimeSnapshot{}, false, fmt.Errorf("open Factory Runtime: replay Factory Definition decoder is required")
	}
	replayConfig, err := f.decodeReplayConfig(input.Legacy.Factory)
	if err != nil {
		return factorydefinitions.RuntimeSnapshot{}, false, fmt.Errorf("open Factory Runtime: decode replay Factory Definition: %w", err)
	}
	if replayConfig == nil {
		return factorydefinitions.RuntimeSnapshot{}, false, fmt.Errorf("open Factory Runtime: replay Factory Definition is empty")
	}
	factoryDir := strings.TrimSpace(replayConfig.FactoryDir())
	runtimeBaseDir := strings.TrimSpace(opening.FactoryDefinition.ExecutionBaseDir)
	if runtimeBaseDir == "" {
		runtimeBaseDir = strings.TrimSpace(replayConfig.RuntimeBaseDir())
	}
	if factoryDir == "" {
		factoryDir = runtimeBaseDir
	}
	if factoryDir == "" {
		factoryDir = "."
	}
	if runtimeBaseDir == "" {
		runtimeBaseDir = factoryDir
	}
	resolved, err := f.factoryDefinitions.ResolveRuntimeSnapshot(ctx, factorydefinitions.ResolveRuntimeSnapshotRequest{
		Canonical:        append([]byte(nil), []byte(*input.Legacy.Factory)...),
		ExecutionBaseDir: runtimeBaseDir,
		Invocation: factorydefinitions.RuntimeSnapshotInvocationContext{
			FactorySessionID: sessionIDForOpening(opening),
			WorkflowID:       opening.Recordings.WorkflowID,
		},
	})
	if err != nil {
		return factorydefinitions.RuntimeSnapshot{}, false, err
	}
	snapshot, err := factorydefinitions.CloneRuntimeSnapshot(resolved.Snapshot)
	if err != nil {
		return factorydefinitions.RuntimeSnapshot{}, false, fmt.Errorf("open Factory Runtime: detach replay Factory Definition snapshot: %w", err)
	}
	if strings.TrimSpace(snapshot.FactoryDir) == "" {
		snapshot.FactoryDir = factoryDir
	}
	if strings.TrimSpace(snapshot.RuntimeBaseDir) == "" {
		snapshot.RuntimeBaseDir = runtimeBaseDir
	}
	return snapshot, true, nil
}

func (f *Factory) resolveActivationDefinitionSource(
	opening factorysessions.RuntimeOpeningRequest,
) (string, string, error) {
	definition := opening.FactoryDefinition
	if strings.TrimSpace(definition.SourcePath) != "" {
		sourcePath := strings.TrimSpace(definition.SourcePath)
		if !strings.HasPrefix(sourcePath, "~") && f.resolveHome == nil {
			return "", sourcePath, nil
		}
		resolved, err := absolutizeActivationPath(sourcePath, f.resolveHome)
		if err != nil {
			return "", "", fmt.Errorf("open Factory Runtime: resolve Factory source: %w", err)
		}
		return "", resolved, nil
	}
	factoryDir := strings.TrimSpace(definition.Directory)
	if factoryDir == "" {
		return "", "", nil
	}
	if f.namedPaths != nil {
		resolved, err := f.namedPaths.ResolveCurrentDir(factoryDir)
		if err != nil {
			return "", "", fmt.Errorf("open Factory Runtime: resolve Factory directory: %w", err)
		}
		factoryDir = resolved
	}
	resolved, err := absolutizeActivationPath(factoryDir, f.resolveHome)
	if err != nil {
		return "", "", fmt.Errorf("open Factory Runtime: resolve Factory directory: %w", err)
	}
	// The authored loader treats a directory as a split layout and therefore
	// requires body-only worker definitions. Opening has historically accepted
	// a direct factory.json with topology-only workers (including mock-worker
	// runs), so resolve the concrete source file while retaining the directory
	// as the snapshot's Factory identity.
	return resolved, filepath.Join(resolved, factorydefinitions.FactoryConfigFile), nil
}

func absolutizeActivationPath(
	path string,
	resolveHome factorysessions.HomeDirectoryResolver,
) (string, error) {
	if resolveHome == nil && path != "~" && !strings.HasPrefix(path, "~/") && !strings.HasPrefix(path, `~\`) {
		resolved, err := filepath.Abs(path)
		if err != nil {
			return "", fmt.Errorf("resolve Factory directory %q: %w", path, err)
		}
		return filepath.Clean(resolved), nil
	}
	return logicaltarget.AbsolutizeFactoryDirectory(path, resolveHome)
}

func runtimeSnapshotResolverUnavailable() error {
	return &factorydefinitions.RuntimeSnapshotResolutionError{
		Diagnostic: factorydefinitions.RuntimeSnapshotDiagnostic{
			Code:    factorydefinitions.RuntimeSnapshotDiagnosticUnavailable,
			Field:   "resolver",
			Message: "Factory Definitions runtime snapshot resolver is unavailable",
		},
		Cause: factorydefinitions.ErrRuntimeSnapshotResolverUnavailable,
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
