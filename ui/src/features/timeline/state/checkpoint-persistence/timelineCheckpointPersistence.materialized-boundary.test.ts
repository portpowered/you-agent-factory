import { expect, it } from "vitest";
import { createTimelineCheckpointIndexedDBTestDouble } from "../../../../testing/timeline-checkpoint-indexeddb-test-utils";
import { emptyReplayWorldState } from "../timeline/replayWorldStateSupport";
import {
  readTimelineCheckpoint,
  type TimelineCheckpointStreamIdentity,
} from "../timelineCheckpointPersistence";

interface ReplayOnlyCheckpointEnvelope {
  checkpoint: {
    afterEventId: string;
    afterSequence: number;
    replayState: ReturnType<typeof emptyReplayWorldState>;
    selectedTick: number;
  };
  schemaVersion: number;
  storageKey: string;
  streamIdentity: TimelineCheckpointStreamIdentity;
}

it("deletes replay-only v3 checkpoints without a materialized boundary", async () => {
  const fixture = createTimelineCheckpointIndexedDBTestDouble();
  const records = fixture.records as Map<string, ReplayOnlyCheckpointEnvelope>;
  const streamIdentity = {
    backendScopeID: "backend-a",
    factorySessionID: "a1b2c3d4-e5f6-4789-a012-3456789abcde",
    logicalSessionKeyID: "logical-a",
    streamGenerationID: "generation-a",
  } satisfies TimelineCheckpointStreamIdentity;
  const storageKey = Object.values(streamIdentity).join("::");
  records.set(storageKey, {
    checkpoint: {
      afterEventId: "event-7",
      afterSequence: 7,
      replayState: emptyReplayWorldState(7),
      selectedTick: 7,
    },
    schemaVersion: 3,
    storageKey,
    streamIdentity,
  });

  await expect(
    readTimelineCheckpoint(fixture.indexedDB, streamIdentity),
  ).resolves.toBeNull();
  expect(records.has(storageKey)).toBe(false);
});
