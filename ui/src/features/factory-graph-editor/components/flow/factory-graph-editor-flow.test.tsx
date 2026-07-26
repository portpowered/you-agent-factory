// @component-test-runner vitest: ELK's browser bundle requires Vitest/jsdom module globals.
import "@xyflow/react/dist/style.css";

import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { Background, ReactFlow, ReactFlowProvider } from "@xyflow/react";

import { installDashboardBrowserTestShims } from "../../../../components/dashboard/test-browser-shims";
import "../../../../styles.css";
import { baseFactoryDefinition } from "../../lib/draft/factory-graph-draft.test-helpers";
import { buildFactoryGraphTopologyFromDefinition } from "../../lib/draft/factory-graph-draft-graph";
import type {
  CanonicalFactoryDefinition,
  FactoryGraphTopology,
} from "../../lib/draft/factory-graph-draft-types";
import { buildFactoryGraphEditorLayout } from "../../lib/editor/factory-graph-editor-layout";
import {
  buildFactoryGraphEditorFlowModel,
  FACTORY_GRAPH_EDITOR_EDGE_TYPES,
  FACTORY_GRAPH_EDITOR_NODE_TYPES,
} from "../flow/factory-graph-editor-flow";

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
    {
      id: "workstation-on-continue:review->story:retry",
      kind: "workstation-on-continue",
      source: { kind: "workstation", name: "review" },
      sourceId: "workstation:review",
      target: {
        kind: "work-state",
        stateName: "retry",
        workTypeName: "story",
      },
      targetId: "work-state:story:retry",
    },
    {
      id: "workstation-on-rejection:review->story:rejected",
      kind: "workstation-on-rejection",
      source: { kind: "workstation", name: "review" },
      sourceId: "workstation:review",
      target: {
        kind: "work-state",
        stateName: "rejected",
        workTypeName: "story",
      },
      targetId: "work-state:story:rejected",
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
    {
      id: "work-state:story:retry",
      key: {
        kind: "work-state",
        stateName: "retry",
        workTypeName: "story",
      },
      kind: "work-state",
      label: "story:retry",
    },
    {
      id: "work-state:story:rejected",
      key: {
        kind: "work-state",
        stateName: "rejected",
        workTypeName: "story",
      },
      kind: "work-state",
      label: "story:rejected",
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
    factoryDefinition?: CanonicalFactoryDefinition | null;
    pendingAdditionNodeIds?: ReadonlySet<string>;
    pendingRemovalNodeIds?: ReadonlySet<string>;
    workerStatusByName?: ReadonlyMap<
      string,
      "active" | "errored" | "idle" | "unavailable"
    >;
  },
) {
  const flow = buildFactoryGraphEditorFlowModel({
    canEditConnections,
    factoryDefinition: options?.factoryDefinition,
    pendingAdditionEdgeIds: new Set<string>(),
    pendingAdditionNodeIds:
      options?.pendingAdditionNodeIds ?? new Set<string>(),
    pendingConnectionSource: null,
    pendingRemovalEdgeIds: new Set<string>(),
    pendingRemovalNodeIds: options?.pendingRemovalNodeIds ?? new Set<string>(),
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
    expect(reviewNode?.className).toContain("shadow-none");
    expect(reviewNode?.className).not.toContain("shadow-af-card");
    expect(reviewNode?.className).not.toContain("shadow-af-panel");
    expect(writerNode?.className).toContain("shadow-none");
    expect(writerNode?.className).not.toContain("shadow-af-card");
    expect(writerNode?.className).not.toContain("shadow-af-panel");
    expect(
      reviewNode.querySelector(
        "[data-factory-entity-semantic-icon] [data-graph-semantic-icon='workstation']",
      ),
    ).not.toBeNull();
    expect(
      reviewNode.querySelector("[data-factory-entity-title]")?.textContent,
    ).toContain("review");
    expect(reviewNode.textContent).toContain("Workstation");
    expect(reviewNode.textContent).toContain("Pending");

    expect(
      writerNode.querySelector(
        "[data-factory-entity-semantic-icon] [data-graph-semantic-icon='active-work']",
      ),
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
          targetHandle: "work-state-input-target",
        }),
        expect.objectContaining({
          id: "workstation-on-failure:review->story:queued",
          sourceHandle: "workstation-on-failure-source",
          targetHandle: "work-state-input-target",
        }),
        expect.objectContaining({
          id: "workstation-on-continue:review->story:retry",
          sourceHandle: "workstation-on-continue-source",
          targetHandle: "work-state-input-target",
        }),
        expect.objectContaining({
          id: "workstation-on-rejection:review->story:rejected",
          sourceHandle: "workstation-on-rejection-source",
          targetHandle: "work-state-input-target",
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

    expect(position("resource:prompt-kit")).toBeLessThan(
      position("worker:writer"),
    );
    expect(position("worker:writer")).toBeLessThan(
      position("workstation:review"),
    );
    expect(position("workstation:review")).toBeLessThan(
      position("work-state:story:done"),
    );
    expect(
      flow.nodes.find((node) => node.id === "workstation:review")?.position,
    ).toEqual(positionsByNodeId.get("workstation:review"));
  });
});

describe("factory graph editor work state lifecycle styling", () => {
  let restoreBrowserTestShims: (() => void) | null = null;

  const lifecycleFactoryDefinition = {
    ...baseFactoryDefinition,
    workTypes: [
      {
        name: "story",
        states: [
          { name: "queued", type: "INITIAL" },
          { name: "review", type: "PROCESSING" },
          { name: "done", type: "TERMINAL" },
          { name: "failed", type: "FAILED" },
        ],
      },
    ],
  } satisfies CanonicalFactoryDefinition;

  const lifecycleTopology = buildFactoryGraphTopologyFromDefinition(
    lifecycleFactoryDefinition,
  );

  beforeEach(() => {
    restoreBrowserTestShims = installDashboardBrowserTestShims();
  });

  afterEach(() => {
    cleanup();
    restoreBrowserTestShims?.();
    restoreBrowserTestShims = null;
  });

  it("applies phase surface classes from projected workStateType", async () => {
    renderEditorFlow(false, lifecycleTopology, {
      factoryDefinition: lifecycleFactoryDefinition,
    });

    const expectSurface = async (title: string) => {
      const node = (await screen.findByTitle(title)).closest("article");
      expect(node).not.toBeNull();
      expect(node?.className).toContain("border-info-border");
      expect(node?.className).toContain("bg-info-container");
    };

    await expectSurface("story:queued");
    await expectSurface("story:review");
    await expectSurface("story:done");
    await expectSurface("story:failed");
  });

  it("keeps non-work-state nodes on existing kind styling", async () => {
    renderEditorFlow(false, lifecycleTopology, {
      factoryDefinition: lifecycleFactoryDefinition,
    });

    const workerNode = (await screen.findByTitle("writer")).closest("article");
    expect(workerNode?.className).toContain("border-info-border");
    expect(workerNode?.className).toContain("bg-info-container");
  });

  it("falls back to neutral work-state styling without factory definition", async () => {
    renderEditorFlow(false, lifecycleTopology);

    const queuedNode = (await screen.findByTitle("story:queued")).closest(
      "article",
    );
    expect(queuedNode?.className).toContain("border-info-border");
    expect(queuedNode?.className).toContain("bg-info-container");
  });

  it("keeps draft addition and removal treatments visible on phase-colored nodes", async () => {
    renderEditorFlow(false, lifecycleTopology, {
      factoryDefinition: lifecycleFactoryDefinition,
      pendingAdditionNodeIds: new Set(["work-state:story:queued"]),
      pendingRemovalNodeIds: new Set(["work-state:story:failed"]),
    });

    const additionNode = (await screen.findByTitle("story:queued")).closest(
      "article",
    );
    const removalNode = (await screen.findByTitle("story:failed")).closest(
      "article",
    );

    expect(additionNode?.className).toContain("border-info-border");
    expect(additionNode?.className).toContain("ring-af-warning-border");
    expect(additionNode?.textContent).toContain("Pending");

    expect(removalNode?.className).toContain("ring-af-danger-border");
    expect(removalNode?.className).toContain("bg-error-container");
    expect(removalNode?.textContent).toContain("Removing");
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
      edge
        .querySelector(".agent-factory-editor-edge")
        ?.getAttribute("data-label-visible"),
    ).toBe("true");
  });
});
