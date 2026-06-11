import { describe, expect, it } from "vitest";
import {
  isOpenApiRunnerID,
  localizeRunnerSelectionSourceValue,
  normalizeRunnerID,
  OPENAPI_RUNNER_IDS,
} from "./runner-openapi-enums";

describe("runner OpenAPI enum localization", () => {
  it("exposes the generated ModelProviderSelection enum set in contract order", () => {
    expect(OPENAPI_RUNNER_IDS).toEqual([
      "DEFAULT",
      "CLAUDE",
      "CODEX",
      "CURSOR",
      "GEMINI",
      "KIRO",
      "OPENCODE",
    ]);
  });

  it("localizes ModelProviderSelectionSource values with unknown fallback", () => {
    expect(localizeRunnerSelectionSourceValue("factory", "en")).toBe("Factory");
    expect(localizeRunnerSelectionSourceValue("worker", "ja")).toBe(
      "ワーカー provider",
    );
    expect(localizeRunnerSelectionSourceValue("workstation", "fr")).toBe(
      "Workstation",
    );
    expect(localizeRunnerSelectionSourceValue("future-source", "en")).toBe(
      "Unknown type: future-source",
    );
  });

  it("validates built-in ModelProviderSelection membership", () => {
    expect(isOpenApiRunnerID("CURSOR")).toBe(true);
    expect(isOpenApiRunnerID(" cursor ")).toBe(true);
    expect(isOpenApiRunnerID("claude")).toBe(true);
    expect(isOpenApiRunnerID("custom-runner")).toBe(false);
  });

  it("normalizes model provider selections before persistence checks", () => {
    expect(normalizeRunnerID(" cursor ")).toBe("CURSOR");
    expect(normalizeRunnerID(null)).toBe("");
  });
});
