import { resolveFactoryGraphNodeDimensions } from "@you-agent-factory/factory-graph";
import type {
  FactoryGraphNode,
  FactoryGraphTopology,
} from "../draft/factory-graph-draft-types";
import type { FactoryLayout } from "./factory-graph-layout-operations";

/** Normalize authored node sizes before they enter the persisted Factory layout. */
export function normalizeFactoryLayoutNodeSizesForTopology(
  layout: FactoryLayout,
  topology: FactoryGraphTopology,
): FactoryLayout {
  const topologyNodesById = new Map(
    topology.nodes.map((node) => [node.id, node]),
  );
  let didChange = false;
  const nodes = (layout.nodes ?? []).map((node) => {
    if (node.size === undefined) {
      return node;
    }

    const topologyNode = topologyNodesById.get(node.id);
    if (!topologyNode) {
      if (isFinitePositiveLayoutNodeSize(node.size)) {
        return node;
      }

      didChange = true;
      const { size: _size, ...nodeWithoutSize } = node;
      return nodeWithoutSize;
    }

    const normalizedSize = resolveFactoryGraphNodeDimensions(
      topologyNode.kind,
      {
        authoredDimensions: node.size,
        content: sizingContentForFactoryGraphNode(topologyNode),
      },
    ).resolvedDimensions;
    if (
      normalizedSize.width === node.size.width &&
      normalizedSize.height === node.size.height
    ) {
      return node;
    }

    didChange = true;
    return {
      ...node,
      size: normalizedSize,
    };
  });

  return didChange ? { ...layout, nodes } : layout;
}

function sizingContentForFactoryGraphNode(
  node: FactoryGraphNode,
): readonly string[] {
  if (node.key.kind === "doc") {
    return [node.label, node.key.name];
  }
  if (node.key.kind === "work-state") {
    return [node.label, node.key.workTypeName, node.key.stateName];
  }
  return [node.label];
}

function isFinitePositiveLayoutNodeSize(size: {
  height: number;
  width: number;
}): boolean {
  return (
    Number.isFinite(size.width) &&
    Number.isFinite(size.height) &&
    size.width > 0 &&
    size.height > 0
  );
}
