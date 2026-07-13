import { describe, expect, it } from "vitest";
import {
  createControlledIndexedDBTestDouble,
  flushPromiseContinuations,
} from "../../../../../testing/controlled-indexeddb-test-utils";
import { createMaterializedWorkOutcomeState } from "../../../../work-outcome/public/materializer";
import { emptyReplayWorldState } from "../../timeline/replayWorldStateSupport";
import type { FactoryTimelineCheckpoint } from "../../timeline/storeState";
import {
  persistTimelineCheckpoint,
  readTimelineCheckpoint,
  type TimelineCheckpointStreamIdentity,
} from "../../timelineCheckpointPersistence";

interface StoredCheckpointEnvelope {
  checkpoint?: FactoryTimelineCheckpoint;
  storageKey?: string;
}

const streamIdentity = {
  backendScopeID: "backend-scope-a",
  factorySessionID: "a1b2c3d4-e5f6-4789-a012-3456789abcde",
  logicalSessionKeyID: "logical-default",
  streamGenerationID: "2026-07-13T00:00:00Z",
} satisfies TimelineCheckpointStreamIdentity;

function checkpoint(
  selectedTick: number,
  afterEventId: string,
  afterSequence: number,
): FactoryTimelineCheckpoint {
  const materializedWorkOutcomeState = createMaterializedWorkOutcomeState();
  materializedWorkOutcomeState.counts.completed = selectedTick;
  return {
    afterEventId,
    afterSequence,
    materializedWorkOutcomeState,
    replayState: emptyReplayWorldState(selectedTick),
    selectedTick,
    syncIdentity: {
      backendScopeId: streamIdentity.backendScopeID,
      factorySessionId: streamIdentity.factorySessionID,
      logicalSessionKeyId: streamIdentity.logicalSessionKeyID,
      streamGenerationId: streamIdentity.streamGenerationID,
    },
  };
}

async function readStoredCheckpoint(
  fixture: ReturnType<
    typeof createControlledIndexedDBTestDouble<StoredCheckpointEnvelope>
  >,
) {
  const read = readTimelineCheckpoint(fixture.indexedDB, streamIdentity);
  fixture.controls.succeed("open");
  await flushPromiseContinuations();
  fixture.controls.succeed("get");
  return read;
}

describe("timeline checkpoint same-stream write ordering", () => {
  it("serializes a strictly newer save behind the pending same-stream write", async () => {
    const fixture =
      createControlledIndexedDBTestDouble<StoredCheckpointEnvelope>();
    const first = persistTimelineCheckpoint(
      fixture.indexedDB,
      checkpoint(41, "event-41", 41),
      streamIdentity,
    );
    const newer = persistTimelineCheckpoint(
      fixture.indexedDB,
      checkpoint(52, "event-52", 52),
      streamIdentity,
    );

    expect(fixture.controls.pendingOperations()).toEqual(["open"]);
    fixture.controls.succeed("open");
    await flushPromiseContinuations();
    fixture.controls.succeed("put");
    expect(fixture.controls.pendingOperations()).toEqual([]);
    fixture.controls.completeTransaction();
    await flushPromiseContinuations();
    await flushPromiseContinuations();
    await flushPromiseContinuations();
    await flushPromiseContinuations();

    expect(fixture.controls.pendingOperations()).toEqual(["open"]);
    fixture.controls.succeed("open");
    await flushPromiseContinuations();
    fixture.controls.succeed("put");
    fixture.controls.completeTransaction();
    await Promise.all([first, newer]);

    await expect(readStoredCheckpoint(fixture)).resolves.toMatchObject({
      afterEventId: "event-52",
      afterSequence: 52,
      selectedTick: 52,
    });
  });

  it("keeps the newer complete envelope when an older save starts last", async () => {
    const fixture =
      createControlledIndexedDBTestDouble<StoredCheckpointEnvelope>();
    const newer = checkpoint(52, "event-52", 52);
    const older = checkpoint(41, "event-41", 41);

    const newerWrite = persistTimelineCheckpoint(
      fixture.indexedDB,
      newer,
      streamIdentity,
    );
    const olderWrite = persistTimelineCheckpoint(
      fixture.indexedDB,
      older,
      streamIdentity,
    );
    await flushPromiseContinuations();

    expect(fixture.controls.pendingOperations()).toEqual(["open"]);
    fixture.controls.succeed("open");
    await flushPromiseContinuations();
    expect(fixture.controls.pendingOperations()).toEqual(["put"]);
    fixture.controls.succeed("put");
    fixture.controls.completeTransaction();
    await Promise.all([newerWrite, olderWrite]);

    await expect(readStoredCheckpoint(fixture)).resolves.toMatchObject({
      afterEventId: "event-52",
      afterSequence: 52,
      materializedWorkOutcomeState: { counts: { completed: 52 } },
      replayState: { tick_count: 52 },
      selectedTick: 52,
      syncIdentity: newer.syncIdentity,
    });
  });

  it("does not admit an equal sequence while that sequence is pending", async () => {
    const fixture =
      createControlledIndexedDBTestDouble<StoredCheckpointEnvelope>();
    const first = persistTimelineCheckpoint(
      fixture.indexedDB,
      checkpoint(52, "event-52", 52),
      streamIdentity,
    );
    const equal = persistTimelineCheckpoint(
      fixture.indexedDB,
      checkpoint(53, "different-event-at-52", 52),
      streamIdentity,
    );
    await flushPromiseContinuations();

    expect(fixture.controls.pendingOperations()).toEqual(["open"]);
    fixture.controls.succeed("open");
    await flushPromiseContinuations();
    fixture.controls.succeed("put");
    fixture.controls.completeTransaction();
    await Promise.all([first, equal]);

    await expect(readStoredCheckpoint(fixture)).resolves.toMatchObject({
      afterEventId: "event-52",
      afterSequence: 52,
      selectedTick: 52,
    });
  });
});
