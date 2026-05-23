import "@xyflow/react/dist/style.css";

import { Background, ReactFlow, ReactFlowProvider } from "@xyflow/react";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";

import { installDashboardBrowserTestShims } from "../../../components/dashboard/test-browser-shims";
import "../../../styles.css";
import type { FactoryGraphTopology } from "../factory-graph-draft-types";
import {
  buildFactoryGraphEditorFlowModel,
  FACTORY_GRAPH_EDITOR_EDGE_TYPES,
  FACTORY_GRAPH_EDITOR_NODE_TYPES,
} from "./factory-graph-editor-flow";

const EDITOR_EDGE_TOPOLOGY: FactoryGraphTopology = {
  edges: [
    {
      id: "workstation-output:review->story:done",
      kind: "workstation-output",
      source: { kind: "workstation", name: "review" },
      sourceId: "workstation:review",
      target: {
        kind: "work-state",
        stateName: "done",
        workTypeName: "story",
      },
      targetId: "work-state:story:done",
    },
  ],
  nodes: [
    {
      id: "workstation:review",
      key: { kind: "workstation", name: "review" },
      kind: "workstation",
      label: "review",
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
  ],
};

const EDITOR_NODE_TOPOLOGY: FactoryGraphTopology = {
  edges: [],
  nodes: [
    {
      id: "worker:writer",
      key: { kind: "worker", name: "writer" },
      kind: "worker",
      label: "writer",
    },
    {
      id: "workstation:review",
      key: { kind: "workstation", name: "review" },
      kind: "workstation",
      label: "review",
    },
  ],
};

function renderEditorFlow(
  canEditConnections = false,
  topology: FactoryGraphTopology = EDITOR_EDGE_TOPOLOGY,
  options?: {
    pendingAdditionNodeIds?: ReadonlySet<string>;
    workerStatusByName?: ReadonlyMap<string, "active" | "errored" | "idle" | "unavailable">;
  },
) {
  const flow = buildFactoryGraphEditorFlowModel({
    canEditConnections,
    pendingAdditionEdgeIds: new Set<string>(),
    pendingAdditionNodeIds: options?.pendingAdditionNodeIds ?? new Set<string>(),
    pendingConnectionSource: null,
    pendingRemovalEdgeIds: new Set<string>(),
    pendingRemovalNodeIds: new Set<string>(),
    topology,
    workerStatusByName: options?.workerStatusByName,
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

describe("factory graph editor edge labels", () => {
  let restoreBrowserTestShims: (() => void) | null = null;

  beforeEach(() => {
    restoreBrowserTestShims = installDashboardBrowserTestShims();
  });

  afterEach(() => {
    cleanup();
    restoreBrowserTestShims?.();
    restoreBrowserTestShims = null;
  });

  it("reuses observer-style badges and title treatment for editor nodes", async () => {
    renderEditorFlow(false, EDITOR_NODE_TOPOLOGY, {
      pendingAdditionNodeIds: new Set(["workstation:review"]),
      workerStatusByName: new Map([["writer", "active"]]),
    });

    const reviewNode = (await screen.findByTitle("review")).closest("article");
    const writerNode = (await screen.findByTitle("writer")).closest("article");

    expect(reviewNode).not.toBeNull();
    expect(writerNode).not.toBeNull();
    expect(
      reviewNode.querySelector("[data-factory-entity-semantic-icon] [data-graph-semantic-icon='workstation']"),
    ).not.toBeNull();
    expect(
      reviewNode.querySelector("[data-factory-entity-title]")?.textContent,
    ).toContain("review");
    expect(reviewNode.textContent).toContain("Workstation");
    expect(reviewNode.textContent).toContain("Pending");

    expect(
      writerNode.querySelector("[data-factory-entity-semantic-icon] [data-graph-semantic-icon='active-work']"),
    ).not.toBeNull();
    expect(writerNode.textContent).toContain("Worker");
    expect(writerNode.textContent).toContain("Active");
  });

  it("keeps inline labels hidden by default while preserving accessible edge names", async () => {
    renderEditorFlow();

    const edge = await screen.findByLabelText(
      "Success route from review to story:done",
    );
    const edgeShape = edge.querySelector(".agent-factory-editor-edge");

    expect(edge.getAttribute("role")).toBe("button");
    expect(edgeShape?.getAttribute("data-label-visible")).toBe("false");
  });

  it("reveals an edge label on hover", async () => {
    renderEditorFlow();

    const edge = await screen.findByLabelText(
      "Success route from review to story:done",
    );
    const edgeShape = edge.querySelector(".agent-factory-editor-edge");

    fireEvent.mouseEnter(edge);
    await waitFor(() => {
      expect(edgeShape?.getAttribute("data-label-visible")).toBe("true");
    });

    fireEvent.mouseLeave(edge);
    await waitFor(() => {
      expect(edgeShape?.getAttribute("data-label-visible")).toBe("false");
    });
  });

  it("shows edge labels while connection editing is active", async () => {
    renderEditorFlow(true);

    const edge = await screen.findByLabelText(
      "Success route from review to story:done",
    );

    expect(
      edge.querySelector(".agent-factory-editor-edge")?.getAttribute(
        "data-label-visible",
      ),
    ).toBe("true");
  });
});
