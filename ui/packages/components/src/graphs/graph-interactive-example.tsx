import "@xyflow/react/dist/style.css";

import {
  Background,
  Controls,
  type Edge,
  type Node,
  type NodeProps,
  ReactFlow,
  ReactFlowProvider,
} from "@xyflow/react";
import { useCallback, useMemo, useState } from "react";

import { GRAPH_EDGE_TYPES } from "./graph-edge";
import { GraphNodeButton } from "./graph-node-button";
import { GraphNodeShell } from "./graph-node-shell";
import type { GraphNodeState } from "./graph-node-state";
import {
  desktopInteractiveGraphNodes,
  type GraphInteractiveFixtureNode,
} from "./graph-story-fixtures";
import { GraphViewportSurface } from "./graph-viewport-surface";

type GenericGraphFlowNodeData = GraphInteractiveFixtureNode & {
  graphState: GraphNodeState;
  onActivate?: () => void;
  shellState: GraphNodeState;
};

function GenericGraphFlowNode({
  data,
}: NodeProps<Node<GenericGraphFlowNodeData>>) {
  return (
    <div className="w-44 max-w-full sm:w-52">
      <GraphNodeShell
        handles={data.handles}
        nodeKind={data.nodeKind}
        state={data.shellState}
        stateLabel={data.stateLabel}
      >
        <GraphNodeButton
          graphState={data.graphState}
          onClick={data.onActivate}
          stateLabel={data.stateLabel}
        >
          {data.label}
        </GraphNodeButton>
      </GraphNodeShell>
    </div>
  );
}

const GENERIC_GRAPH_NODE_TYPES = {
  genericGraph: GenericGraphFlowNode,
};

function buildFlowNodes(
  fixtureNodes: GraphInteractiveFixtureNode[],
  selectedNodeId: string | null,
  onSelectNode: (nodeId: string) => void,
): Node<GenericGraphFlowNodeData>[] {
  return fixtureNodes.map((fixtureNode) => {
    const isSelectable = fixtureNode.selectable === true;
    const isSelected = isSelectable && selectedNodeId === fixtureNode.id;
    const resolvedState =
      fixtureNode.fixedState ?? (isSelected ? "selected" : "default");

    return {
      data: {
        ...fixtureNode,
        graphState: resolvedState,
        onActivate: isSelectable
          ? () => {
              onSelectNode(fixtureNode.id);
            }
          : undefined,
        shellState: resolvedState,
      },
      draggable: false,
      id: fixtureNode.id,
      position: fixtureNode.position,
      selectable: false,
      type: "genericGraph",
    };
  });
}

function buildFlowEdges(
  fixtureNodes: GraphInteractiveFixtureNode[],
  selectedNodeId: string | null,
): Edge[] {
  const readyNode = fixtureNodes.find((node) => node.id === "ready-node");
  const targetNode = fixtureNodes.find((node) => node.id === "target-node");

  if (!readyNode || !targetNode) {
    return [];
  }

  return [
    {
      data: {
        alwaysShowLabel: true,
        label: "Example edge",
        waypoints:
          readyNode.position.y === targetNode.position.y
            ? undefined
            : [
                {
                  x: (readyNode.position.x + targetNode.position.x) / 2 + 72,
                  y: readyNode.position.y + 48,
                },
                {
                  x: (readyNode.position.x + targetNode.position.x) / 2 + 72,
                  y: targetNode.position.y + 48,
                },
              ],
      },
      id: "ready-to-target",
      selectable: false,
      source: readyNode.id,
      target: targetNode.id,
      type: "graphEdge",
    },
  ].map((edge) => ({
    ...edge,
    selected: selectedNodeId === edge.source || selectedNodeId === edge.target,
  }));
}

export type GraphInteractiveExampleProps = {
  "aria-label"?: string;
  className?: string;
  fixtureNodes?: GraphInteractiveFixtureNode[];
  initialSelectedNodeId?: string | null;
  viewportWidthClass?: string;
};

export function GraphInteractiveExample({
  "aria-label": ariaLabel = "Interactive graph example",
  className = "h-[28rem]",
  fixtureNodes = desktopInteractiveGraphNodes,
  initialSelectedNodeId = null,
  viewportWidthClass = "w-[48rem] max-w-full",
}: GraphInteractiveExampleProps) {
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(
    initialSelectedNodeId,
  );

  const handleSelectNode = useCallback((nodeId: string) => {
    setSelectedNodeId(nodeId);
  }, []);

  const nodes = useMemo(
    () => buildFlowNodes(fixtureNodes, selectedNodeId, handleSelectNode),
    [fixtureNodes, handleSelectNode, selectedNodeId],
  );
  const edges = useMemo(
    () => buildFlowEdges(fixtureNodes, selectedNodeId),
    [fixtureNodes, selectedNodeId],
  );

  return (
    <ReactFlowProvider>
      <div className={viewportWidthClass}>
        <GraphViewportSurface aria-label={ariaLabel} className={className}>
          <ReactFlow
            defaultEdgeOptions={{ selectable: false }}
            edgeTypes={GRAPH_EDGE_TYPES}
            edges={edges}
            fitView
            fitViewOptions={{ padding: 0.2 }}
            maxZoom={1.25}
            minZoom={0.5}
            nodeTypes={GENERIC_GRAPH_NODE_TYPES}
            nodes={nodes}
            nodesConnectable={false}
            nodesDraggable={false}
            panOnDrag={true}
            proOptions={{ hideAttribution: true }}
            zoomOnScroll={true}
          >
            <Background gap={16} size={1} />
            <Controls showInteractive={false} />
          </ReactFlow>
        </GraphViewportSurface>
      </div>
    </ReactFlowProvider>
  );
}
