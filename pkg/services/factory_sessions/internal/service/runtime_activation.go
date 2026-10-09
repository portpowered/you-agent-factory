package service

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/recordingreplay"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func (r *Root) activateRuntime(
	ctx context.Context,
	request factoryruntime.RuntimeActivationRequest,
) (roles.LifecycleRuntime, *recordingreplay.Scope, func() error, *factoryruntime.RuntimeActivation, roles.ApplicationRuntime, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, nil, nil, nil, err
	}
	definition, err := definitionRequestFromActivation(request)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	session := sessionRequestFromActivation(request)
	worker := workerRequestFromActivation(request.Inputs.Workers)
	recording := recordingRuntimeSelection(request)
	defaults := operatorsettings.ResolvedDefaults{
		WorkerModelProvider: request.Inputs.OperatorDefaults.WorkerModelProvider,
		WorkerModel:         request.Inputs.OperatorDefaults.WorkerModel,
		ConfigPath:          request.Inputs.OperatorDefaults.ConfigPath,
	}
	canonicalSessionIDProvided := strings.TrimSpace(session.RuntimeSelection.CanonicalSessionID) != ""
	if err := ensureDefaultCanonicalSessionID(&session, recording.ReplayPath, r.canonicalSessionIDGenerator()); err != nil {
		return nil, nil, nil, nil, nil, err
	}
	canonicalSessionIDGenerated := !canonicalSessionIDProvided &&
		strings.TrimSpace(session.RuntimeSelection.CanonicalSessionID) != ""
	// Runtime Root validates the caller context before it invokes this
	// activation operation. A generated canonical identity is allocated at the
	// session-product boundary for compatibility with the historical session-ID
	// edge, which may itself cancel the caller context. Once the activation has
	// been admitted, finish constructing it atomically even if that allocation
	// cancels the caller. Cancellation already present on entry still returns
	// above.
	openingContext := ctx
	if canonicalSessionIDGenerated && ctx.Err() != nil {
		openingContext = context.WithoutCancel(ctx)
	}
	return r.opening.openRuntimeWithOptions(openingContext, definition, request.Runtime, &session, canonicalSessionIDGenerated, worker, recording, request.Inputs.ModelCacheDirectory, defaults, r.baseLogger, &request.Snapshot, nil)
}

// newRuntimeActivation retains the acquired activation's declared handles and
// replaces its cleanup with the session acquisition owner's cleanup. It never
// rediscovers Work/event ingress from the engine or a current-session resolver.
func newRuntimeActivation(opened *factoryruntime.RuntimeActivation, closeArtifacts func() error) (*factoryruntime.RuntimeActivation, error) {
	activation := &factoryruntime.RuntimeActivation{
		Close: func(context.Context) error {
			if closeArtifacts == nil {
				return nil
			}
			return closeArtifacts()
		},
	}
	if opened == nil || opened.Service == nil {
		return activation, fmt.Errorf("activate Factory Runtime: opened Runtime engine service is required")
	}
	if opened.WorkAndEventIngress == nil {
		return activation, fmt.Errorf(
			"activate Factory Runtime: opened runtime Work submission and event subscription are required until Recordings migration",
		)
	}
	activation.Service = opened.Service
	activation.WorkAndEventIngress = opened.WorkAndEventIngress
	return activation, nil
}

func definitionRequestFromActivation(request factoryruntime.RuntimeActivationRequest) (factorydefinitions.RuntimeSelection, error) {
	definitionDirectory := strings.TrimSpace(request.Inputs.Definition.Directory)
	if definitionDirectory == "" {
		definitionDirectory = request.Snapshot.FactoryDir
	}
	executionBaseDir := strings.TrimSpace(request.Inputs.Definition.ExecutionBaseDir)
	if executionBaseDir == "" {
		executionBaseDir = request.Snapshot.RuntimeBaseDir
	}
	if definitionDirectory == "" {
		return factorydefinitions.RuntimeSelection{}, fmt.Errorf("runtime activation inputs: Factory Definition directory is required")
	}
	return definitionRuntimeSelection(definitionDirectory, request.Inputs.Definition.SourcePath, executionBaseDir, request.Snapshot.Invocation.Arguments), nil
}

func sessionRequestFromActivation(request factoryruntime.RuntimeActivationRequest) factorysessions.SessionStartRequest {
	return factorysessions.SessionStartRequest{
		SessionID:   request.FactorySessionID,
		Mode:        factorysessions.SessionOperationModeLive,
		Persistence: factorysessions.PersistencePolicy(request.Inputs.Session.PersistencePolicy),
		RuntimeSelection: &factorysessions.SessionRuntimeSelection{
			Mode: factorysessions.SessionRuntimeMode(request.Runtime.Mode),
			Recording: factorysessions.SessionRecordingSelection{
				ImplicitCurrentBoard: request.Inputs.Session.ImplicitCurrentBoard,
				RecordPath:           request.Inputs.Recordings.RecordPath,
			},
			CanonicalSessionID: request.Inputs.Session.CanonicalSessionID,
			BackendScopeID:     request.Inputs.Session.BackendScopeID,
			SystemConfigHome:   request.Inputs.Session.SystemConfigHome,
			SystemConfigPath:   request.Inputs.Session.SystemConfigPath,
			WorkFile:           request.Inputs.Session.WorkFile,
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
	}
}

func workerRequestFromActivation(input factoryruntime.RuntimeActivationWorkerInputs) workers.RuntimeSelection {
	return workers.RuntimeSelection{
		RunnerID:                          input.RunnerID,
		Worktree:                          input.Worktree,
		WorkerReasoningEffort:             input.WorkerReasoningEffort,
		MockWorkers:                       activationMockWorkers(input.MockWorkers),
		InvocationSkipPermissionsOverride: input.InvocationSkipPermissionsOverride,
		SkipBuiltInPrerequisiteValidation: input.SkipBuiltInPrerequisiteValidation,
	}
}

func definitionRuntimeSelection(
	directory, sourcePath, executionBaseDir string,
	invocationArguments *work.InvocationArguments,
) factorydefinitions.RuntimeSelection {
	return factorydefinitions.RuntimeSelection{
		Directory:           directory,
		SourcePath:          sourcePath,
		ExecutionBaseDir:    executionBaseDir,
		InvocationArguments: work.CloneInvocationArguments(invocationArguments),
	}
}

func recordingRuntimeSelection(request factoryruntime.RuntimeActivationRequest) recordings.RuntimeSelection {
	return recordings.RuntimeSelection{
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

func (r *Root) openActivatedRuntime(
	ctx context.Context,
	request factorysessions.SessionStartRequest,
) (roles.LifecycleRuntime, *recordingreplay.Scope, func() error, error) {
	return r.openActivatedRuntimeWithInputs(ctx, request, nil, nil)
}

func (r *Root) openActivatedRuntimeWithReplayInput(
	ctx context.Context,
	request factorysessions.SessionStartRequest,
	preloadedReplayInput *recordings.LoadReplayInputResult,
) (roles.LifecycleRuntime, *recordingreplay.Scope, func() error, error) {
	return r.openActivatedRuntimeWithInputs(ctx, request, preloadedReplayInput, nil)
}

func (r *Root) openActivatedRuntimeWithResumeInput(
	ctx context.Context,
	request factorysessions.SessionStartRequest,
	resumeInput *recordings.LoadResumeInputResult,
) (roles.LifecycleRuntime, *recordingreplay.Scope, func() error, error) {
	return r.openActivatedRuntimeWithInputs(ctx, request, nil, resumeInput)
}

func (r *Root) openActivatedRuntimeWithInputs(
	ctx context.Context,
	request factorysessions.SessionStartRequest,
	preloadedReplayInput *recordings.LoadReplayInputResult,
	resumeInput *recordings.LoadResumeInputResult,
) (roles.LifecycleRuntime, *recordingreplay.Scope, func() error, error) {
	if r == nil || r.runtimeRoot == nil {
		return nil, nil, nil, fmt.Errorf("open Factory Runtime: Runtime root is required")
	}
	activationRequest, err := r.activationRequestWithInputs(ctx, request, preloadedReplayInput, resumeInput)
	if err != nil {
		return nil, nil, nil, err
	}
	var lifecycle roles.LifecycleRuntime
	var replay *recordingreplay.Scope
	var selectedRuntime roles.ApplicationRuntime
	result, err := r.runtimeRoot.Activate(ctx, activationRequest, func(activationCtx context.Context, activation factoryruntime.RuntimeActivationRequest) (*factoryruntime.RuntimeActivation, error) {
		openedLifecycle, openedReplay, _, published, selected, openErr := r.activateRuntime(activationCtx, activation)
		if openErr != nil {
			return published, openErr
		}
		lifecycle = openedLifecycle
		replay = openedReplay
		selectedRuntime = selected
		return published, nil
	})
	if err != nil {
		return nil, nil, nil, err
	}
	binding := result.Binding
	if binding.IsZero() {
		binding = result.Runtime.Binding
	}
	closeRuntime := activationCloser(r.runtimeRoot, binding, result.RuntimeID)
	if !binding.IsZero() {
		if err := r.opening.openingBinding.PublishRuntime(selectedRuntime, activationRequest.FactorySessionID, binding); err != nil {
			return nil, nil, nil, runtimeBindingPublicationError(err, closeRuntime())
		}
	}
	return lifecycle, replay, closeRuntime, nil
}

// activationCloser retains the selected Runtime owner and opaque binding. A
// later Root selection must not retarget cleanup of this acquisition.
func activationCloser(runtimeRoot FactoryRuntimeRoot, binding factoryruntime.RuntimeBinding, runtimeID string) func() error {
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
			_, err = runtimeRoot.Deactivate(
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

func (r *Root) activationRequest(
	ctx context.Context,
	request factorysessions.SessionStartRequest,
) (factoryruntime.RuntimeActivationRequest, error) {
	return r.activationRequestWithInputs(ctx, request, nil, nil)
}

func (r *Root) activationRequestWithInputs(
	ctx context.Context,
	request factorysessions.SessionStartRequest,
	preloadedReplayInput *recordings.LoadReplayInputResult,
	resumeInput *recordings.LoadResumeInputResult,
) (factoryruntime.RuntimeActivationRequest, error) {
	definition := definitionRequestForStart(request)
	runtimeRequest := runtimeOwnerRequestForStart(request)
	session := request
	worker := workerRequestForStart(request)
	recording := recordingRequestForStart(request)
	selection := runtimeSelectionForStart(request)
	runtimeID, err := r.ensureActivationRuntimeID(&runtimeRequest)
	if err != nil {
		return factoryruntime.RuntimeActivationRequest{}, err
	}
	sessionID := sessionIDForSelection(session)
	resolution, err := r.snapshotSelection.Resolve(
		ctx,
		definition,
		recording,
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
		recording.WorkflowID,
	)
	inputs := runtimeActivationInputs(
		definition,
		session,
		false,
		worker,
		recording,
		selection.ModelCacheDirectory,
		selection.OperatorDefaults,
		resumeInput,
	)
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
		Runtime:          runtimeRequest,
		Inputs:           inputs,
	}, nil
}

type activationSnapshotResolution struct {
	snapshot       factorydefinitions.RuntimeSnapshot
	factoryDir     string
	sourcePath     string
	runtimeBaseDir string
}

func (r *Root) ensureActivationRuntimeID(runtime *factoryruntime.RuntimeSelection) (string, error) {
	if runtime == nil {
		return "", fmt.Errorf("activate Factory Runtime: runtime selection is required")
	}
	runtimeID := strings.TrimSpace(runtime.RuntimeInstanceID)
	if runtimeID != "" {
		return runtimeID, nil
	}
	if r.generateRuntimeInstanceID == nil {
		return "", fmt.Errorf("open Factory Runtime: runtime instance ID generator is required")
	}
	runtimeID = strings.TrimSpace(r.generateRuntimeInstanceID())
	if runtimeID == "" {
		return "", fmt.Errorf("open Factory Runtime: runtime instance ID generator returned an empty identity")
	}
	runtime.RuntimeInstanceID = runtimeID
	return runtimeID, nil
}

func (r *Root) canonicalSessionIDGenerator() func() string {
	if r == nil {
		return nil
	}
	if r.generateSessionID != nil {
		return r.generateSessionID
	}
	// Keep direct internal callers compatible while the process graph adopts
	// the dedicated Factory Session identity edge.
	return r.generateRuntimeInstanceID
}

func ensureDefaultCanonicalSessionID(
	session *factorysessions.SessionStartRequest,
	replayPath string,
	generateID func() string,
) error {
	if session == nil || strings.TrimSpace(replayPath) != "" {
		return nil
	}
	sessionID := strings.TrimSpace(session.SessionID)
	if sessionID == "" {
		sessionID = factorysessions.DefaultSessionID
	}
	selection := sessionRuntimeSelection(session)
	if sessionID != factorysessions.DefaultSessionID || strings.TrimSpace(selection.CanonicalSessionID) != "" {
		return nil
	}
	if generateID == nil {
		return fmt.Errorf("open Factory Session: canonical session ID generator is required")
	}
	canonicalID := strings.TrimSpace(generateID())
	if canonicalID == "" {
		return fmt.Errorf("open Factory Session: canonical session ID generator returned an empty identity")
	}
	selection.CanonicalSessionID = canonicalID
	return nil
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

func sessionIDForSelection(session factorysessions.SessionStartRequest) string {
	if sessionID := strings.TrimSpace(session.SessionID); sessionID != "" {
		return sessionID
	}
	return factorysessions.DefaultSessionID
}

func runtimeActivationInputs(
	definition factorydefinitions.RuntimeSelection,
	session factorysessions.SessionStartRequest,
	canonicalSessionIDGenerated bool,
	worker workers.RuntimeSelection,
	recording recordings.RuntimeSelection,
	modelCacheDirectory string,
	operatorDefaults operatorsettings.ResolvedDefaults,
	resumeInput *recordings.LoadResumeInputResult,
) factoryruntime.RuntimeActivationInputs {
	selection := runtimeSelectionForStart(session)
	inputs := factoryruntime.RuntimeActivationInputs{
		Definition: factoryruntime.RuntimeActivationDefinitionInputs{
			Directory:        definition.Directory,
			SourcePath:       definition.SourcePath,
			ExecutionBaseDir: definition.ExecutionBaseDir,
		},
		Session: factoryruntime.RuntimeActivationSessionInputs{
			ImplicitCurrentBoard:        selection.Recording.ImplicitCurrentBoard,
			CanonicalSessionID:          selection.CanonicalSessionID,
			CanonicalSessionIDGenerated: canonicalSessionIDGenerated,
			PersistencePolicy:           string(session.Persistence),
			BackendScopeID:              selection.BackendScopeID,
			SystemConfigHome:            selection.SystemConfigHome,
			SystemConfigPath:            selection.SystemConfigPath,
			WorkFile:                    selection.WorkFile,
			Host: factoryruntime.RuntimeActivationHostInputs{
				Directory:   selection.Host.Directory,
				RuntimeMode: selection.Host.RuntimeMode,
				WorkFile:    selection.Host.WorkFile,
				MockWorkers: selection.Host.MockWorkers,
				Host:        selection.Host.Host,
				Port:        selection.Host.Port,
				AutoPort:    selection.Host.AutoPort,
				Pprof:       selection.Host.Pprof,
			},
		},
		Workers: factoryruntime.RuntimeActivationWorkerInputs{
			RunnerID:                          worker.RunnerID,
			Worktree:                          worker.Worktree,
			WorkerReasoningEffort:             worker.WorkerReasoningEffort,
			MockWorkers:                       runtimeActivationMockWorkers(worker.MockWorkers),
			InvocationSkipPermissionsOverride: worker.InvocationSkipPermissionsOverride,
			SkipBuiltInPrerequisiteValidation: worker.SkipBuiltInPrerequisiteValidation,
		},
		Recordings: factoryruntime.RuntimeActivationRecordingInputs{
			RecordPath:    recording.RecordPath,
			ReplayPath:    recording.ReplayPath,
			ResumePath:    recording.ResumePath,
			WorkflowID:    recording.WorkflowID,
			FlushInterval: recording.FlushInterval,
		},
		ModelCacheDirectory: modelCacheDirectory,
		OperatorDefaults: factoryruntime.RuntimeActivationOperatorDefaults{
			WorkerModelProvider: operatorDefaults.WorkerModelProvider,
			WorkerModel:         operatorDefaults.WorkerModel,
			ConfigPath:          operatorDefaults.ConfigPath,
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

// RuntimeInitialEngine shapes detached selected facts through fixed capabilities.
// It retains no invocation state or acquired resources.
type RuntimeInitialEngine struct {
	selectSnapshot initialEngineSnapshotSelection
	activate       factoryruntime.InitialRuntimeActivationOperation
}

type initialEngineSnapshotSelection func(context.Context, factorydefinitions.RuntimeSelection,
	recordings.RuntimeSelection, *recordings.LoadReplayInputResult, *recordings.LoadResumeInputResult,
	string) (activationSnapshotResolution, error)

type initialEngineLiveRequest struct {
	Configured       preparedRuntime
	EffectiveFactory factorydefinitions.FactoryConfig
	Workers          []factorydefinitions.FactoryWorkerConfig
	Workstations     []factorydefinitions.FactoryWorkstationConfig
	ResumeInput      *recordings.LoadResumeInputResult
	Recovery         factoryruntime.RuntimeActivationRecoveryInput
}

// NewRuntimeInitialEngine stores direct collaborators without executing them.
func NewRuntimeInitialEngine(selectSnapshot initialEngineSnapshotSelection,
	activate factoryruntime.InitialRuntimeActivationOperation) *RuntimeInitialEngine {
	return &RuntimeInitialEngine{selectSnapshot: selectSnapshot, activate: activate}
}

func (engine *RuntimeInitialEngine) OpenLive(ctx context.Context, request initialEngineLiveRequest,
	observations factoryruntime.SessionObservations) (*factoryruntime.RuntimeInitialOpening, error) {
	configured := request.Configured
	selected := configured.DefinitionSnapshot
	if selected == nil {
		resolved, err := engine.selectSnapshot(ctx, configured.Definition, configured.Recordings,
			nil, request.ResumeInput, configured.Session.SessionID)
		if err != nil {
			return nil, err
		}
		selected = &resolved.snapshot
	}
	snapshot, err := selected.Clone()
	if err != nil {
		return nil, err
	}
	config, err := factorydefinitions.CloneFactoryConfig(&request.EffectiveFactory)
	if err != nil {
		return nil, err
	}
	snapshot.EffectiveFactory = *config
	for index := range snapshot.Workers {
		for _, worker := range request.Workers {
			if worker.Name == snapshot.Workers[index].Name {
				snapshot.Workers[index] = factorydefinitions.CloneWorkerConfig(worker)
				break
			}
		}
	}
	for index := range snapshot.Workstations {
		for _, workstation := range request.Workstations {
			if workstation.Name == snapshot.Workstations[index].Name {
				snapshot.Workstations[index] = factorydefinitions.CloneWorkstationConfig(workstation)
				break
			}
		}
	}
	inputs := runtimeActivationInputs(configured.Definition, configured.Session,
		configured.CanonicalSessionIDGenerated, configured.Workers, configured.Recordings,
		configured.ModelCacheDirectory, configured.OperatorDefaults, request.ResumeInput)
	inputs.RecoveryInput = request.Recovery
	return engine.activate(ctx, factoryruntime.RuntimeActivationRequest{
		RuntimeID: configured.Runtime.RuntimeInstanceID, FactorySessionID: configured.Session.SessionID,
		Snapshot: snapshot, Runtime: configured.Runtime, Inputs: inputs,
	}, observations)
}

func (engine *RuntimeInitialEngine) OpenCheckpoint(ctx context.Context, configured preparedRuntime,
	observations factoryruntime.SessionObservations) (*factoryruntime.RuntimeInitialOpening, error) {
	// Checkpoint continuation selects the authored definition after the caller's
	// successful restoration probe, with no replay selection or normalization.
	resolved, err := engine.selectSnapshot(ctx, configured.Definition, recordings.RuntimeSelection{},
		nil, nil, configured.Session.SessionID)
	if err != nil {
		return nil, err
	}
	snapshot, err := resolved.snapshot.Clone()
	if err != nil {
		return nil, err
	}
	inputs := runtimeActivationInputs(configured.Definition, configured.Session,
		configured.CanonicalSessionIDGenerated, configured.Workers, configured.Recordings,
		configured.ModelCacheDirectory, configured.OperatorDefaults, nil)
	inputs.Session.CanonicalSessionID = configured.Session.SessionID
	inputs.RecoveryInput.CheckpointContinuation = true
	opening, err := engine.activate(ctx, factoryruntime.RuntimeActivationRequest{
		RuntimeID: configured.Runtime.RuntimeInstanceID, FactorySessionID: configured.Session.SessionID,
		Snapshot: snapshot, Runtime: configured.Runtime, Inputs: inputs,
	}, observations)
	if err != nil {
		return opening, fmt.Errorf("construct portable replay runtime: %w", err)
	}
	if opening == nil || opening.Record == nil {
		return opening, fmt.Errorf("construct portable replay runtime: runtime instance is required")
	}
	return opening, nil
}
