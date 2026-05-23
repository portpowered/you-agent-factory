import { describe, expect, it } from "vitest";

import {
  DEFAULT_FACTORY_SESSION_ID,
  factorySessionScopedPath,
  isDefaultFactorySessionID,
} from "./session-routing";

describe("factorySessionScopedPath", () => {
  it("treats null, undefined, and empty sessions as the default session", () => {
    expect(isDefaultFactorySessionID(undefined)).toBe(true);
    expect(isDefaultFactorySessionID(null)).toBe(true);
    expect(isDefaultFactorySessionID("")).toBe(true);
    expect(isDefaultFactorySessionID(DEFAULT_FACTORY_SESSION_ID)).toBe(true);
  });

  it("always returns an explicit default-session scoped path", () => {
    expect(factorySessionScopedPath("/work", undefined)).toBe("/factory-sessions/~default/work");
    expect(factorySessionScopedPath("/work", null)).toBe("/factory-sessions/~default/work");
    expect(factorySessionScopedPath("/work", "")).toBe("/factory-sessions/~default/work");
    expect(factorySessionScopedPath("work", DEFAULT_FACTORY_SESSION_ID)).toBe(
      "/factory-sessions/~default/work",
    );
  });

  it("preserves non-default session identifiers in the scoped path", () => {
    expect(factorySessionScopedPath("/events", "session-beta")).toBe(
      "/factory-sessions/session-beta/events",
    );
  });

  it("maps current-factory reads to the canonical session factory route", () => {
    expect(factorySessionScopedPath("/factory/~current", "session-beta")).toBe(
      "/factory-sessions/session-beta/factory",
    );
  });

  it("maps editable current-factory requests under the canonical session factory root", () => {
    expect(
      factorySessionScopedPath("/factory/~current/editable-definition", "session-beta"),
    ).toBe("/factory-sessions/session-beta/factory/editable-definition");
  });
});
