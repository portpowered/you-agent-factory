import { describe, expect, it } from "vitest";

import { dashboardSessionKey } from "../lib/dashboard-session-key";
import { resolveDashboardInitialReconnectCursor } from "./useDashboardInitialReconnectCursor";

describe("useDashboardInitialReconnectCursor", () => {
  it("returns undefined after same-session refresh even when a checkpoint exists", () => {
    const checkpoint = {
      afterEventId: "factory-event/dispatch-completed/stale-cursor",
      afterSequence: 29,
      selectedTick: 29,
    };

    const initial = resolveDashboardInitialReconnectCursor({
      persistedCheckpoint: checkpoint,
      previousSessionKey: null,
      refreshToken: 0,
      sessionID: "~default",
    });

    expect(initial).toEqual({
      afterEventId: "factory-event/dispatch-completed/stale-cursor",
      afterSequence: 29,
    });

    expect(
      resolveDashboardInitialReconnectCursor({
        persistedCheckpoint: checkpoint,
        previousSessionKey: dashboardSessionKey("~default", 0),
        refreshToken: 1,
        sessionID: "~default",
      }),
    ).toBeUndefined();
  });
});
