import "@xyflow/react/dist/style.css";

import {
  applyNodeChanges,
  type NodeChange,
  ReactFlow,
  type ReactFlowInstance,
} from "@xyflow/react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type {
  DashboardWorkItemRef,
  DashboardWorkRelation,
} from "../../../api/dashboard/types";
import {
  DashboardGraphBackground,
  DashboardGraphControls,
  DashboardGraphFrame,
} from "../../../components/dashboard/dashboard-graph";
import {
  FACTORY_GRAPH_EDGE_TYPES,
  WORK_RELATION_NODE_TYPES,
} from "../../graphs/public";
import {
  traceRelationTopologyLayoutKey,
  useTraceRelationFactoryGraphLayoutPositions,
} from "../hooks/use-trace-relation-factory-graph-layout";
import { applyTraceFactoryGraphLayoutToNode } from "../lib/trace-factory-graph-layout";
import { failOnTraceReactFlowError } from "../lib/trace-react-flow-error";
import type { TraceRelationFlowNode } from "../lib/trace-relation-factory-graph-flow";
import { buildTraceRelationFactoryGraphFlow } from "../lib/trace-relation-factory-graph-flow";
import { useMeasuredTraceGraphViewport } from "../lib/use-measured-trace-graph-viewport";
import { getTraceDrilldownMessages } from "../messages/trace-drilldown";

const GRAPH_SHELL_STYLE = { height: 352, minHeight: 288 };
const GRAPH_VIEWPORT_STYLE = { height: "100%", width: "100%" };
const GRAPH_FIT_VIEW_OPTIONS = { maxZoom: 1.5, padding: 0.08 } as const;

export interface TraceRelationFlowProps {
  locale?: string;
  onSelectWorkID?: (workID: string) => void;
  relations: DashboardWorkRelation[];
  selectedWorkID?: string | null;
  workItemsByWorkId?: ReadonlyMap<string, DashboardWorkItemRef>;
}

export function TraceRelationFlow({
  locale,
  onSelectWorkID,
  relations,
  selectedWorkID,
  workItemsByWorkId,
}: TraceRelationFlowProps) {
  const graph = useMemo(
    () =>
      buildTraceRelationFactoryGraphFlow(relations, {
        locale,
        onSelectWorkID,
        selectedWorkID,
        workItemsByWorkId,
      }),
    [locale, onSelectWorkID, relations, selectedWorkID, workItemsByWorkId],
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
    return graph.nodes.map((node) =>
      applyTraceFactoryGraphLayoutToNode(
        {
          ...node,
          data: {
            ...node.data,
            onSelectWorkID,
            selectable: Boolean(
              node.data.workID && onSelectWorkID && !node.data.isSelectedWork,
            ),
          },
        },
        positionsByTraceNodeId,
      ),
    );
  }, [graph.nodes, onSelectWorkID, positionsByTraceNodeId]);
  const [nodes, setNodes] = useState<TraceRelationFlowNode[]>(baseNodes);
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

  const renderedNodes =
    topologyKeyRef.current === topologyKey ? nodes : baseNodes;
  const fitViewKey = useMemo(
    () =>
      `${topologyKey}:${baseNodes
        .map(
          (node) =>
            `${node.id}:${node.position.x},${node.position.y}:${node.width ?? 0}x${node.height ?? 0}`,
        )
        .join("|")}`,
    [baseNodes, topologyKey],
  );
  const handleNodesChange = useCallback(
    (changes: NodeChange<TraceRelationFlowNode>[]) => {
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

  if (relations.length === 0) {
    return <span>{getTraceDrilldownMessages(locale).noBatchRelations}</span>;
  }

  return (
    <DashboardGraphFrame
      aria-label={getTraceDrilldownMessages(locale).batchRelationGraphLabel}
      className="max-w-full min-w-80 resize overflow-hidden border-transparent bg-transparent"
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
            fitViewKey={fitViewKey}
            nodes={renderedNodes}
            onNodesChange={handleNodesChange}
          />
        ) : null}
      </div>
    </DashboardGraphFrame>
  );
}

function TraceRelationReactFlow({
  edges,
  fitViewKey,
  nodes,
  onNodesChange,
}: {
  edges: ReturnType<typeof buildTraceRelationFactoryGraphFlow>["edges"];
  fitViewKey: string;
  nodes: TraceRelationFlowNode[];
  onNodesChange: (changes: NodeChange<TraceRelationFlowNode>[]) => void;
}) {
  const flowInstanceRef = useRef<ReactFlowInstance<
    TraceRelationFlowNode,
    ReturnType<typeof buildTraceRelationFactoryGraphFlow>["edges"][number]
  > | null>(null);
  const appliedFitViewKeyRef = useRef<string | null>(null);

  useEffect(() => {
    if (!flowInstanceRef.current || nodes.length === 0) {
      return;
    }

    if (appliedFitViewKeyRef.current === fitViewKey) {
      return;
    }

    appliedFitViewKeyRef.current = fitViewKey;
    const animationFrameID = requestAnimationFrame(() => {
      flowInstanceRef.current?.fitView(GRAPH_FIT_VIEW_OPTIONS);
    });

    return () => {
      cancelAnimationFrame(animationFrameID);
    };
  }, [fitViewKey, nodes]);

  return (
    <ReactFlow
      edges={edges}
      edgeTypes={FACTORY_GRAPH_EDGE_TYPES}
      fitView
      fitViewOptions={GRAPH_FIT_VIEW_OPTIONS}
      maxZoom={2}
      minZoom={0.35}
      nodes={nodes}
      nodesDraggable={true}
      nodeTypes={WORK_RELATION_NODE_TYPES}
      onInit={(instance) => {
        flowInstanceRef.current = instance;
      }}
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
