package service

import (
	"context"
	"fmt"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
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
	selected := request
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
	if products.application.FactorySessions == nil {
		return factorysessions.SessionStartResult{}, fmt.Errorf("start Factory Session: activated session is unavailable")
	}
	started, err := products.application.FactorySessions.Start(ctx, selected)
	if err != nil {
		if products.application.Resources.Close != nil {
			_ = products.application.Resources.Close()
		}
		return factorysessions.SessionStartResult{}, err
	}
	if request.ActivationOnly {
		started.Status = "RUNNING"
	}
	if session := r.Resolve(started.SessionID); session != nil && started.Live != nil {
		view := factorysessions.SessionView{
			SessionID: livesession.CanonicalID(session), Mode: factorysessions.SessionOperationModeLive,
			Status: started.Status, FactoryDir: session.FactoryDir, FolderPath: session.FolderPath,
			Project: session.Project, IsDefault: session.IsDefault, Target: session.Target,
			RuntimeAvailable: session.Runtime != nil,
		}
		started.Live.Session = &view
	}
	return started, nil
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
