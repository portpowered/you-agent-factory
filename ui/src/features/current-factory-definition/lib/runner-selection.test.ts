import { describe, expect, it } from "vitest";
import { resolveRunnerSelection } from "./runner-selection";

describe("resolveRunnerSelection", () => {
  it("prefers workstation overrides, then factory, then legacy modelProvider, then default", () => {
    expect(
      resolveRunnerSelection("gemini", "codex", "CODEX"),
    ).toEqual({
      runnerId: "gemini",
      source: "workstation",
    });

    expect(resolveRunnerSelection(null, "cursor-cli", "codex")).toEqual({
      runnerId: "cursor-cli",
      source: "factory",
    });

    expect(resolveRunnerSelection(null, null, "codex")).toEqual({
      runnerId: "codex",
      source: "legacy_provider",
    });

    expect(resolveRunnerSelection(null, null, "CLAUDE")).toEqual({
      runnerId: "codex",
      source: "default",
    });
  });
});
