import ELK from "elkjs/lib/elk.bundled.js";
import type { ElkExtendedEdge, ElkNode, LayoutOptions } from "elkjs/lib/elk.bundled.js";

export interface LayeredGraphLayoutNode<TNodeKind extends string = string> {
  column: number;
  height: number;
  nodeId: string;
  nodeKind: TNodeKind;
  row: number;
  width: number;
  x: number;
  y: number;
}

interface LayeredGraphSeedNode<TNodeKind extends string = string> extends ElkNode {
  height: number;
  id: string;
  nodeId: string;
  nodeKind: TNodeKind;
  width: number;
}

interface LayeredGraphSeedEdge extends ElkExtendedEdge {
  edgeId: string;
  fromNodeId: string;
  label: string;
  toNodeId: string;
}

const PADDING_X = 40;
const PADDING_Y = 36;
const LAYER_SPACING = 56;
const NODE_SPACING = 40;

const elk = new ELK();

const ELK_LAYOUT_OPTIONS: LayoutOptions = {
  "elk.algorithm": "layered",
  "elk.direction": "RIGHT",
  "elk.layered.crossingMinimization.strategy": "LAYER_SWEEP",
  "elk.layered.nodePlacement.strategy": "NETWORK_SIMPLEX",
  "elk.layered.spacing.nodeNodeBetweenLayers": `${LAYER_SPACING}`,
  "elk.spacing.nodeNode": `${NODE_SPACING}`,
};

function buildOrdinalAssignments(
  nodes: Array<{
    nodeId: string;
    x?: number;
    y?: number;
  }>,
  coordinate: "x" | "y",
): Map<string, number> {
  const sortedCoordinates = [
    ...new Set(nodes.map((node) => Math.round(node[coordinate] ?? 0))),
  ].sort((left, right) => left - right);

  return new Map(
    nodes.map((node) => [
      node.nodeId,
      sortedCoordinates.indexOf(Math.round(node[coordinate] ?? 0)),
    ]),
  );
}

export async function buildLayeredGraphLayout<
  TNode extends LayeredGraphSeedNode,
  TEdge extends LayeredGraphSeedEdge,
>(seeds: {
  edges: TEdge[];
  nodes: TNode[];
}): Promise<{
  edges: TEdge[];
  height: number;
  nodes: Array<TNode & LayeredGraphLayoutNode<TNode["nodeKind"]>>;
  width: number;
}> {
  if (seeds.nodes.length === 0) {
    return { edges: [], height: 0, nodes: [], width: 0 };
  }

  const graph: ElkNode = {
    children: seeds.nodes,
    edges: seeds.edges,
    id: "root",
    layoutOptions: ELK_LAYOUT_OPTIONS,
  };
  const layoutedGraph = await elk.layout(graph);
  const layoutedNodes = (layoutedGraph.children ?? []) as TNode[];
  const minX = Math.min(...layoutedNodes.map((node) => node.x ?? 0));
  const minY = Math.min(...layoutedNodes.map((node) => node.y ?? 0));
  const columnAssignments = buildOrdinalAssignments(layoutedNodes, "x");
  const rowAssignments = buildOrdinalAssignments(layoutedNodes, "y");
  const positionedNodes = layoutedNodes
    .map((node) => ({
      ...node,
      column: columnAssignments.get(node.nodeId) ?? 0,
      row: rowAssignments.get(node.nodeId) ?? 0,
      x: (node.x ?? 0) - minX + PADDING_X,
      y: (node.y ?? 0) - minY + PADDING_Y,
    }))
    .sort((left, right) => left.column - right.column || left.row - right.row);
  const rightmostX = Math.max(...positionedNodes.map((node) => node.x + node.width));
  const bottomY = Math.max(...positionedNodes.map((node) => node.y + node.height));

  return {
    edges: ((layoutedGraph.edges ?? []) as TEdge[]).map((edge) => ({
      ...edge,
    })),
    height: bottomY + PADDING_Y,
    nodes: positionedNodes,
    width: rightmostX + PADDING_X,
  };
}
