import type {
  FactoryDefinition,
  FactoryEvent,
} from "@you-agent-factory/client";
import { describe, expect, it } from "vitest";

import { projectFactoryLoad, projectFactoryLoadAtTick } from "./index.js";

const factory: FactoryDefinition = {
  name: "publishing",
  resources: [{ capacity: 2, id: "gpu-stable", name: "gpu" }],
  workTypes: [
    {
      id: "story-stable",
      name: "story",
      states: [
        { id: "ready-stable", name: "ready", type: "INITIAL" },
        { id: "review-stable", name: "review", type: "PROCESSING" },
        { id: "done-stable", name: "done", type: "TERMINAL" },
      ],
    },
  ],
};

function event(
  id: string,
  type: FactoryEvent["type"],
  tick: number,
  sequence: number,
  payload: unknown,
  context: Partial<FactoryEvent["context"]> = {},
): FactoryEvent {
  return {
    context: {
      eventTime: `2026-07-18T20:00:${String(sequence).padStart(2, "0")}Z`,
      sequence,
      tick,
      ...context,
    },
    id,
    payload,
    schemaVersion: "agent-factory.event.v1",
    type,
  } as FactoryEvent;
}

function topologyEvent(): FactoryEvent {
  return event("topology", "INITIAL_STRUCTURE_REQUEST", 1, 0, { factory });
}

function workRequest(
  id: string,
  tick: number,
  sequence: number,
  workId: string,
  workTypeName = "story",
): FactoryEvent {
  return event(id, "WORK_REQUEST", tick, sequence, {
    type: "WORK_REQUEST",
    works: [{ name: workId, workId, workTypeName }],
  });
}

function dispatchRequest(
  id: string,
  tick: number,
  sequence: number,
  dispatchId: string,
  workId: string,
  resources: unknown,
): FactoryEvent {
  return event(
    id,
    "DISPATCH_REQUEST",
    tick,
    sequence,
    { inputs: [{ workId }], resources, transitionId: "review" },
    { dispatchId, workIds: [workId] },
  );
}

describe("projectFactoryLoad", () => {
  it("deduplicates customer Work and attaches counts to stable Work State IDs", () => {
    const result = projectFactoryLoad({
      activeDispatches: [],
      factory,
      selectedTick: 4,
      works: [
        { id: "work-b", stateName: "ready", workTypeId: "story" },
        { id: "work-a", stateId: "ready-stable", workTypeId: "story" },
        { id: "work-a", stateName: "ready", workTypeId: "story-stable" },
        {
          id: "system",
          stateName: "ready",
          workTypeId: "__system_time",
        },
      ],
    });

    expect(result.workStateCounts).toEqual([
      {
        count: 0,
        evidence: "known",
        workIds: [],
        workStateId: "story-stable:done-stable",
        workStateNodeId: "work-state:story-stable:done-stable",
        workTypeId: "story-stable",
      },
      {
        count: 2,
        evidence: "known",
        workIds: ["work-a", "work-b"],
        workStateId: "story-stable:ready-stable",
        workStateNodeId: "work-state:story-stable:ready-stable",
        workTypeId: "story-stable",
      },
      {
        count: 0,
        evidence: "known",
        workIds: [],
        workStateId: "story-stable:review-stable",
        workStateNodeId: "work-state:story-stable:review-stable",
        workTypeId: "story-stable",
      },
    ]);
  });

  it("distinguishes unavailable load evidence from known zero", () => {
    const unavailable = projectFactoryLoad({ factory, selectedTick: 2 });
    const knownZero = projectFactoryLoad({
      activeDispatches: [],
      factory,
      selectedTick: 2,
      works: [],
    });

    expect(
      unavailable.workStateCounts.every(
        (count) =>
          count.evidence === "unavailable" && count.count === undefined,
      ),
    ).toBe(true);
    expect(unavailable.resourceOccupancy).toEqual([
      {
        capacity: 2,
        capacityEvidence: "known",
        evidence: "unavailable",
        resourceId: "gpu-stable",
        resourceNodeId: "resource:gpu-stable",
      },
    ]);
    expect(knownZero.resourceOccupancy[0]).toMatchObject({
      availableQuantity: 2,
      evidence: "known",
      occupiedQuantity: 0,
    });
  });

  it("preserves contradictory and over-capacity resource evidence as issues", () => {
    const result = projectFactoryLoad({
      activeDispatches: [
        {
          id: "dispatch-over",
          resourceClaims: [{ quantity: 3, resourceName: "gpu" }],
        },
        {
          id: "dispatch-missing",
          resourceClaims: [{ resourceName: "missing" }],
        },
      ],
      factory,
      selectedTick: 6,
      works: [],
    });

    expect(result.resourceOccupancy[0]).toMatchObject({
      availableQuantity: 0,
      occupiedQuantity: 3,
    });
    expect(result.issues.map((issue) => issue.code)).toEqual([
      "UNRESOLVED_RESOURCE_CLAIM",
      "RESOURCE_CAPACITY_EXCEEDED",
    ]);
  });
});

describe("projectFactoryLoad contradictory evidence", () => {
  it("deduplicates equivalent Dispatch claims by stable Dispatch identity", () => {
    const claim = {
      id: "dispatch-1",
      resourceClaims: [{ resourceName: "gpu" }],
    };
    const result = projectFactoryLoad({
      activeDispatches: [claim, structuredClone(claim)],
      factory,
      selectedTick: 6,
      works: [],
    });

    expect(result.resourceOccupancy[0].occupiedQuantity).toBe(1);
    expect(result.issues).toEqual([]);
  });

  it("marks conflicting claims for one Dispatch as unavailable", () => {
    const result = projectFactoryLoad({
      activeDispatches: [
        {
          id: "dispatch-1",
          resourceClaims: [{ resourceName: "gpu" }],
        },
        { id: "dispatch-1", resourceClaims: [] },
      ],
      factory,
      selectedTick: 6,
      works: [],
    });

    expect(result.resourceOccupancy[0].evidence).toBe("unavailable");
    expect(result.issues[0].code).toBe("CONTRADICTORY_RESOURCE_CLAIM");
  });

  it("reports invalid capacity and contradictory Work evidence deterministically", () => {
    const invalidFactory = structuredClone(factory);
    if (!invalidFactory.resources?.[0]) throw new Error("expected resource");
    invalidFactory.resources[0].capacity = Number.NaN;
    const result = projectFactoryLoad({
      activeDispatches: [],
      factory: invalidFactory,
      selectedTick: 7,
      works: [
        { id: "work-1", stateName: "ready", workTypeId: "story" },
        { id: "work-1", stateName: "review", workTypeId: "story" },
        { id: "dangling", stateName: "missing", workTypeId: "story" },
      ],
    });

    expect(result.resourceOccupancy[0]).toMatchObject({
      capacityEvidence: "unavailable",
      evidence: "known",
      occupiedQuantity: 0,
    });
    expect(result.issues.map((issue) => issue.code)).toEqual([
      "INVALID_RESOURCE_CAPACITY",
      "UNRESOLVED_WORK_STATE",
      "CONTRADICTORY_WORK_STATE",
    ]);
  });
});

describe("projectFactoryLoadAtTick", () => {
  it("reconstructs Work creation, movement, consumption, and same-tick output", () => {
    const events = [
      event(
        "move",
        "WORK_STATE_CHANGE",
        2,
        2,
        {
          fromPlaceId: "story:ready",
          fromState: "ready",
          source: "runtime",
          toPlaceId: "story:review",
          toState: "review",
          workId: "work-a",
          workTypeName: "story",
        },
        { workIds: ["work-a"] },
      ),
      workRequest("request-b", 2, 1, "work-b"),
      topologyEvent(),
      workRequest("request-a", 2, 0, "work-a"),
      dispatchRequest("start-b", 3, 0, "dispatch-b", "work-b", [
        { capacity: 2, name: "gpu" },
      ]),
      event(
        "finish-b",
        "DISPATCH_RESPONSE",
        3,
        1,
        {
          outcome: "CONTINUE",
          outputWork: [
            {
              name: "work-b",
              state: { name: "review", type: "PROCESSING" },
              workId: "work-b",
              workTypeName: "story",
            },
          ],
          transitionId: "review",
        },
        { dispatchId: "dispatch-b", workIds: ["work-b"] },
      ),
    ];

    const tickTwo = projectFactoryLoadAtTick({ events, tick: 2 });
    const tickThree = projectFactoryLoadAtTick({ events, tick: 3 });
    expect(
      tickTwo.workStateCounts.find((count) =>
        count.workStateId.endsWith(":ready-stable"),
      )?.workIds,
    ).toEqual(["work-b"]);
    expect(
      tickTwo.workStateCounts.find((count) =>
        count.workStateId.endsWith(":review-stable"),
      )?.workIds,
    ).toEqual(["work-a"]);
    expect(
      tickThree.workStateCounts.find((count) =>
        count.workStateId.endsWith(":review-stable"),
      )?.workIds,
    ).toEqual(["work-a", "work-b"]);
    expect(tickThree.resourceOccupancy[0]).toMatchObject({
      availableQuantity: 2,
      occupiedQuantity: 0,
    });
  });
});

describe("projectFactoryLoadAtTick resource occupancy", () => {
  it("tracks shared-capacity claims, release, and incomplete claim evidence", () => {
    const starts = [
      topologyEvent(),
      dispatchRequest("start-a", 2, 0, "dispatch-a", "work-a", [
        { capacity: 2, name: "gpu" },
      ]),
      dispatchRequest("start-b", 2, 1, "dispatch-b", "work-b", [
        { capacity: 2, name: "gpu" },
      ]),
    ];
    const active = projectFactoryLoadAtTick({ events: starts, tick: 2 });
    const released = projectFactoryLoadAtTick({
      events: [
        ...starts,
        event(
          "finish-a",
          "DISPATCH_RESPONSE",
          3,
          0,
          { outcome: "CONTINUE", transitionId: "review" },
          { dispatchId: "dispatch-a" },
        ),
      ],
      tick: 3,
    });
    const incomplete = projectFactoryLoadAtTick({
      events: [
        topologyEvent(),
        dispatchRequest(
          "unknown",
          2,
          0,
          "dispatch-unknown",
          "work-c",
          undefined,
        ),
      ],
      tick: 2,
    });

    expect(active.resourceOccupancy[0]).toMatchObject({
      availableQuantity: 0,
      occupiedQuantity: 2,
    });
    expect(released.resourceOccupancy[0]).toMatchObject({
      availableQuantity: 1,
      occupiedQuantity: 1,
    });
    expect(incomplete.resourceOccupancy[0].evidence).toBe("unavailable");
  });
});
