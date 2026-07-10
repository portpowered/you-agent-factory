import { describe, expect, it } from "vitest";
import {
  createControlledIndexedDBTestDouble,
  flushPromiseContinuations,
} from "../../../../testing/controlled-indexeddb-test-utils";
import {
  readSessionPersistenceInvalidationRecords,
  resetSessionPersistenceInvalidationRecords,
} from "../../../dashboard/lib/session-persistence/diagnostics";
import { emptyReplayWorldState } from "../timeline/replayWorldStateSupport";
import type { FactoryTimelineCheckpoint } from "../timeline/storeState";
import {
  clearTimelineCheckpointsForSession,
  persistTimelineCheckpoint,
  readTimelineCheckpoint,
  type TimelineCheckpointStreamIdentity,
} from "../timelineCheckpointPersistence";

interface StoredCheckpointEnvelope {
  checkpoint?: {
    afterEventId?: string;
    afterSequence?: number;
    replayState?: object;
    selectedTick: number;
  };
  schemaVersion?: number;
  storageKey?: string;
  streamIdentity?: TimelineCheckpointStreamIdentity;
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

    controls.fail("put", new Error("replacement put failed"), 1);
    await replacementWrite;
    expect(records.size).toBe(0);

    controls.succeed("put");
    await firstWrite;
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

    const cleanup = clearTimelineCheckpointsForSession(
      indexedDB,
      streamIdentity.factorySessionID,
    );
    controls.succeed("open");
    await flushPromiseContinuations();
    controls.succeed("getAll");
    await flushPromiseContinuations();
    controls.succeed("open");
    await flushPromiseContinuations();
    controls.succeed("delete");
    await cleanup;

    expect(records.size).toBe(0);
  });

  it("does not diagnose or delete a held read after cancellation", async () => {
    resetSessionPersistenceInvalidationRecords();
    const { controls, indexedDB, records } =
      createControlledIndexedDBTestDouble<StoredCheckpointEnvelope>();
    const write = persistTimelineCheckpoint(
      indexedDB,
      checkpoint(7, "event-7", 42),
      streamIdentity,
    );
    controls.succeed("open");
    await flushPromiseContinuations();
    controls.succeed("put");
    await write;
    const stored = [...records.entries()][0];
    if (!stored) {
      throw new Error("expected committed checkpoint fixture");
    }
    stored[1].streamIdentity = {
      ...streamIdentity,
      streamGenerationID: "superseded-generation",
    };

    const abortController = new AbortController();
    const read = readTimelineCheckpoint(indexedDB, streamIdentity, {
      signal: abortController.signal,
    });
    controls.succeed("open");
    await flushPromiseContinuations();
    expect(controls.pendingOperations()).toEqual(["get"]);
    abortController.abort();
    controls.succeed("get");

    await expect(read).resolves.toBeNull();
    expect(records.has(stored[0])).toBe(true);
    expect(controls.pendingOperations()).toEqual([]);
    expect(readSessionPersistenceInvalidationRecords()).toEqual([]);
    resetSessionPersistenceInvalidationRecords();
  });
});
