import { describe, expect, it } from "vitest";

import {
  FACTORY_GRAPH_WORK_ITEM_MODE_MAXIMUM,
  factoryGraphWorkProgressMode,
} from "./work-progress-presentation.js";

describe("factoryGraphWorkProgressMode", () => {
  it.each([
    [0, "empty"],
    [1, "items"],
    [FACTORY_GRAPH_WORK_ITEM_MODE_MAXIMUM, "items"],
    [FACTORY_GRAPH_WORK_ITEM_MODE_MAXIMUM + 1, "total"],
    [1_000_000, "total"],
  ] as const)("maps count %d to %s presentation", (count, expected) => {
    expect(factoryGraphWorkProgressMode(count)).toBe(expected);
  });
});
