import { buildFactoryGraphEditorLayout } from "./factory-graph-editor-layout";
import type { FactoryGraphTopology } from "./factory-graph-draft-types";
import {
  type FactoryLayout,
  resolveProjectedLayoutPositions,
} from "./factory-graph-layout-operations";
import { projectFactoryGraphToReactFlow } from "./factory-graph-react-flow-projection";

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
