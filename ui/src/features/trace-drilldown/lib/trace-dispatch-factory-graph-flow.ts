import { type Node, Position } from "@xyflow/react";

import type { DashboardTraceDispatch } from "../../../api/dashboard/types";
import {
  type FactoryGraphReactFlowEdge,
  type FactoryGraphReactFlowNode,
  projectFactoryGraphToReactFlow,
} from "../../factory-graph-editor/lib/factory-graph-react-flow-projection";
import type { FactoryGraphTopology } from "../../factory-graph-editor/lib/factory-graph-draft-types";
import {
  projectTraceDispatchesToFactoryGraph,
  type TraceDispatchNodeOverlay,
} from "./trace-dispatch-factory-graph";

export type TraceDispatchFlowNodeData = FactoryGraphReactFlowNode["data"] &
  TraceDispatchNodeOverlay & {
    factoryNodeId: string;
    locale?: string;
  } &
  Record<string, unknown>;

export type TraceDispatchFlowNode = Node<
  TraceDispatchFlowNodeData,
  "factoryEntity"
>;

export interface TraceDispatchFactoryGraphFlow {
  dispatchIdByNodeId: ReadonlyMap<string, string>;
  edges: FactoryGraphReactFlowEdge[];
  nodes: TraceDispatchFlowNode[];
  topology: FactoryGraphTopology;
}

export function buildTraceDispatchFactoryGraphFlow(
  dispatches: DashboardTraceDispatch[],
  locale?: string,
): TraceDispatchFactoryGraphFlow {
  const traceProjection = projectTraceDispatchesToFactoryGraph(
    dispatches,
    locale,
  );
  const factoryProjection = projectFactoryGraphToReactFlow({
    locale,
    mode: "observer",
    topology: traceProjection.topology,
  });
  const nodes: TraceDispatchFlowNode[] = [];

  for (const node of factoryProjection.nodes) {
    const overlay = traceProjection.overlaysByNodeId.get(node.id);
    if (!overlay) {
      throw new Error(`Missing trace overlay for factory node ${node.id}.`);
    }

    const dispatchId = overlay.dispatchId;
    nodes.push({
      ...node,
      data: {
        ...node.data,
        ...overlay,
        factoryNodeId: node.id,
        locale,
      },
      id: dispatchId,
      sourcePosition: Position.Right,
      targetPosition: Position.Left,
      type: "factoryEntity",
    });
  }

  const edges = factoryProjection.edges.map((edge) => ({
    ...edge,
    source:
      traceProjection.dispatchIdByNodeId.get(edge.source) ?? edge.source,
    target:
      traceProjection.dispatchIdByNodeId.get(edge.target) ?? edge.target,
  }));

  return {
    dispatchIdByNodeId: traceProjection.dispatchIdByNodeId,
    edges,
    nodes,
    topology: traceProjection.topology,
  };
}
