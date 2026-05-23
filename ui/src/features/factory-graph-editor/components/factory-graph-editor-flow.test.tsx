import "@xyflow/react/dist/style.css";

import { Background, ReactFlow, ReactFlowProvider } from "@xyflow/react";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";

import { installDashboardBrowserTestShims } from "../../../components/dashboard/test-browser-shims";
import "../../../styles.css";
import type { FactoryGraphTopology } from "../factory-graph-draft-types";
import { buildFactoryGraphEditorLayout } from "../factory-graph-editor-layout";
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
    {
      id: "workstation-on-failure:review->story:queued",
      kind: "workstation-on-failure",
      source: { kind: "workstation", name: "review" },
      sourceId: "workstation:review",
      target: {
        kind: "work-state",
        stateName: "queued",
        workTypeName: "story",
      },
      targetId: "work-state:story:queued",
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

const EDITOR_LAYOUT_TOPOLOGY: FactoryGraphTopology = {
  edges: [
    {
      id: "worker-resource:resource:prompt-kit->worker:writer",
      kind: "worker-resource",
      source: { kind: "resource", name: "prompt-kit" },
      sourceId: "resource:prompt-kit",
      target: { kind: "worker", name: "writer" },
      targetId: "worker:writer",
    },
    {
      id: "worker-assignment:worker:writer->workstation:review",
      kind: "worker-assignment",
      source: { kind: "worker", name: "writer" },
      sourceId: "worker:writer",
      target: { kind: "workstation", name: "review" },
      targetId: "workstation:review",
    },
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
      id: "resource:prompt-kit",
      key: { kind: "resource", name: "prompt-kit" },
      kind: "resource",
      label: "prompt-kit",
    },
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

  it("renders workstation outcome routes from their semantic source and target anchors", () => {
    const flow = buildFactoryGraphEditorFlowModel({
      canEditConnections: false,
      pendingAdditionEdgeIds: new Set<string>(),
      pendingAdditionNodeIds: new Set<string>(),
      pendingConnectionSource: null,
      pendingRemovalEdgeIds: new Set<string>(),
      pendingRemovalNodeIds: new Set<string>(),
      topology: EDITOR_EDGE_TOPOLOGY,
    });

    expect(flow.edges).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          id: "workstation-output:review->story:done",
          sourceHandle: "workstation-output-source",
          targetHandle: "workstation-output-target",
        }),
        expect.objectContaining({
          id: "workstation-on-failure:review->story:queued",
          sourceHandle: "workstation-on-failure-source",
          targetHandle: "workstation-on-failure-target",
        }),
      ]),
    );
  });

  it("uses observer-style layered positions from the editor layout adapter", async () => {
    const layout = await buildFactoryGraphEditorLayout(EDITOR_LAYOUT_TOPOLOGY);
    const positionsByNodeId = new Map(
      layout.nodes.map((node) => [node.nodeId, { x: node.x, y: node.y }]),
    );
    const flow = buildFactoryGraphEditorFlowModel({
      canEditConnections: false,
      layoutPositionsByNodeId: positionsByNodeId,
      pendingAdditionEdgeIds: new Set<string>(),
      pendingAdditionNodeIds: new Set<string>(),
      pendingConnectionSource: null,
      pendingRemovalEdgeIds: new Set<string>(),
      pendingRemovalNodeIds: new Set<string>(),
      topology: EDITOR_LAYOUT_TOPOLOGY,
    });
    const position = (nodeId: string) =>
      flow.nodes.find((node) => node.id === nodeId)?.position.x ?? -1;

    expect(position("resource:prompt-kit")).toBeLessThan(position("worker:writer"));
    expect(position("worker:writer")).toBeLessThan(position("workstation:review"));
    expect(position("workstation:review")).toBeLessThan(
      position("work-state:story:done"),
    );
    expect(flow.nodes.find((node) => node.id === "workstation:review")?.position).toEqual(
      positionsByNodeId.get("workstation:review"),
    );
  });
});

describe("factory graph editor edge interaction states", () => {
  let restoreBrowserTestShims: (() => void) | null = null;

  beforeEach(() => {
    restoreBrowserTestShims = installDashboardBrowserTestShims();
  });

  afterEach(() => {
    cleanup();
    restoreBrowserTestShims?.();
    restoreBrowserTestShims = null;
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
