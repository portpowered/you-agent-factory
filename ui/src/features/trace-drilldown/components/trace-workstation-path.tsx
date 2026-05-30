import "@xyflow/react/dist/style.css";

import {
  applyNodeChanges,
  type NodeChange,
  ReactFlow,
} from "@xyflow/react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { DashboardTraceDispatch } from "../../../api/dashboard/types";
import {
  DashboardGraphBackground,
  DashboardGraphControls,
  DashboardGraphFrame,
} from "../../../components/dashboard/dashboard-graph";
import { FACTORY_GRAPH_EDITOR_EDGE_TYPES } from "../../factory-graph-editor/components/factory-graph-editor-edge";
import {
  traceDispatchTopologyLayoutKey,
  useTraceDispatchFactoryGraphLayoutPositions,
} from "../hooks/use-trace-dispatch-factory-graph-layout";
import { buildTraceDispatchFactoryGraphFlow } from "../lib/trace-dispatch-factory-graph-flow";
import type { TraceDispatchFlowNode } from "../lib/trace-dispatch-factory-graph-flow";
import { failOnTraceReactFlowError } from "../lib/trace-react-flow-error";
import { useMeasuredTraceGraphViewport } from "../lib/use-measured-trace-graph-viewport";
import { getTraceDrilldownMessages } from "../messages/trace-drilldown";
import { TRACE_DISPATCH_FACTORY_GRAPH_NODE_TYPES } from "./trace-dispatch-factory-graph-node";

const GRAPH_SHELL_CLASS =
  "max-w-full min-w-80 resize overflow-hidden border-transparent bg-af-surface-subtle";
const GRAPH_SHELL_STYLE = { height: 320, minHeight: 256 };
const GRAPH_VIEWPORT_STYLE = { height: "100%", width: "100%" };
const TRACE_DISPATCH_FLOW_FIT_VIEW_OPTIONS = {
  maxZoom: 1.15,
  padding: 0.16,
} as const;

export interface TraceWorkstationPathProps {
  dispatches: DashboardTraceDispatch[];
  locale?: string;
}

export function TraceWorkstationPath({
  dispatches,
  locale,
}: TraceWorkstationPathProps) {
  const messages = getTraceDrilldownMessages(locale);
  const graph = useMemo(
    () => buildTraceDispatchFactoryGraphFlow(dispatches, locale),
    [dispatches, locale],
  );
  const topologyKey = useMemo(
    () => traceDispatchTopologyLayoutKey(graph.topology),
    [graph.topology],
  );
  const positionsByTraceNodeId = useTraceDispatchFactoryGraphLayoutPositions(
    graph.topology,
    graph.dispatchIdByNodeId,
    topologyKey,
  );
  const baseNodes = useMemo(() => {
    return graph.nodes.map((node) => ({
      ...node,
      position: positionsByTraceNodeId.get(node.id) ?? node.position,
    }));
  }, [graph.nodes, positionsByTraceNodeId]);
  const [nodes, setNodes] = useState<TraceDispatchFlowNode[]>([]);
  const draggedNodeIdsRef = useRef(new Set<string>());
  const topologyKeyRef = useRef(topologyKey);

  useEffect(() => {
    if (topologyKeyRef.current !== topologyKey) {
      draggedNodeIdsRef.current.clear();
      topologyKeyRef.current = topologyKey;
    }

    setNodes((currentNodes) => {
      const currentPositions = new Map(
        currentNodes.map((node) => [node.id, node.position]),
      );

      return baseNodes.map((node) => {
        const draggedPosition = draggedNodeIdsRef.current.has(node.id)
          ? currentPositions.get(node.id)
          : undefined;

        return {
          ...node,
          position: draggedPosition ?? node.position,
        };
      });
    });
  }, [baseNodes, topologyKey]);

  const handleNodesChange = useCallback(
    (changes: NodeChange<TraceDispatchFlowNode>[]) => {
      for (const change of changes) {
        if (
          change.type === "position" &&
          change.dragging === false &&
          change.id
        ) {
          draggedNodeIdsRef.current.add(change.id);
        }
      }

      setNodes((currentNodes) => applyNodeChanges(changes, currentNodes));
    },
    [],
  );
  const { graphViewportReady, graphViewportRef } =
    useMeasuredTraceGraphViewport();

  if (graph.nodes.length === 0) {
    return <span>{messages.dispatchPathEmpty}</span>;
  }

  return (
    <DashboardGraphFrame
      aria-label={messages.dispatchPathGraphLabel}
      className={GRAPH_SHELL_CLASS}
      data-trace-workstation-path
      style={GRAPH_SHELL_STYLE}
    >
      <div
        className="h-full min-w-0 w-full"
        data-trace-graph-viewport
        ref={graphViewportRef}
        style={GRAPH_VIEWPORT_STYLE}
      >
        {graphViewportReady ? (
          <TraceWorkstationReactFlow
            edges={graph.edges}
            nodes={nodes}
            onNodesChange={handleNodesChange}
          />
        ) : null}
      </div>
    </DashboardGraphFrame>
  );
}

function TraceWorkstationReactFlow({
  edges,
  nodes,
  onNodesChange,
}: {
  edges: ReturnType<typeof buildTraceDispatchFactoryGraphFlow>["edges"];
  nodes: TraceDispatchFlowNode[];
  onNodesChange: (changes: NodeChange<TraceDispatchFlowNode>[]) => void;
}) {
  return (
    <ReactFlow
      edges={edges}
      edgeTypes={FACTORY_GRAPH_EDITOR_EDGE_TYPES}
      fitView
      fitViewOptions={TRACE_DISPATCH_FLOW_FIT_VIEW_OPTIONS}
      maxZoom={1.8}
      minZoom={0.35}
      nodes={nodes}
      nodesDraggable={true}
      nodeTypes={TRACE_DISPATCH_FACTORY_GRAPH_NODE_TYPES}
      onNodesChange={onNodesChange}
      onError={failOnTraceReactFlowError}
      panOnDrag
      proOptions={{ hideAttribution: true }}
      zoomOnScroll
    >
      <DashboardGraphBackground />
      <DashboardGraphControls
        fitViewOptions={TRACE_DISPATCH_FLOW_FIT_VIEW_OPTIONS}
      />
    </ReactFlow>
  );
}
