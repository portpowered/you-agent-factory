import type { Edge, Node, XYPosition } from "@xyflow/react";

import type { GraphNodeHandle } from "../graphs/graph-node-handle";
import type {
  FactoryTopologyReplayConnection,
  FactoryTopologyReplayMessages,
  FactoryTopologyReplayNode,
  FactoryTopologyReplayOccupancy,
  FactoryTopologyReplayProjection,
  FactoryVisualizerError,
} from "./factory-topology-replay-types";

const COLUMN_ORDER = [
  "work-type",
  "work-state",
  "workstation",
  "worker",
  "resource",
] as const;
const COLUMN_GAP = 280;
const ROW_GAP = 160;

export interface FactoryTopologyFlowNodeData extends Record<string, unknown> {
  activeDispatchCount: number;
  formatNumber: (value: number) => string;
  messages: FactoryTopologyReplayMessages;
  node: FactoryTopologyReplayNode;
  occupancy?: FactoryTopologyReplayOccupancy;
  onSelect?: (nodeId: string) => void;
  selected: boolean;
  workStateCount?: number;
}

export type FactoryTopologyFlowNode = Node<FactoryTopologyFlowNodeData>;

export interface FactoryTopologyFlowProjection {
  edges: Edge[];
  nodes: FactoryTopologyFlowNode[];
}

export function projectFactoryTopologyToFlow(input: {
  formatNumber: (value: number) => string;
  messages: FactoryTopologyReplayMessages;
  onSelectNode?: (nodeId: string) => void;
  projection: FactoryTopologyReplayProjection;
  selectedNodeId?: string;
}): FactoryTopologyFlowProjection | FactoryVisualizerError {
  const validationError = validateProjection(input.projection);
  if (validationError) return validationError;

  const positions = layoutPositions(input.projection.topology.nodes);
  const occupancyByNode = new Map(
    input.projection.activity.resourceOccupancy.map((occupancy) => [
      occupancy.resourceNodeId,
      occupancy,
    ]),
  );
  const countsByNode = new Map(
    input.projection.workStateCounts.map(({ count, nodeId }) => [
      nodeId,
      count,
    ]),
  );
  const activeDispatchesByNode = new Map<string, number>();
  for (const dispatch of input.projection.activity.activeDispatches) {
    if (!dispatch.workstationNodeId) continue;
    activeDispatchesByNode.set(
      dispatch.workstationNodeId,
      (activeDispatchesByNode.get(dispatch.workstationNodeId) ?? 0) + 1,
    );
  }

  return {
    edges: input.projection.topology.connections.map((connection) =>
      flowEdge(connection, input.messages),
    ),
    nodes: input.projection.topology.nodes.map((node) => ({
      data: {
        activeDispatchCount: activeDispatchesByNode.get(node.id) ?? 0,
        formatNumber: input.formatNumber,
        messages: input.messages,
        node,
        occupancy: occupancyByNode.get(node.id),
        onSelect: input.onSelectNode,
        selected: input.selectedNodeId === node.id,
        workStateCount: countsByNode.get(node.id),
      },
      draggable: false,
      id: node.id,
      position: positions.get(node.id) ?? { x: 0, y: 0 },
      selectable: false,
      type: "factoryTopology",
    })),
  };
}

export function graphHandles(
  node: FactoryTopologyReplayNode,
  messages: FactoryTopologyReplayMessages,
): GraphNodeHandle[] {
  return node.handles.map((handle) => ({
    connectable: false,
    id: handle.id,
    label: messages.handleLabel(handle.id, handle.role),
    side: handle.role === "target" ? "left" : "right",
    tone: handleTone(handle.id),
    type: handle.role,
  }));
}

function handleTone(handleId: string): GraphNodeHandle["tone"] {
  if (handleId.includes("resource")) return "resource";
  if (handleId.includes("worker")) return "worker";
  if (handleId.includes("failure")) return "failure";
  if (handleId.includes("rejection")) return "rejection";
  if (handleId.includes("continue")) return "continue";
  if (handleId.includes("output")) return "output";
  if (handleId.includes("input")) return "input";
  return "default";
}

function flowEdge(
  connection: FactoryTopologyReplayConnection,
  messages: FactoryTopologyReplayMessages,
): Edge {
  return {
    data: { label: messages.connectionLabel(connection.kind) },
    id: connection.id,
    selectable: false,
    source: connection.source.nodeId,
    sourceHandle: connection.source.handleId,
    target: connection.target.nodeId,
    targetHandle: connection.target.handleId,
    type: "graphEdge",
  };
}

function layoutPositions(
  nodes: readonly FactoryTopologyReplayNode[],
): Map<string, XYPosition> {
  const positions = new Map<string, XYPosition>();
  for (const [column, kind] of COLUMN_ORDER.entries()) {
    const columnNodes = nodes.filter((node) => node.kind === kind);
    for (const [row, node] of columnNodes.entries()) {
      positions.set(node.id, { x: column * COLUMN_GAP, y: row * ROW_GAP });
    }
  }
  return positions;
}

function validateProjection(
  projection: FactoryTopologyReplayProjection,
): FactoryVisualizerError | undefined {
  if (projection.topology.selectedTick !== projection.activity.selectedTick) {
    return projectionError();
  }
  if (
    projection.topology.issues.length > 0 ||
    projection.activity.issues.length > 0
  ) {
    return projectionError();
  }

  const nodes = new Map<string, FactoryTopologyReplayNode>();
  for (const node of projection.topology.nodes) {
    if (nodes.has(node.id)) return projectionError();
    nodes.set(node.id, node);
  }
  for (const connection of projection.topology.connections) {
    if (
      !validEndpoint(nodes, connection.source, "source") ||
      !validEndpoint(nodes, connection.target, "target")
    ) {
      return endpointError();
    }
  }
  for (const { count, nodeId } of projection.workStateCounts) {
    if (
      !Number.isSafeInteger(count) ||
      count < 0 ||
      nodes.get(nodeId)?.kind !== "work-state"
    ) {
      return projectionError();
    }
  }
  for (const occupancy of projection.activity.resourceOccupancy) {
    if (nodes.get(occupancy.resourceNodeId)?.kind !== "resource") {
      return projectionError();
    }
  }
  return undefined;
}

function validEndpoint(
  nodes: ReadonlyMap<string, FactoryTopologyReplayNode>,
  endpoint: { handleId: string; nodeId: string },
  role: "source" | "target",
): boolean {
  return Boolean(
    nodes
      .get(endpoint.nodeId)
      ?.handles.some(
        (handle) => handle.id === endpoint.handleId && handle.role === role,
      ),
  );
}

function endpointError(): FactoryVisualizerError {
  return {
    kind: "endpoint",
    message: "Factory topology contains an invalid connection endpoint.",
    recoverable: false,
  };
}

function projectionError(): FactoryVisualizerError {
  return {
    kind: "projection",
    message: "Factory topology projection is inconsistent.",
    recoverable: false,
  };
}
