import {
  act,
  fireEvent,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { FACTORY_EVENT_TYPES, type FactoryEvent } from "./api/events";
import { APP_SHELL_RESOLVED_DEFAULT_SESSION_UUID } from "./testing/app-shell-session-preflight-test-utils";
import {
  failureAnalysisTimelineEvents,
  graphStateSmokeTimelineEvents,
  resourceCountAvailablePlaceID,
  resourceCountBackendWorldViewCountsByTick,
  resourceCountTimelineEvents,
} from "./components/dashboard/fixtures";
import {
  baselineSnapshot,
  MockEventSource,
  nonPromptTemplateFetchPaths,
  registerAppDashboardTestLifecycle,
  renderApp,
} from "./testing/app-shell-test-utils";
import {
  historicalTimelineSnapshot,
  selectedTickTimelineEvents,
} from "./testing/app-shell-timeline-test-utils";

function getStateNodeByLabel(label: string): HTMLElement {
  const button = screen.getByRole("button", { name: `Select ${label} state` });
  const node = button.closest(".react-flow__node");

  if (!(node instanceof HTMLElement)) {
    throw new Error(
      `expected ${label} state to be rendered in a React Flow node`,
    );
  }

  return node;
}

function getWorkstationNodeByLabel(label: string): HTMLElement {
  const button = screen.getByRole("button", {
    name: `Select ${label} workstation`,
  });
  const node = button.closest(".react-flow__node");

  if (!(node instanceof HTMLElement)) {
    throw new Error(
      `expected ${label} workstation to be rendered in a React Flow node`,
    );
  }

  return node;
}

function expectFixedReviewWorkstationDimensions(): void {
  const reviewNode = getWorkstationNodeByLabel("Review");

  expect(reviewNode.getAttribute("style")).toContain("width: 156px");
  expect(reviewNode.getAttribute("style")).toContain("height: 196px");
}

function expectReactFlowNodePosition(
  node: HTMLElement,
  position: { x: number; y: number },
): void {
  expect(node.style.transform.replace(/\s/g, "")).toContain(
    `translate(${position.x}px,${position.y}px)`,
  );
}

function expectReactFlowViewportTransform(viewport: {
  x: number;
  y: number;
  zoom: number;
}): void {
  const workGraphViewport = screen.getByRole("region", {
    name: "Work graph viewport",
  });
  const flowViewport = workGraphViewport.querySelector<HTMLElement>(
    ".react-flow__viewport",
  );

  expect(flowViewport?.style.transform.replace(/\s/g, "")).toContain(
    `translate(${viewport.x}px,${viewport.y}px)scale(${viewport.zoom})`,
  );
}

function expectRenderedResourceCountMatchesBackendWorldView(
  tick: number,
): void {
  const expectedCount =
    resourceCountBackendWorldViewCountsByTick[tick]?.[
      resourceCountAvailablePlaceID
    ] ?? 0;

  const resourceNodeLabel = resourceCountAvailablePlaceID.split(":")[0];

  expect(screen.getByLabelText(resourceNodeLabel)).toBeTruthy();
  expect(
    screen
      .getByLabelText(`${expectedCount} resource tokens`)
      .textContent?.trim(),
  ).toBe(String(expectedCount));
}

function expectSeparatedStateMarkerZones(label: string, count: number): void {
  const stateNode = getStateNodeByLabel(label);
  const labelZone = stateNode.querySelector("[data-state-label-zone]");
  const markerZone = stateNode.querySelector("[data-state-marker-zone]");

  expect(labelZone).toBeTruthy();
  expect(markerZone).toBeTruthy();
  expect(labelZone?.textContent).not.toContain(`${count} active`);
  expect(markerZone?.textContent).not.toContain("story");
  expect(
    markerZone?.querySelectorAll("[data-state-work-progress-dot]"),
  ).toHaveLength(count);
}

async function requireEventStream(): Promise<MockEventSource> {
  return await waitFor(() => {
    const stream = MockEventSource.instances[0];

    if (!stream) {
      throw new Error("expected factory event stream to be opened");
    }

    return stream;
  });
}

const streamGraphBaseFactory = {
  name: "Streamed Graph Factory",
  layout: {
    nodes: [
      { id: "workstation:review", position: { x: 120, y: 180 } },
      { id: "worker:writer", position: { x: 120, y: 420 } },
    ],
    schemaVersion: 1,
    viewport: { x: 0, y: 0, zoom: 1 },
  },
  version: {
    logical: "1",
    physical: "2026-06-10T00:00:00Z",
  },
  workers: [
    {
      model: "gpt-5",
      name: "writer",
      type: "MODEL_WORKER",
    },
  ],
  workstations: [
    {
      inputs: [{ state: "queued", workType: "story" }],
      name: "Review",
      outputs: [{ state: "done", workType: "story" }],
      type: "MODEL_WORKSTATION",
      worker: "writer",
    },
  ],
  workTypes: [
    {
      name: "story",
      states: [
        { name: "queued", type: "INITIAL" },
        { name: "qa", type: "PROCESSING" },
        { name: "done", type: "TERMINAL" },
      ],
    },
  ],
};

const streamGraphChangedFactory = {
  ...streamGraphBaseFactory,
  layout: {
    edges: [
      {
        id: "worker-assignment:worker:critic->workstation:QA",
        waypoints: [{ x: 460, y: 250 }],
      },
    ],
    nodes: [
      ...(streamGraphBaseFactory.layout.nodes ?? []),
      { id: "workstation:QA", position: { x: 640, y: 260 } },
      { id: "worker:critic", position: { x: 420, y: 420 } },
    ],
    schemaVersion: 1,
    viewport: { x: -180, y: 55, zoom: 0.85 },
  },
  version: {
    logical: "2",
    physical: "2026-06-10T00:01:00Z",
  },
  workers: [
    ...streamGraphBaseFactory.workers,
    {
      model: "gpt-5",
      name: "critic",
      type: "MODEL_WORKER",
    },
  ],
  workstations: [
    ...streamGraphBaseFactory.workstations,
    {
      inputs: [{ state: "qa", workType: "story" }],
      name: "QA",
      outputs: [{ state: "done", workType: "story" }],
      type: "MODEL_WORKSTATION",
      worker: "critic",
    },
  ],
};

const streamedGraphChangeEvents = [
  {
    context: {
      eventTime: "2026-06-10T00:00:00Z",
      sequence: 1,
      tick: 1,
    },
    id: "factory-event/initial-structure/streamed-graph",
    payload: {
      factory: streamGraphBaseFactory,
    },
    type: FACTORY_EVENT_TYPES.initialStructureRequest,
  },
  {
    context: {
      eventTime: "2026-06-10T00:01:00Z",
      sequence: 2,
      tick: 2,
    },
    id: "factory-event/factory-change/streamed-graph",
    payload: {
      factory: streamGraphChangedFactory,
    },
    type: FACTORY_EVENT_TYPES.factoryChange,
  },
] satisfies FactoryEvent[];

describe("App streamed replay rendering flows", () => {
  registerAppDashboardTestLifecycle();

  it("smoke tests session-scoped event replay rendering without the removed dashboard snapshot route", async () => {
    const { fetchMock } = renderApp({ snapshot: historicalTimelineSnapshot });

    const stream = await requireEventStream();
    expect(stream.url).toBe(
      `/factory-sessions/${APP_SHELL_RESOLVED_DEFAULT_SESSION_UUID}/events`,
    );

    act(() => {
      for (const event of selectedTickTimelineEvents) {
        stream.emit("message", event);
      }
    });

    const slider = await screen.findByRole<HTMLInputElement>("slider", {
      name: "Timeline tick",
    });
    await waitFor(() => {
      expect(slider.value).toBe("4");
      expect(screen.getByText("4/4")).toBeTruthy();
      expect(
        screen.getByRole("article", { name: "Current selection" }),
      ).toBeTruthy();
      expect(
        screen.getByRole("article", { name: "Trace drill-down" }),
      ).toBeTruthy();
    });
    expect(nonPromptTemplateFetchPaths(fetchMock)).toEqual([]);

    fireEvent.change(slider, { target: { value: "3" } });

    await waitFor(() => {
      expect(slider.value).toBe("3");
      expect(screen.getByText("3/4")).toBeTruthy();
      expect(screen.queryByText("sess-event-story")).toBeNull();
      expect(
        within(screen.getByLabelText("work totals"))
          .getByText("In progress")
          .closest("article")?.textContent,
      ).toContain("1");
    });
  });

  it("smoke tests failure analysis from streamed events through fixed-tick rendering", async () => {
    const { fetchMock } = renderApp({ snapshot: historicalTimelineSnapshot });

    const stream = await requireEventStream();

    act(() => {
      for (const event of failureAnalysisTimelineEvents) {
        stream.emit("message", event);
      }
    });

    const slider = await screen.findByRole<HTMLInputElement>("slider", {
      name: "Timeline tick",
    });
    await waitFor(() => {
      expect(slider.value).toBe("4");
      expect(screen.getByText("4/4")).toBeTruthy();
      expect(
        screen.getByRole("button", {
          name: "Select work item Blocked Analysis Story",
        }),
      ).toBeTruthy();
      expect(screen.getAllByText("Failed at Review").length).toBeGreaterThan(0);
    });

    fireEvent.click(
      screen.getByRole("button", {
        name: "Select work item Blocked Analysis Story",
      }),
    );

    const failedDetail = await screen.findByRole("article", {
      name: "Current selection",
    });
    expect(failedDetail.textContent).toContain("Blocked Analysis Story");
    expect(
      within(failedDetail).queryByText(
        "Terminal summaries are reconstructed from retained runtime state.",
      ),
    ).toBeNull();

    fireEvent.click(
      await screen.findByRole("button", { name: "Select story:new state" }),
    );

    const currentPositionDetail = await screen.findByRole("article", {
      name: "Current selection",
    });
    expect(
      within(currentPositionDetail).getByText("Current work"),
    ).toBeTruthy();
    expect(
      within(currentPositionDetail).getByText("Queued Analysis Story"),
    ).toBeTruthy();
    expect(
      within(currentPositionDetail).getByText("work-queued-analysis"),
    ).toBeTruthy();
    expect(
      within(currentPositionDetail).queryByText(/^Started at /),
    ).toBeNull();

    fireEvent.change(slider, { target: { value: "3" } });

    await waitFor(() => {
      expect(slider.value).toBe("3");
      expect(screen.getByText("3/4")).toBeTruthy();
      expect(
        screen.queryByRole("button", {
          name: "Select work item Blocked Analysis Story",
        }),
      ).toBeNull();
      expect(screen.queryByText("provider_rate_limit")).toBeNull();
      expect(screen.queryByText("sess-blocked-analysis")).toBeNull();
      expect(screen.getByText("Queued Analysis Story")).toBeTruthy();
    });

    fireEvent.change(slider, { target: { value: "4" } });

    await waitFor(() => {
      expect(slider.value).toBe("4");
      expect(screen.getByText("4/4")).toBeTruthy();
      expect(
        screen.getByRole("button", {
          name: "Select work item Blocked Analysis Story",
        }),
      ).toBeTruthy();
    });

    fireEvent.click(
      screen.getByRole("button", {
        name: "Select work item Blocked Analysis Story",
      }),
    );

    const fixedFailedDetail = await screen.findByRole("article", {
      name: "Current selection",
    });
    expect(
      within(fixedFailedDetail).getAllByText("Blocked Analysis Story").length,
    ).toBeGreaterThan(0);
    expect(nonPromptTemplateFetchPaths(fetchMock)).toEqual([]);
  }, 90_000);

  it("smoke tests resource counts from streamed events against backend world-view counts", async () => {
    const { fetchMock } = renderApp({ snapshot: historicalTimelineSnapshot });

    const stream = await requireEventStream();

    act(() => {
      for (const event of resourceCountTimelineEvents) {
        stream.emit("message", event);
      }
    });

    const slider = await screen.findByRole<HTMLInputElement>("slider", {
      name: "Timeline tick",
    });
    await waitFor(() => {
      expect(slider.value).toBe("4");
      expect(screen.getByText("4/4")).toBeTruthy();
      expectRenderedResourceCountMatchesBackendWorldView(4);
    });

    fireEvent.change(slider, { target: { value: "3" } });

    await waitFor(() => {
      expect(slider.value).toBe("3");
      expect(screen.getByText("3/4")).toBeTruthy();
      expectRenderedResourceCountMatchesBackendWorldView(3);
    });

    fireEvent.change(slider, { target: { value: "1" } });

    await waitFor(() => {
      expect(slider.value).toBe("1");
      expect(screen.getByText("1/4")).toBeTruthy();
      expectRenderedResourceCountMatchesBackendWorldView(1);
    });

    expect(nonPromptTemplateFetchPaths(fetchMock)).toEqual([]);
  });

  it("renders factory graph changes from streamed factory events", async () => {
    const { fetchMock } = renderApp({
      seedTimelineFromSnapshot: false,
      snapshot: baselineSnapshot,
    });

    const stream = await requireEventStream();

    act(() => {
      stream.emit("message", streamedGraphChangeEvents[0]);
    });

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: "Select Review workstation" }),
      ).toBeTruthy();
      expect(
        screen.queryByRole("button", { name: "Select QA workstation" }),
      ).toBeNull();
      expect(screen.getByRole("img", { name: "worker:writer" })).toBeTruthy();
    });

    act(() => {
      stream.emit("message", streamedGraphChangeEvents[1]);
    });

    const qaNode = await waitFor(() => getWorkstationNodeByLabel("QA"));
    expectReactFlowNodePosition(qaNode, { x: 640, y: 260 });
    await waitFor(() => {
      expectReactFlowViewportTransform({ x: -180, y: 55, zoom: 0.85 });
    });
    expect(screen.getByRole("img", { name: "worker:critic" })).toBeTruthy();
    expect(
      screen.getByRole<HTMLInputElement>("slider", {
        name: "Timeline tick",
      }).value,
    ).toBe("2");
    expect(nonPromptTemplateFetchPaths(fetchMock)).toEqual([]);
  });

  it("smoke tests graph state across event replay, terminal selection, and tick changes", async () => {
    renderApp({
      snapshot: baselineSnapshot,
      timelineEvents: graphStateSmokeTimelineEvents,
    });

    const slider = await screen.findByRole<HTMLInputElement>("slider", {
      name: "Timeline tick",
    });
    const dashboardGrid = screen.getByRole("region", {
      name: "you-agent-factory bento board",
    });

    await waitFor(() => {
      expect(slider.value).toBe("9");
      expect(screen.getByText("9/9")).toBeTruthy();
      expectFixedReviewWorkstationDimensions();
      expect(
        getStateNodeByLabel("story:done").querySelector(
          "[aria-label='2 active items']",
        ),
      ).toBeTruthy();
      expect(
        getStateNodeByLabel("story:failed").querySelector(
          "[aria-label='1 active item']",
        ),
      ).toBeTruthy();
    });

    fireEvent.click(
      screen.getByRole("button", { name: "Select story:done state" }),
    );

    const completedDetail = await within(dashboardGrid).findByRole("article", {
      name: "Current selection",
    });
    expect(within(completedDetail).getByText("Current work")).toBeTruthy();
    expect(
      within(completedDetail).getByText("Completed Smoke Story One"),
    ).toBeTruthy();
    expect(
      within(completedDetail).getByText("Completed Smoke Story Two"),
    ).toBeTruthy();
    expect(
      within(completedDetail).getByText("work-smoke-complete-one"),
    ).toBeTruthy();
    expect(
      within(completedDetail).getByText("work-smoke-complete-two"),
    ).toBeTruthy();
    expect(within(completedDetail).queryAllByText(/^Started at /)).toHaveLength(
      2,
    );

    fireEvent.click(
      screen.getByRole("button", { name: "Select story:failed state" }),
    );

    await waitFor(() => {
      const failedDetail = screen.getByRole("article", {
        name: "Current selection",
      });

      expect(within(failedDetail).getByText("Current work")).toBeTruthy();
      expect(within(failedDetail).getByText("Failed Smoke Story")).toBeTruthy();
      expect(within(failedDetail).getByText("work-smoke-failed")).toBeTruthy();
      expect(within(failedDetail).queryAllByText(/^Started at /)).toHaveLength(
        1,
      );
      expect(
        within(failedDetail).getByText("provider_rate_limit"),
      ).toBeTruthy();
      expect(
        within(failedDetail).queryByText(
          "No current work is occupying this place.",
        ),
      ).toBeNull();
    });

    fireEvent.change(slider, { target: { value: "3" } });

    await waitFor(() => {
      expect(slider.value).toBe("3");
      expect(screen.getByText("3/9")).toBeTruthy();
      expect(
        screen.getByRole("button", { name: /Completed Smoke Story One/ }),
      ).toBeTruthy();
      expectFixedReviewWorkstationDimensions();
    });

    fireEvent.change(slider, { target: { value: "2" } });

    await waitFor(() => {
      expect(slider.value).toBe("2");
      expect(screen.getByText("2/9")).toBeTruthy();
      expectSeparatedStateMarkerZones("story:new", 3);
    });

    fireEvent.change(slider, { target: { value: "9" } });

    await waitFor(() => {
      expect(slider.value).toBe("9");
      expect(screen.getByText("9/9")).toBeTruthy();
      expectFixedReviewWorkstationDimensions();
      expect(
        getStateNodeByLabel("story:done").querySelector(
          "[aria-label='2 active items']",
        ),
      ).toBeTruthy();
    });
  }, 90_000);
});
