// biome-ignore-all lint/complexity/noExcessiveLinesPerFunction: Storybook runtime coverage stays consolidated because these tests share the same decorator installation seam.
import { render, screen, waitFor } from "@testing-library/react";
import { createElement } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { DashboardSnapshot } from "../src/api/dashboard";
import { FACTORY_EVENT_TYPES, type FactoryEvent } from "../src/api/events";
import { DEFAULT_FACTORY_SESSION_ID } from "../src/api/session-routing";
import {
  resetSelectionHistoryStore,
  useSelectionHistoryStore,
} from "../src/features/current-selection/base/public";
import { useDashboardSession } from "../src/features/dashboard/session/dashboard-session-provider";
import {
  resetDashboardSessionStore,
  useDashboardSessionStore,
} from "../src/features/dashboard/state/dashboardSessionStore";
import { useFactoryTimelineStore } from "../src/features/timeline/state/factoryTimelineStore";

import { withDashboardStoryRuntime } from "./dashboard-story-runtime";

function SessionScopeProbe() {
  const { sessionID } = useDashboardSession();

  return createElement(
    "div",
    { "data-testid": "story-session-id" },
    sessionID,
  );
}

function renderStoryRuntime(
  parameters: Record<string, unknown> = {},
  storyId = "dashboard-runtime-story",
) {
  const decoratedStory = withDashboardStoryRuntime(
    () => createElement(SessionScopeProbe),
    {
    args: {},
    globals: {},
    hooks: {} as never,
    id: storyId,
    initialArgs: {},
    name: storyId,
    parameters,
    title: "storybook/runtime",
    viewMode: "story",
    },
  );

  return render(decoratedStory);
}

describe("withDashboardStoryRuntime", () => {
  beforeEach(() => {
    resetDashboardSessionStore();
    resetSelectionHistoryStore();
    useFactoryTimelineStore.getState().reset();
    window.fetch = fetch;
  });

  it("resets dashboard session state before installing story mocks", () => {
    useDashboardSessionStore.setState({
      pausedSessionIDs: ["session-beta"],
      selectedSessionID: "session-beta",
    });
    useSelectionHistoryStore.getState().commitSelectionState({
      selection: { kind: "node", nodeId: "review" },
      terminalWorkDetail: null,
    });
    useFactoryTimelineStore.setState({
      events: [],
      latestTick: 4,
      mode: "fixed",
      receivedEventIDs: [],
      selectedTick: 2,
      worldViewCache: {},
    });

    withDashboardStoryRuntime(() => null, {
      args: {},
      globals: {},
      hooks: {} as never,
      id: "dashboard-runtime-reset",
      initialArgs: {},
      name: "Dashboard runtime reset",
      parameters: {},
      title: "storybook/runtime",
      viewMode: "story",
    });

    expect(useDashboardSessionStore.getState().pausedSessionIDs).toEqual([]);
    expect(useDashboardSessionStore.getState().selectedSessionID).toBe(
      DEFAULT_FACTORY_SESSION_ID,
    );
    expect(useSelectionHistoryStore.getState().present.selection).toBeNull();
    expect(useFactoryTimelineStore.getState().selectedTick).toBe(0);
    expect(useFactoryTimelineStore.getState().mode).toBe("current");
  });

  it("applies sessionID from story parameters via DashboardSessionTestProvider", () => {
    useDashboardSessionStore.setState({
      pausedSessionIDs: ["session-beta"],
      selectedSessionID: "session-beta",
    });

    renderStoryRuntime(
      {
        dashboardApi: {
          sessionID: "session-review",
        },
      },
      "dashboard-runtime-session-id",
    );

    expect(useDashboardSessionStore.getState().pausedSessionIDs).toEqual([]);
    expect(useDashboardSessionStore.getState().selectedSessionID).toBe(
      DEFAULT_FACTORY_SESSION_ID,
    );
    expect(screen.getByTestId("story-session-id").textContent).toBe(
      "session-review",
    );
  });

  it("does not leak pinned session scope across story installs", () => {
    const { unmount } = renderStoryRuntime(
      {
        dashboardApi: {
          sessionID: "session-beta",
        },
      },
      "dashboard-runtime-session-leak-a",
    );

    expect(screen.getByTestId("story-session-id").textContent).toBe(
      "session-beta",
    );

    unmount();
    renderStoryRuntime({}, "dashboard-runtime-session-leak-b");

    expect(screen.getByTestId("story-session-id").textContent).toBe(
      DEFAULT_FACTORY_SESSION_ID,
    );
  });

  it("installs dashboard fetch mocks and seeds timeline snapshots on the latest tick", async () => {
    const firstSnapshot = buildDashboardSnapshot(2);
    const latestSnapshot = buildDashboardSnapshot(5);

    withDashboardStoryRuntime(() => null, {
      args: {},
      globals: {},
      hooks: {} as never,
      id: "dashboard-runtime-fetch-mocks",
      initialArgs: {},
      name: "Dashboard runtime fetch mocks",
      parameters: {
        dashboardApi: {
          fetchMocks: [
            {
              path: "/api/dashboard-snapshot",
              response: {
                body: { ok: true },
                status: 202,
              },
            },
          ],
          timelineSnapshots: [firstSnapshot, latestSnapshot],
        },
      },
      title: "storybook/runtime",
      viewMode: "story",
    });

    const response = await window.fetch(
      "http://example.test/api/dashboard-snapshot",
    );

    expect(response.status).toBe(202);
    await expect(response.json()).resolves.toEqual({ ok: true });
    expect(useFactoryTimelineStore.getState()).toMatchObject({
      latestTick: 5,
      mode: "current",
      selectedTick: 5,
    });
    expect(useFactoryTimelineStore.getState().worldViewCache[2]).toMatchObject({
      tick_count: 2,
    });
    expect(useFactoryTimelineStore.getState().worldViewCache[5]).toMatchObject({
      tick_count: 5,
    });
  });

  it("installs dashboard fetch mocks even when the story does not seed timeline data", async () => {
    withDashboardStoryRuntime(() => null, {
      args: {},
      globals: {},
      hooks: {} as never,
      id: "dashboard-runtime-fetch-only",
      initialArgs: {},
      name: "Dashboard runtime fetch-only",
      parameters: {
        dashboardApi: {
          fetchMocks: [
            {
              path: "/factory-sessions",
              response: {
                body: {
                  sessions: [],
                },
              },
            },
          ],
        },
      },
      title: "storybook/runtime",
      viewMode: "story",
    });

    const response = await window.fetch("http://example.test/factory-sessions");

    expect(response.status).toBe(200);
    await expect(response.json()).resolves.toEqual({
      sessions: [],
    });
    expect(useFactoryTimelineStore.getState()).toMatchObject({
      latestTick: 0,
      mode: "current",
      selectedTick: 0,
    });
  });

  it("replaces timeline events when story parameters provide event history directly", () => {
    const events = [timelineEvent("tick-2", 2), timelineEvent("tick-4", 4)];

    withDashboardStoryRuntime(() => null, {
      args: {},
      globals: {},
      hooks: {} as never,
      id: "dashboard-runtime-timeline-events",
      initialArgs: {},
      name: "Dashboard runtime timeline events",
      parameters: {
        dashboardApi: {
          timelineEvents: events,
        },
      },
      title: "storybook/runtime",
      viewMode: "story",
    });

    expect(useFactoryTimelineStore.getState()).toMatchObject({
      events,
      latestTick: 4,
      mode: "current",
      receivedEventIDs: ["tick-2", "tick-4"],
      selectedTick: 4,
    });
  });

  it("installs dashboard event-source mocks that reseed the timeline and emit message events", async () => {
    const opened = vi.fn();
    const receivedEvents: FactoryEvent[] = [];

    withDashboardStoryRuntime(() => null, {
      args: {},
      globals: {},
      hooks: {} as never,
      id: "dashboard-runtime-event-source",
      initialArgs: {},
      name: "Dashboard runtime event source",
      parameters: {
        dashboardApi: {
          eventSourceMocks: [
            {
              events: [timelineEvent("tick-7", 7)],
              path: "/api/factory-sessions/default/events",
              snapshot: buildDashboardSnapshot(7),
            },
          ],
          snapshot: buildDashboardSnapshot(1),
        },
      },
      title: "storybook/runtime",
      viewMode: "story",
    });

    const eventSource = new window.EventSource(
      "http://example.test/api/factory-sessions/default/events",
    );
    eventSource.onopen = opened;
    eventSource.addEventListener("message", (event) => {
      receivedEvents.push(
        JSON.parse((event as MessageEvent).data) as FactoryEvent,
      );
    });

    await waitFor(() => {
      expect(opened).toHaveBeenCalledTimes(1);
      expect(receivedEvents).toHaveLength(1);
      expect(useFactoryTimelineStore.getState()).toMatchObject({
        latestTick: 7,
        mode: "current",
        selectedTick: 7,
      });
    });
    expect(receivedEvents[0]?.id).toBe("tick-7");
  });
});

function buildDashboardSnapshot(tick: number): DashboardSnapshot {
  return {
    factory_state: "UNKNOWN",
    runtime: {} as never,
    tick_count: tick,
    topology: {
      edges: [],
      submit_work_types: [],
      workstation_node_ids: [],
      workstation_nodes_by_id: {},
    },
    uptime_seconds: tick,
  } as DashboardSnapshot;
}

function timelineEvent(id: string, tick: number): FactoryEvent {
  return {
    context: {
      eventTime: `2026-05-24T00:00:0${tick}Z`,
      sequence: tick,
      tick,
    },
    id,
    payload: {
      factory: {
        workTypes: [],
        workers: [],
        workstations: [],
      },
    },
    type: FACTORY_EVENT_TYPES.initialStructureRequest,
  };
}
