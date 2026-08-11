import { beforeEach, describe, expect, it } from "vitest";

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
    useDashboardSessionStore
      .getState()
      .setSessionPaused("  session-beta  ", true);
    expect(useDashboardSessionStore.getState().pausedSessionIDs).toEqual([
      "session-beta",
    ]);

    useDashboardSessionStore.getState().setSessionPaused("session-beta", false);
    expect(useDashboardSessionStore.getState().pausedSessionIDs).toEqual([]);
  });

  it("does not duplicate paused session ids", () => {
    useDashboardSessionStore.getState().setSessionPaused("session-beta", true);
    useDashboardSessionStore.getState().setSessionPaused("session-beta", true);
    expect(useDashboardSessionStore.getState().pausedSessionIDs).toEqual([
      "session-beta",
    ]);
  });

  it("atomically replaces a transient selector in selection and tab order", () => {
    useDashboardSessionStore.setState({
      selectedSessionID: DEFAULT_FACTORY_SESSION_ID,
      sessionTabOrder: [DEFAULT_FACTORY_SESSION_ID, "session-beta"],
    });

    useDashboardSessionStore
      .getState()
      .resolveSessionIdentity(
        DEFAULT_FACTORY_SESSION_ID,
        "a1b2c3d4-e5f6-4789-a012-3456789abcde",
        ["a1b2c3d4-e5f6-4789-a012-3456789abcde", "session-beta"],
      );

    expect(useDashboardSessionStore.getState()).toMatchObject({
      selectedSessionID: "a1b2c3d4-e5f6-4789-a012-3456789abcde",
      sessionTabOrder: ["a1b2c3d4-e5f6-4789-a012-3456789abcde", "session-beta"],
    });
  });

  it("does not replace a newer selection when delayed discovery completes", () => {
    useDashboardSessionStore.setState({
      selectedSessionID: "session-beta",
      sessionTabOrder: [DEFAULT_FACTORY_SESSION_ID, "session-beta"],
    });

    useDashboardSessionStore
      .getState()
      .resolveSessionIdentity(
        DEFAULT_FACTORY_SESSION_ID,
        "a1b2c3d4-e5f6-4789-a012-3456789abcde",
        ["a1b2c3d4-e5f6-4789-a012-3456789abcde", "session-beta"],
      );

    expect(useDashboardSessionStore.getState()).toMatchObject({
      selectedSessionID: "session-beta",
      sessionTabOrder: ["a1b2c3d4-e5f6-4789-a012-3456789abcde", "session-beta"],
    });
  });

  it("atomically remaps a replaced live session without changing sibling tabs", () => {
    useDashboardSessionStore.setState({
      pausedSessionIDs: ["session-stale", "session-beta"],
      selectedSessionID: "session-stale",
      sessionTabOrder: ["session-stale", "session-beta"],
    });

    useDashboardSessionStore
      .getState()
      .remapSelectedSessionIdentity("session-replacement");

    expect(useDashboardSessionStore.getState()).toMatchObject({
      pausedSessionIDs: ["session-beta"],
      selectedSessionID: "session-replacement",
      sessionTabOrder: ["session-replacement", "session-beta"],
    });
  });

  it("does not duplicate a replacement identity already present in tab metadata", () => {
    useDashboardSessionStore.setState({
      pausedSessionIDs: ["session-stale", "session-replacement"],
      selectedSessionID: "session-stale",
      sessionTabOrder: ["session-stale", "session-replacement", "session-beta"],
    });

    useDashboardSessionStore
      .getState()
      .remapSelectedSessionIdentity("session-replacement");

    expect(useDashboardSessionStore.getState()).toMatchObject({
      pausedSessionIDs: ["session-replacement"],
      selectedSessionID: "session-replacement",
      sessionTabOrder: ["session-replacement", "session-beta"],
    });
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

  it("restores default session selection and paused sessions", () => {
    useDashboardSessionStore.getState().setSelectedSessionID("session-beta");
    useDashboardSessionStore.getState().setSessionPaused("session-beta", true);
    resetDashboardSessionStore();
    expect(useDashboardSessionStore.getState()).toMatchObject({
      pausedSessionIDs: [],
      selectedSessionID: DEFAULT_FACTORY_SESSION_ID,
    });
  });
});

describe("useDashboardSessionStore session-list reconciliation", () => {
  beforeEach(() => {
    resetDashboardSessionStore();
  });

  it("reconciles canonical membership without retaining aliases or stale tabs", () => {
    useDashboardSessionStore.setState({
      pausedSessionIDs: ["session-removed", "session-beta"],
      selectedSessionID: DEFAULT_FACTORY_SESSION_ID,
      sessionTabOrder: [
        DEFAULT_FACTORY_SESSION_ID,
        "session-removed",
        "session-beta",
      ],
    });

    useDashboardSessionStore
      .getState()
      .reconcileSessionList(
        ["session-beta", "session-created"],
        "session-beta",
      );

    expect(useDashboardSessionStore.getState()).toMatchObject({
      pausedSessionIDs: ["session-beta"],
      selectedSessionID: "session-beta",
      sessionTabOrder: ["session-beta", "session-created"],
    });
  });

  it("remaps the default selector to the canonical default row", () => {
    useDashboardSessionStore
      .getState()
      .reconcileSessionList(
        ["session-default", "session-beta"],
        "session-default",
      );

    expect(useDashboardSessionStore.getState()).toMatchObject({
      selectedSessionID: "session-default",
      sessionTabOrder: ["session-default", "session-beta"],
    });
  });

  it("clears selection when an authoritative empty list removes every session", () => {
    useDashboardSessionStore.setState({
      selectedSessionID: "session-removed",
      sessionTabOrder: ["session-removed"],
    });

    useDashboardSessionStore.getState().reconcileSessionList([]);

    expect(useDashboardSessionStore.getState()).toMatchObject({
      selectedSessionID: null,
      sessionTabOrder: [],
    });
  });
});
