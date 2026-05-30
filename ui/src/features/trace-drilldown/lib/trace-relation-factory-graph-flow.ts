import { MarkerType, type Node, Position } from "@xyflow/react";

import type { DashboardWorkRelation } from "../../../api/dashboard/types";
import type { FactoryGraphTopology } from "../../factory-graph-editor/lib/factory-graph-draft-types";
import {
  type FactoryGraphReactFlowEdge,
  type FactoryGraphReactFlowNode,
  projectFactoryGraphToReactFlow,
} from "../../factory-graph-editor/lib/factory-graph-react-flow-projection";
import {
  projectTraceRelationsToFactoryGraph,
  type TraceRelationEdgeOverlay,
  type TraceRelationNodeOverlay,
  type ProjectTraceRelationsToFactoryGraphOptions,
} from "./trace-relation-factory-graph";

const RELATION_NODE_WIDTH = 220;
const RELATION_NODE_HEIGHT = 112;

export type TraceRelationFlowNodeData = FactoryGraphReactFlowNode["data"] &
  TraceRelationNodeOverlay & {
    factoryNodeId: string;
    locale?: string;
    onSelectWorkID?: (workID: string) => void;
    selectable: boolean;
  } &
  Record<string, unknown>;

export type TraceRelationFlowNode = Node<
  TraceRelationFlowNodeData,
  "factoryEntity"
>;

export interface TraceRelationFactoryGraphFlow {
  edges: FactoryGraphReactFlowEdge[];
  endpointKeyByNodeId: ReadonlyMap<string, string>;
  graphDimensions: Map<string, { height: number; id: string; width: number }>;
  nodes: TraceRelationFlowNode[];
  topology: FactoryGraphTopology;
}

export function buildTraceRelationFactoryGraphFlow(
  relations: DashboardWorkRelation[],
  options: ProjectTraceRelationsToFactoryGraphOptions & {
    onSelectWorkID?: (workID: string) => void;
  } = {},
): TraceRelationFactoryGraphFlow {
  const { locale, onSelectWorkID, workItemsByWorkId } = options;
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
  const graphDimensions = new Map<
    string,
    { height: number; id: string; width: number }
  >();

  for (const node of factoryProjection.nodes) {
    const overlay = traceProjection.overlaysByNodeId.get(node.id);
    if (!overlay) {
      throw new Error(`Missing trace overlay for factory node ${node.id}.`);
    }

    const endpointKey = overlay.endpointKey;
    nodes.push({
      ...node,
      data: {
        ...node.data,
        ...overlay,
        factoryNodeId: node.id,
        locale,
        onSelectWorkID,
        selectable: Boolean(overlay.workID && onSelectWorkID),
      },
      id: endpointKey,
      sourcePosition: Position.Right,
      targetPosition: Position.Left,
      type: "factoryEntity",
    });
    graphDimensions.set(endpointKey, {
      height: RELATION_NODE_HEIGHT,
      id: endpointKey,
      width: RELATION_NODE_WIDTH,
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
      style: {
        ...edge.style,
        ...relationEdgeStyle(relationLike),
      },
      target:
        traceProjection.endpointKeyByNodeId.get(edge.target) ?? edge.target,
    };
  });

  return {
    edges,
    endpointKeyByNodeId: traceProjection.endpointKeyByNodeId,
    graphDimensions,
    nodes,
    topology: traceProjection.topology,
  };
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
      return "var(--color-af-danger-text)";
    }
    if (tone === "success") {
      return "var(--color-af-success)";
    }

    return "var(--color-af-warning-text)";
  }

  if (relation.type === "PARENT_CHILD") {
    return "var(--color-af-accent)";
  }

  return "var(--color-af-edge-muted)";
}

function relationEdgeStyle(relation: DashboardWorkRelation) {
  return {
    stroke: relationEdgeStroke(relation),
    strokeDasharray: relation.required_state ? "7 5" : undefined,
    strokeWidth: relation.required_state ? 2 : 1.7,
  };
}
