import { describe, expect, it } from "bun:test";

import {
  factoryWorkStateName,
  factoryWorkToItem,
  outputPlaceForWorkstation,
  seedResourceOccupancy,
} from "./replayFactoryTopology";
import {
  addRelation,
  addToken,
  addTraceDispatch,
  addTraceRequest,
  addTraceWork,
  consumeResourceUnits,
  releaseResourceUnits,
  removeWorkToken,
  resourceAvailablePlaceID,
  resourceTokenID,
  traceToken,
} from "./replayGraphState";
import type { ReplayWorldState } from "./types";

function replayStateWithResourcePlaces(): ReplayWorldState {
  return {
    activeDispatches: {},
    completedDispatches: [],
    factory_state: "UNKNOWN",
    failedWorkDetailsByWorkID: {},
    failedWorkItemsByID: {},
    inferenceAttemptsByDispatchID: {},
    occupancyByID: {},
    providerSessions: [],
    relationsByWorkID: {},
    runtime: { in_flight_dispatch_count: 0, session: { has_data: false } },
    scriptRequestsByDispatchID: {},
    scriptResponsesByDispatchID: {},
    terminalWorkByID: {},
    tick_count: 1,
    topology: {
      places: [
        { id: "agent-slot:busy", state: "busy", type_id: "agent-slot" },
        {
          id: "agent-slot:available",
          state: "available",
          type_id: "agent-slot",
        },
        { id: "gpu-slot:reserved", state: "reserved", type_id: "gpu-slot" },
        { id: "gpu-slot:waiting", state: "waiting", type_id: "gpu-slot" },
      ],
      resources: [
        { capacity: 2, id: "agent-slot" },
        { capacity: 1, id: "gpu-slot" },
        { capacity: 1, id: "missing-slot" },
      ],
    },
    tracesByID: {},
    tracesByWorkID: {},
    uptime_seconds: 0,
    workItemsByID: {},
    workstationRequestsByDispatchID: {},
    workRequestsByID: {},
  };
}

describe("seedResourceOccupancy", () => {
  it("seeds resource tokens into the preferred available place", () => {
    const tokens: Array<{ placeID: string | undefined; tokenID: string }> = [];

    seedResourceOccupancy(
      replayStateWithResourcePlaces(),
      (_state, placeID, tokenID) => {
        tokens.push({ placeID, tokenID });
      },
      (resourceID, index) => `${resourceID}-${index}`,
    );

    expect(tokens).toEqual([
      { placeID: "agent-slot:available", tokenID: "agent-slot-0" },
      { placeID: "agent-slot:available", tokenID: "agent-slot-1" },
      { placeID: "gpu-slot:reserved", tokenID: "gpu-slot-0" },
    ]);
  });
});

describe("timeline factory topology helpers", () => {
  it("falls back across legacy work shapes and workstation rejection routes", () => {
    const state = replayStateWithResourcePlaces();
    state.topology.places = [
      { id: "story:new", state: "new", type_id: "story" },
      { id: "story:rejected", state: "rejected", type_id: "story" },
    ];
    state.topology.workstations = [
      {
        id: "review",
        input_place_ids: ["story:new"],
        name: "Review",
        rejection_place_ids: ["story:rejected"],
      },
    ];

    expect(factoryWorkToItem(state, { workId: "work-without-type" })).toEqual(
      expect.objectContaining({
        id: "work-without-type",
        work_type_id: "",
      }),
    );
    expect(factoryWorkStateName({ name: "empty-state", state: "" })).toBe(
      undefined,
    );
    expect(
      outputPlaceForWorkstation(state.topology, "review", "REJECTED", "story"),
    ).toBe("story:rejected");
  });
});

it("ignores incomplete trace and resource replay events", () => {
  const state = replayStateWithResourcePlaces();

  addTraceWork(state, {
    id: "work-without-trace",
    place_id: "story:new",
    work_type_id: "story",
  });
  addTraceRequest(state, "", "request-1");
  addTraceRequest(state, "trace-1", "");
  removeWorkToken(state, "missing-work");
  releaseResourceUnits(
    state,
    [
      {
        placeID: "agent-slot:available",
        resourceID: "agent-slot",
        tokenID: "",
      },
    ],
    [{ name: "agent-slot" }, { name: "gpu-slot" }],
  );

  expect(state.tracesByID).toEqual({});
  expect(state.occupancyByID).toEqual({});
});

it("records trace, relation, work-token, and resource occupancy state", () => {
  const state = replayStateWithResourcePlaces();
  state.topology.places.push({
    id: "story:new",
    state: "new",
    type_id: "story",
  });
  state.workItemsByID["work-1"] = {
    id: "work-1",
    display_name: "Draft story",
    place_id: "story:new",
    trace_id: "trace-1",
    work_type_id: "story",
  };
  state.workItemsByID["work-2"] = {
    id: "work-2",
    display_name: "Review story",
    place_id: "story:new",
    trace_id: "trace-2",
    work_type_id: "story",
  };

  addTraceWork(state, state.workItemsByID["work-1"]);
  addTraceRequest(state, "trace-1", "request-1");
  addRelation(state, {
    request_id: "request-1",
    requiredState: "review",
    source_work_id: "work-1",
    sourceWorkName: "Draft story",
    targetWorkId: "work-2",
    targetWorkName: "Review story",
    trace_id: "trace-1",
    type: "created_by",
  });
  addTraceDispatch(
    state,
    "trace-1",
    {
      consumedResources: [],
      dispatchID: "dispatch-1",
      durationMillis: 10,
      endTime: "2026-05-27T01:00:01Z",
      inputItems: [],
      outcome: "ACCEPTED",
      outputItems: [],
      outputMutations: [],
      resources: [],
      startedAt: "2026-05-27T01:00:00Z",
      systemOnly: false,
      traceIDs: ["trace-1"],
      transitionID: "write-story",
      workItems: [],
      workstationName: "Write story",
    },
    (completion) => ({
      request_id: completion.dispatchID,
      transition_id: completion.transitionID,
    }),
  );
  addToken(state, "story:new", "work-1", "work-1");
  addToken(
    state,
    resourceAvailablePlaceID("agent-slot"),
    resourceTokenID("agent-slot", 0),
  );

  const consumed = consumeResourceUnits(state, [
    { name: "agent-slot" },
    { name: "" },
  ]);
  releaseResourceUnits(state, consumed, [{ name: "agent-slot" }]);
  removeWorkToken(state, "work-1");

  expect(state.tracesByID["trace-1"]).toMatchObject({
    request_ids: ["request-1"],
    transition_ids: ["write-story"],
    work_ids: ["work-1"],
    workstation_sequence: ["Write story"],
  });
  expect(state.tracesByID["trace-2"].relations).toEqual([
    expect.objectContaining({
      required_state: "review",
      target_work_id: "work-2",
    }),
  ]);
  expect(state.relationsByWorkID["work-1"]).toHaveLength(1);
  expect(consumed).toEqual([
    {
      placeID: "agent-slot:available",
      resourceID: "agent-slot",
      tokenID: "agent-slot:resource:0",
    },
  ]);
  expect(state.occupancyByID["story:new"]).toBeUndefined();
  expect(state.occupancyByID["agent-slot:available"]).toMatchObject({
    resourceTokenIDs: ["agent-slot:resource:0"],
    tokenCount: 1,
  });
  expect(
    traceToken(state.workItemsByID["work-1"], "2026-05-27T01:00:00Z"),
  ).toMatchObject({
    name: "Draft story",
    place_id: "story:new",
    token_id: "work-1",
    trace_id: "trace-1",
    work_type_id: "story",
  });
});
