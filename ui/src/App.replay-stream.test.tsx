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
  const displayLabel = label.split(":").at(-1) ?? label;
  const button = screen.getByLabelText(`Select ${displayLabel} work-state`);
  const node = button.closest(".react-flow__node");

  if (!(node instanceof HTMLElement)) {
    throw new Error(
      `expected ${label} state to be rendered in a React Flow node`,
    );
  }

  return node;
}

function getWorkstationNodeByLabel(label: string): HTMLElement {
  const button = screen.getByLabelText(`Select ${label} workstation`);
  const node = button.closest(".react-flow__node");

  if (!(node instanceof HTMLElement)) {
    throw new Error(
      `expected ${label} workstation to be rendered in a React Flow node`,
    );
  }

  return node;
}

function expectRenderedResourceCountMatchesBackendWorldView(
  tick: number,
): void {
  const expectedCount =
    resourceCountBackendWorldViewCountsByTick[tick]?.[
      resourceCountAvailablePlaceID
    ] ?? 0;

  const resourceNodeLabel = resourceCountAvailablePlaceID.split(":")[0];
  const resourceNode = screen.getByLabelText(
    `Select ${resourceNodeLabel} resource`,
  );
  expect(resourceNode.textContent).toContain(
    `${2 - expectedCount} of 2 resource units occupied`,
  );
}

function expectWorkStateCount(label: string, count: number): void {
  expect(getStateNodeByLabel(label).textContent).toContain(`${count} Work`);
}

function withTopologyFactoryChange(events: FactoryEvent[]): FactoryEvent[] {
  const structureEvent = events.find(
    (event) => event.type === FACTORY_EVENT_TYPES.initialStructureRequest,
  );
  if (!structureEvent) {
    return events;
  }
  const factory = structureEvent.payload.factory as {
    name?: string;
    workers?: Array<Record<string, unknown>>;
    workstations?: Array<
      Record<string, unknown> & { type?: string; worker?: string }
    >;
  };
  const workerNames = [
    ...new Set(
      (factory.workstations ?? [])
        .map((workstation) => workstation.worker)
        .filter((worker): worker is string => Boolean(worker)),
    ),
  ];
  const topologyFactory = {
    ...factory,
    name: factory.name ?? "Replay Fixture Factory",
    workers:
      factory.workers ??
      workerNames.map((name) => ({
        model: "gpt-5",
        name,
        type: "MODEL_WORKER",
      })),
    workstations: (factory.workstations ?? []).map((workstation) => ({
      ...workstation,
      type: workstation.type ?? "MODEL_WORKSTATION",
    })),
  };
  return [
    structureEvent,
    {
      ...structureEvent,
      context: {
        ...structureEvent.context,
        sequence: structureEvent.context.sequence + 1,
        ...(structureEvent.context.sessionSequence === undefined
          ? {}
          : {
              sessionSequence:
                structureEvent.context.sessionSequence + 1,
            }),
      },
      id: `${structureEvent.id}/factory-change`,
      payload: { factory: topologyFactory },
      type: FACTORY_EVENT_TYPES.factoryChange,
    },
    ...events
      .filter((event) => event !== structureEvent)
      .map((event) => ({
        ...event,
        context: {
          ...event.context,
          sequence: event.context.sequence + 1,
          ...(event.context.sessionSequence === undefined
            ? {}
            : { sessionSequence: event.context.sessionSequence + 1 }),
        },
      })),
  ];
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
      for (const event of withTopologyFactoryChange(
        failureAnalysisTimelineEvents,
      )) {
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

    const blockedWorkButton = screen.getByLabelText(
      "Select work item Blocked Analysis Story",
    );
    if (blockedWorkButton.getAttribute("aria-pressed") !== "true") {
      fireEvent.click(blockedWorkButton);
    }
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
      await screen.findByLabelText("Select new work-state"),
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
      expect(screen.queryByText("throttled")).toBeNull();
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
      for (const event of withTopologyFactoryChange(
        resourceCountTimelineEvents,
      )) {
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
        screen.getByLabelText("Select Review workstation"),
      ).toBeTruthy();
      expect(
        screen.queryByLabelText("Select QA workstation"),
      ).toBeNull();
      expect(screen.getByLabelText("Select writer worker")).toBeTruthy();
    });

    act(() => {
      stream.emit("message", streamedGraphChangeEvents[1]);
    });

    expect(await waitFor(() => getWorkstationNodeByLabel("QA"))).toBeTruthy();
    expect(screen.getByLabelText("Select critic worker")).toBeTruthy();
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
      timelineEvents: withTopologyFactoryChange(graphStateSmokeTimelineEvents),
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
      expectWorkStateCount("story:done", 2);
      expectWorkStateCount("story:failed", 1);
    });

    fireEvent.click(
      screen.getByLabelText("Select done work-state"),
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
      screen.getByLabelText("Select failed work-state"),
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
        within(failedDetail).getByText("throttled"),
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
      expectWorkStateCount("story:new", 2);
      expect(getWorkstationNodeByLabel("Review")).toBeTruthy();
    });

    fireEvent.change(slider, { target: { value: "2" } });

    await waitFor(() => {
      expect(slider.value).toBe("2");
      expect(screen.getByText("2/9")).toBeTruthy();
      expectWorkStateCount("story:new", 3);
    });

    fireEvent.change(slider, { target: { value: "9" } });

    await waitFor(() => {
      expect(slider.value).toBe("9");
      expect(screen.getByText("9/9")).toBeTruthy();
      expect(getWorkstationNodeByLabel("Review")).toBeTruthy();
      expectWorkStateCount("story:done", 2);
    });
  }, 90_000);
});
