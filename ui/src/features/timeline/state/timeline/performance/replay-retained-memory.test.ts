import { parseFactoryRecording } from "@you-agent-factory/client";
import { advanceFactoryReplay } from "@you-agent-factory/factory-replay";
import { describe, expect, it } from "vitest";

import type { FactoryEvent } from "../../../../../api/events";
import { useFactoryTimelineStore } from "../../factoryTimelineStore";
import { hostedFactoryReplayReducer } from "../buildSnapshot";
import { projectSnapshot } from "../projectSnapshot";
import { emptyReplayWorldState } from "../replayWorldStateSupport";
import { MAX_TIMELINE_WORLD_VIEW_CACHE_ENTRIES } from "../storeState";

const EVENT_COUNT = 10_000;
const FINAL_TICK = 10_000;
const MAX_RETAINED_BYTES = 12_000_000;

const factory = {
  name: "retained-memory-factory",
  workTypes: [
    {
      id: "story-stable",
      name: "story",
      states: [
        { id: "ready-stable", name: "ready", type: "INITIAL" },
        { id: "done-stable", name: "done", type: "TERMINAL" },
        { id: "failed-stable", name: "failed", type: "FAILED" },
      ],
    },
  ],
  workstations: [
    {
      id: "review-stable",
      inputs: [{ state: "ready", workType: "story" }],
      name: "review",
      outputs: [{ state: "done", workType: "story" }],
      worker: "",
    },
  ],
};

function event(
  id: string,
  type: FactoryEvent["type"],
  tick: number,
  sequence: number,
  payload: FactoryEvent["payload"],
): FactoryEvent {
  return {
    context: {
      eventTime: new Date(Date.UTC(2026, 6, 19, 0, 0, sequence)).toISOString(),
      sequence,
      sessionId: "retained-memory-session",
      sessionSequence: sequence,
      tick,
    },
    id,
    payload,
    schemaVersion: "agent-factory.event.v1",
    type,
  } as FactoryEvent;
}

function deterministicRecording() {
  const events: FactoryEvent[] = [
    event("topology", "INITIAL_STRUCTURE_REQUEST", 0, 1, { factory }),
    event("work", "WORK_REQUEST", 1, 2, {
      type: "FACTORY_REQUEST_BATCH",
      works: [
        { name: "Retained story", workId: "work-1", workTypeName: "story" },
      ],
    }),
  ];
  for (let sequence = 3; sequence <= EVENT_COUNT - 2; sequence += 1) {
    events.push(
      event(
        `memory-event-${sequence}`,
        "FACTORY_STATE_RESPONSE",
        sequence,
        sequence,
        { state: sequence % 2 === 0 ? "RUNNING" : "PAUSED" },
      ),
    );
  }
  events.push(
    event("same-tick-failed", "WORK_STATE_CHANGE", FINAL_TICK, 9_999, {
      fromPlaceId: "story:ready",
      fromState: "ready",
      source: "api",
      toPlaceId: "story:failed",
      toState: "failed",
      workId: "work-1",
      workTypeName: "story",
    }),
    event("same-tick-done", "WORK_STATE_CHANGE", FINAL_TICK, 10_000, {
      fromPlaceId: "story:failed",
      fromState: "failed",
      source: "api",
      toPlaceId: "story:done",
      toState: "done",
      workId: "work-1",
      workTypeName: "story",
    }),
  );
  return parseFactoryRecording({
    events,
    factory,
    id: "retained-memory-10k-recording",
    schemaVersion: "factory-recording/v1",
    title: "Deterministic retained-memory recording",
  });
}

function replayMeasurement(
  events: FactoryEvent[],
  checkpoint = {
    acceptedEventIDs: [] as string[],
    selectedTick: 0,
    state: emptyReplayWorldState(0),
  },
) {
  const result = advanceFactoryReplay({
    checkpoint,
    cloneState: structuredClone,
    events: events as Parameters<typeof advanceFactoryReplay>[0]["events"],
    reducer: hostedFactoryReplayReducer,
    setSelectedTick: (state, tick) => {
      state.tick_count = tick;
      return state;
    },
    tick: FINAL_TICK,
  });
  return {
    appliedEventIDs: result.appliedEvents.map((item) => item.id),
    checkpoint: result.checkpoint,
    world: projectSnapshot(result.state),
  };
}

function serializedBytes(value: unknown): number {
  return new TextEncoder().encode(JSON.stringify(value)).byteLength;
}

function retainedGraphMeasurement(
  events: FactoryEvent[],
  packageCheckpoint: ReturnType<typeof replayMeasurement>["checkpoint"],
  packageWorld: ReturnType<typeof replayMeasurement>["world"],
) {
  const timeline = useFactoryTimelineStore.getState();
  timeline.reset();
  timeline.replaceEvents(events);
  for (let selection = 1; selection <= 64; selection += 1) {
    useFactoryTimelineStore
      .getState()
      .selectTick(Math.floor((FINAL_TICK * selection) / 64));
  }

  const retainedState = useFactoryTimelineStore.getState();
  const worldViewCacheEntries = Object.keys(
    retainedState.worldViewCache,
  ).length;
  const checkpointCount = retainedState.currentReplayCheckpoint ? 1 : 0;
  const retainedGraph = {
    canonicalEventHistory: retainedState.events,
    checkpointCount,
    hostedCheckpoint: retainedState.currentReplayCheckpoint,
    hostedReceivedEventIDs: retainedState.receivedEventIDs,
    hostedWorldViewCache: retainedState.worldViewCache,
    packageCheckpoint,
    packageWorld,
  };

  // The fixture parser and transient applied-event batches are outside this
  // retained graph. Resetting the singleton proves it does not remain an
  // additional owner after the measured consumer state is detached.
  useFactoryTimelineStore.getState().reset();
  const retainedBytes = serializedBytes(retainedGraph);
  assertRetainedBudget(retainedBytes, MAX_RETAINED_BYTES);
  return { checkpointCount, retainedBytes, worldViewCacheEntries };
}

function assertRetainedBudget(retainedBytes: number, maximumBytes: number) {
  if (retainedBytes > maximumBytes) {
    throw new Error(
      `Factory replay retained-memory budget exceeded: ${retainedBytes} bytes retained after cleanup; maximum is ${maximumBytes} bytes.`,
    );
  }
}

describe("hosted replay retained-state budget", () => {
  it("replays 10,000 events to the expected Work and topology state within budget", () => {
    const recording = deterministicRecording();
    expect(recording.events).toHaveLength(EVENT_COUNT);

    const historical = replayMeasurement(recording.events.slice(0, -1));
    expect(historical.world.factoryReplay.workProgress.counts.failed).toBe(1);

    const sameTickTail = recording.events.at(-1);
    expect(sameTickTail).toBeDefined();
    const completed = replayMeasurement(
      [
        recording.events[0] as FactoryEvent,
        sameTickTail as FactoryEvent,
        structuredClone(sameTickTail) as FactoryEvent,
      ],
      historical.checkpoint,
    );

    expect(completed.appliedEventIDs).toEqual(["same-tick-done"]);
    expect(completed.checkpoint.acceptedEventIDs).toHaveLength(EVENT_COUNT);
    expect(completed.world.tick_count).toBe(FINAL_TICK);
    expect(completed.world.factoryReplay.workProgress).toMatchObject({
      counts: {
        active: 0,
        completed: 1,
        failed: 0,
        queued: 0,
        unclassified: 0,
      },
      total: 1,
    });
    expect(
      completed.world.factoryReplay.topology.nodes.map(({ id }) => id),
    ).toEqual(
      expect.arrayContaining([
        "workstation:review-stable",
        "work-state:story-stable:ready-stable",
        "work-state:story-stable:done-stable",
        "work-state:story-stable:failed-stable",
      ]),
    );
    expect(historical.world.factoryReplay.workProgress.counts.failed).toBe(1);

    const repeated = replayMeasurement(recording.events, completed.checkpoint);
    expect(repeated.appliedEventIDs).toEqual([]);
    expect(repeated.world).toEqual(completed.world);

    const retained = retainedGraphMeasurement(
      recording.events,
      repeated.checkpoint,
      repeated.world,
    );
    expect(retained.checkpointCount).toBe(1);
    expect(retained.worldViewCacheEntries).toBeLessThanOrEqual(
      MAX_TIMELINE_WORLD_VIEW_CACHE_ENTRIES,
    );
    expect(retained.retainedBytes).toBeGreaterThan(0);
  });

  it("reports the measured retained bytes when the configured bound is exceeded", () => {
    expect(() => assertRetainedBudget(101, 100)).toThrow(
      "Factory replay retained-memory budget exceeded: 101 bytes retained after cleanup; maximum is 100 bytes.",
    );
  });
});
