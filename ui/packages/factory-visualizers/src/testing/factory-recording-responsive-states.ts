export function playbackRecording(): unknown {
  const factory = supportFactory();
  const context = (sequence: number, tick: number, dispatchId?: string) => ({
    ...(dispatchId ? { dispatchId, workIds: ["support-1"] } : {}),
    eventTime: `2026-07-18T20:00:0${sequence}Z`,
    sequence,
    sessionId: "storybook-playback-session",
    sessionSequence: sequence,
    tick,
  });
  return {
    events: [
      {
        context: context(1, 1),
        id: "playback-topology",
        payload: { factory },
        schemaVersion: "agent-factory.event.v1",
        type: "INITIAL_STRUCTURE_REQUEST",
      },
      {
        context: context(2, 3),
        id: "playback-work",
        payload: {
          type: "FACTORY_REQUEST_BATCH",
          works: [
            {
              name: "Support request 1",
              workId: "support-1",
              workTypeName: "support-request",
            },
          ],
        },
        schemaVersion: "agent-factory.event.v1",
        type: "WORK_REQUEST",
      },
      {
        context: context(4, 3),
        id: "playback-resolved",
        payload: {
          fromPlaceId: "support-request:failed",
          fromState: "failed",
          source: "api",
          toPlaceId: "support-request:resolved",
          toState: "resolved",
          workId: "support-1",
          workTypeName: "support-request",
        },
        schemaVersion: "agent-factory.event.v1",
        type: "WORK_STATE_CHANGE",
      },
      {
        context: context(5, 3, "playback-dispatch"),
        id: "playback-dispatch",
        payload: {
          inputs: [{ workId: "support-1" }],
          resources: [],
          transitionId: "triage",
        },
        schemaVersion: "agent-factory.event.v1",
        type: "DISPATCH_REQUEST",
      },
      {
        context: context(3, 3),
        id: "playback-failed-first",
        payload: {
          fromPlaceId: "support-request:queued",
          fromState: "queued",
          source: "api",
          toPlaceId: "support-request:failed",
          toState: "failed",
          workId: "support-1",
          workTypeName: "support-request",
        },
        schemaVersion: "agent-factory.event.v1",
        type: "WORK_STATE_CHANGE",
      },
    ],
    factory,
    id: "same-tick-history-current",
    schemaVersion: "factory-recording/v1",
    title: "Same-tick history and current playback",
  };
}

export function terminalRecording(): unknown {
  const factory = supportFactory();
  return {
    events: [
      event(
        "terminal",
        1,
        0,
        "terminal-topology",
        "INITIAL_STRUCTURE_REQUEST",
        {
          factory,
        },
      ),
      event("terminal", 2, 1, "terminal-work", "WORK_REQUEST", {
        type: "FACTORY_REQUEST_BATCH",
        works: [
          {
            name: "Support request 1",
            workId: "support-1",
            workTypeName: "support-request",
          },
        ],
      }),
      event("terminal", 3, 2, "terminal-resolved", "WORK_STATE_CHANGE", {
        fromPlaceId: "support-request:queued",
        fromState: "queued",
        source: "api",
        toPlaceId: "support-request:resolved",
        toState: "resolved",
        workId: "support-1",
        workTypeName: "support-request",
      }),
    ],
    factory,
    id: "terminal-recording",
    schemaVersion: "factory-recording/v1",
    title: "Completed support request",
  };
}

export function dependencyRecording(): unknown {
  const factory = {
    name: "dependency-neutral-topology",
    workTypes: [
      {
        name: "task",
        states: [
          { name: "ready", type: "INITIAL" },
          { name: "complete", type: "TERMINAL" },
        ],
      },
    ],
  };
  const relation = {
    requiredState: "complete",
    sourceWorkName: "dependent",
    targetWorkId: "prerequisite-id",
    targetWorkName: "prerequisite",
    type: "DEPENDS_ON",
  };
  return {
    events: [
      event(
        "dependency",
        1,
        0,
        "dependency-topology",
        "INITIAL_STRUCTURE_REQUEST",
        { factory },
      ),
      event("dependency", 2, 1, "dependency-work", "WORK_REQUEST", {
        relations: [relation],
        type: "FACTORY_REQUEST_BATCH",
        works: [
          { name: "dependent", workId: "dependent-id", workTypeName: "task" },
          {
            name: "prerequisite",
            workId: "prerequisite-id",
            workTypeName: "task",
          },
        ],
      }),
      event(
        "dependency",
        3,
        1,
        "dependency-relationship",
        "RELATIONSHIP_CHANGE_REQUEST",
        { relation },
      ),
    ],
    factory,
    id: "dependency-neutral-recording",
    schemaVersion: "factory-recording/v1",
    title: "Dependency-neutral topology",
  };
}

function supportFactory() {
  return {
    name: "support-playback",
    workers: [{ name: "support-agent" }],
    workTypes: [
      {
        name: "support-request",
        states: [
          { name: "queued", type: "INITIAL" },
          { name: "resolved", type: "TERMINAL" },
          { name: "failed", type: "FAILED" },
        ],
      },
    ],
    workstations: [
      {
        inputs: [{ state: "queued", workType: "support-request" }],
        name: "triage",
        outputs: [{ state: "resolved", workType: "support-request" }],
        worker: "support-agent",
      },
    ],
  };
}

function event(
  session: string,
  sequence: number,
  tick: number,
  id: string,
  type: string,
  payload: object,
) {
  return {
    context: {
      eventTime: `2026-07-18T21:00:0${sequence}Z`,
      sequence,
      sessionId: `storybook-${session}-session`,
      sessionSequence: sequence,
      tick,
    },
    id,
    payload,
    schemaVersion: "agent-factory.event.v1",
    type,
  };
}
