import type { Node } from "@xyflow/react";
import {
  type FactoryGraphNodeDimensions,
  resolveFactoryGraphNodeDimensions,
} from "@you-agent-factory/factory-graph";
import { buildDocTargetPathFromFileName } from "../../current-factory-definition/lib/doc-editable-values";
import {
  type FactoryGraphNodeKind,
  nodeKeyId,
} from "../../factory-graph-editor/lib/draft/factory-graph-draft-types";
import type { FactoryGraphAddEntityDraft } from "../../factory-graph-editor/lib/editor/factory-graph-editor-additions";
import { factoryGraphDocNodeIdForTargetPath } from "../../factory-graph-editor/lib/factory-graph-doc-editor";
import {
  type AxisAlignedRect,
  axisAlignedRectFromTopLeft,
  type FlowPoint,
  resolveViewportCenterNodePlacement,
  topLeftFromAxisAlignedRectCenter,
} from "./graph-editor-node-placement";
import type { GraphNodePosition } from "./layout/graph-node-positions";

export interface GraphEditorAddNodePlacementViewport {
  height: number;
  viewport: {
    x: number;
    y: number;
    zoom: number;
  };
  width: number;
}

export function factoryGraphNodeIdForAddEntityDraft(
  draft: FactoryGraphAddEntityDraft,
): string {
  if (draft.kind === "doc") {
    return factoryGraphDocNodeIdForTargetPath(
      buildDocTargetPathFromFileName(draft.fileName.trim()),
    );
  }

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
): FactoryGraphNodeKind | "doc" {
  return draft.kind;
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

    occupiedRects.push(axisAlignedRectFromTopLeft(node.position, size));
  }

  return occupiedRects;
}

function placementContentForDraft(
  draft: FactoryGraphAddEntityDraft,
): readonly string[] {
  if (draft.kind === "doc") {
    return [buildDocTargetPathFromFileName(draft.fileName.trim())];
  }

  if (draft.kind === "work-state") {
    return [`${draft.workTypeName.trim()}:${draft.name.trim()}`];
  }

  return [draft.name.trim()];
}

function placementSizeForDraft(
  draft: FactoryGraphAddEntityDraft,
): FactoryGraphNodeDimensions {
  return resolveFactoryGraphNodeDimensions(
    factoryGraphNodeKindForAddEntityDraft(draft),
    { content: placementContentForDraft(draft) },
  ).resolvedDimensions;
}

export function viewportCenterFromPlacementViewport(
  placementViewport: GraphEditorAddNodePlacementViewport,
): FlowPoint {
  const zoom =
    Number.isFinite(placementViewport.viewport.zoom) &&
    placementViewport.viewport.zoom > 0
      ? placementViewport.viewport.zoom
      : 1;

  return {
    x: (placementViewport.width / 2 - placementViewport.viewport.x) / zoom,
    y: (placementViewport.height / 2 - placementViewport.viewport.y) / zoom,
  };
}

export function resolveInitialPlacementTopLeft(input: {
  draft: FactoryGraphAddEntityDraft;
  nodes: readonly Node[];
  viewportBounds?: AxisAlignedRect;
  viewportCenter: FlowPoint;
}): GraphNodePosition | null {
  const nodeId = factoryGraphNodeIdForAddEntityDraft(input.draft);
  const candidateSize = placementSizeForDraft(input.draft);
  const placement = resolveViewportCenterNodePlacement({
    candidateSize,
    occupiedRects: occupiedRectsFromRenderedNodes(input.nodes, nodeId),
    viewportBounds: input.viewportBounds,
    viewportCenter: input.viewportCenter,
  });

  return topLeftFromAxisAlignedRectCenter(placement.center, candidateSize);
}

export function resolveInitialPlacementTopLeftForViewport(input: {
  draft: FactoryGraphAddEntityDraft;
  nodes: readonly Node[];
  placementViewport: GraphEditorAddNodePlacementViewport;
}): GraphNodePosition | null {
  const zoom =
    Number.isFinite(input.placementViewport.viewport.zoom) &&
    input.placementViewport.viewport.zoom > 0
      ? input.placementViewport.viewport.zoom
      : 1;

  return resolveInitialPlacementTopLeft({
    draft: input.draft,
    nodes: input.nodes,
    viewportBounds: {
      height: input.placementViewport.height / zoom,
      width: input.placementViewport.width / zoom,
      x: -input.placementViewport.viewport.x / zoom,
      y: -input.placementViewport.viewport.y / zoom,
    },
    viewportCenter: viewportCenterFromPlacementViewport(
      input.placementViewport,
    ),
  });
}
