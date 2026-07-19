import { advanceFactoryReplay } from "@you-agent-factory/factory-replay";
import { describe, expect, it } from "vitest";

import type { FactoryEvent } from "../../../../../api/events";
import { hostedFactoryReplayReducer } from "../buildSnapshot";
import { projectSnapshot } from "../projectSnapshot";
import { emptyReplayWorldState } from "../replayWorldStateSupport";

const EVENT_COUNT = 10_000;
const MAX_RETAINED_BYTES = 2_000_000;

function factoryStateEvents(): FactoryEvent[] {
  return Array.from({ length: EVENT_COUNT }, (_, index) => {
    const tick = index + 1;
    return {
      context: {
        eventTime: `2026-07-19T00:${String(Math.floor(tick / 60)).padStart(2, "0")}:${String(tick % 60).padStart(2, "0")}Z`,
        sequence: tick,
        tick,
      },
      id: `memory-event-${tick}`,
      payload: { state: tick % 2 === 0 ? "RUNNING" : "PAUSED" },
      type: "FACTORY_STATE_RESPONSE",
    };
  });
}

function replayMeasurement(
  events: FactoryEvent[],
  checkpoint = {
    acceptedEventIDs: [],
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
    tick: EVENT_COUNT,
  });
  const retainedBytes = new TextEncoder().encode(
    JSON.stringify({ events, checkpoint: result.checkpoint }),
  ).byteLength;

  return {
    acceptedEventIDs: result.checkpoint.acceptedEventIDs,
    checkpoint: result.checkpoint,
    retainedBytes,
    world: projectSnapshot(result.state),
  };
}

describe("hosted replay retained-state budget", () => {
  it("replays 10,000 events deterministically within the retained-state budget", () => {
    const events = factoryStateEvents();
    const measurements = [];
    let checkpoint = undefined;
    for (let run = 0; run < 3; run += 1) {
      const measurement = replayMeasurement(events, checkpoint);
      measurements.push(measurement);
      checkpoint = measurement.checkpoint;
    }

    expect(measurements.map(({ retainedBytes }) => retainedBytes)).toEqual([
      measurements[0]?.retainedBytes,
      measurements[0]?.retainedBytes,
      measurements[0]?.retainedBytes,
    ]);
    expect(measurements[0]?.retainedBytes).toBeLessThanOrEqual(
      MAX_RETAINED_BYTES,
    );
    expect(measurements.map(({ acceptedEventIDs }) => acceptedEventIDs)).toEqual(
      [
        measurements[0]?.acceptedEventIDs,
        measurements[0]?.acceptedEventIDs,
        measurements[0]?.acceptedEventIDs,
      ],
    );
    expect(measurements[0]?.acceptedEventIDs).toHaveLength(EVENT_COUNT);
    expect(measurements.map(({ world }) => world)).toEqual([
      measurements[0]?.world,
      measurements[0]?.world,
      measurements[0]?.world,
    ]);
    expect(measurements[0]?.world).toMatchObject({
      factory_state: "RUNNING",
      tick_count: EVENT_COUNT,
    });
  });
});
