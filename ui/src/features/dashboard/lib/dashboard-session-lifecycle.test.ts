import { describe, expect, it } from "vitest";

import {
  dashboardSessionKey,
  shouldResetDashboardSessionScopedState,
} from "./dashboard-session-lifecycle";

describe("dashboardSessionKey", () => {
  it("returns null when session is deselected", () => {
    expect(dashboardSessionKey(null, 0)).toBeNull();
  });

  it("combines session id and refresh token", () => {
    expect(dashboardSessionKey("session-beta", 2)).toBe("session-beta::2");
  });
});

describe("shouldResetDashboardSessionScopedState", () => {
  it("resets when the session is deselected", () => {
    expect(
      shouldResetDashboardSessionScopedState({
        previousSessionKey: "~default::0",
        refreshToken: 0,
        sessionID: null,
      }),
    ).toBe(true);
  });

  it("skips reset on first mount for the default refresh token", () => {
    expect(
      shouldResetDashboardSessionScopedState({
        previousSessionKey: null,
        refreshToken: 0,
        sessionID: "~default",
      }),
    ).toBe(false);
  });

  it("resets when refresh token changes", () => {
    expect(
      shouldResetDashboardSessionScopedState({
        previousSessionKey: "~default::0",
        refreshToken: 1,
        sessionID: "~default",
      }),
    ).toBe(true);
  });

  it("resets when switching to another session", () => {
    expect(
      shouldResetDashboardSessionScopedState({
        previousSessionKey: "~default::0",
        refreshToken: 0,
        sessionID: "session-beta",
      }),
    ).toBe(true);
  });
});
