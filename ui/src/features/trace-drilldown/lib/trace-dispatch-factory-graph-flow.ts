import { type Node, Position } from "@xyflow/react";
import type {
  FactoryGraphNodeHandle,
  FactoryGraphWorkstationNodeData,
} from "@you-agent-factory/factory-graph";
import type { DashboardTraceDispatch } from "../../../api/dashboard/types";
import type { FactoryGraphTopology } from "../../factory-graph-editor/lib/draft/factory-graph-draft-types";
import {
  type FactoryGraphReactFlowEdge,
  projectFactoryGraphToReactFlow,
} from "../../factory-graph-editor/lib/projection/factory-graph-react-flow-projection";
import {
  projectTraceDispatchesToFactoryGraph,
  type TraceDispatchNodeOverlay,
} from "./trace-dispatch-factory-graph";

export type TraceDispatchFlowNodeData = FactoryGraphWorkstationNodeData &
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
  const nodes = factoryProjection.nodes.map((node) => {
    const overlay = traceProjection.overlaysByNodeId.get(node.id);
    if (!overlay) {
      throw new Error(`Missing trace overlay for factory node ${node.id}.`);
    }

    const dispatchId = overlay.dispatchId;
    const handles = node.data.handles.map((handle) => ({
      ...handle,
      hidden: true,
    }));
    return {
      ...node,
      data: {
        ...overlay,
        active: false,
        activeFlow: false,
        executions: [],
        factoryNodeId: node.id,
        handles,
        kind: "workstation",
        locale,
        muted: false,
        now: 0,
        runtimeStatus: overlay.outcome,
        selectedWorkID: null,
        selectedWorkstation: false,
        summaryOnly: true,
        workstation: traceDispatchWorkstationNode(overlay),
        workstationSemantics: node.data.workstationSemantics,
      },
      id: dispatchId,
      sourcePosition: Position.Right,
      targetPosition: Position.Left,
      type: "workstation",
    } satisfies TraceDispatchFlowNode;
  });

  const nodesByID = new Map(nodes.map((node) => [node.id, node]));

  const edges = factoryProjection.edges.map((edge) => {
    const source =
      traceProjection.dispatchIdByNodeId.get(edge.source) ?? edge.source;
    const target =
      traceProjection.dispatchIdByNodeId.get(edge.target) ?? edge.target;
    const sourceNode = nodesByID.get(source);
    const targetNode = nodesByID.get(target);

    return {
      ...edge,
      data:
        edge.data?.kind === "workstation-on-continue"
          ? {
              ...edge.data,
              label: "",
            }
          : edge.data,
      source,
      sourceHandle: resolveTraceEdgeHandle(
        edge.sourceHandle,
        sourceNode?.data.handles,
        "source",
      ),
      target,
      targetHandle: resolveTraceEdgeHandle(
        edge.targetHandle,
        targetNode?.data.handles,
        "target",
      ),
    };
  });

  return {
    dispatchIdByNodeId: traceProjection.dispatchIdByNodeId,
    edges,
    nodes,
    topology: traceProjection.topology,
  };
}

function resolveTraceEdgeHandle(
  requestedHandle: string | null | undefined,
  handles: readonly FactoryGraphNodeHandle[] | undefined,
  role: FactoryGraphNodeHandle["type"],
): string | undefined {
  return (
    handles?.find((handle) => handle.id === requestedHandle)?.id ??
    handles?.find((handle) => handle.type === role)?.id
  );
}

function traceDispatchWorkstationNode(
  overlay: TraceDispatchNodeOverlay,
): FactoryGraphWorkstationNodeData["workstation"] {
  return {
    node_id: overlay.dispatchId,
    transition_id: overlay.displayLabel,
    workstation_name: overlay.displayLabel,
  };
}
