package runtimeopening

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/portpowered/infinite-you/pkg/initializer/lifecycle"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/logicaltarget"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// activatedRuntimeService is the private handoff stored by the Runtime root.
// The embedded service is the completed Factory Sessions runtime; the product
// roles remain private to this opener and are exposed only through the same
// activation result that published the service. The wrapper deliberately does
// not carry the widened Work and event operations: those are published as the
// activation's declared ingress so no peer recovers them from this type.
type activatedRuntimeService struct {
	factoryruntime.Service
	products runtimeProducts
}

// RuntimeDelegate exposes only the concrete Runtime service to the owning
// Factory Runtime root. The wrapper still carries opening products for the
// Factory Sessions handoff, but widened Work and event operations must never
// resolve through that wrapper again.
func (s *activatedRuntimeService) RuntimeDelegate() factoryruntime.Service {
	if s == nil {
		return nil
	}
	return s.Service
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
	return newRuntimeActivation(products)
}

// newRuntimeActivation publishes the opened engine as the Runtime activation.
// It resolves the migration-only Work and event ingress once here, at
// construction, and hands it to the Runtime root as a declared activation
// value so no later Work submission or event subscription has to recover a
// legacy owner from the published service.
func newRuntimeActivation(products runtimeProducts) (*factoryruntime.RuntimeActivation, error) {
	service := runtimeEngineService(products)
	if service == nil {
		return nil, fmt.Errorf("activate Factory Runtime: opened Runtime engine service is required")
	}
	ingress, ok := service.(factoryruntime.APIFactory)
	if !ok {
		return nil, fmt.Errorf(
			"activate Factory Runtime: opened runtime Work submission and event subscription are required until Recordings migration",
		)
	}
	return &factoryruntime.RuntimeActivation{
		Service: &activatedRuntimeService{
			Service:  service,
			products: products,
		},
		WorkAndEventIngress: ingress,
		Close: func(closeCtx context.Context) error {
			if products.application.Resources.Close == nil {
				return nil
			}
			return products.application.Resources.Close()
		},
	}, nil
}

// runtimeEngineService returns the live engine the opening resolved from the
// Runtime instance. The application HTTP view is intentionally not used here:
// during the migration it is allowed to be a Factory Sessions resolver proxy,
// and publishing that proxy as the binding would re-enter the same session
// lookup for every Work or Worker operation.
func runtimeEngineService(products runtimeProducts) factoryruntime.Service {
	return products.engine
}

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
			request.Snapshot.Invocation.Arguments,
		),
		FactoryRuntime: request.Runtime,
		FactorySession: factorysessions.SessionRuntimeOpeningRequest{
			FactorySessionID:            request.FactorySessionID,
			CanonicalSessionID:          request.Inputs.Session.CanonicalSessionID,
			CanonicalSessionIDGenerated: request.Inputs.Session.CanonicalSessionIDGenerated,
			PersistencePolicy:           factorysessions.PersistencePolicy(request.Inputs.Session.PersistencePolicy),
			BackendScopeID:              request.Inputs.Session.BackendScopeID,
			SystemConfigHome:            request.Inputs.Session.SystemConfigHome,
			SystemConfigPath:            request.Inputs.Session.SystemConfigPath,
			WorkFile:                    request.Inputs.Session.WorkFile,
			Host: factorysessions.RuntimeHostRequest{
				Directory:   request.Inputs.Session.Host.Directory,
				RuntimeMode: request.Inputs.Session.Host.RuntimeMode,
				WorkFile:    request.Inputs.Session.Host.WorkFile,
				MockWorkers: request.Inputs.Session.Host.MockWorkers,
				Host:        request.Inputs.Session.Host.Host,
				Port:        request.Inputs.Session.Host.Port,
				AutoPort:    request.Inputs.Session.Host.AutoPort,
				Pprof:       request.Inputs.Session.Host.Pprof,
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

func factorydefinitionsRuntimeOpeningRequest(
	directory, sourcePath, executionBaseDir string,
	invocationArguments *work.InvocationArguments,
) factorydefinitions.RuntimeOpeningRequest {
	return factorydefinitions.RuntimeOpeningRequest{
		Directory:           directory,
		SourcePath:          sourcePath,
		ExecutionBaseDir:    executionBaseDir,
		InvocationArguments: work.CloneInvocationArguments(invocationArguments),
	}
}

func recordingsRuntimeOpeningRequest(request factoryruntime.RuntimeActivationRequest) recordings.RuntimeOpeningRequest {
	return recordings.RuntimeOpeningRequest{
		RecordPath:    request.Inputs.Recordings.RecordPath,
		ReplayPath:    request.Inputs.Recordings.ReplayPath,
		ResumePath:    request.Inputs.Recordings.ResumePath,
		ResumeInput:   request.Inputs.ResumeInput,
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

func (f *Factory) openActivatedRuntime(
	ctx context.Context,
	request *factorysessions.RuntimeOpeningRequest,
) (runtimeProducts, error) {
	return f.openActivatedRuntimeWithInputs(ctx, request, nil, nil)
}

func (f *Factory) openActivatedRuntimeWithReplayInput(
	ctx context.Context,
	request *factorysessions.RuntimeOpeningRequest,
	preloadedReplayInput *recordings.LoadReplayInputResult,
) (runtimeProducts, error) {
	return f.openActivatedRuntimeWithInputs(ctx, request, preloadedReplayInput, nil)
}

func (f *Factory) openActivatedRuntimeWithResumeInput(
	ctx context.Context,
	request *factorysessions.RuntimeOpeningRequest,
	resumeInput *recordings.LoadResumeInputResult,
) (runtimeProducts, error) {
	return f.openActivatedRuntimeWithInputs(ctx, request, nil, resumeInput)
}

func (f *Factory) openActivatedRuntimeWithInputs(
	ctx context.Context,
	request *factorysessions.RuntimeOpeningRequest,
	preloadedReplayInput *recordings.LoadReplayInputResult,
	resumeInput *recordings.LoadResumeInputResult,
) (runtimeProducts, error) {
	if f == nil || f.runtimeRoot == nil {
		return runtimeProducts{}, fmt.Errorf("open Factory Runtime: Runtime root is required")
	}
	activationRequest, err := f.activationRequestWithInputs(ctx, request, preloadedReplayInput, resumeInput)
	if err != nil {
		return runtimeProducts{}, err
	}
	result, err := f.runtimeRoot.Activate(ctx, activationRequest)
	if err != nil {
		return runtimeProducts{}, err
	}
	binding := result.Binding
	if binding.IsZero() {
		binding = result.Runtime.Binding
	}
	handoff, ok := result.Runtime.Service.(*activatedRuntimeService)
	if !ok || handoff == nil {
		if cleanupErr := f.activationCloser(binding, result.RuntimeID)(); cleanupErr != nil {
			return runtimeProducts{}, fmt.Errorf("open Factory Runtime: activation handoff is unavailable; cleanup: %w", cleanupErr)
		}
		return runtimeProducts{}, fmt.Errorf("open Factory Runtime: activation handoff is unavailable")
	}
	products := handoff.runtimeProducts()
	closeRuntime := f.activationCloser(binding, result.RuntimeID)
	if !binding.IsZero() && products.bindRuntime != nil {
		if err := products.bindRuntime(binding); err != nil {
			return runtimeProducts{}, runtimeBindingPublicationError(err, closeRuntime())
		}
	}
	if binding.Service() != nil {
		products.application.FactoryRuntime = binding.Service()
	} else {
		products.application.FactoryRuntime = f.runtimeRoot
	}
	products.application.Resources.Close = closeRuntime
	products.invocation.CloseArtifacts = closeRuntime
	products.execution.Resources.Close = closeRuntime
	return products, nil
}

func (f *Factory) activationCloser(binding factoryruntime.RuntimeBinding, runtimeID string) func() error {
	var mu sync.Mutex
	closed := false
	return func() error {
		mu.Lock()
		defer mu.Unlock()
		if closed {
			return nil
		}
		var err error
		if !binding.IsZero() {
			_, err = binding.Deactivate(context.Background())
		} else {
			_, err = f.runtimeRoot.Deactivate(
				context.Background(),
				factoryruntime.RuntimeDeactivationRequest{RuntimeID: runtimeID},
			)
		}
		if errors.Is(err, factoryruntime.ErrRuntimeNotActive) {
			err = nil
		}
		if err == nil {
			closed = true
		}
		return err
	}
}

func runtimeBindingPublicationError(bindErr, cleanupErr error) error {
	if cleanupErr == nil {
		return fmt.Errorf("open Factory Runtime: publish Runtime binding to Factory Session: %w", bindErr)
	}
	return fmt.Errorf(
		"open Factory Runtime: publish Runtime binding to Factory Session: %w",
		errors.Join(bindErr, fmt.Errorf("cleanup activated Runtime: %w", cleanupErr)),
	)
}

func (f *Factory) activationRequest(
	ctx context.Context,
	request *factorysessions.RuntimeOpeningRequest,
) (factoryruntime.RuntimeActivationRequest, error) {
	return f.activationRequestWithInputs(ctx, request, nil, nil)
}

func (f *Factory) activationRequestWithInputs(
	ctx context.Context,
	request *factorysessions.RuntimeOpeningRequest,
	preloadedReplayInput *recordings.LoadReplayInputResult,
	resumeInput *recordings.LoadResumeInputResult,
) (factoryruntime.RuntimeActivationRequest, error) {
	opening, runtimeID, err := f.activationOpening(request)
	if err != nil {
		return factoryruntime.RuntimeActivationRequest{}, err
	}
	sessionID := sessionIDForOpening(opening)
	resolution, err := f.resolveActivationSnapshot(
		ctx,
		opening,
		preloadedReplayInput,
		resumeInput,
		sessionID,
	)
	if err != nil {
		return factoryruntime.RuntimeActivationRequest{}, err
	}
	normalizeActivationSnapshot(
		&resolution.snapshot,
		resolution.factoryDir,
		resolution.sourcePath,
		resolution.runtimeBaseDir,
		sessionID,
		opening.Recordings.WorkflowID,
	)
	canonicalSessionIDProvided := strings.TrimSpace(opening.FactorySession.CanonicalSessionID) != ""
	if err := ensureDefaultCanonicalSessionID(&opening, f.canonicalSessionIDGenerator()); err != nil {
		return factoryruntime.RuntimeActivationRequest{}, err
	}
	opening.FactorySession.CanonicalSessionIDGenerated = !canonicalSessionIDProvided &&
		strings.TrimSpace(opening.FactorySession.CanonicalSessionID) != ""
	inputs := runtimeActivationInputs(opening, resumeInput)
	// Runtime root activation must receive the same resolved source identity
	// that Definitions used. In particular, named paths and directory-backed
	// authored files cannot be rediscovered from the caller's shorthand after
	// the snapshot has crossed the boundary. Keep the caller's Directory as
	// the session/factory-root scope; SourcePath carries the concrete source
	// identity used to resolve the snapshot.
	if resolution.sourcePath != "" {
		inputs.Definition.SourcePath = resolution.sourcePath
	}
	return factoryruntime.RuntimeActivationRequest{
		RuntimeID:        runtimeID,
		FactorySessionID: sessionID,
		Snapshot:         resolution.snapshot,
		Runtime:          opening.FactoryRuntime,
		Inputs:           inputs,
	}, nil
}

type activationSnapshotResolution struct {
	snapshot       factorydefinitions.RuntimeSnapshot
	factoryDir     string
	sourcePath     string
	runtimeBaseDir string
}

func (f *Factory) activationOpening(
	request *factorysessions.RuntimeOpeningRequest,
) (factorysessions.RuntimeOpeningRequest, string, error) {
	if request == nil {
		return factorysessions.RuntimeOpeningRequest{}, "", fmt.Errorf("open Factory Runtime: runtime opening request is required")
	}
	opening := *request
	runtimeID := strings.TrimSpace(opening.FactoryRuntime.RuntimeInstanceID)
	if runtimeID != "" {
		return opening, runtimeID, nil
	}
	if f.generateRuntimeInstanceID == nil {
		return factorysessions.RuntimeOpeningRequest{}, "", fmt.Errorf("open Factory Runtime: runtime instance ID generator is required")
	}
	runtimeID = strings.TrimSpace(f.generateRuntimeInstanceID())
	if runtimeID == "" {
		return factorysessions.RuntimeOpeningRequest{}, "", fmt.Errorf("open Factory Runtime: runtime instance ID generator returned an empty identity")
	}
	opening.FactoryRuntime.RuntimeInstanceID = runtimeID
	return opening, runtimeID, nil
}

func (f *Factory) canonicalSessionIDGenerator() func() string {
	if f == nil {
		return nil
	}
	if f.generateSessionID != nil {
		return f.generateSessionID
	}
	// Keep direct internal callers compatible while the process graph adopts
	// the dedicated Factory Session identity edge.
	return f.generateRuntimeInstanceID
}

func ensureDefaultCanonicalSessionID(
	request *factorysessions.RuntimeOpeningRequest,
	generateID func() string,
) error {
	if request == nil || strings.TrimSpace(request.Recordings.ReplayPath) != "" {
		return nil
	}
	sessionID := strings.TrimSpace(request.FactorySession.FactorySessionID)
	if sessionID == "" {
		sessionID = factorysessions.DefaultSessionID
	}
	if sessionID != factorysessions.DefaultSessionID || strings.TrimSpace(request.FactorySession.CanonicalSessionID) != "" {
		return nil
	}
	if generateID == nil {
		return fmt.Errorf("open Factory Session: canonical session ID generator is required")
	}
	canonicalID := strings.TrimSpace(generateID())
	if canonicalID == "" {
		return fmt.Errorf("open Factory Session: canonical session ID generator returned an empty identity")
	}
	request.FactorySession.CanonicalSessionID = canonicalID
	return nil
}

func (f *Factory) resolveActivationSnapshot(
	ctx context.Context,
	opening factorysessions.RuntimeOpeningRequest,
	preloadedReplayInput *recordings.LoadReplayInputResult,
	resumeInput *recordings.LoadResumeInputResult,
	sessionID string,
) (activationSnapshotResolution, error) {
	if replaySnapshot, ok, err := f.resolveLegacyReplaySnapshot(ctx, opening, preloadedReplayInput); err != nil {
		return activationSnapshotResolution{}, err
	} else if ok {
		return activationSnapshotResolution{
			snapshot:       replaySnapshot,
			factoryDir:     strings.TrimSpace(replaySnapshot.FactoryDir),
			runtimeBaseDir: firstNonEmptyString(opening.FactoryDefinition.ExecutionBaseDir, replaySnapshot.RuntimeBaseDir),
		}, nil
	}
	if resumeSnapshot, ok, err := f.resolveLegacyResumeSnapshot(ctx, opening, resumeInput); err != nil {
		return activationSnapshotResolution{}, err
	} else if ok {
		return activationSnapshotResolution{
			snapshot:       resumeSnapshot,
			factoryDir:     strings.TrimSpace(resumeSnapshot.FactoryDir),
			runtimeBaseDir: firstNonEmptyString(opening.FactoryDefinition.ExecutionBaseDir, resumeSnapshot.RuntimeBaseDir),
		}, nil
	}
	factoryDir, sourcePath, err := f.resolveActivationDefinitionSource(opening)
	if err != nil {
		return activationSnapshotResolution{}, err
	}
	if factoryDir == "" && sourcePath == "" {
		return activationSnapshotResolution{}, fmt.Errorf("open Factory Runtime: Factory Definition directory is required")
	}
	runtimeBaseDir := firstNonEmptyString(opening.FactoryDefinition.ExecutionBaseDir, factoryDir, sourcePath)
	snapshot, err := f.resolveActivationDefinitionSnapshot(ctx, sourcePath, runtimeBaseDir, opening, sessionID)
	if err != nil {
		return activationSnapshotResolution{}, err
	}
	return activationSnapshotResolution{
		snapshot:       snapshot,
		factoryDir:     factoryDir,
		sourcePath:     sourcePath,
		runtimeBaseDir: runtimeBaseDir,
	}, nil
}

func (f *Factory) resolveActivationDefinitionSnapshot(
	ctx context.Context,
	sourcePath, runtimeBaseDir string,
	opening factorysessions.RuntimeOpeningRequest,
	sessionID string,
) (factorydefinitions.RuntimeSnapshot, error) {
	if f.factoryDefinitions == nil {
		return factorydefinitions.RuntimeSnapshot{}, runtimeSnapshotResolverUnavailable()
	}
	resolved, err := f.factoryDefinitions.ResolveRuntimeSnapshot(ctx, factorydefinitions.ResolveRuntimeSnapshotRequest{
		// SourcePath is the concrete authored file for direct layouts. Do not
		// send the retained directory as FactoryDir as the Definitions root
		// rejects two distinct source identities.
		SourcePath:       sourcePath,
		ExecutionBaseDir: runtimeBaseDir,
		Invocation: factorydefinitions.RuntimeSnapshotInvocationContext{
			FactorySessionID: sessionID,
			WorkflowID:       opening.Recordings.WorkflowID,
			Arguments:        work.CloneInvocationArguments(opening.FactoryDefinition.InvocationArguments),
		},
	})
	if err != nil {
		return factorydefinitions.RuntimeSnapshot{}, err
	}
	snapshot, err := resolved.Snapshot.Clone()
	if err != nil {
		return factorydefinitions.RuntimeSnapshot{}, fmt.Errorf("open Factory Runtime: detach resolved Factory Definition snapshot: %w", err)
	}
	return snapshot, nil
}

func normalizeActivationSnapshot(
	snapshot *factorydefinitions.RuntimeSnapshot,
	factoryDir, sourcePath, runtimeBaseDir, sessionID, workflowID string,
) {
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
		snapshot.EffectiveFactory.Name = runtimeFactoryName(factoryDir)
	}
	snapshot.Invocation.FactorySessionID = sessionID
	if workflowID = strings.TrimSpace(workflowID); workflowID != "" {
		snapshot.Invocation.WorkflowID = workflowID
	}
	if snapshot.DefinitionVersion == nil {
		snapshot.DefinitionVersion = &factorydefinitions.FactoryVersion{Logical: 1}
	}
}

func runtimeFactoryName(factoryDir string) string {
	name := filepath.Base(filepath.Clean(factoryDir))
	if name == "." || name == string(filepath.Separator) || name == "" {
		return "runtime"
	}
	return name
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
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
	if !legacyReplayRequested(opening, preloadedReplayInput, f.replayInputs != nil) {
		return factorydefinitions.RuntimeSnapshot{}, false, nil
	}
	input, err := f.loadReplayInputForActivation(opening, preloadedReplayInput)
	if err != nil {
		return factorydefinitions.RuntimeSnapshot{}, false, err
	}
	if input.Portable != nil || input.Legacy == nil || input.Legacy.Factory == nil {
		return factorydefinitions.RuntimeSnapshot{}, false, nil
	}
	snapshot, err := f.resolveLegacyFactorySnapshot(ctx, opening, input.Legacy.Factory, "replay")
	if err != nil {
		return factorydefinitions.RuntimeSnapshot{}, false, err
	}
	return snapshot, true, nil
}

func (f *Factory) resolveLegacyResumeSnapshot(
	ctx context.Context,
	opening factorysessions.RuntimeOpeningRequest,
	resumeInput *recordings.LoadResumeInputResult,
) (factorydefinitions.RuntimeSnapshot, bool, error) {
	if resumeInput == nil {
		return factorydefinitions.RuntimeSnapshot{}, false, nil
	}
	input := resumeInput.Input
	if input.Portable != nil || input.Legacy == nil || input.Legacy.Factory == nil {
		return factorydefinitions.RuntimeSnapshot{}, false, fmt.Errorf(
			"open Factory Runtime: resume recording Factory Definition is required",
		)
	}
	snapshot, err := f.resolveLegacyFactorySnapshot(ctx, opening, input.Legacy.Factory, "resume")
	if err != nil {
		return factorydefinitions.RuntimeSnapshot{}, false, err
	}
	return snapshot, true, nil
}

func (f *Factory) resolveLegacyFactorySnapshot(
	ctx context.Context,
	opening factorysessions.RuntimeOpeningRequest,
	factoryJSON *factorydefinitions.FactorySnapshot,
	intent string,
) (factorydefinitions.RuntimeSnapshot, error) {
	if f.factoryDefinitions == nil {
		return factorydefinitions.RuntimeSnapshot{}, runtimeSnapshotResolverUnavailable()
	}
	replayConfig, err := f.decodeLegacyReplayConfig(factoryJSON)
	if err != nil {
		return factorydefinitions.RuntimeSnapshot{}, err
	}
	factoryDir, runtimeBaseDir := legacyReplayPaths(opening, replayConfig)
	resolved, err := f.factoryDefinitions.ResolveRuntimeSnapshot(ctx, factorydefinitions.ResolveRuntimeSnapshotRequest{
		Canonical:        append([]byte(nil), []byte(*factoryJSON)...),
		ExecutionBaseDir: runtimeBaseDir,
		Invocation: factorydefinitions.RuntimeSnapshotInvocationContext{
			FactorySessionID: sessionIDForOpening(opening),
			WorkflowID:       opening.Recordings.WorkflowID,
		},
	})
	if err != nil {
		return factorydefinitions.RuntimeSnapshot{}, err
	}
	snapshot, err := resolved.Snapshot.Clone()
	if err != nil {
		return factorydefinitions.RuntimeSnapshot{}, fmt.Errorf(
			"open Factory Runtime: detach %s Factory Definition snapshot: %w",
			intent,
			err,
		)
	}
	if strings.TrimSpace(snapshot.FactoryDir) == "" {
		snapshot.FactoryDir = factoryDir
	}
	if strings.TrimSpace(snapshot.RuntimeBaseDir) == "" {
		snapshot.RuntimeBaseDir = runtimeBaseDir
	}
	return snapshot, nil
}

func legacyReplayRequested(
	opening factorysessions.RuntimeOpeningRequest,
	preloadedReplayInput *recordings.LoadReplayInputResult,
	replayInputsAvailable bool,
) bool {
	return strings.TrimSpace(opening.Recordings.ReplayPath) != "" && (preloadedReplayInput != nil || replayInputsAvailable)
}

func (f *Factory) loadReplayInputForActivation(
	opening factorysessions.RuntimeOpeningRequest,
	preloadedReplayInput *recordings.LoadReplayInputResult,
) (recordings.LoadReplayInputResult, error) {
	if preloadedReplayInput != nil {
		return *preloadedReplayInput, nil
	}
	loaded, err := f.replayInputs.LoadReplayInput(
		recordings.LoadReplayInputRequest{Path: opening.Recordings.ReplayPath},
	)
	if err != nil {
		return recordings.LoadReplayInputResult{}, fmt.Errorf("open Factory Runtime: load replay input for activation: %w", err)
	}
	return loaded, nil
}

func (f *Factory) decodeLegacyReplayConfig(
	factoryJSON *factorydefinitions.FactorySnapshot,
) (factorydefinitions.ReplayRuntimeConfig, error) {
	if f.factoryDefinitions == nil {
		return nil, runtimeSnapshotResolverUnavailable()
	}
	if f.decodeReplayConfig == nil {
		return nil, fmt.Errorf("open Factory Runtime: replay Factory Definition decoder is required")
	}
	replayConfig, err := f.decodeReplayConfig(factoryJSON)
	if err != nil {
		return nil, fmt.Errorf("open Factory Runtime: decode replay Factory Definition: %w", err)
	}
	if replayConfig == nil {
		return nil, fmt.Errorf("open Factory Runtime: replay Factory Definition is empty")
	}
	return replayConfig, nil
}

func legacyReplayPaths(
	opening factorysessions.RuntimeOpeningRequest,
	replayConfig factorydefinitions.ReplayRuntimeConfig,
) (string, string) {
	factoryDir := strings.TrimSpace(replayConfig.FactoryDir())
	runtimeBaseDir := firstNonEmptyString(opening.FactoryDefinition.ExecutionBaseDir, replayConfig.RuntimeBaseDir())
	factoryDir = firstNonEmptyString(factoryDir, runtimeBaseDir, ".")
	runtimeBaseDir = firstNonEmptyString(runtimeBaseDir, factoryDir)
	return factoryDir, runtimeBaseDir
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

func runtimeActivationInputs(
	request factorysessions.RuntimeOpeningRequest,
	resumeInput *recordings.LoadResumeInputResult,
) factoryruntime.RuntimeActivationInputs {
	inputs := factoryruntime.RuntimeActivationInputs{
		Definition: factoryruntime.RuntimeActivationDefinitionInputs{
			Directory:        request.FactoryDefinition.Directory,
			SourcePath:       request.FactoryDefinition.SourcePath,
			ExecutionBaseDir: request.FactoryDefinition.ExecutionBaseDir,
		},
		Session: factoryruntime.RuntimeActivationSessionInputs{
			CanonicalSessionID:          request.FactorySession.CanonicalSessionID,
			CanonicalSessionIDGenerated: request.FactorySession.CanonicalSessionIDGenerated,
			PersistencePolicy:           string(request.FactorySession.PersistencePolicy),
			BackendScopeID:              request.FactorySession.BackendScopeID,
			SystemConfigHome:            request.FactorySession.SystemConfigHome,
			SystemConfigPath:            request.FactorySession.SystemConfigPath,
			WorkFile:                    request.FactorySession.WorkFile,
			Host: factoryruntime.RuntimeActivationHostInputs{
				Directory:   request.FactorySession.Host.Directory,
				RuntimeMode: request.FactorySession.Host.RuntimeMode,
				WorkFile:    request.FactorySession.Host.WorkFile,
				MockWorkers: request.FactorySession.Host.MockWorkers,
				Host:        request.FactorySession.Host.Host,
				Port:        request.FactorySession.Host.Port,
				AutoPort:    request.FactorySession.Host.AutoPort,
				Pprof:       request.FactorySession.Host.Pprof,
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
			ResumePath:    request.Recordings.ResumePath,
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
	if resumeInput != nil {
		inputs.ResumeInput = *resumeInput
	}
	return inputs
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
		if worker.GateConfig != nil {
			converted.GateConfig = &factoryruntime.RuntimeActivationMockGate{
				ArrivedFile: worker.GateConfig.ArrivedFile,
				ReleaseFile: worker.GateConfig.ReleaseFile,
				Timeout:     worker.GateConfig.Timeout,
			}
		}
		if worker.Usage != nil {
			converted.Usage = &factoryruntime.RuntimeActivationMockUsage{
				Provider:              worker.Usage.Provider,
				Model:                 worker.Usage.Model,
				InputTokens:           cloneInt64Pointer(worker.Usage.InputTokens),
				OutputTokens:          cloneInt64Pointer(worker.Usage.OutputTokens),
				CachedInputTokens:     cloneInt64Pointer(worker.Usage.CachedInputTokens),
				ReasoningOutputTokens: cloneInt64Pointer(worker.Usage.ReasoningOutputTokens),
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

func cloneInt64Pointer(value *int64) *int64 {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

// newOrderlyRecordingFlush adapts the already-composed Recordings root to the
// initializer lifecycle boundary. A missing record path means that this
// runtime has no live recording to flush, so the orderly-stop phase remains a
// no-op.
func newOrderlyRecordingFlush(
	service recordings.Service,
	recordingID string,
	recordPath string,
) lifecycle.OrderlyStopOperation {
	if service == nil || strings.TrimSpace(recordingID) == "" || strings.TrimSpace(recordPath) == "" {
		return nil
	}
	return func(ctx context.Context) error {
		if ctx != nil {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		if _, err := service.FlushRecording(recordings.FlushRecordingRequest{
			RecordingID: recordings.RecordingID(recordingID),
		}); err != nil {
			return fmt.Errorf("flush live recording during orderly shutdown: %w", err)
		}
		return nil
	}
}
