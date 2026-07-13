// biome-ignore-all lint/nursery/noExcessiveLinesPerFile: the materializer fixture keeps every reducer boundary visible in one focused behavioral suite.
import { describe, expect, it } from "vitest";

import { FACTORY_EVENT_TYPES, type FactoryEvent } from "../../../api/events";
import {
  applyMaterializedWorkOutcomeEvent,
  createMaterializedWorkOutcomeState,
  MATERIALIZED_WORK_OUTCOME_VERSION,
  MAX_MATERIALIZED_WORK_OUTCOME_SAMPLES,
  type MaterializedWorkOutcomeState,
} from "./materialized-work-outcome";

describe("empty materialized work outcome", () => {
  it("creates an explicit empty JSON-safe continuation state", () => {
    const state = createMaterializedWorkOutcomeState();

    expect(state).toEqual({
      accumulator: {
        activeDispatchesByID: {},
        appliedEventCount: 0,
        completedAcceptedCount: 0,
        completedDispatchCount: 0,
        failedWorkItemsByID: {},
        initialPlaceIDs: [],
        workItemsByID: {},
      },
      counts: {
        completed: 0,
        dispatched: 0,
        failed: 0,
        inFlight: 0,
        queued: 0,
      },
      cursor: null,
      failedByWorkType: {},
      failedWorkLabels: [],
      samples: [],
      version: MATERIALIZED_WORK_OUTCOME_VERSION,
    });
    expect(JSON.parse(JSON.stringify(state))).toEqual(state);
  });
});

describe("materialized work outcome continuation", () => {
  it("materializes the established uninterrupted timeline and continuation context", () => {
    const states = reduceStates(baselineEvents());
    const queuedState = states[1];
    const activeState = states[4];
    const acceptedState = states[6];
    const finalState = states.at(-1);

    expect(queuedState.cursor).toEqual({
      eventID: "work-request",
      eventTime: "2026-04-29T12:00:01.000Z",
      sequence: 1,
      tick: 1,
    });
    expect(queuedState.counts).toEqual({
      completed: 0,
      dispatched: 0,
      failed: 0,
      inFlight: 0,
      queued: 1,
    });
    expect(queuedState.accumulator.initialPlaceIDs).toEqual(["story:init"]);
    expect(queuedState.accumulator.workItemsByID["work-1"]).toEqual({
      displayName: "Story One",
      id: "work-1",
      placeID: "story:init",
      traceID: "trace-1",
      workTypeID: "story",
    });
    expect(activeState.accumulator.activeDispatchesByID).toEqual({
      "dispatch-1": {
        inputWorkIDs: ["work-1"],
        systemOnly: false,
      },
      "dispatch-system-time": {
        inputWorkIDs: [],
        systemOnly: true,
      },
    });
    expect(activeState.counts).toEqual({
      completed: 0,
      dispatched: 1,
      failed: 0,
      inFlight: 1,
      queued: 0,
    });
    expect(acceptedState.accumulator).toMatchObject({
      activeDispatchesByID: {},
      completedAcceptedCount: 1,
      completedDispatchCount: 1,
    });

    expect(finalState).toMatchObject({
      counts: {
        completed: 1,
        dispatched: 2,
        failed: 1,
        inFlight: 0,
        queued: 0,
      },
      cursor: {
        eventID: "dispatch-response-2",
        eventTime: "2026-04-29T12:00:06.000Z",
        sequence: 9,
        tick: 6,
      },
      failedByWorkType: { story: 1 },
      failedWorkLabels: ["Story Two"],
      version: 1,
    });
    expect(finalState?.accumulator.failedWorkItemsByID).toEqual({
      "work-2": {
        displayName: "Story Two",
        id: "work-2",
        traceID: "trace-2",
        workTypeID: "story",
      },
    });
    expect(finalState?.samples).toEqual(expectedSamples());
    expect(JSON.parse(JSON.stringify(finalState))).toEqual(finalState);
  });
});

describe("materialized work outcome purity", () => {
  it("returns new values without mutating the input state or event", () => {
    const inputState = deepFreeze(createMaterializedWorkOutcomeState());
    const inputEvent = deepFreeze(baselineEvents()[0]);
    const stateBefore = structuredClone(inputState);
    const eventBefore = structuredClone(inputEvent);

    const nextState = applyMaterializedWorkOutcomeEvent(inputState, inputEvent);

    expect(nextState).not.toBe(inputState);
    expect(nextState.accumulator).not.toBe(inputState.accumulator);
    expect(nextState.samples).not.toBe(inputState.samples);
    expect(inputState).toEqual(stateBefore);
    expect(inputEvent).toEqual(eventBefore);
  });
});

describe("materialized work outcome event behavior", () => {
  it("preserves missing-data, output-work, and system-time exclusion behavior", () => {
    const events = [
      factoryEvent(
        "initial",
        0,
        0,
        FACTORY_EVENT_TYPES.initialStructureRequest,
        {
          factory: factoryDefinition(),
        },
      ),
      factoryEvent(
        "unknown-accepted",
        1,
        1,
        FACTORY_EVENT_TYPES.dispatchResponse,
        {
          outcome: "ACCEPTED",
          outputWork: [],
        },
        { dispatchId: "unknown-dispatch" },
      ),
      factoryEvent(
        "unknown-failed",
        2,
        2,
        FACTORY_EVENT_TYPES.dispatchResponse,
        {
          outcome: "FAILED",
          outputWork: [],
        },
        { dispatchId: "missing-input-dispatch" },
      ),
      factoryEvent(
        "system-request",
        3,
        3,
        FACTORY_EVENT_TYPES.dispatchRequest,
        {
          inputs: [{ workId: "system-work" }],
          transitionId: "__system_time:expire",
        },
        { dispatchId: "system-dispatch" },
      ),
      factoryEvent(
        "system-response",
        4,
        4,
        FACTORY_EVENT_TYPES.dispatchResponse,
        {
          outcome: "ACCEPTED",
          outputWork: [
            {
              name: "System clock",
              state: "expired",
              traceId: "system-trace",
              workId: "system-work",
              workTypeName: "__system_time",
            },
          ],
        },
        { dispatchId: "system-dispatch" },
      ),
    ];

    const state = reduce(events);

    expect(state.counts).toEqual({
      completed: 1,
      dispatched: 2,
      failed: 0,
      inFlight: 0,
      queued: 0,
    });
    expect(state.accumulator.workItemsByID).toEqual({});
    expect(state.failedByWorkType).toEqual({});
    expect(state.failedWorkLabels).toEqual([]);
  });
});

describe("materialized work outcome retention", () => {
  it("bounds samples while preserving the latest sample and continuation accumulator", () => {
    let state = applyMaterializedWorkOutcomeEvent(
      createMaterializedWorkOutcomeState(),
      factoryEvent("run", 0, 0, FACTORY_EVENT_TYPES.runRequest, {
        factory: factoryDefinition(),
        recordedAt: "2026-04-29T12:00:00Z",
      }),
    );
    for (
      let tick = 1;
      tick <= MAX_MATERIALIZED_WORK_OUTCOME_SAMPLES;
      tick += 1
    ) {
      state = applyMaterializedWorkOutcomeEvent(
        state,
        factoryEvent(
          `lifecycle-${tick}`,
          tick,
          tick,
          FACTORY_EVENT_TYPES.sessionStarted,
          {},
        ),
      );
    }
    state = applyMaterializedWorkOutcomeEvent(
      state,
      factoryEvent(
        "tail-work",
        MAX_MATERIALIZED_WORK_OUTCOME_SAMPLES + 1,
        MAX_MATERIALIZED_WORK_OUTCOME_SAMPLES + 1,
        FACTORY_EVENT_TYPES.workRequest,
        {
          works: [
            {
              name: "Tail Story",
              traceId: "tail-trace",
              workId: "tail-work",
              workTypeName: "story",
            },
          ],
        },
      ),
    );

    expect(state.samples).toHaveLength(MAX_MATERIALIZED_WORK_OUTCOME_SAMPLES);
    expect(state.samples[0]?.tick).toBe(2);
    expect(state.samples.at(-1)).toMatchObject({
      queuedCount: 1,
      tick: MAX_MATERIALIZED_WORK_OUTCOME_SAMPLES + 1,
    });
    expect(state.counts.queued).toBe(1);
    expect(state.accumulator).toMatchObject({
      appliedEventCount: MAX_MATERIALIZED_WORK_OUTCOME_SAMPLES + 2,
      initialPlaceIDs: ["story:init"],
      workItemsByID: {
        "tail-work": {
          placeID: "story:init",
          workTypeID: "story",
        },
      },
    });
  });
});

function reduce(events: FactoryEvent[]): MaterializedWorkOutcomeState {
  return events.reduce(
    applyMaterializedWorkOutcomeEvent,
    createMaterializedWorkOutcomeState(),
  );
}

function reduceStates(events: FactoryEvent[]): MaterializedWorkOutcomeState[] {
  const states: MaterializedWorkOutcomeState[] = [];
  let state = createMaterializedWorkOutcomeState();
  for (const event of events) {
    state = applyMaterializedWorkOutcomeEvent(state, event);
    states.push(state);
  }
  return states;
}

function baselineEvents(): FactoryEvent[] {
  return [
    factoryEvent("run-started", 0, 0, FACTORY_EVENT_TYPES.runRequest, {
      factory: factoryDefinition(),
      recordedAt: "2026-04-29T12:00:00Z",
    }),
    factoryEvent("work-request", 1, 1, FACTORY_EVENT_TYPES.workRequest, {
      works: [
        {
          name: "Story One",
          traceId: "trace-1",
          workId: "work-1",
          workTypeName: "story",
        },
      ],
    }),
    factoryEvent("system-work", 1, 2, FACTORY_EVENT_TYPES.workRequest, {
      works: [
        {
          name: "System clock",
          traceId: "system-trace",
          workId: "system-work",
          workTypeName: "__system_time",
        },
      ],
    }),
    factoryEvent(
      "dispatch-request",
      2,
      3,
      FACTORY_EVENT_TYPES.dispatchRequest,
      {
        inputs: [{ workId: "work-1" }],
        transitionId: "review",
      },
      { dispatchId: "dispatch-1" },
    ),
    factoryEvent(
      "system-dispatch",
      2,
      4,
      FACTORY_EVENT_TYPES.dispatchRequest,
      {
        inputs: [{ workId: "system-work" }],
        transitionId: "__system_time:expire",
      },
      { dispatchId: "dispatch-system-time" },
    ),
    factoryEvent(
      "dispatch-response",
      3,
      5,
      FACTORY_EVENT_TYPES.dispatchResponse,
      {
        outcome: "ACCEPTED",
        outputWork: [
          {
            name: "Story One",
            state: "done",
            traceId: "trace-1",
            workId: "work-1",
            workTypeName: "story",
          },
        ],
      },
      { dispatchId: "dispatch-1" },
    ),
    factoryEvent(
      "system-response",
      3,
      6,
      FACTORY_EVENT_TYPES.dispatchResponse,
      {
        outcome: "ACCEPTED",
        outputWork: [
          {
            name: "System clock",
            state: "expired",
            traceId: "system-trace",
            workId: "system-work",
            workTypeName: "__system_time",
          },
        ],
      },
      { dispatchId: "dispatch-system-time" },
    ),
    factoryEvent("work-request-2", 4, 7, FACTORY_EVENT_TYPES.workRequest, {
      works: [
        {
          name: "Story Two",
          traceId: "trace-2",
          workId: "work-2",
          workTypeName: "story",
        },
      ],
    }),
    factoryEvent(
      "dispatch-request-2",
      5,
      8,
      FACTORY_EVENT_TYPES.dispatchRequest,
      {
        inputs: [{ workId: "work-2" }],
        transitionId: "review",
      },
      { dispatchId: "dispatch-2" },
    ),
    factoryEvent(
      "dispatch-response-2",
      6,
      9,
      FACTORY_EVENT_TYPES.dispatchResponse,
      {
        outcome: "FAILED",
        outputWork: [
          {
            name: "Story Two",
            state: "failed",
            traceId: "trace-2",
            workId: "work-2",
            workTypeName: "story",
          },
        ],
      },
      { dispatchId: "dispatch-2" },
    ),
  ];
}

function expectedSamples() {
  return [
    sample(0, 0, 0, 0, 0, 0, 1777464000000),
    sample(1, 1, 0, 0, 0, 0, 1777464001000),
    sample(2, 0, 1, 1, 0, 0, 1777464002000),
    sample(3, 0, 0, 1, 1, 0, 1777464003000),
    sample(4, 1, 0, 1, 1, 0, 1777464004000),
    sample(5, 0, 1, 2, 1, 0, 1777464005000),
    {
      ...sample(6, 0, 0, 2, 1, 1, 1777464006000),
      failedByWorkType: { story: 1 },
      failedWorkLabels: ["Story Two"],
    },
  ];
}

function sample(
  tick: number,
  queuedCount: number,
  inFlightCount: number,
  dispatchedCount: number,
  completedCount: number,
  failedCount: number,
  observedAt: number,
) {
  return {
    completedCount,
    dispatchedCount,
    failedByWorkType: {},
    failedCount,
    failedWorkLabels: [],
    inFlightCount,
    observedAt,
    queuedCount,
    tick,
  };
}

function factoryDefinition() {
  return {
    resources: [],
    workTypes: [
      {
        name: "story",
        states: [
          { name: "init", type: "INITIAL" as const },
          { name: "done", type: "TERMINAL" as const },
          { name: "failed", type: "FAILED" as const },
        ],
      },
      {
        name: "__system_time",
        states: [
          { name: "pending", type: "INITIAL" as const },
          { name: "expired", type: "TERMINAL" as const },
        ],
      },
    ],
    workers: [],
    workstations: [],
  };
}

function factoryEvent(
  id: string,
  tick: number,
  sequence: number,
  type: FactoryEvent["type"],
  payload: FactoryEvent["payload"],
  context: Partial<FactoryEvent["context"]> = {},
): FactoryEvent {
  return {
    context: {
      eventTime: new Date(Date.UTC(2026, 3, 29, 12, 0, tick)).toISOString(),
      sequence,
      tick,
      ...context,
    },
    id,
    payload,
    type,
  };
}

function deepFreeze<T>(value: T): T {
  if (value && typeof value === "object") {
    Object.freeze(value);
    for (const child of Object.values(value)) {
      deepFreeze(child);
    }
  }
  return value;
}
