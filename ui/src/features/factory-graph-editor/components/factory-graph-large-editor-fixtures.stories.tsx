import { Background, Controls, ReactFlow } from "@xyflow/react";
import { FactoryGraphGroupRegionLayer } from "@you-agent-factory/factory-graph";
import { useMemo, useState } from "react";
import { expect, userEvent, waitFor, within } from "storybook/test";

import "../../../styles.css";
import {
  buildGridAutoLayoutPositionsByNodeId,
  buildLargeFactoryEditorParityFixture,
  factoryGraphLargeEditorFixtures,
} from "../lib/fixtures/factory-graph-large-editor-fixtures";
import { resolveProjectedLayoutPositions } from "../lib/layout/factory-graph-layout-operations";
import {
  buildFactoryGraphEditorFlowModel,
  FACTORY_GRAPH_EDITOR_EDGE_TYPES,
  FACTORY_GRAPH_EDITOR_NODE_TYPES,
} from "./flow/factory-graph-editor-flow";

const fiveHundredNodeFixture = factoryGraphLargeEditorFixtures.fiveHundred;
const largeParityFixture = buildLargeFactoryEditorParityFixture(
  fiveHundredNodeFixture,
);
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

const largeParityNodeIds = largeParityFixture.fixture.topology.nodes.map(
  (node) => node.id,
);
const largeParityLayoutPositions = resolveProjectedLayoutPositions({
  autoLayoutPositionsByNodeId:
    buildGridAutoLayoutPositionsByNodeId(largeParityNodeIds),
  canonicalLayout: largeParityFixture.layout,
  nodeIds: largeParityNodeIds,
});
const largeParityWorkStateCounts = [1, 3, 4, 25] as const;

function LargeFactoryVisualParityStory() {
  const [liveOverlay, setLiveOverlay] = useState(false);
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null);
  const [draggedNodePositions, setDraggedNodePositions] = useState<
    ReadonlyMap<string, { x: number; y: number }>
  >(new Map());
  const layoutPositionsByNodeId = useMemo(() => {
    const positions = new Map(largeParityLayoutPositions);
    for (const [nodeId, position] of draggedNodePositions) {
      positions.set(nodeId, position);
    }
    return positions;
  }, [draggedNodePositions]);
  const workStateCounts = useMemo(
    () =>
      new Map(
        largeParityFixture.workStateNodeIds
          .slice(0, largeParityWorkStateCounts.length)
          .map((nodeId, index) => [
            nodeId,
            liveOverlay ? (largeParityWorkStateCounts[index] ?? 0) : 0,
          ]),
      ),
    [liveOverlay],
  );
  const activeNodeIds = useMemo(
    () =>
      liveOverlay
        ? new Set(largeParityFixture.workStateNodeIds.slice(0, 2))
        : new Set<string>(),
    [liveOverlay],
  );
  const flowModel = useMemo(
    () =>
      buildFactoryGraphEditorFlowModel({
        activeNodeIds,
        canEditConnections: false,
        factoryDefinition: largeParityFixture.fixture.factoryDefinition,
        focusedNodeIds: activeNodeIds,
        layout: largeParityFixture.layout,
        layoutPositionsByNodeId,
        mutedNodeIds: new Set<string>(),
        pendingAdditionEdgeIds: new Set<string>(),
        pendingConnectionSource: null,
        pendingAdditionNodeIds: new Set<string>(),
        pendingRemovalEdgeIds: new Set<string>(),
        pendingRemovalNodeIds: new Set<string>(),
        placeTokenCountsByNodeId: workStateCounts,
        topology: largeParityFixture.fixture.topology,
      }),
    [activeNodeIds, layoutPositionsByNodeId, workStateCounts],
  );
  const displayNodes = useMemo(
    () =>
      flowModel.nodes.map((node) => ({
        ...node,
        selected: node.id === selectedNodeId,
      })),
    [flowModel.nodes, selectedNodeId],
  );

  return (
    <div
      className="grid gap-3 rounded-[1.5rem] border border-outline bg-surface-container-high p-4"
      data-large-parity-overlay={liveOverlay ? "live" : "idle"}
    >
      <button
        aria-pressed={liveOverlay}
        className="w-fit rounded-lg border border-outline bg-surface px-3 py-2 text-sm font-semibold text-on-surface"
        onClick={() => setLiveOverlay((current) => !current)}
        type="button"
      >
        {liveOverlay ? "Reset live graph overlay" : "Apply live graph overlay"}
      </button>
      <div
        className="relative h-[640px] w-full overflow-hidden rounded-[1.5rem] border border-outline bg-surface-container-high"
        data-large-parity-viewport="true"
      >
        <ReactFlow
          defaultEdgeOptions={{ selectable: false }}
          defaultViewport={{ x: 24, y: 24, zoom: 0.35 }}
          edgeTypes={FACTORY_GRAPH_EDITOR_EDGE_TYPES}
          edges={flowModel.edges}
          fitView={false}
          nodeTypes={FACTORY_GRAPH_EDITOR_NODE_TYPES}
          nodes={displayNodes}
          nodesDraggable
          onNodeClick={(_event, node) => setSelectedNodeId(node.id)}
          onNodeDragStop={(_event, node) => {
            setDraggedNodePositions((current) => {
              const next = new Map(current);
              next.set(node.id, {
                x: node.position.x,
                y: node.position.y,
              });
              return next;
            });
          }}
          onlyRenderVisibleElements={false}
        >
          <FactoryGraphGroupRegionLayer groups={largeParityFixture.groups} />
          <Background />
          <Controls showInteractive={false} />
        </ReactFlow>
      </div>
    </div>
  );
}

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

export const LargeFactoryVisualParityMatrix = {
  render: () => <LargeFactoryVisualParityStory />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const applyOverlay = canvas.getByRole("button", {
      name: "Apply live graph overlay",
    });
    const authoredNodeId = largeParityFixture.authoredSizeByNodeId
      .keys()
      .next().value;
    if (typeof authoredNodeId !== "string") {
      throw new Error("Expected a large parity fixture authored-size node.");
    }
    const authoredNodeSelector = `.react-flow__node[data-id="${authoredNodeId}"]`;
    await waitFor(
      () => {
        expect(canvasElement.querySelector(authoredNodeSelector)).toBeTruthy();
      },
      { timeout: 30_000 },
    );
    const authoredNode =
      canvasElement.querySelector<HTMLElement>(authoredNodeSelector);
    if (!authoredNode) {
      throw new Error(`Expected authored-size node ${authoredNodeId}.`);
    }
    const initialSize = {
      height: authoredNode.style.height,
      width: authoredNode.style.width,
    };

    await waitFor(
      () => {
        expect(
          canvasElement.querySelectorAll("[data-factory-graph-group-region]"),
        ).toHaveLength(3);
      },
      { timeout: 30_000 },
    );
    await expect(applyOverlay).toHaveAttribute("aria-pressed", "false");
    await userEvent.click(applyOverlay);
    await expect(applyOverlay).toHaveAttribute("aria-pressed", "true");
    await expect(
      canvasElement.querySelectorAll('[data-state-work-progress="numeric"]'),
    ).toHaveLength(2);
    await expect(
      canvasElement.querySelectorAll("[data-state-work-progress-dot]"),
    ).toHaveLength(4);
    await expect(
      canvasElement.querySelector('[data-graph-visual-active-flow="true"]'),
    ).toBeInTheDocument();

    const largeCountNode = canvasElement.querySelector<HTMLElement>(
      `.react-flow__node[data-id="${largeParityFixture.workStateNodeIds[3]}"]`,
    );
    await expect(largeCountNode).toBeInTheDocument();
    await expect(
      largeCountNode?.querySelector('[data-state-work-progress="numeric"]'),
    ).toBeInTheDocument();
    await expect(
      largeCountNode?.querySelectorAll("[data-state-work-progress-dot]"),
    ).toHaveLength(0);
    await expect(
      canvasElement.querySelector<HTMLElement>(
        `.react-flow__node[data-id="${authoredNodeId}"]`,
      ),
    ).toHaveAttribute("style", expect.stringContaining(initialSize.width));
    await expect(
      canvasElement.querySelector<HTMLElement>(
        `.react-flow__node[data-id="${authoredNodeId}"]`,
      ),
    ).toHaveAttribute("style", expect.stringContaining(initialSize.height));
  },
};
