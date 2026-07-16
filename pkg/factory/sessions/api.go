package factorysessions

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

// NewLiveSession constructs a registry entry for a started session.
func NewLiveSession(
	sessionID string,
	factoryDir string,
	folderPath string,
	executionBaseDir string,
	target TargetRef,
	handle any,
	isDefault bool,
	project string,
) *LiveSession {
	session := &LiveSession{
		ID: sessionID,
		SessionState: SessionState{
			FactoryDir:       factoryDir,
			FolderPath:       folderPath,
			ExecutionBaseDir: executionBaseDir,
		},
		Handle:    handle,
		IsDefault: isDefault,
		Project:   project,
		Target:    target,
	}
	EnsureRuntimeFactorySessionID(session)
	session.ResponseEvents = NewSessionResponseEventStore(CanonicalFactorySessionID(session))
	return session
}

// SessionFactoryRootDir resolves the editable-definition root for a live session.
func SessionFactoryRootDir(serviceRootDir string, session *LiveSession) string {
	if session == nil {
		return ""
	}
	rootDir := session.FolderPath
	if session.FolderPath == "" {
		return rootDir
	}
	if session.FactoryDir == "" || !SameFactoryDir(session.FactoryDir, session.FolderPath) {
		return rootDir
	}
	serviceRoot := filepath.Clean(serviceRootDir)
	if serviceRoot != "" && filepath.Dir(session.FactoryDir) == serviceRoot {
		return serviceRoot
	}
	return rootDir
}

// FactoryName derives the API factory name for a session runtime config.
func FactoryName(rootDir string, runtimeCfg *factoryconfig.LoadedFactoryConfig) factoryapi.FactoryName {
	if runtimeCfg == nil {
		return apisurface.DefaultCurrentFactoryName
	}
	factoryDir := runtimeCfg.FactoryDir()
	cleanRoot := filepath.Clean(rootDir)
	if SameFactoryDir(factoryDir, cleanRoot) {
		return apisurface.DefaultCurrentFactoryName
	}
	if rootDir != "" && filepath.Dir(factoryDir) == cleanRoot {
		name := filepath.Base(factoryDir)
		if err := factoryconfig.ValidateNamedFactoryName(name); err == nil {
			return factoryapi.FactoryName(name)
		}
	}
	cfg := runtimeCfg.FactoryConfig()
	if cfg != nil {
		if name := strings.TrimSpace(cfg.Name); name != "" {
			return factoryapi.FactoryName(name)
		}
		if project := strings.TrimSpace(cfg.Project); project != "" {
			return factoryapi.FactoryName(project)
		}
	}
	return factoryapi.FactoryName("factory")
}

// ValidateInitNewFactoryNestedDir rejects init-new-factory when the canonical nested
// factory directory already exists with content that init cannot populate without
// overwrite or cleanup.
func ValidateInitNewFactoryNestedDir(resolvedFolder string) error {
	nestedFactoryDir := filepath.Join(resolvedFolder, interfaces.FactoryDir)
	info, err := os.Stat(nestedFactoryDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return NewValidationError(
			validationReasonUnreadable,
			"folderPath",
			fmt.Errorf("inspect nested factory directory %s: %w", nestedFactoryDir, err),
		)
	}
	if !info.IsDir() {
		return NewValidationError(
			validationReasonConflict,
			"folderPath",
			fmt.Errorf(
				"cannot initialize factory scaffold: %q exists and is not a directory",
				nestedFactoryDir,
			),
		)
	}

	entries, err := os.ReadDir(nestedFactoryDir)
	if err != nil {
		return NewValidationError(
			validationReasonUnreadable,
			"folderPath",
			fmt.Errorf("read nested factory directory %s: %w", nestedFactoryDir, err),
		)
	}
	if len(entries) > 0 {
		return NewValidationError(
			validationReasonConflict,
			"folderPath",
			fmt.Errorf(
				"cannot initialize factory scaffold: %q already exists with conflicting content",
				nestedFactoryDir,
			),
		)
	}
	return nil
}

// SessionResponse maps a live session and its domain runtime projection to the
// API detail shape. Generated transport values are confined to this adapter.
func SessionResponse(ctx ProjectionContext) factoryapi.FactorySession {
	summary := sessionSummaryToAPI(ctx.Session)
	runtime := projectSessionReadRuntime(ctx)
	return factoryapi.FactorySession{
		FactoryDir: summary.FactoryDir, FolderPath: summary.FolderPath, Id: summary.Id,
		IsDefault: summary.IsDefault, Project: summary.Project, Target: summary.Target, Runtime: runtime,
	}
}

// SummaryWithRuntime maps a live session to the API summary shape with runtime projection.
func SummaryWithRuntime(ctx ProjectionContext) factoryapi.FactorySessionSummary {
	summary := sessionSummaryToAPI(ctx.Session)
	runtime := projectSessionReadRuntime(ctx)
	summary.Runtime = &runtime
	return summary
}

func sessionSummaryToAPI(session *LiveSession) factoryapi.FactorySessionSummary {
	return factoryapi.FactorySessionSummary{
		FactoryDir: session.FactoryDir,
		FolderPath: session.FolderPath,
		Id:         CanonicalFactorySessionID(session),
		IsDefault:  session.IsDefault,
		Project:    session.Project,
		Target: factoryapi.FactorySessionTargetRef{
			Kind: factoryapi.FactorySessionTargetRefKind(session.Target.Kind),
			Name: stringPointerOrNil(session.Target.Name),
		},
	}
}

func stringPointerOrNil(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func projectSessionReadRuntime(ctx ProjectionContext) factoryapi.FactorySessionRuntime {
	runtime := runtimeProjectionToAPI(ProjectRuntimeContract(ctx), ctx)
	runtime.StopSummary = apisurface.BuildFactorySessionStopSummary(
		sessionIDFromProjectionContext(ctx), ctx.Snapshot, ctx.JavaScript,
	)
	return runtime
}

// ProjectRuntime maps the Factory Session-owned runtime projection to the
// generated compatibility contract. New domain callers should use ProjectRuntimeContract.
func ProjectRuntime(ctx ProjectionContext) factoryapi.FactorySessionRuntime {
	return projectSessionReadRuntime(ctx)
}

func runtimeProjectionToAPI(projection RuntimeProjection, ctx ProjectionContext) factoryapi.FactorySessionRuntime {
	runtime := factoryapi.FactorySessionRuntime{
		Artifacts: runtimeArtifactsToAPI(projection.Artifacts), Budgets: runtimeBudgetsToAPI(projection.Budgets),
		Dialect: projection.Dialect, Javascript: javascriptRuntimeProjectionToAPI(projection.JavaScript),
		Lifecycle:        runtimeLifecycleToAPI(projection.Lifecycle),
		OrchestratorKind: factoryapi.FactoryOrchestratorKind(projection.OrchestratorKind),
		Petri:            petriRuntimeProjectionToAPI(projection.Petri), PolicyHash: projection.PolicyHash,
		Progress: runtimeProgressToAPI(projection.Progress), SourceHash: projection.SourceHash,
		SourceRef: projection.SourceRef, Status: factoryapi.FactorySessionStatus(projection.Status),
		StreamIdentity: runtimeStreamIdentityToAPI(projection.StreamIdentity, ctx.NormalizedTarget),
		Usage:          runtimeUsageToAPI(projection.Usage),
	}
	if projection.LifecycleControlStatus != nil {
		status := factoryapi.FactorySessionDurableLifecycleStatus(*projection.LifecycleControlStatus)
		runtime.LifecycleControlStatus = &status
	}
	return runtime
}

func runtimeArtifactsToAPI(artifacts *[]interfaces.FactoryArtifact) *[]factoryapi.FactoryArtifact {
	if artifacts == nil {
		return nil
	}
	return apisurface.WorkflowArtifactsToAPI(*artifacts)
}

func runtimeBudgetsToAPI(budgets *RuntimeBudgets) *factoryapi.FactorySessionBudgets {
	if budgets == nil {
		return nil
	}
	return &factoryapi.FactorySessionBudgets{MaxAgents: budgets.MaxAgents}
}

func runtimeLifecycleToAPI(lifecycle RuntimeLifecycle) factoryapi.FactorySessionLifecycle {
	return factoryapi.FactorySessionLifecycle{
		FinishedAt: lifecycle.FinishedAt, StartedAt: lifecycle.StartedAt, UpdatedAt: lifecycle.UpdatedAt,
	}
}

func runtimeStreamIdentityToAPI(
	identity *RuntimeStreamIdentity,
	normalizedTarget *RuntimeLogicalTarget,
) *factoryapi.FactorySessionStreamIdentity {
	if identity == nil {
		return nil
	}
	return &factoryapi.FactorySessionStreamIdentity{
		BackendScopeID: identity.BackendScopeID, FactorySessionID: identity.FactorySessionID,
		LogicalSessionKeyID: identity.LogicalSessionKeyID, NormalizedTarget: runtimeLogicalTargetToAPI(normalizedTarget),
		StreamGenerationID: identity.StreamGenerationID,
	}
}

func runtimeLogicalTargetToAPI(target *RuntimeLogicalTarget) *factoryapi.FactorySessionLogicalTarget {
	if target == nil {
		return nil
	}
	public := &factoryapi.FactorySessionLogicalTarget{
		FolderPath:  target.FolderPath,
		Kind:        factoryapi.FactorySessionLogicalTargetKind(target.Kind),
		NamedTarget: target.NamedTarget,
	}
	if target.ProviderBoundary != nil {
		public.ProviderBoundary = &factoryapi.FactorySessionLogicalProviderBoundary{
			Boundary: target.ProviderBoundary.Boundary,
			Kind:     target.ProviderBoundary.Kind,
			Provider: target.ProviderBoundary.Provider,
		}
	}
	return public
}

func runtimeProgressToAPI(progress RuntimeProgress) factoryapi.FactorySessionProgress {
	return factoryapi.FactorySessionProgress{
		Categories: factoryapi.StatusCategories{
			Failed: progress.Categories.Failed, Initial: progress.Categories.Initial,
			Processing: progress.Categories.Processing, Terminal: progress.Categories.Terminal,
		},
		FactoryState: progress.FactoryState, InFlightCount: progress.InFlightCount,
		TotalTokens: progress.TotalTokens,
	}
}

func runtimeUsageToAPI(usage RuntimeUsage) factoryapi.FactorySessionUsage {
	resources := make([]factoryapi.ResourceUsage, 0, len(usage.Resources))
	for _, resource := range usage.Resources {
		resources = append(resources, factoryapi.ResourceUsage{
			Available: resource.Available, Name: resource.Name, Total: resource.Total,
		})
	}
	return factoryapi.FactorySessionUsage{Resources: resources}
}

func petriRuntimeProjectionToAPI(projection *PetriRuntimeProjection) *factoryapi.FactorySessionPetriProjection {
	if projection == nil {
		return nil
	}
	marking := make([]factoryapi.TokenResponse, 0, len(projection.Marking))
	for _, token := range projection.Marking {
		var tags *factoryapi.StringMap
		if token.Tags != nil {
			converted := factoryapi.StringMap(*token.Tags)
			tags = &converted
		}
		marking = append(marking, factoryapi.TokenResponse{
			ChainingTraceDepth: token.ChainingTraceDepth, CreatedAt: token.CreatedAt,
			CurrentChainingTraceId: token.CurrentChainingTraceID, EnteredAt: token.EnteredAt,
			Id: token.ID, Name: token.Name, PlaceId: token.PlaceID,
			PreviousChainingTraceIds: token.PreviousChainingTraceIDs, Tags: tags,
			TraceId: token.TraceID, WorkId: token.WorkID, WorkType: token.WorkType,
		})
	}
	enabled := make([]factoryapi.FactorySessionPetriEnabledTransition, 0, len(projection.EnabledTransitions))
	for _, transition := range projection.EnabledTransitions {
		enabled = append(enabled, factoryapi.FactorySessionPetriEnabledTransition{
			TransitionId: transition.TransitionID, WorkerType: transition.WorkerType,
		})
	}
	return &factoryapi.FactorySessionPetriProjection{Marking: marking, EnabledTransitions: enabled}
}

func javascriptRuntimeProjectionToAPI(projection *JavaScriptRuntimeProjection) *factoryapi.FactorySessionJavaScriptProjection {
	if projection == nil {
		return nil
	}
	checkpoints := apisurface.WorkflowCheckpointRefsToAPI(valueOrEmpty(projection.Checkpoints))
	result := &factoryapi.FactorySessionJavaScriptProjection{
		ArgsDigest: projection.ArgsDigest,
		ChildDispatchCounts: factoryapi.FactorySessionJavaScriptChildDispatchCounts{
			Completed: projection.ChildDispatchCounts.Completed,
			Queued:    projection.ChildDispatchCounts.Queued, Running: projection.ChildDispatchCounts.Running,
		},
		Phase: projection.Phase, Phases: append([]string(nil), projection.Phases...),
		ScriptStatus: factoryapi.FactorySessionJavaScriptScriptStatus(projection.ScriptStatus),
	}
	if len(checkpoints) > 0 {
		result.Checkpoints = &checkpoints
	}
	return result
}

func valueOrEmpty[T any](value *[]T) []T {
	if value == nil {
		return nil
	}
	return *value
}
