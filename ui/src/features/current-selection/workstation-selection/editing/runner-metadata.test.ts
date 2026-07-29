import { describe, expect, it } from "vitest";

import { OPENAPI_RUNNER_IDS } from "../messages/runner-openapi-enums";
import {
  BUILT_IN_RUNNER_IDS,
  getRunnerDisplayName,
  getRunnerMetadata,
} from "./runner-metadata";

describe("runner-metadata", () => {
  it("exposes built-in runner ids in OpenAPI contract order", () => {
    expect(BUILT_IN_RUNNER_IDS).toEqual([...OPENAPI_RUNNER_IDS]);
  });

  it("returns metadata for every built-in runner id", () => {
    for (const runnerID of BUILT_IN_RUNNER_IDS) {
      expect(getRunnerMetadata(runnerID)).toMatchObject({
        displayName: expect.any(String),
        id: runnerID,
      });
    }
  });

  it("returns display names for built-in runner ids and null for unknown ids", () => {
    expect(getRunnerDisplayName("codex")).toBe("Codex");
    expect(getRunnerDisplayName("cursor-cli")).toBe("Cursor CLI");
    expect(getRunnerDisplayName("antigravity")).toBe("Antigravity");
    expect(getRunnerDisplayName("claude")).toBe("Claude");
    expect(getRunnerDisplayName(null)).toBeNull();
    expect(getRunnerDisplayName(undefined)).toBeNull();
  });

  it("falls back safely for unknown runner ids", () => {
    expect(getRunnerMetadata("pi")).toBeNull();
    expect(getRunnerMetadata(null)).toBeNull();
    expect(getRunnerMetadata(undefined)).toBeNull();
  });
});
