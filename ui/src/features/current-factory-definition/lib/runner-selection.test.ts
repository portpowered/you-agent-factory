import { describe, expect, it } from "vitest";
import { resolveRunnerSelection } from "./runner-selection";

describe("resolveRunnerSelection", () => {
  it("prefers workstation overrides, then factory, then worker modelProvider, then operator default", () => {
    expect(resolveRunnerSelection("GEMINI", "CODEX", "CODEX")).toEqual({
      runnerId: "GEMINI",
      source: "workstation",
    });

    expect(resolveRunnerSelection(null, "CURSOR", "CODEX")).toEqual({
      runnerId: "CURSOR",
      source: "factory",
    });

    expect(resolveRunnerSelection(null, null, "CODEX")).toEqual({
      runnerId: "CODEX",
      source: "worker",
    });

    expect(resolveRunnerSelection(null, null, "CLAUDE")).toEqual({
      runnerId: "CLAUDE",
      source: "worker",
    });

    expect(resolveRunnerSelection(null, null, null)).toEqual({
      runnerId: "CODEX",
      source: "operator_default",
    });
  });
});
