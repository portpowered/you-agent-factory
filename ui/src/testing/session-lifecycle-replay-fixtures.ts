import { FACTORY_EVENT_TYPES, type FactoryEvent } from "../api/events";

export const SESSION_LIFECYCLE_REPLAY_SESSION_ID = "session-alpha";

export function sessionLifecycleReplayEvent(
  type: FactoryEvent["type"],
  id: string,
  sequence: number,
  tick: number,
  payload: Record<string, unknown>,
): FactoryEvent {
  return {
    context: {
      eventTime: "2026-06-09T12:00:00Z",
      orchestratorKind: "JAVASCRIPT",
      sequence,
      sessionId: SESSION_LIFECYCLE_REPLAY_SESSION_ID,
      sessionSequence: sequence,
      tick,
    },
    id,
    payload,
    type,
  };
}

export const sessionLifecycleStartedEvent = sessionLifecycleReplayEvent(
  FACTORY_EVENT_TYPES.sessionStarted,
  "session-lifecycle-replay-started",
  1,
  1,
  {
    factoryId: "factory-alpha",
    sourceRef: "workflow/main.js",
    startedAt: "2026-06-09T12:00:00Z",
  },
);

export const sessionLifecyclePausedEvent = sessionLifecycleReplayEvent(
  FACTORY_EVENT_TYPES.sessionPaused,
  "session-lifecycle-replay-paused",
  2,
  2,
  {
    pausedAt: "2026-06-09T12:00:02Z",
    status: "PAUSED",
  },
);

export const sessionLifecycleResumedEvent = sessionLifecycleReplayEvent(
  FACTORY_EVENT_TYPES.sessionResumed,
  "session-lifecycle-replay-resumed",
  3,
  3,
  {
    resumedAt: "2026-06-09T12:00:04Z",
    status: "RUNNING",
  },
);

export const canonicalSessionLifecycleReplayEvents = [
  sessionLifecycleStartedEvent,
  sessionLifecyclePausedEvent,
  sessionLifecycleResumedEvent,
] as const;
