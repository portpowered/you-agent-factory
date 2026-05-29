package interfaces

import (
	"fmt"
	"sort"
	"strings"
)

// WorkPayloadLineageProjection is the canonical replay-safe source of truth for
// user-visible work payload history. It records submitted and response-produced
// payload snapshots, plus the dispatch-time snapshot actually consumed by each
// workstation request.
type WorkPayloadLineageProjection struct {
	SnapshotsByID                    map[string]WorkPayloadSnapshot       `json:"snapshots_by_id,omitempty"`
	InitialSnapshotIDByWorkID        map[string]string                    `json:"initial_snapshot_id_by_work_id,omitempty"`
	LatestSnapshotIDByWorkID         map[string]string                    `json:"latest_snapshot_id_by_work_id,omitempty"`
	ConsumedSnapshotRefsByDispatchID map[string]map[string]WorkPayloadRef `json:"consumed_snapshot_refs_by_dispatch_id,omitempty"`
	OutputSnapshotRefsByDispatchID   map[string]map[string]WorkPayloadRef `json:"output_snapshot_refs_by_dispatch_id,omitempty"`
	SnapshotIDsByWorkID              map[string][]string                  `json:"snapshot_ids_by_work_id,omitempty"`
}

// WorkPayloadSnapshot is one immutable payload-bearing work view captured from
// a canonical event class.
type WorkPayloadSnapshot struct {
	SnapshotID           string                  `json:"snapshot_id"`
	WorkID               string                  `json:"work_id"`
	LogicalWorkID        string                  `json:"logical_work_id"`
	SourceKind           WorkPayloadSnapshotKind `json:"source_kind"`
	SourceEventType      string                  `json:"source_event_type"`
	RequestID            string                  `json:"request_id,omitempty"`
	DispatchID           string                  `json:"dispatch_id,omitempty"`
	ObservedTick         int                     `json:"observed_tick,omitempty"`
	Continuity           WorkPayloadContinuity   `json:"continuity"`
	ParentSnapshotIDs    []string                `json:"parent_snapshot_ids,omitempty"`
	ParentWorkIDs        []string                `json:"parent_work_ids,omitempty"`
	ParentLogicalWorkIDs []string                `json:"parent_logical_work_ids,omitempty"`
	WorkItem             FactoryWorkItem         `json:"work_item"`
}

type WorkPayloadSnapshotKind string

const (
	WorkPayloadSnapshotKindWorkRequest    WorkPayloadSnapshotKind = "WORK_REQUEST"
	WorkPayloadSnapshotKindDispatchOutput WorkPayloadSnapshotKind = "DISPATCH_RESPONSE_OUTPUT"
)

type WorkPayloadContinuity string

const (
	WorkPayloadContinuityInitial           WorkPayloadContinuity = "INITIAL_SUBMISSION"
	WorkPayloadContinuitySameWorkID        WorkPayloadContinuity = "SAME_WORK_ID_CONTINUATION"
	WorkPayloadContinuityNewDownstreamWork WorkPayloadContinuity = "NEW_DOWNSTREAM_WORK"
)

type WorkPayloadResolutionStatus string

const (
	WorkPayloadResolutionResolved    WorkPayloadResolutionStatus = "RESOLVED"
	WorkPayloadResolutionUnavailable WorkPayloadResolutionStatus = "UNAVAILABLE"
)

// WorkPayloadRef points at the canonical snapshot chosen for one specific
// lineage lookup context.
type WorkPayloadRef struct {
	Status     WorkPayloadResolutionStatus `json:"status"`
	SnapshotID string                      `json:"snapshot_id,omitempty"`
	Reason     string                      `json:"reason,omitempty"`
}

// WorkPayloadResolution is the caller-facing answer for one lineage lookup.
type WorkPayloadResolution struct {
	Status   WorkPayloadResolutionStatus `json:"status"`
	Reason   string                      `json:"reason,omitempty"`
	Snapshot *WorkPayloadSnapshot        `json:"snapshot,omitempty"`
}

func (p *WorkPayloadLineageProjection) ensureMaps() {
	if p.SnapshotsByID == nil {
		p.SnapshotsByID = make(map[string]WorkPayloadSnapshot)
	}
	if p.InitialSnapshotIDByWorkID == nil {
		p.InitialSnapshotIDByWorkID = make(map[string]string)
	}
	if p.LatestSnapshotIDByWorkID == nil {
		p.LatestSnapshotIDByWorkID = make(map[string]string)
	}
	if p.ConsumedSnapshotRefsByDispatchID == nil {
		p.ConsumedSnapshotRefsByDispatchID = make(map[string]map[string]WorkPayloadRef)
	}
	if p.OutputSnapshotRefsByDispatchID == nil {
		p.OutputSnapshotRefsByDispatchID = make(map[string]map[string]WorkPayloadRef)
	}
	if p.SnapshotIDsByWorkID == nil {
		p.SnapshotIDsByWorkID = make(map[string][]string)
	}
}

// RecordWorkRequestSnapshot captures a canonical payload-bearing WORK_REQUEST
// snapshot. This is the precedence owner for initial-submission lookups.
func (p *WorkPayloadLineageProjection) RecordWorkRequestSnapshot(observedTick int, requestID string, item FactoryWorkItem) {
	if item.ID == "" {
		return
	}
	p.ensureMaps()
	logicalWorkID := item.ID
	continuity := WorkPayloadContinuityInitial
	parentSnapshotIDs := []string(nil)
	parentWorkIDs := []string(nil)
	parentLogicalWorkIDs := []string(nil)
	if latest := p.snapshotByID(p.LatestSnapshotIDByWorkID[item.ID]); latest != nil {
		logicalWorkID = latest.LogicalWorkID
		continuity = WorkPayloadContinuitySameWorkID
		parentSnapshotIDs = append(parentSnapshotIDs, latest.SnapshotID)
		parentWorkIDs = append(parentWorkIDs, latest.WorkID)
		parentLogicalWorkIDs = append(parentLogicalWorkIDs, latest.LogicalWorkID)
	}
	snapshot := WorkPayloadSnapshot{
		SnapshotID:           fmt.Sprintf("work-request:%s:%s:%d", requestID, item.ID, len(p.SnapshotIDsByWorkID[item.ID])+1),
		WorkID:               item.ID,
		LogicalWorkID:        logicalWorkID,
		SourceKind:           WorkPayloadSnapshotKindWorkRequest,
		SourceEventType:      string(WorkPayloadSnapshotKindWorkRequest),
		RequestID:            requestID,
		ObservedTick:         observedTick,
		Continuity:           continuity,
		ParentSnapshotIDs:    parentSnapshotIDs,
		ParentWorkIDs:        parentWorkIDs,
		ParentLogicalWorkIDs: parentLogicalWorkIDs,
		WorkItem:             cloneLineageWorkItem(item),
	}
	p.SnapshotsByID[snapshot.SnapshotID] = snapshot
	if _, exists := p.InitialSnapshotIDByWorkID[item.ID]; !exists {
		p.InitialSnapshotIDByWorkID[item.ID] = snapshot.SnapshotID
	}
	p.LatestSnapshotIDByWorkID[item.ID] = snapshot.SnapshotID
	p.SnapshotIDsByWorkID[item.ID] = append(p.SnapshotIDsByWorkID[item.ID], snapshot.SnapshotID)
}

// RecordConsumedInputSnapshot fixes the dispatch-time payload snapshot that
// actually fed one dispatch input. This is the precedence owner for
// consumed-input lookups and must not be recomputed from later latest state.
func (p *WorkPayloadLineageProjection) RecordConsumedInputSnapshot(dispatchID string, item FactoryWorkItem) {
	if dispatchID == "" || item.ID == "" {
		return
	}
	p.ensureMaps()
	dispatchRefs := p.ConsumedSnapshotRefsByDispatchID[dispatchID]
	if dispatchRefs == nil {
		dispatchRefs = make(map[string]WorkPayloadRef)
		p.ConsumedSnapshotRefsByDispatchID[dispatchID] = dispatchRefs
	}
	if latest := p.snapshotByID(p.LatestSnapshotIDByWorkID[item.ID]); latest != nil {
		dispatchRefs[item.ID] = WorkPayloadRef{
			Status:     WorkPayloadResolutionResolved,
			SnapshotID: latest.SnapshotID,
		}
		return
	}
	dispatchRefs[item.ID] = WorkPayloadRef{
		Status: WorkPayloadResolutionUnavailable,
		Reason: "no lineage snapshot was recorded before this dispatch consumed the work item",
	}
}

// RecordDispatchOutputSnapshot captures the payload-bearing work snapshot
// introduced by a DISPATCH_RESPONSE output and records its continuation
// relationship to the dispatch's consumed lineage context.
func (p *WorkPayloadLineageProjection) RecordDispatchOutputSnapshot(
	observedTick int,
	dispatchID string,
	consumedInputs []FactoryWorkItem,
	item FactoryWorkItem,
	outputIndex int,
) {
	if dispatchID == "" || item.ID == "" {
		return
	}
	p.ensureMaps()
	parentSnapshots := p.resolvedConsumedSnapshotsForDispatch(dispatchID)
	parentSnapshotIDs := make([]string, 0, len(parentSnapshots))
	parentWorkIDs := make([]string, 0, len(parentSnapshots))
	parentLogicalWorkIDs := make([]string, 0, len(parentSnapshots))
	for _, snapshot := range parentSnapshots {
		parentSnapshotIDs = appendUniqueString(parentSnapshotIDs, snapshot.SnapshotID)
		parentWorkIDs = appendUniqueString(parentWorkIDs, snapshot.WorkID)
		parentLogicalWorkIDs = appendUniqueString(parentLogicalWorkIDs, snapshot.LogicalWorkID)
	}
	logicalWorkID := item.ID
	continuity := WorkPayloadContinuityNewDownstreamWork
	for _, snapshot := range parentSnapshots {
		if snapshot.WorkID == item.ID {
			logicalWorkID = snapshot.LogicalWorkID
			continuity = WorkPayloadContinuitySameWorkID
			break
		}
	}
	if continuity == WorkPayloadContinuityNewDownstreamWork && len(parentSnapshots) == 0 {
		for _, input := range consumedInputs {
			if input.ID == item.ID {
				logicalWorkID = item.ID
				continuity = WorkPayloadContinuitySameWorkID
				break
			}
		}
	}
	snapshot := WorkPayloadSnapshot{
		SnapshotID:           fmt.Sprintf("dispatch-output:%s:%s:%d", dispatchID, item.ID, outputIndex),
		WorkID:               item.ID,
		LogicalWorkID:        logicalWorkID,
		SourceKind:           WorkPayloadSnapshotKindDispatchOutput,
		SourceEventType:      string(WorkPayloadSnapshotKindDispatchOutput),
		DispatchID:           dispatchID,
		ObservedTick:         observedTick,
		Continuity:           continuity,
		ParentSnapshotIDs:    parentSnapshotIDs,
		ParentWorkIDs:        parentWorkIDs,
		ParentLogicalWorkIDs: parentLogicalWorkIDs,
		WorkItem:             cloneLineageWorkItem(item),
	}
	p.SnapshotsByID[snapshot.SnapshotID] = snapshot
	p.LatestSnapshotIDByWorkID[item.ID] = snapshot.SnapshotID
	p.SnapshotIDsByWorkID[item.ID] = append(p.SnapshotIDsByWorkID[item.ID], snapshot.SnapshotID)
	dispatchRefs := p.OutputSnapshotRefsByDispatchID[dispatchID]
	if dispatchRefs == nil {
		dispatchRefs = make(map[string]WorkPayloadRef)
		p.OutputSnapshotRefsByDispatchID[dispatchID] = dispatchRefs
	}
	dispatchRefs[item.ID] = WorkPayloadRef{
		Status:     WorkPayloadResolutionResolved,
		SnapshotID: snapshot.SnapshotID,
	}
}

// ResolveInitialSubmittedSnapshot chooses the original submitted snapshot for a
// work item. This never falls forward to later dispatch outputs.
func (p WorkPayloadLineageProjection) ResolveInitialSubmittedSnapshot(workID string) WorkPayloadResolution {
	return p.resolveSnapshotID(p.InitialSnapshotIDByWorkID[workID], "no initial work-request payload snapshot was recorded for this work item")
}

// ResolveConsumedInputSnapshot chooses the dispatch-time consumed snapshot for
// one dispatch/work pair. This never falls forward to the latest known state.
func (p WorkPayloadLineageProjection) ResolveConsumedInputSnapshot(dispatchID, workID string) WorkPayloadResolution {
	dispatchRefs := p.ConsumedSnapshotRefsByDispatchID[dispatchID]
	if dispatchRefs == nil {
		return unavailableWorkPayloadResolution("no consumed-input lineage was recorded for this dispatch")
	}
	ref, ok := dispatchRefs[workID]
	if !ok {
		return unavailableWorkPayloadResolution("the dispatch did not record a consumed lineage snapshot for this work item")
	}
	return p.resolveRef(ref)
}

// ResolveSelectedWorkSnapshot chooses the latest known payload snapshot for the
// current selectable state of a work item.
func (p WorkPayloadLineageProjection) ResolveSelectedWorkSnapshot(workID string) WorkPayloadResolution {
	return p.resolveSnapshotID(p.LatestSnapshotIDByWorkID[workID], "no lineage snapshot is available for this work item")
}

// ResolveOutputWorkSnapshot chooses the response-produced payload snapshot for
// one dispatch/work pair. This never falls back to a later selected-work view.
func (p WorkPayloadLineageProjection) ResolveOutputWorkSnapshot(dispatchID, workID string) WorkPayloadResolution {
	dispatchRefs := p.OutputSnapshotRefsByDispatchID[dispatchID]
	if dispatchRefs == nil {
		return unavailableWorkPayloadResolution("no output-work lineage was recorded for this dispatch")
	}
	ref, ok := dispatchRefs[workID]
	if !ok {
		return unavailableWorkPayloadResolution("the dispatch did not record an output lineage snapshot for this work item")
	}
	return p.resolveRef(ref)
}

func (p WorkPayloadLineageProjection) resolveRef(ref WorkPayloadRef) WorkPayloadResolution {
	if ref.Status == WorkPayloadResolutionUnavailable {
		return unavailableWorkPayloadResolution(ref.Reason)
	}
	return p.resolveSnapshotID(ref.SnapshotID, ref.Reason)
}

func (p WorkPayloadLineageProjection) resolveSnapshotID(snapshotID string, unavailableReason string) WorkPayloadResolution {
	snapshot := p.snapshotByID(snapshotID)
	if snapshot == nil {
		return unavailableWorkPayloadResolution(unavailableReason)
	}
	cloned := cloneLineageSnapshot(*snapshot)
	return WorkPayloadResolution{
		Status:   WorkPayloadResolutionResolved,
		Snapshot: &cloned,
	}
}

func unavailableWorkPayloadResolution(reason string) WorkPayloadResolution {
	return WorkPayloadResolution{
		Status: WorkPayloadResolutionUnavailable,
		Reason: reason,
	}
}

func (p WorkPayloadLineageProjection) snapshotByID(snapshotID string) *WorkPayloadSnapshot {
	if snapshotID == "" {
		return nil
	}
	snapshot, ok := p.SnapshotsByID[snapshotID]
	if !ok {
		return nil
	}
	return &snapshot
}

func (p WorkPayloadLineageProjection) resolvedConsumedSnapshotsForDispatch(dispatchID string) []WorkPayloadSnapshot {
	dispatchRefs := p.ConsumedSnapshotRefsByDispatchID[dispatchID]
	if len(dispatchRefs) == 0 {
		return nil
	}
	snapshots := make([]WorkPayloadSnapshot, 0, len(dispatchRefs))
	for _, workID := range sortedStringKeys(dispatchRefs) {
		ref := dispatchRefs[workID]
		if ref.Status != WorkPayloadResolutionResolved {
			continue
		}
		snapshot := p.snapshotByID(ref.SnapshotID)
		if snapshot == nil {
			continue
		}
		snapshots = append(snapshots, *snapshot)
	}
	return snapshots
}

func cloneLineageSnapshot(snapshot WorkPayloadSnapshot) WorkPayloadSnapshot {
	snapshot.ParentSnapshotIDs = cloneStringSlice(snapshot.ParentSnapshotIDs)
	snapshot.ParentWorkIDs = cloneStringSlice(snapshot.ParentWorkIDs)
	snapshot.ParentLogicalWorkIDs = cloneStringSlice(snapshot.ParentLogicalWorkIDs)
	snapshot.WorkItem = cloneLineageWorkItem(snapshot.WorkItem)
	return snapshot
}

func cloneLineageWorkItem(item FactoryWorkItem) FactoryWorkItem {
	item.PreviousChainingTraceIDs = cloneStringSlice(item.PreviousChainingTraceIDs)
	item.Content = cloneWorkContentParts(item.Content)
	item.Tags = cloneStringMap(item.Tags)
	return item
}

func sortedStringKeys(values map[string]WorkPayloadRef) []string {
	if len(values) == 0 {
		return nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func appendUniqueString(values []string, value string) []string {
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

// CanonicalChainingTraceIDs applies the shared chaining-trace fan-in rule:
// keep only non-empty predecessor chain IDs, dedupe them, and sort them.
func CanonicalChainingTraceIDs(traceIDs []string) []string {
	if len(traceIDs) == 0 {
		return nil
	}

	ordered := append([]string(nil), traceIDs...)
	sort.Strings(ordered)

	deduped := ordered[:0]
	var previous string
	for i, traceID := range ordered {
		if traceID == "" || (i > 0 && traceID == previous) {
			previous = traceID
			continue
		}
		deduped = append(deduped, traceID)
		previous = traceID
	}
	if len(deduped) == 0 {
		return nil
	}
	return deduped
}

// PreviousChainingTraceIDsFromTokens collects predecessor chain IDs from
// non-resource input tokens using the canonical fan-in ordering rule.
func PreviousChainingTraceIDsFromTokens(tokens []Token) []string {
	traceIDs := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if token.Color.DataType == DataTypeResource {
			continue
		}
		traceIDs = append(traceIDs, firstNonEmptyString(token.Color.CurrentChainingTraceID, token.Color.TraceID))
	}
	return CanonicalChainingTraceIDs(traceIDs)
}

// PreviousChainingTraceIDsFromTokenColors collects predecessor chain IDs from
// non-resource token colors using the canonical fan-in ordering rule.
func PreviousChainingTraceIDsFromTokenColors(colors []TokenColor) []string {
	traceIDs := make([]string, 0, len(colors))
	for _, color := range colors {
		if color.DataType == DataTypeResource {
			continue
		}
		traceIDs = append(traceIDs, firstNonEmptyString(color.CurrentChainingTraceID, color.TraceID))
	}
	return CanonicalChainingTraceIDs(traceIDs)
}

// PreviousChainingTraceIDsFromWorkItems collects predecessor chain IDs from
// canonical work items using the shared deterministic fan-in rule.
func PreviousChainingTraceIDsFromWorkItems(items []FactoryWorkItem) []string {
	traceIDs := make([]string, 0, len(items))
	for _, item := range items {
		traceIDs = append(traceIDs, firstNonEmptyString(item.CurrentChainingTraceID, item.TraceID))
	}
	return CanonicalChainingTraceIDs(traceIDs)
}

// CurrentChainingTraceIDFromWorkItems resolves the current dispatch chain from
// the first non-system customer work item, falling back to any work item when
// only system work is present.
func CurrentChainingTraceIDFromWorkItems(items []FactoryWorkItem) string {
	for _, item := range items {
		if IsSystemTimeWorkType(item.WorkTypeID) {
			continue
		}
		if item.CurrentChainingTraceID != "" {
			return item.CurrentChainingTraceID
		}
		if item.TraceID != "" {
			return item.TraceID
		}
	}
	for _, item := range items {
		if item.CurrentChainingTraceID != "" {
			return item.CurrentChainingTraceID
		}
		if item.TraceID != "" {
			return item.TraceID
		}
	}
	return ""
}

// CurrentChainingTraceIDFromTokens resolves the current dispatch chain from
// the first non-resource customer work token, falling back to any non-resource
// token when only system work is present.
func CurrentChainingTraceIDFromTokens(tokens []Token) string {
	for _, token := range tokens {
		if token.Color.DataType == DataTypeResource || token.Color.WorkTypeID == SystemTimeWorkTypeID {
			continue
		}
		return firstNonEmptyString(token.Color.CurrentChainingTraceID, token.Color.TraceID)
	}
	for _, token := range tokens {
		if token.Color.DataType != DataTypeResource {
			return firstNonEmptyString(token.Color.CurrentChainingTraceID, token.Color.TraceID)
		}
	}
	return ""
}

// ChainingTraceDepthForTokenColor returns the stored chain depth for one token
// color, falling back to depth 1 when only a trace identifier is available.
func ChainingTraceDepthForTokenColor(color TokenColor) int {
	if color.ChainingTraceDepth > 0 {
		return color.ChainingTraceDepth
	}
	if firstNonEmptyString(color.CurrentChainingTraceID, color.TraceID) != "" {
		return 1
	}
	return 0
}

// ChainingTraceDepthForWorkItem returns the stored chain depth for one work
// item, falling back to depth 1 when only a trace identifier is available.
func ChainingTraceDepthForWorkItem(item FactoryWorkItem) int {
	if item.ChainingTraceDepth > 0 {
		return item.ChainingTraceDepth
	}
	if firstNonEmptyString(item.CurrentChainingTraceID, item.TraceID) != "" {
		return 1
	}
	return 0
}

// ChainingTraceDepthFromTokenColors resolves the next chain depth from the
// deepest non-resource token color, defaulting initial traced work to depth 1.
func ChainingTraceDepthFromTokenColors(colors []TokenColor) int {
	depth := 0
	for _, color := range colors {
		if color.DataType == DataTypeResource {
			continue
		}
		if candidate := ChainingTraceDepthForTokenColor(color); candidate > depth {
			depth = candidate
		}
	}
	if depth > 0 {
		return depth + 1
	}
	if CurrentChainingTraceIDFromTokenColors(colors) != "" {
		return 1
	}
	return 0
}

func CurrentChainingTraceIDFromTokenColors(colors []TokenColor) string {
	for _, color := range colors {
		if color.DataType == DataTypeResource || color.WorkTypeID == SystemTimeWorkTypeID {
			continue
		}
		return firstNonEmptyString(color.CurrentChainingTraceID, color.TraceID)
	}
	for _, color := range colors {
		if color.DataType != DataTypeResource {
			return firstNonEmptyString(color.CurrentChainingTraceID, color.TraceID)
		}
	}
	return ""
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// RunnerExecutionRequest is the shared runner-owned execution request contract
// used by standard orchestration flows.
type RunnerExecutionRequest = ProviderInferenceRequest

// RunnerExecutionResult is the shared runner-owned execution result contract
// used by standard orchestration flows.
type RunnerExecutionResult = InferenceResponse

// RunnerToolExecutionMode describes whether a runner invocation is expected to
// permit tool execution during the request lifecycle.
type RunnerToolExecutionMode string

const (
	// RunnerToolExecutionModeRequired means the orchestration path expects the
	// selected runner to support tool execution for this request.
	RunnerToolExecutionModeRequired RunnerToolExecutionMode = "required"
	// RunnerToolExecutionModeDisabled means the request does not require tool
	// execution support.
	RunnerToolExecutionModeDisabled RunnerToolExecutionMode = "disabled"
)

// RunnerBaselineCapability is a v1 baseline runner behavior that every built-in
// runner must support for standard orchestration participation.
type RunnerBaselineCapability string

const (
	RunnerBaselineCapabilityPromptSubmission RunnerBaselineCapability = "prompt_submission"
	RunnerBaselineCapabilityToolExecution    RunnerBaselineCapability = "tool_execution"
)

var runnerBaselineCapabilitiesV1 = []RunnerBaselineCapability{
	RunnerBaselineCapabilityPromptSubmission,
	RunnerBaselineCapabilityToolExecution,
}

// V1RunnerBaselineCapabilities returns the explicit v1 baseline capability set.
func V1RunnerBaselineCapabilities() []RunnerBaselineCapability {
	return append([]RunnerBaselineCapability(nil), runnerBaselineCapabilitiesV1...)
}

// RunnerOptionalCapability identifies an execution behavior that may be
// supported by some runners without being required for baseline participation.
type RunnerOptionalCapability string

const (
	RunnerOptionalCapabilityImageInput       RunnerOptionalCapability = "image_input"
	RunnerOptionalCapabilitySessionResume    RunnerOptionalCapability = "session_resume"
	RunnerOptionalCapabilityStructuredOutput RunnerOptionalCapability = "structured_output"
	RunnerOptionalCapabilityWorkingDirectory RunnerOptionalCapability = "working_directory"
	RunnerOptionalCapabilityWorktree         RunnerOptionalCapability = "worktree"
)

// RunnerOptionalCapabilityStatus reports whether a runner can satisfy one
// optional capability.
type RunnerOptionalCapabilityStatus string

const (
	RunnerOptionalCapabilityStatusSupported   RunnerOptionalCapabilityStatus = "supported"
	RunnerOptionalCapabilityStatusUnsupported RunnerOptionalCapabilityStatus = "unsupported"
)

// RunnerOptionalCapabilitySupport is the machine-readable capability status
// shape intended for backend, CLI, API, and UI consumption.
type RunnerOptionalCapabilitySupport struct {
	Capability RunnerOptionalCapability       `json:"capability"`
	Status     RunnerOptionalCapabilityStatus `json:"status"`
	Detail     string                         `json:"detail,omitempty"`
}

// RunnerCapabilities describes one runner's baseline and optional capability
// support in a product-surface-friendly shape.
type RunnerCapabilities struct {
	Baseline []RunnerBaselineCapability        `json:"baseline"`
	Optional []RunnerOptionalCapabilitySupport `json:"optional,omitempty"`
}

// NewRunnerCapabilities creates a capability payload with the explicit v1
// baseline and detached optional capability support entries.
func NewRunnerCapabilities(optional ...RunnerOptionalCapabilitySupport) RunnerCapabilities {
	return RunnerCapabilities{
		Baseline: V1RunnerBaselineCapabilities(),
		Optional: cloneRunnerOptionalCapabilitySupport(optional),
	}
}

// RunnerMetadata is the canonical metadata shape that product surfaces can use
// to inspect one runner's identity and capability support.
type RunnerMetadata struct {
	ID           string             `json:"id"`
	DisplayName  string             `json:"display_name,omitempty"`
	Capabilities RunnerCapabilities `json:"capabilities"`
}

func cloneRunnerOptionalCapabilitySupport(values []RunnerOptionalCapabilitySupport) []RunnerOptionalCapabilitySupport {
	if len(values) == 0 {
		return nil
	}
	clone := make([]RunnerOptionalCapabilitySupport, len(values))
	copy(clone, values)
	return clone
}

const (
	RunnerIDCodex     = "codex"
	RunnerIDGemini    = "gemini"
	RunnerIDKiro      = "kiro"
	RunnerIDCursorCLI = "cursor-cli"
	RunnerIDOpenCode  = "opencode"
)

// RunnerSelectionSource reports which configuration layer supplied the runner
// choice that execution should use.
type RunnerSelectionSource string

const (
	RunnerSelectionSourceWorkstation    RunnerSelectionSource = "workstation"
	RunnerSelectionSourceFactory        RunnerSelectionSource = "factory"
	RunnerSelectionSourceLegacyProvider RunnerSelectionSource = "legacy_provider"
	RunnerSelectionSourceDefault        RunnerSelectionSource = "default"
)

// ResolvedRunnerSelection is the canonical runner-selection result used by
// backend runtime code before dispatch starts.
type ResolvedRunnerSelection struct {
	RunnerID string                `json:"runner_id,omitempty"`
	Source   RunnerSelectionSource `json:"source,omitempty"`
}

var builtInRunnerMetadata = map[string]RunnerMetadata{
	RunnerIDCodex: {
		ID:          RunnerIDCodex,
		DisplayName: "Codex",
		Capabilities: NewRunnerCapabilities(
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityImageInput, Status: RunnerOptionalCapabilityStatusSupported},
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilitySessionResume, Status: RunnerOptionalCapabilityStatusSupported},
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityStructuredOutput, Status: RunnerOptionalCapabilityStatusSupported},
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityWorkingDirectory, Status: RunnerOptionalCapabilityStatusSupported},
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityWorktree, Status: RunnerOptionalCapabilityStatusUnsupported, Detail: "codex rejects workstation worktree selection in v1"},
		),
	},
	RunnerIDGemini: {
		ID:          RunnerIDGemini,
		DisplayName: "Gemini",
		Capabilities: NewRunnerCapabilities(
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityImageInput, Status: RunnerOptionalCapabilityStatusUnsupported},
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilitySessionResume, Status: RunnerOptionalCapabilityStatusUnsupported},
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityStructuredOutput, Status: RunnerOptionalCapabilityStatusUnsupported},
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityWorkingDirectory, Status: RunnerOptionalCapabilityStatusUnsupported},
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityWorktree, Status: RunnerOptionalCapabilityStatusUnsupported},
		),
	},
	RunnerIDKiro: {
		ID:          RunnerIDKiro,
		DisplayName: "Kiro",
		Capabilities: NewRunnerCapabilities(
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityImageInput, Status: RunnerOptionalCapabilityStatusUnsupported},
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilitySessionResume, Status: RunnerOptionalCapabilityStatusSupported},
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityStructuredOutput, Status: RunnerOptionalCapabilityStatusUnsupported},
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityWorkingDirectory, Status: RunnerOptionalCapabilityStatusUnsupported},
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityWorktree, Status: RunnerOptionalCapabilityStatusUnsupported},
		),
	},
	RunnerIDCursorCLI: {
		ID:          RunnerIDCursorCLI,
		DisplayName: "Cursor CLI",
		Capabilities: NewRunnerCapabilities(
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityImageInput, Status: RunnerOptionalCapabilityStatusUnsupported},
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilitySessionResume, Status: RunnerOptionalCapabilityStatusSupported},
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityStructuredOutput, Status: RunnerOptionalCapabilityStatusUnsupported},
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityWorkingDirectory, Status: RunnerOptionalCapabilityStatusSupported},
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityWorktree, Status: RunnerOptionalCapabilityStatusUnsupported},
		),
	},
	RunnerIDOpenCode: {
		ID:          RunnerIDOpenCode,
		DisplayName: "OpenCode",
		Capabilities: NewRunnerCapabilities(
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityImageInput, Status: RunnerOptionalCapabilityStatusUnsupported},
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilitySessionResume, Status: RunnerOptionalCapabilityStatusSupported},
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityStructuredOutput, Status: RunnerOptionalCapabilityStatusUnsupported},
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityWorkingDirectory, Status: RunnerOptionalCapabilityStatusSupported},
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityWorktree, Status: RunnerOptionalCapabilityStatusUnsupported},
		),
	},
}

// BuiltInRunnerMetadata returns the metadata for one stable built-in runner ID.
func BuiltInRunnerMetadata(id string) (RunnerMetadata, bool) {
	metadata, ok := builtInRunnerMetadata[NormalizeRunnerID(id)]
	if !ok {
		return RunnerMetadata{}, false
	}
	return metadata, true
}

// IsBuiltInRunnerID reports whether a runner ID is one of the stable built-ins.
func IsBuiltInRunnerID(id string) bool {
	_, ok := BuiltInRunnerMetadata(id)
	return ok
}

// ResolveOpenCodeAgent returns the configured OpenCode agent profile for one
// dispatch using workstation override precedence over the worker default.
func ResolveOpenCodeAgent(workstationAgent, workerAgent string) string {
	if agent := strings.TrimSpace(workstationAgent); agent != "" {
		return agent
	}
	return strings.TrimSpace(workerAgent)
}

// ResolveRunnerSelection applies the v1 precedence rules for backend runtime
// runner choice: workstation override, then factory override, then legacy
// worker modelProvider compatibility, then the codex default.
func ResolveRunnerSelection(workstationRunner, factoryRunner, workerModelProvider string) ResolvedRunnerSelection {
	if runner := NormalizeRunnerID(workstationRunner); runner != "" {
		return ResolvedRunnerSelection{RunnerID: runner, Source: RunnerSelectionSourceWorkstation}
	}
	if runner := NormalizeRunnerID(factoryRunner); runner != "" {
		return ResolvedRunnerSelection{RunnerID: runner, Source: RunnerSelectionSourceFactory}
	}
	if runner := NormalizeRunnerID(workerModelProvider); IsBuiltInRunnerID(runner) {
		return ResolvedRunnerSelection{RunnerID: runner, Source: RunnerSelectionSourceLegacyProvider}
	}
	return ResolvedRunnerSelection{RunnerID: RunnerIDCodex, Source: RunnerSelectionSourceDefault}
}

// NormalizeRunnerID trims operator-supplied runner IDs into the canonical
// lowercase form used by the registry and runtime selection logic.
func NormalizeRunnerID(id string) string {
	return strings.ToLower(strings.TrimSpace(id))
}
