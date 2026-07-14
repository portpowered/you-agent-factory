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

function checkpoint(): FactoryTimelineCheckpoint {
  return {
    afterEventId: "event-42",
    afterSequence: 42,
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

describe("timeline checkpoint durable transaction settlement", () => {
  it("keeps the write and database connection pending until transaction completion", async () => {
    const fixture = await startWrite();

    fixture.controls.completeTransaction();
    await fixture.write;

    expect(fixture.isSettled()).toBe(true);
    expect(fixture.controls.closedDatabaseCount()).toBe(1);
    expect(fixture.records.size).toBe(1);
  });

  it("settles as a failed best-effort write when the transaction aborts after put success", async () => {
    const fixture = await startWrite();

    fixture.controls.abortTransaction(new Error("transaction aborted"));
    await fixture.write;

    expect(fixture.isSettled()).toBe(true);
    expect(fixture.controls.closedDatabaseCount()).toBe(1);
    expect(fixture.records.size).toBe(0);
  });

  it("settles as a failed best-effort write when the transaction errors after put success", async () => {
    const fixture = await startWrite();

    fixture.controls.failTransaction(new Error("transaction failed"));
    await fixture.write;

    expect(fixture.isSettled()).toBe(true);
    expect(fixture.controls.closedDatabaseCount()).toBe(1);
    expect(fixture.records.size).toBe(0);
  });
});
