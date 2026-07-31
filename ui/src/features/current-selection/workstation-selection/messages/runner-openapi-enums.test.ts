import { describe, expect, it } from "vitest";
import {
  isOpenApiRunnerID,
  localizeRunnerSelectionSourceValue,
  normalizeRunnerID,
  OPENAPI_RUNNER_IDS,
} from "./runner-openapi-enums";

describe("runner OpenAPI enum localization", () => {
  it("exposes the generated RunnerID enum set in contract order", () => {
    expect(OPENAPI_RUNNER_IDS).toEqual([
      "codex",
      "claude",
      "antigravity",
    ]);
  });

  it("localizes RunnerSelectionSource values with unknown fallback", () => {
    expect(localizeRunnerSelectionSourceValue("factory", "en")).toBe("Factory");
    expect(localizeRunnerSelectionSourceValue("legacy_provider", "ja")).toBe(
      "ワーカー provider",
    );
    expect(localizeRunnerSelectionSourceValue("workstation", "fr")).toBe(
      "Workstation",
    );
    expect(localizeRunnerSelectionSourceValue("future-source", "en")).toBe(
      "Unknown type: future-source",
    );
  });

  it("validates built-in RunnerID membership", () => {
    expect(isOpenApiRunnerID("codex")).toBe(true);
    expect(isOpenApiRunnerID(" CODEX ")).toBe(true);
    expect(isOpenApiRunnerID("antigravity")).toBe(true);
    expect(isOpenApiRunnerID(" ANTIGRAVITY ")).toBe(true);
    expect(isOpenApiRunnerID("pi")).toBe(false);
  });

  it("normalizes runner IDs before persistence checks", () => {
    expect(normalizeRunnerID(" CODEX ")).toBe("codex");
    expect(normalizeRunnerID(null)).toBe("");
  });
});
