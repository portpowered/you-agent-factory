import "@xyflow/react/dist/style.css";

import { cleanup, render, screen } from "@testing-library/react";
import { Background, ReactFlow, ReactFlowProvider } from "@xyflow/react";

import { installDashboardBrowserTestShims } from "../../../components/dashboard/test-browser-shims";
import "../../../styles.css";
import { baseFactoryDefinition } from "../lib/factory-graph-draft.test-helpers";
import type { FactoryGraphTopology } from "../lib/factory-graph-draft-types";
import {
  buildFactoryGraphEditorFlowModel,
  FACTORY_GRAPH_EDITOR_EDGE_TYPES,
  FACTORY_GRAPH_EDITOR_NODE_TYPES,
} from "./factory-graph-editor-flow";

const PROGRESS_OUTCOME_ROUTE_TOPOLOGY: FactoryGraphTopology = {
  edges: [],
  nodes: [
    {
      id: "workstation:draft",
      key: { kind: "workstation", name: "draft" },
      kind: "workstation",
      label: "draft",
    },
    {
      id: "work-state:story:done",
      key: {
        kind: "work-state",
        stateName: "done",
        workTypeName: "story",
      },
      kind: "work-state",
      label: "story:done",
    },
    {
      id: "work-state:story:queued",
      key: {
        kind: "work-state",
        stateName: "queued",
        workTypeName: "story",
      },
      kind: "work-state",
      label: "story:queued",
    },
  ],
};

function renderProgressOutcomeRouteFlow(
  workstations: NonNullable<typeof baseFactoryDefinition.workstations>,
) {
  const flow = buildFactoryGraphEditorFlowModel({
    canEditConnections: true,
    pendingAdditionEdgeIds: new Set<string>(),
    pendingAdditionNodeIds: new Set<string>(),
    pendingConnectionSource: null,
    pendingRemovalEdgeIds: new Set<string>(),
    pendingRemovalNodeIds: new Set<string>(),
    topology: PROGRESS_OUTCOME_ROUTE_TOPOLOGY,
    workstations,
  });

  render(
    <div style={{ height: 420, width: 720 }}>
      <ReactFlowProvider>
        <ReactFlow
          edgeTypes={FACTORY_GRAPH_EDITOR_EDGE_TYPES}
          edges={flow.edges}
          fitView
          nodeTypes={FACTORY_GRAPH_EDITOR_NODE_TYPES}
          nodes={flow.nodes}
        >
          <Background />
        </ReactFlow>
      </ReactFlowProvider>
    </div>,
  );
}

describe("factory graph editor progress outcome route handles", () => {
  let restoreBrowserTestShims: (() => void) | null = null;

  beforeEach(() => {
    restoreBrowserTestShims = installDashboardBrowserTestShims();
  });

  afterEach(() => {
    cleanup();
    restoreBrowserTestShims?.();
    restoreBrowserTestShims = null;
  });

  it("hides continue and reject connect handles for standard processors without stopWords", async () => {
    renderProgressOutcomeRouteFlow([
      {
        ...baseFactoryDefinition.workstations[0],
        behavior: "STANDARD",
        stopWords: undefined,
      },
    ]);

    await screen.findByRole("button", { name: "Connect: draft Success" });
    expect(
      screen.getByRole("button", { name: "Connect: draft Failure" }),
    ).not.toBeNull();
    expect(
      screen.queryByRole("button", { name: "Connect: draft Continue" }),
    ).toBeNull();
    expect(
      screen.queryByRole("button", { name: "Connect: draft Reject" }),
    ).toBeNull();
  });

  it("shows continue and reject connect handles when stopWords are configured", async () => {
    renderProgressOutcomeRouteFlow([
      {
        ...baseFactoryDefinition.workstations[0],
        behavior: "STANDARD",
        stopWords: ["DONE"],
      },
    ]);

    await screen.findByRole("button", { name: "Connect: draft Continue" });
    expect(
      screen.getByRole("button", { name: "Connect: draft Reject" }),
    ).not.toBeNull();
  });
});
