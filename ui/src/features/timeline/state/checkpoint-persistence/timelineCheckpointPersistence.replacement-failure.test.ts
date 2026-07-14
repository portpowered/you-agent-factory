import { describe, expect, it } from "vitest";
import {
  createControlledIndexedDBTestDouble,
  flushPromiseContinuations,
} from "../../../../testing/controlled-indexeddb-test-utils";
import {
  readSessionPersistenceDiagnosticRecords,
  readSessionPersistenceInvalidationRecords,
  resetSessionPersistenceDiagnosticRecords,
  resetSessionPersistenceInvalidationRecords,
} from "../../../dashboard/lib/session-persistence/diagnostics";
import { createMaterializedWorkOutcomeState } from "../../../work-outcome/public/materializer";
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
  const materializedWorkOutcomeState = createMaterializedWorkOutcomeState();
  materializedWorkOutcomeState.counts.completed = selectedTick;
  return {
    afterEventId,
    afterSequence,
    materializedWorkOutcomeState,
    replayState: emptyReplayWorldState(selectedTick),
    selectedTick,
  };
}

type ControlledFixture = ReturnType<
  typeof createControlledIndexedDBTestDouble<StoredCheckpointEnvelope>
>;

async function commitCheckpoint(
  fixture: ControlledFixture,
  value: FactoryTimelineCheckpoint,
): Promise<void> {
  const write = persistTimelineCheckpoint(
    fixture.indexedDB,
    value,
    streamIdentity,
  );
  await flushPromiseContinuations();
  await flushPromiseContinuations();
  fixture.controls.succeed("open");
  await flushPromiseContinuations();
  fixture.controls.succeed("get");
  fixture.controls.succeed("put");
  fixture.controls.completeTransaction();
  await write;
}

async function readStoredCheckpoint(fixture: ControlledFixture) {
  const read = readTimelineCheckpoint(fixture.indexedDB, streamIdentity);
  fixture.controls.succeed("open");
  await flushPromiseContinuations();
  fixture.controls.succeed("get");
  return read;
}

async function expectFailedReplacementRecovery(
  failReplacement: (
    fixture: ControlledFixture,
    isReplacementSettled: () => boolean,
  ) => Promise<void>,
): Promise<void> {
  resetSessionPersistenceDiagnosticRecords();
  const fixture =
    createControlledIndexedDBTestDouble<StoredCheckpointEnvelope>();
  const lastKnownGood = checkpoint(7, "event-7", 42);
  const attemptedReplacement = checkpoint(11, "event-11", 51);
  const staleCheckpoint = checkpoint(9, "event-9", 45);
  const recoveredCheckpoint = checkpoint(15, "event-15", 60);

  await commitCheckpoint(fixture, lastKnownGood);
  resetSessionPersistenceDiagnosticRecords();

  let replacementSettled = false;
  const replacementWrite = persistTimelineCheckpoint(
    fixture.indexedDB,
    attemptedReplacement,
    streamIdentity,
  ).finally(() => {
    replacementSettled = true;
  });
  await flushPromiseContinuations();
  await flushPromiseContinuations();
  fixture.controls.succeed("open");
  await flushPromiseContinuations();
  fixture.controls.succeed("get");
  expect(fixture.controls.pendingOperations()).toEqual(["put"]);

  await persistTimelineCheckpoint(
    fixture.indexedDB,
    staleCheckpoint,
    streamIdentity,
  );
  expect(fixture.controls.pendingOperations()).toEqual(["put"]);

  await failReplacement(fixture, () => replacementSettled);
  await replacementWrite;

  expect(readSessionPersistenceDiagnosticRecords()).toEqual([
    expect.objectContaining({
      outcome: "durable_write_failed",
      recoveryAction: "retain_last_committed_checkpoint",
    }),
  ]);

  await expect(readStoredCheckpoint(fixture)).resolves.toMatchObject({
    afterEventId: "event-7",
    afterSequence: 42,
    materializedWorkOutcomeState: { counts: { completed: 7 } },
    replayState: { tick_count: 7 },
    selectedTick: 7,
  });

  await commitCheckpoint(fixture, recoveredCheckpoint);
  expect(readSessionPersistenceDiagnosticRecords()).toEqual([
    expect.objectContaining({ outcome: "durable_write_failed" }),
    expect.objectContaining({
      outcome: "durable_write_succeeded",
      recoveryAction: "none_required",
    }),
  ]);
  await expect(readStoredCheckpoint(fixture)).resolves.toMatchObject({
    afterEventId: "event-15",
    afterSequence: 60,
    materializedWorkOutcomeState: { counts: { completed: 15 } },
    replayState: { tick_count: 15 },
    selectedTick: 15,
  });
}

describe("timeline checkpoint replacement failure", () => {
  it("recovers after a replacement request fails and its transaction aborts", async () => {
    await expectFailedReplacementRecovery(async ({ controls }, isSettled) => {
      controls.fail("put", new Error("replacement put failed"));
      await flushPromiseContinuations();
      expect(isSettled()).toBe(false);
      controls.abortTransaction(new Error("replacement transaction aborted"));
    });
  });

  it("recovers after a replacement transaction fails following put success", async () => {
    await expectFailedReplacementRecovery(async ({ controls, records }) => {
      controls.succeed("put");
      await flushPromiseContinuations();
      expect([...records.values()][0]?.checkpoint).toMatchObject({
        afterEventId: "event-7",
        afterSequence: 42,
        selectedTick: 7,
      });
      controls.failTransaction(new Error("replacement transaction failed"));
    });
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
    controls.succeed("get");
    controls.succeed("put");
    controls.completeTransaction();
    await write;
    resetSessionPersistenceInvalidationRecords();
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
