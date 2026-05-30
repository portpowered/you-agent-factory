import { describe, expect, it } from "vitest";

import { DEFAULT_FACTORY_SESSION_ID } from "./session-routing";
import { buildSessionScope } from "./session-scope";

describe("buildSessionScope", () => {
  it("maps null, empty, and ~default selection to the default session scope", () => {
    for (const rawSessionID of [null, "", DEFAULT_FACTORY_SESSION_ID] as const) {
      const scope = buildSessionScope(rawSessionID, []);

      expect(scope).toEqual({
        sessionID: DEFAULT_FACTORY_SESSION_ID,
        rawSessionID,
        isDefault: true,
        factoryPath: "/factory-sessions/~default/factory",
        workPath: "/factory-sessions/~default/work",
        eventsPath: "/factory-sessions/~default/events",
        isPaused: false,
      });
    }
  });

  it("preserves non-default session identifiers and URL-encodes path segments", () => {
    const scope = buildSessionScope("session/beta", []);

    expect(scope.sessionID).toBe("session/beta");
    expect(scope.rawSessionID).toBe("session/beta");
    expect(scope.isDefault).toBe(false);
    expect(scope.factoryPath).toBe("/factory-sessions/session%2Fbeta/factory");
    expect(scope.workPath).toBe("/factory-sessions/session%2Fbeta/work");
    expect(scope.eventsPath).toBe("/factory-sessions/session%2Fbeta/events");
    expect(scope.isPaused).toBe(false);
  });

  it("reflects pause state for the active non-default session", () => {
    const scope = buildSessionScope("session-beta", ["session-beta", "other-session"]);

    expect(scope.isPaused).toBe(true);
  });

  it("does not mark the default session paused when only other sessions are paused", () => {
    const scope = buildSessionScope(DEFAULT_FACTORY_SESSION_ID, ["session-beta"]);

    expect(scope.isPaused).toBe(false);
  });
});
