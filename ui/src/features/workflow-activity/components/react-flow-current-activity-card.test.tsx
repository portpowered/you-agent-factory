import "@testing-library/jest-dom/vitest";
import "./react-flow-current-activity-card-component.mocks.test";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  fireEvent,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import type { CanonicalFactoryDefinition } from "../../../api/current-factory-definition";
import type {
  DashboardSnapshot,
  DashboardWorkItemRef,
} from "../../../api/dashboard/types";
import {
  type ImportFactoryValue,
  SessionFactoryAPIError,
} from "../../../api/session-factory";
import {
  semanticWorkflowDashboardSnapshot,
  singleNodeDashboardSnapshot,
  twentyNodeDashboardSnapshot,
  workstationKindParityDashboardSnapshot,
  workstationKindParityExpectations,
} from "../../../components/dashboard/test-fixtures";
import { DashboardSessionTestProvider } from "../../../testing/dashboard-session-test-provider";
import {
  SYSTEM_TIME_EXPIRY_TRANSITION_ID,
  SYSTEM_TIME_WORK_TYPE_ID,
  systemTimeGraphNodeId,
} from "../../factory-graph-editor/lib/operations/factory-graph-customer-display";
import type { ReadFactoryImportFile } from "../../import/hooks/use-factory-png-drop";
import type { FactoryImportConfirmInput } from "../../import/lib/factory-import-save-choice";
import { getImportPreviewDialogMessages } from "../../import/messages/import-preview-dialog";
import {
  buildCurrentActivityGraphLayoutFromFactory,
  dashboardWorkstationFromFactory,
} from "../lib/current-activity-factory-graph-layout";
import {
  currentActivityGraphKey,
  currentActivityTopologyKey,
} from "../lib/react-flow-current-activity-card-keys";
import { getDashboardFlowAxisLegendMessages } from "../messages/dashboard-flow-axis-legend";
import { getWorkflowActivityGraphImportMessages } from "../messages/graph-import";
import { ReactFlowCurrentActivityCard } from "./react-flow-current-activity-card";
import {
  createFactoryImportValue,
  createFileDropTransfer,
  dashboardSnapshotWithActiveWorkItemCount,
  dashboardSnapshotWithStateCounts,
  expandGraphLegend,
  getStateNodeArticle,
  refreshFactoryFromTopology,
  registerCurrentActivityCardTestLifecycle,
  renderCurrentActivity,
  renderWithQueryClient,
} from "./react-flow-current-activity-card-component.harness.test";

const workflowGraphLocaleFallbackTimeoutMs = 180_000;

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

function dashboardSnapshotWithLongWorkstationAndActiveWorkLabels(): DashboardSnapshot {
  return dashboardSnapshotWithActiveWorkLabels(
    [
      "Short Active Story",
      "Active Story With A Medium Sized Label",
      "Active Story With A Deliberately Long Label That Must Stay Inside The Workstation Node",
    ],
    "Review Requests With A Deliberately Long Workstation Title",
  );
}

function dashboardSnapshotWithActiveWorkLabels(
  labels: string[],
  workstationName?: string,
): DashboardSnapshot {
  const snapshot = structuredClone(semanticWorkflowDashboardSnapshot);
  const activeExecution =
    snapshot.runtime.active_executions_by_dispatch_id?.[
      "dispatch-review-active"
    ];
  const reviewWorkstation = snapshot.topology.workstation_nodes_by_id.review;

  if (reviewWorkstation && workstationName) {
    reviewWorkstation.workstation_name = workstationName;
  }

  if (activeExecution) {
    activeExecution.work_items = labels.map(
      (label, index): DashboardWorkItemRef => {
        const itemNumber = index + 1;

        return {
          display_name: label,
          trace_id: `trace-active-story-${itemNumber}`,
          work_id: `work-active-story-${itemNumber}`,
          work_type_id: "story",
        };
      },
    );
    activeExecution.trace_ids = activeExecution.work_items.map(
      (workItem) => workItem.trace_id ?? workItem.work_id,
    );
  }

  return refreshFactoryFromTopology(snapshot);
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

describe("ReactFlowCurrentActivityCard topology selection and localization", () => {
  registerCurrentActivityCardTestLifecycle();

  it("derives a stable topology cache key for equivalent cloned workflow topology", () => {
    const firstKey = currentActivityTopologyKey(
      semanticWorkflowDashboardSnapshot.topology,
    );
    const secondKey = currentActivityTopologyKey(
      structuredClone(semanticWorkflowDashboardSnapshot).topology,
    );

    expect(secondKey).toBe(firstKey);
  });

  it("selects work-state nodes and exposes resource nodes as resource selectors", async () => {
    const { onSelectResource, onSelectStateNode } = renderCurrentActivity({
      snapshot: structuredClone(semanticWorkflowDashboardSnapshot),
      selection: { kind: "state-node", placeId: "story:implemented" },
    });

    const stateButton = await screen.findByRole("button", {
      name: "Select story:implemented state",
    });

    expect(stateButton.getAttribute("aria-pressed")).toBe("true");
    expect(stateButton.getAttribute("data-selected-state")).toBe("true");
    expect(
      screen.queryByRole("button", {
        name: "Select agent-slot state",
      }),
    ).toBeNull();
    expect(
      screen.getByRole("button", { name: "Select agent-slot resource" }),
    ).toBeTruthy();

    fireEvent.click(
      await screen.findByRole("button", { name: "Select story:ready state" }),
    );

    expect(onSelectStateNode).toHaveBeenCalledWith("story:ready");

    fireEvent.click(
      screen.getByRole("button", { name: "Select agent-slot resource" }),
    );

    expect(onSelectResource).toHaveBeenCalledWith("agent-slot");
  });

  it("keeps long workstation and active work labels from hiding the duration", async () => {
    const labels = [
      "Short Active Story",
      "Active Story With A Medium Sized Label",
      "Active Story With A Deliberately Long Label That Must Stay Inside The Workstation Node",
    ];
    const { onSelectWorkID } = renderCurrentActivity({
      snapshot: dashboardSnapshotWithLongWorkstationAndActiveWorkLabels(),
    });
    const longWorkstationButton = await screen.findByRole("button", {
      name: /Select Review Requests With A Deliberately Long Workstation Title workstation/,
    });
    const longWorkstationLabel = longWorkstationButton.querySelector(
      "[data-workstation-title]",
    );
    const longWorkButton = await screen.findByRole("button", {
      name: /Active Story With A Deliberately Long Label/,
    });
    const longWorkLabel = longWorkButton.querySelector(
      "[data-active-work-label]",
    );
    const durationLabel = longWorkButton.querySelector(
      "[data-active-work-duration]",
    );
    const reviewNode = longWorkButton.closest(".react-flow__node");

    expect(reviewNode?.getAttribute("style")).toContain("width: 156px");
    expect(longWorkstationButton.getAttribute("title")).toBe(
      "Review Requests With A Deliberately Long Workstation Title",
    );
    expect(longWorkstationLabel?.className).toContain("truncate");
    expect(longWorkstationLabel?.className).toContain("whitespace-nowrap");
    expect(longWorkButton.className).toContain("min-w-0");
    expect(longWorkButton.className).toContain(
      "grid-cols-[minmax(0,1fr)_auto]",
    );
    expect(longWorkButton.className).toContain("overflow-hidden");
    expect(longWorkLabel?.className).toContain("truncate");
    expect(longWorkLabel?.className).toContain("basis-0");
    expect(durationLabel?.textContent).toBe("4s");
    expect(durationLabel?.className).toContain("whitespace-nowrap");
    expect(durationLabel?.className).toContain("text-right");
    expect(durationLabel?.className).not.toContain("overflow-hidden");
    labels.forEach((label) => {
      const activeWorkButton = screen.getByRole("button", {
        name: new RegExp(label),
      });
      const labelElement = activeWorkButton.querySelector(
        "[data-active-work-label]",
      );
      const durationElement = activeWorkButton.querySelector(
        "[data-active-work-duration]",
      );

      expect(labelElement?.textContent).toBe(label);
      expect(labelElement?.className).toContain("min-w-0");
      expect(labelElement?.className).toContain("truncate");
      expect(durationElement?.textContent).toBe("4s");
      expect(durationElement?.className).toContain("shrink-0");
      expect(durationElement?.className).toContain("whitespace-nowrap");
    });
    expect(longWorkButton.getAttribute("aria-pressed")).toBe("false");

    fireEvent.click(longWorkButton);

    await waitFor(() => {
      expect(onSelectWorkID).toHaveBeenCalled();
    });
    expect(onSelectWorkID).toHaveBeenCalledWith("work-active-story-3", {
      dispatchID: "dispatch-review-active",
      nodeID: "review",
    });

    cleanup();
    renderCurrentActivity({
      snapshot: dashboardSnapshotWithLongWorkstationAndActiveWorkLabels(),
      selection: {
        dispatchId: "dispatch-review-active",
        kind: "work-item",
        nodeId: "review",
        workID: "work-active-story-3",
      },
    });

    expect(
      (
        await screen.findByRole("button", {
          name: /Active Story With A Deliberately Long Label/,
        })
      ).getAttribute("aria-pressed"),
    ).toBe("true");
  });

  it("renders a safe fallback label when an active work item is missing both display name and work id", async () => {
    const snapshot = structuredClone(semanticWorkflowDashboardSnapshot);
    const activeExecution =
      snapshot.runtime.active_executions_by_dispatch_id?.[
        "dispatch-review-active"
      ];

    if (!activeExecution) {
      throw new Error(
        "expected semantic workflow fixture to include an active review execution",
      );
    }

    activeExecution.work_items = [
      {
        trace_id: "trace-malformed-active-story",
        work_type_id: "story",
      } as DashboardWorkItemRef,
    ];
    activeExecution.trace_ids = ["trace-malformed-active-story"];

    renderCurrentActivity({ snapshot });

    expect(
      await screen.findByRole("button", { name: "Select Review workstation" }),
    ).toBeTruthy();
    expect(screen.getByRole("button", { name: /Unknown work/ })).toBeTruthy();
  });

  it("renders a single-node workflow without edge data", async () => {
    renderCurrentActivity({ snapshot: singleNodeDashboardSnapshot });

    expect(
      await screen.findByRole("button", { name: "Select Intake workstation" }),
    ).toBeTruthy();
    expect(screen.queryByText("Idle")).toBeNull();
  });

  it("renders a twenty-node workflow fixture for larger graphs", async () => {
    const legendMessages = getDashboardFlowAxisLegendMessages("en");

    renderCurrentActivity({ snapshot: twentyNodeDashboardSnapshot });

    expect(
      await screen.findByRole("button", {
        name: "Select Station 20 workstation",
      }),
    ).toBeTruthy();
    expect(
      screen.getAllByRole("button", { name: /Select .* workstation/ }),
    ).toHaveLength(20);
    expect(
      screen.getAllByRole("img", { name: "Standard workstation" }).length,
    ).toBeGreaterThan(0);
    expect(
      screen
        .getAllByRole("img", { name: "Queue" })[0]
        .getAttribute("data-graph-semantic-icon"),
    ).toBe("queue");
    const _legend = await expandGraphLegend();
    expect(
      within(screen.getByLabelText(legendMessages.title))
        .getByRole("img", {
          name: legendMessages.iconLabel(legendMessages.iconLabels.workstation),
        })
        .getAttribute("data-graph-semantic-icon"),
    ).toBe("workstation");
    expect(screen.queryByText("Workstation Definition")).toBeNull();
    expect(
      screen.getByRole("button", { name: "Select story:step-6 state" }),
    ).toBeTruthy();
  });

  it("renders localized legend copy for the workflow graph without changing the graph interactions", async () => {
    const messages = getDashboardFlowAxisLegendMessages("ja");

    renderCurrentActivity({
      snapshot: semanticWorkflowDashboardSnapshot,
      locale: "ja",
    });

    const expandButton = await screen.findByRole("button", {
      name: messages.expandToggleLabel(messages.title),
    });

    fireEvent.click(expandButton);

    const legend = await screen.findByLabelText(messages.title);

    expect(
      within(legend).getByText(messages.edgeLabels.activeFlow),
    ).toBeTruthy();
    expect(
      within(legend)
        .getByRole("img", {
          name: messages.iconLabel(messages.iconLabels.workstation),
        })
        .getAttribute("data-graph-semantic-icon"),
    ).toBe("workstation");
    expect(
      await screen.findByRole("button", { name: "Select Review workstation" }),
    ).toBeTruthy();
  });

  it("renders localized graph-import overlay and preview dialog shell copy without changing import behavior", async () => {
    const graphMessages = getWorkflowActivityGraphImportMessages("ja");
    const previewMessages = getImportPreviewDialogMessages("ja");
    const file = new File(["png"], "factory-import.png", { type: "image/png" });
    const importValue = createFactoryImportValue();
    const onFactoryImportReady =
      vi.fn<(value: FactoryPngImportValue, file: File) => void>();
    const readFactoryImportFile = vi
      .fn<ReadFactoryImportFile>()
      .mockResolvedValue({
        ok: true,
        value: importValue,
      });

    renderCurrentActivity({
      locale: "ja",
      onFactoryImportReady,
      readFactoryImportFile,
      snapshot: semanticWorkflowDashboardSnapshot,
    });

    const viewport = await screen.findByRole("region", {
      name: "Work graph viewport",
    });

    fireEvent.dragOver(viewport, createFileDropTransfer([file]));

    expect(screen.getByText(graphMessages.graphDropTitle)).toBeTruthy();
    expect(screen.getByText(graphMessages.graphDropHint)).toBeTruthy();

    fireEvent.drop(viewport, createFileDropTransfer([file]));

    const previewDialog = await screen.findByRole("dialog", {
      name: previewMessages.title,
    });

    expect(
      within(previewDialog).getByRole("button", {
        name: previewMessages.closeLabel,
      }),
    ).toBeTruthy();
  });

  it("renders localized graph-import validation errors and preserves dismiss reset behavior", async () => {
    const graphMessages = getWorkflowActivityGraphImportMessages("ja");
    const file = new File(["png"], "invalid-factory.png", {
      type: "image/png",
    });
    const onFactoryImportReady =
      vi.fn<(value: FactoryPngImportValue, file: File) => void>();
    const readFactoryImportFile = vi
      .fn<ReadFactoryImportFile>()
      .mockResolvedValue({
        error: {
          code: "PNG_METADATA_MISSING",
          message:
            "The selected PNG does not contain you-agent-factory factory metadata.",
        },
        ok: false,
      });

    renderCurrentActivity({
      locale: "ja",
      onFactoryImportReady,
      readFactoryImportFile,
      snapshot: semanticWorkflowDashboardSnapshot,
    });

    const viewport = await screen.findByRole("region", {
      name: "Work graph viewport",
    });

    fireEvent.drop(viewport, createFileDropTransfer([file]));

    const alert = await screen.findByRole("alert");

    expect(alert.textContent).toContain(graphMessages.graphImportErrorTitle);
    expect(alert.textContent).toContain("invalid-factory.png");
    expect(alert.textContent).toContain(
      graphMessages.importErrorMetadataMissing,
    );
    expect(onFactoryImportReady).not.toHaveBeenCalled();
    expect(
      screen.queryByRole("dialog", { name: "Review factory import" }),
    ).toBeNull();
    expect(viewport.getAttribute("data-current-activity-drop-state")).toBe(
      "error",
    );

    fireEvent.click(
      screen.getByRole("button", { name: graphMessages.dismissAction }),
    );

    await waitFor(() => {
      expect(screen.queryByRole("alert")).toBeNull();
    });
    expect(viewport.getAttribute("data-current-activity-drop-state")).toBe(
      "idle",
    );
  });

  it("keeps localized preview dialog shell controls available when activation fails", async () => {
    const graphMessages = getWorkflowActivityGraphImportMessages("ja");
    const previewMessages = getImportPreviewDialogMessages("ja");
    const file = new File(["png"], "factory-import.png", { type: "image/png" });
    const importValue = createFactoryImportValue();
    const activateFactory = vi
      .fn<(input: FactoryImportConfirmInput) => Promise<ImportFactoryValue>>()
      .mockRejectedValue(
        new SessionFactoryAPIError("Named factory already exists.", {
          code: "FACTORY_ALREADY_EXISTS",
          status: 409,
        }),
      );
    const onFactoryActivated = vi.fn<() => void>();
    const readFactoryImportFile = vi
      .fn<ReadFactoryImportFile>()
      .mockResolvedValue({
        ok: true,
        value: importValue,
      });

    renderCurrentActivity({
      activateFactory,
      locale: "ja",
      onFactoryActivated,
      readFactoryImportFile,
      snapshot: semanticWorkflowDashboardSnapshot,
    });

    const viewport = await screen.findByRole("region", {
      name: "Work graph viewport",
    });

    fireEvent.drop(viewport, createFileDropTransfer([file]));

    const previewDialog = await screen.findByRole("dialog", {
      name: previewMessages.title,
    });

    expect(
      within(previewDialog).getByText(graphMessages.dialogFlowLabel),
    ).toBeTruthy();

    fireEvent.click(
      within(previewDialog).getByRole("button", {
        name: previewMessages.activateAction,
      }),
    );

    const alert = await within(previewDialog).findByRole("alert");

    expect(alert.textContent).toContain(previewMessages.activationErrorTitle);
    expect(alert.textContent).toContain(
      previewMessages.errorByCode.FACTORY_ALREADY_EXISTS,
    );
    expect(onFactoryActivated).not.toHaveBeenCalled();
    expect(
      screen.getByRole("dialog", { name: previewMessages.title }),
    ).toBeTruthy();
    expect(
      within(previewDialog).getByRole("button", {
        name: previewMessages.closeLabel,
      }),
    ).toBeTruthy();
    expect(importValue.revokePreviewImageSrc).not.toHaveBeenCalled();
  });

  it("falls back to English graph-import copy for unsupported locales", async () => {
    const graphMessages = getWorkflowActivityGraphImportMessages("en");
    const file = new File(["png"], "factory-import.png", { type: "image/png" });

    renderCurrentActivity({
      locale: "fr-CA",
      snapshot: semanticWorkflowDashboardSnapshot,
    });

    const viewport = await screen.findByRole("region", {
      name: "Work graph viewport",
    });

    fireEvent.dragOver(viewport, createFileDropTransfer([file]));

    expect(screen.getByText(graphMessages.graphDropTitle)).toBeTruthy();
    expect(screen.getByText(graphMessages.graphDropHint)).toBeTruthy();
  });

  it(
    "falls back to English legend copy for unsupported workflow graph locales",
    async () => {
      const messages = getDashboardFlowAxisLegendMessages("en");

      renderCurrentActivity({
        snapshot: semanticWorkflowDashboardSnapshot,
        locale: "fr-CA",
      });

      const legend = await expandGraphLegend("fr-CA");

      expect(
        within(legend).getByText(messages.edgeLabels.activeFlow),
      ).toBeTruthy();
      expect(
        within(legend)
          .getByRole("img", {
            name: messages.iconLabel(messages.iconLabels.queue),
          })
          .getAttribute("data-graph-semantic-icon"),
      ).toBe("queue");
      expect(
        await screen.findByRole("button", {
          name: "Select Review workstation",
        }),
      ).toBeTruthy();
    },
    workflowGraphLocaleFallbackTimeoutMs,
  );
});
