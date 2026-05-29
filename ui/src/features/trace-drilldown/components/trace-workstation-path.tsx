import "@xyflow/react/dist/style.css";

import {
  applyNodeChanges,
  type Edge,
  Handle,
  MarkerType,
  type Node,
  type NodeChange,
  type NodeProps,
  Position,
  ReactFlow,
} from "@xyflow/react";
import { useCallback, useEffect, useMemo, useState } from "react";
import type { DashboardTraceDispatch } from "../../../api/dashboard/types";
import {
  DashboardGraphBackground,
  DashboardGraphControls,
  DashboardGraphFrame,
} from "../../../components/dashboard/dashboard-graph";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SUPPORTING_LABEL_CLASS,
} from "../../../components/ui/dashboard-typography";
import { formatTraceOutcome } from "../../../components/ui/formatters";
import { cn } from "../../../lib/cn";
import {
  getCachedTraceGraphLayout,
  layoutTraceGraphWithElk,
  traceGraphLayoutKey,
} from "../lib/trace-elk-layout";
import { failOnTraceReactFlowError } from "../lib/trace-react-flow-error";
import { useMeasuredTraceGraphViewport } from "../lib/use-measured-trace-graph-viewport";
import { projectTraceDispatchesToFactoryGraph } from "../lib/trace-dispatch-factory-graph";
import { getTraceDrilldownMessages } from "../messages/trace-drilldown";

const GRAPH_SHELL_CLASS =
  "max-w-full min-w-80 resize overflow-hidden border-transparent bg-af-surface-subtle";
const GRAPH_SHELL_STYLE = { height: 320, minHeight: 256 };
const GRAPH_VIEWPORT_STYLE = { height: "100%", width: "100%" };
const PATH_NODE_CLASS =
  "flex h-full min-w-0 w-full flex-col gap-1.5 overflow-hidden rounded-lg border px-3 py-3 text-left text-af-text shadow-af-card";
const DISPATCH_NODE_WIDTH = 240;
const DISPATCH_NODE_HEIGHT = 124;
const TRACE_DISPATCH_FLOW_FIT_VIEW_OPTIONS = {
  maxZoom: 1.15,
  padding: 0.16,
} as const;

interface PathNodeData extends Record<string, unknown> {
  inputSummary: string;
  label: string;
  locale?: string;
  outcome?: string;
  outputSummary: string;
}

type WorkstationPathNode = Node<PathNodeData, "trace-workstation">;

const PATH_NODE_TYPES = {
  "trace-workstation": WorkstationPathGraphNode,
};

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
    () => buildDispatchGraph(dispatches, locale),
    [dispatches, locale],
  );
  const graphDimensions = useMemo(
    () =>
      new Map(
        graph.nodes.map((node) => [
          node.id,
          {
            height: DISPATCH_NODE_HEIGHT,
            id: node.id,
            width: DISPATCH_NODE_WIDTH,
          },
        ]),
      ),
    [graph.nodes],
  );
  const layoutKey = useMemo(
    () => traceGraphLayoutKey(graph.nodes, graph.edges, graphDimensions),
    [graph.edges, graph.nodes, graphDimensions],
  );
  const [layoutedNodes, setLayoutedNodes] = useState<WorkstationPathNode[]>(
    () => getCachedTraceGraphLayout(layoutKey, graph.nodes) ?? graph.nodes,
  );

  useEffect(() => {
    setLayoutedNodes(
      getCachedTraceGraphLayout(layoutKey, graph.nodes) ?? graph.nodes,
    );
  }, [graph.nodes, layoutKey]);

  useEffect(() => {
    let cancelled = false;

    void layoutTraceGraphWithElk(
      graph.nodes,
      graph.edges,
      graphDimensions,
    ).then((nextNodes) => {
      if (!cancelled) {
        setLayoutedNodes(nextNodes);
      }
    });

    return () => {
      cancelled = true;
    };
  }, [graph.edges, graph.nodes, graphDimensions]);

  const baseNodes = useMemo(() => {
    const positionsByID = new Map(
      layoutedNodes.map((node) => [node.id, node.position]),
    );

    return graph.nodes.map((node) => ({
      ...node,
      position: positionsByID.get(node.id) ?? node.position,
    }));
  }, [graph.nodes, layoutedNodes]);
  const [nodes, setNodes] = useState<WorkstationPathNode[]>(baseNodes);

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
    (changes: NodeChange<WorkstationPathNode>[]) => {
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
  edges: Edge[];
  nodes: WorkstationPathNode[];
  onNodesChange: (changes: NodeChange<WorkstationPathNode>[]) => void;
}) {
  return (
    <ReactFlow
      defaultEdgeOptions={{
        animated: false,
        markerEnd: {
          color: "var(--color-af-edge-muted)",
          type: MarkerType.ArrowClosed,
        },
        style: {
          stroke: "var(--color-af-edge-muted)",
          strokeWidth: 1.7,
        },
        type: "smoothstep",
      }}
      edges={edges}
      fitView
      fitViewOptions={TRACE_DISPATCH_FLOW_FIT_VIEW_OPTIONS}
      maxZoom={1.8}
      minZoom={0.35}
      nodes={nodes}
      nodesDraggable={true}
      nodeTypes={PATH_NODE_TYPES}
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

function WorkstationPathGraphNode({ data }: NodeProps<WorkstationPathNode>) {
  const messages = getTraceDrilldownMessages(data.locale);

  return (
    <article
      className={cn(PATH_NODE_CLASS, outcomeToneClassName(data.outcome))}
    >
      <Handle className="opacity-0" position={Position.Left} type="target" />
      <Handle className="opacity-0" position={Position.Right} type="source" />
      <div className="flex items-center justify-between gap-3">
        <span
          className={cn(
            "inline-flex rounded-full px-2 py-0.5 text-[0.68rem] font-semibold uppercase tracking-[0.12em]",
            DASHBOARD_SUPPORTING_LABEL_CLASS,
          )}
        >
          {messages.dispatchPathSectionLabel}
        </span>
        <span
          className={cn(
            "inline-flex rounded-full px-2 py-0.5 text-[0.68rem] font-semibold uppercase tracking-[0.08em]",
            DASHBOARD_SUPPORTING_LABEL_CLASS,
          )}
        >
          {data.outcome
            ? formatTraceOutcome(data.outcome)
            : messages.dispatchPathPendingOutcome}
        </span>
      </div>
      <strong
        className={cn(
          "text-sm text-af-text [overflow-wrap:anywhere]",
          DASHBOARD_BODY_TEXT_CLASS,
        )}
      >
        {data.label}
      </strong>
      <p className="text-[0.76rem] text-af-text-muted [overflow-wrap:anywhere]">
        {messages.dispatchPathInputPrefix}: {data.inputSummary}
      </p>
      <p className="text-[0.76rem] text-af-text-muted [overflow-wrap:anywhere]">
        {messages.dispatchPathOutputPrefix}: {data.outputSummary}
      </p>
    </article>
  );
}

function buildDispatchGraph(
  dispatches: DashboardTraceDispatch[],
  locale?: string,
): {
  edges: Edge[];
  nodes: WorkstationPathNode[];
} {
  const projection = projectTraceDispatchesToFactoryGraph(dispatches, locale);

  return {
    edges: projection.topology.edges.map((edge) => ({
      id: edge.id,
      source:
        projection.dispatchIdByNodeId.get(edge.sourceId) ?? edge.sourceId,
      target:
        projection.dispatchIdByNodeId.get(edge.targetId) ?? edge.targetId,
    })),
    nodes: projection.topology.nodes.map((node, index) => {
      const overlay = projection.overlaysByNodeId.get(node.id);
      if (!overlay) {
        throw new Error(`Missing trace overlay for factory node ${node.id}.`);
      }

      return {
        data: {
          label: overlay.displayLabel,
          inputSummary: overlay.inputSummary,
          locale,
          outcome: overlay.outcome,
          outputSummary: overlay.outputSummary,
        },
        id: overlay.dispatchId,
        position: { x: index * (DISPATCH_NODE_WIDTH + 24), y: 0 },
        sourcePosition: Position.Right,
        targetPosition: Position.Left,
        type: "trace-workstation",
      };
    }),
  };
}

function outcomeToneClassName(outcome?: string): string {
  if (!outcome) {
    // hardcoded-ui-copy-exception: non-product-diagnostic
    return "border-af-border bg-af-surface";
  }

  if (
    outcome.toUpperCase() === "FAILED" ||
    outcome.toUpperCase() === "REJECTED"
  ) {
    // hardcoded-ui-copy-exception: non-product-diagnostic
    return "border-af-danger-border bg-af-danger-surface";
  }

  // hardcoded-ui-copy-exception: non-product-diagnostic
  return "border-af-success-border bg-af-success-surface";
}
