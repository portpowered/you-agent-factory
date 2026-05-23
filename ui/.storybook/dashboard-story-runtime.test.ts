import { beforeEach, describe, expect, it } from "vitest";

import { DEFAULT_FACTORY_SESSION_ID } from "../src/api/session-routing";
import { resetSelectionHistoryStore, useSelectionHistoryStore } from "../src/features/current-selection/state/selectionHistoryStore";
import {
  resetDashboardSessionStore,
  useDashboardSessionStore,
} from "../src/features/dashboard/state/dashboardSessionStore";
import { useFactoryTimelineStore } from "../src/features/timeline/state/factoryTimelineStore";

import { withDashboardStoryRuntime } from "./dashboard-story-runtime";

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
});
