import type { FactoryRecording } from "@you-agent-factory/client";
import { describe, expect, it } from "vitest";

import {
  createRecordingProjectionCache,
  MAX_CACHED_RECORDING_PROJECTIONS,
  projectRecordingAtTick,
} from "./factory-recording-topology-replay";

const events = [
  {
    context: {
      eventTime: "2026-07-20T00:00:00Z",
      sequence: 1,
      sessionId: "projection-cache-session",
      sessionSequence: 1,
      tick: 0,
    },
    id: "projection-cache-event",
    payload: { state: "RUNNING" },
    schemaVersion: "agent-factory.event.v1",
    type: "FACTORY_STATE_RESPONSE",
  },
] satisfies FactoryRecording["events"];

describe("FactoryRecordingTopologyReplay projection cache retention", () => {
  it("bounds retained historical projections across repeated selections", () => {
    const cache = createRecordingProjectionCache();

    for (let tick = 0; tick < 100; tick += 1) {
      projectRecordingAtTick(events, tick, cache);
    }

    expect(cache.projections.size).toBe(MAX_CACHED_RECORDING_PROJECTIONS);
    expect(cache.projections.has(99)).toBe(true);
    expect(cache.projections.has(0)).toBe(false);
  });
});
