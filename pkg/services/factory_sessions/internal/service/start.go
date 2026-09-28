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

func (r *Root) startLive(ctx context.Context, request factorysessions.SessionStartRequest) (factorysessions.SessionStartResult, error) {
	selected := request
	selectedID, err := r.sessionIDForStart(request)
	if err != nil {
		return factorysessions.SessionStartResult{}, err
	}
	var previousControl *runtimebinding.SessionState
	if request.ActivationOnly && strings.TrimSpace(request.SessionID) != "" {
		if existing := r.Resolve(selectedID); existing != nil {
			bound := runtimebinding.SessionStateFrom(existing)
			if bound == nil || !bound.CanReplaceTerminatedSession() {
				return factorysessions.SessionStartResult{}, fmt.Errorf("start Factory Session: session %q is already active", selectedID)
			}
			if _, err := r.Control(ctx, factorysessions.SessionControlRequest{
				SessionID: selectedID, Mode: factorysessions.SessionOperationModeLive,
				Operation: factorysessions.SessionControlClose,
			}); err != nil {
				return factorysessions.SessionStartResult{}, fmt.Errorf("replace Factory Session %q: %w", selectedID, err)
			}
			previousControl = bound
		}
	}
	selected.SessionID = selectedID
	runtimeSelection := factorysessions.SessionRuntimeSelection{}
	if request.RuntimeSelection != nil {
		runtimeSelection = *request.RuntimeSelection
	}
	if strings.TrimSpace(runtimeSelection.SystemConfigHome) == "" && r.resolveHome != nil {
		home, homeErr := r.resolveHome()
		if homeErr != nil {
			return factorysessions.SessionStartResult{}, fmt.Errorf("resolve Factory Sessions home: %w", homeErr)
		}
		runtimeSelection.SystemConfigHome = home
	}
	selectedFolder, resolveErr := r.resolveStartFolder(ctx, request, runtimeSelection)
	if resolveErr != nil {
		return factorysessions.SessionStartResult{}, resolveErr
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
	opening, err := runtimeRequestForStart(selected)
	if err != nil {
		return factorysessions.SessionStartResult{}, err
	}
	products, err := r.openForRequest(ctx, &opening)
	if err != nil {
		return factorysessions.SessionStartResult{}, err
	}
	if request.ValidateOnly || request.InitNewFactory {
		if products.application.FactorySessions == nil {
			return factorysessions.SessionStartResult{}, fmt.Errorf("start Factory Session: validation service is unavailable")
		}
		return products.application.FactorySessions.Start(ctx, selected)
	}
	if products.invocation.Lifecycle == nil {
		if products.application.Resources.Close != nil {
			_ = products.application.Resources.Close()
		}
		return factorysessions.SessionStartResult{}, fmt.Errorf("start Factory Session: lifecycle is unavailable")
	}
	runContext, cancel := context.WithCancel(context.WithoutCancel(ctx))
	activation := &sessionActivation{
		lifecycle:      products.invocation.Lifecycle,
		cancel:         cancel,
		closeArtifacts: products.application.Resources.Close,
	}
	if err := activation.lifecycle.StartLifecycle(ctx, runContext); err == nil {
		activation.stopWorker, err = activation.lifecycle.StartWorkerLifecycle(ctx)
		if err == nil {
			err = activation.lifecycle.CompleteStartup(ctx)
		}
	}
	if err != nil {
		return factorysessions.SessionStartResult{}, fmt.Errorf("start Factory Session lifecycle: %w", errors.Join(err, activation.Close(ctx), activation.lifecycle.StopLifecycle(ctx)))
	}
	sessionID := selectedID
	session := r.Resolve(sessionID)
	if session == nil {
		_ = activation.Close(ctx)
		_ = activation.lifecycle.StopLifecycle(ctx)
		return factorysessions.SessionStartResult{}, fmt.Errorf("start Factory Session: activated session %q is unavailable", sessionID)
	}
	bound := runtimebinding.SessionStateFrom(session)
	if bound == nil {
		_ = activation.Close(ctx)
		_ = activation.lifecycle.StopLifecycle(ctx)
		return factorysessions.SessionStartResult{}, fmt.Errorf("start Factory Session: session runtime state is unavailable")
	}
	bound.Activation = activation
	bound.Process = products.application.Process
	bound.Diagnostics = products.application.Resources.Diagnostics
	bound.SetStartRequestID(strings.TrimSpace(request.Correlation.RequestID))
	bound.InheritTerminalControl(previousControl)
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
	}, nil
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

func runtimeRequestForStart(request factorysessions.SessionStartRequest) (factorysessions.RuntimeOpeningRequest, error) {
	folder := strings.TrimSpace(request.FolderPath)
	if folder == "" {
		return factorysessions.RuntimeOpeningRequest{}, &factorysessions.DetachedRequestError{Field: "folderPath", Message: "folder path is required"}
	}
	selection := request.RuntimeSelection
	if selection == nil {
		selection = &factorysessions.SessionRuntimeSelection{}
	}
	mode := factorydefinitions.RuntimeMode(selection.Mode)
	if mode == "" {
		mode = factorydefinitions.RuntimeModeBatch
	}
	return factorysessions.RuntimeOpeningRequest{
		FactoryDefinition: factorydefinitions.RuntimeOpeningRequest{
			Directory: folder, SourcePath: selection.DefinitionSourcePath,
			ExecutionBaseDir: selection.ExecutionBaseDir,
		},
		FactoryRuntime: factoryruntime.RuntimeOpeningRequest{
			Mode: mode, Verbose: selection.Verbose, RuntimeInstanceID: selection.RuntimeInstanceID,
			LogDirectory: selection.LogDirectory, FileLoggingPolicy: factoryruntime.RuntimeFileLoggingPolicy(selection.LogPolicy),
			LogConfig:        factoryruntime.RuntimeLogStorageConfig(selection.LogConfig),
			MetricsDirectory: selection.MetricsDirectory, MetricsPolicy: factoryruntime.RuntimeMetricsPolicy(selection.MetricsPolicy),
			MetricsConfig: factoryruntime.RuntimeMetricsStorageConfig(selection.MetricsConfig),
		},
		FactorySession: factorysessions.SessionRuntimeOpeningRequest{
			FactorySessionID: request.SessionID, CanonicalSessionID: selection.CanonicalSessionID,
			PersistencePolicy: request.Persistence, BackendScopeID: selection.BackendScopeID,
			SystemConfigHome: selection.SystemConfigHome, SystemConfigPath: selection.SystemConfigPath,
			WorkFile: selection.WorkFile, Host: selection.Host,
		},
		Workers: workers.RuntimeOpeningRequest{
			RunnerID: selection.Workers.RunnerID, Worktree: selection.Workers.Worktree,
			WorkerReasoningEffort: selection.Workers.WorkerReasoningEffort, MockWorkers: selection.Workers.MockWorkers,
			InvocationSkipPermissionsOverride: selection.Workers.InvocationSkipPermissionsOverride,
			SkipBuiltInPrerequisiteValidation: selection.Workers.SkipBuiltInPrerequisiteValidation,
		},
		Recordings: recordings.RuntimeOpeningRequest{
			RecordPath: selection.Recording.RecordPath, ReplayPath: selection.Recording.ReplayPath,
			ResumePath: selection.Recording.ResumePath, WorkflowID: selection.Recording.WorkflowID,
			FlushInterval: selection.Recording.FlushInterval,
		},
		ModelCacheDirectory: selection.ModelCacheDirectory,
		OperatorDefaults:    selection.OperatorDefaults,
	}, nil
}
