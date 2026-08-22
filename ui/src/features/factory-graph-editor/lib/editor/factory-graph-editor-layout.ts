import {
  type FactoryGraphNodeDimensions,
  factoryGraphNodeFamilyRole,
  resolveFactoryGraphNodeDimensions,
} from "@you-agent-factory/factory-graph";
import {
  buildLayeredGraphLayout,
  type LayeredGraphLayoutNode,
} from "../../../flowchart/lib/layered-layout";
import type {
  FactoryGraphNodeKind,
  FactoryGraphTopology,
} from "../draft/factory-graph-draft-types";

/** Node bounds used by the factory graph editor layout and viewport placement resolver. */
export const FACTORY_GRAPH_EDITOR_NODE_DIMENSIONS_BY_KIND: Record<
  FactoryGraphNodeKind,
  FactoryGraphNodeDimensions
> = {
  doc: factoryGraphNodeFamilyRole("doc").defaultDimensions,
  resource: factoryGraphNodeFamilyRole("resource").defaultDimensions,
  worker: factoryGraphNodeFamilyRole("worker").defaultDimensions,
  "work-state": factoryGraphNodeFamilyRole("work-state").defaultDimensions,
  "work-type": factoryGraphNodeFamilyRole("work-type").defaultDimensions,
  workstation: factoryGraphNodeFamilyRole("workstation").defaultDimensions,
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
      ...resolveFactoryGraphNodeDimensions(node.kind, {
        content: [node.label],
      }).resolvedDimensions,
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
