import { describe, expect, it } from "vitest";
import { createTimelineCheckpointIndexedDBTestDouble } from "../../../../testing/timeline-checkpoint-indexeddb-test-utils";
import {
  createMaterializedWorkOutcomeState,
  MATERIALIZED_WORK_OUTCOME_RETENTION,
  type MaterializedWorkOutcomeState,
} from "../../../work-outcome/public/materializer";
import { emptyReplayWorldState } from "../timeline/replayWorldStateSupport";
import {
  peekPersistedTimelineCheckpoint,
  persistTimelineCheckpoint,
  readTimelineCheckpoint,
  type TimelineCheckpointStreamIdentity,
} from "../timelineCheckpointPersistence";

const IDENTITY = {
  backendScopeID: "backend-a",
  factorySessionID: "a1b2c3d4-e5f6-4789-a012-3456789abcde",
  logicalSessionKeyID: "logical-a",
  streamGenerationID: "generation-a",
} satisfies TimelineCheckpointStreamIdentity;

const STORAGE_KEY = Object.values(IDENTITY).join("::");

interface StoredEnvelope {
  checkpoint: {
    materializedWorkOutcomeState?: MaterializedWorkOutcomeState;
  };
  schemaVersion: number;
  sessionID?: string;
  storageKey: string;
}

interface MutableMaterializedState extends Record<string, unknown> {
  accumulator: Record<string, unknown> & {
    activeDispatchesByID: unknown;
  };
  counts: Record<string, unknown>;
  failedByWorkType: unknown;
  failedWorkLabels: unknown;
  samples: unknown;
  version: unknown;
}

describe("materialized timeline checkpoint schema migration", () => {
  it.each([1, 2, 3, 999])(
    "deletes non-migratable envelope schema version %i",
    async (schemaVersion) => {
      const fixture = await persistedFixture();
      storedEnvelope(fixture.records).schemaVersion = schemaVersion;

      await expect(
        readTimelineCheckpoint(fixture.indexedDB, IDENTITY),
      ).resolves.toBeNull();
      expect(fixture.records.has(STORAGE_KEY)).toBe(false);
    },
  );

  it.each([
    {
      label: "missing materialized state",
      mutate: (envelope: StoredEnvelope) => {
        delete envelope.checkpoint.materializedWorkOutcomeState;
      },
    },
    {
      label: "missing accumulator",
      mutate: (envelope: StoredEnvelope) => {
        Reflect.deleteProperty(requiredState(envelope), "accumulator");
      },
    },
    {
      label: "malformed counts",
      mutate: (envelope: StoredEnvelope) => {
        const state = requiredState(envelope);
        state.counts = { ...state.counts, queued: "1" };
      },
    },
    {
      label: "missing breakdown",
      mutate: (envelope: StoredEnvelope) => {
        Reflect.deleteProperty(requiredState(envelope), "failedByWorkType");
      },
    },
    {
      label: "malformed sample",
      mutate: (envelope: StoredEnvelope) => {
        requiredState(envelope).samples = [{ tick: 1 }];
      },
    },
    {
      label: "unordered samples",
      mutate: (envelope: StoredEnvelope) => {
        requiredState(envelope).samples = [sample(2), sample(1)];
      },
    },
    {
      label: "unsupported nested version",
      mutate: (envelope: StoredEnvelope) => {
        requiredState(envelope).version = 999;
      },
    },
    {
      label: "over-limit samples",
      mutate: (envelope: StoredEnvelope) => {
        const state = requiredState(envelope);
        state.samples = Array.from(
          { length: MATERIALIZED_WORK_OUTCOME_RETENTION.samples + 1 },
          (_, tick) => sample(tick),
        );
      },
    },
    {
      label: "over-limit breakdown entries",
      mutate: (envelope: StoredEnvelope) => {
        const state = requiredState(envelope);
        state.failedByWorkType = Object.fromEntries(
          Array.from(
            {
              length: MATERIALIZED_WORK_OUTCOME_RETENTION.breakdownEntries + 1,
            },
            (_, index) => [`type-${index}`, index],
          ),
        );
      },
    },
    {
      label: "over-limit labels",
      mutate: (envelope: StoredEnvelope) => {
        const state = requiredState(envelope);
        state.failedWorkLabels = Array.from(
          { length: MATERIALIZED_WORK_OUTCOME_RETENTION.labels + 1 },
          (_, index) => `label-${index}`,
        );
      },
    },
    {
      label: "over-limit nested IDs",
      mutate: (envelope: StoredEnvelope) => {
        const state = requiredState(envelope);
        state.accumulator.activeDispatchesByID = {
          "dispatch-1": {
            inputWorkIDs: Array.from(
              { length: MATERIALIZED_WORK_OUTCOME_RETENTION.nestedIDs + 1 },
              (_, index) => `work-${index}`,
            ),
            systemOnly: false,
          },
        };
      },
    },
    {
      label: "over-limit text",
      mutate: (envelope: StoredEnvelope) => {
        const state = requiredState(envelope);
        state.failedWorkLabels = [
          "x".repeat(MATERIALIZED_WORK_OUTCOME_RETENTION.textChars + 1),
        ];
      },
    },
  ])("deletes a current envelope with $label", async ({ mutate }) => {
    const fixture = await persistedFixture();
    mutate(storedEnvelope(fixture.records));

    await expect(
      readTimelineCheckpoint(fixture.indexedDB, IDENTITY),
    ).resolves.toBeNull();
    expect(fixture.records.has(STORAGE_KEY)).toBe(false);
  });

  it.each([1, 2, 3])(
    "deletes UUID-keyed legacy schema version %i during session preflight",
    async (schemaVersion) => {
      const fixture = await persistedFixture();
      const envelope = storedEnvelope(fixture.records);
      envelope.schemaVersion = schemaVersion;
      envelope.sessionID = IDENTITY.factorySessionID;
      Reflect.deleteProperty(envelope, "streamIdentity");

      await expect(
        peekPersistedTimelineCheckpoint(
          fixture.indexedDB,
          IDENTITY.factorySessionID,
        ),
      ).resolves.toBeNull();
      expect(fixture.records.has(STORAGE_KEY)).toBe(false);
    },
  );
});

async function persistedFixture() {
  const fixture = createTimelineCheckpointIndexedDBTestDouble();
  await persistTimelineCheckpoint(
    fixture.indexedDB,
    {
      materializedWorkOutcomeState: createMaterializedWorkOutcomeState(),
      replayState: emptyReplayWorldState(0),
      selectedTick: 0,
    },
    IDENTITY,
  );
  return fixture;
}

function storedEnvelope(
  records: Map<string, { storageKey?: string }>,
): StoredEnvelope {
  const envelope = records.get(STORAGE_KEY) as StoredEnvelope | undefined;
  if (!envelope) {
    throw new Error("expected a persisted timeline checkpoint");
  }
  return envelope;
}

function requiredState(envelope: StoredEnvelope): MutableMaterializedState {
  const state = envelope.checkpoint.materializedWorkOutcomeState;
  if (!state) {
    throw new Error("expected materialized work-outcome state");
  }
  return state as unknown as MutableMaterializedState;
}

function sample(tick: number) {
  return {
    completedCount: tick,
    dispatchedCount: tick,
    failedByWorkType: {},
    failedCount: tick,
    failedWorkLabels: [],
    inFlightCount: tick,
    observedAt: tick,
    queuedCount: tick,
    tick,
  };
}
