import "@testing-library/jest-dom/vitest";
import "./react-flow-current-activity-card-component.mocks";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  fireEvent,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import type { CanonicalFactoryDefinition } from "../../../../api/current-factory-definition";
import type { DashboardSnapshot } from "../../../../api/dashboard/types";
import {
  semanticWorkflowDashboardSnapshot,
  workstationKindParityDashboardSnapshot,
  workstationKindParityExpectations,
} from "../../../../components/dashboard/test-fixtures";
import { DashboardSessionTestProvider } from "../../../../testing/dashboard-session-test-provider";
import {
  SYSTEM_TIME_EXPIRY_TRANSITION_ID,
  SYSTEM_TIME_WORK_TYPE_ID,
  systemTimeGraphNodeId,
} from "../../../factory-graph-editor/lib/operations/factory-graph-customer-display";
import {
  buildCurrentActivityGraphLayoutFromFactory,
  dashboardWorkstationFromFactory,
} from "../../lib/current-activity-factory-graph-layout";
import { currentActivityGraphKey } from "../../lib/react-flow-current-activity-card-keys";
import { ReactFlowCurrentActivityCard } from "../react-flow-current-activity-card";
import {
  dashboardSnapshotWithActiveWorkItemCount,
  dashboardSnapshotWithStateCounts,
  expandGraphLegend,
  getStateNodeArticle,
  refreshFactoryFromTopology,
  registerCurrentActivityCardTestLifecycle,
  renderCurrentActivity,
  renderWithQueryClient,
} from "./react-flow-current-activity-card-component.harness";

async function getWorkstationNode(label = "Review"): Promise<HTMLElement> {
  const button = await screen.findByRole("button", {
    name: `Select ${label} workstation`,
  });
  return button.closest(".react-flow__node") as HTMLElement;
}

function expectFixedWorkstationNodeDimensions(node: Element | null) {
  expect(node?.getAttribute("style")).toContain("width: 156px");
  expect(node?.getAttribute("style")).toContain("height: 196px");
}

function dashboardSnapshotWithLongStateLabels(): DashboardSnapshot {
  const snapshot = structuredClone(semanticWorkflowDashboardSnapshot);

  for (const workstation of Object.values(
    snapshot.topology.workstation_nodes_by_id,
  )) {
    for (const place of [
      ...(workstation.input_places ?? []),
      ...(workstation.output_places ?? []),
    ]) {
      if (place.place_id === "story:ready") {
        place.type_id =
          "customer-escalation-story-with-a-deliberately-long-type";
        place.state_value =
          "ready-for-review-after-multiple-dependent-checks-complete";
      }
    }
  }

  return refreshFactoryFromTopology(snapshot);
}

function cronSystemTimeFactoryDefinition(): CanonicalFactoryDefinition {
  return {
    name: "cron-system-time-card",
    workers: [
      {
        command: "./cron.sh",
        name: "scheduler",
        type: "SCRIPT_WORKER",
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
          { name: "new", type: "INITIAL" },
          { name: "scheduled", type: "PROCESSING" },
          { name: "done", type: "TERMINAL" },
        ],
      },
      {
        name: "schedule",
        states: [{ name: "tick", type: "INITIAL" }],
      },
      {
        name: SYSTEM_TIME_WORK_TYPE_ID,
        states: [{ name: "pending", type: "PROCESSING" }],
      },
    ],
    workstations: [
      {
        behavior: "CRON",
        cron: { schedule: "0 0 * * *" },
        id: "nightly-cron",
        inputs: [{ state: "tick", workType: "schedule" }],
        name: "Nightly Cron",
        outputs: [{ state: "scheduled", workType: "story" }],
        worker: "scheduler",
      },
      {
        behavior: "STANDARD",
        id: "review",
        inputs: [{ state: "scheduled", workType: "story" }],
        name: "Review",
        outputs: [{ state: "done", workType: "story" }],
        type: "MODEL_WORKSTATION",
        worker: "reviewer",
      },
      {
        id: SYSTEM_TIME_EXPIRY_TRANSITION_ID,
        inputs: [{ state: "pending", workType: SYSTEM_TIME_WORK_TYPE_ID }],
        name: SYSTEM_TIME_EXPIRY_TRANSITION_ID,
        outputs: [],
        worker: "",
      },
    ],
  } satisfies CanonicalFactoryDefinition;
}

function dashboardSnapshotWithCronSystemTimeFactory(): DashboardSnapshot {
  const factory = cronSystemTimeFactoryDefinition();
  const workstations = (factory.workstations ?? []).map(
    dashboardWorkstationFromFactory,
  );

  return {
    factory,
    factory_state: "IDLE",
    runtime: {
      active_executions_by_dispatch_id: {},
      current_work_items_by_place_id: {},
      place_occupancy_work_items_by_place_id: {},
      place_token_counts: {
        "schedule:tick": 1,
        "story:scheduled": 1,
      },
      session: {
        completed_count: 0,
        dispatched_count: 0,
        failed_count: 0,
        has_data: true,
        provider_sessions: [],
      },
      workstation_requests_by_dispatch_id: {},
    },
    tick_count: 0,
    topology: {
      edges: [],
      workstation_node_ids: workstations.map(
        (workstation) => workstation.node_id,
      ),
      workstation_nodes_by_id: Object.fromEntries(
        workstations.map((workstation) => [workstation.node_id, workstation]),
      ),
    },
    uptime_seconds: 0,
  };
}

describe("ReactFlowCurrentActivityCard node layout behavior", () => {
  registerCurrentActivityCardTestLifecycle();

  it("renders the shared workstation-kind parity fixture with distinct supported icons", async () => {
    const { onSelectWorkstation } = renderCurrentActivity({
      snapshot: workstationKindParityDashboardSnapshot,
    });

    const legend = await expandGraphLegend();
    const legendScope = within(legend);

    for (const expectation of workstationKindParityExpectations) {
      const button = await screen.findByRole("button", {
        name: expectation.buttonName,
      });
      const icon = within(button).getByRole("img", {
        name: expectation.metadata.label,
      });
      const legendIcon = legendScope.getByRole("img", {
        name: `${expectation.metadata.label} legend icon`,
      });

      expect(icon.getAttribute("data-graph-semantic-icon")).toBe(
        expectation.metadata.iconKind,
      );
      expect(button.textContent).toContain(expectation.workstationName);
      expect(button.textContent).not.toContain(expectation.metadata.label);
      expect(legendIcon.getAttribute("data-graph-semantic-icon")).toBe(
        expectation.metadata.iconKind,
      );
      expect(legendScope.getByText(expectation.metadata.label)).toBeTruthy();
    }

    const cronExpectation = workstationKindParityExpectations.find(
      (expectation) => expectation.nodeID === "nightly-cron",
    );
    const cronButton = await screen.findByRole("button", {
      name: cronExpectation?.buttonName ?? "Select Nightly Cron workstation",
    });

    expect(cronButton.getAttribute("title")).toBe("Nightly Cron");
    expect(
      cronButton.closest("article")?.className.includes("border-dashed"),
    ).toBe(true);

    const pollerExpectation = workstationKindParityExpectations.find(
      (expectation) => expectation.nodeID === "linear-poller",
    );
    const pollerButton = await screen.findByRole("button", {
      name: pollerExpectation?.buttonName ?? "Select Linear Poller workstation",
    });

    expect(
      pollerButton.closest("article")?.className.includes("border-dotted"),
    ).toBe(true);

    fireEvent.click(cronButton);

    expect(onSelectWorkstation).toHaveBeenCalledWith("nightly-cron");
  });

  it("hides system-time graph nodes from cron factories while keeping customer cron workstations selectable", async () => {
    const factory = cronSystemTimeFactoryDefinition();
    const layout = await buildCurrentActivityGraphLayoutFromFactory(factory);
    const renderedNodeIds = layout.nodes.map((node) => node.nodeId);

    expect(renderedNodeIds).not.toEqual(
      expect.arrayContaining([
        systemTimeGraphNodeId("work-type", SYSTEM_TIME_WORK_TYPE_ID),
        systemTimeGraphNodeId(
          "work-state",
          SYSTEM_TIME_WORK_TYPE_ID,
          "pending",
        ),
        systemTimeGraphNodeId("workstation", SYSTEM_TIME_EXPIRY_TRANSITION_ID),
      ]),
    );
    expect(renderedNodeIds).toEqual(
      expect.arrayContaining([
        systemTimeGraphNodeId("workstation", "nightly-cron", "Nightly Cron"),
        systemTimeGraphNodeId("work-type", "story"),
      ]),
    );

    const { onSelectWorkstation } = renderCurrentActivity({
      snapshot: dashboardSnapshotWithCronSystemTimeFactory(),
    });

    const cronButton = await screen.findByRole("button", {
      name: "Select Nightly Cron workstation",
    });

    expect(cronButton.getAttribute("title")).toBe("Nightly Cron");
    expect(
      screen.queryByLabelText(`work-type:${SYSTEM_TIME_WORK_TYPE_ID}`),
    ).toBeNull();
    expect(
      screen.queryByRole("button", {
        name: `Select ${SYSTEM_TIME_EXPIRY_TRANSITION_ID} workstation`,
      }),
    ).toBeNull();
    expect(
      screen.queryByRole("button", {
        name: `Select ${SYSTEM_TIME_WORK_TYPE_ID}:pending state`,
      }),
    ).toBeNull();
    expect(screen.queryByText(SYSTEM_TIME_EXPIRY_TRANSITION_ID)).toBeNull();

    fireEvent.click(cronButton);

    expect(onSelectWorkstation).toHaveBeenCalledWith("nightly-cron");
  });

  it("renders state category icons without replacing state labels", async () => {
    renderCurrentActivity({ snapshot: semanticWorkflowDashboardSnapshot });

    const initialStateArticle = await getStateNodeArticle("story:init");
    const processingStateArticle = await getStateNodeArticle("story:ready");
    const terminalStateArticle = await getStateNodeArticle("story:complete");
    const failedStateArticle = await getStateNodeArticle("story:blocked");

    expect(
      within(initialStateArticle)
        .getByRole("img", { name: "Queue" })
        .getAttribute("data-graph-semantic-icon"),
    ).toBe("queue");
    expect(
      within(processingStateArticle)
        .getByRole("img", { name: "Processing state" })
        .getAttribute("data-graph-semantic-icon"),
    ).toBe("processing");
    expect(
      within(terminalStateArticle)
        .getByRole("img", { name: "Terminal" })
        .getAttribute("data-graph-semantic-icon"),
    ).toBe("terminal");
    expect(
      within(failedStateArticle)
        .getByRole("img", { name: "Failed" })
        .getAttribute("data-graph-semantic-icon"),
    ).toBe("failed");
    expect(
      initialStateArticle.querySelector("[data-state-work-type]")?.textContent,
    ).toBe("story");
    expect(
      initialStateArticle.querySelector("[data-state-value]")?.textContent,
    ).toBe("init");
    expect(
      processingStateArticle.querySelector("[data-state-value]")?.textContent,
    ).toBe("ready");
    expect(
      terminalStateArticle.querySelector("[data-state-value]")?.textContent,
    ).toBe("complete");
    expect(
      failedStateArticle.querySelector("[data-state-value]")?.textContent,
    ).toBe("blocked");
    expect(
      initialStateArticle.querySelector("article")?.textContent,
    ).not.toContain("Queue");
    expect(
      terminalStateArticle.querySelector("article")?.textContent,
    ).not.toContain("Terminal");
    expect(
      failedStateArticle.querySelector("article")?.textContent,
    ).not.toContain("Failed");
    expect(
      failedStateArticle.querySelector("article")?.textContent,
    ).not.toContain("Queue");
  });

  it("themes the React Flow controls with dashboard colors", async () => {
    renderCurrentActivity({ snapshot: semanticWorkflowDashboardSnapshot });

    expect(
      await screen.findByRole("region", { name: "Work graph viewport" }),
    ).toBeTruthy();
    const controls = document.querySelector<HTMLElement>(
      ".react-flow__controls",
    );
    const zoomIn = controls?.querySelector<HTMLButtonElement>(
      ".react-flow__controls-zoomin",
    );

    expect(controls?.getAttribute("style")).toContain(
      "--xy-controls-button-background-color-props: var(--color-surface-container-high)",
    );
    expect(controls?.getAttribute("style")).toContain(
      "--xy-controls-button-color-props: var(--color-on-surface-variant)",
    );
    expect(controls?.getAttribute("style")).toContain(
      "--xy-controls-box-shadow: none",
    );
    expect(controls?.getAttribute("style")).not.toContain("#fefefe");
    expect(zoomIn).toBeTruthy();
  });

  it("renders state-position markers as green dots for low-count active states", async () => {
    const snapshot = dashboardSnapshotWithStateCounts({ "story:ready": 3 });
    renderCurrentActivity({ snapshot });
    const readyStateArticle = await getStateNodeArticle("story:ready");
    const dotContainer = readyStateArticle.querySelector(
      "[data-state-work-progress='dots']",
    );
    const dots = readyStateArticle.querySelectorAll(
      "[data-state-work-progress-dot]",
    );
    const dotIndices = Array.from(dots).map((dot) =>
      dot.getAttribute("data-state-work-progress-dot"),
    );

    expect(dotContainer).toBeTruthy();
    expect(dotIndices).toEqual(["0", "1", "2"]);
    expect(dotContainer?.getAttribute("aria-label")).toBe("3 active items");
    expect(
      within(readyStateArticle).getByLabelText("3 active items"),
    ).toBeTruthy();
  });

  it("renders work-state labels and markers in separated stable zones", async () => {
    const snapshot = dashboardSnapshotWithStateCounts({ "story:ready": 3 });
    renderCurrentActivity({ snapshot });
    const readyStateArticle = await getStateNodeArticle("story:ready");
    const labelZone = readyStateArticle.querySelector(
      "[data-state-label-zone]",
    );
    const markerZone = readyStateArticle.querySelector(
      "[data-state-marker-zone]",
    );
    const workType = readyStateArticle.querySelector("[data-state-work-type]");
    const stateValue = readyStateArticle.querySelector("[data-state-value]");

    expect(labelZone).toBeTruthy();
    expect(markerZone).toBeTruthy();
    expect(workType?.textContent).toBe("story");
    expect(stateValue?.textContent).toBe("ready");
    expect(labelZone?.textContent).not.toContain(":");
    expect(labelZone?.textContent).not.toContain("3 active items");
    expect(within(readyStateArticle).queryByText("story:ready")).toBeNull();
    expect(markerZone?.textContent).not.toContain("story");
    expect(
      markerZone?.querySelectorAll("[data-state-work-progress-dot]"),
    ).toHaveLength(3);
  });

  it("renders exactly 10 state-position markers in a compact ordered grid", async () => {
    const snapshot = dashboardSnapshotWithStateCounts({ "story:ready": 10 });
    renderCurrentActivity({ snapshot });
    const readyStateArticle = await getStateNodeArticle("story:ready");
    const dotContainer = readyStateArticle.querySelector(
      "[data-state-work-progress='dots']",
    );
    const dotIndices = Array.from(
      readyStateArticle.querySelectorAll("[data-state-work-progress-dot]"),
    ).map((dot) => dot.getAttribute("data-state-work-progress-dot"));

    expect(dotContainer?.className).toContain("grid-cols-[repeat(5,0.5rem)]");
    expect(dotIndices).toEqual([
      "0",
      "1",
      "2",
      "3",
      "4",
      "5",
      "6",
      "7",
      "8",
      "9",
    ]);
  });

  it("uses numeric fallback for state-position active counts above 10", async () => {
    const snapshot = dashboardSnapshotWithStateCounts({ "story:ready": 11 });
    renderCurrentActivity({ snapshot });
    const readyStateArticle = await getStateNodeArticle("story:ready");
    const numeric = readyStateArticle.querySelector(
      "[data-state-work-progress='numeric']",
    );

    expect(numeric).toBeTruthy();
    expect(numeric?.textContent?.trim()).toBe("11");
    expect(numeric?.getAttribute("aria-label")).toBe("11 active items");
    expect(
      readyStateArticle.querySelector("[data-state-work-progress='dots']"),
    ).toBeNull();
  });

  it("keeps long work-state labels bounded inside the state label zone", async () => {
    renderCurrentActivity({ snapshot: dashboardSnapshotWithLongStateLabels() });
    const longStateButton = await screen.findByRole("button", {
      name: "Select customer-escalation-story-with-a-deliberately-long-type:ready-for-review-after-multiple-dependent-checks-complete state",
    });
    const longStateNode = longStateButton.closest(".react-flow__node");
    const labelZone = longStateNode?.querySelector("[data-state-label-zone]");
    const workType = longStateNode?.querySelector("[data-state-work-type]");
    const stateValue = longStateNode?.querySelector("[data-state-value]");
    const markerZone = longStateNode?.querySelector("[data-state-marker-zone]");

    expect(longStateNode?.getAttribute("style")).toContain("width: 164px");
    expect(longStateNode?.getAttribute("style")).toContain("height: 86px");
    expect(longStateButton.className).toContain("flex-col");
    expect(longStateButton.className).toContain("overflow-hidden");
    expect(labelZone?.className).toContain("h-6");
    expect(labelZone?.className).toContain("max-h-6");
    expect(labelZone?.className).toContain("overflow-hidden");
    expect(workType?.className).toContain("text-ellipsis");
    expect(workType?.getAttribute("title")).toBe(
      "customer-escalation-story-with-a-deliberately-long-type",
    );
    expect(stateValue?.className).toContain("overflow-hidden");
    expect(stateValue?.className).toContain("truncate");
    expect(stateValue?.className).toContain("whitespace-nowrap");
    expect(stateValue?.getAttribute("title")).toBe(
      "ready-for-review-after-multiple-dependent-checks-complete",
    );
    expect(markerZone).toBeTruthy();
    expect(markerZone?.className).toContain("shrink-0");
    expect(markerZone?.getAttribute("title")).toBe(
      "customer-escalation-story-with-a-deliberately-long-type:ready-for-review-after-multiple-dependent-checks-complete",
    );
  });

  it("applies green in-progress state styling only when active items are > 0", async () => {
    const snapshot = dashboardSnapshotWithStateCounts({
      "story:ready": 4,
      "story:documented": 0,
    });
    renderCurrentActivity({ snapshot });
    const readyStateArticle = await getStateNodeArticle("story:ready");
    const documentedStateArticle =
      await getStateNodeArticle("story:documented");

    expect(readyStateArticle.querySelector("article")?.className).toContain(
      "border-af-success-border",
    );
    expect(
      documentedStateArticle.querySelector("article")?.className,
    ).not.toContain("border-af-success-border");
    expect(
      within(readyStateArticle).getByRole("status", {
        name: "4 active items",
      }),
    ).toBeTruthy();
    expect(
      within(documentedStateArticle).queryByRole("status", {
        name: /active items/,
      }),
    ).toBeNull();
  });

  it("selects workstation and work item context through the dashboard callbacks", async () => {
    const { onSelectWorkID, onSelectWorkstation } = renderCurrentActivity({
      snapshot: semanticWorkflowDashboardSnapshot,
    });

    fireEvent.click(
      await screen.findByRole("button", { name: "Select Review workstation" }),
    );

    expect(onSelectWorkstation).toHaveBeenCalledWith("review");

    fireEvent.click(
      (await screen.findAllByRole("button", { name: /Active Story/ }))[0],
    );

    await waitFor(() => {
      expect(onSelectWorkID).toHaveBeenCalled();
    });
    expect(onSelectWorkID).toHaveBeenCalledWith("work-active-story", {
      dispatchID: "dispatch-review-active",
      nodeID: "review",
    });
  });

  it("caps workstation work item names at three and summarizes the rest", async () => {
    renderCurrentActivity({
      snapshot: dashboardSnapshotWithActiveWorkItemCount(5),
    });

    expect(
      await screen.findByRole("button", { name: /Active Story 1/ }),
    ).toBeTruthy();
    expect(screen.getByRole("button", { name: /Active Story 3/ })).toBeTruthy();
    expect(screen.queryByRole("button", { name: /Active Story 4/ })).toBeNull();
    expect(screen.queryByRole("button", { name: /Active Story 5/ })).toBeNull();
    expect(screen.getByLabelText("5 active items")).toBeTruthy();
    expect(screen.getAllByText("+2")).toHaveLength(1);
  });

  it("keeps workstation height stable while summarizing more than three active items", async () => {
    const { onSelectWorkstation } = renderCurrentActivity({
      snapshot: dashboardSnapshotWithActiveWorkItemCount(6),
    });

    const reviewButton = await screen.findByRole("button", {
      name: "Select Review workstation",
    });
    const reviewNode = reviewButton.closest(".react-flow__node");
    const dots = reviewNode?.querySelector(
      "[data-workstation-work-progress='dots']",
    );

    expect(dots).toBeTruthy();
    expect(dots?.getAttribute("aria-label")).toBe("6 active items");
    expectFixedWorkstationNodeDimensions(reviewNode);
    expect(screen.getByRole("button", { name: /Active Story 1/ })).toBeTruthy();
    expect(screen.getByRole("button", { name: /Active Story 3/ })).toBeTruthy();
    expect(screen.queryByRole("button", { name: /Active Story 4/ })).toBeNull();
    expect(reviewNode?.querySelector("article")?.className).toContain(
      "border-af-success-border",
    );

    fireEvent.click(reviewButton);

    expect(onSelectWorkstation).toHaveBeenCalledWith("review");
  });

  it("keeps workstation node dimensions fixed across zero, one, five, and six active items", async () => {
    const { rerender } = renderWithQueryClient(
      <ReactFlowCurrentActivityCard
        now={Date.parse("2026-04-08T12:00:04Z")}
        onSelectWorkID={vi.fn()}
        selection={null}
        snapshot={dashboardSnapshotWithActiveWorkItemCount(0)}
        onSelectStateNode={vi.fn()}
        onSelectResource={vi.fn()}
        onSelectWorker={vi.fn()}
        onSelectWorkType={vi.fn()}
        onSelectWorkstation={vi.fn()}
      />,
    );

    for (const activeItemCount of [0, 1, 5, 6]) {
      rerender(
        <QueryClientProvider
          client={
            new QueryClient({
              defaultOptions: {
                mutations: { retry: false },
                queries: { gcTime: Infinity, retry: false },
              },
            })
          }
        >
          <DashboardSessionTestProvider>
            <ReactFlowCurrentActivityCard
              now={Date.parse("2026-04-08T12:00:04Z")}
              onSelectWorkID={vi.fn()}
              selection={null}
              snapshot={dashboardSnapshotWithActiveWorkItemCount(
                activeItemCount,
              )}
              onSelectStateNode={vi.fn()}
              onSelectResource={vi.fn()}
              onSelectWorker={vi.fn()}
              onSelectWorkType={vi.fn()}
              onSelectWorkstation={vi.fn()}
            />
          </DashboardSessionTestProvider>
        </QueryClientProvider>,
      );

      const reviewNode = await getWorkstationNode();

      expectFixedWorkstationNodeDimensions(reviewNode);
    }
  });

  it("keeps workstation position keys stable when selected ticks change active work counts", async () => {
    const zeroActiveSnapshot = dashboardSnapshotWithActiveWorkItemCount(0);
    const sixActiveSnapshot = dashboardSnapshotWithActiveWorkItemCount(6);
    if (!zeroActiveSnapshot.factory || !sixActiveSnapshot.factory) {
      throw new Error("expected active-count fixtures to include factories");
    }
    const zeroActiveLayout = await buildCurrentActivityGraphLayoutFromFactory(
      zeroActiveSnapshot.factory,
    );
    const sixActiveLayout = await buildCurrentActivityGraphLayoutFromFactory(
      sixActiveSnapshot.factory,
    );
    const graphKey = currentActivityGraphKey(zeroActiveLayout);

    expect(currentActivityGraphKey(sixActiveLayout)).toBe(graphKey);

    const callbacks = {
      onSelectWorkID: vi.fn(),
      onSelectStateNode: vi.fn(),
      onSelectDoc: vi.fn(),
      onSelectResource: vi.fn(),
      onSelectWorker: vi.fn(),
      onSelectWorkType: vi.fn(),
      onSelectWorkstation: vi.fn(),
    };
    const { rerender } = renderWithQueryClient(
      <ReactFlowCurrentActivityCard
        now={Date.parse("2026-04-08T12:00:04Z")}
        selection={null}
        snapshot={zeroActiveSnapshot}
        {...callbacks}
      />,
    );

    let reviewNode = await getWorkstationNode();
    expectFixedWorkstationNodeDimensions(reviewNode);

    rerender(
      <QueryClientProvider
        client={
          new QueryClient({
            defaultOptions: {
              mutations: { retry: false },
              queries: { gcTime: Infinity, retry: false },
            },
          })
        }
      >
        <DashboardSessionTestProvider>
          <ReactFlowCurrentActivityCard
            now={Date.parse("2026-04-08T12:00:04Z")}
            selection={null}
            snapshot={sixActiveSnapshot}
            {...callbacks}
          />
        </DashboardSessionTestProvider>
      </QueryClientProvider>,
    );

    reviewNode = await getWorkstationNode();
    expectFixedWorkstationNodeDimensions(reviewNode);
  });
});
