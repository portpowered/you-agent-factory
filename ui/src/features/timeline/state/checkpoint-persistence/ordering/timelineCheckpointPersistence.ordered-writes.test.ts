import { describe, expect, it } from "vitest";
import {
  createControlledIndexedDBTestDouble,
  flushPromiseContinuations,
} from "../../../../../testing/controlled-indexeddb-test-utils";
import { createMaterializedWorkOutcomeState } from "../../../../work-outcome/public/materializer";
import { emptyReplayWorldState } from "../../timeline/replayWorldStateSupport";
import type { FactoryTimelineCheckpoint } from "../../timeline/storeState";
import {
  clearTimelineCheckpoint,
  persistTimelineCheckpoint,
  readTimelineCheckpoint,
  type TimelineCheckpointStreamIdentity,
} from "../../timelineCheckpointPersistence";

interface StoredCheckpointEnvelope {
  checkpoint?: FactoryTimelineCheckpoint;
  schemaVersion?: number;
  storageKey?: string;
  streamIdentity?: TimelineCheckpointStreamIdentity;
}

const streamIdentity = {
  backendScopeID: "backend-scope-a",
  factorySessionID: "a1b2c3d4-e5f6-4789-a012-3456789abcde",
  logicalSessionKeyID: "logical-default",
  streamGenerationID: "2026-07-13T00:00:00Z",
} satisfies TimelineCheckpointStreamIdentity;

function storageKey(identity: TimelineCheckpointStreamIdentity): string {
  return [
    identity.backendScopeID,
    identity.factorySessionID,
    identity.logicalSessionKeyID,
    identity.streamGenerationID,
  ].join("::");
}

function checkpoint(
  selectedTick: number,
  afterEventId: string,
  afterSequence: number,
  identity: TimelineCheckpointStreamIdentity = streamIdentity,
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
      backendScopeId: identity.backendScopeID,
      factorySessionId: identity.factorySessionID,
      logicalSessionKeyId: identity.logicalSessionKeyID,
      streamGenerationId: identity.streamGenerationID,
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

async function releaseAtomicWrite(
  fixture: ReturnType<
    typeof createControlledIndexedDBTestDouble<StoredCheckpointEnvelope>
  >,
  replaces = true,
) {
  fixture.controls.succeed("open");
  await flushPromiseContinuations();
  fixture.controls.succeed("get");
  if (replaces) {
    fixture.controls.succeed("put");
  }
  fixture.controls.completeTransaction();
  await flushPromiseContinuations();
  await flushPromiseContinuations();
  await flushPromiseContinuations();
  await flushPromiseContinuations();
  await flushPromiseContinuations();
  await flushPromiseContinuations();
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
    await releaseAtomicWrite(fixture);

    expect(fixture.controls.pendingOperations()).toEqual(["open"]);
    await releaseAtomicWrite(fixture);
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
    await releaseAtomicWrite(fixture);
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
    await releaseAtomicWrite(fixture);
    await Promise.all([first, equal]);

    await expect(readStoredCheckpoint(fixture)).resolves.toMatchObject({
      afterEventId: "event-52",
      afterSequence: 52,
      selectedTick: 52,
    });
  });
});

describe("timeline checkpoint independent stream writes", () => {
  it.each([
    ["backend scope", { backendScopeID: "backend-scope-b" }],
    [
      "Factory Session",
      { factorySessionID: "b1b2c3d4-e5f6-4789-a012-3456789abcde" },
    ],
    ["logical session key", { logicalSessionKeyID: "logical-other" }],
    ["stream generation", { streamGenerationID: "2026-07-14T00:00:00Z" }],
  ] as const)(
    "allows a stream with a different %s to reach IndexedDB concurrently",
    async (_label, identityDifference) => {
      const fixture =
        createControlledIndexedDBTestDouble<StoredCheckpointEnvelope>();
      const otherIdentity = {
        ...streamIdentity,
        ...identityDifference,
      } satisfies TimelineCheckpointStreamIdentity;
      let firstSettled = false;
      let otherSettled = false;

      const first = persistTimelineCheckpoint(
        fixture.indexedDB,
        checkpoint(61, "event-61", 61),
        streamIdentity,
      ).then(() => {
        firstSettled = true;
      });
      const other = persistTimelineCheckpoint(
        fixture.indexedDB,
        checkpoint(72, "event-72", 72, otherIdentity),
        otherIdentity,
      ).then(() => {
        otherSettled = true;
      });

      expect(fixture.controls.pendingOperations()).toEqual(["open", "open"]);
      fixture.controls.succeed("open");
      fixture.controls.succeed("open");
      await flushPromiseContinuations();
      fixture.controls.succeed("get");
      fixture.controls.succeed("get");
      expect(fixture.controls.pendingOperations()).toEqual(["put", "put"]);

      fixture.controls.succeed("put");
      fixture.controls.succeed("put");
      fixture.controls.completeTransaction(1);
      await flushPromiseContinuations();
      await flushPromiseContinuations();
      await flushPromiseContinuations();
      await flushPromiseContinuations();

      expect(otherSettled).toBe(true);
      expect(firstSettled).toBe(false);
      expect([...fixture.records.values()]).toEqual([
        expect.objectContaining({
          checkpoint: expect.objectContaining({
            afterEventId: "event-72",
            afterSequence: 72,
            materializedWorkOutcomeState: expect.objectContaining({
              counts: expect.objectContaining({ completed: 72 }),
            }),
          }),
          streamIdentity: otherIdentity,
        }),
      ]);

      fixture.controls.failTransaction(new Error("first stream failed"));
      await Promise.all([first, other]);

      expect([...fixture.records.values()]).toHaveLength(1);
    },
  );
});

describe("timeline checkpoint lane lifecycle", () => {
  it("removes idle lane state after the last pending write settles", async () => {
    const fixture =
      createControlledIndexedDBTestDouble<StoredCheckpointEnvelope>();
    const first = persistTimelineCheckpoint(
      fixture.indexedDB,
      checkpoint(81, "event-81", 81),
      streamIdentity,
    );

    await releaseAtomicWrite(fixture);
    await first;
    await flushPromiseContinuations();

    const next = persistTimelineCheckpoint(
      fixture.indexedDB,
      checkpoint(82, "event-82", 82),
      streamIdentity,
    );

    expect(fixture.controls.pendingOperations()).toEqual(["open"]);
    await releaseAtomicWrite(fixture);
    await next;

    expect([...fixture.records.values()]).toEqual([
      expect.objectContaining({
        checkpoint: expect.objectContaining({
          afterEventId: "event-82",
          afterSequence: 82,
        }),
      }),
    ]);
  });
});

describe("timeline checkpoint durable admission lifecycle", () => {
  it("rejects older and equal saves after a newer checkpoint is committed", async () => {
    const fixture =
      createControlledIndexedDBTestDouble<StoredCheckpointEnvelope>();
    const committedCheckpoint = checkpoint(82, "event-82", 82);
    const committed = persistTimelineCheckpoint(
      fixture.indexedDB,
      committedCheckpoint,
      streamIdentity,
    );

    await releaseAtomicWrite(fixture);
    await committed;
    await flushPromiseContinuations();

    const older = persistTimelineCheckpoint(
      fixture.indexedDB,
      checkpoint(81, "stale-event-81", 81),
      streamIdentity,
    );
    const equal = persistTimelineCheckpoint(
      fixture.indexedDB,
      checkpoint(83, "different-event-at-82", 82),
      streamIdentity,
    );
    expect(fixture.controls.pendingOperations()).toEqual(["open"]);
    await releaseAtomicWrite(fixture, false);
    await releaseAtomicWrite(fixture, false);
    await Promise.all([older, equal]);

    expect(fixture.controls.pendingOperations()).toEqual([]);
    expect([...fixture.records.values()]).toEqual([
      expect.objectContaining({
        checkpoint: expect.objectContaining({
          afterEventId: "event-82",
          afterSequence: 82,
          materializedWorkOutcomeState: expect.objectContaining({
            counts: expect.objectContaining({ completed: 82 }),
          }),
          replayState: expect.objectContaining({ tick_count: 82 }),
          selectedTick: 82,
          syncIdentity: committedCheckpoint.syncIdentity,
        }),
        streamIdentity,
      }),
    ]);
  });

  it("reconciles a fresh lane with a checkpoint committed by a prior realm", async () => {
    const fixture =
      createControlledIndexedDBTestDouble<StoredCheckpointEnvelope>();
    const committedCheckpoint = checkpoint(82, "event-82", 82);
    fixture.records.set(storageKey(streamIdentity), {
      checkpoint: committedCheckpoint,
      schemaVersion: 4,
      storageKey: storageKey(streamIdentity),
      streamIdentity,
    });

    const older = persistTimelineCheckpoint(
      fixture.indexedDB,
      checkpoint(81, "stale-event-81", 81),
      streamIdentity,
    );
    const equal = persistTimelineCheckpoint(
      fixture.indexedDB,
      checkpoint(83, "different-event-at-82", 82),
      streamIdentity,
    );

    await releaseAtomicWrite(fixture, false);
    await releaseAtomicWrite(fixture, false);
    await Promise.all([older, equal]);

    expect(fixture.controls.pendingOperations()).toEqual([]);
    expect([...fixture.records.values()]).toEqual([
      expect.objectContaining({
        checkpoint: expect.objectContaining({
          afterEventId: "event-82",
          afterSequence: 82,
          materializedWorkOutcomeState: expect.objectContaining({
            counts: expect.objectContaining({ completed: 82 }),
          }),
          replayState: expect.objectContaining({ tick_count: 82 }),
          selectedTick: 82,
          syncIdentity: committedCheckpoint.syncIdentity,
        }),
        streamIdentity,
      }),
    ]);
  });
});

describe("timeline checkpoint clear lifecycle", () => {
  it("allows a lower replay checkpoint after the durable checkpoint is cleared", async () => {
    const fixture =
      createControlledIndexedDBTestDouble<StoredCheckpointEnvelope>();
    const committed = persistTimelineCheckpoint(
      fixture.indexedDB,
      checkpoint(82, "event-82", 82),
      streamIdentity,
    );
    await releaseAtomicWrite(fixture);
    await committed;
    await flushPromiseContinuations();

    const cleared = clearTimelineCheckpoint(fixture.indexedDB, streamIdentity);
    fixture.controls.succeed("open");
    await flushPromiseContinuations();
    fixture.controls.succeed("delete");
    fixture.controls.completeTransaction();
    await cleared;
    await flushPromiseContinuations();

    const replayCheckpoint = checkpoint(21, "replayed-event-21", 21);
    const replayed = persistTimelineCheckpoint(
      fixture.indexedDB,
      replayCheckpoint,
      streamIdentity,
    );
    await releaseAtomicWrite(fixture);
    await replayed;

    expect([...fixture.records.values()]).toEqual([
      expect.objectContaining({
        checkpoint: expect.objectContaining({
          afterEventId: "replayed-event-21",
          afterSequence: 21,
          materializedWorkOutcomeState: expect.objectContaining({
            counts: expect.objectContaining({ completed: 21 }),
          }),
          replayState: expect.objectContaining({ tick_count: 21 }),
          selectedTick: 21,
          syncIdentity: replayCheckpoint.syncIdentity,
        }),
        streamIdentity,
      }),
    ]);
  });
});
