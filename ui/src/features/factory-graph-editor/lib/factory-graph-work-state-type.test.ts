import { describe, expect, it } from "vitest";

import type { CanonicalFactoryDefinition } from "./factory-graph-draft-types";
import { resolveWorkStateTypeForGraphNode } from "./factory-graph-work-state-type";

const lifecycleFixture = {
  metadata: {
    owner: "operations",
  },
  name: "Lifecycle Fixture",
  workTypes: [
    {
      name: "story",
      states: [
        { name: "queued", type: "INITIAL" },
        { name: "review", type: "PROCESSING" },
        { name: "done", type: "TERMINAL" },
        { name: "failed", type: "FAILED" },
      ],
    },
  ],
} satisfies CanonicalFactoryDefinition;

describe("resolveWorkStateTypeForGraphNode", () => {
  it.each([
    ["INITIAL", "queued"],
    ["PROCESSING", "review"],
    ["TERMINAL", "done"],
    ["FAILED", "failed"],
  ] as const)("returns %s for matching work-state nodes", (type, stateName) => {
    expect(
      resolveWorkStateTypeForGraphNode(lifecycleFixture, {
        kind: "work-state",
        stateName,
        workTypeName: "story",
      }),
    ).toBe(type);
  });

  it("returns undefined for a missing work state", () => {
    expect(
      resolveWorkStateTypeForGraphNode(lifecycleFixture, {
        kind: "work-state",
        stateName: "missing",
        workTypeName: "story",
      }),
    ).toBeUndefined();
  });

  it("returns undefined for a missing work type", () => {
    expect(
      resolveWorkStateTypeForGraphNode(lifecycleFixture, {
        kind: "work-state",
        stateName: "queued",
        workTypeName: "missing",
      }),
    ).toBeUndefined();
  });

  it("returns undefined when the factory definition is absent", () => {
    expect(
      resolveWorkStateTypeForGraphNode(null, {
        kind: "work-state",
        stateName: "queued",
        workTypeName: "story",
      }),
    ).toBeUndefined();
  });
});
