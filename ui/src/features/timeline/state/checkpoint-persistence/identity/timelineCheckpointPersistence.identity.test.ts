import { describe, expect, it } from "vitest";
import { createTimelineCheckpointIndexedDBTestDouble } from "../../../../../testing/timeline-checkpoint-indexeddb-test-utils";
import { createMaterializedWorkOutcomeState } from "../../../../work-outcome/public/materializer";
import { emptyReplayWorldState } from "../../timeline/replayWorldStateSupport";
import type { FactoryTimelineCheckpoint } from "../../timeline/storeState";
import {
  persistTimelineCheckpoint,
  readTimelineCheckpoint,
  type TimelineCheckpointStreamIdentity,
} from "../../timelineCheckpointPersistence";

const STREAM_IDENTITY = {
  backendScopeID: "backend-a",
  factorySessionID: "a1b2c3d4-e5f6-4789-a012-3456789abcde",
  logicalSessionKeyID: "logical-a",
  streamGenerationID: "generation-a",
} satisfies TimelineCheckpointStreamIdentity;

function checkpointFixture(): FactoryTimelineCheckpoint {
  return {
    materializedWorkOutcomeState: createMaterializedWorkOutcomeState(),
    replayState: emptyReplayWorldState(1),
    selectedTick: 1,
    syncIdentity: {
      backendScopeId: STREAM_IDENTITY.backendScopeID,
      factorySessionId: STREAM_IDENTITY.factorySessionID,
      logicalSessionKeyId: STREAM_IDENTITY.logicalSessionKeyID,
      streamGenerationId: STREAM_IDENTITY.streamGenerationID,
    },
  };
}

describe("timeline checkpoint concrete stream identity", () => {
  it.each([
    "session-a",
    "session-a-uuid",
    "not-a-real-session",
    "00000000-0000-0000-0000-000000000000",
  ])(
    "refuses unresolved non-UUID Factory Session identity %s",
    async (factorySessionID) => {
      const { indexedDB, records } =
        createTimelineCheckpointIndexedDBTestDouble();
      const identity = { ...STREAM_IDENTITY, factorySessionID };

      await persistTimelineCheckpoint(indexedDB, checkpointFixture(), identity);

      expect(records.size).toBe(0);
      await expect(
        readTimelineCheckpoint(indexedDB, identity),
      ).resolves.toBeNull();
    },
  );

  it("normalizes whitespace and UUID case before storing and matching identity", async () => {
    const { indexedDB, records } =
      createTimelineCheckpointIndexedDBTestDouble();
    const inputIdentity = {
      backendScopeID: ` ${STREAM_IDENTITY.backendScopeID} `,
      factorySessionID: ` ${STREAM_IDENTITY.factorySessionID.toUpperCase()} `,
      logicalSessionKeyID: ` ${STREAM_IDENTITY.logicalSessionKeyID} `,
      streamGenerationID: ` ${STREAM_IDENTITY.streamGenerationID} `,
    };

    await persistTimelineCheckpoint(
      indexedDB,
      checkpointFixture(),
      inputIdentity,
    );

    expect(records.size).toBe(1);
    await expect(
      readTimelineCheckpoint(indexedDB, STREAM_IDENTITY),
    ).resolves.toEqual(checkpointFixture());
  });

  it("refuses a checkpoint whose reconnect identity differs from its storage identity", async () => {
    const { indexedDB, records } =
      createTimelineCheckpointIndexedDBTestDouble();
    const checkpoint = checkpointFixture();
    if (!checkpoint.syncIdentity) {
      throw new Error("expected checkpoint sync identity");
    }
    checkpoint.syncIdentity.streamGenerationId = "replacement-generation";

    await persistTimelineCheckpoint(indexedDB, checkpoint, STREAM_IDENTITY);

    expect(records.size).toBe(0);
  });
});
