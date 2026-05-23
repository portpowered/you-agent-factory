import type { Edge, Node } from "@xyflow/react";
import ELK from "elkjs/lib/elk.bundled.js";
import type { ElkExtendedEdge, ElkNode } from "elkjs/lib/elk.bundled.js";

const elk = new ELK();
const TRACE_GRAPH_LAYOUT_CACHE = new Map<string, Map<string, { x: number; y: number }>>();
const TRACE_GRAPH_LAYOUT_PROMISE_CACHE = new Map<
  string,
  Promise<Map<string, { x: number; y: number }>>
>();

export interface TraceElkLayoutNode {
  height: number;
  id: string;
  width: number;
}

export interface TraceElkLayoutOptions {
  betweenLayerSpacing?: number;
  direction?: "DOWN" | "LEFT" | "RIGHT" | "UP";
  edgeNodeSpacing?: number;
  nodeNodeSpacing?: number;
}

function applyTraceGraphPositions<TNode extends Node>(
  nodes: TNode[],
  positionsByID: Map<string, { x: number; y: number }>,
): TNode[] {
  return nodes.map((node) => ({
    ...node,
    position: positionsByID.get(node.id) ?? node.position,
  }));
}

export function traceGraphLayoutKey(
  nodes: Node[],
  edges: Edge[],
  dimensions: Map<string, TraceElkLayoutNode>,
  options: TraceElkLayoutOptions = {},
): string {
  const nodeKey = [...nodes]
    .map((node) => {
      const dimension = dimensions.get(node.id);
      return [
        node.id,
        dimension?.width ?? 220,
        dimension?.height ?? 120,
      ].join(":");
    })
    .sort()
    .join("|");
  const edgeKey = [...edges]
    .map((edge) => [edge.id, edge.source, edge.target].join(":"))
    .sort()
    .join("|");

  return JSON.stringify({
    betweenLayerSpacing: options.betweenLayerSpacing,
    direction: options.direction ?? "RIGHT",
    edgeKey,
    edgeNodeSpacing: options.edgeNodeSpacing,
    nodeKey,
    nodeNodeSpacing: options.nodeNodeSpacing,
  });
}

export function getCachedTraceGraphLayout<TNode extends Node>(
  layoutKey: string,
  nodes: TNode[],
): TNode[] | null {
  const cachedPositions = TRACE_GRAPH_LAYOUT_CACHE.get(layoutKey);
  if (!cachedPositions) {
    return null;
  }

  return applyTraceGraphPositions(nodes, cachedPositions);
}

export async function layoutTraceGraphWithElk<TNode extends Node>(
  nodes: TNode[],
  edges: Edge[],
  dimensions: Map<string, TraceElkLayoutNode>,
  options: TraceElkLayoutOptions = {},
): Promise<TNode[]> {
  if (nodes.length === 0) {
    return nodes;
  }

  const layoutKey = traceGraphLayoutKey(nodes, edges, dimensions, options);
  const cachedPositions = TRACE_GRAPH_LAYOUT_CACHE.get(layoutKey);
  if (cachedPositions) {
    return applyTraceGraphPositions(nodes, cachedPositions);
  }

  const layoutGraph: ElkNode = {
    children: nodes.map((node) => {
      const dimensionsEntry = dimensions.get(node.id);
      return {
        height: dimensionsEntry?.height ?? 120,
        id: node.id,
        width: dimensionsEntry?.width ?? 220,
      };
    }),
    edges: edges.map(
      (edge): ElkExtendedEdge => ({
        id: edge.id,
        sources: [edge.source],
        targets: [edge.target],
      }),
    ),
    id: "trace-root",
    layoutOptions: {
      "elk.algorithm": "layered",
      "elk.direction": options.direction ?? "RIGHT",
      "elk.separateConnectedComponents": "true",
      ...(options.edgeNodeSpacing !== undefined
        ? {
            "elk.layered.spacing.edgeNodeBetweenLayers": `${options.edgeNodeSpacing}`,
          }
        : {}),
      ...(options.betweenLayerSpacing !== undefined
        ? {
            "elk.layered.spacing.nodeNodeBetweenLayers": `${options.betweenLayerSpacing}`,
          }
        : {}),
      ...(options.nodeNodeSpacing !== undefined
        ? {
            "elk.spacing.nodeNode": `${options.nodeNodeSpacing}`,
          }
        : {}),
    },
  };

  const layoutPromise =
    TRACE_GRAPH_LAYOUT_PROMISE_CACHE.get(layoutKey) ??
    elk.layout(layoutGraph)
      .then((layoutResult) => {
        const positionsByID = new Map(
          (layoutResult.children ?? []).map((child) => [
            child.id,
            { x: child.x ?? 0, y: child.y ?? 0 },
          ]),
        );

        TRACE_GRAPH_LAYOUT_CACHE.set(layoutKey, positionsByID);
        TRACE_GRAPH_LAYOUT_PROMISE_CACHE.delete(layoutKey);

        return positionsByID;
      })
      .catch((error: unknown) => {
        TRACE_GRAPH_LAYOUT_PROMISE_CACHE.delete(layoutKey);
        throw error;
      });

  TRACE_GRAPH_LAYOUT_PROMISE_CACHE.set(layoutKey, layoutPromise);

  const positionsByID = await layoutPromise;

  return applyTraceGraphPositions(nodes, positionsByID);
}

