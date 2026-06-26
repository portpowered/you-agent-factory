package factorysessions

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/factory/scheduler"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	workflowpolicy "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/policy"
	"github.com/portpowered/infinite-you/pkg/petri"
)

// ProjectionContext carries the runtime inputs needed to project one live session.
type ProjectionContext struct {
	Session                *LiveSession
	FactoryCfg             *interfaces.FactoryConfig
	Snapshot               *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]
	LifecycleControlStatus string
	BackendScopeID        string
	Enabled               []interfaces.EnabledTransition
	JavaScript            *interfaces.FactorySessionJavaScriptRuntimeState
	JavaScriptCheckpoints []interfaces.JavaScriptCheckpointRecord
	Now                   time.Time
}

// SessionResponse maps a live session and runtime projection to the API detail shape.
func SessionResponse(ctx ProjectionContext) factoryapi.FactorySession {
	summary := SummaryResponse(ctx.Session)
	runtime := ProjectRuntime(ctx)
	return factoryapi.FactorySession{
		FactoryDir: summary.FactoryDir,
		FolderPath: summary.FolderPath,
		Id:         summary.Id,
		IsDefault:  summary.IsDefault,
		Project:    summary.Project,
		Target:     summary.Target,
		Runtime:    runtime,
	}
}

// SummaryWithRuntime maps a live session to the API summary shape with runtime projection.
func SummaryWithRuntime(ctx ProjectionContext) factoryapi.FactorySessionSummary {
	summary := SummaryResponse(ctx.Session)
	runtime := ProjectRuntime(ctx)
	summary.Runtime = &runtime
	return summary
}

// ProjectRuntime builds the orchestrator-aware runtime projection for one session.
func ProjectRuntime(ctx ProjectionContext) factoryapi.FactorySessionRuntime {
	now := ctx.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	kind := interfaces.EffectiveOrchestratorKind(ctx.FactoryCfg)
	runtime := factoryapi.FactorySessionRuntime{
		OrchestratorKind: interfaces.GeneratedPublicFactoryOrchestratorKind(kind),
		Status:           projectedSessionStatus(ctx),
		Progress:         projectedSessionProgress(ctx),
		Usage:            projectedSessionUsage(ctx),
		Lifecycle:        projectedSessionLifecycle(ctx, now),
	}
	if streamIdentity := projectedSessionStreamIdentity(ctx, runtime.Lifecycle); streamIdentity != nil {
		runtime.StreamIdentity = streamIdentity
	}
	if lifecycleControlStatus := projectedSessionLifecycleControlStatus(ctx); lifecycleControlStatus != "" {
		status := factoryapi.FactorySessionDurableLifecycleStatus(lifecycleControlStatus)
		runtime.LifecycleControlStatus = &status
	}
	projectOrchestratorIdentity(&runtime, ctx.FactoryCfg)
	switch kind {
	case interfaces.OrchestratorKindJavaScript:
		runtime.Javascript = projectedJavaScriptRuntime(ctx)
	default:
		runtime.Petri = projectedPetriRuntime(ctx)
	}
	dispatches, artifacts := projectedSessionDispatchArtifacts(ctx, kind)
	if dispatches != nil {
		runtime.Dispatches = dispatches
	}
	if artifacts != nil {
		runtime.Artifacts = artifacts
	}
	return runtime
}

func projectedSessionStreamIdentity(
	ctx ProjectionContext,
	lifecycle factoryapi.FactorySessionLifecycle,
) *factoryapi.FactorySessionStreamIdentity {
	sessionID := ""
	if ctx.Session != nil {
		sessionID = strings.TrimSpace(ctx.Session.ID)
	}
	backendScopeID := strings.TrimSpace(ctx.BackendScopeID)
	if backendScopeID == "" || sessionID == "" || lifecycle.StartedAt.IsZero() {
		return nil
	}
	streamGenerationID := lifecycle.StartedAt.UTC().Format(time.RFC3339Nano)
	return &factoryapi.FactorySessionStreamIdentity{
		BackendScopeID:    backendScopeID,
		FactorySessionID:  sessionID,
		StreamGenerationID: streamGenerationID,
	}
}

func projectedSessionDispatchArtifacts(
	ctx ProjectionContext,
	kind string,
) (*[]factoryapi.FactoryDispatch, *[]factoryapi.FactoryArtifact) {
	sessionID := ""
	if ctx.Session != nil {
		sessionID = strings.TrimSpace(ctx.Session.ID)
	}
	orchestratorKind := interfaces.GeneratedPublicFactoryOrchestratorKind(kind)
	switch kind {
	case interfaces.OrchestratorKindJavaScript:
		if ctx.JavaScript == nil {
			return nil, nil
		}
		dispatchStates := projectedJavaScriptDispatchStates(ctx.JavaScript.Dispatches)
		artifactStates := ArtifactStatesFromJavaScriptRuntime(ctx.JavaScriptCheckpoints, ctx.JavaScript.Artifacts)
		return projectedDispatches(sessionID, orchestratorKind, dispatchStates),
			projectedArtifacts(artifactStates)
	default:
		dispatchStates := PetriDispatchStatesFromSnapshot(ctx.Snapshot)
		return projectedDispatches(sessionID, orchestratorKind, dispatchStates), nil
	}
}

func projectOrchestratorIdentity(runtime *factoryapi.FactorySessionRuntime, cfg *interfaces.FactoryConfig) {
	if runtime == nil || cfg == nil || cfg.Orchestrator == nil {
		return
	}
	if cfg.Orchestrator.JavaScript != nil {
		js := cfg.Orchestrator.JavaScript
		runtime.Dialect = stringPointerOrNil(js.Dialect)
		runtime.SourceRef = stringPointerOrNil(js.SourceRef)
		runtime.SourceHash = stringPointerOrNil(js.SourceHash)
		if policyHash := workflowpolicy.HashDocument(js.DefaultPolicy); policyHash != "" {
			runtime.PolicyHash = &policyHash
		}
		if budgets := projectedJavaScriptBudgets(js.DefaultPolicy); budgets != nil {
			runtime.Budgets = budgets
		}
	}
}

func projectedSessionStatus(ctx ProjectionContext) factoryapi.FactorySessionStatus {
	if ctx.Snapshot == nil {
		if ctx.JavaScript != nil && strings.TrimSpace(ctx.JavaScript.ScriptStatus) == "FINISHED" {
			return factoryapi.FactorySessionStatusFINISHED
		}
		return factoryapi.FactorySessionStatusIDLE
	}
	return factoryapi.FactorySessionStatus(ctx.Snapshot.RuntimeStatus)
}

func projectedSessionLifecycleControlStatus(ctx ProjectionContext) string {
	if ctx.Snapshot != nil {
		return strings.TrimSpace(ctx.Snapshot.LifecycleControlStatus)
	}
	return strings.TrimSpace(ctx.LifecycleControlStatus)
}

func projectedSessionFactoryState(ctx ProjectionContext) string {
	if ctx.Snapshot == nil {
		return "UNKNOWN"
	}
	return ctx.Snapshot.FactoryState
}

func projectedSessionProgress(ctx ProjectionContext) factoryapi.FactorySessionProgress {
	if ctx.Snapshot == nil {
		return factoryapi.FactorySessionProgress{
			FactoryState:  projectedSessionFactoryState(ctx),
			Categories:    factoryapi.StatusCategories{},
			InFlightCount: 0,
			TotalTokens:   0,
		}
	}
	categories, _ := categorizeProjectionTokens(&ctx.Snapshot.Marking, ctx.Snapshot.Topology)
	return factoryapi.FactorySessionProgress{
		FactoryState:  projectedSessionFactoryState(ctx),
		Categories:    categories,
		InFlightCount: ctx.Snapshot.InFlightCount,
		TotalTokens:   countProjectionTokens(&ctx.Snapshot.Marking),
	}
}

func projectedSessionUsage(ctx ProjectionContext) factoryapi.FactorySessionUsage {
	if ctx.Snapshot == nil {
		return factoryapi.FactorySessionUsage{Resources: []factoryapi.ResourceUsage{}}
	}
	_, resources := categorizeProjectionTokens(&ctx.Snapshot.Marking, ctx.Snapshot.Topology)
	return factoryapi.FactorySessionUsage{Resources: resources}
}

func projectedSessionLifecycle(ctx ProjectionContext, now time.Time) factoryapi.FactorySessionLifecycle {
	startedAt := now
	if ctx.Snapshot != nil && ctx.Snapshot.Uptime > 0 {
		startedAt = now.Add(-ctx.Snapshot.Uptime).UTC()
	}
	lifecycle := factoryapi.FactorySessionLifecycle{
		StartedAt: startedAt,
		UpdatedAt: now.UTC(),
	}
	if ctx.Snapshot != nil && ctx.Snapshot.RuntimeStatus == interfaces.RuntimeStatusFinished {
		finishedAt := now.UTC()
		lifecycle.FinishedAt = &finishedAt
	}
	return lifecycle
}

func projectedPetriRuntime(ctx ProjectionContext) *factoryapi.FactorySessionPetriProjection {
	if ctx.Snapshot == nil {
		return &factoryapi.FactorySessionPetriProjection{
			Marking:            []factoryapi.TokenResponse{},
			EnabledTransitions: []factoryapi.FactorySessionPetriEnabledTransition{},
		}
	}
	return &factoryapi.FactorySessionPetriProjection{
		Marking:            projectedMarkingTokens(&ctx.Snapshot.Marking),
		EnabledTransitions: projectedEnabledTransitions(ctx.Enabled),
	}
}

func projectedJavaScriptRuntime(ctx ProjectionContext) *factoryapi.FactorySessionJavaScriptProjection {
	state := ctx.JavaScript
	if state == nil {
		state = defaultJavaScriptRuntimeState(ctx.FactoryCfg)
	}
	projection := &factoryapi.FactorySessionJavaScriptProjection{
		Phase:               stringPointerOrNil(state.Phase),
		Phases:              append([]string(nil), state.Phases...),
		ArgsDigest:          stringPointerOrNil(state.ArgsDigest),
		ScriptStatus:        projectedJavaScriptScriptStatus(state.ScriptStatus),
		ChildDispatchCounts: projectedJavaScriptChildDispatchCounts(state),
	}
	if checkpoints := projectedJavaScriptCheckpoints(state.Checkpoints); len(checkpoints) > 0 {
		projection.Checkpoints = &checkpoints
	}
	return projection
}

func defaultJavaScriptRuntimeState(cfg *interfaces.FactoryConfig) *interfaces.FactorySessionJavaScriptRuntimeState {
	state := &interfaces.FactorySessionJavaScriptRuntimeState{
		ScriptStatus: "IDLE",
	}
	if cfg != nil && cfg.Orchestrator != nil && cfg.Orchestrator.JavaScript != nil {
		if digest := digestJSON(cfg.Orchestrator.JavaScript.ArgsSchema); digest != "" {
			state.ArgsDigest = digest
		}
	}
	return state
}

func projectedJavaScriptScriptStatus(value string) factoryapi.FactorySessionJavaScriptScriptStatus {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "RUNNING":
		return factoryapi.FactorySessionJavaScriptScriptStatusRUNNING
	case "PAUSED":
		return factoryapi.FactorySessionJavaScriptScriptStatusPAUSED
	case "FINISHED":
		return factoryapi.FactorySessionJavaScriptScriptStatusFINISHED
	case "FAILED":
		return factoryapi.FactorySessionJavaScriptScriptStatusFAILED
	default:
		return factoryapi.FactorySessionJavaScriptScriptStatusIDLE
	}
}

func projectedJavaScriptChildDispatchCounts(
	state *interfaces.FactorySessionJavaScriptRuntimeState,
) factoryapi.FactorySessionJavaScriptChildDispatchCounts {
	if state == nil {
		return factoryapi.FactorySessionJavaScriptChildDispatchCounts{}
	}
	return factoryapi.FactorySessionJavaScriptChildDispatchCounts{
		Queued:    state.QueuedDispatches,
		Running:   state.RunningDispatches,
		Completed: state.CompletedDispatches,
	}
}

func projectedJavaScriptCheckpoints(
	checkpoints []interfaces.FactorySessionJavaScriptCheckpointRef,
) []factoryapi.FactorySessionJavaScriptCheckpointRef {
	if len(checkpoints) == 0 {
		return nil
	}
	projected := make([]factoryapi.FactorySessionJavaScriptCheckpointRef, 0, len(checkpoints))
	for _, checkpoint := range checkpoints {
		item := factoryapi.FactorySessionJavaScriptCheckpointRef{Id: checkpoint.ID}
		if label := strings.TrimSpace(checkpoint.Label); label != "" {
			item.Label = &label
		}
		if summary := strings.TrimSpace(checkpoint.Summary); summary != "" {
			item.Summary = &summary
		}
		if !checkpoint.Timestamp.IsZero() {
			timestamp := checkpoint.Timestamp.UTC()
			item.Timestamp = &timestamp
		}
		if checkpoint.ArtifactRef != nil {
			artifactRef := projectedCheckpointArtifactRef(*checkpoint.ArtifactRef)
			item.ArtifactRef = &artifactRef
		}
		projected = append(projected, item)
	}
	return projected
}

func projectedCheckpointArtifactRef(ref interfaces.JavaScriptCheckpointArtifactRef) factoryapi.FactoryArtifactRef {
	artifactID := strings.TrimSpace(ref.ID)
	kind := factoryapi.FactoryArtifactKind(strings.TrimSpace(ref.Kind))
	if kind == "" {
		kind = factoryapi.FactoryArtifactKindCHECKPOINT
	}
	visibility := factoryapi.FactoryArtifactVisibility(strings.TrimSpace(ref.Visibility))
	if visibility == "" {
		visibility = factoryapi.FactoryArtifactVisibilityINTERNALCHECKPOINT
	}
	projected := factoryapi.FactoryArtifactRef{
		Id:         artifactID,
		Kind:       kind,
		Visibility: visibility,
	}
	if hash := strings.TrimSpace(ref.ContentHash); hash != "" {
		projected.ContentHash = &hash
	}
	if ref.SizeBytes > 0 {
		size := ref.SizeBytes
		projected.SizeBytes = &size
	}
	return projected
}

func projectedJavaScriptBudgets(raw json.RawMessage) *factoryapi.FactorySessionBudgets {
	resolution := apisurface.ResolveWorkflowPolicy(workflowpolicy.Request{FactoryDefault: raw})
	if resolution.Policy.MaxAgents <= 0 {
		return nil
	}
	value := resolution.Policy.MaxAgents
	return &factoryapi.FactorySessionBudgets{MaxAgents: &value}
}

func projectedMarkingTokens(marking *petri.MarkingSnapshot) []factoryapi.TokenResponse {
	if marking == nil || len(marking.Tokens) == 0 {
		return []factoryapi.TokenResponse{}
	}
	tokenIDs := make([]string, 0, len(marking.Tokens))
	for tokenID := range marking.Tokens {
		tokenIDs = append(tokenIDs, tokenID)
	}
	sort.Strings(tokenIDs)
	tokens := make([]factoryapi.TokenResponse, 0, len(tokenIDs))
	for _, tokenID := range tokenIDs {
		token := marking.Tokens[tokenID]
		if token == nil || interfaces.IsSystemTimeToken(token) {
			continue
		}
		tokens = append(tokens, projectedTokenResponse(token))
	}
	return tokens
}

func projectedEnabledTransitions(
	enabled []interfaces.EnabledTransition,
) []factoryapi.FactorySessionPetriEnabledTransition {
	if len(enabled) == 0 {
		return []factoryapi.FactorySessionPetriEnabledTransition{}
	}
	projected := make([]factoryapi.FactorySessionPetriEnabledTransition, 0, len(enabled))
	for _, transition := range enabled {
		projected = append(projected, factoryapi.FactorySessionPetriEnabledTransition{
			TransitionId: transition.TransitionID,
			WorkerType:   transition.WorkerType,
		})
	}
	sort.Slice(projected, func(i, j int) bool {
		if projected[i].TransitionId == projected[j].TransitionId {
			return projected[i].WorkerType < projected[j].WorkerType
		}
		return projected[i].TransitionId < projected[j].TransitionId
	})
	return projected
}

func projectedTokenResponse(token *interfaces.Token) factoryapi.TokenResponse {
	resp := factoryapi.TokenResponse{
		Id:        token.ID,
		PlaceId:   token.PlaceID,
		WorkId:    token.Color.WorkID,
		WorkType:  token.Color.WorkTypeID,
		TraceId:   token.Color.TraceID,
		CreatedAt: token.CreatedAt,
		EnteredAt: token.EnteredAt,
	}
	if token.Color.Name != "" {
		resp.Name = &token.Color.Name
	}
	if token.Color.ChainingTraceDepth > 0 {
		depth := token.Color.ChainingTraceDepth
		resp.ChainingTraceDepth = &depth
	}
	if traceID := firstNonEmptyString(token.Color.CurrentChainingTraceID, token.Color.TraceID); traceID != "" {
		resp.CurrentChainingTraceId = &traceID
	}
	if len(token.Color.PreviousChainingTraceIDs) > 0 {
		previous := append([]string(nil), token.Color.PreviousChainingTraceIDs...)
		resp.PreviousChainingTraceIds = &previous
	}
	if len(token.Color.Tags) > 0 {
		tags := factoryapi.StringMap(token.Color.Tags)
		resp.Tags = &tags
	}
	return resp
}

func categorizeProjectionTokens(
	marking *petri.MarkingSnapshot,
	net *state.Net,
) (factoryapi.StatusCategories, []factoryapi.ResourceUsage) {
	var categories factoryapi.StatusCategories
	resourceCounts := make(map[string]int)
	resourceTotals := resourceTotalsFromTopology(net)
	if marking == nil {
		return categories, resourceUsage(resourceCounts, resourceTotals)
	}
	for _, token := range marking.Tokens {
		if token == nil || interfaces.IsSystemTimeToken(token) {
			continue
		}
		if token.Color.DataType == interfaces.DataTypeResource {
			resourceID, resourceState := state.SplitPlaceID(token.PlaceID)
			if _, ok := resourceTotals[resourceID]; !ok {
				resourceTotals[resourceID]++
			}
			if resourceState == interfaces.ResourceStateAvailable {
				resourceCounts[resourceID]++
			}
			continue
		}
		switch projectionStateCategory(net, token.PlaceID) {
		case state.StateCategoryFailed:
			categories.Failed++
		case state.StateCategoryTerminal:
			categories.Terminal++
		case state.StateCategoryInitial:
			categories.Initial++
		default:
			categories.Processing++
		}
	}
	return categories, resourceUsage(resourceCounts, resourceTotals)
}

func countProjectionTokens(marking *petri.MarkingSnapshot) int {
	if marking == nil {
		return 0
	}
	count := 0
	for _, token := range marking.Tokens {
		if token == nil || interfaces.IsSystemTimeToken(token) {
			continue
		}
		count++
	}
	return count
}

func projectionStateCategory(net *state.Net, placeID string) state.StateCategory {
	if net == nil {
		return state.StateCategoryProcessing
	}
	return net.StateCategoryForPlace(placeID)
}

func resourceTotalsFromTopology(net *state.Net) map[string]int {
	totals := make(map[string]int)
	if net == nil {
		return totals
	}
	for id, resource := range net.Resources {
		if resource == nil {
			continue
		}
		totals[id] = resource.Capacity
	}
	return totals
}

func resourceUsage(counts map[string]int, totals map[string]int) []factoryapi.ResourceUsage {
	ids := make([]string, 0, len(totals))
	for id := range totals {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	resources := make([]factoryapi.ResourceUsage, 0, len(ids))
	for _, id := range ids {
		resources = append(resources, factoryapi.ResourceUsage{
			Available: counts[id],
			Name:      id,
			Total:     totals[id],
		})
	}
	return resources
}

func digestJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// EnabledTransitionsForSnapshot evaluates Petri enablement for one engine snapshot.
func EnabledTransitionsForSnapshot(
	ctx context.Context,
	snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
	runtimeCfg interfaces.RuntimeDefinitionLookup,
) []interfaces.EnabledTransition {
	if snapshot == nil || snapshot.Topology == nil {
		return nil
	}
	evaluator := scheduler.NewEnablementEvaluator(
		nil,
		scheduler.WithEnablementRuntimeConfig(runtimeCfg),
	)
	return evaluator.FindEnabledTransitions(ctx, snapshot.Topology, &snapshot.Marking)
}
