import {
  Background,
  Controls,
  type Edge,
  type Node,
  type NodeProps,
  ReactFlow,
} from "@xyflow/react";
import {
  GraphNodeButton,
  type GraphNodeHandle,
  GraphNodeShell,
} from "@you-agent-factory/components/graphs";
import type {
  FactoryActivityProjection,
  FactoryLoadProjection,
  FactoryTopologyConnection,
  FactoryTopologyNode,
  FactoryTopologyProjection,
} from "@you-agent-factory/factory-replay";
import { useMemo } from "react";

export interface FactoryTopologyReplayProjection {
  activity: FactoryActivityProjection;
  load: FactoryLoadProjection;
  topology: FactoryTopologyProjection;
}

export interface FactoryTopologyReplayMessages {
  activeDispatches: (count: number) => string;
  inactiveDispatches: string;
  nodeLabel: (kind: FactoryTopologyNode["kind"], label: string) => string;
  regionLabel: string;
  resourceOccupancy: (occupied: number, capacity: number) => string;
  resourceOccupancyUnavailable: string;
  selectedNode: string;
  workStateCount: (count: number) => string;
  workStateCountUnavailable: string;
}

export interface FactoryTopologyReplayProps {
  messages: FactoryTopologyReplayMessages;
  onSelectNode?: (node: FactoryTopologyNode) => void;
  projection: FactoryTopologyReplayProjection;
  selectedNodeId?: string;
}

interface TopologyNodeData extends Record<string, unknown> {
  activityCount: number;
  messages: FactoryTopologyReplayMessages;
  node: FactoryTopologyNode;
  occupancy?: {
    capacity?: number;
    evidence: "known" | "unavailable";
    occupied?: number;
  };
  onSelectNode?: (node: FactoryTopologyNode) => void;
  selected: boolean;
  workStateCount?: {
    count?: number;
    evidence: "known" | "unavailable";
  };
}

export interface FactoryTopologyFlowProjection {
  edges: Edge[];
  nodes: Node<TopologyNodeData>[];
  validEndpoints: boolean;
}

const nodeTypes = { factoryTopologyNode: FactoryTopologyNodeView };
const columnByKind: Record<FactoryTopologyNode["kind"], number> = {
  resource: 0,
  worker: 1,
  "work-type": 2,
  "work-state": 3,
  workstation: 4,
};

/** Convert immutable replay projections into disposable React Flow view data. */
export function projectFactoryTopologyFlow(
  projection: FactoryTopologyReplayProjection,
  messages: FactoryTopologyReplayMessages,
  selectedNodeId: string | undefined,
  onSelectNode: FactoryTopologyReplayProps["onSelectNode"],
): FactoryTopologyFlowProjection {
  const topologyNodes = projection.topology.nodes;
  const nodeById = new Map(topologyNodes.map((node) => [node.id, node]));
  const endpointsValid = projection.topology.connections.every((connection) =>
    connectionHasRenderedEndpoints(connection, nodeById),
  );
  const activityCountByNode = activityCounts(projection.activity);
  const occupancyByNode = new Map(
    projection.load.resourceOccupancy.map((occupancy) => [
      occupancy.resourceNodeId,
      occupancy,
    ]),
  );
  const workStateCountByNode = new Map(
    projection.load.workStateCounts.map((count) => [
      count.workStateNodeId,
      count,
    ]),
  );
  const rowByKind = new Map<FactoryTopologyNode["kind"], number>();

  const nodes = topologyNodes.map<Node<TopologyNodeData>>((node) => {
    const row = rowByKind.get(node.kind) ?? 0;
    rowByKind.set(node.kind, row + 1);
    const occupancy = occupancyByNode.get(node.id);
    const workStateCount = workStateCountByNode.get(node.id);
    return {
      data: {
        activityCount: activityCountByNode.get(node.id) ?? 0,
        messages,
        node,
        ...(occupancy
          ? {
              occupancy: {
                capacity: occupancy.capacity,
                evidence: occupancy.evidence,
                occupied: occupancy.occupiedQuantity,
              },
            }
          : {}),
        onSelectNode,
        selected: selectedNodeId === node.id,
        ...(workStateCount
          ? {
              workStateCount: {
                count: workStateCount.count,
                evidence: workStateCount.evidence,
              },
            }
          : {}),
      },
      draggable: false,
      id: node.id,
      position: { x: columnByKind[node.kind] * 260, y: row * 170 },
      selectable: false,
      type: "factoryTopologyNode",
    };
  });

  return {
    edges: endpointsValid
      ? projection.topology.connections.map((connection) => ({
          animated: projection.activity.activeDispatchOverlays.some((overlay) =>
            overlay.connectionIds.includes(connection.id),
          ),
          data: { relationship: connection.kind },
          id: connection.id,
          source: connection.source.nodeId,
          sourceHandle: connection.source.handleId,
          target: connection.target.nodeId,
          targetHandle: connection.target.handleId,
        }))
      : [],
    nodes,
    validEndpoints: endpointsValid,
  };
}

export function FactoryTopologyReplay({
  messages,
  onSelectNode,
  projection,
  selectedNodeId,
}: FactoryTopologyReplayProps) {
  const flow = useMemo(
    () =>
      projectFactoryTopologyFlow(
        projection,
        messages,
        selectedNodeId,
        onSelectNode,
      ),
    [messages, onSelectNode, projection, selectedNodeId],
  );

  return (
    <section
      aria-label={messages.regionLabel}
      className="factory-topology-replay"
      data-endpoints-valid={flow.validEndpoints ? "true" : "false"}
    >
      <ReactFlow
        edges={flow.edges}
        edgesFocusable={false}
        elementsSelectable={false}
        fitView
        nodes={flow.nodes}
        nodesConnectable={false}
        nodesDraggable={false}
        nodeTypes={nodeTypes}
        panOnDrag
        proOptions={{ hideAttribution: true }}
      >
        <Background />
        <Controls showInteractive={false} />
      </ReactFlow>
    </section>
  );
}

function FactoryTopologyNodeView({ data }: NodeProps<Node<TopologyNodeData>>) {
  const {
    activityCount,
    messages,
    node,
    occupancy,
    onSelectNode,
    selected,
    workStateCount,
  } = data;
  const state = selected ? "selected" : "default";
  const handles: GraphNodeHandle[] = node.handles.map((handle) => ({
    connectable: false,
    id: handle.id,
    label: handle.id,
    side: handle.role === "target" ? "left" : "right",
    type: handle.role,
  }));
  const content = (
    <GraphNodeShell
      className={
        activityCount > 0 ? "factory-topology-replay__node--active" : ""
      }
      data-dispatch-activity={activityCount > 0 ? "active" : "inactive"}
      handles={handles}
      nodeKind={node.kind}
      showStateIndicator={false}
      state={state}
    >
      <strong className="factory-topology-replay__node-title">
        {node.label}
      </strong>
      <span className="factory-topology-replay__node-kind">{node.kind}</span>
      <span className="factory-topology-replay__node-cue">
        {activityCount > 0 ? "●" : "○"}{" "}
        {activityCount > 0
          ? messages.activeDispatches(activityCount)
          : messages.inactiveDispatches}
      </span>
      {node.kind === "resource" ? (
        <span className="factory-topology-replay__node-cue">
          ◫{" "}
          {occupancy?.evidence === "known" &&
          occupancy.occupied !== undefined &&
          occupancy.capacity !== undefined
            ? messages.resourceOccupancy(occupancy.occupied, occupancy.capacity)
            : messages.resourceOccupancyUnavailable}
        </span>
      ) : null}
      {node.kind === "work-state" ? (
        <span className="factory-topology-replay__node-cue">
          ∑{" "}
          {workStateCount?.evidence === "known" &&
          workStateCount.count !== undefined
            ? messages.workStateCount(workStateCount.count)
            : messages.workStateCountUnavailable}
        </span>
      ) : null}
      {selected ? (
        <span className="factory-topology-replay__node-cue">
          ✓ {messages.selectedNode}
        </span>
      ) : null}
    </GraphNodeShell>
  );

  return onSelectNode ? (
    <GraphNodeButton
      aria-label={messages.nodeLabel(node.kind, node.label)}
      className="factory-topology-replay__node-button"
      graphState={state}
      onClick={() => onSelectNode(node)}
    >
      {content}
    </GraphNodeButton>
  ) : (
    <figure
      aria-label={messages.nodeLabel(node.kind, node.label)}
      className="factory-topology-replay__node-static"
    >
      {content}
    </figure>
  );
}

function connectionHasRenderedEndpoints(
  connection: FactoryTopologyConnection,
  nodeById: ReadonlyMap<string, FactoryTopologyNode>,
): boolean {
  const source = nodeById.get(connection.source.nodeId);
  const target = nodeById.get(connection.target.nodeId);
  return Boolean(
    source?.handles.some(
      (handle) =>
        handle.id === connection.source.handleId && handle.role === "source",
    ) &&
      target?.handles.some(
        (handle) =>
          handle.id === connection.target.handleId && handle.role === "target",
      ),
  );
}

function activityCounts(
  activity: FactoryActivityProjection,
): Map<string, number> {
  const counts = new Map<string, number>();
  for (const overlay of activity.activeDispatchOverlays) {
    const nodeIds = new Set([
      overlay.workerNodeId,
      overlay.workstationNodeId,
      ...(overlay.resourceNodeIds ?? []),
    ]);
    for (const nodeId of nodeIds) {
      if (nodeId) counts.set(nodeId, (counts.get(nodeId) ?? 0) + 1);
    }
  }
  return counts;
}
