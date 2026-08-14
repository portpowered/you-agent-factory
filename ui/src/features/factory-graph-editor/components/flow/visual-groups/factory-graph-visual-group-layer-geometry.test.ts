import { describe, expect, it } from "vitest";
import {
  groupOutlineAffordanceStyle,
  keyboardDelta,
  resizeBoundsFromCorner,
} from "./factory-graph-visual-group-layer-geometry";

describe("factory graph visual group affordance geometry", () => {
  it("maps keyboard arrows to deterministic movement deltas", () => {
    expect(keyboardDelta("ArrowRight", 16)).toEqual({ x: 16, y: 0 });
    expect(keyboardDelta("ArrowLeft", 16)).toEqual({ x: -16, y: 0 });
    expect(keyboardDelta("ArrowDown", 16)).toEqual({ x: 0, y: 16 });
    expect(keyboardDelta("ArrowUp", 16)).toEqual({ x: 0, y: -16 });
    expect(keyboardDelta("Enter", 16)).toBeNull();
  });

  it("keeps outline hit targets on the perimeter instead of the region interior", () => {
    expect(groupOutlineAffordanceStyle("top")).toEqual({
      height: 8,
      left: 8,
      right: 8,
      top: -4,
    });
    expect(groupOutlineAffordanceStyle("left")).toEqual({
      bottom: 8,
      left: -4,
      top: 8,
      width: 8,
    });
  });

  it("resizes from a keyboard direction while honoring minimum bounds", () => {
    expect(
      resizeBoundsFromCorner({ height: 120, width: 200, x: 40, y: 60 }, "se", {
        x: -400,
        y: -400,
      }),
    ).toEqual({ height: 80, width: 120, x: 40, y: 60 });
  });
});
