import { describe, expect, it } from "vitest";
import {
  createControlledIndexedDBTestDouble,
  flushPromiseContinuations,
} from "../../../../testing/controlled-indexeddb-test-utils";
import { emptyReplayWorldState } from "../timeline/replayWorldStateSupport";
import type { FactoryTimelineCheckpoint } from "../timeline/storeState";
import {
  persistTimelineCheckpoint,
  readTimelineCheckpoint,
  type TimelineCheckpointStreamIdentity,
} from "../timelineCheckpointPersistence";

interface StoredCheckpointEnvelope {
  checkpoint?: {
    afterEventId?: string;
    afterSequence?: number;
    selectedTick: number;
  };
  storageKey?: string;
}

const streamIdentity = {
  backendScopeID: "backend-scope-a",
  factorySessionID: "a1b2c3d4-e5f6-4789-a012-3456789abcde",
  logicalSessionKeyID: "logical-default",
  streamGenerationID: "2026-06-26T00:00:00Z",
} satisfies TimelineCheckpointStreamIdentity;

function checkpoint(
  selectedTick: number,
  afterEventId: string,
  afterSequence: number,
): FactoryTimelineCheckpoint {
  return {
    afterEventId,
    afterSequence,
    replayState: emptyReplayWorldState(selectedTick),
    selectedTick,
  };
}

describe("timeline checkpoint replacement failure", () => {
  it("preserves the last committed checkpoint when a delayed replacement put fails", async () => {
    const { controls, indexedDB, records } =
      createControlledIndexedDBTestDouble<StoredCheckpointEnvelope>();
    const lastKnownGood = checkpoint(7, "event-7", 42);
    const attemptedReplacement = checkpoint(11, "event-11", 51);

    const firstWrite = persistTimelineCheckpoint(
      indexedDB,
      lastKnownGood,
      streamIdentity,
    );
    controls.succeed("open");
    await flushPromiseContinuations();

    const replacementWrite = persistTimelineCheckpoint(
      indexedDB,
      attemptedReplacement,
      streamIdentity,
    );
    controls.succeed("open");
    await flushPromiseContinuations();
    expect(controls.pendingOperations()).toEqual(["put", "put"]);

    controls.succeed("put");
    await firstWrite;
    expect([...records.values()][0]?.checkpoint).toMatchObject({
      afterEventId: "event-7",
      afterSequence: 42,
      selectedTick: 7,
    });

    controls.fail("put", new Error("replacement put failed"));
    await replacementWrite;
    expect(controls.pendingOperations()).not.toContain("delete");
    expect([...records.values()][0]?.checkpoint).toMatchObject({
      afterEventId: "event-7",
      afterSequence: 42,
      selectedTick: 7,
    });

    const restoredCheckpoint = readTimelineCheckpoint(
      indexedDB,
      streamIdentity,
    );
    controls.succeed("open");
    await flushPromiseContinuations();
    controls.succeed("get");

    await expect(restoredCheckpoint).resolves.toMatchObject({
      afterEventId: "event-7",
      afterSequence: 42,
      selectedTick: 7,
    });
    expect(await restoredCheckpoint).not.toMatchObject({
      afterEventId: "event-11",
      afterSequence: 51,
      selectedTick: 11,
    });
  });
});
