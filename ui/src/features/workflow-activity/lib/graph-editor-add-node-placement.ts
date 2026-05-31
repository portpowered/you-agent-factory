import type { Node, ReactFlowInstance } from "@xyflow/react";
import {
  type FactoryGraphNodeKind,
  nodeKeyId,
} from "../../factory-graph-editor/lib/factory-graph-draft-types";
import type { FactoryGraphAddEntityDraft } from "../../factory-graph-editor/lib/factory-graph-editor-additions";
import type { GraphNodePosition } from "../state/currentActivityGraphStore";
import {
  axisAlignedRectFromTopLeft,
  type FlowPoint,
  graphEditorNodeDimensionsForKind,
  resolveViewportCenterNodePlacement,
  topLeftFromAxisAlignedRectCenter,
} from "./graph-editor-node-placement";

export function factoryGraphNodeIdForAddEntityDraft(
  draft: FactoryGraphAddEntityDraft,
): string {
  const name = draft.name.trim();
  switch (draft.kind) {
    case "resource":
      return nodeKeyId({ kind: "resource", name });
    case "worker":
      return nodeKeyId({ kind: "worker", name });
    case "work-type":
      return nodeKeyId({ kind: "work-type", name });
    case "work-state":
      return nodeKeyId({
        kind: "work-state",
        stateName: name,
        workTypeName: draft.workTypeName.trim(),
      });
    case "workstation":
      return nodeKeyId({ kind: "workstation", name });
  }
}

export function factoryGraphNodeKindForAddEntityDraft(
  draft: FactoryGraphAddEntityDraft,
): FactoryGraphNodeKind {
  return draft.kind;
}

function finiteStoredPosition(
  position: GraphNodePosition | undefined,
): position is GraphNodePosition {
  return (
    position !== undefined &&
    Number.isFinite(position.x) &&
    Number.isFinite(position.y)
  );
}

function nodeKindFromRenderedNode(node: Node): FactoryGraphNodeKind | null {
  const kind = (node.data as { kind?: FactoryGraphNodeKind } | undefined)?.kind;
  return kind ?? null;
}

function renderedNodeSize(
  node: Node,
): { height: number; width: number } | null {
  const width = node.width ?? node.measured?.width;
  const height = node.height ?? node.measured?.height;
  if (
    width === undefined ||
    height === undefined ||
    !Number.isFinite(width) ||
    !Number.isFinite(height)
  ) {
    return null;
  }

  return { height, width };
}

export function occupiedRectsFromRenderedNodes(
  nodes: readonly Node[],
  excludeNodeId?: string,
) {
  const occupiedRects = [];

  for (const node of nodes) {
    if (excludeNodeId && node.id === excludeNodeId) {
      continue;
    }

    const size = renderedNodeSize(node);
    if (!size) {
      continue;
    }

    const kind = nodeKindFromRenderedNode(node);
    const dimensions = kind
      ? graphEditorNodeDimensionsForKind(kind)
      : { height: size.height, width: size.width };

    occupiedRects.push(
      axisAlignedRectFromTopLeft(node.position, {
        height: dimensions.height,
        width: dimensions.width,
      }),
    );
  }

  return occupiedRects;
}

export function viewportCenterInFlowCoordinates(
  instance: ReactFlowInstance,
  container: HTMLElement,
): FlowPoint {
  const bounds = container.getBoundingClientRect();
  return instance.screenToFlowPosition({
    x: bounds.left + bounds.width / 2,
    y: bounds.top + bounds.height / 2,
  });
}

export function resolveInitialPlacementTopLeft(input: {
  draft: FactoryGraphAddEntityDraft;
  nodes: readonly Node[];
  storedPositions: Record<string, GraphNodePosition | undefined>;
  viewportCenter: FlowPoint;
}): GraphNodePosition | null {
  const nodeId = factoryGraphNodeIdForAddEntityDraft(input.draft);
  if (finiteStoredPosition(input.storedPositions[nodeId])) {
    return null;
  }

  const candidateSize = graphEditorNodeDimensionsForKind(
    factoryGraphNodeKindForAddEntityDraft(input.draft),
  );
  const placement = resolveViewportCenterNodePlacement({
    candidateSize,
    occupiedRects: occupiedRectsFromRenderedNodes(input.nodes, nodeId),
    viewportCenter: input.viewportCenter,
  });

  return topLeftFromAxisAlignedRectCenter(placement.center, candidateSize);
}
