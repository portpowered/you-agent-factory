import { type Node, Position } from "@xyflow/react";
import type {
  DashboardTraceDispatch,
  DashboardWorkstationNode,
} from "../../../api/dashboard/types";
import type { FactoryGraphTopology } from "../../factory-graph-editor/lib/draft/factory-graph-draft-types";
import {
  type FactoryGraphReactFlowEdge,
  projectFactoryGraphToReactFlow,
} from "../../factory-graph-editor/lib/projection/factory-graph-react-flow-projection";
import { STANDARD_WORKSTATION_KIND } from "../../flowchart/lib/workstation-icon-metadata";
import type { WorkstationNodeData } from "../../graphs/components/workstation-node-view";
import {
  projectTraceDispatchesToFactoryGraph,
  type TraceDispatchNodeOverlay,
} from "./trace-dispatch-factory-graph";

export type TraceDispatchFlowNodeData = WorkstationNodeData &
  TraceDispatchNodeOverlay & {
    factoryNodeId: string;
    locale?: string;
  } & Record<string, unknown>;

export type TraceDispatchFlowNode = Node<
  TraceDispatchFlowNodeData,
  "workstation"
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
        ...overlay,
        active: false,
        activeFlow: false,
        executions: [],
        factoryNodeId: node.id,
        handles: node.data.connectionAnchors.map((handle) => ({
          ...handle,
          hidden: true,
        })),
        kind: "workstation",
        locale,
        muted: false,
        now: 0,
        selectedWorkID: null,
        selectedWorkstation: false,
        summaryOnly: true,
        workstation: traceDispatchWorkstationNode(overlay),
      },
      id: dispatchId,
      sourcePosition: Position.Right,
      targetPosition: Position.Left,
      type: "workstation",
    });
  }

  const edges = factoryProjection.edges.map((edge) => ({
    ...edge,
    data:
      edge.data?.kind === "workstation-on-continue"
        ? {
            ...edge.data,
            label: "",
          }
        : edge.data,
    source: traceProjection.dispatchIdByNodeId.get(edge.source) ?? edge.source,
    target: traceProjection.dispatchIdByNodeId.get(edge.target) ?? edge.target,
  }));

  return {
    dispatchIdByNodeId: traceProjection.dispatchIdByNodeId,
    edges,
    nodes,
    topology: traceProjection.topology,
  };
}

function traceDispatchWorkstationNode(
  overlay: TraceDispatchNodeOverlay,
): DashboardWorkstationNode {
  return {
    node_id: overlay.dispatchId,
    transition_id: overlay.displayLabel,
    workstation_kind: STANDARD_WORKSTATION_KIND,
    workstation_name: overlay.displayLabel,
  };
}
