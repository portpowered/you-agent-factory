import { describe, expect, test } from "vitest";

import {
  createFactoryGraphSource,
  isFactoryGraphSource,
  type FactoryGraphSource,
} from "./source";

function source(selectedTick = 7): FactoryGraphSource {
  return {
    factory: { name: "Support" },
    runtime: {
      activity: { activeDispatchOverlays: [], activeWorkstationNodeIds: [] },
      load: { resourceOccupancy: [], workStateCounts: [] },
      topology: { connections: [], nodes: [] },
    },
    selectedTick,
  } as FactoryGraphSource;
}

describe("FactoryGraphSource", () => {
  test("keeps the complete Factory and selected runtime projection together", () => {
    const graphSource = source();

    expect(createFactoryGraphSource(graphSource)).toBe(graphSource);
    expect(graphSource.factory.name).toBe("Support");
    expect(graphSource.runtime.topology.nodes).toEqual([]);
  });

  test("rejects a negative or non-integral selected tick", () => {
    expect(isFactoryGraphSource(source(-1))).toBe(false);
    expect(isFactoryGraphSource(source(1.5))).toBe(false);
    expect(() => createFactoryGraphSource(source(-1))).toThrow(
      "non-negative selected tick",
    );
  });
});
