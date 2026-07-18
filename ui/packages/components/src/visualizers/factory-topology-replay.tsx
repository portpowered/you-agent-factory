import {
  Background,
  Controls,
  type NodeProps,
  ReactFlow,
  ReactFlowProvider,
} from "@xyflow/react";
import { useEffect, useMemo, useRef } from "react";

import { GRAPH_EDGE_TYPES } from "../graphs/graph-edge";
import { GraphNodeButton } from "../graphs/graph-node-button";
import { GraphNodeShell } from "../graphs/graph-node-shell";
import { GraphViewportSurface } from "../graphs/graph-viewport-surface";
import { cn } from "../utilities/cn";
import {
  type FactoryTopologyFlowNode,
  graphHandles,
  projectFactoryTopologyToFlow,
} from "./factory-topology-replay-projection";
import type {
  FactoryTopologyReplayMessages,
  FactoryTopologyReplayProjection,
  FactoryVisualizerError,
} from "./factory-topology-replay-types";

export interface FactoryTopologyReplayProps {
  className?: string;
  formatNumber?: (value: number) => string;
  messages: FactoryTopologyReplayMessages;
  onError?: (error: FactoryVisualizerError) => void;
  onSelectNode?: (nodeId: string) => void;
  projection: FactoryTopologyReplayProjection;
  selectedNodeId?: string;
}

function FactoryTopologyNode({ data }: NodeProps<FactoryTopologyFlowNode>) {
  const {
    activeDispatchCount,
    formatNumber,
    messages,
    node,
    occupancy,
    onSelect,
    selected,
  } = data;
  const stateLabel = selected
    ? `${node.label}: ${messages.selectedNode}`
    : node.label;

  return (
    <div className="w-52 max-w-full" data-factory-topology-node-id={node.id}>
      <GraphNodeShell
        handles={graphHandles(node, messages)}
        nodeKind={node.kind}
        showStateIndicator={false}
        state={selected ? "selected" : "default"}
        stateLabel={stateLabel}
      >
        <GraphNodeButton
          aria-label={stateLabel}
          className="flex w-full flex-col gap-1"
          graphState={selected ? "selected" : "default"}
          onClick={onSelect ? () => onSelect(node.id) : undefined}
          stateLabel={stateLabel}
        >
          <span className="text-label-small text-on-surface-variant">
            {messages.nodeKind(node.kind)}
          </span>
          <span className="text-title-small font-semibold text-on-surface">
            {node.label}
          </span>
          {selected ? (
            <span className="text-label-small font-semibold text-primary">
              {messages.selectedNode}
            </span>
          ) : null}
          <NodeActivity
            activeDispatchCount={activeDispatchCount}
            formatNumber={formatNumber}
            messages={messages}
            nodeKind={node.kind}
            occupancy={occupancy}
            workStateCount={data.workStateCount}
          />
        </GraphNodeButton>
      </GraphNodeShell>
    </div>
  );
}

const FACTORY_TOPOLOGY_NODE_TYPES = { factoryTopology: FactoryTopologyNode };

export function FactoryTopologyReplay({
  className,
  formatNumber = String,
  messages,
  onError,
  onSelectNode,
  projection,
  selectedNodeId,
}: FactoryTopologyReplayProps) {
  const flow = useMemo(
    () =>
      projectFactoryTopologyToFlow({
        formatNumber,
        messages,
        onSelectNode,
        projection,
        selectedNodeId,
      }),
    [formatNumber, messages, onSelectNode, projection, selectedNodeId],
  );
  const error = "kind" in flow ? flow : undefined;
  const reportedError = useRef<string | undefined>(undefined);

  useEffect(() => {
    if (!error) {
      reportedError.current = undefined;
      return;
    }
    const signature = `${error.kind}:${error.message}`;
    if (reportedError.current === signature) return;
    reportedError.current = signature;
    onError?.(error);
  }, [error, onError]);

  if ("kind" in flow) {
    return (
      <GraphViewportSurface
        aria-label={messages.regionLabel}
        className={cn(
          "flex min-h-72 w-full items-center justify-center p-6",
          className,
        )}
        data-factory-topology-state="failed"
      >
        <div className="max-w-lg text-center" role="alert">
          <h3 className="text-title-medium font-semibold text-on-surface">
            {messages.failedTitle}
          </h3>
          <p className="mt-2 text-body-medium text-on-surface-variant">
            {messages.failedDescription}
          </p>
        </div>
      </GraphViewportSurface>
    );
  }

  return (
    <ReactFlowProvider>
      <GraphViewportSurface
        aria-label={messages.regionLabel}
        className={cn("h-96 min-h-72 w-full", className)}
        data-factory-topology-state="ready"
      >
        <p className="absolute left-3 top-3 z-10 rounded-lg bg-surface px-3 py-1 text-label-small text-on-surface shadow-sm">
          {messages.selectedTick(
            formatNumber(projection.topology.selectedTick),
          )}
        </p>
        <ReactFlow
          defaultEdgeOptions={{ selectable: false }}
          edgeTypes={GRAPH_EDGE_TYPES}
          edges={flow.edges}
          fitView
          fitViewOptions={{ padding: 0.2 }}
          maxZoom={1.5}
          minZoom={0.2}
          nodeTypes={FACTORY_TOPOLOGY_NODE_TYPES}
          nodes={flow.nodes}
          nodesConnectable={false}
          nodesDraggable={false}
          panOnDrag
          proOptions={{ hideAttribution: true }}
          zoomOnScroll
        >
          <Background gap={16} size={1} />
          <Controls showInteractive={false} />
        </ReactFlow>
      </GraphViewportSurface>
    </ReactFlowProvider>
  );
}

function NodeActivity({
  activeDispatchCount,
  formatNumber,
  messages,
  nodeKind,
  occupancy,
  workStateCount,
}: {
  activeDispatchCount: number;
  formatNumber: (value: number) => string;
  messages: FactoryTopologyReplayMessages;
  nodeKind: FactoryTopologyFlowNode["data"]["node"]["kind"];
  occupancy: FactoryTopologyFlowNode["data"]["occupancy"];
  workStateCount: number | undefined;
}) {
  if (nodeKind === "workstation") {
    return (
      <span
        className={cn(
          "mt-1 rounded-md border px-2 py-1 text-label-small font-semibold",
          activeDispatchCount > 0
            ? "border-primary bg-primary-container text-on-primary-container"
            : "border-dashed border-outline-variant text-on-surface-variant",
        )}
        data-active-dispatch={activeDispatchCount > 0 ? "true" : "false"}
      >
        {activeDispatchCount > 0
          ? messages.activeDispatchCount(activeDispatchCount)
          : messages.inactiveDispatch}
      </span>
    );
  }
  if (nodeKind === "resource" && occupancy) {
    return (
      <span className="mt-1 text-label-small text-on-surface-variant">
        {occupancy.evidence === "known"
          ? messages.occupancy(
              formatNumber(occupancy.occupiedQuantity ?? 0),
              formatNumber(occupancy.capacity),
            )
          : messages.occupancyUnavailable}
      </span>
    );
  }
  if (nodeKind === "work-state" && workStateCount !== undefined) {
    return (
      <span className="mt-1 text-label-small font-semibold text-on-surface-variant">
        {messages.workStateCount(formatNumber(workStateCount))}
      </span>
    );
  }
  return null;
}
