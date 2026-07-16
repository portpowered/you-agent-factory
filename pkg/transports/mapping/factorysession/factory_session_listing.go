package factorysession

import (
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factorysessions "github.com/portpowered/infinite-you/pkg/factory/sessions"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/logicaltarget"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

// OpenRequestFromAPI detaches a generated open request before Factory Session
// policy executes.
func OpenRequestFromAPI(request factoryapi.OpenFactorySessionRequest) factorysessions.OpenRequest {
	var target *factorysessions.TargetRef
	if request.Target != nil {
		targetName := ""
		if request.Target.Name != nil {
			targetName = strings.TrimSpace(*request.Target.Name)
		}
		target = &factorysessions.TargetRef{
			Kind: factorysessions.TargetKind(request.Target.Kind),
			Name: targetName,
		}
	}
	return factorysessions.OpenRequest{
		FolderPath:     request.FolderPath,
		Target:         target,
		ValidateOnly:   request.ValidateOnly != nil && *request.ValidateOnly,
		InitNewFactory: request.InitNewFactory != nil && *request.InitNewFactory,
	}
}

// OpenResultToAPI maps an owner-defined open result and its optional live
// session to the generated public response.
func OpenResultToAPI(
	result *factorysessions.OpenResult,
	session *factorysessions.LiveSession,
) factoryapi.OpenFactorySessionResponse {
	response := factoryapi.OpenFactorySessionResponse{}
	if result == nil {
		return response
	}
	if result.InitsNewFactory {
		initsNewFactory := true
		response.InitsNewFactory = &initsNewFactory
		if folderPath := strings.TrimSpace(result.FolderPath); folderPath != "" {
			response.FolderPath = &folderPath
		}
	}
	if len(result.Targets) > 0 {
		targets := TargetsToAPI(result.Targets)
		response.Targets = &targets
	}
	if session != nil {
		summary := SessionSummaryToAPI(session)
		response.Session = &summary
	}
	return response
}

// SessionSummaryToAPI maps one live Factory Session to its public summary.
func SessionSummaryToAPI(session *factorysessions.LiveSession) factoryapi.FactorySessionSummary {
	if session == nil {
		return factoryapi.FactorySessionSummary{}
	}
	return factoryapi.FactorySessionSummary{
		FactoryDir: session.FactoryDir,
		FolderPath: session.FolderPath,
		Id:         factorysessions.CanonicalFactorySessionID(session),
		IsDefault:  session.IsDefault,
		Project:    session.Project,
		Target: factoryapi.FactorySessionTargetRef{
			Kind: factoryapi.FactorySessionTargetRefKind(session.Target.Kind),
			Name: optionalTrimmedString(session.Target.Name),
		},
	}
}

// SessionResponseToAPI maps a live session and its owner-defined runtime
// projection to the public detail contract.
func SessionResponseToAPI(ctx factorysessions.ProjectionContext) factoryapi.FactorySession {
	summary := SessionSummaryToAPI(ctx.Session)
	runtime := sessionRuntimeToAPI(ctx)
	return factoryapi.FactorySession{
		FactoryDir: summary.FactoryDir, FolderPath: summary.FolderPath, Id: summary.Id,
		IsDefault: summary.IsDefault, Project: summary.Project, Target: summary.Target, Runtime: runtime,
	}
}

// SummaryWithRuntimeToAPI maps a live session and runtime projection to the
// public summary contract.
func SummaryWithRuntimeToAPI(ctx factorysessions.ProjectionContext) factoryapi.FactorySessionSummary {
	summary := SessionSummaryToAPI(ctx.Session)
	runtime := sessionRuntimeToAPI(ctx)
	summary.Runtime = &runtime
	return summary
}

// ReadProjectionsToAPI maps domain-owned list reads to the public response.
func ReadProjectionsToAPI(reads []factorysessions.ReadProjection) factoryapi.ListFactorySessionsResponse {
	summaries := make([]factoryapi.FactorySessionSummary, 0, len(reads))
	for _, read := range reads {
		if read.RuntimeAvailable {
			summaries = append(summaries, SummaryWithRuntimeToAPI(read.Context))
			continue
		}
		summaries = append(summaries, SessionSummaryToAPI(read.Context.Session))
	}
	return factoryapi.ListFactorySessionsResponse{Sessions: summaries}
}

// RuntimeFromContextToAPI projects owner-defined runtime state and maps it to
// the public contract, including transport-owned stop-summary compatibility.
func RuntimeFromContextToAPI(ctx factorysessions.ProjectionContext) factoryapi.FactorySessionRuntime {
	return sessionRuntimeToAPI(ctx)
}

// RuntimeProjectionToAPI maps the Factory Session-owned runtime projection to
// the generated public contract.
func RuntimeProjectionToAPI(
	projection factorysessions.RuntimeProjection,
	normalizedTarget *factorysessions.RuntimeLogicalTarget,
) factoryapi.FactorySessionRuntime {
	runtime := factoryapi.FactorySessionRuntime{
		Artifacts: runtimeArtifactsToAPI(projection.Artifacts), Budgets: runtimeBudgetsToAPI(projection.Budgets),
		Dialect: projection.Dialect, Javascript: javascriptRuntimeProjectionToAPI(projection.JavaScript),
		Lifecycle:        runtimeLifecycleToAPI(projection.Lifecycle),
		OrchestratorKind: factoryapi.FactoryOrchestratorKind(projection.OrchestratorKind),
		Petri:            petriRuntimeProjectionToAPI(projection.Petri), PolicyHash: projection.PolicyHash,
		Progress: runtimeProgressToAPI(projection.Progress), SourceHash: projection.SourceHash,
		SourceRef: projection.SourceRef, Status: factoryapi.FactorySessionStatus(projection.Status),
		StreamIdentity: runtimeStreamIdentityToAPI(projection.StreamIdentity, normalizedTarget),
		Usage:          runtimeUsageToAPI(projection.Usage),
	}
	if projection.LifecycleControlStatus != nil {
		status := factoryapi.FactorySessionDurableLifecycleStatus(*projection.LifecycleControlStatus)
		runtime.LifecycleControlStatus = &status
	}
	return runtime
}

func sessionRuntimeToAPI(ctx factorysessions.ProjectionContext) factoryapi.FactorySessionRuntime {
	projection := factorysessions.ProjectRuntimeContract(ctx)
	runtime := RuntimeProjectionToAPI(projection, ctx.NormalizedTarget)
	sessionID := ""
	if ctx.Session != nil {
		sessionID = strings.TrimSpace(ctx.Session.ID)
	}
	runtime.StopSummary = apisurface.BuildFactorySessionStopSummary(sessionID, ctx.Snapshot, ctx.JavaScript)
	return runtime
}

func runtimeArtifactsToAPI(artifacts *[]interfaces.FactoryArtifact) *[]factoryapi.FactoryArtifact {
	if artifacts == nil {
		return nil
	}
	return apisurface.WorkflowArtifactsToAPI(*artifacts)
}

func runtimeBudgetsToAPI(budgets *factorysessions.RuntimeBudgets) *factoryapi.FactorySessionBudgets {
	if budgets == nil {
		return nil
	}
	return &factoryapi.FactorySessionBudgets{MaxAgents: budgets.MaxAgents}
}

func runtimeLifecycleToAPI(lifecycle factorysessions.RuntimeLifecycle) factoryapi.FactorySessionLifecycle {
	return factoryapi.FactorySessionLifecycle{
		FinishedAt: lifecycle.FinishedAt, StartedAt: lifecycle.StartedAt, UpdatedAt: lifecycle.UpdatedAt,
	}
}

func runtimeStreamIdentityToAPI(
	identity *factorysessions.RuntimeStreamIdentity,
	normalizedTarget *factorysessions.RuntimeLogicalTarget,
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

func runtimeLogicalTargetToAPI(target *factorysessions.RuntimeLogicalTarget) *factoryapi.FactorySessionLogicalTarget {
	if target == nil {
		return nil
	}
	public := &factoryapi.FactorySessionLogicalTarget{
		FolderPath: target.FolderPath, Kind: factoryapi.FactorySessionLogicalTargetKind(target.Kind),
		NamedTarget: target.NamedTarget,
	}
	if target.ProviderBoundary != nil {
		public.ProviderBoundary = &factoryapi.FactorySessionLogicalProviderBoundary{
			Boundary: target.ProviderBoundary.Boundary, Kind: target.ProviderBoundary.Kind,
			Provider: target.ProviderBoundary.Provider,
		}
	}
	return public
}

func runtimeProgressToAPI(progress factorysessions.RuntimeProgress) factoryapi.FactorySessionProgress {
	return factoryapi.FactorySessionProgress{
		Categories: factoryapi.StatusCategories{
			Failed: progress.Categories.Failed, Initial: progress.Categories.Initial,
			Processing: progress.Categories.Processing, Terminal: progress.Categories.Terminal,
		},
		FactoryState: progress.FactoryState, InFlightCount: progress.InFlightCount,
		TotalTokens: progress.TotalTokens,
	}
}

func runtimeUsageToAPI(usage factorysessions.RuntimeUsage) factoryapi.FactorySessionUsage {
	resources := make([]factoryapi.ResourceUsage, 0, len(usage.Resources))
	for _, resource := range usage.Resources {
		resources = append(resources, factoryapi.ResourceUsage{
			Available: resource.Available, Name: resource.Name, Total: resource.Total,
		})
	}
	return factoryapi.FactorySessionUsage{Resources: resources}
}

func petriRuntimeProjectionToAPI(projection *factorysessions.PetriRuntimeProjection) *factoryapi.FactorySessionPetriProjection {
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

func javascriptRuntimeProjectionToAPI(projection *factorysessions.JavaScriptRuntimeProjection) *factoryapi.FactorySessionJavaScriptProjection {
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

// TargetToAPI maps one discovered Factory Session target to the public shape.
func TargetToAPI(target factorysessions.Target) factoryapi.FactorySessionTarget {
	return factoryapi.FactorySessionTarget{
		FactoryDir: target.FactoryDir,
		FolderPath: target.FolderPath,
		Label:      target.Label,
		Project:    target.Project,
		Ref: factoryapi.FactorySessionTargetRef{
			Kind: factoryapi.FactorySessionTargetRefKind(target.Ref.Kind),
			Name: optionalTrimmedString(target.Ref.Name),
		},
	}
}

// TargetsToAPI maps discovered Factory Session targets to the public shape.
func TargetsToAPI(targets []factorysessions.Target) []factoryapi.FactorySessionTarget {
	if len(targets) == 0 {
		return nil
	}
	response := make([]factoryapi.FactorySessionTarget, 0, len(targets))
	for _, target := range targets {
		response = append(response, TargetToAPI(target))
	}
	return response
}

// ListSessionsRequestFromAPI maps one public session list request into the shared
// service contract. Scope defaults to live for backward-compatible live workspace
// session listing.
func ListSessionsRequestFromAPI(params factoryapi.ListFactorySessionsParams) (factorysessionexecution.ListSessionsRequest, error) {
	req := factorysessionexecution.ListSessionsRequest{}
	if params.Scope != nil {
		req.Scope = factorysessionexecution.SessionListScope(*params.Scope)
	}
	return factorysessionexecution.NormalizeListSessionsRequest(req)
}

// ListSessionsResponseToAPI maps one scoped session list result to the public response shape.
func ListSessionsResponseToAPI(result factorysessionexecution.ListSessionsResult) factoryapi.ListFactorySessionsResponse {
	scope := factoryapi.FactorySessionListScope(result.Scope)
	response := factoryapi.ListFactorySessionsResponse{
		Scope:    &scope,
		Sessions: make([]factoryapi.FactorySessionSummary, 0, len(result.LiveSessions)),
	}
	for _, session := range result.LiveSessions {
		response.Sessions = append(response.Sessions, LiveSessionSummaryToAPI(session))
	}
	if len(result.DurableSessions) > 0 {
		durable := make([]factoryapi.FactorySessionDurableSummary, 0, len(result.DurableSessions))
		for _, summary := range result.DurableSessions {
			durable = append(durable, DurableSessionSummaryToAPI(summary))
		}
		response.DurableSessions = &durable
	}
	return response
}

// LiveSessionSummaryToAPI maps one live workspace session row to the public summary shape.
func LiveSessionSummaryToAPI(session factorysessionexecution.LiveSessionSummary) factoryapi.FactorySessionSummary {
	return factoryapi.FactorySessionSummary{
		Id:         session.ID,
		FactoryDir: session.FactoryDir,
		FolderPath: session.FolderPath,
		Project:    session.Project,
		IsDefault:  session.IsDefault,
		Target:     factoryapi.FactorySessionTargetRef{},
	}
}

// LogicalTargetToAPI maps one normalized Factory Session logical target to the
// public client-safe API shape.
func LogicalTargetToAPI(ref logicaltarget.CanonicalReference) factoryapi.FactorySessionLogicalTarget {
	target := factoryapi.FactorySessionLogicalTarget{
		Kind:       logicalTargetKindToAPI(ref.Kind),
		FolderPath: ref.FolderPath,
	}
	if ref.Kind == logicaltarget.KindNamed {
		namedTarget := ref.NamedTarget
		target.NamedTarget = &namedTarget
	}
	if ref.Kind == logicaltarget.KindProvider && ref.Provider != nil {
		target.ProviderBoundary = &factoryapi.FactorySessionLogicalProviderBoundary{
			Provider: ref.Provider.Provider,
			Kind:     ref.Provider.Kind,
			Boundary: ref.Provider.Boundary,
		}
	}
	return target
}

// LogicalTargetFromSession derives public normalized target metadata for one
// live Factory Session within backendScopeID.
func LogicalTargetFromSession(
	backendScopeID string,
	session *factorysessions.LiveSession,
) (*factoryapi.FactorySessionLogicalTarget, error) {
	if session == nil {
		return nil, nil
	}
	ref, err := logicaltarget.NormalizeTargetRef(backendScopeID, session.FolderPath, session.Target)
	if err != nil {
		return nil, err
	}
	target := LogicalTargetToAPI(ref)
	return &target, nil
}

func logicalTargetKindToAPI(kind logicaltarget.Kind) factoryapi.FactorySessionLogicalTargetKind {
	switch kind {
	case logicaltarget.KindNamed:
		return factoryapi.FactorySessionLogicalTargetKindNamed
	case logicaltarget.KindProvider:
		return factoryapi.FactorySessionLogicalTargetKindProvider
	default:
		return factoryapi.FactorySessionLogicalTargetKindDefault
	}
}

// DurableSessionSummaryToAPI maps one durable session list row to the public summary shape.
func DurableSessionSummaryToAPI(summary factorysessionexecution.DurableSessionListSummary) factoryapi.FactorySessionDurableSummary {
	response := factoryapi.FactorySessionDurableSummary{
		SessionId:        summary.SessionID,
		Status:           factoryapi.FactorySessionDurableLifecycleStatus(summary.Status),
		OrchestratorKind: factoryapi.FactoryOrchestratorKind(interfaces.StrictPublicFactoryOrchestratorKind(summary.OrchestratorKind)),
		ResolvedSource:   resolvedSourceToAPI(summary.ResolvedSource),
	}
	if dialect := strings.TrimSpace(summary.Dialect); dialect != "" {
		response.Dialect = &dialect
	}
	if sourceHash := strings.TrimSpace(summary.SourceHash); sourceHash != "" {
		response.SourceHash = &sourceHash
	}
	if requested := policyToAPI(summary.Policy.Requested); requested != nil {
		response.RequestedPolicy = requested
	}
	if effective := effectivePolicyToAPI(summary.Policy.Effective); effective != nil {
		response.EffectivePolicy = effective
	}
	if effectiveHash := strings.TrimSpace(summary.Policy.EffectiveHash); effectiveHash != "" {
		response.EffectivePolicyHash = &effectiveHash
	}
	if phase := strings.TrimSpace(summary.Phase); phase != "" {
		response.Phase = &phase
	}
	if progress := progressCountsToAPI(summary.Progress); progress != nil {
		response.Progress = progress
	}
	if resultSummary := resultSummaryToAPI(summary.ResultSummary); resultSummary != nil {
		response.ResultSummary = resultSummary
	}
	if summary.ArtifactCount > 0 {
		count := summary.ArtifactCount
		response.ArtifactCount = &count
	}
	if summary.Recoverable {
		recoverable := true
		response.Recoverable = &recoverable
	}
	response.Actions = sessionActionAvailabilityToAPI(summary.Actions)
	if summary.StaleLease {
		stale := true
		response.StaleLease = &stale
	}
	if lifecycle := lifecycleTimestampsToAPI(summary.Lifecycle); lifecycle != nil {
		response.Lifecycle = lifecycle
	}
	if links := executionLinksToAPI(summary.Links); links != nil {
		response.Links = links
	}
	return response
}

// SyncPreflightResultToAPI maps the Factory Session reconnect decision to the
// generated public response at the transport boundary.
func SyncPreflightResultToAPI(result factorysessions.SyncPreflightResult) factoryapi.FactorySessionSyncPreflightResponse {
	return factoryapi.FactorySessionSyncPreflightResponse{
		BackendScopeId:      result.BackendScopeID,
		CheckpointReusable:  result.CheckpointReusable,
		FactorySessionId:    result.FactorySessionID,
		LogicalSessionKeyId: result.LogicalSessionKeyID,
		ReasonCode:          factoryapi.FactorySessionSyncPreflightReasonCode(result.Reason),
		ReconnectCursor: factoryapi.FactorySessionSyncPreflightReconnectCursor{
			AfterEventId:             result.ReconnectCursor.AfterEventID,
			AfterSequence:            result.ReconnectCursor.AfterSequence,
			Provided:                 result.ReconnectCursor.Provided,
			ValidForStreamGeneration: result.ReconnectCursor.ValidForStreamGeneration,
		},
		RequestedSessionId: result.RequestedSessionID,
		StreamGenerationId: result.StreamGenerationID,
	}
}
