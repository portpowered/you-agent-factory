import { describe, expect, it } from "vitest";
import {
  isOpenApiRunnerID,
  localizeRunnerSelectionSourceValue,
  OPENAPI_RUNNER_IDS,
} from "./runner-openapi-enums";

describe("runner OpenAPI enum localization", () => {
  it("exposes the generated RunnerID enum set in contract order", () => {
    expect(OPENAPI_RUNNER_IDS).toEqual([
      "codex",
      "gemini",
      "kiro",
      "cursor-cli",
      "opencode",
    ]);
  });

  it("localizes RunnerSelectionSource values with unknown fallback", () => {
    expect(localizeRunnerSelectionSourceValue("factory", "en")).toBe("Factory");
    expect(localizeRunnerSelectionSourceValue("legacy_provider", "ja")).toBe(
      "旧 provider",
    );
    expect(localizeRunnerSelectionSourceValue("future-source", "en")).toBe(
      "Unknown type: future-source",
    );
  });

  it("validates built-in RunnerID membership", () => {
    expect(isOpenApiRunnerID("cursor-cli")).toBe(true);
    expect(isOpenApiRunnerID(" CURSOR-CLI ")).toBe(true);
    expect(isOpenApiRunnerID("claude")).toBe(false);
  });
});
