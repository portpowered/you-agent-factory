import { beforeEach, describe, expect, it } from "vitest";
import {
  createControlledIndexedDBTestDouble,
  flushPromiseContinuations,
} from "../../../../../testing/controlled-indexeddb-test-utils";
import {
  correlationTokenForIdentityScope,
  readSessionPersistenceDiagnosticRecords,
  resetSessionPersistenceDiagnosticRecords,
} from "../../../../dashboard/public/session-persistence-diagnostics";
import { createMaterializedWorkOutcomeState } from "../../../../work-outcome/public/materializer";
import { emptyReplayWorldState } from "../../timeline/replayWorldStateSupport";
import type { FactoryTimelineCheckpoint } from "../../timeline/storeState";
import {
  persistTimelineCheckpoint,
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

function checkpoint(afterSequence = 42): FactoryTimelineCheckpoint {
  return {
    afterEventId: `event-${afterSequence}`,
    afterSequence,
    materializedWorkOutcomeState: createMaterializedWorkOutcomeState(),
    replayState: emptyReplayWorldState(7),
    selectedTick: 7,
  };
}

async function startWrite() {
  const fixture =
    createControlledIndexedDBTestDouble<StoredCheckpointEnvelope>();
  let settled = false;
  const write = persistTimelineCheckpoint(
    fixture.indexedDB,
    checkpoint(),
    streamIdentity,
  ).finally(() => {
    settled = true;
  });

  fixture.controls.succeed("open");
  await flushPromiseContinuations();
  expect(fixture.controls.pendingOperations()).toEqual(["get"]);
  fixture.controls.succeed("get");
  expect(fixture.controls.pendingOperations()).toEqual(["put"]);
  expect(fixture.controls.pendingTransactionCount()).toBe(1);

  fixture.controls.succeed("put");
  await flushPromiseContinuations();
  expect(settled).toBe(false);
  expect(fixture.controls.closedDatabaseCount()).toBe(0);
  expect(fixture.records.size).toBe(0);

  return { ...fixture, isSettled: () => settled, write };
}

beforeEach(() => {
  resetSessionPersistenceDiagnosticRecords();
});

describe("timeline checkpoint durable transaction settlement", () => {
  it("keeps the write and database connection pending until transaction completion", async () => {
    const fixture = await startWrite();

    expect(readSessionPersistenceDiagnosticRecords()).toEqual([]);

    fixture.controls.completeTransaction();
    await fixture.write;

    expect(fixture.isSettled()).toBe(true);
    expect(fixture.controls.closedDatabaseCount()).toBe(1);
    expect(fixture.records.size).toBe(1);
    expect(readSessionPersistenceDiagnosticRecords()).toEqual([
      {
        correlationToken: correlationTokenForIdentityScope(streamIdentity),
        outcome: "durable_write_succeeded",
        recoveryAction: "none_required",
      },
    ]);
  });

  it("reports one failed durable write after a put request and transaction fail", async () => {
    const fixture =
      createControlledIndexedDBTestDouble<StoredCheckpointEnvelope>();
    const write = persistTimelineCheckpoint(
      fixture.indexedDB,
      checkpoint(),
      streamIdentity,
    );

    fixture.controls.succeed("open");
    await flushPromiseContinuations();
    fixture.controls.succeed("get");
    fixture.controls.fail("put", new Error("put failed"));
    await flushPromiseContinuations();
    expect(readSessionPersistenceDiagnosticRecords()).toEqual([]);

    fixture.controls.failTransaction(new Error("transaction failed"));
    await write;

    expect(readSessionPersistenceDiagnosticRecords()).toEqual([
      {
        correlationToken: correlationTokenForIdentityScope(streamIdentity),
        outcome: "durable_write_failed",
        recoveryAction: "retain_last_committed_checkpoint",
      },
    ]);
  });

  it("does not report rejected or invalid persistence candidates", async () => {
    const fixture =
      createControlledIndexedDBTestDouble<StoredCheckpointEnvelope>();
    const committed = persistTimelineCheckpoint(
      fixture.indexedDB,
      checkpoint(),
      streamIdentity,
    );
    fixture.controls.succeed("open");
    await flushPromiseContinuations();
    fixture.controls.succeed("get");
    fixture.controls.succeed("put");
    fixture.controls.completeTransaction();
    await committed;
    resetSessionPersistenceDiagnosticRecords();

    const older = persistTimelineCheckpoint(
      fixture.indexedDB,
      checkpoint(41),
      streamIdentity,
    );
    const equal = persistTimelineCheckpoint(
      fixture.indexedDB,
      checkpoint(),
      streamIdentity,
    );
    fixture.controls.succeed("open");
    await flushPromiseContinuations();
    fixture.controls.succeed("get");
    fixture.controls.completeTransaction();
    await older;
    fixture.controls.succeed("open");
    await flushPromiseContinuations();
    fixture.controls.succeed("get");
    fixture.controls.completeTransaction();

    await Promise.all([
      equal,
      persistTimelineCheckpoint(undefined, checkpoint(43), streamIdentity),
      persistTimelineCheckpoint(fixture.indexedDB, undefined, streamIdentity),
    ]);

    expect(fixture.controls.pendingOperations()).toEqual([]);
    expect(readSessionPersistenceDiagnosticRecords()).toEqual([]);
  });

  it("settles as a failed best-effort write when the transaction aborts after put success", async () => {
    const fixture = await startWrite();

    fixture.controls.abortTransaction(new Error("transaction aborted"));
    await fixture.write;

    expect(fixture.isSettled()).toBe(true);
    expect(fixture.controls.closedDatabaseCount()).toBe(1);
    expect(fixture.records.size).toBe(0);
    expect(readSessionPersistenceDiagnosticRecords()).toEqual([
      expect.objectContaining({
        outcome: "durable_write_failed",
        recoveryAction: "retain_last_committed_checkpoint",
      }),
    ]);
  });

  it("settles as a failed best-effort write when the transaction errors after put success", async () => {
    const fixture = await startWrite();

    fixture.controls.failTransaction(new Error("transaction failed"));
    await fixture.write;

    expect(fixture.isSettled()).toBe(true);
    expect(fixture.controls.closedDatabaseCount()).toBe(1);
    expect(fixture.records.size).toBe(0);
    expect(readSessionPersistenceDiagnosticRecords()).toEqual([
      expect.objectContaining({
        outcome: "durable_write_failed",
        recoveryAction: "retain_last_committed_checkpoint",
      }),
    ]);
  });
});
