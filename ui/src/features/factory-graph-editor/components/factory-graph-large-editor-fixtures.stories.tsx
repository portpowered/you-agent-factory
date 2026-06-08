import { Background, ReactFlow } from "@xyflow/react";
import { useEffect, useState } from "react";
import { expect, waitFor, within } from "storybook/test";

import "../../../styles.css";
import { factoryGraphLargeEditorFixtures } from "../lib/fixtures/factory-graph-large-editor-fixtures";
import { projectFactoryGraphWithCanonicalLayout } from "../lib/layout/factory-graph-layout-projection";
import {
  buildFactoryGraphEditorFlowModel,
  FACTORY_GRAPH_EDITOR_EDGE_TYPES,
  FACTORY_GRAPH_EDITOR_NODE_TYPES,
} from "./flow/factory-graph-editor-flow";

const fiveHundredNodeFixture = factoryGraphLargeEditorFixtures.fiveHundred;

function FiveHundredNodeEditorFixtureStory() {
  const [status, setStatus] = useState<"loading" | "ready" | "error">(
    "loading",
  );
  const [viewportReady, setViewportReady] = useState(false);
  const [flowModel, setFlowModel] = useState<ReturnType<
    typeof buildFactoryGraphEditorFlowModel
  > | null>(null);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;

    void projectFactoryGraphWithCanonicalLayout({
      canonicalLayout: fiveHundredNodeFixture.layout,
      topology: fiveHundredNodeFixture.topology,
    })
      .then((projection) => {
        if (cancelled) {
          return;
        }

        setFlowModel(
          buildFactoryGraphEditorFlowModel({
            canEditConnections: false,
            factoryDefinition: fiveHundredNodeFixture.factoryDefinition,
            layoutPositionsByNodeId: projection.layoutPositionsByNodeId,
            pendingAdditionEdgeIds: new Set<string>(),
            pendingConnectionSource: null,
            pendingAdditionNodeIds: new Set<string>(),
            pendingRemovalEdgeIds: new Set<string>(),
            pendingRemovalNodeIds: new Set<string>(),
            topology: fiveHundredNodeFixture.topology,
          }),
        );
        setStatus("ready");
      })
      .catch((error: unknown) => {
        if (cancelled) {
          return;
        }

        setStatus("error");
        setErrorMessage(
          error instanceof Error ? error.message : "Failed to project graph",
        );
      });

    return () => {
      cancelled = true;
    };
  }, []);

  return (
    <div
      className="h-[640px] w-full rounded-[1.5rem] border border-outline bg-surface-container-high p-4"
      data-factory-graph-editor-canvas="true"
      data-large-fixture-viewport-ready={viewportReady ? "true" : "false"}
    >
      {status === "loading" ? (
        <p role="status">Loading 500 node factory graph fixture…</p>
      ) : null}
      {status === "error" ? (
        <p role="alert">{errorMessage ?? "Failed to load fixture"}</p>
      ) : null}
      {status === "ready" && flowModel ? (
        <ReactFlow
          defaultEdgeOptions={{ selectable: false }}
          edgeTypes={FACTORY_GRAPH_EDITOR_EDGE_TYPES}
          edges={flowModel.edges}
          fitView={false}
          nodeTypes={FACTORY_GRAPH_EDITOR_NODE_TYPES}
          nodes={flowModel.nodes}
          nodesDraggable={false}
          onInit={(instance) => {
            void instance
              .fitView({
                maxZoom: 1.25,
                nodes: [{ id: "workstation:ws-0" }],
                padding: 0.2,
              })
              .then(() => {
                setViewportReady(true);
              });
          }}
        >
          <Background />
        </ReactFlow>
      ) : null}
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
      { timeout: 60_000 },
    );
    await expect(
      canvas.findByText("ws-0", undefined, { timeout: 60_000 }),
    ).resolves.toBeVisible();
    await expect(canvas.getByText("processor")).toBeVisible();
    await expect(
      canvas.queryByRole("status", { name: /loading 500 node/i }),
    ).toBeNull();
  },
};
