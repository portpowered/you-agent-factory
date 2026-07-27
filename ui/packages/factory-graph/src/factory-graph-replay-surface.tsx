import {
  Background,
  Controls,
  ReactFlow,
  type Edge,
} from "@xyflow/react";
import { GraphViewportSurface } from "@you-agent-factory/components/graphs";
import type { FactoryTopologyNode } from "@you-agent-factory/factory-replay";

import type { FactoryGraphSource } from "./source.js";
import {
  FACTORY_GRAPH_NODE_TYPES,
  type FactoryGraphNode,
} from "./semantic-nodes.js";
import type { FactoryGraphNodeHandle } from "./semantic-node-shell.js";

export interface FactoryGraphReplaySurfaceProps {
  className?: string;
  onSelectNode?: (nodeId: string) => void;
  selectedNodeId?: string;
  source: FactoryGraphSource;
}

/**
 * Read-only canonical Factory graph for replay and emulator hosts.
 * It consumes the complete Factory and its selected-tick runtime projection,
 * retaining the authored Factory layout rather than re-inventing topology.
 */
export function FactoryGraphReplaySurface({
  className,
  onSelectNode,
  selectedNodeId,
  source,
}: FactoryGraphReplaySurfaceProps) {
  const flow = projectFactoryGraphReplayFlow(source, selectedNodeId);
  return (
    <GraphViewportSurface className={className} data-factory-graph-replay>
      <ReactFlow
        defaultViewport={source.factory.layout?.viewport}
        edges={flow.edges}
        edgesFocusable={false}
        fitView
        fitViewOptions={{ padding: 0.12 }}
        nodes={flow.nodes}
        nodesConnectable={false}
        nodesDraggable={false}
        nodeTypes={FACTORY_GRAPH_NODE_TYPES}
        onNodeClick={(_event, node) => onSelectNode?.(node.id)}
        proOptions={{ hideAttribution: true }}
      >
        <Background />
        <Controls showInteractive={false} />
      </ReactFlow>
    </GraphViewportSurface>
  );
}

export interface FactoryGraphReplayFlow {
  edges: Edge[];
  nodes: FactoryGraphNode[];
}

/** Project replay data into the original Factory semantic node family. */
export function projectFactoryGraphReplayFlow(
  source: FactoryGraphSource,
  selectedNodeId?: string,
): FactoryGraphReplayFlow {
  const positions = new Map(
    (source.factory.layout?.nodes ?? []).map((node) => [
      node.id,
      node.position,
    ]),
  );
  const activeIds = replayActiveNodeIds(source);
  const topologyNodes = source.runtime.topology.nodes.map((topologyNode, index) =>
    semanticNode(topologyNode, {
      active: activeIds.has(topologyNode.id),
      position: positions.get(topologyNode.id) ?? fallbackPosition(index),
      selected: topologyNode.id === selectedNodeId,
      source,
    }),
  );
  const docs = (source.factory.supportingFiles?.bundledFiles ?? []).flatMap(
    (file, index) => {
      const id = `doc:${file.id?.trim() || file.targetPath}`;
      if (topologyNodes.some((node) => node.id === id)) return [];
      return [
        {
          data: {
            displayLabel: file.targetPath.split("/").at(-1) ?? file.targetPath,
            factoryGraphNodeId: id,
            fileType: file.type,
            handles: [],
            kind: "doc" as const,
            selectedDoc: id === selectedNodeId,
            targetPath: file.targetPath,
          },
          id,
          position:
            positions.get(id) ?? fallbackPosition(topologyNodes.length + index),
          type: "doc" as const,
        },
      ];
    },
  );
  return {
    edges: source.runtime.topology.connections.map((connection) => ({
      animated: source.runtime.activity.activeDispatchOverlays.some((overlay) =>
        overlay.connectionIds.includes(connection.id),
      ),
      id: connection.id,
      source: connection.source.nodeId,
      sourceHandle: connection.source.handleId,
      target: connection.target.nodeId,
      targetHandle: connection.target.handleId,
    })),
    nodes: [...topologyNodes, ...docs],
  };
}

function semanticNode(
  node: FactoryTopologyNode,
  input: {
    active: boolean;
    position: { x: number; y: number };
    selected: boolean;
    source: FactoryGraphSource;
  },
): FactoryGraphNode {
  const handles = node.handles.map(toSemanticHandle);
  const base = { position: input.position };
  switch (node.kind) {
    case "worker":
      return {
        ...base,
        data: {
          activeFlow: input.active,
          factoryGraphNodeId: node.id,
          handles,
          kind: "worker",
          muted: false,
          place: { place_id: node.id, state_value: node.label },
          selectedWorker: input.selected,
        },
        id: node.id,
        type: "worker",
      };
    case "work-type":
      return {
        ...base,
        data: {
          activeFlow: input.active,
          factoryGraphNodeId: node.id,
          handles,
          kind: "work-type",
          muted: false,
          place: { place_id: node.id, state_value: node.label },
          selectedWorkType: input.selected,
        },
        id: node.id,
        type: "workType",
      };
    case "resource":
      return {
        ...base,
        data: {
          activeFlow: input.active,
          factoryGraphNodeId: node.id,
          handles,
          kind: "resource",
          muted: false,
          place: { kind: "resource", place_id: node.id, type_id: node.label },
          selectedResource: input.selected,
          tokenCount: resourceCount(node.id, input.source),
        },
        id: node.id,
        type: "resource",
      };
    case "work-state":
      return {
        ...base,
        data: {
          activeFlow: input.active,
          activeItemLabels: [],
          factoryGraphNodeId: node.id,
          handles,
          muted: false,
          place: {
            kind: "work_state",
            place_id: node.id,
            state_category: node.category,
            state_value: node.label,
          },
          selectedStateNode: input.selected,
          tokenCount: workStateCount(node.id, input.source),
        },
        id: node.id,
        type: "statePosition",
      };
    case "workstation":
      return {
        ...base,
        data: {
          active: input.active,
          activeFlow: input.active,
          executions: [],
          factoryGraphNodeId: node.id,
          handles,
          muted: false,
          now: 0,
          selectedWorkID: null,
          selectedWorkstation: input.selected,
          summaryOnly: true,
          workstation: {
            node_id: node.id,
            transition_id: node.label,
            workstation_name: node.label,
          },
        },
        id: node.id,
        type: "workstation",
      };
  }
}

function toSemanticHandle(
  handle: FactoryTopologyNode["handles"][number],
): FactoryGraphNodeHandle {
  return {
    connectable: false,
    id: handle.id,
    label: handle.id,
    side: handle.role === "target" ? "left" : "right",
    type: handle.role,
  };
}

function replayActiveNodeIds(source: FactoryGraphSource): ReadonlySet<string> {
  const ids = new Set<string>();
  for (const overlay of source.runtime.activity.activeDispatchOverlays) {
    if (overlay.workstationNodeId) ids.add(overlay.workstationNodeId);
    if (overlay.workerNodeId) ids.add(overlay.workerNodeId);
    for (const id of overlay.resourceNodeIds ?? []) ids.add(id);
  }
  return ids;
}

function resourceCount(nodeId: string, source: FactoryGraphSource): number {
  const occupancy = source.runtime.load.resourceOccupancy.find(
    (entry) => entry.resourceNodeId === nodeId,
  );
  return occupancy?.evidence === "known"
    ? (occupancy.occupiedQuantity ?? 0)
    : 0;
}

function workStateCount(nodeId: string, source: FactoryGraphSource): number {
  const count = source.runtime.load.workStateCounts.find(
    (entry) => entry.workStateNodeId === nodeId,
  );
  return count?.evidence === "known" ? (count.count ?? 0) : 0;
}

function fallbackPosition(index: number): { x: number; y: number } {
  return { x: (index % 4) * 280, y: Math.floor(index / 4) * 180 };
}
