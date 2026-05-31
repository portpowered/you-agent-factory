import { describe, expect, it, vi } from "vitest";

import { CURRENT_FACTORY_DEFINITION_QUERY_KEY_PREFIX } from "../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import {
  dashboardSessionKey,
  resetDashboardSessionScopedState,
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

describe("resetDashboardSessionScopedState", () => {
  it("resets timeline, stream, selection history, and factory-definition queries once", () => {
    const queryClient = {
      removeQueries: vi.fn(),
    };
    const resetTimeline = vi.fn();
    const resetStreamState = vi.fn();

    resetDashboardSessionScopedState(
      queryClient as never,
      resetStreamState,
      resetTimeline,
      "en",
    );

    expect(resetTimeline).toHaveBeenCalledTimes(1);
    expect(resetStreamState).toHaveBeenCalledTimes(1);
    expect(resetStreamState).toHaveBeenCalledWith("en");
    expect(queryClient.removeQueries).toHaveBeenCalledTimes(1);
    expect(queryClient.removeQueries).toHaveBeenCalledWith({
      queryKey: [CURRENT_FACTORY_DEFINITION_QUERY_KEY_PREFIX],
      exact: false,
    });
  });
});
