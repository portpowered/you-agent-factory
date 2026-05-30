import "@xyflow/react/dist/style.css";

import {
  applyNodeChanges,
  type NodeChange,
  ReactFlow,
} from "@xyflow/react";
import { useCallback, useEffect, useMemo, useState } from "react";
import type { DashboardWorkRelation } from "../../../api/dashboard/types";
import {
  DashboardGraphBackground,
  DashboardGraphControls,
  DashboardGraphFrame,
} from "../../../components/dashboard/dashboard-graph";
import { FACTORY_GRAPH_EDITOR_EDGE_TYPES } from "../../factory-graph-editor/components/factory-graph-editor-edge";
import {
  traceRelationTopologyLayoutKey,
  useTraceRelationFactoryGraphLayoutPositions,
} from "../hooks/use-trace-relation-factory-graph-layout";
import { buildTraceRelationFactoryGraphFlow } from "../lib/trace-relation-factory-graph-flow";
import type { TraceRelationFlowNode } from "../lib/trace-relation-factory-graph-flow";
import { failOnTraceReactFlowError } from "../lib/trace-react-flow-error";
import { useMeasuredTraceGraphViewport } from "../lib/use-measured-trace-graph-viewport";
import { getTraceDrilldownMessages } from "../messages/trace-drilldown";
import { TRACE_RELATION_FACTORY_GRAPH_NODE_TYPES } from "./trace-relation-factory-graph-node";

const GRAPH_SHELL_CLASS =
  "max-w-full min-w-80 resize overflow-hidden border-transparent bg-af-surface-subtle";
const GRAPH_SHELL_STYLE = { height: 352, minHeight: 288 };
const GRAPH_VIEWPORT_STYLE = { height: "100%", width: "100%" };
const GRAPH_FIT_VIEW_OPTIONS = { maxZoom: 1.5, padding: 0.08 } as const;

export interface TraceRelationFlowProps {
  locale?: string;
  onSelectWorkID?: (workID: string) => void;
  relations: DashboardWorkRelation[];
}

export function TraceRelationFlow({
  locale,
  onSelectWorkID,
  relations,
}: TraceRelationFlowProps) {
  const graph = useMemo(
    () =>
      buildTraceRelationFactoryGraphFlow(relations, {
        locale,
        onSelectWorkID,
      }),
    [locale, onSelectWorkID, relations],
  );
  const topologyKey = useMemo(
    () => traceRelationTopologyLayoutKey(graph.topology),
    [graph.topology],
  );
  const positionsByTraceNodeId = useTraceRelationFactoryGraphLayoutPositions(
    graph.topology,
    graph.endpointKeyByNodeId,
    topologyKey,
  );
  const baseNodes = useMemo<TraceRelationFlowNode[]>(() => {
    return graph.nodes.map((node) => ({
      ...node,
      data: {
        ...node.data,
        onSelectWorkID,
        selectable: Boolean(node.data.workID && onSelectWorkID),
      },
      position: positionsByTraceNodeId.get(node.id) ?? node.position,
    }));
  }, [graph.nodes, onSelectWorkID, positionsByTraceNodeId]);
  const [nodes, setNodes] = useState<TraceRelationFlowNode[]>(baseNodes);

  useEffect(() => {
    setNodes((currentNodes) => {
      const currentPositions = new Map(
        currentNodes.map((node) => [node.id, node.position]),
      );

      return baseNodes.map((node) => ({
        ...node,
        position: currentPositions.get(node.id) ?? node.position,
      }));
    });
  }, [baseNodes]);

  const handleNodesChange = useCallback(
    (changes: NodeChange<TraceRelationFlowNode>[]) => {
      setNodes((currentNodes) => applyNodeChanges(changes, currentNodes));
    },
    [],
  );
  const { graphViewportReady, graphViewportRef } =
    useMeasuredTraceGraphViewport();

  if (relations.length === 0) {
    return <span>{getTraceDrilldownMessages(locale).noBatchRelations}</span>;
  }

  return (
    <DashboardGraphFrame
      aria-label={getTraceDrilldownMessages(locale).batchRelationGraphLabel}
      className={GRAPH_SHELL_CLASS}
      data-trace-relation-flow
      style={GRAPH_SHELL_STYLE}
    >
      <div
        className="h-full min-w-0 w-full"
        data-trace-graph-viewport
        ref={graphViewportRef}
        style={GRAPH_VIEWPORT_STYLE}
      >
        {graphViewportReady ? (
          <TraceRelationReactFlow
            edges={graph.edges}
            nodes={nodes}
            onNodesChange={handleNodesChange}
          />
        ) : null}
      </div>
    </DashboardGraphFrame>
  );
}

function TraceRelationReactFlow({
  edges,
  nodes,
  onNodesChange,
}: {
  edges: ReturnType<typeof buildTraceRelationFactoryGraphFlow>["edges"];
  nodes: TraceRelationFlowNode[];
  onNodesChange: (changes: NodeChange<TraceRelationFlowNode>[]) => void;
}) {
  return (
    <ReactFlow
      edges={edges}
      edgeTypes={FACTORY_GRAPH_EDITOR_EDGE_TYPES}
      fitView
      fitViewOptions={GRAPH_FIT_VIEW_OPTIONS}
      maxZoom={2}
      minZoom={0.35}
      nodes={nodes}
      nodesDraggable={true}
      nodeTypes={TRACE_RELATION_FACTORY_GRAPH_NODE_TYPES}
      onNodesChange={onNodesChange}
      onError={failOnTraceReactFlowError}
      panOnDrag
      proOptions={{ hideAttribution: true }}
      zoomOnScroll
    >
      <DashboardGraphBackground />
      <DashboardGraphControls fitViewOptions={GRAPH_FIT_VIEW_OPTIONS} />
    </ReactFlow>
  );
}
