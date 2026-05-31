import { describe, expect, it, beforeEach } from "vitest";

import { DEFAULT_FACTORY_SESSION_ID } from "../../../api/session-routing";
import {
  resetDashboardSessionStore,
  useDashboardSessionStore,
} from "./dashboardSessionStore";

describe("useDashboardSessionStore", () => {
  beforeEach(() => {
    resetDashboardSessionStore();
  });

  it("ignores pause updates for blank session ids", () => {
    useDashboardSessionStore.getState().setSessionPaused("   ", true);
    expect(useDashboardSessionStore.getState().pausedSessionIDs).toEqual([]);
  });

  it("tracks pause and resume for a normalized session id", () => {
    useDashboardSessionStore.getState().setSessionPaused("  session-beta  ", true);
    expect(useDashboardSessionStore.getState().pausedSessionIDs).toEqual(["session-beta"]);

    useDashboardSessionStore.getState().setSessionPaused("session-beta", false);
    expect(useDashboardSessionStore.getState().pausedSessionIDs).toEqual([]);
  });

  it("does not duplicate paused session ids", () => {
    useDashboardSessionStore.getState().setSessionPaused("session-beta", true);
    useDashboardSessionStore.getState().setSessionPaused("session-beta", true);
    expect(useDashboardSessionStore.getState().pausedSessionIDs).toEqual(["session-beta"]);
  });

  it("clears the selected session when set to null", () => {
    useDashboardSessionStore.getState().setSelectedSessionID(null);
    expect(useDashboardSessionStore.getState().selectedSessionID).toBeNull();
  });

  it("falls back to the default session when given a blank selection", () => {
    useDashboardSessionStore.getState().setSelectedSessionID("   ");
    expect(useDashboardSessionStore.getState().selectedSessionID).toBe(
      DEFAULT_FACTORY_SESSION_ID,
    );
  });
});
