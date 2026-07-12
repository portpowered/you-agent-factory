import { describe, expect, it } from "vitest";
import { DEFAULT_FACTORY_SESSION_ID } from "../../../../api/session-routing";
import { MULTI_SESSION_TIMELINE_CHECKPOINT_SCENARIO } from "../../../../testing/multi-session-timeline-checkpoint-scenario";
import { createTimelineCheckpointIndexedDBTestDouble } from "../../../../testing/timeline-checkpoint-indexeddb-test-utils";
import type { FactoryTimelineCheckpoint } from "../timeline/storeState";
import {
  clearTimelineCheckpointsForSession,
  peekPersistedTimelineCheckpoint,
  persistTimelineCheckpoint,
} from "../timelineCheckpointPersistence";

const checkpointInsertionOrders = [
  ["A", "B", "C"],
  ["C", "B", "A"],
] as const;

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
}

describe("multi-session timeline checkpoint identity regression", () => {
  it.each(
    checkpointInsertionOrders,
  )("does not choose an arbitrary concrete checkpoint for ~default after %s then %s insertion", async (...order) => {
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
  });

  it.each(
    checkpointInsertionOrders,
  )("keeps B intact when resolved session A is cleared after %s then %s insertion", async (...order) => {
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
  });

  it.each(
    checkpointInsertionOrders,
  )("does not clear either concrete checkpoint for ambiguous ~default after %s then %s insertion", async (...order) => {
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
  });
});
