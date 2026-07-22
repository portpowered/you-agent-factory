package factorysessions

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"maps"
	"sort"
	"strings"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

// ProjectionContext carries the runtime inputs needed to project one live session.
type ProjectionContext struct {
	Session                *LiveSession
	FactoryCfg             *interfaces.FactoryConfig
	Snapshot               *factory.StateSnapshot
	LifecycleControlStatus string
	BackendScopeID         string
	LogicalSessionKeyID    string
	NormalizedTarget       *RuntimeLogicalTarget
	RuntimeStartedAt       time.Time
	Enabled                []interfaces.EnabledTransition
	JavaScript             *interfaces.FactorySessionJavaScriptRuntimeState
	JavaScriptSession      *interfaces.FactoryWorldSessionBracketState
	JavaScriptCheckpoints  []interfaces.JavaScriptCheckpointRecord
	Now                    time.Time
}

// ReadProjection carries one live Factory Session read and records whether its
// runtime projection was available. List reads retain the summary when runtime
// projection fails; detail reads return that failure directly.
type ReadProjection struct {
	Context          ProjectionContext
	Runtime          RuntimeProjection
	RuntimeAvailable bool
}

// SessionProjection is the Factory Sessions-owned result for one live session
// detail read. Transports receive the completed runtime projection and only
// convert it to their public representation.
type SessionProjection struct {
	Context ProjectionContext
	Runtime RuntimeProjection
}

// ProjectRuntimeContract builds the Factory Session-owned orchestrator-aware
// runtime projection for one session.
func ProjectRuntimeContract(ctx ProjectionContext) RuntimeProjection {
	now := ctx.Now
	kind := interfaces.EffectiveOrchestratorKind(ctx.FactoryCfg)
	runtime := RuntimeProjection{
		OrchestratorKind: kind,
		Status:           projectedSessionStatus(ctx),
		Progress:         projectedSessionProgress(ctx),
		Usage:            projectedSessionUsage(ctx),
		Lifecycle:        projectedSessionLifecycle(ctx, now),
	}
	if streamIdentity := projectedSessionStreamIdentity(ctx, runtime.Lifecycle); streamIdentity != nil {
		runtime.StreamIdentity = streamIdentity
	}
	if lifecycleControlStatus := projectedSessionLifecycleControlStatus(ctx); lifecycleControlStatus != "" {
		status := lifecycleControlStatus
		runtime.LifecycleControlStatus = &status
	}
	projectOrchestratorIdentity(&runtime, ctx.FactoryCfg)
	switch kind {
	case interfaces.OrchestratorKindJavaScript:
		runtime.JavaScript = projectedJavaScriptRuntime(ctx)
	default:
		runtime.Petri = projectedPetriRuntime(ctx)
	}
	artifacts := projectedSessionArtifacts(ctx, kind)
	runtime.Artifacts = artifacts
	sessionID := ""
	if ctx.Session != nil {
		sessionID = CanonicalFactorySessionID(ctx.Session)
	}
	runtime.StopSummary = ProjectFactorySessionStopSummary(sessionID, ctx.Snapshot, ctx.JavaScript)
	return runtime
}

func projectedSessionStreamIdentity(
	ctx ProjectionContext,
	lifecycle RuntimeLifecycle,
) *RuntimeStreamIdentity {
	sessionID := ""
	if ctx.Session != nil {
		sessionID = CanonicalFactorySessionID(ctx.Session)
	}
	backendScopeID := strings.TrimSpace(ctx.BackendScopeID)
	if backendScopeID == "" || sessionID == "" {
		return nil
	}
	streamGenerationID := ""
	if ctx.Snapshot != nil {
		streamGenerationID = strings.TrimSpace(ctx.Snapshot.StreamGenerationID)
	}
	if streamGenerationID == "" {
		if lifecycle.StartedAt.IsZero() {
			return nil
		}
		streamGenerationID = lifecycle.StartedAt.UTC().Format(time.RFC3339Nano)
	}
	logicalSessionKeyID := strings.TrimSpace(ctx.LogicalSessionKeyID)
	if logicalSessionKeyID == "" {
		return nil
	}
	return &RuntimeStreamIdentity{
		BackendScopeID:      backendScopeID,
		LogicalSessionKeyID: logicalSessionKeyID,
		FactorySessionID:    sessionID,
		StreamGenerationID:  streamGenerationID,
	}
}

func projectedSessionArtifacts(
	ctx ProjectionContext,
	kind string,
) *[]interfaces.FactoryArtifact {
	switch kind {
	case interfaces.OrchestratorKindJavaScript:
		if ctx.JavaScript == nil {
			return nil
		}
		artifactStates := ArtifactStatesFromJavaScriptRuntime(ctx.JavaScriptCheckpoints, ctx.JavaScript.Artifacts)
		return projectedArtifacts(artifactStates)
	default:
		return nil
	}
}

func projectOrchestratorIdentity(runtime *RuntimeProjection, cfg *interfaces.FactoryConfig) {
	if runtime == nil || cfg == nil || cfg.Orchestrator == nil {
		return
	}
	if cfg.Orchestrator.JavaScript != nil {
		js := cfg.Orchestrator.JavaScript
		runtime.Dialect = stringPointerOrNil(js.Dialect)
		runtime.SourceRef = stringPointerOrNil(js.SourceRef)
		runtime.SourceHash = stringPointerOrNil(js.SourceHash)
		if policyHash := factory.HashJavaScriptPolicyDocument(js.DefaultPolicy); policyHash != "" {
			runtime.PolicyHash = &policyHash
		}
		if budgets := projectedJavaScriptBudgets(js.DefaultPolicy); budgets != nil {
			runtime.Budgets = budgets
		}
	}
}

func projectedSessionStatus(ctx ProjectionContext) string {
	if ctx.Snapshot == nil {
		if ctx.JavaScript != nil && strings.TrimSpace(ctx.JavaScript.ScriptStatus) == "FINISHED" {
			return string(interfaces.RuntimeStatusFinished)
		}
		return string(interfaces.RuntimeStatusIdle)
	}
	return string(ctx.Snapshot.RuntimeStatus)
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

func projectedSessionProgress(ctx ProjectionContext) RuntimeProgress {
	if ctx.Snapshot == nil {
		return RuntimeProgress{
			FactoryState:  projectedSessionFactoryState(ctx),
			Categories:    RuntimeStatusCategories{},
			InFlightCount: 0,
			TotalTokens:   0,
		}
	}
	categories, _ := categorizeProjectionTokens(&ctx.Snapshot.Marking, ctx.Snapshot.Topology)
	return RuntimeProgress{
		FactoryState:  projectedSessionFactoryState(ctx),
		Categories:    categories,
		InFlightCount: ctx.Snapshot.InFlightCount,
		TotalTokens:   countProjectionTokens(&ctx.Snapshot.Marking),
	}
}

func projectedSessionUsage(ctx ProjectionContext) RuntimeUsage {
	if ctx.Snapshot == nil {
		return RuntimeUsage{Resources: []RuntimeResourceUsage{}}
	}
	_, resources := categorizeProjectionTokens(&ctx.Snapshot.Marking, ctx.Snapshot.Topology)
	return RuntimeUsage{Resources: resources}
}

func projectedSessionLifecycle(ctx ProjectionContext, now time.Time) RuntimeLifecycle {
	startedAt := now
	if ctx.Snapshot != nil && ctx.Snapshot.Uptime > 0 {
		startedAt = now.Add(-ctx.Snapshot.Uptime).UTC()
	} else if !ctx.RuntimeStartedAt.IsZero() {
		startedAt = ctx.RuntimeStartedAt.UTC()
	}
	lifecycle := RuntimeLifecycle{
		StartedAt: startedAt,
		UpdatedAt: now.UTC(),
	}
	if ctx.Snapshot != nil && ctx.Snapshot.RuntimeStatus == interfaces.RuntimeStatusFinished {
		finishedAt := now.UTC()
		lifecycle.FinishedAt = &finishedAt
	}
	return lifecycle
}

func projectedPetriRuntime(ctx ProjectionContext) *PetriRuntimeProjection {
	if ctx.Snapshot == nil {
		return &PetriRuntimeProjection{
			Marking:            []RuntimeToken{},
			EnabledTransitions: []PetriEnabledTransition{},
		}
	}
	return &PetriRuntimeProjection{
		Marking:            projectedMarkingTokens(&ctx.Snapshot.Marking),
		EnabledTransitions: projectedEnabledTransitions(ctx.Enabled),
	}
}

func projectedJavaScriptRuntime(ctx ProjectionContext) *JavaScriptRuntimeProjection {
	state := ctx.JavaScript
	if state == nil {
		state = defaultJavaScriptRuntimeState(ctx.FactoryCfg)
	}
	projection := &JavaScriptRuntimeProjection{
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

func projectedJavaScriptScriptStatus(value string) interfaces.FactorySessionJavaScriptScriptStatus {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "RUNNING":
		return interfaces.FactorySessionJavaScriptScriptStatusRunning
	case "PAUSED":
		return interfaces.FactorySessionJavaScriptScriptStatusPaused
	case "FINISHED":
		return interfaces.FactorySessionJavaScriptScriptStatusFinished
	case "FAILED":
		return interfaces.FactorySessionJavaScriptScriptStatusFailed
	default:
		return interfaces.FactorySessionJavaScriptScriptStatusIdle
	}
}

func projectedJavaScriptChildDispatchCounts(
	state *interfaces.FactorySessionJavaScriptRuntimeState,
) interfaces.FactorySessionChildDispatchCounts {
	if state == nil {
		return interfaces.FactorySessionChildDispatchCounts{}
	}
	return interfaces.FactorySessionChildDispatchCounts{
		Queued:    state.QueuedDispatches,
		Running:   state.RunningDispatches,
		Completed: state.CompletedDispatches,
	}
}

func projectedJavaScriptCheckpoints(
	checkpoints []interfaces.FactorySessionJavaScriptCheckpointRef,
) []interfaces.FactorySessionJavaScriptCheckpointEventRef {
	if len(checkpoints) == 0 {
		return nil
	}
	projected := make([]interfaces.FactorySessionJavaScriptCheckpointEventRef, 0, len(checkpoints))
	for _, checkpoint := range checkpoints {
		item := interfaces.FactorySessionJavaScriptCheckpointEventRef{ID: checkpoint.ID}
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

func projectedCheckpointArtifactRef(ref interfaces.JavaScriptCheckpointArtifactRef) interfaces.FactoryArtifactRef {
	artifactID := strings.TrimSpace(ref.ID)
	kind := strings.TrimSpace(ref.Kind)
	if kind == "" {
		kind = "CHECKPOINT"
	}
	visibility := strings.TrimSpace(ref.Visibility)
	if visibility == "" {
		visibility = "INTERNAL_CHECKPOINT"
	}
	projected := interfaces.FactoryArtifactRef{
		ID:         artifactID,
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

func projectedJavaScriptBudgets(raw json.RawMessage) *RuntimeBudgets {
	resolution := factory.ResolveJavaScriptPolicy(factory.JavaScriptPolicyRequest{FactoryDefault: raw})
	if resolution.Policy.MaxAgents <= 0 {
		return nil
	}
	value := resolution.Policy.MaxAgents
	return &RuntimeBudgets{MaxAgents: &value}
}

func projectedMarkingTokens(marking *factory.PetriMarkingSnapshot) []RuntimeToken {
	if marking == nil || len(marking.Tokens) == 0 {
		return []RuntimeToken{}
	}
	tokenIDs := make([]string, 0, len(marking.Tokens))
	for tokenID := range marking.Tokens {
		tokenIDs = append(tokenIDs, tokenID)
	}
	sort.Strings(tokenIDs)
	tokens := make([]RuntimeToken, 0, len(tokenIDs))
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
) []PetriEnabledTransition {
	if len(enabled) == 0 {
		return []PetriEnabledTransition{}
	}
	projected := make([]PetriEnabledTransition, 0, len(enabled))
	for _, transition := range enabled {
		projected = append(projected, PetriEnabledTransition{
			TransitionID: transition.TransitionID,
			WorkerType:   transition.WorkerType,
		})
	}
	sort.Slice(projected, func(i, j int) bool {
		if projected[i].TransitionID == projected[j].TransitionID {
			return projected[i].WorkerType < projected[j].WorkerType
		}
		return projected[i].TransitionID < projected[j].TransitionID
	})
	return projected
}

func projectedTokenResponse(token *workerexecution.Token) RuntimeToken {
	resp := RuntimeToken{
		ID:        token.ID,
		PlaceID:   token.PlaceID,
		WorkID:    token.Color.WorkID,
		WorkType:  token.Color.WorkTypeID,
		TraceID:   token.Color.TraceID,
		CreatedAt: token.CreatedAt,
		EnteredAt: token.EnteredAt,
		History: &RuntimeTokenHistory{
			ConsecutiveFailures: maps.Clone(token.History.ConsecutiveFailures),
			LastError:           token.History.LastError,
			PlaceVisits:         maps.Clone(token.History.PlaceVisits),
			TotalVisits:         maps.Clone(token.History.TotalVisits),
		},
	}
	if token.Color.Name != "" {
		resp.Name = &token.Color.Name
	}
	if token.Color.ChainingTraceDepth > 0 {
		depth := token.Color.ChainingTraceDepth
		resp.ChainingTraceDepth = &depth
	}
	if traceID := firstNonEmptyString(token.Color.CurrentChainingTraceID, token.Color.TraceID); traceID != "" {
		resp.CurrentChainingTraceID = &traceID
	}
	if len(token.Color.PreviousChainingTraceIDs) > 0 {
		previous := append([]string(nil), token.Color.PreviousChainingTraceIDs...)
		resp.PreviousChainingTraceIDs = &previous
	}
	if len(token.Color.Tags) > 0 {
		tags := maps.Clone(token.Color.Tags)
		resp.Tags = &tags
	}
	return resp
}

func categorizeProjectionTokens(
	marking *factory.PetriMarkingSnapshot,
	net *factory.RuntimeNet,
) (RuntimeStatusCategories, []RuntimeResourceUsage) {
	var categories RuntimeStatusCategories
	resourceCounts := make(map[string]int)
	resourceTotals := resourceTotalsFromTopology(net)
	if marking == nil {
		return categories, resourceUsage(resourceCounts, resourceTotals)
	}
	for _, token := range marking.Tokens {
		if token == nil || interfaces.IsSystemTimeToken(token) {
			continue
		}
		if token.Color.DataType == workerexecution.DataTypeResource {
			resourceID, resourceState := factory.SplitPlaceID(token.PlaceID)
			if _, ok := resourceTotals[resourceID]; !ok {
				resourceTotals[resourceID]++
			}
			if resourceState == interfaces.ResourceStateAvailable {
				resourceCounts[resourceID]++
			}
			continue
		}
		switch projectionStateCategory(net, token.PlaceID) {
		case factory.StateCategoryFailed:
			categories.Failed++
		case factory.StateCategoryTerminal:
			categories.Terminal++
		case factory.StateCategoryInitial:
			categories.Initial++
		default:
			categories.Processing++
		}
	}
	return categories, resourceUsage(resourceCounts, resourceTotals)
}

func countProjectionTokens(marking *factory.PetriMarkingSnapshot) int {
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

func projectionStateCategory(net *factory.RuntimeNet, placeID string) factory.StateCategory {
	if net == nil {
		return factory.StateCategoryProcessing
	}
	return net.StateCategoryForPlace(placeID)
}

func resourceTotalsFromTopology(net *factory.RuntimeNet) map[string]int {
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

func resourceUsage(counts map[string]int, totals map[string]int) []RuntimeResourceUsage {
	ids := make([]string, 0, len(totals))
	for id := range totals {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	resources := make([]RuntimeResourceUsage, 0, len(ids))
	for _, id := range ids {
		resources = append(resources, RuntimeResourceUsage{
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
