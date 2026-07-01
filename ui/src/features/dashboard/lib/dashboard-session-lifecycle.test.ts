import { describe, expect, it, vi } from "vitest";

import { CURRENT_FACTORY_DEFINITION_QUERY_KEY_PREFIX } from "../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import {
  clearDashboardSessionRuntimeQueries,
  dashboardSessionKey,
  recoverDashboardSessionScopedState,
  resetDashboardSessionScopedState,
  shouldResetDashboardSessionScopedState,
  shouldResumeFromPersistedCheckpoint,
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

describe("shouldResumeFromPersistedCheckpoint", () => {
  it("resumes from persisted checkpoints on first mount", () => {
    expect(
      shouldResumeFromPersistedCheckpoint({
        previousSessionKey: null,
        refreshToken: 0,
        sessionID: "~default",
      }),
    ).toBe(true);
  });

  it("skips persisted checkpoints after same-session refresh", () => {
    expect(
      shouldResumeFromPersistedCheckpoint({
        previousSessionKey: "~default::0",
        refreshToken: 1,
        sessionID: "~default",
      }),
    ).toBe(false);
  });

  it("resumes persisted checkpoints when switching sessions after refresh", () => {
    expect(
      shouldResumeFromPersistedCheckpoint({
        previousSessionKey: "~default::1",
        refreshToken: 1,
        sessionID: "session-beta",
      }),
    ).toBe(true);
  });
});

describe("resetDashboardSessionScopedState", () => {
  it("removes factory-definition queries when requested", () => {
    const queryClient = {
      invalidateQueries: vi.fn(),
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
    expect(queryClient.invalidateQueries).not.toHaveBeenCalled();
    expect(queryClient.removeQueries).toHaveBeenCalledWith({
      queryKey: [CURRENT_FACTORY_DEFINITION_QUERY_KEY_PREFIX],
      exact: false,
    });
  });

  it("invalidates factory-definition queries without dropping cached data when requested", () => {
    const queryClient = {
      invalidateQueries: vi.fn(),
      removeQueries: vi.fn(),
    };
    const resetTimeline = vi.fn();
    const resetStreamState = vi.fn();

    resetDashboardSessionScopedState(
      queryClient as never,
      resetStreamState,
      resetTimeline,
      "en",
      "invalidate",
    );

    expect(queryClient.invalidateQueries).toHaveBeenCalledTimes(1);
    expect(queryClient.invalidateQueries).toHaveBeenCalledWith({
      queryKey: [CURRENT_FACTORY_DEFINITION_QUERY_KEY_PREFIX],
      exact: false,
    });
    expect(queryClient.removeQueries).not.toHaveBeenCalled();
  });

  it("resets stream state without locale when omitted", () => {
    const queryClient = {
      invalidateQueries: vi.fn(),
      removeQueries: vi.fn(),
    };
    const resetTimeline = vi.fn();
    const resetStreamState = vi.fn();

    resetDashboardSessionScopedState(
      queryClient as never,
      resetStreamState,
      resetTimeline,
    );

    expect(resetStreamState).toHaveBeenCalledWith(undefined);
    expect(queryClient.removeQueries).toHaveBeenCalledTimes(1);
  });
});

describe("clearDashboardSessionRuntimeQueries", () => {
  it("removes only runtime queries for the affected session", () => {
    const queryClient = {
      removeQueries: vi.fn(),
    };

    clearDashboardSessionRuntimeQueries(queryClient as never, "session-beta");

    expect(queryClient.removeQueries).toHaveBeenCalledTimes(2);
    expect(queryClient.removeQueries).toHaveBeenNthCalledWith(1, {
      queryKey: ["current-factory-definition", "session-beta"],
      exact: false,
    });
    expect(queryClient.removeQueries).toHaveBeenNthCalledWith(2, {
      queryKey: ["factory-session-detail", "session-beta"],
      exact: false,
    });
  });
});

describe("recoverDashboardSessionScopedState", () => {
  it("resets timeline, selection state, and only the affected session runtime queries", () => {
    const queryClient = {
      removeQueries: vi.fn(),
    };
    const resetTimeline = vi.fn();

    recoverDashboardSessionScopedState(
      queryClient as never,
      "session-beta",
      resetTimeline,
    );

    expect(resetTimeline).toHaveBeenCalledTimes(1);
    expect(queryClient.removeQueries).toHaveBeenCalledTimes(3);
    expect(queryClient.removeQueries).toHaveBeenNthCalledWith(1, {
      queryKey: ["current-factory-definition", "session-beta"],
      exact: false,
    });
    expect(queryClient.removeQueries).toHaveBeenNthCalledWith(2, {
      queryKey: ["factory-session-detail", "session-beta"],
      exact: false,
    });
    expect(queryClient.removeQueries).toHaveBeenNthCalledWith(3, {
      queryKey: ["current-factory-definition", "session-beta", "document"],
      exact: true,
    });
  });
});
