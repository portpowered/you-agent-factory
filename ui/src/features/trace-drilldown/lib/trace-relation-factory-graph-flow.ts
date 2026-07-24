import { MarkerType, type Node, Position } from "@xyflow/react";

import type { DashboardWorkRelation } from "../../../api/dashboard/types";
import type { FactoryGraphTopology } from "../../factory-graph-editor/lib/draft/factory-graph-draft-types";
import {
  type FactoryGraphReactFlowEdge,
  type FactoryGraphReactFlowNode,
  projectFactoryGraphToReactFlow,
} from "../../factory-graph-editor/lib/projection/factory-graph-react-flow-projection";
import {
  type ProjectTraceRelationsToFactoryGraphOptions,
  projectTraceRelationsToFactoryGraph,
  type TraceRelationNodeOverlay,
} from "./trace-relation-factory-graph";

const TRACE_RELATION_SOURCE_HANDLE_ID = "trace-relation-source";
const TRACE_RELATION_TARGET_HANDLE_ID = "trace-relation-target";

export type TraceRelationFlowNodeData = FactoryGraphReactFlowNode["data"] &
  TraceRelationNodeOverlay & {
    factoryNodeId: string;
    isSelectedWork: boolean;
    locale?: string;
    onSelectWorkID?: (workID: string) => void;
    selectable: boolean;
  } & Record<string, unknown>;

export type TraceRelationFlowNode = Node<
  TraceRelationFlowNodeData,
  "workRelation"
>;

export interface TraceRelationFactoryGraphFlow {
  edges: FactoryGraphReactFlowEdge[];
  endpointKeyByNodeId: ReadonlyMap<string, string>;
  nodes: TraceRelationFlowNode[];
  topology: FactoryGraphTopology;
}

export function buildTraceRelationFactoryGraphFlow(
  relations: DashboardWorkRelation[],
  options: ProjectTraceRelationsToFactoryGraphOptions & {
    onSelectWorkID?: (workID: string) => void;
    selectedWorkID?: string | null;
  } = {},
): TraceRelationFactoryGraphFlow {
  const { locale, onSelectWorkID, selectedWorkID, workItemsByWorkId } = options;
  const traceProjection = projectTraceRelationsToFactoryGraph(relations, {
    locale,
    workItemsByWorkId,
  });
  const factoryProjection = projectFactoryGraphToReactFlow({
    locale,
    mode: "observer",
    topology: traceProjection.topology,
  });
  const nodesByEndpointKey = new Map<string, TraceRelationFlowNode>();

  for (const node of factoryProjection.nodes) {
    const overlay = traceProjection.overlaysByNodeId.get(node.id);
    if (!overlay) {
      throw new Error(`Missing trace overlay for factory node ${node.id}.`);
    }

    const endpointKey = overlay.endpointKey;
    const isSelectedWork = Boolean(
      selectedWorkID && overlay.workID === selectedWorkID,
    );
    const nextNode: TraceRelationFlowNode = {
      ...node,
      data: {
        ...node.data,
        ...overlay,
        connectionAnchors: traceRelationConnectionAnchors(
          node.data.connectionAnchors,
        ),
        factoryNodeId: node.id,
        isSelectedWork,
        locale,
        onSelectWorkID,
        selectable: Boolean(
          overlay.workID && onSelectWorkID && !isSelectedWork,
        ),
      },
      id: endpointKey,
      sourcePosition: Position.Right,
      targetPosition: Position.Left,
      type: "workRelation",
    };
    const existingNode = nodesByEndpointKey.get(endpointKey);
    if (existingNode) {
      nodesByEndpointKey.set(
        endpointKey,
        mergeTraceRelationFlowNodes(existingNode, nextNode),
      );
      continue;
    }

    nodesByEndpointKey.set(endpointKey, nextNode);
  }

  const nodes = [...nodesByEndpointKey.values()];

  const edges = factoryProjection.edges.map((edge) => {
    const overlay = traceProjection.edgeOverlaysByEdgeId.get(edge.id);

    return {
      ...edge,
      ariaLabel: overlay?.ariaLabel ?? edge.ariaLabel,
      data: {
        active: edge.data?.active ?? false,
        alwaysShowLabel: edge.data?.alwaysShowLabel ?? false,
        kind: edge.data?.kind ?? "work-type-state",
        label: "",
        pendingStatus: edge.data?.pendingStatus ?? "none",
      },
      markerEnd: {
        color: "var(--color-outline-variant)",
        type: MarkerType.ArrowClosed,
      },
      source:
        traceProjection.endpointKeyByNodeId.get(edge.source) ?? edge.source,
      sourceHandle: edge.sourceHandle ?? TRACE_RELATION_SOURCE_HANDLE_ID,
      style: {
        stroke: "var(--color-outline-variant)",
        strokeWidth: 1.7,
      },
      target:
        traceProjection.endpointKeyByNodeId.get(edge.target) ?? edge.target,
      targetHandle: edge.targetHandle ?? TRACE_RELATION_TARGET_HANDLE_ID,
    };
  });

  return {
    edges,
    endpointKeyByNodeId: traceProjection.endpointKeyByNodeId,
    nodes,
    topology: traceProjection.topology,
  };
}

function mergeTraceRelationFlowNodes(
  left: TraceRelationFlowNode,
  right: TraceRelationFlowNode,
): TraceRelationFlowNode {
  return {
    ...left,
    data: {
      ...left.data,
      ...right.data,
      relationStates: [
        ...new Set([...left.data.relationStates, ...right.data.relationStates]),
      ].sort(),
      relationTypes: [
        ...new Set([...left.data.relationTypes, ...right.data.relationTypes]),
      ].sort(),
      isSelectedWork: left.data.isSelectedWork || right.data.isSelectedWork,
      selectable: left.data.selectable || right.data.selectable,
    },
  };
}

function traceRelationConnectionAnchors(
  anchors: FactoryGraphReactFlowNode["data"]["connectionAnchors"],
): FactoryGraphReactFlowNode["data"]["connectionAnchors"] {
  const hasSource = anchors.some(
    (anchor) => anchor.id === TRACE_RELATION_SOURCE_HANDLE_ID,
  );
  const hasTarget = anchors.some(
    (anchor) => anchor.id === TRACE_RELATION_TARGET_HANDLE_ID,
  );

  return [
    ...anchors,
    ...(hasTarget
      ? []
      : [
          {
            id: TRACE_RELATION_TARGET_HANDLE_ID,
            label: TRACE_RELATION_TARGET_HANDLE_ID,
            side: "left" as const,
            type: "target" as const,
          },
        ]),
    ...(hasSource
      ? []
      : [
          {
            id: TRACE_RELATION_SOURCE_HANDLE_ID,
            label: TRACE_RELATION_SOURCE_HANDLE_ID,
            side: "right" as const,
            type: "source" as const,
          },
        ]),
  ];
}
