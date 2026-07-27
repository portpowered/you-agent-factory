import "@testing-library/jest-dom/vitest";
import "./test-support/react-flow-current-activity-card-component.mocks";

import {
  cleanup,
  fireEvent,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import type { DashboardSnapshot } from "../../../../api/dashboard/types";
import { semanticWorkflowDashboardSnapshot } from "../../../../components/dashboard/test-fixtures";
import { resourceOccupancySnapshotForTick } from "../../../../components/dashboard/timeline-test-fixtures";
import {
  baseFactoryDefinitionDocument,
  workerDenseFactoryDefinitionDocument,
} from "../../../../testing/graph-editor-harness";
import { useCurrentFactoryDocument } from "../../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import { useFactoryGraphDraftState } from "../../../factory-graph-editor/hooks/factory-graph-draft-hook";
import { maintainerRuntimeShapedFactory } from "../../../factory-graph-editor/lib/fixtures/maintainer-runtime-shaped-factory.fixture";
import {
  EXHAUSTION_WORKSTATION_ICON_METADATA,
  SUPPORTED_WORKSTATION_ICON_METADATA,
} from "../../../flowchart/lib/workstation-icon-metadata";
import { buildCurrentActivityGraphLayoutFromFactory } from "../../lib/current-activity-factory-graph-layout";
import { buildGraphEdges } from "../../lib/react-flow-current-activity-card-edges";
import {
  buildActiveGraphHighlights,
  buildActiveItemLabelsByPlaceId,
  buildCurrentActivityNodes,
  buildHandleAssignments,
  buildVisibleGraphEdges,
} from "../../lib/react-flow-current-activity-card-graph";
import {
  dashboardSnapshotWithActiveWorkItemCount,
  dashboardSnapshotWithStateCounts,
  defaultDraftState,
  expandGraphLegend,
  getStateNodeArticle,
  refreshFactoryFromTopology,
  registerCurrentActivityCardTestLifecycle,
  reinstallDashboardBrowserTestShims,
  renderCurrentActivity,
  workerDenseSnapshot,
} from "./test-support/react-flow-current-activity-card-component.harness";

const LEGEND_ICON_EXPECTATIONS = [
  ["Queue", "queue"],
  ["Processing", "processing"],
  ["Terminal", "terminal"],
  ["Failed state", "failed"],
  ["Resource", "resource"],
  ["Constraint", "constraint"],
  ["Limit", "limit"],
  ...SUPPORTED_WORKSTATION_ICON_METADATA.map((metadata) => [
    metadata.label,
    metadata.iconKind,
  ]),
  ["Active work", "active-work"],
  [
    EXHAUSTION_WORKSTATION_ICON_METADATA.label,
    EXHAUSTION_WORKSTATION_ICON_METADATA.iconKind,
  ],
] as const;
function canonicalObserverSnapshot(): DashboardSnapshot {
  const snapshot = structuredClone(semanticWorkflowDashboardSnapshot);
  snapshot.factory = {
    name: "canonical-observer",
    resources: [{ capacity: 2, name: "agent-slot" }],
    workers: [
      {
        model: "gpt-5",
        name: "planner",
        type: "MODEL_WORKER",
      },
      {
        model: "gpt-5",
        name: "agent",
        resources: [{ capacity: 1, name: "agent-slot" }],
        type: "MODEL_WORKER",
      },
      {
        model: "gpt-5",
        name: "reviewer",
        type: "MODEL_WORKER",
      },
    ],
    workTypes: [
      {
        name: "story",
        states: [
          { name: "init", type: "INITIAL" },
          { name: "ready", type: "PROCESSING" },
          { name: "implemented", type: "PROCESSING" },
          { name: "documented", type: "PROCESSING" },
          { name: "blocked", type: "FAILED" },
          { name: "complete", type: "TERMINAL" },
        ],
      },
    ],
    workstations: [
      {
        behavior: "STANDARD",
        id: "plan",
        inputs: [{ state: "init", workType: "story" }],
        name: "Plan",
        outputs: [{ state: "ready", workType: "story" }],
        type: "MODEL_WORKSTATION",
        worker: "planner",
      },
      {
        behavior: "REPEATER",
        id: "implement",
        inputs: [{ state: "ready", workType: "story" }],
        name: "Implement",
        onFailure: [{ state: "blocked", workType: "story" }],
        outputs: [{ state: "implemented", workType: "story" }],
        resources: [{ capacity: 1, name: "agent-slot" }],
        type: "MODEL_WORKSTATION",
        worker: "agent",
      },
      {
        behavior: "STANDARD",
        id: "document",
        inputs: [{ state: "ready", workType: "story" }],
        name: "Document",
        outputs: [{ state: "documented", workType: "story" }],
        type: "MODEL_WORKSTATION",
        worker: "agent",
      },
      {
        behavior: "REPEATER",
        id: "review",
        inputs: [
          { state: "implemented", workType: "story" },
          { state: "documented", workType: "story" },
        ],
        name: "Review",
        onContinue: [{ state: "ready", workType: "story" }],
        outputs: [{ state: "complete", workType: "story" }],
        type: "MODEL_WORKSTATION",
        worker: "reviewer",
      },
      {
        behavior: "STANDARD",
        id: "repair",
        inputs: [{ state: "blocked", workType: "story" }],
        name: "Repair",
        outputs: [{ state: "ready", workType: "story" }],
        type: "MODEL_WORKSTATION",
        worker: "agent",
      },
    ],
  };
  snapshot.topology = {
    edges: [],
    workstation_node_ids: [],
    workstation_nodes_by_id: {},
  };

  return snapshot;
}
async function expectRenderableCurrentActivityGraphEdges(
  snapshot: DashboardSnapshot,
) {
  const factory = snapshot.factory;
  if (!factory) {
    throw new Error("Expected snapshot factory for graph edge assertions.");
  }

  const graphLayout = await buildCurrentActivityGraphLayoutFromFactory(factory);
  const visibleGraphEdges = buildVisibleGraphEdges(graphLayout);
  const handleAssignments = buildHandleAssignments(
    visibleGraphEdges,
    graphLayout.nodes,
  );
  const nodes = buildCurrentActivityNodes({
    activeExecutionsByWorkstationNodeID: {},
    activeGraphHighlights: buildActiveGraphHighlights(
      [],
      visibleGraphEdges,
      graphLayout.nodes,
    ),
    activeItemLabelsByPlaceId: buildActiveItemLabelsByPlaceId([]),
    graphLayout,
    now: Date.parse("2026-04-08T12:00:04Z"),
    onSelectStateNode: vi.fn(),
    onSelectWorkID: vi.fn(),
    onSelectDoc: vi.fn(),
    onSelectResource: vi.fn(),
    onSelectWorker: vi.fn(),
    onSelectWorkType: vi.fn(),
    onSelectWorkstation: vi.fn(),
    selection: null,
    snapshot,
  });
  const reactFlowEdges = buildGraphEdges(
    buildActiveGraphHighlights([], visibleGraphEdges, graphLayout.nodes),
    handleAssignments,
    new Set(),
    visibleGraphEdges,
    nodes,
  );

  expect(visibleGraphEdges.length).toBeGreaterThan(0);
  expect(reactFlowEdges.length).toBeGreaterThan(0);
  expect(
    reactFlowEdges.every(
      (edge) =>
        edge.sourceHandle && edge.targetHandle && edge.source && edge.target,
    ),
  ).toBe(true);
}
function dashboardSnapshotWithActiveImplementWorkstation(): DashboardSnapshot {
  const snapshot = structuredClone(semanticWorkflowDashboardSnapshot);
  const activeExecution =
    snapshot.runtime.active_executions_by_dispatch_id?.[
      "dispatch-review-active"
    ];
  const implementWorkstation =
    snapshot.topology.workstation_nodes_by_id.implement;

  if (activeExecution && implementWorkstation) {
    activeExecution.workstation_node_id = "implement";
    activeExecution.transition_id = implementWorkstation.transition_id;
    activeExecution.workstation_name = implementWorkstation.workstation_name;
    activeExecution.consumed_tokens = [
      {
        token_id: "token-implement-story",
        place_id: "story:ready",
        name: "Active Story",
        work_id: "work-active-story",
        work_type_id: "story",
        trace_id: "trace-active-story",
        created_at: "2026-04-08T12:00:00Z",
        entered_at: "2026-04-08T12:00:00Z",
      },
      {
        token_id: "token-implement-agent-slot",
        place_id: "agent-slot:available",
        name: "Agent Slot",
        work_id: "resource-agent-slot",
        work_type_id: "agent-slot",
        created_at: "2026-04-08T12:00:00Z",
        entered_at: "2026-04-08T12:00:00Z",
      },
    ];
  }

  snapshot.runtime.active_workstation_node_ids = ["implement"];
  snapshot.runtime.current_work_items_by_place_id = {
    ...(snapshot.runtime.current_work_items_by_place_id ?? {}),
    "story:ready": [
      {
        display_name: "Active Story",
        trace_id: "trace-active-story",
        work_id: "work-active-story",
        work_type_id: "story",
      },
    ],
  };

  return refreshFactoryFromTopology(snapshot);
}

function dashboardSnapshotWithResourceReturnEdge(): DashboardSnapshot {
  const snapshot = structuredClone(semanticWorkflowDashboardSnapshot);
  const implementWorkstation =
    snapshot.topology.workstation_nodes_by_id.implement;
  const agentSlotPlace: DashboardPlaceRef = {
    kind: "resource",
    place_id: "agent-slot:available",
    state_value: "available",
    type_id: "agent-slot",
  };

  if (implementWorkstation) {
    implementWorkstation.output_places = [
      ...(implementWorkstation.output_places ?? []),
      agentSlotPlace,
    ];
    implementWorkstation.output_place_ids = [
      ...(implementWorkstation.output_place_ids ?? []),
      agentSlotPlace.place_id,
    ];
  }

  return refreshFactoryFromTopology(snapshot);
}
function dashboardSnapshotWithExhaustionRuleNode(): DashboardSnapshot {
  const snapshot = structuredClone(semanticWorkflowDashboardSnapshot);

  snapshot.topology.workstation_node_ids = [
    ...snapshot.topology.workstation_node_ids,
    "executor-loop-breaker",
  ];
  snapshot.topology.workstation_nodes_by_id["executor-loop-breaker"] = {
    input_place_ids: ["story:ready"],
    input_places: [
      {
        kind: "work_state",
        place_id: "story:ready",
        state_category: "PROCESSING",
        state_value: "ready",
        type_id: "story",
      },
    ],
    node_id: "executor-loop-breaker",
    output_place_ids: ["story:blocked"],
    output_places: [
      {
        kind: "work_state",
        place_id: "story:blocked",
        state_category: "FAILED",
        state_value: "blocked",
        type_id: "story",
      },
    ],
    transition_id: "executor-loop-breaker",
    workstation_name: "executor-loop-breaker",
  };
  snapshot.runtime.active_dispatch_ids = [
    ...(snapshot.runtime.active_dispatch_ids ?? []),
    "dispatch-exhaustion-should-not-render-work",
  ];
  snapshot.runtime.active_executions_by_dispatch_id = {
    ...(snapshot.runtime.active_executions_by_dispatch_id ?? {}),
    "dispatch-exhaustion-should-not-render-work": {
      consumed_tokens: [],
      dispatch_id: "dispatch-exhaustion-should-not-render-work",
      started_at: "2026-04-08T12:00:00Z",
      transition_id: "executor-loop-breaker",
      workstation_node_id: "executor-loop-breaker",
      workstation_name: "executor-loop-breaker",
      work_items: [
        {
          display_name: "Should Not Render",
          trace_id: "trace-hidden-exhaustion",
          work_id: "work-hidden-exhaustion",
          work_type_id: "story",
        },
      ],
    },
  };

  return snapshot;
}

describe("ReactFlowCurrentActivityCard graph semantics", () => {
  registerCurrentActivityCardTestLifecycle();

  it("renders active observer graph state from the canonical snapshot factory without topology fallback", async () => {
    const snapshot = canonicalObserverSnapshot();

    renderCurrentActivity({ snapshot });

    expect(
      await screen.findByRole("region", { name: "Work graph viewport" }),
    ).toBeTruthy();
    await waitFor(() => {
      expect(
        screen.getAllByRole("button", { name: /Select .* workstation/ }),
      ).toHaveLength(5);
    });

    expect(screen.getByLabelText("agent-slot")).toBeTruthy();
    expect(screen.getByText("worker:agent")).toBeTruthy();
    expect(screen.getByText("worker:reviewer")).toBeTruthy();
    expect(screen.getByLabelText("2 resource tokens")).toBeTruthy();
    expect(screen.getByText("Active Story")).toBeTruthy();
    const reviewWorkstationButton = screen.getByRole("button", {
      name: "Select Review workstation",
    });
    expect(
      within(reviewWorkstationButton)
        .getByRole("img", { name: "Repeater workstation" })
        .getAttribute("data-graph-semantic-icon"),
    ).toBe("repeater");
    expect(
      reviewWorkstationButton
        .closest("[data-current-activity-node-type='workstation']")
        ?.className.includes("border-af-success-border"),
    ).toBe(true);
    expect(
      within(reviewWorkstationButton).queryByRole("img", { name: "Active" }),
    ).toBeNull();
    expect(await getStateNodeArticle("story:implemented")).toBeTruthy();
    expect(
      (await getStateNodeArticle("story:documented"))
        .querySelector("article")
        ?.className.includes("opacity-[0.45]"),
    ).toBe(true);
  });

  it("selects resource nodes from the live activity graph", async () => {
    const { onSelectResource } = renderCurrentActivity({
      snapshot: semanticWorkflowDashboardSnapshot,
    });

    const resourceButton = await screen.findByRole("button", {
      name: "Select agent-slot resource",
    });

    fireEvent.click(resourceButton);

    expect(onSelectResource).toHaveBeenCalledWith("agent-slot");
  });

  it("shows selected styling for the active resource selection", async () => {
    renderCurrentActivity({
      selection: { kind: "resource", resourceName: "agent-slot" },
      snapshot: semanticWorkflowDashboardSnapshot,
    });

    const resourceButton = await screen.findByRole("button", {
      name: "Select agent-slot resource",
    });
    expect(resourceButton.getAttribute("aria-pressed")).toBe("true");
    expect(resourceButton.getAttribute("data-selected-resource")).toBe("true");
  });

  it("selects worker nodes from the live activity graph", async () => {
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: workerDenseFactoryDefinitionDocument,
      error: null,
      status: "success",
    } as never);
    vi.mocked(useFactoryGraphDraftState).mockReturnValue({
      ...defaultDraftState,
      latestDocument: workerDenseFactoryDefinitionDocument,
      pendingFactoryDefinition: workerDenseFactoryDefinitionDocument,
    } as never);
    const snapshot = workerDenseSnapshot();
    snapshot.factory = workerDenseFactoryDefinitionDocument;

    const { onSelectWorker } = renderCurrentActivity({
      snapshot,
    });

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: "Select writer worker" }),
      ).toBeTruthy();
    });

    fireEvent.click(
      screen.getByRole("button", { name: "Select writer worker" }),
    );

    expect(onSelectWorker).toHaveBeenCalledWith("writer");
  });

  it("renders maintainer runtime-shaped factory workers without a bare worker: node", async () => {
    const factoryDocument = {
      ...maintainerRuntimeShapedFactory,
      version: { logical: "1", physical: "2026-05-31T20:00:00Z" },
    };
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: factoryDocument,
      error: null,
      status: "success",
    } as never);
    vi.mocked(useFactoryGraphDraftState).mockReturnValue({
      ...defaultDraftState,
      latestDocument: factoryDocument,
      pendingFactoryDefinition: factoryDocument,
    } as never);
    const snapshot = structuredClone(semanticWorkflowDashboardSnapshot);
    snapshot.factory = maintainerRuntimeShapedFactory;

    const { onSelectWorker } = renderCurrentActivity({ snapshot });

    expect(
      await screen.findByRole("button", { name: "Select processor worker" }),
    ).toBeTruthy();
    expect(
      screen.getByRole("button", { name: "Select workspace-setup worker" }),
    ).toBeTruthy();
    expect(screen.getByLabelText("worker:processor")).toBeTruthy();
    expect(screen.getByLabelText("worker:workspace-setup")).toBeTruthy();
    expect(screen.queryByLabelText("worker:")).toBeNull();

    fireEvent.click(
      screen.getByRole("button", { name: "Select processor worker" }),
    );
    expect(onSelectWorker).toHaveBeenCalledWith("processor");
  });

  it("shows selected styling for the active worker selection", async () => {
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: workerDenseFactoryDefinitionDocument,
      error: null,
      status: "success",
    } as never);
    vi.mocked(useFactoryGraphDraftState).mockReturnValue({
      ...defaultDraftState,
      latestDocument: workerDenseFactoryDefinitionDocument,
      pendingFactoryDefinition: workerDenseFactoryDefinitionDocument,
    } as never);
    const snapshot = workerDenseSnapshot();
    snapshot.factory = workerDenseFactoryDefinitionDocument;

    renderCurrentActivity({
      selection: { kind: "worker", workerName: "writer" },
      snapshot,
    });

    const workerButton = await screen.findByRole("button", {
      name: "Select writer worker",
    });
    expect(workerButton.getAttribute("aria-pressed")).toBe("true");
    expect(workerButton.getAttribute("data-selected-worker")).toBe("true");
  });

  it("keeps observer selection callbacks stable for canonical factory-backed graph nodes", async () => {
    const { onSelectStateNode, onSelectWorkID, onSelectWorkstation } =
      renderCurrentActivity({
        snapshot: canonicalObserverSnapshot(),
        selection: { kind: "state-node", placeId: "story:implemented" },
      });

    const implementedState = await screen.findByRole("button", {
      name: "Select story:implemented state",
    });
    expect(implementedState.getAttribute("data-selected-state")).toBe("true");

    fireEvent.click(
      screen.getByRole("button", { name: "Select Review workstation" }),
    );
    fireEvent.click(
      screen.getByRole("button", { name: "Select story:ready state" }),
    );
    fireEvent.click(screen.getByRole("button", { name: /Active Story/ }));

    expect(onSelectWorkstation).toHaveBeenCalledWith("review");
    expect(onSelectStateNode).toHaveBeenCalledWith("story:ready");
    expect(onSelectWorkID).toHaveBeenCalledWith("work-active-story", {
      dispatchID: "dispatch-review-active",
      nodeID: "review",
    });
  });

  it("renders semantic workflow activity with active, terminal, and failed graph states", async () => {
    renderCurrentActivity({ snapshot: semanticWorkflowDashboardSnapshot });

    expect(
      await screen.findByRole("region", { name: "Work graph viewport" }),
    ).toBeTruthy();
    await waitFor(() => {
      expect(
        screen.getAllByRole("button", { name: /Select .* workstation/ }),
      ).toHaveLength(5);
    });
    expect(screen.queryByText("Workstation Definition")).toBeNull();
    expect(screen.queryByText("State Position")).toBeNull();
    expect(
      screen.getByRole("button", { name: "Select story:ready state" }),
    ).toBeTruthy();
    expect(screen.getByLabelText("agent-slot")).toBeTruthy();
    expect(screen.getByLabelText("worker:agent")).toBeTruthy();
    expect(
      screen.getByRole("button", { name: "Select story work type" }),
    ).toBeTruthy();
    expect(
      screen
        .getAllByRole("img", { name: "Queue" })[0]
        .getAttribute("data-graph-semantic-icon"),
    ).toBe("queue");
    expect(
      screen
        .getAllByRole("img", { name: "Resource" })[0]
        .getAttribute("data-graph-semantic-icon"),
    ).toBe("resource");
    expect(
      screen
        .getAllByRole("img", { name: "Worker" })[0]
        .getAttribute("data-graph-semantic-icon"),
    ).toBe("worker");
    expect(screen.getByLabelText("2 resource tokens")).toBeTruthy();
    const reviewButton = screen.getByRole("button", {
      name: "Select Review workstation",
    });
    expect(reviewButton).toBeTruthy();
    expect(
      within(reviewButton)
        .getByRole("img", { name: "Repeater workstation" })
        .getAttribute("data-graph-semantic-icon"),
    ).toBe("repeater");
    expect(
      reviewButton
        .closest("[data-current-activity-node-type='workstation']")
        ?.className.includes("border-af-success-border"),
    ).toBe(true);
    expect(
      within(reviewButton).queryByRole("img", { name: "Active" }),
    ).toBeNull();
    expect(await getStateNodeArticle("story:documented")).toBeTruthy();
    expect(screen.getByText("Active Story")).toBeTruthy();
    expect(screen.queryByText("dispatch-review-active")).toBeNull();
    expect(screen.queryByText("Active Work")).toBeNull();
    expect(
      screen.getByRole("button", { name: "Select story:blocked state" }),
    ).toBeTruthy();
  });

  it("renders React Flow edges for visible graph connections", async () => {
    const reactFlowErrorSpy = vi
      .spyOn(console, "error")
      .mockImplementation(() => {});
    try {
      renderCurrentActivity({ snapshot: semanticWorkflowDashboardSnapshot });

      expect(
        await screen.findByRole("region", { name: "Work graph viewport" }),
      ).toBeTruthy();
      await waitFor(() => {
        expect(
          screen.getAllByRole("button", { name: /Select .* workstation/ }),
        ).toHaveLength(5);
      });
      await expectRenderableCurrentActivityGraphEdges(
        semanticWorkflowDashboardSnapshot,
      );
      expect(
        reactFlowErrorSpy.mock.calls.some(([firstArg]) =>
          String(firstArg).includes(
            "Couldn't create edge for source handle id",
          ),
        ),
      ).toBe(false);
    } finally {
      reactFlowErrorSpy.mockRestore();
    }
  });

  it("does not trigger React Flow missing-handle errors after entering embedded editor mode", async () => {
    const reactFlowErrorSpy = vi
      .spyOn(console, "error")
      .mockImplementation(() => {});
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: baseFactoryDefinitionDocument,
      error: null,
      status: "success",
    } as never);

    try {
      renderCurrentActivity({ snapshot: semanticWorkflowDashboardSnapshot });

      fireEvent.click(screen.getByRole("button", { name: "Edit mode" }));

      await expectRenderableCurrentActivityGraphEdges(
        semanticWorkflowDashboardSnapshot,
      );

      expect(
        reactFlowErrorSpy.mock.calls.some(([firstArg]) =>
          String(firstArg).includes(
            "Couldn't create edge for source handle id",
          ),
        ),
      ).toBe(false);
    } finally {
      reactFlowErrorSpy.mockRestore();
    }
  });

  it("keeps pending shared-surface editor route drafts free of handle-attachment errors", async () => {
    const reactFlowErrorSpy = vi
      .spyOn(console, "error")
      .mockImplementation(() => {});
    const idleSnapshot = dashboardSnapshotWithActiveWorkItemCount(0);

    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: baseFactoryDefinitionDocument,
      error: null,
      status: "success",
    } as never);
    vi.mocked(useFactoryGraphDraftState).mockReturnValue({
      ...defaultDraftState,
      draft: {
        ...defaultDraftState.draft,
        edgeChanges: {
          additions: [
            {
              kind: "workstation-output",
              source: { kind: "workstation", name: "review" },
              target: {
                kind: "work-state",
                stateName: "blocked",
                workTypeName: "story",
              },
            },
          ],
          removals: [],
        },
      },
      hasChanges: true,
    } as never);

    try {
      renderCurrentActivity({ snapshot: idleSnapshot });

      fireEvent.click(screen.getByRole("button", { name: "Edit mode" }));

      await screen.findByRole("button", { name: "Save changes" });
      expect(screen.queryByText("Unsaved changes")).toBeNull();
      expect(screen.queryByText("Topology edits are blocked")).toBeNull();
      await expectRenderableCurrentActivityGraphEdges(idleSnapshot);

      expect(
        reactFlowErrorSpy.mock.calls.some(([firstArg]) =>
          String(firstArg).includes(
            "Couldn't create edge for source handle id",
          ),
        ),
      ).toBe(false);
    } finally {
      reactFlowErrorSpy.mockRestore();
    }
  });

  it("renders every graph place family through custom React Flow node types", async () => {
    renderCurrentActivity({ snapshot: semanticWorkflowDashboardSnapshot });

    expect(
      await screen.findByRole("region", { name: "Work graph viewport" }),
    ).toBeTruthy();
    await waitFor(() => {
      expect(
        document.querySelector(
          "[data-current-activity-node-type='workstation']",
        ),
      ).toBeTruthy();
    });

    expect(
      document.querySelector(
        "[data-current-activity-node-type='statePosition']",
      ),
    ).toBeTruthy();
    expect(
      document.querySelector("[data-current-activity-node-type='resource']"),
    ).toBeTruthy();
    expect(
      document.querySelector("[data-current-activity-node-type='worker']"),
    ).toBeTruthy();
    expect(
      document.querySelector("[data-current-activity-node-type='workType']"),
    ).toBeTruthy();
    expect(screen.queryByText("Workstation Definition")).toBeNull();
    expect(screen.queryByText("State Position")).toBeNull();
  });

  it("keeps zero-count resources visible and readable", async () => {
    const snapshot = dashboardSnapshotWithStateCounts({
      "agent-slot:available": 0,
    });
    renderCurrentActivity({ snapshot });

    const resourceCount = await screen.findByLabelText("0 resource tokens");
    const resourceNode = resourceCount.closest(".react-flow__node");
    const resourceArticle = resourceCount.closest("article");

    expect(resourceCount.textContent?.trim()).toBe("0");
    expect(screen.getByLabelText("agent-slot")).toBeTruthy();
    expect(
      resourceArticle?.querySelector("[data-resource-name]")?.textContent,
    ).toBe("agent-slot");
    expect(
      within(resourceArticle as HTMLElement)
        .getByRole("img", { name: "Resource" })
        .getAttribute("data-graph-semantic-icon"),
    ).toBe("resource");
    expect(resourceArticle?.textContent).not.toContain("Resource");
    expect(resourceNode?.getAttribute("style")).toContain("width: 168px");
    expect(resourceNode?.getAttribute("style")).toContain("height: 86px");
    expect(resourceArticle?.className).not.toContain("opacity-[0.45]");
  });

  it("renders resource, worker, and work-type role icons while preserving identifiers", async () => {
    renderCurrentActivity({ snapshot: semanticWorkflowDashboardSnapshot });

    const resourceLabelContainer = await screen.findByLabelText("agent-slot");
    const resourceArticle = resourceLabelContainer.closest("article");
    const workerLabelContainer = screen.getByLabelText("worker:agent");
    const workerArticle = workerLabelContainer.closest("article");
    const workTypeButton = screen.getByRole("button", {
      name: "Select story work type",
    });
    const workTypeArticle = workTypeButton.closest("article");

    expect(
      within(resourceArticle as HTMLElement)
        .getByRole("img", { name: "Resource" })
        .getAttribute("data-graph-semantic-icon"),
    ).toBe("resource");
    expect(
      within(workerArticle as HTMLElement)
        .getByRole("img", { name: "Worker" })
        .getAttribute("data-graph-semantic-icon"),
    ).toBe("worker");
    expect(
      workTypeArticle
        ?.querySelector("[data-graph-semantic-icon='work-type']")
        ?.getAttribute("data-graph-semantic-icon"),
    ).toBe("work-type");
    expect(resourceLabelContainer.getAttribute("aria-label")).toBe(
      "agent-slot",
    );
    expect(workerLabelContainer.getAttribute("aria-label")).toBe(
      "worker:agent",
    );
    expect(workTypeButton.getAttribute("aria-label")).toBe(
      "Select story work type",
    );
    expect(
      resourceArticle?.querySelector("[data-resource-name]")?.textContent,
    ).toBe("agent-slot");
    expect(resourceArticle?.textContent).not.toContain("Resource");
    expect(workerArticle?.textContent).toContain("agent");
    expect(workTypeArticle?.textContent).toContain("story");
  });

  it("renders selected-tick resource counts while active dispatches occupy and return slots", async () => {
    const idleSnapshot = resourceOccupancySnapshotForTick(1);

    expect(idleSnapshot.runtime.in_flight_dispatch_count).toBe(0);
    expect(
      idleSnapshot.runtime.place_token_counts?.["agent-slot:available"],
    ).toBe(2);

    renderCurrentActivity({ snapshot: idleSnapshot });

    const idleResourceCount = await screen.findByLabelText("2 resource tokens");

    expect(idleResourceCount.textContent?.trim()).toBe("2");
    expect(screen.getByLabelText("agent-slot")).toBeTruthy();

    cleanup();
    reinstallDashboardBrowserTestShims();

    const activeSnapshot = resourceOccupancySnapshotForTick(3);

    expect(activeSnapshot.runtime.in_flight_dispatch_count).toBe(1);
    expect(
      activeSnapshot.runtime.place_token_counts?.["agent-slot:available"],
    ).toBe(1);

    renderCurrentActivity({ snapshot: activeSnapshot });

    const activeResourceCount =
      await screen.findByLabelText("1 resource tokens");

    expect(activeResourceCount.textContent?.trim()).toBe("1");
    expect(screen.getByLabelText("agent-slot")).toBeTruthy();
    expect(screen.queryByLabelText("2 resource tokens")).toBeNull();

    cleanup();
    reinstallDashboardBrowserTestShims();

    const returnedSnapshot = resourceOccupancySnapshotForTick(4);

    expect(returnedSnapshot.runtime.in_flight_dispatch_count).toBe(0);
    expect(
      returnedSnapshot.runtime.place_token_counts?.["agent-slot:available"],
    ).toBe(2);

    renderCurrentActivity({ snapshot: returnedSnapshot });

    const returnedResourceCount =
      await screen.findByLabelText("2 resource tokens");

    expect(returnedResourceCount.textContent?.trim()).toBe("2");
    expect(screen.getByLabelText("agent-slot")).toBeTruthy();
    expect(screen.queryByLabelText("1 resource tokens")).toBeNull();
  });

  it("animates active graph flow while muting unrelated graph chrome", async () => {
    renderCurrentActivity({ snapshot: semanticWorkflowDashboardSnapshot });

    expect(
      await screen.findByRole("button", { name: /Active Story/ }),
    ).toBeTruthy();
    const idleStateArticle = await getStateNodeArticle("story:documented");
    const idleResourceArticle = screen
      .getByLabelText("agent-slot")
      .closest("article");
    expect(idleStateArticle.querySelector("article")?.className).toContain(
      "opacity-[0.45]",
    );
    expect(idleResourceArticle?.className).toContain("border-outline");
    expect(idleResourceArticle?.className).not.toContain("opacity-[0.45]");
  });

  it("keeps inactive and failed output paths unlabeled and out of active green flow", async () => {
    renderCurrentActivity({
      snapshot: dashboardSnapshotWithActiveImplementWorkstation(),
    });

    expect(
      await screen.findByRole("button", {
        name: "Select Implement workstation",
      }),
    ).toBeTruthy();
    expect(
      screen.getByRole("button", { name: "Select story:blocked state" }),
    ).toBeTruthy();
    expect(screen.queryByText(/Flowing/)).toBeNull();
    expect(screen.queryByText(/Failure Path/)).toBeNull();
  });

  it("hides workstation return edges to resource nodes while keeping resource inputs visible", async () => {
    renderCurrentActivity({
      snapshot: dashboardSnapshotWithResourceReturnEdge(),
    });

    expect(await screen.findByLabelText("agent-slot")).toBeTruthy();
    expect(
      screen.getByRole("button", { name: "Select Implement workstation" }),
    ).toBeTruthy();
    expect(screen.getAllByLabelText("agent-slot")).toHaveLength(1);
  });

  it("uses selected accent styling over active flow styling", async () => {
    renderCurrentActivity({
      snapshot: semanticWorkflowDashboardSnapshot,
      selection: { kind: "state-node", placeId: "story:complete" },
    });

    const activeSelectedState = await getStateNodeArticle("story:complete");
    const activeSelectedArticle = activeSelectedState.querySelector("article");

    expect(activeSelectedArticle?.className).toContain("border-primary");
    expect(activeSelectedArticle?.className).not.toContain(
      "shadow-af-success-chip",
    );

    cleanup();
    reinstallDashboardBrowserTestShims();

    renderCurrentActivity({
      snapshot: semanticWorkflowDashboardSnapshot,
      selection: { kind: "node", nodeId: "review" },
    });

    const reviewButton = await screen.findByRole("button", {
      name: "Select Review workstation",
    });
    const reviewArticle = reviewButton.closest("article");

    expect(reviewArticle?.className).toContain("border-primary");
    expect(reviewArticle?.className).not.toContain("agent-flow-node--active");
  });

  it("renders the legend minimized by default and expands it for graph node and edge semantics", async () => {
    renderCurrentActivity({ snapshot: semanticWorkflowDashboardSnapshot });

    const expandButton = await screen.findByRole("button", {
      name: "Expand graph legend",
    });

    expect(expandButton.getAttribute("aria-expanded")).toBe("false");
    expect(screen.queryByLabelText("Graph legend")).toBeNull();

    const legend = await expandGraphLegend();
    const legendScope = within(legend);
    const collapseButton = screen.getByRole("button", {
      name: "Collapse graph legend",
    });

    expect(legendScope.getByText("Active flow")).toBeTruthy();
    expect(legendScope.getByText("Failure path")).toBeTruthy();
    expect(legend.querySelector("[data-legend-flow]")).toBeTruthy();
    expect(collapseButton.getAttribute("aria-expanded")).toBe("true");
    for (const [label, kind] of LEGEND_ICON_EXPECTATIONS) {
      const iconEntry = legend.querySelector(`[data-legend-icon='${kind}']`);
      const icon = legendScope.getByRole("img", {
        name: `${label} legend icon`,
      });

      expect(iconEntry).toBeTruthy();
      expect(icon.getAttribute("data-graph-semantic-icon")).toBe(kind);
      expect(within(iconEntry as HTMLElement).getByText(label)).toBeTruthy();
    }
    expect(
      legend.querySelector("[data-legend-icon='queue'] span.h-3"),
    ).toBeNull();
    expect(
      legend.querySelector("[data-legend-icon='workstation'] span.border-2"),
    ).toBeNull();
    expect(
      legend.querySelector(
        "[data-legend-icon='exhaustion'] span.border-dashed",
      ),
    ).toBeNull();

    fireEvent.click(collapseButton);

    await waitFor(() => {
      expect(screen.queryByLabelText("Graph legend")).toBeNull();
    });
    expect(
      screen.getByRole("button", { name: "Expand graph legend" }),
    ).toBeTruthy();
  });

  it("does not render runtime-only exhaustion-rule topology nodes", async () => {
    renderCurrentActivity({
      snapshot: dashboardSnapshotWithExhaustionRuleNode(),
    });

    expect(
      await screen.findByRole("button", {
        name: "Select Review workstation",
      }),
    ).toBeTruthy();
    expect(
      screen.queryByRole("button", {
        name: "Select executor-loop-breaker exhaustion rule",
      }),
    ).toBeNull();
    expect(
      screen.queryByRole("button", { name: /Should Not Render/ }),
    ).toBeNull();
    expect(screen.queryByText("Should Not Render")).toBeNull();
  });
});
