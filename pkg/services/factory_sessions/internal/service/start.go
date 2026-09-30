package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// Start admits a live Factory Session through the process root's fixed
// collaborators. The selected runtime is published to the same Assembly that
// backs the root's session registry.
func (r *Root) Start(ctx context.Context, request factorysessions.SessionStartRequest) (factorysessions.SessionStartResult, error) {
	if r == nil || r.Assembly == nil {
		return factorysessions.SessionStartResult{}, fmt.Errorf("Factory Sessions process root is required")
	}
	if request.Mode != factorysessions.SessionOperationModeLive {
		if request.Mode == factorysessions.SessionOperationModeDurable && request.RuntimeSelection == nil {
			request = inheritCurrentMockWorkers(request, r.Resolve(factorysessions.DefaultSessionID))
		}
		return r.Assembly.Start(ctx, request)
	}
	if request.Wait.TimeoutMillis < 0 {
		return factorysessions.SessionStartResult{}, &factorysessions.DetachedRequestError{Field: "wait.timeoutMillis", Message: "timeout must not be negative"}
	}
	if request.ValidateOnly && request.InitNewFactory {
		return factorysessions.SessionStartResult{}, &factorysessions.DetachedRequestError{Field: "initNewFactory", Message: "initNewFactory cannot be combined with validateOnly"}
	}
	requestID := strings.TrimSpace(request.Correlation.RequestID)
	if request.ActivationOnly && requestID != "" {
		if existing, ok := r.startedForRequestID(requestID); ok {
			return existing, nil
		}
		value, err, _ := r.startFlights.Do(requestID, func() (any, error) {
			if existing, ok := r.startedForRequestID(requestID); ok {
				return existing, nil
			}
			return r.startLive(ctx, request)
		})
		if err != nil {
			return factorysessions.SessionStartResult{}, err
		}
		return value.(factorysessions.SessionStartResult), nil
	}
	return r.startLive(ctx, request)
}

func inheritCurrentMockWorkers(request factorysessions.SessionStartRequest, current *livesession.LiveSession) factorysessions.SessionStartRequest {
	if current == nil || strings.TrimSpace(request.FolderPath) != strings.TrimSpace(current.FactoryDir) {
		return request
	}
	bound := runtimebinding.SessionStateFrom(current)
	if bound == nil {
		return request
	}
	mockWorkers := bound.MockWorkersConfig()
	if mockWorkers == nil {
		return request
	}
	request.RuntimeSelection = &factorysessions.SessionRuntimeSelection{
		Workers: factorysessions.SessionWorkerSelection{MockWorkers: mockWorkers},
	}
	return request
}

func (r *Root) startLive(ctx context.Context, request factorysessions.SessionStartRequest) (factorysessions.SessionStartResult, error) {
	selectedID, err := r.sessionIDForStart(request)
	if err != nil {
		return factorysessions.SessionStartResult{}, err
	}
	previousControl, err := r.closeReplacedSession(ctx, request, selectedID)
	if err != nil {
		return factorysessions.SessionStartResult{}, err
	}
	selected, err := r.prepareLiveStartRequest(ctx, request, selectedID)
	if err != nil {
		return factorysessions.SessionStartResult{}, err
	}
	products, err := r.openForRequest(ctx, selected)
	if err != nil {
		return factorysessions.SessionStartResult{}, err
	}
	if request.ValidateOnly || request.InitNewFactory {
		if products.sessions == nil {
			return factorysessions.SessionStartResult{}, fmt.Errorf("start Factory Session: validation service is unavailable")
		}
		return products.sessions.Start(ctx, selected)
	}
	activation, err := startSessionLifecycle(ctx, products)
	if err != nil {
		return factorysessions.SessionStartResult{}, err
	}
	session := r.Resolve(selectedID)
	if session == nil {
		_ = activation.Close(ctx)
		_ = activation.lifecycle.StopLifecycle(ctx)
		return factorysessions.SessionStartResult{}, fmt.Errorf("start Factory Session: activated session %q is unavailable", selectedID)
	}
	bound := runtimebinding.SessionStateFrom(session)
	if bound == nil {
		_ = activation.Close(ctx)
		_ = activation.lifecycle.StopLifecycle(ctx)
		return factorysessions.SessionStartResult{}, fmt.Errorf("start Factory Session: session runtime state is unavailable")
	}
	bindSessionProducts(bound, products, activation, request.Correlation.RequestID, previousControl)
	bound.SetMockWorkers(selected.RuntimeSelection.Workers.MockWorkers)
	return liveStartResult(session), nil
}

func (r *Root) closeReplacedSession(ctx context.Context, request factorysessions.SessionStartRequest, selectedID string) (*runtimebinding.SessionState, error) {
	if !request.ActivationOnly || strings.TrimSpace(request.SessionID) == "" {
		return nil, nil
	}
	existing := r.Resolve(selectedID)
	if existing == nil {
		return nil, nil
	}
	bound := runtimebinding.SessionStateFrom(existing)
	if bound == nil || !bound.CanReplaceTerminatedSession() {
		return nil, fmt.Errorf("start Factory Session: session %q is already active", selectedID)
	}
	if _, err := r.Control(ctx, factorysessions.SessionControlRequest{
		SessionID: selectedID, Mode: factorysessions.SessionOperationModeLive,
		Operation: factorysessions.SessionControlClose,
	}); err != nil {
		return nil, fmt.Errorf("replace Factory Session %q: %w", selectedID, err)
	}
	return bound, nil
}

func (r *Root) prepareLiveStartRequest(ctx context.Context, request factorysessions.SessionStartRequest, selectedID string) (factorysessions.SessionStartRequest, error) {
	selected := request
	selected.SessionID = selectedID
	runtimeSelection := factorysessions.SessionRuntimeSelection{}
	if request.RuntimeSelection != nil {
		runtimeSelection = *request.RuntimeSelection
	}
	if strings.TrimSpace(runtimeSelection.SystemConfigHome) == "" && r.resolveHome != nil {
		home, homeErr := r.resolveHome()
		if homeErr != nil {
			return factorysessions.SessionStartRequest{}, fmt.Errorf("resolve Factory Sessions home: %w", homeErr)
		}
		runtimeSelection.SystemConfigHome = home
	}
	selectedFolder, resolveErr := r.resolveStartFolder(ctx, request, runtimeSelection)
	if resolveErr != nil {
		return factorysessions.SessionStartRequest{}, resolveErr
	}
	selected.FolderPath = selectedFolder
	if strings.TrimSpace(runtimeSelection.ExecutionBaseDir) == "" {
		runtimeSelection.ExecutionBaseDir = strings.TrimSpace(selected.FolderPath)
	}
	if runtimeSelection.LogPolicy == "" {
		runtimeSelection.LogPolicy = factorysessions.SessionArtifactPolicyDisabled
	}
	if runtimeSelection.MetricsPolicy == "" {
		runtimeSelection.MetricsPolicy = factorysessions.SessionArtifactPolicyDisabled
	}
	selected.RuntimeSelection = &runtimeSelection
	return selected, nil
}

func startSessionLifecycle(ctx context.Context, products runtimeProducts) (*sessionActivation, error) {
	if products.lifecycle == nil {
		if products.closeArtifacts != nil {
			_ = products.closeArtifacts()
		}
		return nil, fmt.Errorf("start Factory Session: lifecycle is unavailable")
	}
	runContext, cancel := context.WithCancel(context.WithoutCancel(ctx))
	activation := &sessionActivation{
		lifecycle:      products.lifecycle,
		cancel:         cancel,
		closeArtifacts: products.closeArtifacts,
	}
	err := activation.lifecycle.StartLifecycle(ctx, runContext)
	if err == nil {
		activation.stopWorker, err = activation.lifecycle.StartWorkerLifecycle(ctx)
		if err == nil {
			err = activation.lifecycle.CompleteStartup(ctx)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("start Factory Session lifecycle: %w", errors.Join(err, activation.Close(ctx), activation.lifecycle.StopLifecycle(ctx)))
	}
	return activation, nil
}

func bindSessionProducts(bound *runtimebinding.SessionState, products runtimeProducts, activation *sessionActivation, requestID string, previousControl *runtimebinding.SessionState) {
	bound.Activation = activation
	bound.Process = products.process
	bound.Diagnostics = products.diagnostics
	bound.ModelInvoker = products.modelInvoker
	bound.InputResolver = products.inputResolver
	bound.FactoryRuntime = products.factoryRuntime
	bound.ModelsScope = products.modelsScope
	bound.WorkerSessions = products.workerSessions
	bound.Logger = products.logger
	bound.Reader = products.reader
	bound.Projections = products.projections
	bound.Clock = products.clock
	bound.OperatorSettingsPath = products.operatorSettingsPath
	bound.Recordings = products.recordings
	bound.ReplayMetadataWarnings = append([]recordings.MetadataMismatchWarning(nil), products.replayMetadataWarnings...)
	bound.ResumeRecoveryMetadata = products.resumeRecoveryMetadata
	bound.OrderlyStop = products.orderlyStop
	bound.SetStartRequestID(strings.TrimSpace(requestID))
	bound.InheritTerminalControl(previousControl)
}

func liveStartResult(session *livesession.LiveSession) factorysessions.SessionStartResult {
	status := "RUNNING"
	view := factorysessions.SessionView{
		SessionID: livesession.CanonicalID(session), Mode: factorysessions.SessionOperationModeLive,
		Status: status, FactoryDir: session.FactoryDir, FolderPath: session.FolderPath,
		Project: session.Project, IsDefault: session.IsDefault, Target: session.Target,
		RuntimeAvailable: session.Runtime != nil,
	}
	return factorysessions.SessionStartResult{
		SessionID: view.SessionID, Mode: factorysessions.SessionOperationModeLive, Status: status,
		Live: &factorysessions.SessionOpenResult{SessionID: view.SessionID, Session: &view, FolderPath: session.FolderPath},
	}
}

func (r *Root) startedForRequestID(requestID string) (factorysessions.SessionStartResult, bool) {
	for _, id := range r.ListLiveSessionIDs() {
		session := r.Resolve(id)
		bound := runtimebinding.SessionStateFrom(session)
		if bound == nil || bound.StartRequestID() != requestID || bound.Activation == nil {
			continue
		}
		view := factorysessions.SessionView{
			SessionID: livesession.CanonicalID(session), Mode: factorysessions.SessionOperationModeLive,
			Status: "RUNNING", FactoryDir: session.FactoryDir, FolderPath: session.FolderPath,
			Project: session.Project, IsDefault: session.IsDefault, Target: session.Target,
			RuntimeAvailable: session.Runtime != nil,
		}
		return factorysessions.SessionStartResult{
			SessionID: view.SessionID, Mode: factorysessions.SessionOperationModeLive, Status: "RUNNING",
			Live: &factorysessions.SessionOpenResult{SessionID: view.SessionID, Session: &view, FolderPath: session.FolderPath},
		}, true
	}
	return factorysessions.SessionStartResult{}, false
}

func (r *Root) sessionIDForStart(request factorysessions.SessionStartRequest) (string, error) {
	if id := strings.TrimSpace(request.SessionID); id != "" {
		return id, nil
	}
	if !request.ActivationOnly {
		return factorysessions.DefaultSessionID, nil
	}
	if r.generateSessionID == nil {
		return "", fmt.Errorf("start Factory Session: session ID generator is required")
	}
	id := strings.TrimSpace(r.generateSessionID())
	if id == "" || id == factorysessions.DefaultSessionID {
		return "", fmt.Errorf("start Factory Session: session ID generator returned an invalid identity")
	}
	return id, nil
}

func (r *Root) resolveStartFolder(ctx context.Context, request factorysessions.SessionStartRequest, selection factorysessions.SessionRuntimeSelection) (string, error) {
	name := strings.TrimSpace(request.Source.FactoryID)
	if name == "" || request.Source.Kind != factoryruntime.WorkflowSourceKindFactoryID ||
		!strings.HasPrefix(name, "@") || strings.TrimSpace(selection.DefinitionSourcePath) != "" {
		return request.FolderPath, nil
	}
	workingRoot := strings.TrimSpace(request.FolderPath)
	if value, ok := request.Args["workingRoot"].(string); ok && strings.TrimSpace(value) != "" {
		workingRoot = strings.TrimSpace(value)
	}
	// An already resolved target carries its Factory directory in FolderPath.
	if strings.TrimSpace(request.FolderPath) != workingRoot {
		return request.FolderPath, nil
	}
	if r.factoryDefinitions == nil {
		return "", fmt.Errorf("resolve named Factory: Factory Definitions service is required")
	}
	roots, err := factorydefinitions.ResolveNamedFactoryRoots(selection.SystemConfigHome, workingRoot)
	if err != nil {
		return "", err
	}
	resolved, err := r.factoryDefinitions.ResolveNamedFactory(ctx, factorydefinitions.ResolveNamedFactoryRequest{
		ProjectRoot: roots.Project, GlobalRoot: roots.Global, Name: name,
	})
	if err != nil {
		return "", err
	}
	return resolved.Resolution.FactoryDir, nil
}

func runtimeSelectionForStart(request factorysessions.SessionStartRequest) factorysessions.SessionRuntimeSelection {
	if request.RuntimeSelection == nil {
		return factorysessions.SessionRuntimeSelection{}
	}
	return *request.RuntimeSelection
}

func definitionRequestForStart(request factorysessions.SessionStartRequest) factorydefinitions.RuntimeSelection {
	selection := runtimeSelectionForStart(request)
	return factorydefinitions.RuntimeSelection{
		Directory: strings.TrimSpace(request.FolderPath), SourcePath: selection.DefinitionSourcePath,
		InvocationArguments: work.CloneInvocationArguments(selection.DefinitionInvocationArguments),
		ExecutionBaseDir:    selection.ExecutionBaseDir,
	}
}

func runtimeOwnerRequestForStart(request factorysessions.SessionStartRequest) factoryruntime.RuntimeSelection {
	selection := runtimeSelectionForStart(request)
	mode := factorydefinitions.RuntimeMode(selection.Mode)
	if mode == "" {
		mode = factorydefinitions.RuntimeModeBatch
	}
	return factoryruntime.RuntimeSelection{
		Mode: mode, Verbose: selection.Verbose, RuntimeInstanceID: selection.RuntimeInstanceID,
		LogDirectory: selection.LogDirectory, FileLoggingPolicy: factoryruntime.RuntimeFileLoggingPolicy(selection.LogPolicy),
		LogConfig:        factoryruntime.RuntimeLogStorageConfig(selection.LogConfig),
		MetricsDirectory: selection.MetricsDirectory, MetricsPolicy: factoryruntime.RuntimeMetricsPolicy(selection.MetricsPolicy),
		MetricsConfig: factoryruntime.RuntimeMetricsStorageConfig(selection.MetricsConfig),
	}
}

func workerRequestForStart(request factorysessions.SessionStartRequest) workers.RuntimeSelection {
	selection := runtimeSelectionForStart(request)
	return workers.RuntimeSelection{
		RunnerID: selection.Workers.RunnerID, Worktree: selection.Workers.Worktree,
		WorkerReasoningEffort: selection.Workers.WorkerReasoningEffort, MockWorkers: selection.Workers.MockWorkers,
		InvocationSkipPermissionsOverride: selection.Workers.InvocationSkipPermissionsOverride,
		SkipBuiltInPrerequisiteValidation: selection.Workers.SkipBuiltInPrerequisiteValidation,
	}
}

func recordingRequestForStart(request factorysessions.SessionStartRequest) recordings.RuntimeSelection {
	selection := runtimeSelectionForStart(request)
	return recordings.RuntimeSelection{
		RecordPath: selection.Recording.RecordPath, ReplayPath: selection.Recording.ReplayPath,
		ResumePath: selection.Recording.ResumePath, WorkflowID: selection.Recording.WorkflowID,
		FlushInterval: selection.Recording.FlushInterval,
	}
}
