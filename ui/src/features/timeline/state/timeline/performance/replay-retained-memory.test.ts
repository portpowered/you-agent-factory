import { parseFactoryRecording } from "@you-agent-factory/client";
import { advanceFactoryReplay } from "@you-agent-factory/factory-replay";
import { describe, expect, it } from "vitest";

import customerSupportRecording from "../../../../../../packages/client/examples/customer-support.factory-recording.v1.json";
import { hostedFactoryReplayReducer } from "../buildSnapshot";
import { projectSnapshot } from "../projectSnapshot";
import { emptyReplayWorldState } from "../replayWorldStateSupport";

const EVENT_COUNT = 10_000;
const MAX_RETAINED_BYTES = 2_000_000;

function deterministicRecording() {
  const recording = structuredClone(customerSupportRecording);
  recording.id = "retained-memory-10k-recording";
  recording.title = "Deterministic retained-memory recording";
  recording.events.push(
    ...Array.from({ length: EVENT_COUNT - 1 }, (_, index) => {
      const tick = index + 2;
      return {
        context: {
          eventTime: new Date(Date.UTC(2026, 6, 19, 0, 0, tick)).toISOString(),
          sequence: tick,
          sessionId: "session-customer-support-example",
          sessionSequence: tick,
          tick,
        },
        id: `memory-event-${tick}`,
        payload: { state: tick % 2 === 0 ? "RUNNING" : "PAUSED" },
        schemaVersion: "agent-factory.event.v1",
        type: "FACTORY_STATE_RESPONSE",
      };
    }),
  );
  return parseFactoryRecording(recording);
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
    JSON.stringify(result.checkpoint),
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
    const recording = deterministicRecording();
    expect(recording.events).toHaveLength(EVENT_COUNT);
    const events = recording.events;
    const measurements = [];
    let checkpoint: Parameters<typeof replayMeasurement>[1];
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
    expect(
      measurements.map(({ acceptedEventIDs }) => acceptedEventIDs),
    ).toEqual([
      measurements[0]?.acceptedEventIDs,
      measurements[0]?.acceptedEventIDs,
      measurements[0]?.acceptedEventIDs,
    ]);
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
