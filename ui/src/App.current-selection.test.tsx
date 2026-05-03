import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { App } from "./App";
import type {
  DashboardSnapshot,
  DashboardTrace,
  DashboardWorkItemRef,
  DashboardWorkstationRequest,
} from "./api/dashboard";
import {
  buildDashboardSnapshotFixture,
  dashboardWorkstationRequestFixtures,
  mediumBranchingDashboardTopology,
} from "./components/dashboard/fixtures";
import { installDashboardBrowserTestShims } from "./components/dashboard/test-browser-shims";
import { semanticWorkflowDashboardSnapshot } from "./components/dashboard/test-fixtures";
import { useDashboardBentoStore } from "./features/bento/state/dashboardBentoStore";
import { reloadDashboardLayoutFromStorage } from "./features/bento/useDashboardLayout";
import {
  createDefaultDashboardStreamState,
  useDashboardStreamStore,
} from "./features/dashboard/state/dashboardStreamStore";
import { useExportDialogStore } from "./features/export/state/exportDialogStore";
import type { WorldState } from "./features/timeline/state/factoryTimelineStore";
import { useFactoryTimelineStore } from "./features/timeline/state/factoryTimelineStore";

class MockEventSource {
  public static instances: MockEventSource[] = [];

  public onerror: ((event: Event) => void) | null = null;
  public onopen: ((event: Event) => void) | null = null;

  private readonly listeners = new Map<string, EventListener[]>();

  constructor(public readonly url: string) {
    MockEventSource.instances.push(this);
  }

  public addEventListener(type: string, listener: EventListener): void {
    const existing = this.listeners.get(type) ?? [];
    existing.push(listener);
    this.listeners.set(type, existing);
  }

  public close(): void {}

  public emit(type: string, data: unknown): void {
    if (type === "snapshot") {
      const state = useFactoryTimelineStore.getState();
      const tracesByWorkID =
        state.worldViewCache[state.selectedTick]?.tracesByWorkID ?? {};
      seedTimelineSnapshot(data as DashboardSnapshot, tracesByWorkID);
    }

    const event = new MessageEvent(type, {
      data: JSON.stringify(data),
    });

    for (const listener of this.listeners.get(type) ?? []) {
      listener(event);
    }
  }
}

interface RenderAppOptions {
  snapshot: DashboardSnapshot;
  timelineSnapshots?: DashboardSnapshot[];
  traceFixtures?: Record<string, DashboardTrace>;
  workstationRequestsByDispatchID?: Record<string, DashboardWorkstationRequest>;
}

const activeWorkID = "work-active-story";
const completedWorkID = "work-complete";
const failedWorkID = "work-failed-story";
const activeWorkLabel = "Active Story";

const baselineSnapshot = buildDashboardSnapshotFixture(
  mediumBranchingDashboardTopology,
);
const activeSnapshot = semanticWorkflowDashboardSnapshot;
const activeSnapshotWithoutTraceID = removeTraceIDsFromSnapshot(activeSnapshot);
const terminalBaseSnapshot = semanticWorkflowDashboardSnapshot;
const terminalSnapshot = {
  ...terminalBaseSnapshot,
  tick_count: 4,
  runtime: {
    ...terminalBaseSnapshot.runtime,
    place_occupancy_work_items_by_place_id: {
      ...(terminalBaseSnapshot.runtime.place_occupancy_work_items_by_place_id ??
        {}),
      "story:blocked": [
        {
          display_name: "Failed Story",
          trace_id: "trace-failed-story",
          work_id: failedWorkID,
          work_type_id: "story",
        },
      ],
      "story:complete": [
        {
          display_name: "Done Story",
          trace_id: "trace-done-story",
          work_id: completedWorkID,
          work_type_id: "story",
        },
      ],
    },
    place_token_counts: {
      ...(terminalBaseSnapshot.runtime.place_token_counts ?? {}),
      "story:blocked": 1,
      "story:complete": 1,
    },
    session: {
      ...terminalBaseSnapshot.runtime.session,
      completed_count: 1,
      completed_work_labels: ["Done Story"],
      provider_sessions: [
        ...(terminalBaseSnapshot.runtime.session.provider_sessions ?? []),
        {
          dispatch_id: "dispatch-complete",
          outcome: "ACCEPTED",
          provider_session: {
            id: "sess-done-story",
            kind: "session_id",
            provider: "codex",
          },
          transition_id: "complete",
          workstation_name: "Complete",
          work_items: [
            {
              display_name: "Done Story",
              trace_id: "trace-done-story",
              work_id: completedWorkID,
              work_type_id: "story",
            },
          ],
        },
      ],
    },
  },
} satisfies DashboardSnapshot;
const historicalTimelineSnapshot = {
  ...baselineSnapshot,
  tick_count: 1,
} satisfies DashboardSnapshot;

const traceSnapshot: DashboardTrace = {
  trace_id: "trace-active-story",
  work_ids: [activeWorkID],
  transition_ids: ["plan", "review"],
  workstation_sequence: ["Plan", "Review"],
  dispatches: [
    {
      dispatch_id: "dispatch-review-active",
      transition_id: "plan",
      workstation_name: "Plan",
      outcome: "ACCEPTED",
      provider_session: {
        provider: "codex",
        kind: "session_id",
        id: "sess-active-story",
      },
      start_time: "2026-04-08T12:00:00Z",
      end_time: "2026-04-08T12:00:01Z",
      duration_millis: 1000,
      consumed_tokens: [
        {
          token_id: "tok-plan-in",
          place_id: "story:init",
          work_id: activeWorkID,
          work_type_id: "story",
          trace_id: "trace-active-story",
          created_at: "2026-04-08T11:59:58Z",
          entered_at: "2026-04-08T11:59:59Z",
        },
      ],
      output_mutations: [
        {
          type: "MOVE",
          token_id: "tok-plan-in",
          from_place: "story:init",
          to_place: "story:ready",
          resulting_token: {
            token_id: "tok-plan-out",
            place_id: "story:ready",
            work_id: activeWorkID,
            work_type_id: "story",
            trace_id: "trace-active-story",
            created_at: "2026-04-08T12:00:01Z",
            entered_at: "2026-04-08T12:00:01Z",
          },
        },
      ],
    },
  ],
};

const reworkTraceSnapshot: DashboardTrace = {
  ...traceSnapshot,
  transition_ids: ["plan", "review", "plan"],
  workstation_sequence: ["Plan", "Review", "Plan"],
  dispatches: [
    ...traceSnapshot.dispatches,
    {
      dispatch_id: "dispatch-review-rejected",
      transition_id: "review",
      workstation_name: "Review",
      outcome: "REJECTED",
      start_time: "2026-04-08T12:00:01Z",
      end_time: "2026-04-08T12:03:13Z",
      duration_millis: 192_000,
      consumed_tokens: [],
      output_mutations: [
        {
          type: "MOVE",
          token_id: "tok-review-in",
          from_place: "story:implemented",
          to_place: "story:ready",
          reason: "review rejected story",
        },
      ],
    },
  ],
};

const completedTraceSnapshot: DashboardTrace = {
  ...traceSnapshot,
  trace_id: "trace-done-story",
  work_ids: [completedWorkID],
  workstation_sequence: ["Complete"],
  dispatches: [
    {
      ...traceSnapshot.dispatches[0],
      dispatch_id: "dispatch-done-story",
      workstation_name: "Complete",
    },
  ],
};

const failedTraceSnapshot: DashboardTrace = {
  ...traceSnapshot,
  trace_id: "trace-failed-story",
  work_ids: [failedWorkID],
  workstation_sequence: ["Review", "Failure"],
  dispatches: [
    {
      ...traceSnapshot.dispatches[0],
      dispatch_id: "dispatch-failed-story",
      outcome: "FAILED",
      workstation_name: "Failure",
    },
  ],
};

const queryClients: QueryClient[] = [];
let restoreBrowserTestShims: (() => void) | null = null;

function timelineSnapshot(
  snapshot: DashboardSnapshot,
  tracesByWorkID: Record<string, DashboardTrace> = {},
  workstationRequestsByDispatchID: Record<
    string,
    DashboardWorkstationRequest
  > = {},
): WorldState {
  return {
    ...snapshot,
    relationsByWorkID: {},
    tracesByWorkID,
    workstationRequestsByDispatchID,
    workRequestsByID: {},
  };
}

function seedTimelineSnapshot(
  snapshot: DashboardSnapshot,
  tracesByWorkID: Record<string, DashboardTrace> = {},
  workstationRequestsByDispatchID: Record<
    string,
    DashboardWorkstationRequest
  > = {},
): void {
  useFactoryTimelineStore.setState({
    events: [],
    latestTick: snapshot.tick_count,
    mode: "current",
    receivedEventIDs: [],
    selectedTick: snapshot.tick_count,
    worldViewCache: {
      [snapshot.tick_count]: timelineSnapshot(
        snapshot,
        tracesByWorkID,
        workstationRequestsByDispatchID,
      ),
    },
  });
}

function seedTimelineSnapshots(snapshots: DashboardSnapshot[]): void {
  const worldViewCache = Object.fromEntries(
    snapshots.map(
      (snapshot) =>
        [
          snapshot.tick_count,
          timelineSnapshot(snapshot) satisfies WorldState,
        ] as const,
    ),
  );
  const latestTick = Math.max(
    ...snapshots.map((snapshot) => snapshot.tick_count),
  );

  useFactoryTimelineStore.setState({
    events: [],
    latestTick,
    mode: "current",
    receivedEventIDs: [],
    selectedTick: latestTick,
    worldViewCache,
  });
}

function renderApp({
  snapshot,
  timelineSnapshots,
  traceFixtures = {},
  workstationRequestsByDispatchID = {},
}: RenderAppOptions) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        gcTime: Infinity,
        retry: false,
      },
    },
  });
  queryClients.push(queryClient);

  const fetchMock = vi
    .fn()
    .mockImplementation(async (input: RequestInfo | URL) => {
      const path =
        typeof input === "string"
          ? input
          : input instanceof URL
            ? `${input.pathname}${input.search}`
            : input.url;

      throw new Error(`unexpected fetch for ${path}`);
    });

  vi.stubGlobal("fetch", fetchMock);
  vi.stubGlobal("EventSource", MockEventSource);
  reloadDashboardLayoutFromStorage();
  if (timelineSnapshots) {
    seedTimelineSnapshots(timelineSnapshots);
  } else {
    seedTimelineSnapshot(
      snapshot,
      traceFixtures,
      workstationRequestsByDispatchID,
    );
  }

  const result = render(
    <QueryClientProvider client={queryClient}>
      <App />
    </QueryClientProvider>,
  );

  return { ...result, fetchMock };
}

function requireValue<T>(value: T | null | undefined, message: string): T {
  if (value === null || value === undefined) {
    throw new Error(message);
  }

  return value;
}

function getDispatchHistoryCard(
  container: HTMLElement,
  dispatchId: string,
): HTMLElement {
  const dispatchBadge = within(container).getAllByText(dispatchId)[0];
  const card = dispatchBadge.closest("article");

  if (!(card instanceof HTMLElement)) {
    throw new Error(`expected dispatch history card for ${dispatchId}`);
  }

  return card;
}

function expectDefinitionValue(
  section: HTMLElement,
  label: string,
  expectedValue: string,
): void {
  const term = within(section).getByText(label, { selector: "dt" });
  const row = term.closest("div");

  if (!(row instanceof HTMLElement)) {
    throw new Error(`expected definition row for ${label}`);
  }

  expect(within(row).getByText(expectedValue)).toBeTruthy();
}

function removeTraceIDFromWorkItem(
  workItem: DashboardWorkItemRef,
): DashboardWorkItemRef {
  const withoutTraceID: DashboardWorkItemRef = { work_id: workItem.work_id };
  if (workItem.display_name) {
    withoutTraceID.display_name = workItem.display_name;
  }
  if (workItem.work_type_id) {
    withoutTraceID.work_type_id = workItem.work_type_id;
  }
  return withoutTraceID;
}

function removeTraceIDsFromSnapshot(
  snapshot: DashboardSnapshot,
): DashboardSnapshot {
  return {
    ...snapshot,
    runtime: {
      ...snapshot.runtime,
      active_executions_by_dispatch_id: Object.fromEntries(
        Object.entries(
          snapshot.runtime.active_executions_by_dispatch_id ?? {},
        ).map(([dispatchID, execution]) => [
          dispatchID,
          {
            ...execution,
            trace_ids: [],
            work_items: execution.work_items?.map(removeTraceIDFromWorkItem),
          },
        ]),
      ),
      current_work_items_by_place_id: Object.fromEntries(
        Object.entries(
          snapshot.runtime.current_work_items_by_place_id ?? {},
        ).map(([placeID, workItems]) => [
          placeID,
          workItems.map(removeTraceIDFromWorkItem),
        ]),
      ),
      session: {
        ...snapshot.runtime.session,
        provider_sessions: snapshot.runtime.session.provider_sessions?.map(
          (attempt) => ({
            ...attempt,
            work_items: attempt.work_items?.map(removeTraceIDFromWorkItem),
          }),
        ),
      },
      workstation_activity_by_node_id: Object.fromEntries(
        Object.entries(
          snapshot.runtime.workstation_activity_by_node_id ?? {},
        ).map(([nodeID, activity]) => [
          nodeID,
          {
            ...activity,
            active_work_items: activity.active_work_items?.map(
              removeTraceIDFromWorkItem,
            ),
            trace_ids: [],
          },
        ]),
      ),
    },
  };
}

function resizeDashboardViewport(width: number): void {
  Object.defineProperty(window, "innerWidth", {
    configurable: true,
    value: width,
    writable: true,
  });
  Object.defineProperty(window, "innerHeight", {
    configurable: true,
    value: width < 720 ? 720 : 900,
    writable: true,
  });
  window.dispatchEvent(new Event("resize"));
}

describe("App current selection", () => {
  beforeEach(() => {
    window.localStorage.clear();
    MockEventSource.instances = [];
    restoreBrowserTestShims = installDashboardBrowserTestShims();
  });

  afterEach(() => {
    for (const queryClient of queryClients.splice(0)) {
      queryClient.clear();
    }
    cleanup();
    useDashboardBentoStore.setState({
      refreshToken: 0,
      selectedTraceID: null,
    });
    useExportDialogStore.setState({
      isExportDialogOpen: false,
    });
    useDashboardStreamStore.setState({
      streamState: createDefaultDashboardStreamState(),
    });
    useFactoryTimelineStore.getState().reset();
    restoreBrowserTestShims?.();
    restoreBrowserTestShims = null;
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it("renders a trace drill-down for a selected work item", async () => {
    renderApp({
      snapshot: activeSnapshot,
      traceFixtures: {
        [activeWorkID]: traceSnapshot,
      },
    });

    expect(
      (await screen.findAllByText("dispatch-review-active")).length,
    ).toBeGreaterThan(0);
    fireEvent.click(
      (await screen.findAllByRole("button", { name: /Active Story/ }))[0],
    );

    const currentSelection = await screen.findByRole("article", {
      name: "Current selection",
    });
    expect(
      within(currentSelection).queryByRole("heading", {
        name: "Execution details",
      }),
    ).toBeNull();
    expect(
      within(currentSelection).getByText(
        "codex / session_id / sess-active-story",
      ),
    ).toBeTruthy();
    expect(
      document
        .querySelector("[data-bento-card-id='trace']")
        ?.getAttribute("id"),
    ).toBe("trace");
    expect(
      within(currentSelection).getByRole("heading", {
        name: "Workstation dispatches",
      }),
    ).toBeTruthy();
    expect(
      within(currentSelection).getAllByText(
        /codex \/ session_id \/ sess-active-story/,
      )[0],
    ).toBeTruthy();
    expect(
      within(currentSelection).queryByRole("heading", {
        name: "Work session runs list",
      }),
    ).toBeNull();
    const traceCard = screen.getByRole("article", { name: "Trace drill-down" });
    expect(traceCard).toBeTruthy();
    expect(within(traceCard).getByText("Trace dispatch grid")).toBeTruthy();
    expect(within(traceCard).getByText("Accepted · 1s")).toBeTruthy();
    expect(within(traceCard).queryByText("Workstation run")).toBeNull();
    expect(within(traceCard).queryByText("Consumed tokens")).toBeNull();
    expect(within(traceCard).queryByText("Output mutations")).toBeNull();
  });

  it("renders one selected-work dispatch history list with mixed inference and script-backed rows", async () => {
    renderApp({
      snapshot: activeSnapshot,
      traceFixtures: {
        [activeWorkID]: traceSnapshot,
      },
      workstationRequestsByDispatchID: {
        [dashboardWorkstationRequestFixtures.noResponse.dispatch_id]:
          dashboardWorkstationRequestFixtures.noResponse,
        [dashboardWorkstationRequestFixtures.ready.dispatch_id]:
          dashboardWorkstationRequestFixtures.ready,
        [dashboardWorkstationRequestFixtures.rejected.dispatch_id]:
          dashboardWorkstationRequestFixtures.rejected,
        [dashboardWorkstationRequestFixtures.errored.dispatch_id]:
          dashboardWorkstationRequestFixtures.errored,
        [dashboardWorkstationRequestFixtures.scriptSuccess.dispatch_id]:
          dashboardWorkstationRequestFixtures.scriptSuccess,
        [dashboardWorkstationRequestFixtures.scriptFailed.dispatch_id]:
          dashboardWorkstationRequestFixtures.scriptFailed,
      },
    });

    fireEvent.click(
      (await screen.findAllByRole("button", { name: /Active Story/ }))[0],
    );

    const currentSelection = await screen.findByRole("article", {
      name: "Current selection",
    });
    const dispatchHistory = within(currentSelection).getByRole("region", {
      name: "Workstation dispatches",
    });

    expect(
      within(currentSelection).getByRole("heading", {
        name: "Workstation dispatches",
      }),
    ).toBeTruthy();
    expect(
      within(currentSelection).queryByRole("heading", {
        name: "Work session runs list",
      }),
    ).toBeNull();
    expect(within(dispatchHistory).getByText("6 dispatches")).toBeTruthy();

    const pendingCard = getDispatchHistoryCard(
      dispatchHistory,
      dashboardWorkstationRequestFixtures.noResponse.dispatch_id,
    );
    expect(
      within(pendingCard).getByText(
        "Review the active story while the provider response is still pending.",
      ),
    ).toBeTruthy();
    expect(
      within(pendingCard).getByText("No response yet for this dispatch."),
    ).toBeTruthy();

    const readyCard = getDispatchHistoryCard(
      dispatchHistory,
      dashboardWorkstationRequestFixtures.ready.dispatch_id,
    );
    expect(
      within(readyCard).getByText(
        "Review the active story and decide whether it is ready.",
      ),
    ).toBeTruthy();
    expect(
      within(readyCard).getAllByText("Ready for the next workstation.").length,
    ).toBeGreaterThan(0);

    const rejectedCard = getDispatchHistoryCard(
      dispatchHistory,
      dashboardWorkstationRequestFixtures.rejected.dispatch_id,
    );
    expect(
      within(rejectedCard).getByText(
        "Review the active story and explain what needs to change before approval.",
      ),
    ).toBeTruthy();
    expect(
      within(rejectedCard).getAllByText(
        "The active story needs revision before it can continue.",
      ).length,
    ).toBeGreaterThan(0);

    const erroredCard = getDispatchHistoryCard(
      dispatchHistory,
      dashboardWorkstationRequestFixtures.errored.dispatch_id,
    );
    expect(
      within(erroredCard).getByText(
        "Review the blocked story and explain the failure.",
      ),
    ).toBeTruthy();
    expect(
      within(erroredCard).getByText(
        "Provider rate limit exceeded while reviewing the story.",
      ),
    ).toBeTruthy();
    expect(
      within(erroredCard).getByText(
        "Response text is unavailable because this dispatch ended with an error.",
      ),
    ).toBeTruthy();

    const scriptSuccessCard = getDispatchHistoryCard(
      dispatchHistory,
      dashboardWorkstationRequestFixtures.scriptSuccess.dispatch_id,
    );
    expect(
      within(scriptSuccessCard).getAllByText("script-tool").length,
    ).toBeGreaterThan(0);
    expect(
      within(scriptSuccessCard).getByRole("region", { name: "Script attempts" }),
    ).toBeTruthy();
    expect(
      within(scriptSuccessCard).getAllByText("script success stdout").length,
    ).toBeGreaterThan(0);
    expect(
      within(scriptSuccessCard).getAllByText("SUCCEEDED").length,
    ).toBeGreaterThan(0);

    const scriptFailedCard = getDispatchHistoryCard(
      dispatchHistory,
      dashboardWorkstationRequestFixtures.scriptFailed.dispatch_id,
    );
    expect(
      within(scriptFailedCard).getAllByText("TIMEOUT").length,
    ).toBeGreaterThan(0);
    expect(
      within(scriptFailedCard).getAllByText("script timed out").length,
    ).toBeGreaterThan(0);
    expect(
      within(scriptFailedCard).getByText(
        "Prompt details are not applicable to this script-backed dispatch.",
      ),
    ).toBeTruthy();
  });

  it("renders the selected-work empty dispatch-history state without reviving top-level execution details", async () => {
    const snapshotWithoutSelectedWorkDispatchHistory = {
      ...activeSnapshot,
      runtime: {
        ...activeSnapshot.runtime,
        session: {
          ...activeSnapshot.runtime.session,
          provider_sessions: [],
        },
        workstation_requests_by_dispatch_id: {},
      },
    } satisfies DashboardSnapshot;

    renderApp({
      snapshot: snapshotWithoutSelectedWorkDispatchHistory,
      traceFixtures: {
        [activeWorkID]: traceSnapshot,
      },
    });

    fireEvent.click(
      (await screen.findAllByRole("button", { name: /Active Story/ }))[0],
    );

    const currentSelection = await screen.findByRole("article", {
      name: "Current selection",
    });
    expect(
      within(currentSelection).queryByRole("heading", {
        name: "Execution details",
      }),
    ).toBeNull();
    expect(
      within(currentSelection).queryByRole("heading", {
        name: "Inference attempts",
      }),
    ).toBeNull();
    expectDefinitionValue(currentSelection, "Workstation dispatches", "0");
    expect(
      within(currentSelection).getByText(
        "No workstation dispatch has been recorded yet for this work item.",
      ),
    ).toBeTruthy();
    expect(
      within(currentSelection).getByRole("heading", {
        name: "Workstation dispatches",
      }),
    ).toBeTruthy();
    expect(
      await screen.findByRole("article", { name: "Trace drill-down" }),
    ).toBeTruthy();
  });

  it("renders selected work item trace unavailable copy when no trace ID exists", async () => {
    renderApp({
      snapshot: activeSnapshotWithoutTraceID,
    });

    fireEvent.click(
      (await screen.findAllByRole("button", { name: /Active Story/ }))[0],
    );

    const currentSelection = await screen.findByRole("article", {
      name: "Current selection",
    });
    expect(
      within(currentSelection).queryByRole("heading", {
        name: "Execution details",
      }),
    ).toBeNull();
    expect(
      await screen.findByRole("article", { name: "Trace drill-down" }),
    ).toBeTruthy();
    expect(await screen.findByText("Trace history unavailable")).toBeTruthy();
  });

  it("keeps workstation and work-item selection usable after React Flow zoom", async () => {
    renderApp({
      snapshot: activeSnapshot,
      traceFixtures: {
        [activeWorkID]: traceSnapshot,
      },
    });

    await screen.findAllByText("dispatch-review-active");

    const workGraphViewport = screen.getByRole("region", {
      name: "Work graph viewport",
    });
    fireEvent.click(
      within(workGraphViewport).getByRole("button", { name: "Zoom In" }),
    );

    fireEvent.click(
      await screen.findByRole("button", { name: "Select Plan workstation" }),
    );
    await waitFor(() => {
      expect(screen.getAllByText("planner").length).toBeGreaterThanOrEqual(1);
    });
    expect(screen.getByText("Input work types")).toBeTruthy();

    fireEvent.click(
      (await screen.findAllByRole("button", { name: /Active Story/ }))[0],
    );
    expect(await screen.findByText("Trace drill-down")).toBeTruthy();
  });

  it("separates workstation selection from active work selection", async () => {
    renderApp({
      snapshot: activeSnapshot,
      traceFixtures: {
        [activeWorkID]: traceSnapshot,
      },
    });

    await screen.findAllByText("dispatch-review-active");

    const reviewButton = await screen.findByRole("button", {
      name: "Select Review workstation",
    });
    fireEvent.click(reviewButton);
    await waitFor(() => {
      expect(reviewButton.getAttribute("aria-pressed")).toBe("true");
    });
    const reviewNode = reviewButton.closest("[data-workstation-kind]");
    const updatedReviewNode = screen
      .getByRole("button", { name: "Select Review workstation" })
      .closest("[data-workstation-kind]");
    expect(
      updatedReviewNode?.getAttribute("data-selected-workstation") === "true",
    ).toBe(true);
    expect(
      updatedReviewNode?.getAttribute("data-selected-work") === "true",
    ).toBe(false);
    expect(
      screen.getByRole("heading", { name: "Current selection" }),
    ).toBeTruthy();

    const workButton = (
      await screen.findAllByRole("button", { name: /Active Story/ })
    )[0];
    fireEvent.click(workButton);

    await waitFor(() => {
      expect(
        screen.getByRole("heading", { name: "Current selection" }),
      ).toBeTruthy();
    });
    expect(reviewButton.getAttribute("aria-pressed")).toBe("false");
    expect(workButton.getAttribute("aria-pressed")).toBe("true");
    expect(
      reviewNode?.getAttribute("data-selected-workstation") === "true",
    ).toBe(false);
    expect(reviewNode?.getAttribute("data-selected-work") === "true").toBe(
      true,
    );
    expect(workButton.getAttribute("data-selected") === "true").toBe(true);

    fireEvent.click(
      await screen.findByRole("button", { name: "Select Plan workstation" }),
    );

    await waitFor(() => {
      expect(
        screen.getByRole("heading", { name: "Current selection" }),
      ).toBeTruthy();
    });
    const planNode = screen
      .getByRole("button", { name: "Select Plan workstation" })
      .closest("[data-workstation-kind]");
    const restoredReviewNode = screen
      .getByRole("button", { name: "Select Review workstation" })
      .closest("[data-workstation-kind]");
    expect(planNode?.getAttribute("data-selected-workstation") === "true").toBe(
      true,
    );
    expect(
      restoredReviewNode?.getAttribute("data-selected-work") === "true",
    ).toBe(false);
    expect(workButton.getAttribute("aria-pressed")).toBe("false");
  });

  it("shows active executions from the selected workstation instead of provider history", async () => {
    const reviewExecution =
      activeSnapshot.runtime.active_executions_by_dispatch_id?.[
        "dispatch-review-active"
      ];
    const resolvedReviewExecution = requireValue(
      reviewExecution,
      "expected active review execution fixture",
    );

    const snapshot = {
      ...activeSnapshot,
      runtime: {
        ...activeSnapshot.runtime,
        active_dispatch_ids: ["dispatch-review-active", "dispatch-plan-active"],
        active_executions_by_dispatch_id: {
          "dispatch-plan-active": {
            ...resolvedReviewExecution,
            dispatch_id: "dispatch-plan-active",
            started_at: "2026-04-08T12:00:01Z",
            trace_ids: ["trace-plan-active"],
            transition_id: "plan",
            workstation_name: "Plan",
            workstation_node_id: "plan",
            work_items: [
              {
                display_name: "Plan Active",
                trace_id: "trace-plan-active",
                work_id: "work-plan-active",
                work_type_id: "story",
              },
            ],
          },
          "dispatch-review-active": {
            ...resolvedReviewExecution,
            transition_id: "review",
            workstation_name: "Review",
            workstation_node_id: "legacy-review-node",
          },
        },
        active_workstation_node_ids: ["review", "plan"],
        session: {
          ...activeSnapshot.runtime.session,
          provider_sessions: [],
        },
      },
    } satisfies DashboardSnapshot;

    renderApp({ snapshot });

    fireEvent.click(
      await screen.findByRole("button", { name: "Select Review workstation" }),
    );

    const reviewInfo = await screen.findByRole("article", {
      name: "Current selection",
    });
    await waitFor(() => {
      expect(
        within(reviewInfo).getByRole("heading", { name: "Active work" }),
      ).toBeTruthy();
      expect(within(reviewInfo).getByText(activeWorkLabel)).toBeTruthy();
      expect(within(reviewInfo).getByText(activeWorkID)).toBeTruthy();
      expect(
        within(reviewInfo).getAllByText("dispatch-review-active").length,
      ).toBeGreaterThan(0);
      expect(within(reviewInfo).queryByText("Plan Active")).toBeNull();
    });

    fireEvent.click(
      await screen.findByRole("button", { name: "Select Plan workstation" }),
    );

    const planInfo = await screen.findByRole("article", {
      name: "Current selection",
    });
    await waitFor(() => {
      expect(within(planInfo).getByText("Plan Active")).toBeTruthy();
      expect(within(planInfo).getByText("work-plan-active")).toBeTruthy();
      expect(
        within(planInfo).getAllByText("dispatch-plan-active").length,
      ).toBeGreaterThan(0);
      expect(within(planInfo).queryByText(activeWorkLabel)).toBeNull();
    });

    fireEvent.click(
      await screen.findByRole("button", {
        name: "Select Implement workstation",
      }),
    );

    const implementInfo = await screen.findByRole("article", {
      name: "Current selection",
    });
    await waitFor(() => {
      expect(
        within(implementInfo).getByText(
          "No active work is running on this workstation.",
        ),
      ).toBeTruthy();
      expect(within(implementInfo).queryByText(activeWorkLabel)).toBeNull();
      expect(within(implementInfo).queryByText("Plan Active")).toBeNull();
    });
  });

  it("shows selected state node details from the graph", async () => {
    renderApp({ snapshot: activeSnapshot });

    const stateButton = await screen.findByRole("button", {
      name: "Select story:implemented state",
    });
    fireEvent.click(stateButton);

    const stateInfo = await screen.findByRole("article", {
      name: "Current selection",
    });
    const stateSelectionSlot = stateInfo.closest("[data-bento-card-id]");
    expect(stateButton.getAttribute("aria-pressed")).toBe("true");
    expect(stateSelectionSlot?.getAttribute("data-bento-card-id")).toBe(
      "current-selection",
    );
    expect(within(stateInfo).getByTitle("story:implemented")).toBeTruthy();
    expect(within(stateInfo).getByText("Count")).toBeTruthy();
    expect(within(stateInfo).getByText("Current work")).toBeTruthy();
    expect(within(stateInfo).getByText(activeWorkLabel)).toBeTruthy();
    expect(within(stateInfo).getByText(activeWorkID)).toBeTruthy();

    fireEvent.click(
      within(stateInfo).getByRole("button", {
        name: "Select work item Active Story",
      }),
    );

    const selectedWorkInfo = await screen.findByRole("article", {
      name: "Current selection",
    });
    expect(
      within(selectedWorkInfo).queryByRole("heading", {
        name: "Execution details",
      }),
    ).toBeNull();

    fireEvent.click(
      await screen.findByRole("button", { name: "Select story:blocked state" }),
    );

    const emptyStateInfo = await screen.findByRole("article", {
      name: "Current selection",
    });
    expect(
      within(emptyStateInfo).getAllByText("blocked").length,
    ).toBeGreaterThan(0);
    expect(within(emptyStateInfo).getByTitle("story:blocked")).toBeTruthy();
    expect(
      within(emptyStateInfo).getByText(
        "No work is recorded for this place at the selected tick.",
      ),
    ).toBeTruthy();

    fireEvent.click(
      await screen.findByRole("button", { name: "Select Review workstation" }),
    );

    const workstationInfo = await screen.findByRole("article", {
      name: "Current selection",
    });
    const workstationSelectionSlot = workstationInfo.closest(
      "[data-bento-card-id]",
    );
    expect(workstationInfo).toBeTruthy();
    expect(workstationSelectionSlot).toBe(stateSelectionSlot);
    expect(within(workstationInfo).getByText("Input work types")).toBeTruthy();
  });
});

describe("App current selection layout", () => {
  beforeEach(() => {
    window.localStorage.clear();
    MockEventSource.instances = [];
    restoreBrowserTestShims = installDashboardBrowserTestShims();
  });

  afterEach(() => {
    for (const queryClient of queryClients.splice(0)) {
      queryClient.clear();
    }
    cleanup();
    useDashboardBentoStore.setState({
      refreshToken: 0,
      selectedTraceID: null,
    });
    useExportDialogStore.setState({
      isExportDialogOpen: false,
    });
    useDashboardStreamStore.setState({
      streamState: createDefaultDashboardStreamState(),
    });
    useFactoryTimelineStore.getState().reset();
    restoreBrowserTestShims?.();
    restoreBrowserTestShims = null;
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it("keeps selection detail out of the workflow graph inspector layer", async () => {
    renderApp({
      snapshot: activeSnapshot,
      traceFixtures: {
        [activeWorkID]: traceSnapshot,
      },
    });

    await screen.findAllByText("dispatch-review-active");

    expect(
      screen.getByRole("region", { name: "Work graph viewport" }),
    ).toBeTruthy();
    expect(
      screen.queryByRole("complementary", { name: "Workstation Info" }),
    ).toBeNull();
    expect(
      screen.queryByRole("button", { name: "Collapse inspector" }),
    ).toBeNull();
    expect(
      screen.queryByRole("button", { name: "Expand inspector" }),
    ).toBeNull();
    expect(
      screen.getByRole("article", { name: "Current selection" }),
    ).toBeTruthy();
  });

  it("renders selected work and traces on the shared dashboard grid", async () => {
    renderApp({
      snapshot: activeSnapshot,
      traceFixtures: {
        [activeWorkID]: traceSnapshot,
      },
    });

    fireEvent.click(
      (await screen.findAllByRole("button", { name: /Active Story/ }))[0],
    );

    const dashboardGrid = screen.getByRole("region", {
      name: "Agent Factory bento board",
    });
    const workInfo = await within(dashboardGrid).findByRole("article", {
      name: "Current selection",
    });
    expect(workInfo).toBeTruthy();
    expect(screen.getByLabelText("Work graph viewport")).toBeTruthy();
    expect(
      within(dashboardGrid).getByRole("article", {
        name: "Completed and failed work",
      }),
    ).toBeTruthy();
    expect(
      within(dashboardGrid).getByRole("article", { name: "Trace drill-down" }),
    ).toBeTruthy();
    expect(within(dashboardGrid).getByText("Trace drill-down")).toBeTruthy();
    expect(
      await within(dashboardGrid).findByText("Trace dispatch grid"),
    ).toBeTruthy();
  });

  it("supports rearranging shared-grid widgets without replacing graph selection", async () => {
    renderApp({
      snapshot: activeSnapshot,
      traceFixtures: {
        [activeWorkID]: traceSnapshot,
      },
    });

    fireEvent.click(
      (await screen.findAllByRole("button", { name: /Active Story/ }))[0],
    );

    const dashboardGrid = screen.getByRole("region", {
      name: "Agent Factory bento board",
    });
    const traceWidget = await within(dashboardGrid).findByRole("article", {
      name: "Trace drill-down",
    });
    const traceGridItem = traceWidget.closest(
      ".react-grid-item",
    ) as HTMLElement;
    const initialStyle = traceGridItem.getAttribute("style");

    fireEvent.mouseDown(
      within(traceWidget).getByRole("button", {
        name: "Move Trace drill-down",
      }),
      {
        button: 0,
        buttons: 1,
        clientX: 120,
        clientY: 40,
      },
    );
    fireEvent.mouseMove(document, {
      buttons: 1,
      clientX: 360,
      clientY: 40,
    });
    fireEvent.mouseUp(document, {
      button: 0,
      clientX: 360,
      clientY: 40,
    });

    await waitFor(() => {
      expect(traceGridItem.getAttribute("style")).not.toBe(initialStyle);
    });
    const storedLayout = window.localStorage.getItem(
      "agent-factory.dashboard.layout.v2",
    );
    expect(storedLayout).toContain('"id":"trace"');

    const movedStyle = traceGridItem.getAttribute("style");
    const stream = MockEventSource.instances[0];
    if (!stream) {
      throw new Error("expected dashboard stream to be opened");
    }

    act(() => {
      stream.emit("snapshot", {
        ...activeSnapshot,
        tick_count: activeSnapshot.tick_count + 1,
      } satisfies DashboardSnapshot);
    });

    await waitFor(() => {
      expect(traceGridItem.getAttribute("style")).toBe(movedStyle);
    });
    expect(
      (
        await screen.findByRole("button", { name: "Select Review workstation" })
      ).getAttribute("aria-pressed"),
    ).toBe("false");
    expect(
      await within(dashboardGrid).findByText("Trace dispatch grid"),
    ).toBeTruthy();
  });

  it("renders queued, in-flight, completed, and failed work in one ranged outcome chart", async () => {
    renderApp({ snapshot: baselineSnapshot });

    await screen.findByRole("heading", { name: "Agent Factory" });
    const dashboardGrid = screen.getByRole("region", {
      name: "Agent Factory bento board",
    });
    const trendWidget = await within(dashboardGrid).findByRole("article", {
      name: "Work outcome chart",
    });

    expect(
      within(trendWidget).queryByRole("combobox", { name: "Time range" }),
    ).toBeNull();
    expect(
      within(trendWidget).getByRole("img", {
        name: "Work outcome chart for Session",
      }),
    ).toBeTruthy();
    expect(
      within(trendWidget).queryByRole("list", { name: "Work outcome totals" }),
    ).toBeNull();
    expect(within(trendWidget).getByText("Queued")).toBeTruthy();
    expect(within(trendWidget).getByText("In-flight")).toBeTruthy();
    expect(within(trendWidget).getByText("Completed")).toBeTruthy();
    expect(within(trendWidget).getByText("Failed/retried")).toBeTruthy();
    expect(within(trendWidget).getByText("Ticks")).toBeTruthy();
    expect(within(trendWidget).getByText("Work count")).toBeTruthy();

    const stream = MockEventSource.instances[0];
    if (!stream) {
      throw new Error("expected dashboard stream to be opened");
    }

    act(() => {
      stream.emit("snapshot", {
        ...baselineSnapshot,
        tick_count: baselineSnapshot.tick_count + 1,
        runtime: {
          ...baselineSnapshot.runtime,
          in_flight_dispatch_count: 3,
          place_token_counts: {
            ...(baselineSnapshot.runtime.place_token_counts ?? {}),
            "story:init": 5,
          },
          session: {
            ...baselineSnapshot.runtime.session,
            completed_count: 4,
            dispatched_count: 7,
            failed_by_work_type: { story: 2 },
            failed_count: 2,
            failed_work_labels: ["Blocked Story", "Rejected Story"],
          },
        },
      } satisfies DashboardSnapshot);
    });

    await waitFor(() => {
      expect(
        within(trendWidget).getByRole("img", {
          name: "Work outcome chart for Session",
        }),
      ).toBeTruthy();
    });

    expect(
      within(trendWidget).getByRole("img", {
        name: "Work outcome chart for Session",
      }),
    ).toBeTruthy();
  });

  it("updates work outcome chart values when toggling timeline mode between fixed and live snapshots", async () => {
    const historicalWorkOutcomeSnapshot = {
      ...baselineSnapshot,
      tick_count: 1,
      runtime: {
        ...baselineSnapshot.runtime,
        in_flight_dispatch_count: 1,
        place_token_counts: {
          ...(baselineSnapshot.runtime.place_token_counts ?? {}),
          "story:init": 2,
        },
        session: {
          ...baselineSnapshot.runtime.session,
          completed_count: 1,
          completed_work_labels: ["Old Story"],
          failed_count: 0,
          dispatched_count: 2,
          failed_by_work_type: {},
          failed_work_labels: [],
        },
      },
    };
    const liveWorkOutcomeSnapshot = {
      ...historicalWorkOutcomeSnapshot,
      tick_count: 4,
      runtime: {
        ...historicalWorkOutcomeSnapshot.runtime,
        in_flight_dispatch_count: 2,
        place_token_counts: {
          ...(historicalWorkOutcomeSnapshot.runtime.place_token_counts ?? {}),
          "story:init": 4,
        },
        session: {
          ...historicalWorkOutcomeSnapshot.runtime.session,
          completed_count: 8,
          completed_work_labels: ["Old Story", "New Story"],
          failed_count: 3,
          dispatched_count: 10,
          failed_by_work_type: { story: 3 },
          failed_work_labels: [
            "Blocked Story",
            "Rejected Story",
            "Reworked Story",
          ],
        },
      },
    };

    renderApp({
      snapshot: historicalWorkOutcomeSnapshot,
      timelineSnapshots: [
        historicalWorkOutcomeSnapshot,
        liveWorkOutcomeSnapshot,
      ],
    });

    const dashboardGrid = screen.getByRole("region", {
      name: "Agent Factory bento board",
    });
    const trendWidget = await within(dashboardGrid).findByRole("article", {
      name: "Work outcome chart",
    });
    const slider = screen.getByRole<HTMLInputElement>("slider", {
      name: "Timeline tick",
    });

    expect(
      within(trendWidget).getByRole("region", {
        name: "Work outcome chart region",
      }),
    ).toBeTruthy();
    fireEvent.change(slider, { target: { value: "1" } });
    await waitFor(() => {
      expect(
        within(trendWidget).getByRole("img", {
          name: "Work outcome chart for Session",
        }),
      ).toBeTruthy();
    });

    fireEvent.change(slider, { target: { value: "4" } });
    await waitFor(() => {
      expect(
        within(trendWidget).getByRole("img", {
          name: "Work outcome chart for Session",
        }),
      ).toBeTruthy();
    });
  });

  it("keeps retry, rework, and timing trends hidden when selected trace data is available", async () => {
    renderApp({
      snapshot: terminalSnapshot,
      traceFixtures: {
        [activeWorkID]: reworkTraceSnapshot,
      },
    });

    await screen.findByRole("heading", { name: "Agent Factory" });
    const dashboardGrid = screen.getByRole("region", {
      name: "Agent Factory bento board",
    });
    expect(
      within(dashboardGrid).queryByRole("article", { name: "Failure trend" }),
    ).toBeNull();

    fireEvent.click(
      (await screen.findAllByRole("button", { name: /Active Story/ }))[0],
    );

    const workDetail = screen.getByRole("region", {
      name: "Agent Factory bento board",
    });
    expect(
      await within(workDetail).findByRole("article", {
        name: "Current selection",
      }),
    ).toBeTruthy();
    expect(
      within(workDetail).getByRole("article", { name: "Trace drill-down" }),
    ).toBeTruthy();
    expect(
      within(workDetail).queryByRole("article", {
        name: "Retry and rework trend",
      }),
    ).toBeNull();
    expect(
      within(workDetail).queryByRole("article", { name: "Timing trend" }),
    ).toBeNull();
    expect(
      workDetail.querySelector('[data-bento-card-id="rework-trend"]'),
    ).toBeNull();
    expect(
      workDetail.querySelector('[data-bento-card-id="timing-trend"]'),
    ).toBeNull();
  });

  it.each([
    1366, 1024, 640,
  ])("keeps the widget cards readable at %ipx viewport width", async (viewportWidth) => {
    resizeDashboardViewport(viewportWidth);
    renderApp({
      snapshot: terminalSnapshot,
      traceFixtures: {
        [activeWorkID]: reworkTraceSnapshot,
      },
    });

    fireEvent.click(
      (await screen.findAllByRole("button", { name: /Active Story/ }))[0],
    );

    const dashboardGrid = screen.getByRole("region", {
      name: "Agent Factory bento board",
    });

    const widgets = within(dashboardGrid).getAllByRole("article");
    const widgetNames = widgets.map(
      (widget) => widget.getAttribute("aria-label") ?? "",
    );

    expect(widgetNames).toContain("Work outcome chart");
    expect(widgetNames).toContain("Submit work");
    expect(widgetNames).not.toContain("Completion trend");
    expect(widgetNames).not.toContain("Failure trend");
    expect(widgetNames).not.toContain("Retry and rework trend");
    expect(widgetNames).not.toContain("Timing trend");
    expect(widgetNames).toContain("Completed and failed work");
    expect(widgetNames).toContain("Current selection");
    expect(widgetNames).toContain("Trace drill-down");
    const bentoItems = Array.from(
      dashboardGrid.querySelectorAll<HTMLElement>("[data-bento-card-id]"),
    );
    const cardIds = bentoItems.map((item) => item.dataset.bentoCardId);
    expect(cardIds).toContain("work-outcome-chart");
    expect(cardIds).toContain("submit-work");
    expect(cardIds).not.toContain("completion-trend");
    expect(cardIds).not.toContain("failure-trend");
    expect(cardIds).not.toContain("rework-trend");
    expect(cardIds).not.toContain("timing-trend");
    expect(cardIds).toContain("terminal-work");
    expect(cardIds).toContain("trace");
    expect(cardIds).toContain("current-selection");

    expect(
      within(dashboardGrid).getByRole("img", {
        name: "Work outcome chart for Session",
      }),
    ).toBeTruthy();
    expect(
      within(dashboardGrid).queryByRole("img", {
        name: `Timing trend for ${activeWorkID}`,
      }),
    ).toBeNull();
  });

  it("smoke tests the composed bento dashboard at a narrow viewport", async () => {
    resizeDashboardViewport(640);
    renderApp({
      snapshot: terminalSnapshot,
      traceFixtures: {
        [activeWorkID]: traceSnapshot,
      },
    });

    await screen.findByRole("heading", { name: "Agent Factory" });
    expect(
      screen.getAllByRole("region", { name: "Agent Factory bento board" }),
    ).toHaveLength(1);
    expect(screen.getByRole("article", { name: "Factory graph" })).toBeTruthy();
    expect(
      screen.getByRole("region", { name: "Work graph viewport" }),
    ).toBeTruthy();

    const activeWorkButton = (
      await screen.findAllByRole("button", { name: /Active Story/ })
    )[0];
    fireEvent.click(activeWorkButton);

    const dashboardGrid = screen.getByRole("region", {
      name: "Agent Factory bento board",
    });
    expect(
      within(dashboardGrid).getByRole("article", {
        name: "Work outcome chart",
      }),
    ).toBeTruthy();
    expect(
      within(dashboardGrid).getByRole("article", { name: "Submit work" }),
    ).toBeTruthy();
    expect(
      within(dashboardGrid).getByRole("img", {
        name: "Work outcome chart for Session",
      }),
    ).toBeTruthy();
    expect(
      within(dashboardGrid).getByRole("article", { name: "Trace drill-down" }),
    ).toBeTruthy();
    expect(
      within(dashboardGrid).getByRole("article", {
        name: "Completed and failed work",
      }),
    ).toBeTruthy();
    expect(
      await within(dashboardGrid).findByText("Trace dispatch grid"),
    ).toBeTruthy();
    await waitFor(() => {
      expect(
        screen
          .getAllByRole("button", { name: /Active Story/ })[0]
          ?.getAttribute("aria-pressed"),
      ).toBe("true");
    });

    const outcomeWidget = within(dashboardGrid).getByRole("article", {
      name: "Work outcome chart",
    });
    const outcomeGridItem = outcomeWidget.closest(
      ".react-grid-item",
    ) as HTMLElement;
    const initialOutcomeStyle = outcomeGridItem.getAttribute("style");

    fireEvent.mouseDown(
      within(outcomeWidget).getByRole("button", {
        name: "Move Work outcome chart",
      }),
      {
        button: 0,
        buttons: 1,
        clientX: 120,
        clientY: 40,
      },
    );
    fireEvent.mouseMove(document, {
      buttons: 1,
      clientX: 360,
      clientY: 40,
    });
    fireEvent.mouseUp(document, {
      button: 0,
      clientX: 360,
      clientY: 40,
    });

    await waitFor(() => {
      expect(outcomeGridItem.getAttribute("style")).not.toBe(
        initialOutcomeStyle,
      );
    });

    const terminalWidget = within(dashboardGrid).getByRole("article", {
      name: "Completed and failed work",
    });
    const completedRow = within(terminalWidget)
      .getByRole("heading", { name: "Completed" })
      .closest("section");
    const failedRow = within(terminalWidget)
      .getByRole("heading", { name: "Failed" })
      .closest("section");

    if (
      !(completedRow instanceof HTMLElement) ||
      !(failedRow instanceof HTMLElement)
    ) {
      throw new Error(
        "expected completed and failed rows to render as terminal sections",
      );
    }

    fireEvent.click(
      within(completedRow).getByRole("button", { name: "Collapse" }),
    );
    fireEvent.click(
      within(failedRow).getByRole("button", { name: "Collapse" }),
    );
    fireEvent.click(
      within(completedRow).getByRole("button", { name: "Expand" }),
    );
    fireEvent.click(within(failedRow).getByRole("button", { name: "Expand" }));

    expect(
      within(completedRow).getByRole("button", { name: "Done Story" }),
    ).toBeTruthy();
    expect(
      within(failedRow).getByRole("button", { name: "Failed Story" }),
    ).toBeTruthy();
    expect(document.documentElement.scrollWidth).toBeLessThanOrEqual(
      window.innerWidth,
    );
  });
});

describe("App current selection terminal states", () => {
  beforeEach(() => {
    window.localStorage.clear();
    MockEventSource.instances = [];
    restoreBrowserTestShims = installDashboardBrowserTestShims();
  });

  afterEach(() => {
    for (const queryClient of queryClients.splice(0)) {
      queryClient.clear();
    }
    cleanup();
    useDashboardBentoStore.setState({
      refreshToken: 0,
      selectedTraceID: null,
    });
    useExportDialogStore.setState({
      isExportDialogOpen: false,
    });
    useDashboardStreamStore.setState({
      streamState: createDefaultDashboardStreamState(),
    });
    useFactoryTimelineStore.getState().reset();
    restoreBrowserTestShims?.();
    restoreBrowserTestShims = null;
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it("opens completed and failed work summaries and updates the trace card", async () => {
    renderApp({
      snapshot: terminalSnapshot,
      traceFixtures: {
        [completedWorkID]: completedTraceSnapshot,
        [failedWorkID]: failedTraceSnapshot,
      },
    });

    await screen.findByRole("heading", { name: "Agent Factory" });
    const dashboardGrid = screen.getByRole("region", {
      name: "Agent Factory bento board",
    });
    fireEvent.click(
      within(dashboardGrid).getByRole("button", { name: "Done Story" }),
    );

    const completedDetail = await screen.findByRole("article", {
      name: "Current selection",
    });
    expect(within(completedDetail).getByText("Done Story")).toBeTruthy();
    expect(
      within(completedDetail).queryByRole("heading", {
        name: "Execution details",
      }),
    ).toBeNull();
    expect(within(completedDetail).queryByText("Failure reason")).toBeNull();
    expect(completedDetail).toBeTruthy();
    expect(
      await within(dashboardGrid).findByText("dispatch-done-story"),
    ).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Failed Story" }));

    const failedDetail = await screen.findByRole("article", {
      name: "Current selection",
    });
    expect(within(failedDetail).getByText("Failed Story")).toBeTruthy();
    expect(
      within(failedDetail).queryByRole("heading", {
        name: "Execution details",
      }),
    ).toBeNull();
    expect(
      within(failedDetail).getByRole("heading", {
        name: "Workstation dispatches",
      }),
    ).toBeTruthy();
    expect(
      within(failedDetail).getAllByText(/FAILED|Failed/).length,
    ).toBeGreaterThanOrEqual(1);
    expect(within(failedDetail).queryByText("Failure reason")).toBeNull();
    expect(within(failedDetail).getByText("Current dispatch")).toBeTruthy();
    expect(
      within(failedDetail).getByText("Session log unavailable"),
    ).toBeTruthy();
    expect(
      within(failedDetail).getByText("codex / session_id / sess-failed-story"),
    ).toBeTruthy();
    expect(
      within(failedDetail).queryByText(
        "Terminal summaries are reconstructed from retained runtime state.",
      ),
    ).toBeNull();
    expect(
      await within(dashboardGrid).findByText("dispatch-repair-failed"),
    ).toBeTruthy();
  });

  it("shows terminal and failed state occupancy in current-selection details", async () => {
    renderApp({
      snapshot: terminalSnapshot,
      timelineSnapshots: [historicalTimelineSnapshot, terminalSnapshot],
    });

    await screen.findByRole("heading", { name: "Agent Factory" });
    const dashboardGrid = screen.getByRole("region", {
      name: "Agent Factory bento board",
    });

    fireEvent.click(
      await screen.findByRole("button", {
        name: "Select story:complete state",
      }),
    );

    const completedDetail = await within(dashboardGrid).findByRole("article", {
      name: "Current selection",
    });
    expect(within(completedDetail).getByTitle("story:complete")).toBeTruthy();
    expect(within(completedDetail).getByText("Count")).toBeTruthy();
    expect(within(completedDetail).getByText("Current work")).toBeTruthy();
    expect(within(completedDetail).getByText("Done Story")).toBeTruthy();
    expect(within(completedDetail).getByText(completedWorkID)).toBeTruthy();
    expect(
      within(completedDetail).queryByText(
        "No current work is occupying this place.",
      ),
    ).toBeNull();

    fireEvent.click(
      within(completedDetail).getByRole("button", {
        name: "Select work item Done Story",
      }),
    );

    const completedWorkDetail = await within(dashboardGrid).findByRole(
      "article",
      {
        name: "Current selection",
      },
    );
    expect(within(completedWorkDetail).getByText("Done Story")).toBeTruthy();
    expect(
      within(completedWorkDetail).queryByRole("heading", {
        name: "Execution details",
      }),
    ).toBeNull();

    fireEvent.click(
      screen.getByRole("button", { name: "Select story:blocked state" }),
    );

    await waitFor(() => {
      const failedDetail = screen.getByRole("article", {
        name: "Current selection",
      });

      expect(
        within(failedDetail).getAllByText("blocked").length,
      ).toBeGreaterThan(0);
      expect(within(failedDetail).getByText("Count")).toBeTruthy();
      expect(within(failedDetail).getByText("Current work")).toBeTruthy();
      expect(within(failedDetail).getByText("Failed Story")).toBeTruthy();
      expect(within(failedDetail).getByText(failedWorkID)).toBeTruthy();
      expect(
        within(failedDetail).getAllByText("Failure reason").length,
      ).toBeGreaterThan(0);
      expect(
        within(failedDetail).getAllByText("provider_rate_limit").length,
      ).toBeGreaterThan(0);
      expect(within(failedDetail).getByText("Failure message")).toBeTruthy();
      expect(
        within(failedDetail).getByText(
          "Provider rate limit exceeded while generating the repair.",
        ),
      ).toBeTruthy();
      expect(
        within(failedDetail).queryByText(
          "No current work is occupying this place.",
        ),
      ).toBeNull();
    });
  });

  it("shows an explicit unavailable state when no retained trace history exists", async () => {
    renderApp({
      snapshot: activeSnapshot,
    });

    fireEvent.click(
      (await screen.findAllByRole("button", { name: /Active Story/ }))[0],
    );

    expect(await screen.findByText("Trace history unavailable")).toBeTruthy();
    expect(
      screen.getByText(
        "No retained dispatch history is currently available for this work item.",
      ),
    ).toBeTruthy();
  });
});
