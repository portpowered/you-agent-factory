import {
  type FactoryGraphNodeDimensions,
  type FactoryGraphNodeFamily,
  type FactoryGraphNodeSizingContent,
  fitFactoryGraphNodeDimensions,
  resolveFactoryGraphNodeResizeDimensions,
} from "@you-agent-factory/factory-graph";
import type { components } from "../../../../api/generated/openapi";
import type { CanonicalFactoryDefinition } from "../draft/factory-graph-draft-types";

export type FactoryLayout = NonNullable<
  components["schemas"]["Factory"]["layout"]
>;
export type FactoryLayoutPoint = FactoryLayout["nodes"] extends
  | (infer TNode)[]
  | undefined
  ? TNode extends { position: infer Position }
    ? Position
    : { x: number; y: number }
  : { x: number; y: number };
export type FactoryLayoutViewport = NonNullable<FactoryLayout["viewport"]>;
export type FactoryLayoutNode = NonNullable<FactoryLayout["nodes"]>[number];
export type FactoryLayoutNodeSize = NonNullable<FactoryLayoutNode["size"]>;

export const FACTORY_LAYOUT_SCHEMA_VERSION = 1;

export function createDefaultFactoryLayout(): FactoryLayout {
  return {
    schemaVersion: FACTORY_LAYOUT_SCHEMA_VERSION,
  };
}

export function factoryLayoutFromDefinition(
  factoryDefinition: CanonicalFactoryDefinition | null | undefined,
): FactoryLayout {
  if (!factoryDefinition?.layout) {
    return createDefaultFactoryLayout();
  }

  return structuredClone(factoryDefinition.layout);
}

export function factoryLayoutNodePosition(
  layout: FactoryLayout,
  nodeId: string,
): FactoryLayoutPoint | undefined {
  const node = layout.nodes?.find((entry) => entry.id === nodeId);
  if (!node) {
    return undefined;
  }

  const { x, y } = node.position;
  if (!Number.isFinite(x) || !Number.isFinite(y)) {
    return undefined;
  }

  return { x, y };
}

export function factoryLayoutNodeSize(
  layout: FactoryLayout,
  nodeId: string,
): FactoryLayoutNodeSize | undefined {
  const node = layout.nodes?.find((entry) => entry.id === nodeId);
  if (!node?.size) {
    return undefined;
  }

  return {
    height: node.size.height,
    width: node.size.width,
  };
}

/** Set one node's authored size while preserving every other layout field. */
export function setFactoryLayoutNodeSize(
  layout: FactoryLayout,
  nodeId: string,
  size: FactoryLayoutNodeSize,
  position?: FactoryLayoutPoint,
): FactoryLayout {
  const nodes = [...(layout.nodes ?? [])];
  const existingIndex = nodes.findIndex((entry) => entry.id === nodeId);
  const existingNode = existingIndex >= 0 ? nodes[existingIndex] : undefined;
  if (!existingNode && !isFiniteFactoryLayoutPoint(position)) {
    return layout;
  }

  const nextNode: FactoryLayoutNode = existingNode
    ? {
        ...existingNode,
        size: { height: size.height, width: size.width },
      }
    : {
        id: nodeId,
        position: { x: position?.x ?? 0, y: position?.y ?? 0 },
        size: { height: size.height, width: size.width },
      };

  if (existingIndex >= 0) {
    nodes[existingIndex] = nextNode;
  } else {
    nodes.push(nextNode);
  }

  return {
    ...layout,
    nodes,
    schemaVersion: layout.schemaVersion ?? FACTORY_LAYOUT_SCHEMA_VERSION,
  };
}

/** Remove one node's authored size and leave its position and metadata intact. */
export function resetFactoryLayoutNodeSize(
  layout: FactoryLayout,
  nodeId: string,
): FactoryLayout {
  const existingIndex = (layout.nodes ?? []).findIndex(
    (entry) => entry.id === nodeId,
  );
  if (existingIndex < 0 || layout.nodes?.[existingIndex]?.size === undefined) {
    return layout;
  }

  const nodes = [...(layout.nodes ?? [])];
  const { size: _size, ...nodeWithoutSize } = nodes[existingIndex];
  nodes[existingIndex] = nodeWithoutSize;
  return { ...layout, nodes };
}

/** Normalize an interactive resize request through the package family policy. */
export function resizeFactoryLayoutNode(
  layout: FactoryLayout,
  nodeId: string,
  family: FactoryGraphNodeFamily,
  requestedDimensions: FactoryGraphNodeDimensions,
  position?: FactoryLayoutPoint,
): FactoryLayout {
  return setFactoryLayoutNodeSize(
    layout,
    nodeId,
    resolveFactoryGraphNodeResizeDimensions(family, requestedDimensions),
    position,
  );
}

/** Store deterministic fitted dimensions as the node's authored override. */
export function fitFactoryLayoutNode(
  layout: FactoryLayout,
  nodeId: string,
  family: FactoryGraphNodeFamily,
  content: FactoryGraphNodeSizingContent,
  position?: FactoryLayoutPoint,
): FactoryLayout {
  return setFactoryLayoutNodeSize(
    layout,
    nodeId,
    fitFactoryGraphNodeDimensions(family, content),
    position,
  );
}

function isFiniteFactoryLayoutPoint(
  point: FactoryLayoutPoint | undefined,
): point is FactoryLayoutPoint {
  return (
    point !== undefined && Number.isFinite(point.x) && Number.isFinite(point.y)
  );
}

export function moveFactoryLayoutNode(
  layout: FactoryLayout,
  nodeId: string,
  position: FactoryLayoutPoint,
): FactoryLayout {
  const nodes = [...(layout.nodes ?? [])];
  const existingIndex = nodes.findIndex((entry) => entry.id === nodeId);
  const nextNode = {
    ...(existingIndex >= 0 ? nodes[existingIndex] : { id: nodeId }),
    id: nodeId,
    position: {
      x: position.x,
      y: position.y,
    },
  };

  if (existingIndex >= 0) {
    nodes[existingIndex] = nextNode;
  } else {
    nodes.push(nextNode);
  }

  return {
    ...layout,
    nodes,
    schemaVersion: layout.schemaVersion ?? FACTORY_LAYOUT_SCHEMA_VERSION,
  };
}

export function updateFactoryLayoutViewport(
  layout: FactoryLayout,
  viewport: FactoryLayoutViewport,
): FactoryLayout {
  return {
    ...layout,
    schemaVersion: layout.schemaVersion ?? FACTORY_LAYOUT_SCHEMA_VERSION,
    viewport: {
      x: viewport.x,
      y: viewport.y,
      zoom: viewport.zoom,
    },
  };
}

export function moveFactoryLayoutNodesByDelta(
  layout: FactoryLayout,
  nodeIds: readonly string[],
  delta: FactoryLayoutPoint,
  resolvedPositionsByNodeId: ReadonlyMap<string, FactoryLayoutPoint>,
): FactoryLayout {
  let nextLayout = layout;
  for (const nodeId of nodeIds) {
    const currentPosition =
      factoryLayoutNodePosition(nextLayout, nodeId) ??
      resolvedPositionsByNodeId.get(nodeId);
    if (!currentPosition) {
      continue;
    }

    nextLayout = moveFactoryLayoutNode(nextLayout, nodeId, {
      x: currentPosition.x + delta.x,
      y: currentPosition.y + delta.y,
    });
  }

  return nextLayout;
}

export function factoryLayoutPositionsByNodeId(
  layout: FactoryLayout,
): Map<string, FactoryLayoutPoint> {
  const positions = new Map<string, FactoryLayoutPoint>();
  for (const node of layout.nodes ?? []) {
    const position = factoryLayoutNodePosition(layout, node.id);
    if (position) {
      positions.set(node.id, position);
    }
  }
  return positions;
}

export function resolveProjectedLayoutPositions(input: {
  autoLayoutPositionsByNodeId: ReadonlyMap<string, FactoryLayoutPoint>;
  canonicalLayout: FactoryLayout;
  nodeIds: readonly string[];
}): Map<string, FactoryLayoutPoint> {
  const canonicalPositions = factoryLayoutPositionsByNodeId(
    input.canonicalLayout,
  );
  const projected = new Map<string, FactoryLayoutPoint>();

  for (const nodeId of input.nodeIds) {
    const canonicalPosition = canonicalPositions.get(nodeId);
    if (canonicalPosition) {
      projected.set(nodeId, canonicalPosition);
      continue;
    }

    const autoLayoutPosition = input.autoLayoutPositionsByNodeId.get(nodeId);
    if (autoLayoutPosition) {
      projected.set(nodeId, autoLayoutPosition);
    }
  }

  return projected;
}

export function hasFactoryLayoutChanges(
  baseLayout: FactoryLayout,
  pendingLayout: FactoryLayout,
): boolean {
  return (
    JSON.stringify(normalizeFactoryLayoutForComparison(baseLayout)) !==
    JSON.stringify(normalizeFactoryLayoutForComparison(pendingLayout))
  );
}

export function applyPendingFactoryLayout(
  factoryDefinition: CanonicalFactoryDefinition,
  pendingLayout: FactoryLayout,
): CanonicalFactoryDefinition {
  return {
    ...factoryDefinition,
    layout: structuredClone(pendingLayout),
  };
}

function normalizeFactoryLayoutForComparison(layout: FactoryLayout) {
  return {
    edges: layout.edges ?? [],
    groups: layout.groups ?? [],
    nodes: [...(layout.nodes ?? [])].sort((left, right) =>
      left.id.localeCompare(right.id),
    ),
    preferences: layout.preferences ?? null,
    schemaVersion: layout.schemaVersion ?? FACTORY_LAYOUT_SCHEMA_VERSION,
    viewport: layout.viewport ?? null,
  };
}
