package work

import (
	"encoding/json"

	"github.com/portpowered/infinite-you/pkg/services/work/internal/lineagegraph"
)

const (
	VisualizationFormatMermaid         = lineagegraph.VisualizationFormatMermaid
	VisualizationFormatMarkdownMermaid = lineagegraph.VisualizationFormatMarkdownMermaid
)

// WorkPayloadLineageProjection is the canonical replay-safe source of truth for
// user-visible work payload history.
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

type WorkPayloadSnapshotKind = lineagegraph.WorkPayloadSnapshotKind

const (
	WorkPayloadSnapshotKindWorkRequest    = lineagegraph.WorkPayloadSnapshotKindWorkRequest
	WorkPayloadSnapshotKindDispatchOutput = lineagegraph.WorkPayloadSnapshotKindDispatchOutput
)

type WorkPayloadContinuity = lineagegraph.WorkPayloadContinuity

const (
	WorkPayloadContinuityInitial           = lineagegraph.WorkPayloadContinuityInitial
	WorkPayloadContinuitySameWorkID        = lineagegraph.WorkPayloadContinuitySameWorkID
	WorkPayloadContinuityNewDownstreamWork = lineagegraph.WorkPayloadContinuityNewDownstreamWork
)

type WorkPayloadResolutionStatus = lineagegraph.WorkPayloadResolutionStatus

const (
	WorkPayloadResolutionResolved    = lineagegraph.WorkPayloadResolutionResolved
	WorkPayloadResolutionUnavailable = lineagegraph.WorkPayloadResolutionUnavailable
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

func (p *WorkPayloadLineageProjection) RecordWorkRequestSnapshot(observedTick int, requestID string, item FactoryWorkItem) {
	inner := toLineageProjection(*p)
	inner.RecordWorkRequestSnapshot(observedTick, requestID, toLineageWorkItem(item))
	*p = fromLineageProjection(inner)
}

func (p *WorkPayloadLineageProjection) RecordConsumedInputSnapshot(dispatchID string, item FactoryWorkItem) {
	inner := toLineageProjection(*p)
	inner.RecordConsumedInputSnapshot(dispatchID, toLineageWorkItem(item))
	*p = fromLineageProjection(inner)
}

func (p *WorkPayloadLineageProjection) RecordDispatchOutputSnapshot(
	observedTick int,
	dispatchID string,
	consumedInputs []FactoryWorkItem,
	item FactoryWorkItem,
	outputIndex int,
) {
	inner := toLineageProjection(*p)
	inner.RecordDispatchOutputSnapshot(
		observedTick,
		dispatchID,
		toLineageWorkItems(consumedInputs),
		toLineageWorkItem(item),
		outputIndex,
	)
	*p = fromLineageProjection(inner)
}

func (p WorkPayloadLineageProjection) ResolveInitialSubmittedSnapshot(workID string) WorkPayloadResolution {
	return fromLineageResolution(p.toLineage().ResolveInitialSubmittedSnapshot(workID))
}

func (p WorkPayloadLineageProjection) ResolveConsumedInputSnapshot(dispatchID, workID string) WorkPayloadResolution {
	return fromLineageResolution(p.toLineage().ResolveConsumedInputSnapshot(dispatchID, workID))
}

func (p WorkPayloadLineageProjection) ResolveSelectedWorkSnapshot(workID string) WorkPayloadResolution {
	return fromLineageResolution(p.toLineage().ResolveSelectedWorkSnapshot(workID))
}

func (p WorkPayloadLineageProjection) ResolveOutputWorkSnapshot(dispatchID, workID string) WorkPayloadResolution {
	return fromLineageResolution(p.toLineage().ResolveOutputWorkSnapshot(dispatchID, workID))
}

func (p WorkPayloadLineageProjection) toLineage() lineagegraph.WorkPayloadLineageProjection {
	return toLineageProjection(p)
}

// CanonicalChainingTraceIDs applies the shared chaining-trace fan-in rule.
func CanonicalChainingTraceIDs(traceIDs []string) []string {
	return lineagegraph.CanonicalChainingTraceIDs(traceIDs)
}

// PreviousChainingTraceIDsFromWorkItems collects predecessor chain IDs from
// canonical work items using the shared deterministic fan-in rule.
func PreviousChainingTraceIDsFromWorkItems(items []FactoryWorkItem) []string {
	return lineagegraph.PreviousChainingTraceIDsFromWorkItems(toLineageWorkItems(items))
}

// CurrentChainingTraceIDFromWorkItems resolves the current dispatch chain from
// the first non-system customer work item.
func CurrentChainingTraceIDFromWorkItems(items []FactoryWorkItem) string {
	return lineagegraph.CurrentChainingTraceIDFromWorkItems(toLineageWorkItems(items))
}

// Node is one work item in a derived dependency graph.
type Node = lineagegraph.Node

// Edge is a directed relationship between two work items.
type Edge = lineagegraph.Edge

// Graph is a deterministic projection of work items and declared relationships.
type Graph = lineagegraph.Graph

// DeriveFromWorkRequest projects a parsed batch request into a dependency graph.
func DeriveFromWorkRequest(req WorkRequest) (Graph, error) {
	return lineagegraph.DeriveFromBatchRequest(toLineageBatchRequest(req))
}

// RenderMermaidFlowchart renders a deterministic Mermaid flowchart diagram.
func RenderMermaidFlowchart(g Graph) string {
	return lineagegraph.RenderMermaidFlowchart(g)
}

// RenderMarkdownMermaid renders a Markdown document with a title, short summary,
// and one fenced Mermaid flowchart.
func RenderMarkdownMermaid(g Graph) string {
	return lineagegraph.RenderMarkdownMermaid(g)
}

type VisualizationFileSystem interface {
	ReadFile(string) ([]byte, error)
}

type VisualizationRequest = lineagegraph.VisualizationRequest

type VisualizationOperation = lineagegraph.VisualizationOperation

// NewVisualizationOperation binds the exact filesystem read edge to Work's
// batch dependency visualization policy.
func NewVisualizationOperation(filesystem VisualizationFileSystem) VisualizationOperation {
	return lineagegraph.NewVisualizationOperation(
		visualizationFileSystemAdapter{filesystem: filesystem},
		parseVisualizationBatchRequest,
	)
}

type visualizationFileSystemAdapter struct {
	filesystem VisualizationFileSystem
}

func (adapter visualizationFileSystemAdapter) ReadFile(path string) ([]byte, error) {
	if adapter.filesystem == nil {
		return nil, nil
	}
	return adapter.filesystem.ReadFile(path)
}

func parseVisualizationBatchRequest(data []byte) (lineagegraph.BatchRequest, error) {
	workRequest, err := ParseCanonicalWorkRequestJSON(data)
	if err != nil {
		return lineagegraph.BatchRequest{}, err
	}
	return toLineageBatchRequest(workRequest), nil
}

func toLineageProjection(projection WorkPayloadLineageProjection) lineagegraph.WorkPayloadLineageProjection {
	inner := lineagegraph.WorkPayloadLineageProjection{
		InitialSnapshotIDByWorkID:        cloneStringMap(projection.InitialSnapshotIDByWorkID),
		LatestSnapshotIDByWorkID:         cloneStringMap(projection.LatestSnapshotIDByWorkID),
		SnapshotIDsByWorkID:              cloneStringSliceMap(projection.SnapshotIDsByWorkID),
		ConsumedSnapshotRefsByDispatchID: toLineageRefMap(projection.ConsumedSnapshotRefsByDispatchID),
		OutputSnapshotRefsByDispatchID:   toLineageRefMap(projection.OutputSnapshotRefsByDispatchID),
	}
	if len(projection.SnapshotsByID) > 0 {
		inner.SnapshotsByID = make(map[string]lineagegraph.WorkPayloadSnapshot, len(projection.SnapshotsByID))
		for id, snapshot := range projection.SnapshotsByID {
			inner.SnapshotsByID[id] = toLineageSnapshot(snapshot)
		}
	}
	return inner
}

func fromLineageProjection(projection lineagegraph.WorkPayloadLineageProjection) WorkPayloadLineageProjection {
	outer := WorkPayloadLineageProjection{
		InitialSnapshotIDByWorkID:        cloneStringMap(projection.InitialSnapshotIDByWorkID),
		LatestSnapshotIDByWorkID:         cloneStringMap(projection.LatestSnapshotIDByWorkID),
		SnapshotIDsByWorkID:              cloneStringSliceMap(projection.SnapshotIDsByWorkID),
		ConsumedSnapshotRefsByDispatchID: fromLineageRefMap(projection.ConsumedSnapshotRefsByDispatchID),
		OutputSnapshotRefsByDispatchID:   fromLineageRefMap(projection.OutputSnapshotRefsByDispatchID),
	}
	if len(projection.SnapshotsByID) > 0 {
		outer.SnapshotsByID = make(map[string]WorkPayloadSnapshot, len(projection.SnapshotsByID))
		for id, snapshot := range projection.SnapshotsByID {
			outer.SnapshotsByID[id] = fromLineageSnapshot(snapshot)
		}
	}
	return outer
}

func toLineageSnapshot(snapshot WorkPayloadSnapshot) lineagegraph.WorkPayloadSnapshot {
	return lineagegraph.WorkPayloadSnapshot{
		SnapshotID:           snapshot.SnapshotID,
		WorkID:               snapshot.WorkID,
		LogicalWorkID:        snapshot.LogicalWorkID,
		SourceKind:           snapshot.SourceKind,
		SourceEventType:      snapshot.SourceEventType,
		RequestID:            snapshot.RequestID,
		DispatchID:           snapshot.DispatchID,
		ObservedTick:         snapshot.ObservedTick,
		Continuity:           snapshot.Continuity,
		ParentSnapshotIDs:    cloneStringSlice(snapshot.ParentSnapshotIDs),
		ParentWorkIDs:        cloneStringSlice(snapshot.ParentWorkIDs),
		ParentLogicalWorkIDs: cloneStringSlice(snapshot.ParentLogicalWorkIDs),
		WorkItem:             toLineageWorkItem(snapshot.WorkItem),
	}
}

func fromLineageSnapshot(snapshot lineagegraph.WorkPayloadSnapshot) WorkPayloadSnapshot {
	return WorkPayloadSnapshot{
		SnapshotID:           snapshot.SnapshotID,
		WorkID:               snapshot.WorkID,
		LogicalWorkID:        snapshot.LogicalWorkID,
		SourceKind:           snapshot.SourceKind,
		SourceEventType:      snapshot.SourceEventType,
		RequestID:            snapshot.RequestID,
		DispatchID:           snapshot.DispatchID,
		ObservedTick:         snapshot.ObservedTick,
		Continuity:           snapshot.Continuity,
		ParentSnapshotIDs:    cloneStringSlice(snapshot.ParentSnapshotIDs),
		ParentWorkIDs:        cloneStringSlice(snapshot.ParentWorkIDs),
		ParentLogicalWorkIDs: cloneStringSlice(snapshot.ParentLogicalWorkIDs),
		WorkItem:             fromLineageWorkItem(snapshot.WorkItem),
	}
}

func fromLineageResolution(resolution lineagegraph.WorkPayloadResolution) WorkPayloadResolution {
	outer := WorkPayloadResolution{
		Status: resolution.Status,
		Reason: resolution.Reason,
	}
	if resolution.Snapshot != nil {
		snapshot := fromLineageSnapshot(*resolution.Snapshot)
		outer.Snapshot = &snapshot
	}
	return outer
}

func toLineageRefMap(values map[string]map[string]WorkPayloadRef) map[string]map[string]lineagegraph.WorkPayloadRef {
	if len(values) == 0 {
		return nil
	}
	clone := make(map[string]map[string]lineagegraph.WorkPayloadRef, len(values))
	for dispatchID, refs := range values {
		clone[dispatchID] = make(map[string]lineagegraph.WorkPayloadRef, len(refs))
		for workID, ref := range refs {
			clone[dispatchID][workID] = lineagegraph.WorkPayloadRef{
				Status:     ref.Status,
				SnapshotID: ref.SnapshotID,
				Reason:     ref.Reason,
			}
		}
	}
	return clone
}

func fromLineageRefMap(values map[string]map[string]lineagegraph.WorkPayloadRef) map[string]map[string]WorkPayloadRef {
	if len(values) == 0 {
		return nil
	}
	clone := make(map[string]map[string]WorkPayloadRef, len(values))
	for dispatchID, refs := range values {
		clone[dispatchID] = make(map[string]WorkPayloadRef, len(refs))
		for workID, ref := range refs {
			clone[dispatchID][workID] = WorkPayloadRef{
				Status:     ref.Status,
				SnapshotID: ref.SnapshotID,
				Reason:     ref.Reason,
			}
		}
	}
	return clone
}

func toLineageWorkItems(items []FactoryWorkItem) []lineagegraph.WorkItem {
	if len(items) == 0 {
		return nil
	}
	converted := make([]lineagegraph.WorkItem, len(items))
	for i, item := range items {
		converted[i] = toLineageWorkItem(item)
	}
	return converted
}

func toLineageWorkItem(item FactoryWorkItem) lineagegraph.WorkItem {
	return lineagegraph.WorkItem{
		ID:                       item.ID,
		WorkTypeID:               item.WorkTypeID,
		State:                    item.State,
		DisplayName:              item.DisplayName,
		ChainingTraceDepth:       item.ChainingTraceDepth,
		CurrentChainingTraceID:   item.CurrentChainingTraceID,
		PreviousChainingTraceIDs: cloneStringSlice(item.PreviousChainingTraceIDs),
		TraceID:                  item.TraceID,
		Content:                  toLineageContentParts(item.Content),
		ParentID:                 item.ParentID,
		PlaceID:                  item.PlaceID,
		Tags:                     cloneStringMap(item.Tags),
	}
}

func fromLineageWorkItem(item lineagegraph.WorkItem) FactoryWorkItem {
	return FactoryWorkItem{
		ID:                       item.ID,
		WorkTypeID:               item.WorkTypeID,
		State:                    item.State,
		DisplayName:              item.DisplayName,
		ChainingTraceDepth:       item.ChainingTraceDepth,
		CurrentChainingTraceID:   item.CurrentChainingTraceID,
		PreviousChainingTraceIDs: cloneStringSlice(item.PreviousChainingTraceIDs),
		TraceID:                  item.TraceID,
		Content:                  fromLineageContentParts(item.Content),
		ParentID:                 item.ParentID,
		PlaceID:                  item.PlaceID,
		Tags:                     cloneStringMap(item.Tags),
	}
}

func toLineageContentParts(parts []WorkContentPart) []lineagegraph.ContentPart {
	if len(parts) == 0 {
		return nil
	}
	converted := make([]lineagegraph.ContentPart, len(parts))
	for i, part := range parts {
		converted[i] = lineagegraph.ContentPart{
			Type:        string(part.Type),
			Text:        part.Text,
			URL:         part.URL,
			File:        part.File,
			JSON:        append(json.RawMessage(nil), part.JSON...),
			Slot:        part.Slot,
			Label:       part.Label,
			Role:        part.Role,
			ContentType: part.ContentType,
			ArtifactID:  part.ArtifactID,
			Metadata:    cloneAnyMap(part.Metadata),
		}
	}
	return converted
}

func fromLineageContentParts(parts []lineagegraph.ContentPart) []WorkContentPart {
	if len(parts) == 0 {
		return nil
	}
	converted := make([]WorkContentPart, len(parts))
	for i, part := range parts {
		converted[i] = WorkContentPart{
			Type:        WorkContentPartType(part.Type),
			Text:        part.Text,
			URL:         part.URL,
			File:        part.File,
			JSON:        append(json.RawMessage(nil), part.JSON...),
			Slot:        part.Slot,
			Label:       part.Label,
			Role:        part.Role,
			ContentType: part.ContentType,
			ArtifactID:  part.ArtifactID,
			Metadata:    cloneAnyMap(part.Metadata),
		}
	}
	return converted
}

func toLineageBatchRequest(req WorkRequest) lineagegraph.BatchRequest {
	batch := lineagegraph.BatchRequest{
		RequestID: req.RequestID,
		Type:      string(req.Type),
	}
	for _, work := range req.Works {
		batch.Works = append(batch.Works, lineagegraph.BatchWork{
			Name:       work.Name,
			WorkID:     work.WorkID,
			WorkTypeID: work.WorkTypeID,
		})
	}
	for _, rel := range req.Relations {
		batch.Relations = append(batch.Relations, lineagegraph.BatchRelation{
			Type:           string(rel.Type),
			SourceWorkName: rel.SourceWorkName,
			TargetWorkName: rel.TargetWorkName,
		})
	}
	return batch
}
