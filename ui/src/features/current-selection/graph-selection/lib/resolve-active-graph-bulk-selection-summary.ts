import type { FactoryGraphBulkSelectionSummary } from "../../../factory-graph-editor/lib/selection/factory-graph-bulk-selection-summary";
import type { FactoryGraphEditorSelectionBridgeSnapshot } from "../../../workflow-activity/state/factory-graph-editor-selection-bridge";
import type { DashboardSelection } from "../../base/state/dashboardSelection";

const DASHBOARD_SELECTION_KINDS_WITHOUT_GRAPH_BULK = new Set([
  "work-item",
  "workstation-request",
]);

const STALE_GRAPH_BULK_DASHBOARD_SELECTION_KINDS = new Set([
  "node",
  "state-node",
]);

export function resolveGraphNodeIdCandidatesForDashboardSelection(
  selection: DashboardSelection,
): readonly string[] {
  switch (selection.kind) {
    case "node":
      return selection.nodeId.startsWith("workstation:")
        ? [selection.nodeId]
        : [`workstation:${selection.nodeId}`, selection.nodeId];
    case "state-node":
      return [`work-state:${selection.placeId}`, `place:${selection.placeId}`];
    case "worker":
      return [`worker:${selection.workerName}`];
    case "resource":
      return [`resource:${selection.resourceName}`];
    case "work-type":
      return [`work-type:${selection.workTypeName}`];
    case "doc":
      return [`doc:${selection.targetPath}`];
    default:
      return [];
  }
}

function dashboardSelectionMatchesGraphBridgeSelection(
  selection: DashboardSelection,
  bridgeSnapshot: FactoryGraphEditorSelectionBridgeSnapshot,
): boolean {
  const selectedNodeIds = new Set(bridgeSnapshot.selectedNodeIds);

  return resolveGraphNodeIdCandidatesForDashboardSelection(selection).some(
    (nodeId) => selectedNodeIds.has(nodeId),
  );
}

export function resolveActiveGraphBulkSelectionSummary(
  selection: DashboardSelection | null,
  bridgeSnapshot: FactoryGraphEditorSelectionBridgeSnapshot | null,
): FactoryGraphBulkSelectionSummary | null {
  const bulkSummary = bridgeSnapshot?.bulkSelectionSummary ?? null;
  if (!bulkSummary || !selection || !bridgeSnapshot) {
    return null;
  }

  if (DASHBOARD_SELECTION_KINDS_WITHOUT_GRAPH_BULK.has(selection.kind)) {
    return null;
  }

  if (STALE_GRAPH_BULK_DASHBOARD_SELECTION_KINDS.has(selection.kind)) {
    return bulkSummary;
  }

  return dashboardSelectionMatchesGraphBridgeSelection(
    selection,
    bridgeSnapshot,
  )
    ? bulkSummary
    : null;
}
