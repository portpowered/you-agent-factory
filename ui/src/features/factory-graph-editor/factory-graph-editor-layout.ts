import {
  buildLayeredGraphLayout,
  type LayeredGraphLayoutNode,
} from "../flowchart/layered-layout";
import type {
  FactoryGraphNodeKind,
  FactoryGraphTopology,
} from "./factory-graph-draft-types";

const NODE_DIMENSIONS_BY_KIND: Record<
  FactoryGraphNodeKind,
  { height: number; width: number }
> = {
  resource: { height: 86, width: 168 },
  worker: { height: 86, width: 164 },
  "work-state": { height: 86, width: 164 },
  "work-type": { height: 58, width: 156 },
  workstation: { height: 196, width: 156 },
};

export interface FactoryGraphEditorLayoutNode
  extends LayeredGraphLayoutNode<FactoryGraphNodeKind> {}

export interface FactoryGraphEditorLayout {
  height: number;
  nodes: FactoryGraphEditorLayoutNode[];
  width: number;
}

export async function buildFactoryGraphEditorLayout(
  topology: FactoryGraphTopology,
): Promise<FactoryGraphEditorLayout> {
  const layout = await buildLayeredGraphLayout({
    edges: topology.edges.map((edge) => ({
      edgeId: edge.id,
      fromNodeId: edge.sourceId,
      id: edge.id,
      label: "",
      sources: [edge.sourceId],
      targets: [edge.targetId],
      toNodeId: edge.targetId,
    })),
    nodes: topology.nodes.map((node) => ({
      ...NODE_DIMENSIONS_BY_KIND[node.kind],
      id: node.id,
      nodeId: node.id,
      nodeKind: node.kind,
    })),
  });

  return {
    height: layout.height,
    nodes: layout.nodes.map((node) => ({
      column: node.column,
      height: node.height,
      nodeId: node.nodeId,
      nodeKind: node.nodeKind,
      row: node.row,
      width: node.width,
      x: node.x,
      y: node.y,
    })),
    width: layout.width,
  };
}
