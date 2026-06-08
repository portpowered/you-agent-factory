import { Background, ReactFlow } from "@xyflow/react";
import { expect, waitFor, within } from "storybook/test";

import "../../../styles.css";
import {
  buildGridAutoLayoutPositionsByNodeId,
  factoryGraphLargeEditorFixtures,
} from "../lib/fixtures/factory-graph-large-editor-fixtures";
import { resolveProjectedLayoutPositions } from "../lib/layout/factory-graph-layout-operations";
import {
  buildFactoryGraphEditorFlowModel,
  FACTORY_GRAPH_EDITOR_EDGE_TYPES,
  FACTORY_GRAPH_EDITOR_NODE_TYPES,
} from "./flow/factory-graph-editor-flow";

const fiveHundredNodeFixture = factoryGraphLargeEditorFixtures.fiveHundred;
const fiveHundredNodeNodeIds = fiveHundredNodeFixture.topology.nodes.map(
  (node) => node.id,
);
const fiveHundredNodeFlowModel = buildFactoryGraphEditorFlowModel({
  canEditConnections: false,
  factoryDefinition: fiveHundredNodeFixture.factoryDefinition,
  layoutPositionsByNodeId: resolveProjectedLayoutPositions({
    autoLayoutPositionsByNodeId: buildGridAutoLayoutPositionsByNodeId(
      fiveHundredNodeNodeIds,
    ),
    canonicalLayout: fiveHundredNodeFixture.layout,
    nodeIds: fiveHundredNodeNodeIds,
  }),
  pendingAdditionEdgeIds: new Set<string>(),
  pendingConnectionSource: null,
  pendingAdditionNodeIds: new Set<string>(),
  pendingRemovalEdgeIds: new Set<string>(),
  pendingRemovalNodeIds: new Set<string>(),
  topology: fiveHundredNodeFixture.topology,
});

function FiveHundredNodeEditorFixtureStory() {
  return (
    <div
      className="relative h-[640px] w-full rounded-[1.5rem] border border-outline bg-surface-container-high p-4"
      data-factory-graph-editor-canvas="true"
      data-large-fixture-viewport-ready="true"
    >
      <ReactFlow
        defaultEdgeOptions={{ selectable: false }}
        defaultViewport={{ x: 0, y: 0, zoom: 1 }}
        edgeTypes={FACTORY_GRAPH_EDITOR_EDGE_TYPES}
        edges={fiveHundredNodeFlowModel.edges}
        fitView={false}
        nodeTypes={FACTORY_GRAPH_EDITOR_NODE_TYPES}
        nodes={fiveHundredNodeFlowModel.nodes}
        nodesDraggable={false}
        onlyRenderVisibleElements={false}
      >
        <Background />
      </ReactFlow>
    </div>
  );
}

export default {
  title: "Factory Graph Editor/Large Fixtures",
  tags: ["test"],
};

export const FiveHundredNodeCanonicalProjection = {
  render: () => <FiveHundredNodeEditorFixtureStory />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);

    await waitFor(
      () => {
        expect(
          canvasElement.querySelector(
            '[data-large-fixture-viewport-ready="true"]',
          ),
        ).toBeTruthy();
      },
      { timeout: 30_000 },
    );
    await expect(
      canvas.findByTitle("ws-0", undefined, { timeout: 30_000 }),
    ).resolves.toBeInTheDocument();
    await expect(canvas.getByTitle("processor")).toBeInTheDocument();
    expect(
      canvasElement.querySelectorAll(".react-flow__node").length,
    ).toBeGreaterThanOrEqual(fiveHundredNodeFixture.graphNodeCount);
  },
};
