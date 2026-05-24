import "@xyflow/react/dist/style.css";

import {
  applyNodeChanges,
  Handle,
  MarkerType,
  Position,
  ReactFlow,
  type Edge,
  type Node,
  type NodeChange,
  type NodeProps,
} from "@xyflow/react";
import { useCallback, useEffect, useMemo, useState } from "react";

import { cn } from "../../../lib/cn";
import {
  DashboardGraphBackground,
  DashboardGraphControls,
  DashboardGraphFrame,
} from "../../../components/dashboard/dashboard-graph";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SUPPORTING_LABEL_CLASS,
} from "../../../components/ui/dashboard-typography";
import {
  formatTraceOutcome,
  formatTypedWorkItemLabel,
} from "../../../components/ui/formatters";
import type {
  DashboardTraceDispatch,
  DashboardWorkItemRef,
} from "../../../api/dashboard/types";
import { getTraceDrilldownMessages } from "../messages/trace-drilldown";
import {
  getCachedTraceGraphLayout,
  layoutTraceGraphWithElk,
  traceGraphLayoutKey,
} from "../lib/trace-elk-layout";

// tailwind-exception: intrinsic-sizing
const GRAPH_SHELL_CLASS =
  "h-[36rem] min-h-[36rem] border-transparent bg-af-surface-subtle";
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
      new Map(graph.nodes.map((node) => [node.id, {
        height: DISPATCH_NODE_HEIGHT,
        id: node.id,
        width: DISPATCH_NODE_WIDTH,
      }])),
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
    setLayoutedNodes(getCachedTraceGraphLayout(layoutKey, graph.nodes) ?? graph.nodes);
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

  const handleNodesChange = useCallback((changes: NodeChange<WorkstationPathNode>[]) => {
    setNodes((currentNodes) => applyNodeChanges(changes, currentNodes));
  }, []);

  if (graph.nodes.length === 0) {
    return <span>{messages.dispatchPathEmpty}</span>;
  }

  return (
    <DashboardGraphFrame
      aria-label={messages.dispatchPathGraphLabel}
      className={GRAPH_SHELL_CLASS}
      data-trace-workstation-path
      style={{ overflowX: "hidden", overflowY: "hidden" }}
    >
      <ReactFlow
        defaultEdgeOptions={{
          animated: false,
          markerEnd: {
            color: "var(--color-af-edge-muted)",
            type: MarkerType.ArrowClosed,
          },
          style: { stroke: "var(--color-af-edge-muted)", strokeWidth: 1.7 },
          type: "smoothstep",
        }}
        edges={graph.edges}
        fitView
        fitViewOptions={TRACE_DISPATCH_FLOW_FIT_VIEW_OPTIONS}
        maxZoom={1.8}
        minZoom={0.35}
        nodes={nodes}
        nodesDraggable={true}
        nodeTypes={PATH_NODE_TYPES}
        onNodesChange={handleNodesChange}
        panOnDrag
        proOptions={{ hideAttribution: true }}
        zoomOnScroll
      >
        <DashboardGraphBackground />
        <DashboardGraphControls
          fitViewOptions={TRACE_DISPATCH_FLOW_FIT_VIEW_OPTIONS}
        />
      </ReactFlow>
    </DashboardGraphFrame>
  );
}

function WorkstationPathGraphNode({
  data,
}: NodeProps<WorkstationPathNode>) {
  const messages = getTraceDrilldownMessages(data.locale);

  return (
    <article className={cn(PATH_NODE_CLASS, outcomeToneClassName(data.outcome))}>
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
        className={cn("text-sm text-af-text [overflow-wrap:anywhere]", DASHBOARD_BODY_TEXT_CLASS)}
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
  const dispatchGraph = dispatchDependencyGraph(dispatches, locale);

  return {
    edges: dispatchGraph.edges,
    nodes: dispatchGraph.nodes.map((node, index) => ({
      data: {
        label: node.label,
        inputSummary: node.inputSummary,
        locale,
        outcome: node.outcome,
        outputSummary: node.outputSummary,
      },
      id: node.id,
      position: { x: index * (DISPATCH_NODE_WIDTH + 24), y: 0 },
      sourcePosition: Position.Right,
      targetPosition: Position.Left,
      type: "trace-workstation",
    })),
  };
}

function dispatchDependencyGraph(
  dispatches: DashboardTraceDispatch[],
  locale?: string,
): {
  edges: Edge[];
  nodes: Array<{
    id: string;
    inputSummary: string;
    label: string;
    outcome?: string;
    outputSummary: string;
  }>;
} {
  const nodes = dispatches.map((dispatch) => ({
    id: dispatch.dispatch_id,
    inputSummary: summarizeWorkItems(dispatch.input_items, locale),
    label: dispatch.workstation_name || dispatch.transition_id || "Unknown workstation",
    outcome: dispatch.outcome,
    outputSummary: summarizeWorkItems(dispatch.output_items, locale),
  }));
  const edgeKeys = new Set<string>();
  const latestDispatchIDByChainingTraceID = new Map<string, string>();

  for (let currentIndex = 0; currentIndex < dispatches.length; currentIndex += 1) {
    const currentDispatch = dispatches[currentIndex];
    const predecessorDispatchIDs =
      resolveExplicitPredecessorDispatchIDs(
        currentDispatch,
        latestDispatchIDByChainingTraceID,
      ) ??
      resolveWorkItemProducerDispatchIDs(dispatches, currentIndex) ??
      resolveSequentialPredecessorDispatchIDs(dispatches, currentIndex) ??
      [];

    for (const producerDispatchID of predecessorDispatchIDs) {
      if (producerDispatchID === currentDispatch.dispatch_id) {
        continue;
      }
      edgeKeys.add(`${producerDispatchID}->${currentDispatch.dispatch_id}`);
    }

    for (const chainingTraceID of collectCurrentChainingTraceIDs(currentDispatch)) {
      latestDispatchIDByChainingTraceID.set(chainingTraceID, currentDispatch.dispatch_id);
    }
  }

  const edges = [...edgeKeys].map((edgeKey) => {
    const [source, target] = edgeKey.split("->");

    return {
      id: edgeKey,
      source,
      target,
    };
  });

  return {
    edges,
    nodes,
  };
}

function resolveExplicitPredecessorDispatchIDs(
  dispatch: DashboardTraceDispatch,
  latestDispatchIDByChainingTraceID: Map<string, string>,
): string[] | null {
  const predecessorDispatchIDs = collectPreviousChainingTraceIDs(dispatch)
    .map((traceID) => latestDispatchIDByChainingTraceID.get(traceID))
    .filter((dispatchID): dispatchID is string => Boolean(dispatchID));

  return predecessorDispatchIDs.length > 0
    ? uniqueNonEmptyStrings(predecessorDispatchIDs)
    : null;
}

function resolveWorkItemProducerDispatchIDs(
  dispatches: DashboardTraceDispatch[],
  currentIndex: number,
): string[] | null {
  const currentDispatch = dispatches[currentIndex];
  const producerDispatchIDs = new Set<string>();

  for (const inputItem of currentDispatch.input_items ?? []) {
    for (let producerIndex = 0; producerIndex < currentIndex; producerIndex += 1) {
      const producerDispatch = dispatches[producerIndex];
      const matchingOutput = producerDispatch.output_items?.find(
        (outputItem) => outputItem.work_id === inputItem.work_id,
      );

      if (!matchingOutput) {
        continue;
      }

      producerDispatchIDs.add(producerDispatch.dispatch_id);
    }
  }

  return producerDispatchIDs.size > 0 ? [...producerDispatchIDs] : null;
}

function resolveSequentialPredecessorDispatchIDs(
  dispatches: DashboardTraceDispatch[],
  currentIndex: number,
): string[] | null {
  return currentIndex > 0 ? [dispatches[currentIndex - 1].dispatch_id] : null;
}

function collectCurrentChainingTraceIDs(dispatch: DashboardTraceDispatch): string[] {
  const chainingTraceIDs = [
    dispatch.current_chaining_trace_id,
    ...(dispatch.output_items ?? []).map((item) => item.current_chaining_trace_id),
  ];

  return uniqueNonEmptyStrings(chainingTraceIDs);
}

function collectPreviousChainingTraceIDs(dispatch: DashboardTraceDispatch): string[] {
  return uniqueNonEmptyStrings([
    ...(dispatch.previous_chaining_trace_ids ?? []),
    ...(dispatch.input_items ?? []).flatMap((item) => item.previous_chaining_trace_ids ?? []),
  ]);
}

function uniqueNonEmptyStrings(values: Array<string | undefined>): string[] {
  const seen = new Set<string>();

  for (const value of values) {
    const nextValue = value?.trim();
    if (!nextValue) {
      continue;
    }
    seen.add(nextValue);
  }

  return [...seen];
}

function summarizeWorkItems(
  workItems: DashboardWorkItemRef[] | undefined,
  locale?: string,
): string {
  if (!workItems || workItems.length === 0) {
    return getTraceDrilldownMessages(locale).noBatchRelations;
  }

  const labels = dedupeWorkItems(workItems).map(formatTypedWorkItemLabel);
  if (labels.length <= 2) {
    return labels.join(", ");
  }

  return `${labels.slice(0, 2).join(", ")} +${labels.length - 2}`;
}

function dedupeWorkItems(workItems: DashboardWorkItemRef[]): DashboardWorkItemRef[] {
  const itemsByID = new Map<string, DashboardWorkItemRef>();

  for (const workItem of workItems) {
    if (workItem.work_id) {
      itemsByID.set(workItem.work_id, workItem);
    }
  }

  return [...itemsByID.values()];
}

function outcomeToneClassName(outcome?: string): string {
  if (!outcome) {
    return "border-af-border bg-af-surface";
  }

  if (outcome.toUpperCase() === "FAILED" || outcome.toUpperCase() === "REJECTED") {
    return "border-af-danger-border bg-af-danger-surface";
  }

  return "border-af-success-border bg-af-success-surface";
}
