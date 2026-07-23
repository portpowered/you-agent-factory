import { describe, expect, it } from "vitest";
import { DEFAULT_FACTORY_SESSION_ID } from "../../../../../api/session-routing";
import {
  createControlledIndexedDBTestDouble,
  flushPromiseContinuations,
} from "../../../../../testing/controlled-indexeddb-test-utils";
import { createMaterializedWorkOutcomeState } from "../../../../work-outcome/public/materializer";
import { streamDerivedCheckpointStorageKey } from "../../../lib/stream-derived-cache-identity";
import { emptyReplayWorldState } from "../../timeline/replayWorldStateSupport";
import type { FactoryTimelineCheckpoint } from "../../timeline/storeState";
import {
  clearTimelineCheckpoint,
  clearTimelineCheckpointsForSession,
  persistTimelineCheckpoint,
  type TimelineCheckpointStreamIdentity,
} from "../../timelineCheckpointPersistence";

interface StoredCheckpointEnvelope {
  checkpoint: FactoryTimelineCheckpoint;
  schemaVersion: number;
  storageKey: string;
  streamIdentity: TimelineCheckpointStreamIdentity;
}

const SESSION_A = "a1b2c3d4-e5f6-4789-a012-3456789abcde";
const SESSION_B = "b1b2c3d4-e5f6-4789-a012-3456789abcde";
const IDENTITY_A = {
  backendScopeID: "backend-scope-a",
  factorySessionID: SESSION_A,
  logicalSessionKeyID: "logical-default",
  streamGenerationID: "generation-current",
} satisfies TimelineCheckpointStreamIdentity;
const IDENTITY_B = {
  ...IDENTITY_A,
  factorySessionID: SESSION_B,
} satisfies TimelineCheckpointStreamIdentity;

function checkpoint(
  sequence: number,
  eventID: string,
  selectedTick: number,
  identity = IDENTITY_A,
): FactoryTimelineCheckpoint {
  const materializedWorkOutcomeState = createMaterializedWorkOutcomeState();
  materializedWorkOutcomeState.counts.completed = selectedTick;
  return {
    afterEventId: eventID,
    afterSequence: sequence,
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

async function flushPersistence(): Promise<void> {
  for (let index = 0; index < 6; index += 1) {
    await flushPromiseContinuations();
  }
}

describe("timeline checkpoint shared IndexedDB contexts", () => {
  it("keeps the newer whole envelope when the older independent context settles last", async () => {
    const fixture =
      createControlledIndexedDBTestDouble<StoredCheckpointEnvelope>();
    const olderContext = fixture.createIndexedDBContext();
    const newerContext = fixture.createIndexedDBContext();
    let olderSettled = false;

    const older = persistTimelineCheckpoint(
      olderContext,
      checkpoint(41, "older-event", 41),
      IDENTITY_A,
    ).then(() => {
      olderSettled = true;
    });
    const newerCheckpoint = checkpoint(52, "newer-event", 52);
    const newer = persistTimelineCheckpoint(
      newerContext,
      newerCheckpoint,
      IDENTITY_A,
    );

    expect(fixture.controls.pendingOperations()).toEqual(["open", "open"]);
    fixture.controls.succeed("open", 1);
    await flushPromiseContinuations();
    fixture.controls.succeed("get");
    fixture.controls.succeed("put");
    fixture.controls.completeTransaction();
    await newer;
    expect(olderSettled).toBe(false);

    fixture.controls.succeed("open");
    await flushPromiseContinuations();
    fixture.controls.succeed("get");
    expect(fixture.controls.pendingOperations()).not.toContain("put");
    fixture.controls.completeTransaction();
    await older;

    expect(
      fixture.records.get(streamDerivedCheckpointStorageKey(IDENTITY_A)),
    ).toMatchObject({
      checkpoint: {
        afterEventId: "newer-event",
        afterSequence: 52,
        materializedWorkOutcomeState: { counts: { completed: 52 } },
        replayState: { tick_count: 52 },
        selectedTick: 52,
        syncIdentity: newerCheckpoint.syncIdentity,
      },
      streamIdentity: IDENTITY_A,
    });
  });
});

describe("timeline checkpoint shared IndexedDB session isolation", () => {
  it("rejects an equal-sequence envelope from an independent context", async () => {
    const fixture =
      createControlledIndexedDBTestDouble<StoredCheckpointEnvelope>();
    const durableCheckpoint = checkpoint(52, "durable-event", 52);
    const storageKey = streamDerivedCheckpointStorageKey(IDENTITY_A);
    fixture.records.set(storageKey, {
      checkpoint: durableCheckpoint,
      schemaVersion: 4,
      storageKey,
      streamIdentity: IDENTITY_A,
    });

    const competing = persistTimelineCheckpoint(
      fixture.createIndexedDBContext(),
      checkpoint(52, "competing-event", 99),
      IDENTITY_A,
    );
    fixture.controls.succeed("open");
    await flushPromiseContinuations();
    fixture.controls.succeed("get");
    expect(fixture.controls.pendingOperations()).not.toContain("put");
    fixture.controls.completeTransaction();
    await competing;

    expect(fixture.records.get(storageKey)).toMatchObject({
      checkpoint: {
        afterEventId: "durable-event",
        afterSequence: 52,
        materializedWorkOutcomeState: { counts: { completed: 52 } },
        replayState: { tick_count: 52 },
        selectedTick: 52,
        syncIdentity: durableCheckpoint.syncIdentity,
      },
    });
  });

  it("commits overlapping independent-session writes without cross-session fields", async () => {
    const fixture =
      createControlledIndexedDBTestDouble<StoredCheckpointEnvelope>();
    const contextA = fixture.createIndexedDBContext();
    const contextB = fixture.createIndexedDBContext();

    const writeA = persistTimelineCheckpoint(
      contextA,
      checkpoint(61, "event-a", 11, IDENTITY_A),
      IDENTITY_A,
    );
    const writeB = persistTimelineCheckpoint(
      contextB,
      checkpoint(72, "event-b", 22, IDENTITY_B),
      IDENTITY_B,
    );
    fixture.controls.succeed("open");
    fixture.controls.succeed("open");
    await flushPromiseContinuations();
    fixture.controls.succeed("get");
    fixture.controls.succeed("get");
    fixture.controls.succeed("put");
    fixture.controls.succeed("put");
    fixture.controls.completeTransaction(1);
    fixture.controls.completeTransaction();
    await Promise.all([writeA, writeB]);

    expect(
      fixture.records.get(streamDerivedCheckpointStorageKey(IDENTITY_A)),
    ).toMatchObject({
      checkpoint: { afterEventId: "event-a", selectedTick: 11 },
      streamIdentity: IDENTITY_A,
    });
    expect(
      fixture.records.get(streamDerivedCheckpointStorageKey(IDENTITY_B)),
    ).toMatchObject({
      checkpoint: { afterEventId: "event-b", selectedTick: 22 },
      streamIdentity: IDENTITY_B,
    });
  });
});

describe("timeline checkpoint shared IndexedDB invalidation settlement", () => {
  it("settles a concrete-session clear with no matching records without deleting another session", async () => {
    const fixture =
      createControlledIndexedDBTestDouble<StoredCheckpointEnvelope>();
    const sessionBStorageKey = streamDerivedCheckpointStorageKey(IDENTITY_B);
    fixture.records.set(sessionBStorageKey, {
      checkpoint: checkpoint(90, "session-b-event", 90, IDENTITY_B),
      schemaVersion: 4,
      storageKey: sessionBStorageKey,
      streamIdentity: IDENTITY_B,
    });

    const clearA = clearTimelineCheckpointsForSession(
      fixture.createIndexedDBContext(),
      SESSION_A,
    );
    fixture.controls.succeed("open");
    await flushPromiseContinuations();
    fixture.controls.succeed("getAll");
    await clearA;
    fixture.controls.completeTransaction();

    expect(fixture.records.get(sessionBStorageKey)).toMatchObject({
      checkpoint: { afterEventId: "session-b-event" },
    });
    expect(fixture.controls.closedDatabaseCount()).toBe(1);
  });

  it("keeps durable records when a concrete-session clear is aborted before its read settles", async () => {
    const fixture =
      createControlledIndexedDBTestDouble<StoredCheckpointEnvelope>();
    const sessionAStorageKey = streamDerivedCheckpointStorageKey(IDENTITY_A);
    fixture.records.set(sessionAStorageKey, {
      checkpoint: checkpoint(80, "current-event", 80),
      schemaVersion: 4,
      storageKey: sessionAStorageKey,
      streamIdentity: IDENTITY_A,
    });
    const controller = new AbortController();

    const clearA = clearTimelineCheckpointsForSession(
      fixture.createIndexedDBContext(),
      SESSION_A,
      { signal: controller.signal },
    );
    fixture.controls.succeed("open");
    await flushPromiseContinuations();
    controller.abort();
    fixture.controls.abortTransaction(new Error("clear aborted"));
    await clearA;

    expect(fixture.records.get(sessionAStorageKey)).toMatchObject({
      checkpoint: { afterEventId: "current-event" },
    });
    expect(fixture.controls.closedDatabaseCount()).toBe(1);
  });

  it("keeps durable records when the concrete-session record scan fails", async () => {
    const fixture =
      createControlledIndexedDBTestDouble<StoredCheckpointEnvelope>();
    const sessionAStorageKey = streamDerivedCheckpointStorageKey(IDENTITY_A);
    fixture.records.set(sessionAStorageKey, {
      checkpoint: checkpoint(80, "current-event", 80),
      schemaVersion: 4,
      storageKey: sessionAStorageKey,
      streamIdentity: IDENTITY_A,
    });

    const clearA = clearTimelineCheckpointsForSession(
      fixture.createIndexedDBContext(),
      SESSION_A,
    );
    fixture.controls.succeed("open");
    await flushPromiseContinuations();
    fixture.controls.fail("getAll", new Error("record scan failed"));
    fixture.controls.failTransaction(new Error("transaction failed"));
    await clearA;

    expect(fixture.records.get(sessionAStorageKey)).toMatchObject({
      checkpoint: { afterEventId: "current-event" },
    });
    expect(fixture.controls.closedDatabaseCount()).toBe(1);
  });
});

describe("timeline checkpoint shared IndexedDB invalidation", () => {
  it("isolates stale-generation mutation and exact concrete-session clearing", async () => {
    const fixture =
      createControlledIndexedDBTestDouble<StoredCheckpointEnvelope>();
    const currentCheckpoint = checkpoint(80, "current-event", 80);
    const staleIdentity = {
      ...IDENTITY_A,
      streamGenerationID: "generation-stale",
    };
    fixture.records.set(streamDerivedCheckpointStorageKey(IDENTITY_A), {
      checkpoint: currentCheckpoint,
      schemaVersion: 4,
      storageKey: streamDerivedCheckpointStorageKey(IDENTITY_A),
      streamIdentity: IDENTITY_A,
    });
    fixture.records.set(streamDerivedCheckpointStorageKey(IDENTITY_B), {
      checkpoint: checkpoint(90, "session-b-event", 90, IDENTITY_B),
      schemaVersion: 4,
      storageKey: streamDerivedCheckpointStorageKey(IDENTITY_B),
      streamIdentity: IDENTITY_B,
    });

    await clearTimelineCheckpoint(
      fixture.createIndexedDBContext(),
      IDENTITY_A,
      { requestedSessionID: DEFAULT_FACTORY_SESSION_ID, userInitiated: true },
    );
    expect(fixture.controls.pendingOperations()).toEqual([]);
    expect(
      fixture.records.get(streamDerivedCheckpointStorageKey(IDENTITY_A)),
    ).toMatchObject({ checkpoint: { afterEventId: "current-event" } });

    const staleWrite = persistTimelineCheckpoint(
      fixture.createIndexedDBContext(),
      checkpoint(100, "stale-event", 100, staleIdentity),
      staleIdentity,
    );
    fixture.controls.succeed("open");
    await flushPromiseContinuations();
    fixture.controls.succeed("get");
    fixture.controls.succeed("put");
    fixture.controls.completeTransaction();
    await staleWrite;
    expect(
      fixture.records.get(streamDerivedCheckpointStorageKey(IDENTITY_A)),
    ).toMatchObject({ checkpoint: { afterEventId: "current-event" } });

    const staleClear = clearTimelineCheckpoint(
      fixture.createIndexedDBContext(),
      staleIdentity,
    );
    fixture.controls.succeed("open");
    await flushPromiseContinuations();
    fixture.controls.succeed("delete");
    fixture.controls.completeTransaction();
    await staleClear;
    expect(
      fixture.records.get(streamDerivedCheckpointStorageKey(IDENTITY_A)),
    ).toMatchObject({ checkpoint: { afterEventId: "current-event" } });

    await clearTimelineCheckpointsForSession(
      fixture.createIndexedDBContext(),
      DEFAULT_FACTORY_SESSION_ID,
    );
    expect(fixture.controls.pendingOperations()).toEqual([]);

    const clearA = clearTimelineCheckpointsForSession(
      fixture.createIndexedDBContext(),
      SESSION_A,
    );
    fixture.controls.succeed("open");
    await flushPromiseContinuations();
    fixture.controls.succeed("getAll");
    await flushPromiseContinuations();
    fixture.controls.succeed("delete");
    fixture.controls.completeTransaction();
    await clearA;
    await flushPersistence();

    expect(
      fixture.records.has(streamDerivedCheckpointStorageKey(IDENTITY_A)),
    ).toBe(false);
    expect(
      fixture.records.get(streamDerivedCheckpointStorageKey(IDENTITY_B)),
    ).toMatchObject({ checkpoint: { afterEventId: "session-b-event" } });
  });
});
