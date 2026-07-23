import type { FactoryGraphTopology } from "../draft/factory-graph-draft-types";
import { buildFactoryGraphEditorLayout } from "../editor/factory-graph-editor-layout";
import { projectFactoryGraphToReactFlow } from "../projection/factory-graph-react-flow-projection";
import {
  type FactoryLayout,
  resolveProjectedLayoutPositions,
} from "./factory-graph-layout-operations";

export async function projectFactoryGraphWithCanonicalLayout(input: {
  autoLayoutTopology?: FactoryGraphTopology;
  canonicalLayout: FactoryLayout;
  topology: FactoryGraphTopology;
}) {
  const autoLayout = await buildFactoryGraphEditorLayout(
    input.autoLayoutTopology ?? input.topology,
  );
  const autoLayoutPositions = new Map(
    autoLayout.nodes.map((node) => [node.nodeId, { x: node.x, y: node.y }]),
  );
  const layoutPositionsByNodeId = resolveProjectedLayoutPositions({
    autoLayoutPositionsByNodeId: autoLayoutPositions,
    canonicalLayout: input.canonicalLayout,
    nodeIds: input.topology.nodes.map((node) => node.id),
  });

  return {
    layoutPositionsByNodeId,
    projection: projectFactoryGraphToReactFlow({
      layoutPositionsByNodeId,
      topology: input.topology,
    }),
  };
}
