import type { FactoryGraphTopology } from "../../factory-graph-editor/lib/factory-graph-draft-types";
import { buildFactoryGraphEditorLayout } from "../../factory-graph-editor/lib/factory-graph-editor-layout";

export type TraceFactoryGraphLayoutPosition = {
  x: number;
  y: number;
  width: number;
  height: number;
};

/** Applies ELK layout bounds to a trace React Flow node (work-graph node shell contract). */
export function applyTraceFactoryGraphLayoutToNode<
  TNode extends { id: string; position: { x: number; y: number } },
>(
  node: TNode,
  layoutByNodeId: ReadonlyMap<string, TraceFactoryGraphLayoutPosition>,
): TNode {
  const layout = layoutByNodeId.get(node.id);
  if (!layout) {
    return node;
  }

  return {
    ...node,
    position: { x: layout.x, y: layout.y },
    width: layout.width,
    height: layout.height,
    initialWidth: layout.width,
    initialHeight: layout.height,
    measured: { width: layout.width, height: layout.height },
  };
}

/**
 * Positions trace React Flow nodes using the shared factory graph editor layout.
 * Layout runs on factory topology node ids and remaps coordinates through
 * `reactFlowIdByFactoryNodeId` (e.g. dispatch id or relation endpoint key).
 */
export async function buildTraceFactoryGraphLayoutPositions(
  topology: FactoryGraphTopology,
  reactFlowIdByFactoryNodeId: ReadonlyMap<string, string>,
): Promise<Map<string, TraceFactoryGraphLayoutPosition>> {
  if (topology.nodes.length === 0) {
    return new Map();
  }

  const layout = await buildFactoryGraphEditorLayout(topology);
  const positionsByTraceFlowNodeId = new Map<
    string,
    TraceFactoryGraphLayoutPosition
  >();

  for (const layoutNode of layout.nodes) {
    const traceFlowNodeId = reactFlowIdByFactoryNodeId.get(layoutNode.nodeId);
    if (!traceFlowNodeId) {
      continue;
    }

    positionsByTraceFlowNodeId.set(traceFlowNodeId, {
      x: layoutNode.x,
      y: layoutNode.y,
      width: layoutNode.width,
      height: layoutNode.height,
    });
  }

  return positionsByTraceFlowNodeId;
}
