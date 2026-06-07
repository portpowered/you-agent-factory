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
  type TraceRelationEdgeOverlay,
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
  "factoryEntity"
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
  const nodes: TraceRelationFlowNode[] = [];

  for (const node of factoryProjection.nodes) {
    const overlay = traceProjection.overlaysByNodeId.get(node.id);
    if (!overlay) {
      throw new Error(`Missing trace overlay for factory node ${node.id}.`);
    }

    const endpointKey = overlay.endpointKey;
    const isSelectedWork = Boolean(
      selectedWorkID && overlay.workID === selectedWorkID,
    );
    nodes.push({
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
        selectable: Boolean(overlay.workID && onSelectWorkID && !isSelectedWork),
      },
      id: endpointKey,
      sourcePosition: Position.Right,
      targetPosition: Position.Left,
      type: "factoryEntity",
    });
  }

  const edges = factoryProjection.edges.map((edge) => {
    const overlay = traceProjection.edgeOverlaysByEdgeId.get(edge.id);
    const relationLike = relationFromEdgeOverlay(
      edge.source,
      edge.target,
      overlay,
      traceProjection.endpointKeyByNodeId,
    );

    return {
      ...edge,
      ariaLabel: overlay?.ariaLabel ?? edge.ariaLabel,
      markerEnd: {
        color: relationEdgeStroke(relationLike),
        type: MarkerType.ArrowClosed,
      },
      source:
        traceProjection.endpointKeyByNodeId.get(edge.source) ?? edge.source,
      sourceHandle: edge.sourceHandle ?? TRACE_RELATION_SOURCE_HANDLE_ID,
      style: {
        ...edge.style,
        ...relationEdgeStyle(relationLike),
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

function relationFromEdgeOverlay(
  sourceId: string,
  targetId: string,
  overlay: TraceRelationEdgeOverlay | undefined,
  endpointKeyByNodeId: ReadonlyMap<string, string>,
): DashboardWorkRelation {
  return {
    request_id: overlay?.requestId,
    required_state: overlay?.requiredState,
    source_work_id: endpointKeyByNodeId.get(sourceId) ?? sourceId,
    target_work_id: endpointKeyByNodeId.get(targetId) ?? targetId,
    type: overlay?.relationType ?? "RELATED_TO",
  };
}

function relationStateToneClassName(relationState: string): string {
  const normalizedState = relationState.trim().toUpperCase();
  if (
    normalizedState === "FAILED" ||
    normalizedState === "FAIL" ||
    normalizedState === "REJECTED"
  ) {
    return "danger";
  }

  if (
    normalizedState === "DONE" ||
    normalizedState === "ACCEPTED" ||
    normalizedState === "COMPLETED"
  ) {
    return "success";
  }

  return "warning";
}

function relationEdgeStroke(relation: DashboardWorkRelation): string {
  if (relation.required_state) {
    const tone = relationStateToneClassName(relation.required_state);
    if (tone === "danger") {
      return "var(--color-on-error-container)";
    }
    if (tone === "success") {
      return "var(--color-success)";
    }

    return "var(--color-on-warning-container)";
  }

  if (relation.type === "PARENT_CHILD") {
    return "var(--color-primary)";
  }

  return "var(--color-outline-variant)";
}

function relationEdgeStyle(relation: DashboardWorkRelation) {
  return {
    stroke: relationEdgeStroke(relation),
    strokeDasharray: relation.required_state ? "7 5" : undefined,
    strokeWidth: relation.required_state ? 2 : 1.7,
  };
}
