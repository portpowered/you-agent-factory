import { describe, expect, it } from "vitest";
import {
  visitCountGuardsAllow,
  visitsAfterTransition,
} from "./logical-move.js";

describe("Factory emulator logical-move projection", () => {
  const guards = [
    { type: "VISIT_COUNT", workstation: "review", maxVisits: 3 },
  ] as const;

  it("evaluates VISIT_COUNT below, at, and above its inclusive threshold", () => {
    expect(visitCountGuardsAllow(guards, { review: 2 })).toBe(false);
    expect(visitCountGuardsAllow(guards, { review: 3 })).toBe(true);
    expect(visitCountGuardsAllow(guards, { review: 4 })).toBe(true);
  });

  it("merges contributing lineage visits by maximum and records the move", () => {
    expect(
      visitsAfterTransition(
        [
          { execute: 2, review: 1 },
          { execute: 1, review: 3 },
        ],
        "loop-breaker",
      ),
    ).toEqual({ execute: 2, review: 3, "loop-breaker": 1 });
  });
});
