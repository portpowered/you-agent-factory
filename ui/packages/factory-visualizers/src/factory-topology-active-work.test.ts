import { describe, expect, it } from "vitest";

import { projectFactoryTopologyActiveWork } from "./factory-topology-active-work";
import { createFactoryTopologyProjection } from "./testing/factory-topology-projection";

describe("projectFactoryTopologyActiveWork", () => {
  it("keeps the first three Work rows, reports overflow, and retains the longest active duration", () => {
    const projection = createFactoryTopologyProjection();
    projection.activity.selectedTick = 12;
    projection.activity.activeDispatchOverlays = [
      {
        ...projection.activity.activeDispatchOverlays[0],
        startedTick: 8,
        workIds: ["work-b", "work-a", "work-c", "work-d"],
      },
      {
        ...projection.activity.activeDispatchOverlays[0],
        dispatchId: "dispatch-2",
        id: "overlay:dispatch-2",
        startedTick: 5,
        workIds: ["work-a"],
      },
    ];

    expect(projectFactoryTopologyActiveWork(projection.activity)).toEqual({
      overflowCount: 1,
      rows: [
        { durationTicks: 7, id: "work-a" },
        { durationTicks: 4, id: "work-b" },
        { durationTicks: 4, id: "work-c" },
      ],
    });
  });
});
