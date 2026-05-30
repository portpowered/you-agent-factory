// biome-ignore lint/nursery/noExcessiveLinesPerFile: current activity graph projection helpers remain grouped around shared node and edge fixtures.
import type {
  DashboardActiveExecution,
  DashboardSnapshot,
} from "../../../api/dashboard/types";
import type {
  GraphLayout,
  PositionedEdge,
  PositionedNode,
  PositionedPlaceNode,
  PositionedWorkstationNode,
} from "../../flowchart/lib/layout";
import type { CurrentActivityNode } from "../../flowchart/public";
import type { CurrentActivitySelection } from "../components/react-flow-current-activity-card";
import type {
  GraphNodePosition,
  GraphNodePositions,
} from "../state/currentActivityGraphStore";
import { findFactoryWorkstationByNodeId } from "./current-activity-factory-graph-layout";
import { resolveFactoryGraphPlaceNode } from "./current-activity-factory-graph-node-ids";
import {
  authoredProgressOutcomeSourceHandlesByWorkstationNodeId,
  buildSemanticGraphHandles,
  type CurrentActivityEditorState,
  resolveWorkstationConnectionAnchorContext,
  supportedSemanticHandleIdsForEdge,
} from "./react-flow-current-activity-card-editor-handles";
import type { WorkstationProgressOutcomeRouteContext } from "../../current-factory-definition/lib/workstation-progress-outcome-routes";

export const EMPTY_GRAPH_LAYOUT: GraphLayout = {
  edges: [],
  height: 0,
  nodes: [],
  width: 0,
};
export const EMPTY_NODE_POSITIONS: GraphNodePositions = {};

export interface HandleAssignments {
  sourceHandlesByEdgeId: Map<string, string>;
  targetHandlesByEdgeId: Map<string, string>;
}

export interface ActiveGraphHighlights {
  activeEdgeIds: ReadonlySet<string>;
  activePlaceNodeIds: ReadonlySet<string>;
  activeWorkstationNodeIds: ReadonlySet<string>;
  hasActiveFlow: boolean;
  relatedNodeIds: ReadonlySet<string>;
}

function edgeTouchesResource(edge: PositionedEdge): boolean {
  return (
    edge.sourcePlaceKind === "resource" || edge.targetPlaceKind === "resource"
  );
}

function edgeReturnsToResource(edge: PositionedEdge): boolean {
  return edge.targetPlaceKind === "resource";
}

function resourceNodeNames(nodes: GraphLayout["nodes"]): ReadonlySet<string> {
  const names = new Set<string>();

  for (const node of nodes) {
    if (node.nodeKind === "resource" && node.nodeId.startsWith("resource:")) {
      names.add(node.nodeId.slice("resource:".length));
    }
  }

  return names;
}

function canonicalResourceAliasNodeId(
  nodeId: string,
  resourceNames: ReadonlySet<string>,
): string | null {
  const workStateMatch = /^work-state:([^:]+):(.+)$/.exec(nodeId);
  if (workStateMatch && resourceNames.has(workStateMatch[1] ?? "")) {
    return `resource:${workStateMatch[1]}`;
  }

  const placeMatch = /^place:([^:]+):(.+)$/.exec(nodeId);
  if (placeMatch && resourceNames.has(placeMatch[1] ?? "")) {
    return `resource:${placeMatch[1]}`;
  }

  return null;
}

function resourceAliasNodeIds(
  nodes: GraphLayout["nodes"],
): ReadonlySet<string> {
  const names = resourceNodeNames(nodes);
  const aliases = new Set<string>();

  for (const node of nodes) {
    if (canonicalResourceAliasNodeId(node.nodeId, names)) {
      aliases.add(node.nodeId);
      continue;
    }
    if (
      node.nodeId.startsWith("work-type:") &&
      names.has(node.nodeId.slice("work-type:".length))
    ) {
      aliases.add(node.nodeId);
    }
  }

  return aliases;
}

function canonicalRenderedNodeId(
  nodeId: string,
  resourceNames: ReadonlySet<string>,
): string {
  return canonicalResourceAliasNodeId(nodeId, resourceNames) ?? nodeId;
}

function activeTokenLabel(
  execution: DashboardActiveExecution,
  workID: string,
  fallbackID: string,
): string {
  const workItem = execution.work_items?.find(
    (item) => item.work_id === workID,
  );
  return workItem?.display_name || workItem?.work_id || fallbackID;
}

function workstationGraphNodeId(nodeId: string): string {
  return `workstation:${nodeId}`;
}

function placeGraphNodeId(placeId: string): string {
  return `place:${placeId}`;
}

export function buildHandleAssignments(
  edges: PositionedEdge[],
  nodes: PositionedNode[] = [],
): HandleAssignments {
  const sourceHandlesByEdgeId = new Map<string, string>();
  const targetHandlesByEdgeId = new Map<string, string>();
  const nodeKindsById = new Map(
    nodes.map((node) => [node.nodeId, node.nodeKind]),
  );

  for (const edge of edges) {
    const supportedHandles = supportedSemanticHandleIdsForEdge(
      edge,
      nodeKindsById,
    );
    if (!supportedHandles) {
      throw new Error(
        `Expected semantic handle ids for current activity edge "${edge.edgeId}".`,
      );
    }

    sourceHandlesByEdgeId.set(edge.edgeId, supportedHandles.sourceHandleId);
    targetHandlesByEdgeId.set(edge.edgeId, supportedHandles.targetHandleId);
  }

  return {
    sourceHandlesByEdgeId,
    targetHandlesByEdgeId,
  };
}

export function buildActiveGraphHighlights(
  activeExecutions: DashboardActiveExecution[],
  edges: PositionedEdge[],
  nodes: GraphLayout["nodes"] = [],
): ActiveGraphHighlights {
  const activeEdgeIds = new Set<string>();
  const activePlaceNodeIds = new Set<string>();
  const activeWorkstationNodeIds = new Set<string>();
  const consumedPlaceNodeIds = new Set<string>();
  const relatedNodeIds = new Set<string>();
  const visibleWorkstationNodeIdsByRuntimeId = new Map<string, string>();

  for (const node of nodes) {
    if (node.nodeKind === "workstation") {
      visibleWorkstationNodeIdsByRuntimeId.set(
        node.workstationNodeId,
        node.nodeId,
      );
    }
  }

  for (const execution of activeExecutions) {
    const workstationNodeId =
      visibleWorkstationNodeIdsByRuntimeId.get(execution.workstation_node_id) ??
      workstationGraphNodeId(execution.workstation_node_id);
    activeWorkstationNodeIds.add(workstationNodeId);
    relatedNodeIds.add(workstationNodeId);

    for (const token of execution.consumed_tokens ?? []) {
      const placeNodeId = placeGraphNodeId(token.place_id);
      consumedPlaceNodeIds.add(placeNodeId);
      relatedNodeIds.add(placeNodeId);
    }
  }

  for (const edge of edges) {
    const resourceEdge = edgeTouchesResource(edge);
    const flowsIntoActiveWorkstation =
      !resourceEdge &&
      activeWorkstationNodeIds.has(edge.toNodeId) &&
      consumedPlaceNodeIds.has(edge.fromNodeId);
    const flowsOutOfActiveWorkstation =
      !resourceEdge &&
      activeWorkstationNodeIds.has(edge.fromNodeId) &&
      !(edge.outcomeKind === "failed" || edge.stateCategory === "FAILED");

    if (!flowsIntoActiveWorkstation && !flowsOutOfActiveWorkstation) {
      continue;
    }

    activeEdgeIds.add(edge.edgeId);
    relatedNodeIds.add(edge.fromNodeId);
    relatedNodeIds.add(edge.toNodeId);

    if (flowsOutOfActiveWorkstation) {
      activePlaceNodeIds.add(edge.toNodeId);
    }
  }

  return {
    activeEdgeIds,
    activePlaceNodeIds,
    activeWorkstationNodeIds,
    hasActiveFlow: activeExecutions.length > 0,
    relatedNodeIds,
  };
}

export function buildVisibleGraphEdges(
  graphLayout: GraphLayout,
): PositionedEdge[] {
  const names = resourceNodeNames(graphLayout.nodes);
  const edgesById = new Map<string, PositionedEdge>();

  for (const edge of graphLayout.edges) {
    const fromNodeId = canonicalRenderedNodeId(edge.fromNodeId, names);
    const toNodeId = canonicalRenderedNodeId(edge.toNodeId, names);
    if (fromNodeId === toNodeId) {
      continue;
    }

    const canonicalKind =
      edge.edgeId.startsWith("workstation-input:") &&
      fromNodeId.startsWith("resource:")
        ? "workstation-resource"
        : edge.edgeId.split(":")[0];
    const edgeId =
      fromNodeId === edge.fromNodeId &&
      toNodeId === edge.toNodeId &&
      canonicalKind === edge.edgeId.split(":")[0]
        ? edge.edgeId
        : `${canonicalKind}:${fromNodeId}->${toNodeId}`;
    const nextEdge = {
      ...edge,
      edgeId,
      fromNodeId,
      sourcePlaceKind: fromNodeId.startsWith("resource:")
        ? "resource"
        : edge.sourcePlaceKind,
      targetPlaceKind: toNodeId.startsWith("resource:")
        ? "resource"
        : edge.targetPlaceKind,
      toNodeId,
    } satisfies PositionedEdge;

    if (!edgeReturnsToResource(nextEdge)) {
      edgesById.set(nextEdge.edgeId, nextEdge);
    }
  }

  return [...edgesById.values()];
}

export function buildActiveItemLabelsByPlaceId(
  activeExecutions: DashboardActiveExecution[],
) {
  const labelsByPlaceId = new Map<string, string[]>();
  const seenByPlaceId = new Map<string, Set<string>>();

  for (const execution of activeExecutions) {
    for (const token of execution.consumed_tokens ?? []) {
      const label =
        token.name ||
        activeTokenLabel(execution, token.work_id, token.token_id);
      const placeLabels = labelsByPlaceId.get(token.place_id) ?? [];
      const seenLabels = seenByPlaceId.get(token.place_id) ?? new Set<string>();
      if (seenLabels.has(label)) {
        continue;
      }

      seenLabels.add(label);
      placeLabels.push(label);
      seenByPlaceId.set(token.place_id, seenLabels);
      labelsByPlaceId.set(token.place_id, placeLabels);
    }
  }

  return labelsByPlaceId;
}

function finitePosition(
  position: GraphNodePosition | undefined,
): position is GraphNodePosition {
  return (
    position !== undefined &&
    Number.isFinite(position.x) &&
    Number.isFinite(position.y)
  );
}

function nodePosition(
  nodeId: string,
  fallback: GraphNodePosition,
  storedPositions: GraphNodePositions,
): GraphNodePosition {
  const storedPosition = storedPositions[nodeId];
  return finitePosition(storedPosition) ? storedPosition : fallback;
}

interface BuildCurrentActivityNodesInput {
  activeExecutionsByWorkstationNodeID: Record<
    string,
    DashboardActiveExecution[]
  >;
  activeGraphHighlights: ActiveGraphHighlights;
  activeItemLabelsByPlaceId: Map<string, string[]>;
  factoryDefinition?: DashboardSnapshot["factory"];
  graphLayout: GraphLayout;
  locale?: string;
  now: number;
  onSelectStateNode: (placeId: string) => void;
  onSelectWorkID: (
    workID: string,
    hint?: { dispatchID?: string; nodeID?: string },
  ) => void;
  onSelectWorker: (workerName: string) => void;
  onSelectWorkstation: (nodeId: string) => void;
  editor?: CurrentActivityEditorState;
  selection: CurrentActivitySelection | null;
  snapshot: DashboardSnapshot;
  storedNodePositions: GraphNodePositions;
}

function buildPlaceNode(
  positionedNode: PositionedPlaceNode,
  input: BuildCurrentActivityNodesInput,
): CurrentActivityNode {
  const place = positionedNode.place;
  const position = nodePosition(
    positionedNode.nodeId,
    { x: positionedNode.x, y: positionedNode.y },
    input.storedNodePositions,
  );
  const basePlaceNode = {
    className: "border-0 bg-transparent p-0 text-af-text",
    draggable: true,
    height: positionedNode.height,
    id: positionedNode.nodeId,
    initialHeight: positionedNode.height,
    initialWidth: positionedNode.width,
    measured: { height: positionedNode.height, width: positionedNode.width },
    position,
    width: positionedNode.width,
  };
  const factoryGraphNode = resolveFactoryGraphPlaceNode(place);
  const basePlaceData = {
    activeFlow: input.activeGraphHighlights.activePlaceNodeIds.has(
      positionedNode.nodeId,
    ),
    activeItemLabels: input.activeItemLabelsByPlaceId.get(place.place_id) ?? [],
    factoryGraphNodeId: factoryGraphNode?.nodeId ?? positionedNode.nodeId,
    handles: factoryGraphNode
      ? buildSemanticGraphHandles({
          editor: input.editor,
          locale: input.locale,
          nodeId: factoryGraphNode.nodeId,
          nodeKind: factoryGraphNode.kind,
        })
      : [],
    locale: input.locale,
    muted:
      place.kind !== "resource" &&
      input.activeGraphHighlights.hasActiveFlow &&
      !input.activeGraphHighlights.relatedNodeIds.has(positionedNode.nodeId),
    selectedStateNode:
      input.selection?.kind === "state-node" &&
      input.selection.placeId === place.place_id,
    tokenCount:
      input.snapshot.runtime.place_token_counts?.[place.place_id] ?? 0,
  };

  if (place.kind === "work_state") {
    return {
      ...basePlaceNode,
      data: {
        ...basePlaceData,
        kind: "work-state" as const,
        onSelectStateNode: input.onSelectStateNode,
        place,
      },
      selectable: true,
      type: "statePosition",
    };
  }

  if (place.kind === "resource") {
    return {
      ...basePlaceNode,
      data: { ...basePlaceData, kind: "resource" as const, place },
      selectable: false,
      type: "resource",
    };
  }

  if (factoryGraphNode?.kind === "worker") {
    const workerName = place.state_value ?? factoryGraphNode.nodeId.replace(/^worker:/, "");
    return {
      ...basePlaceNode,
      data: {
        ...basePlaceData,
        kind: "worker" as const,
        onSelectWorker: input.onSelectWorker,
        place,
        selectedWorker:
          input.selection?.kind === "worker" &&
          input.selection.workerName === workerName,
      },
      selectable: true,
      type: "worker",
    };
  }

  if (factoryGraphNode?.kind === "work-type") {
    return {
      ...basePlaceNode,
      data: {
        ...basePlaceData,
        kind: "work-type" as const,
        place,
      },
      selectable: false,
      type: "workType",
    };
  }

  return {
    ...basePlaceNode,
    data: {
      ...basePlaceData,
      kind: factoryGraphNode?.kind,
      place,
    },
    selectable: false,
    type: "constraint",
  };
}

function buildWorkstationNode(
  positionedNode: PositionedWorkstationNode,
  input: BuildCurrentActivityNodesInput,
  authoredProgressOutcomeSourceHandleIds?: ReadonlySet<string>,
): CurrentActivityNode | null {
  const workstation =
    input.snapshot.topology.workstation_nodes_by_id[
      positionedNode.workstationNodeId
    ] ??
    findFactoryWorkstationByNodeId(
      input.factoryDefinition ?? input.snapshot.factory,
      positionedNode.workstationNodeId,
    );
  if (!workstation) {
    return null;
  }

  const factory = input.factoryDefinition ?? input.snapshot.factory;
  const connectionAnchorContext = resolveWorkstationConnectionAnchorContext(
    factory,
    positionedNode.nodeId,
  );
  const progressOutcomeRouteWorkstation: WorkstationProgressOutcomeRouteContext | undefined =
    connectionAnchorContext?.workstation;

  const executions =
    input.activeExecutionsByWorkstationNodeID[workstation.node_id] ?? [];
  const position = nodePosition(
    positionedNode.nodeId,
    { x: positionedNode.x, y: positionedNode.y },
    input.storedNodePositions,
  );

  return {
    className: "border-0 bg-transparent p-0 text-af-text",
    data: {
      active: executions.length > 0,
      activeFlow: input.activeGraphHighlights.activeWorkstationNodeIds.has(
        positionedNode.nodeId,
      ),
      executions,
      factoryGraphNodeId: positionedNode.nodeId,
      handles: buildSemanticGraphHandles({
        authoredProgressOutcomeSourceHandleIds,
        connectionAnchorContext,
        editor: input.editor,
        locale: input.locale,
        nodeId: positionedNode.nodeId,
        nodeKind: "workstation",
      }),
      kind: "workstation",
      progressOutcomeRouteWorkstation,
      locale: input.locale,
      muted:
        input.activeGraphHighlights.hasActiveFlow &&
        !input.activeGraphHighlights.relatedNodeIds.has(positionedNode.nodeId),
      now: input.now,
      onSelectWorkID: input.onSelectWorkID,
      onSelectWorkstation: input.onSelectWorkstation,
      selectedWorkID:
        input.selection?.kind === "work-item" &&
        input.selection.nodeId === workstation.node_id
          ? input.selection.workID
          : null,
      selectedWorkstation:
        input.selection?.kind === "node" &&
        input.selection.nodeId === workstation.node_id,
      workstation,
    },
    draggable: true,
    height: positionedNode.height,
    id: positionedNode.nodeId,
    initialHeight: positionedNode.height,
    initialWidth: positionedNode.width,
    measured: {
      height: positionedNode.height,
      width: positionedNode.width,
    },
    position,
    selectable: true,
    type: "workstation",
    width: positionedNode.width,
  };
}

export function buildCurrentActivityNodes({
  activeExecutionsByWorkstationNodeID,
  activeGraphHighlights,
  activeItemLabelsByPlaceId,
  factoryDefinition,
  editor,
  graphLayout,
  locale,
  now,
  onSelectStateNode,
  onSelectWorkID,
  onSelectWorker,
  onSelectWorkstation,
  selection,
  snapshot,
  storedNodePositions,
}: BuildCurrentActivityNodesInput): CurrentActivityNode[] {
  const nextNodes: CurrentActivityNode[] = [];
  const resourceAliases = resourceAliasNodeIds(graphLayout.nodes);
  const visibleGraphEdges = buildVisibleGraphEdges(graphLayout);
  const authoredProgressOutcomeSourceHandlesByNodeId =
    authoredProgressOutcomeSourceHandlesByWorkstationNodeId(
      visibleGraphEdges,
      graphLayout.nodes,
    );
  const input = {
    activeExecutionsByWorkstationNodeID,
    activeGraphHighlights,
    activeItemLabelsByPlaceId,
    editor,
    factoryDefinition,
    graphLayout,
    locale,
    now,
    onSelectStateNode,
    onSelectWorkID,
    onSelectWorker,
    onSelectWorkstation,
    selection,
    snapshot,
    storedNodePositions,
  } satisfies BuildCurrentActivityNodesInput;

  for (const positionedNode of graphLayout.nodes) {
    if (resourceAliases.has(positionedNode.nodeId)) {
      continue;
    }

    if (positionedNode.nodeKind !== "workstation") {
      nextNodes.push(
        buildPlaceNode(positionedNode as PositionedPlaceNode, input),
      );
      continue;
    }

    const workstationNode = buildWorkstationNode(
      positionedNode as PositionedWorkstationNode,
      input,
      authoredProgressOutcomeSourceHandlesByNodeId.get(positionedNode.nodeId),
    );
    if (workstationNode) {
      nextNodes.push(workstationNode);
    }
  }

  return dedupeNodesByFactoryGraphNodeId(nextNodes);
}

function dedupeNodesByFactoryGraphNodeId(
  nodes: CurrentActivityNode[],
): CurrentActivityNode[] {
  const selectedNodeByFactoryGraphId = new Map<string, CurrentActivityNode>();
  const selectedIndexByFactoryGraphId = new Map<string, number>();
  const nextNodes: CurrentActivityNode[] = [];

  for (const node of nodes) {
    const factoryGraphNodeId = node.data.factoryGraphNodeId;
    if (!factoryGraphNodeId) {
      nextNodes.push(node);
      continue;
    }

    const selectedNode = selectedNodeByFactoryGraphId.get(factoryGraphNodeId);
    if (!selectedNode) {
      selectedNodeByFactoryGraphId.set(factoryGraphNodeId, node);
      selectedIndexByFactoryGraphId.set(factoryGraphNodeId, nextNodes.length);
      nextNodes.push(node);
      continue;
    }

    if (
      selectedNode.id === factoryGraphNodeId ||
      node.id !== factoryGraphNodeId
    ) {
      continue;
    }

    const selectedIndex = selectedIndexByFactoryGraphId.get(factoryGraphNodeId);
    if (selectedIndex === undefined) {
      continue;
    }

    selectedNodeByFactoryGraphId.set(factoryGraphNodeId, node);
    nextNodes[selectedIndex] = node;
  }

  return nextNodes;
}
