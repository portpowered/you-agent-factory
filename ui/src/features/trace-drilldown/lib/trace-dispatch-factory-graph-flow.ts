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
  type TraceDispatchFactoryGraphProjection,
  type TraceDispatchNodeOverlay,
} from "./trace-dispatch-factory-graph";
import type { TraceRelationPathEntry } from "./trace-relation-path";
import {
  type TraceSelectionIdentity,
  traceSelectionKey,
  traceSelectionMatches,
} from "./trace-selection";

export type TraceDispatchFlowNodeData = FactoryGraphWorkstationNodeData &
  TraceDispatchNodeOverlay & {
    factoryNodeId: string;
    locale?: string;
    onSelectTraceSelection?: (selection: TraceSelectionIdentity) => void;
    selectionIdentities: readonly TraceSelectionIdentity[];
    traceSelectionKeys: readonly string[];
  } & Record<string, unknown>;

export type TraceDispatchFlowNode = Node<
  TraceDispatchFlowNodeData,
  "workstation"
>;

export interface TraceDispatchFactoryGraphFlow {
  dispatchIdByNodeId: ReadonlyMap<string, string>;
  edges: FactoryGraphReactFlowEdge[];
  lineageStatus: "resolved" | "unresolved";
  nodes: TraceDispatchFlowNode[];
  relations: readonly TraceRelationPathEntry[];
  selectionIdentitiesByNodeId: ReadonlyMap<
    string,
    readonly TraceSelectionIdentity[]
  >;
  topology: FactoryGraphTopology;
}

export function buildTraceDispatchFactoryGraphFlow(
  dispatches: DashboardTraceDispatch[],
  localeOrOptions:
    | string
    | {
        locale?: string;
        onSelectTraceSelection?: (selection: TraceSelectionIdentity) => void;
        selectedTraceSelection?: TraceSelectionIdentity | null;
      } = {},
): TraceDispatchFactoryGraphFlow {
  const { locale, onSelectTraceSelection, selectedTraceSelection } =
    normalizeTraceDispatchGraphOptions(localeOrOptions);
  const traceProjection = projectTraceDispatchesToFactoryGraph(
    dispatches,
    locale,
  );
  const factoryProjection = projectFactoryGraphToReactFlow({
    locale,
    mode: "observer",
    topology: traceProjection.topology,
  });
  const nodes = factoryProjection.nodes.map((node) =>
    buildTraceDispatchFlowNode(
      node,
      traceProjection,
      locale,
      onSelectTraceSelection,
      selectedTraceSelection,
    ),
  );

  const nodesByID = new Map(nodes.map((node) => [node.id, node]));

  const edges = factoryProjection.edges.map((edge) => {
    const source =
      traceProjection.traceNodeIdByFactoryNodeId.get(edge.source) ??
      edge.source;
    const target =
      traceProjection.traceNodeIdByFactoryNodeId.get(edge.target) ??
      edge.target;
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

  const selectionIdentitiesByNodeId = new Map(
    factoryProjection.nodes.flatMap((node) => {
      const flowNodeID = traceProjection.traceNodeIdByFactoryNodeId.get(
        node.id,
      );
      const identities = traceProjection.selectionIdentitiesByNodeId.get(
        node.id,
      );
      return flowNodeID && identities
        ? [[flowNodeID, identities] as const]
        : [];
    }),
  );

  return {
    dispatchIdByNodeId: traceProjection.traceNodeIdByFactoryNodeId,
    edges,
    lineageStatus: traceProjection.lineageStatus,
    nodes,
    relations: traceProjection.relations,
    selectionIdentitiesByNodeId,
    topology: traceProjection.topology,
  };
}

function buildTraceDispatchFlowNode(
  node: ReturnType<typeof projectFactoryGraphToReactFlow>["nodes"][number],
  traceProjection: TraceDispatchFactoryGraphProjection,
  locale: string | undefined,
  onSelectTraceSelection:
    | ((selection: TraceSelectionIdentity) => void)
    | undefined,
  selectedTraceSelection: TraceSelectionIdentity | null | undefined,
): TraceDispatchFlowNode {
  const overlay = traceProjection.overlaysByNodeId.get(node.id);
  if (!overlay) {
    throw new Error(`Missing trace overlay for factory node ${node.id}.`);
  }

  const flowNodeID =
    traceProjection.traceNodeIdByFactoryNodeId.get(node.id) ??
    overlay.dispatchId;
  const selectionIdentities =
    traceProjection.selectionIdentitiesByNodeId.get(node.id) ?? [];
  const selectedSelection = selectionIdentities.find((selection) =>
    traceSelectionMatches(selection, selectedTraceSelection),
  );
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
      onSelectTraceSelection,
      runtimeStatus: overlay.outcome,
      selectedWorkID: selectedSelection?.work_id || null,
      selectedWorkstation: Boolean(selectedSelection),
      selectionIdentities,
      traceSelectionKeys: selectionIdentities.map(traceSelectionKey),
      summaryOnly: true,
      workstation: traceDispatchWorkstationNode(overlay),
      workstationSemantics: node.data.workstationSemantics,
      ...(onSelectTraceSelection && selectionIdentities[0]
        ? {
            onSelectWorkstation: () =>
              onSelectTraceSelection(selectionIdentities[0]),
          }
        : {}),
    },
    id: flowNodeID,
    sourcePosition: Position.Right,
    targetPosition: Position.Left,
    type: "workstation",
  } satisfies TraceDispatchFlowNode;
}

function normalizeTraceDispatchGraphOptions(
  localeOrOptions:
    | string
    | {
        locale?: string;
        onSelectTraceSelection?: (selection: TraceSelectionIdentity) => void;
        selectedTraceSelection?: TraceSelectionIdentity | null;
      },
): {
  locale?: string;
  onSelectTraceSelection?: (selection: TraceSelectionIdentity) => void;
  selectedTraceSelection?: TraceSelectionIdentity | null;
} {
  return typeof localeOrOptions === "string"
    ? { locale: localeOrOptions }
    : localeOrOptions;
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
