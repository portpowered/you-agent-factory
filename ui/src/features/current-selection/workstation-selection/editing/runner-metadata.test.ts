import { describe, expect, it } from "vitest";

import { OPENAPI_RUNNER_IDS } from "../messages/runner-openapi-enums";
import {
  BUILT_IN_RUNNER_IDS,
  getRunnerDisplayName,
  getRunnerMetadata,
} from "./runner-metadata";

describe("runner-metadata", () => {
  it("exposes built-in model providers in OpenAPI contract order without DEFAULT", () => {
    expect(BUILT_IN_RUNNER_IDS).toEqual(
      OPENAPI_RUNNER_IDS.filter((value) => value !== "DEFAULT"),
    );
  });

  it("returns metadata for every built-in model provider", () => {
    for (const modelProvider of BUILT_IN_RUNNER_IDS) {
      expect(getRunnerMetadata(modelProvider)).toMatchObject({
        displayName: expect.any(String),
        id: modelProvider,
      });
    }
  });

  it("returns display names for built-in model providers and null for unknown ids", () => {
    expect(getRunnerDisplayName("CODEX")).toBe("Codex");
    expect(getRunnerDisplayName("CURSOR")).toBe("Cursor CLI");
    expect(getRunnerDisplayName("custom-runner")).toBeNull();
    expect(getRunnerDisplayName(null)).toBeNull();
    expect(getRunnerDisplayName(undefined)).toBeNull();
  });

  it("falls back safely for unknown model providers", () => {
    expect(getRunnerMetadata("custom-runner")).toBeNull();
    expect(getRunnerMetadata(null)).toBeNull();
    expect(getRunnerMetadata(undefined)).toBeNull();
  });
});
