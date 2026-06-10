import {
  buildWorkstationSaveScopeKey,
  parseWorkstationSaveScopeKey,
} from "./workstation-save-scope-key";

describe("workstation save scope key", () => {
  it("round-trips node, transition, and workstation name segments", () => {
    const scopeKey = buildWorkstationSaveScopeKey({
      nodeId: "review",
      transitionId: "transition",
      workstationName: "Review",
    });

    expect(scopeKey).toBe("review:transition:Review");
    expect(parseWorkstationSaveScopeKey(scopeKey)).toEqual({
      nodeId: "review",
      transitionId: "transition",
      workstationName: "Review",
    });
  });

  it("preserves workstation names that contain colons", () => {
    const scopeKey = buildWorkstationSaveScopeKey({
      nodeId: "review",
      transitionId: "transition",
      workstationName: "Review:Alpha",
    });

    expect(parseWorkstationSaveScopeKey(scopeKey)).toEqual({
      nodeId: "review",
      transitionId: "transition",
      workstationName: "Review:Alpha",
    });
  });

  it("returns null for malformed scope keys", () => {
    expect(parseWorkstationSaveScopeKey("review-only")).toBeNull();
    expect(parseWorkstationSaveScopeKey("review:transition-only")).toBeNull();
  });
});
