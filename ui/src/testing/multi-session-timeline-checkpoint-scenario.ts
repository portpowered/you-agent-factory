import type {
  FactoryTimelineCheckpoint,
  TimelineCheckpointStreamIdentity,
} from "../features/timeline/public";
import { emptyReplayWorldState } from "../features/timeline/state/timeline/replayWorldStateSupport";
import { createMaterializedWorkOutcomeState } from "../features/work-outcome/public/materializer";

export interface MultiSessionTimelineCheckpointFixture {
  checkpoint: FactoryTimelineCheckpoint;
  eventCount: number;
  label: "A" | "B" | "C";
  streamIdentity: TimelineCheckpointStreamIdentity;
}

function createSessionFixture({
  afterEventId,
  afterSequence,
  eventCount,
  factorySessionID,
  label,
  logicalSessionKeyID,
  selectedTick,
  streamGenerationID,
}: {
  afterEventId: string;
  afterSequence: number;
  eventCount: number;
  factorySessionID: string;
  label: MultiSessionTimelineCheckpointFixture["label"];
  logicalSessionKeyID: string;
  selectedTick: number;
  streamGenerationID: string;
}): MultiSessionTimelineCheckpointFixture {
  const backendScopeID = "checkpoint-regression-backend";
  const replayState = emptyReplayWorldState(selectedTick);
  replayState.runtime.session.dispatched_count = eventCount;
  replayState.runtime.session.has_data = true;

  const streamIdentity = {
    backendScopeID,
    factorySessionID,
    logicalSessionKeyID,
    streamGenerationID,
  } satisfies TimelineCheckpointStreamIdentity;

  return {
    checkpoint: {
      afterEventId,
      afterSequence,
      materializedWorkOutcomeState: createMaterializedWorkOutcomeState(),
      replayState,
      selectedTick,
      syncIdentity: {
        backendScopeId: backendScopeID,
        factorySessionId: factorySessionID,
        logicalSessionKeyId: logicalSessionKeyID,
        streamGenerationId: streamGenerationID,
      },
    },
    eventCount,
    label,
    streamIdentity,
  };
}

export const MULTI_SESSION_TIMELINE_CHECKPOINT_SCENARIO = {
  A: createSessionFixture({
    afterEventId: "session-a-event-7",
    afterSequence: 17,
    eventCount: 3,
    factorySessionID: "11111111-1111-4111-8111-111111111111",
    label: "A",
    logicalSessionKeyID: "logical-session-a",
    selectedTick: 7,
    streamGenerationID: "2026-07-10T10:00:00Z",
  }),
  B: createSessionFixture({
    afterEventId: "session-b-event-13",
    afterSequence: 29,
    eventCount: 5,
    factorySessionID: "22222222-2222-4222-8222-222222222222",
    label: "B",
    logicalSessionKeyID: "logical-session-b",
    selectedTick: 13,
    streamGenerationID: "2026-07-10T11:00:00Z",
  }),
  C: createSessionFixture({
    afterEventId: "session-c-event-19",
    afterSequence: 41,
    eventCount: 7,
    factorySessionID: "33333333-3333-4333-8333-333333333333",
    label: "C",
    logicalSessionKeyID: "logical-session-c",
    selectedTick: 19,
    streamGenerationID: "2026-07-10T12:00:00Z",
  }),
} satisfies Record<"A" | "B" | "C", MultiSessionTimelineCheckpointFixture>;
