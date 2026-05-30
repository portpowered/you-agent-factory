import "../testing/bun-app-shell-module-mocks";
import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import { describe, expect, it } from "bun:test";
import {
  DASHBOARD_WIDGET_IDS,
} from "./features/bento/hooks/dashboardLayoutSchema";
import {
  activeSnapshot,
  baselineSnapshot,
  registerAppDashboardTestLifecycle,
  renderApp,
} from "./testing/app-shell-test-utils";
import {
  singleNodeSnapshotWithoutEdges,
  tickZeroInitialStructureRequestEvents,
  twentyNodeSnapshot,
} from "./testing/app-shell-layout-test-utils";

function getStateNodeByLabel(label: string): HTMLElement {
  const button = screen.getByRole("button", { name: `Select ${label} state` });
  const node = button.closest(".react-flow__node");

  if (!(node instanceof HTMLElement)) {
    throw new Error(`expected ${label} state to be rendered in a React Flow node`);
  }

  return node;
}

function expectStateNodeDotCount(label: string, count: number): void {
  expect(
    getStateNodeByLabel(label).querySelectorAll("[data-state-work-progress-dot]"),
  ).toHaveLength(count);
}

describe("App layout behavior", () => {
  registerAppDashboardTestLifecycle();

  it("renders backend tick-zero initial structure instead of staying in loading state", async () => {
    renderApp({
      snapshot: baselineSnapshot,
      timelineEvents: tickZeroInitialStructureRequestEvents,
    });

    expect(
      await screen.findByRole("heading", { name: "U" }),
    ).toBeTruthy();
    expect(screen.queryByText("Loading dashboard")).toBeNull();
    expect(
      await screen.findByRole("region", { name: "Work graph viewport" }),
    ).toBeTruthy();
    expect(
      within(screen.getByRole("region", { name: "Work graph viewport" })).getByRole("button", {
        name: "Zoom In",
      }),
    ).toBeTruthy();
    expect(
      (
        screen.getByRole("slider", {
          name: "Timeline tick",
        }) as HTMLInputElement
      ).value,
    ).toBe("0");
    expect(screen.getByText("Waiting for more ticks")).toBeTruthy();
  });

  it("starts with full-width totals above a full-width Factory graph card", async () => {
    renderApp({ snapshot: baselineSnapshot });

    await screen.findByRole("heading", { name: "U" });

    const dashboardGrid = screen.getByRole("region", {
      name: "you-agent-factory bento board",
    });
    const workTotals = dashboardGrid.querySelector<HTMLElement>(
      '[data-bento-card-id="work-totals"]',
    );
    const workflowActivity = dashboardGrid.querySelector<HTMLElement>(
      '[data-bento-card-id="work-graph"]',
    );
    if (!workTotals || !workflowActivity) {
      throw new Error("expected totals and workflow cards in the dashboard grid");
    }

    expect(workTotals.dataset.layoutSignature).toContain("work-totals:0:0:12:2");
    expect(workflowActivity.dataset.layoutSignature).toContain("work-graph:0:2:12:8");
    expect(within(screen.getByLabelText("work totals")).getByText("In progress")).toBeTruthy();
    expect(within(screen.getByLabelText("work totals")).getByText("Completed")).toBeTruthy();
    expect(within(screen.getByLabelText("work totals")).getByText("Failed")).toBeTruthy();
    expect(within(screen.getByLabelText("work totals")).getByText("Dispatched")).toBeTruthy();
  });
});

describe("App layout migration behavior", () => {
  registerAppDashboardTestLifecycle();

  it("migrates the stored dashboard baseline to the compacted grid slots", async () => {
    window.localStorage.setItem(
      "agent-factory.dashboard.layout.v2",
      JSON.stringify([
        { h: 2, id: "work-totals", w: 12, x: 0, y: 0 },
        { h: 10, id: "work-graph", w: 12, x: 0, y: 2 },
        { h: 5, id: "current-selection", w: 4, x: 0, y: 12 },
        { h: 6, id: "submit-work", w: 4, x: 8, y: 18 },
        { h: 9, id: "trace", w: 8, x: 0, y: 18 },
        { h: 5, id: "completion-trend", w: 5, x: 7, y: 12 },
        { h: 6, id: "work-info", w: 5, x: 7, y: 18 },
      ]),
    );

    renderApp({ snapshot: activeSnapshot });

    await screen.findByRole("heading", { name: "U" });

    const dashboardGrid = screen.getByRole("region", {
      name: "you-agent-factory bento board",
    });
    expect(
      dashboardGrid.querySelector<HTMLElement>('[data-bento-card-id="work-graph"]')
        ?.dataset.layoutSignature,
    ).toContain("work-graph:0:2:12:8");
    expect(
      dashboardGrid.querySelector<HTMLElement>('[data-bento-card-id="current-selection"]')
        ?.dataset.layoutSignature,
    ).toMatch(/current-selection:\d+:\d+:\d+:\d+/);
    expect(
      dashboardGrid.querySelector<HTMLElement>('[data-bento-card-id="work-outcome-chart"]')
        ?.dataset.layoutSignature,
    ).toMatch(/work-outcome-chart:\d+:\d+:\d+:\d+/);
    expect(
      dashboardGrid.querySelector<HTMLElement>('[data-bento-card-id="trace"]')
        ?.dataset.layoutSignature,
    ).toMatch(/trace:0:\d+:8:9/);
    expect(
      dashboardGrid.querySelector(
        '[data-bento-card-id="work-info"], [data-bento-card-id="completion-trend"]',
      ),
    ).toBeNull();
  });

  it("migrates legacy selection detail layout IDs into one current selection slot", async () => {
    window.localStorage.setItem(
      "agent-factory.dashboard.layout.v2",
      JSON.stringify([
        { h: 5, id: "work-totals", w: 12, x: 0, y: 0 },
        { h: 10, id: "work-graph", w: 12, x: 0, y: 2 },
        { h: 6, id: "terminal-summary", w: 5, x: 7, y: 12 },
        { h: 9, id: "trace", w: 8, x: 0, y: 18 },
      ]),
    );

    renderApp({ snapshot: activeSnapshot });

    await screen.findByRole("heading", { name: "U" });

    const dashboardGrid = screen.getByRole("region", {
      name: "you-agent-factory bento board",
    });
    const currentSelection = dashboardGrid.querySelector<HTMLElement>(
      '[data-bento-card-id="current-selection"]',
    );

    expect(currentSelection).toBeTruthy();
    expect(
      dashboardGrid.querySelector(
        '[data-bento-card-id="work-info"], [data-bento-card-id="workstation-info"], [data-bento-card-id="terminal-summary"]',
      ),
    ).toBeNull();
    expect(currentSelection?.dataset.layoutSignature).toMatch(/current-selection:7:\d+:5:6/);
  });

  it("migrates stored completion and failure chart layout IDs into one work outcome chart slot", async () => {
    window.localStorage.setItem(
      "agent-factory.dashboard.layout.v2",
      JSON.stringify([
        { h: 5, id: "completion-trend", w: 5, x: 7, y: 12 },
        { h: 5, id: "failure-trend", w: 4, x: 0, y: 17 },
      ]),
    );

    renderApp({ snapshot: activeSnapshot });

    await screen.findByRole("heading", { name: "U" });

    const dashboardGrid = screen.getByRole("region", {
      name: "you-agent-factory bento board",
    });
    const workOutcome = dashboardGrid.querySelector<HTMLElement>(
      '[data-bento-card-id="work-outcome-chart"]',
    );

    expect(workOutcome).toBeTruthy();
    expect(
      dashboardGrid.querySelector(
        '[data-bento-card-id="completion-trend"], [data-bento-card-id="failure-trend"]',
      ),
    ).toBeNull();
    expect(workOutcome?.dataset.layoutSignature).toMatch(
      new RegExp(`${DASHBOARD_WIDGET_IDS.workOutcomeChart}:\\d+:\\d+:\\d+:5`),
    );
  });

  it("ignores stored retry, rework, and timing trend card IDs in the visible dashboard layout", async () => {
    window.localStorage.setItem(
      "agent-factory.dashboard.layout.v2",
      JSON.stringify([
        { h: 5, id: "rework-trend", w: 4, x: 0, y: 18 },
        { h: 5, id: "timing-trend", w: 4, x: 4, y: 18 },
        { h: 7, id: "trace", w: 4, x: 8, y: 18 },
      ]),
    );

    renderApp({ snapshot: activeSnapshot });

    fireEvent.click((await screen.findAllByRole("button", { name: /Active Story/ }))[0]);

    const dashboardGrid = screen.getByRole("region", {
      name: "you-agent-factory bento board",
    });
    const trace = await within(dashboardGrid).findByRole("article", {
      name: "Trace drill-down",
    });

    expect(
      dashboardGrid.querySelector(
        '[data-bento-card-id="rework-trend"], [data-bento-card-id="timing-trend"]',
      ),
    ).toBeNull();
    expect(trace.closest<HTMLElement>("[data-bento-card-id]")?.dataset.layoutSignature).toMatch(
      /trace:\d+:\d+:\d+:\d+/,
    );
  });
});

describe("App graph behavior", () => {
  registerAppDashboardTestLifecycle();

  it("renders distinct graph semantics for topology places, active work, and retry outcomes", async () => {
    renderApp({ snapshot: activeSnapshot });

    await screen.findByRole("button", { name: "Select work item Active Story" });
    await waitFor(() => {
      expect(screen.getAllByRole("button", { name: /Select .* workstation/ })).toHaveLength(5);
    });
    expect(screen.getByLabelText("agent-slot")).toBeTruthy();
    expect(screen.getByLabelText("2 resource tokens")).toBeTruthy();
    expect(screen.getByRole("img", { name: "worker:reviewer" })).toBeTruthy();
    expect(screen.getByRole("img", { name: "work-type:story" })).toBeTruthy();
    expect(
      screen.getByRole("img", { name: "Resource" }).getAttribute("data-graph-semantic-icon"),
    ).toBe("resource");
    expect(
      screen
        .getAllByRole("img", { name: "Worker" })[0]
        ?.getAttribute("data-graph-semantic-icon"),
    ).toBe("worker");
    expect(screen.queryByText("Active Work")).toBeNull();
    expectStateNodeDotCount("story:ready", 3);
    expect(getStateNodeByLabel("story:blocked")).toBeTruthy();
    expect(getStateNodeByLabel("story:complete")).toBeTruthy();
  });

  it("renders a valid single-workstation topology when the API omits empty edges", async () => {
    renderApp({ snapshot: singleNodeSnapshotWithoutEdges });

    expect(
      await screen.findByRole("button", { name: "Select Intake workstation" }),
    ).toBeTruthy();
    const currentSelection = screen.getByRole("article", {
      name: "Current selection",
    });
    fireEvent.click(within(currentSelection).getByRole("button", { name: "Expand" }));
    expect(
      within(currentSelection).getByText(
        "No workstation runs have been recorded for this workstation yet.",
      ),
    ).toBeTruthy();
  });

  it("uses React Flow controls for work graph zoom interaction", async () => {
    renderApp({ snapshot: baselineSnapshot });

    await screen.findByRole("heading", { name: "U" });

    const workGraphViewport = screen.getByRole("region", {
      name: "Work graph viewport",
    });
    const flowViewport = document.querySelector<HTMLElement>(".react-flow__viewport");
    const initialTransform = flowViewport?.style.transform;

    fireEvent.click(within(workGraphViewport).getByRole("button", { name: "Zoom In" }));

    await waitFor(() => {
      expect(flowViewport?.style.transform).not.toBe(initialTransform);
    });
  });

  it("renders and interacts with a 20-node workflow through React Flow", async () => {
    renderApp({ snapshot: twentyNodeSnapshot });

    const workGraphViewport = await screen.findByRole("region", {
      name: "Work graph viewport",
    });
    await waitFor(() => {
      expect(screen.getAllByRole("button", { name: /Select .* workstation/ })).toHaveLength(20);
    });
    const flowViewport = document.querySelector<HTMLElement>(".react-flow__viewport");
    const initialTransform = flowViewport?.style.transform;

    fireEvent.click(within(workGraphViewport).getByRole("button", { name: "Zoom In" }));
    await waitFor(() => {
      expect(flowViewport?.style.transform).not.toBe(initialTransform);
    });
  }, 30_000);
});
