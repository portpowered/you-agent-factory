import { describe, expect, it } from "vitest";
import { resolveRunnerSelection } from "./runner-selection";

describe("resolveRunnerSelection", () => {
  it("prefers workstation overrides, then factory, then legacy modelProvider, then default", () => {
    expect(resolveRunnerSelection("antigravity", "codex", "CODEX")).toEqual({
      runnerId: "antigravity",
      source: "workstation",
    });

    expect(resolveRunnerSelection(null, "codex", "codex")).toEqual({
      runnerId: "codex",
      source: "factory",
    });

    expect(resolveRunnerSelection(null, null, "codex")).toEqual({
      runnerId: "codex",
      source: "legacy_provider",
    });

    expect(resolveRunnerSelection(null, null, "CLAUDE")).toEqual({
	  runnerId: "claude",
	  source: "legacy_provider",
    });
  });
});
