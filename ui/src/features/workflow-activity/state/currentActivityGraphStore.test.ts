import { describe, expect, it } from "vitest";

import {
  CURRENT_ACTIVITY_GRAPH_STORAGE_KEY,
  useCurrentActivityGraphStore,
} from "./currentActivityGraphStore";

describe("currentActivityGraphStore persistence", () => {
  it("rehydrates node positions but drops persisted viewport state", () => {
    window.localStorage.setItem(
      CURRENT_ACTIVITY_GRAPH_STORAGE_KEY,
      JSON.stringify({
        state: {
          positionsByGraphKey: {
            "graph-key": {
              "workstation:draft": { x: 40, y: 80 },
            },
          },
          viewportByGraphKey: {
            "graph-key": { x: 10, y: 20, zoom: 1.2 },
          },
        },
        version: 1,
      }),
    );

    useCurrentActivityGraphStore.persist.rehydrate();

    expect(
      useCurrentActivityGraphStore.getState().positionsByGraphKey["graph-key"],
    ).toEqual({
      "workstation:draft": { x: 40, y: 80 },
    });
    expect(
      useCurrentActivityGraphStore.getState().viewportByGraphKey["graph-key"],
    ).toBeUndefined();
  });
});
