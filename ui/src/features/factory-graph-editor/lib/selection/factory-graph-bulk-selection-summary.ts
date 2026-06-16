import type { FactoryGraphNodeKind } from "../draft/factory-graph-draft-types";
import {
  parseFactoryGraphWorkStateNodeId,
  parseFactoryGraphWorkstationNodeId,
  parseFactoryGraphWorkTypeNodeId,
} from "../draft/factory-graph-draft-types";
import type { FactoryGraphEditorSelectionState } from "./factory-graph-editor-selection";

export type FactoryGraphBulkSelectionItemKind =
  | FactoryGraphNodeKind
  | "edge"
  | "unknown";

export type FactoryGraphBulkSelectionKindCount = {
  kind: FactoryGraphBulkSelectionItemKind;
  count: number;
};

export type FactoryGraphBulkSelectionSummary = {
  totalCount: number;
  kindCounts: readonly FactoryGraphBulkSelectionKindCount[];
};

const FACTORY_GRAPH_BULK_SELECTION_KIND_ORDER: readonly FactoryGraphBulkSelectionItemKind[] =
  [
    "workstation",
    "worker",
    "work-type",
    "work-state",
    "resource",
    "doc",
    "edge",
    "unknown",
  ];

export function resolveFactoryGraphNodeKindFromNodeId(
  nodeId: string,
): FactoryGraphNodeKind | "unknown" {
  if (nodeId.startsWith("doc:")) {
    return "doc";
  }
  if (nodeId.startsWith("resource:")) {
    return "resource";
  }
  if (nodeId.startsWith("worker:")) {
    return "worker";
  }
  if (nodeId.startsWith("work-type:")) {
    return "work-type";
  }
  if (nodeId.startsWith("work-state:") || nodeId.startsWith("place:")) {
    return "work-state";
  }
  if (nodeId.startsWith("workstation:")) {
    return "workstation";
  }

  return "unknown";
}

export function buildFactoryGraphBulkSelectionSummary(
  state: Pick<
    FactoryGraphEditorSelectionState,
    "selectedEdgeIds" | "selectedNodeIds"
  >,
): FactoryGraphBulkSelectionSummary | null {
  const countsByKind = new Map<FactoryGraphBulkSelectionItemKind, number>();

  for (const nodeId of state.selectedNodeIds) {
    const kind = resolveFactoryGraphNodeKindFromNodeId(nodeId);
    countsByKind.set(kind, (countsByKind.get(kind) ?? 0) + 1);
  }

  if (state.selectedEdgeIds.size > 0) {
    countsByKind.set("edge", state.selectedEdgeIds.size);
  }

  const totalCount = state.selectedNodeIds.size + state.selectedEdgeIds.size;
  if (totalCount <= 1) {
    return null;
  }

  const kindCounts = FACTORY_GRAPH_BULK_SELECTION_KIND_ORDER.flatMap((kind) => {
    const count = countsByKind.get(kind);
    return count && count > 0 ? [{ kind, count }] : [];
  });

  return {
    totalCount,
    kindCounts,
  };
}

export function factoryGraphEditorSelectionSignature(
  state: FactoryGraphEditorSelectionState,
): string {
  return [
    [...state.selectedNodeIds].sort().join(","),
    [...state.selectedEdgeIds].sort().join(","),
    state.primaryTarget
      ? `${state.primaryTarget.kind}:${state.primaryTarget.id}`
      : "",
  ].join("|");
}

export type GraphSelectionDashboardSyncAction =
  | { kind: "doc"; targetPath: string }
  | { kind: "resource"; resourceName: string }
  | { kind: "state-node"; placeId: string }
  | { kind: "worker"; workerName: string }
  | { kind: "work-type"; workTypeName: string }
  | { kind: "workstation"; nodeId: string };

function resolvePlaceIdFromGraphNodeId(nodeId: string): string | null {
  if (nodeId.startsWith("place:")) {
    const placeId = nodeId.slice("place:".length);
    return placeId.length > 0 ? placeId : null;
  }

  return parseFactoryGraphWorkStateNodeId(nodeId);
}

export function resolveGraphSelectionDashboardSyncAction(
  nodeId: string,
): GraphSelectionDashboardSyncAction | null {
  if (nodeId.startsWith("doc:")) {
    const targetPath = nodeId.slice("doc:".length);
    return targetPath.length > 0 ? { kind: "doc", targetPath } : null;
  }

  if (nodeId.startsWith("resource:")) {
    const resourceName = nodeId.slice("resource:".length);
    return resourceName.length > 0 ? { kind: "resource", resourceName } : null;
  }

  const workerName = nodeId.startsWith("worker:")
    ? nodeId.slice("worker:".length)
    : null;
  if (workerName) {
    return workerName.length > 0 ? { kind: "worker", workerName } : null;
  }

  const workTypeName = parseFactoryGraphWorkTypeNodeId(nodeId);
  if (workTypeName) {
    return { kind: "work-type", workTypeName };
  }

  const placeId = resolvePlaceIdFromGraphNodeId(nodeId);
  if (placeId) {
    return { kind: "state-node", placeId };
  }

  const workstationNodeId = parseFactoryGraphWorkstationNodeId(nodeId);
  if (workstationNodeId) {
    return { kind: "workstation", nodeId: workstationNodeId };
  }

  return null;
}
