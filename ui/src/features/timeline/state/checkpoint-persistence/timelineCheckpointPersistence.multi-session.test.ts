import { describe, expect, it } from "vitest";
import { DEFAULT_FACTORY_SESSION_ID } from "../../../../api/session-routing";
import { MULTI_SESSION_TIMELINE_CHECKPOINT_SCENARIO } from "../../../../testing/multi-session-timeline-checkpoint-scenario";
import { createTimelineCheckpointIndexedDBTestDouble } from "../../../../testing/timeline-checkpoint-indexeddb-test-utils";
import { streamDerivedCheckpointStorageKey } from "../../lib/stream-derived-cache-identity";
import type { FactoryTimelineCheckpoint } from "../timeline/storeState";
import {
  clearTimelineCheckpointsForSession,
  peekPersistedTimelineCheckpoint,
  persistTimelineCheckpoint,
  readTimelineCheckpoint,
} from "../timelineCheckpointPersistence";

const checkpointInsertionOrders = [
  ["A", "B", "C"],
  ["C", "B", "A"],
] as const;

const unresolvedSessionIDs = [null, "", "   ", DEFAULT_FACTORY_SESSION_ID];

async function persistScenarioInOrder(
  indexedDB: IDBFactory,
  order: (typeof checkpointInsertionOrders)[number],
): Promise<void> {
  for (const sessionLabel of order) {
    const session = MULTI_SESSION_TIMELINE_CHECKPOINT_SCENARIO[sessionLabel];
    await persistTimelineCheckpoint(
      indexedDB,
      session.checkpoint,
      session.streamIdentity,
    );
  }
}

async function seedCrossKeyIdentityMismatch(
  indexedDB: IDBFactory,
  records: Map<string, { storageKey?: string }>,
  options: { includeLegacySessionID?: boolean } = {},
): Promise<{ invalidStorageKey: string; validStorageKey: string }> {
  const { A, B } = MULTI_SESSION_TIMELINE_CHECKPOINT_SCENARIO;
  const invalidStorageKey = streamDerivedCheckpointStorageKey(A.streamIdentity);
  const validStorageKey = streamDerivedCheckpointStorageKey(B.streamIdentity);
  await persistTimelineCheckpoint(indexedDB, B.checkpoint, B.streamIdentity);
  const validEnvelope = records.get(validStorageKey);
  if (!validEnvelope) {
    throw new Error("expected persisted session B checkpoint fixture");
  }

  records.clear();
  records.set(invalidStorageKey, {
    ...validEnvelope,
    ...(options.includeLegacySessionID
      ? { sessionID: A.streamIdentity.factorySessionID }
      : {}),
    storageKey: invalidStorageKey,
  });
  records.set(validStorageKey, validEnvelope);
  return { invalidStorageKey, validStorageKey };
}

function expectCheckpointToMatchScenario(
  checkpoint: FactoryTimelineCheckpoint | null | undefined,
  session: (typeof MULTI_SESSION_TIMELINE_CHECKPOINT_SCENARIO)["A" | "B" | "C"],
): void {
  expect(checkpoint?.selectedTick).toBe(session.checkpoint.selectedTick);
  expect(checkpoint?.afterEventId).toBe(session.checkpoint.afterEventId);
  expect(checkpoint?.afterSequence).toBe(session.checkpoint.afterSequence);
  expect(checkpoint?.replayState.runtime.session.dispatched_count).toBe(
    session.eventCount,
  );
  expect(checkpoint?.materializedWorkOutcomeState).toEqual(
    session.checkpoint.materializedWorkOutcomeState,
  );
}

describe("multi-session timeline checkpoint identity regression", () => {
  it("preflight deletes a key-mismatched envelope without deleting the valid derived-key checkpoint", async () => {
    const { indexedDB, records } =
      createTimelineCheckpointIndexedDBTestDouble();
    const { B } = MULTI_SESSION_TIMELINE_CHECKPOINT_SCENARIO;
    const { invalidStorageKey, validStorageKey } =
      await seedCrossKeyIdentityMismatch(indexedDB, records);

    await expect(
      peekPersistedTimelineCheckpoint(
        indexedDB,
        B.streamIdentity.factorySessionID,
      ),
    ).resolves.toBe(null);
    expect(records.has(invalidStorageKey)).toBe(false);
    expect(records.has(validStorageKey)).toBe(true);
    expectCheckpointToMatchScenario(
      (
        await peekPersistedTimelineCheckpoint(
          indexedDB,
          B.streamIdentity.factorySessionID,
        )
      )?.checkpoint,
      B,
    );
  });

  it.each(checkpointInsertionOrders)(
    "does not choose an arbitrary concrete checkpoint for ~default after %s then %s insertion",
    async (...order) => {
      const { indexedDB } = createTimelineCheckpointIndexedDBTestDouble();
      const { A, B, C } = MULTI_SESSION_TIMELINE_CHECKPOINT_SCENARIO;
      await persistScenarioInOrder(indexedDB, order);

      const selectedA = await peekPersistedTimelineCheckpoint(
        indexedDB,
        A.streamIdentity.factorySessionID,
      );
      const selectedB = await peekPersistedTimelineCheckpoint(
        indexedDB,
        B.streamIdentity.factorySessionID,
      );
      const selectedC = await peekPersistedTimelineCheckpoint(
        indexedDB,
        C.streamIdentity.factorySessionID,
      );
      expectCheckpointToMatchScenario(selectedA?.checkpoint, A);
      expectCheckpointToMatchScenario(selectedB?.checkpoint, B);
      expectCheckpointToMatchScenario(selectedC?.checkpoint, C);

      await expect(
        peekPersistedTimelineCheckpoint(indexedDB, DEFAULT_FACTORY_SESSION_ID),
      ).resolves.toBe(null);
    },
  );

  it("does not restore the previous generation's materialized cursor or outcome history", async () => {
    const { indexedDB } = createTimelineCheckpointIndexedDBTestDouble();
    const { A, B } = MULTI_SESSION_TIMELINE_CHECKPOINT_SCENARIO;
    const replacementGeneration = {
      ...A.streamIdentity,
      streamGenerationID: "2026-07-11T10:00:00Z",
    };
    await persistTimelineCheckpoint(indexedDB, A.checkpoint, A.streamIdentity);
    await persistTimelineCheckpoint(indexedDB, B.checkpoint, B.streamIdentity);

    await expect(
      readTimelineCheckpoint(indexedDB, replacementGeneration),
    ).resolves.toBeNull();
    expectCheckpointToMatchScenario(
      await readTimelineCheckpoint(indexedDB, A.streamIdentity),
      A,
    );
    expectCheckpointToMatchScenario(
      await readTimelineCheckpoint(indexedDB, B.streamIdentity),
      B,
    );
  });

  it.each(unresolvedSessionIDs)(
    "returns no checkpoint for unresolved session identity %j",
    async (sessionID) => {
      const { indexedDB } = createTimelineCheckpointIndexedDBTestDouble();
      const { A, B } = MULTI_SESSION_TIMELINE_CHECKPOINT_SCENARIO;
      await persistScenarioInOrder(indexedDB, checkpointInsertionOrders[0]);

      await expect(
        peekPersistedTimelineCheckpoint(indexedDB, sessionID),
      ).resolves.toBe(null);
      expectCheckpointToMatchScenario(
        (
          await peekPersistedTimelineCheckpoint(
            indexedDB,
            A.streamIdentity.factorySessionID,
          )
        )?.checkpoint,
        A,
      );
      expectCheckpointToMatchScenario(
        (
          await peekPersistedTimelineCheckpoint(
            indexedDB,
            B.streamIdentity.factorySessionID,
          )
        )?.checkpoint,
        B,
      );
    },
  );
});

describe("multi-session timeline checkpoint targeted invalidation", () => {
  it("session cleanup deletes a key-mismatched envelope without deleting the valid derived-key checkpoint", async () => {
    const { indexedDB, records } =
      createTimelineCheckpointIndexedDBTestDouble();
    const { A, B } = MULTI_SESSION_TIMELINE_CHECKPOINT_SCENARIO;
    const { invalidStorageKey, validStorageKey } =
      await seedCrossKeyIdentityMismatch(indexedDB, records, {
        includeLegacySessionID: true,
      });

    await clearTimelineCheckpointsForSession(
      indexedDB,
      A.streamIdentity.factorySessionID,
    );

    expect(records.has(invalidStorageKey)).toBe(false);
    expect(records.has(validStorageKey)).toBe(true);
    expectCheckpointToMatchScenario(
      (
        await peekPersistedTimelineCheckpoint(
          indexedDB,
          B.streamIdentity.factorySessionID,
        )
      )?.checkpoint,
      B,
    );
  });
});

describe("multi-session timeline checkpoint clearing regression", () => {
  it.each(checkpointInsertionOrders)(
    "keeps B intact when resolved session A is cleared after %s then %s insertion",
    async (...order) => {
      const { indexedDB } = createTimelineCheckpointIndexedDBTestDouble();
      const { A, B, C } = MULTI_SESSION_TIMELINE_CHECKPOINT_SCENARIO;
      await persistScenarioInOrder(indexedDB, order);

      await clearTimelineCheckpointsForSession(
        indexedDB,
        A.streamIdentity.factorySessionID,
      );
      await expect(
        peekPersistedTimelineCheckpoint(
          indexedDB,
          A.streamIdentity.factorySessionID,
        ),
      ).resolves.toBe(null);
      expectCheckpointToMatchScenario(
        (
          await peekPersistedTimelineCheckpoint(
            indexedDB,
            B.streamIdentity.factorySessionID,
          )
        )?.checkpoint,
        B,
      );
      expectCheckpointToMatchScenario(
        (
          await peekPersistedTimelineCheckpoint(
            indexedDB,
            C.streamIdentity.factorySessionID,
          )
        )?.checkpoint,
        C,
      );
    },
  );

  it.each(unresolvedSessionIDs)(
    "does not clear concrete checkpoints for unresolved session identity %j",
    async (sessionID) => {
      const { indexedDB } = createTimelineCheckpointIndexedDBTestDouble();
      const { A, B } = MULTI_SESSION_TIMELINE_CHECKPOINT_SCENARIO;
      await persistScenarioInOrder(indexedDB, checkpointInsertionOrders[1]);

      await clearTimelineCheckpointsForSession(indexedDB, sessionID);

      expectCheckpointToMatchScenario(
        (
          await peekPersistedTimelineCheckpoint(
            indexedDB,
            A.streamIdentity.factorySessionID,
          )
        )?.checkpoint,
        A,
      );
      expectCheckpointToMatchScenario(
        (
          await peekPersistedTimelineCheckpoint(
            indexedDB,
            B.streamIdentity.factorySessionID,
          )
        )?.checkpoint,
        B,
      );
    },
  );

  it.each(checkpointInsertionOrders)(
    "does not clear either concrete checkpoint for ambiguous ~default after %s then %s insertion",
    async (...order) => {
      const { indexedDB } = createTimelineCheckpointIndexedDBTestDouble();
      const { A, B, C } = MULTI_SESSION_TIMELINE_CHECKPOINT_SCENARIO;
      await persistScenarioInOrder(indexedDB, order);

      await clearTimelineCheckpointsForSession(
        indexedDB,
        DEFAULT_FACTORY_SESSION_ID,
      );

      expectCheckpointToMatchScenario(
        (
          await peekPersistedTimelineCheckpoint(
            indexedDB,
            A.streamIdentity.factorySessionID,
          )
        )?.checkpoint,
        A,
      );
      expectCheckpointToMatchScenario(
        (
          await peekPersistedTimelineCheckpoint(
            indexedDB,
            B.streamIdentity.factorySessionID,
          )
        )?.checkpoint,
        B,
      );
      expectCheckpointToMatchScenario(
        (
          await peekPersistedTimelineCheckpoint(
            indexedDB,
            C.streamIdentity.factorySessionID,
          )
        )?.checkpoint,
        C,
      );
    },
  );
});
