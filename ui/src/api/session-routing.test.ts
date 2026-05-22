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
    expect(factorySessionScopedPath("/work", undefined)).toBe("/factories/~default/work");
    expect(factorySessionScopedPath("/work", null)).toBe("/factories/~default/work");
    expect(factorySessionScopedPath("/work", "")).toBe("/factories/~default/work");
    expect(factorySessionScopedPath("work", DEFAULT_FACTORY_SESSION_ID)).toBe(
      "/factories/~default/work",
    );
  });

  it("preserves non-default session identifiers in the scoped path", () => {
    expect(factorySessionScopedPath("/events", "session-beta")).toBe(
      "/factories/session-beta/events",
    );
  });
});
